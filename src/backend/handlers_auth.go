package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"

	bolt "go.etcd.io/bbolt"
)

// Routes of the auth router, which main.go mounts at authRoutePrefix
const (
	authRoutePrefix     = "/auth"
	googleRoutePrefix   = "/google/"
	googleLoginRoute    = googleRoutePrefix + "login"
	googleCallbackRoute = googleRoutePrefix + "callback"
)

// GoogleCallbackPath is where Google sends the browser back after sign-in
const GoogleCallbackPath = authRoutePrefix + googleCallbackRoute

const googleStateCookieName = "google_login"

const googleStateTTL = 10 * time.Minute

// maxRedirectPathLength keeps the login state cookie well under the 4096
// bytes browsers store per cookie
const maxRedirectPathLength = 1024

// Error codes passed to the login page as ?error=
const (
	loginErrorGoogle       = "google"
	loginErrorGoogleDomain = "google_domain"
)

// HandleIsLoggedIn returns 200 if user is logged in
func HandleIsLoggedIn(w http.ResponseWriter, r *http.Request) {
	// See AuthMi
}

// HandleLogIn verifies password and logs the user in
func HandleLogIn(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if rejectRateLimited(w, r, logger) {
		return
	}

	// Create and store session token
	password := r.FormValue("password")

	var hashedPassword []byte

	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(GlobalBucket))
		hashedPassword = b.Get([]byte("password"))
		return nil
	})
	if err != nil {
		logger.Error("read password hash", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		// Never log the submitted password.
		logger.Warn("login failed", "err", err)
		if recordAuthFailure(w, r, logger) {
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusForbidden)
		writeBody(logger, w, []byte("Incorrect password"))
		return
	}
	globalAuthAttemptLimiter.success(authClient(r))

	c, err := GlobalSessionStorage.New(r, SessionTTL)
	if err != nil {
		logger.Error("create session", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Set session cookie
	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}

// HandleLogOut logs the user out
func HandleLogOut(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	sessionToken, err := r.Cookie("session")
	if err == nil {
		err = GlobalSessionStorage.Delete(sessionToken.Value)
		if err != nil {
			logger.Error("delete session", "err", err)
		}
	}

	// Set session cookie
	cookie := sessionCookie(r)
	cookie.Value = "delete"
	cookie.Expires = time.Unix(0, 0)
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusNoContent)
}

// AuthMethods lists the login methods offered on the login page
type AuthMethods struct {
	Google bool `json:"google"`
}

// HandleAuthMethods returns the available login methods
func HandleAuthMethods(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	payloadB, err := json.Marshal(AuthMethods{Google: GlobalGoogleAuth != nil})
	if err != nil {
		logger.Error("marshal auth methods", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
}

// googleLoginState is kept in a cookie from the redirect to Google until the
// callback, binding the callback to the browser that started the login
type googleLoginState struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Redirect string `json:"redirect"`
}

func googleStateCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     googleStateCookieName,
		Path:     authRoutePrefix + googleRoutePrefix,
		HttpOnly: true,
		// Lax, not Strict: the callback is a top-level navigation from Google
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	}
}

func encodeGoogleLoginState(login googleLoginState) (string, error) {
	data, err := json.Marshal(login)
	if err != nil {
		return "", fmt.Errorf("marshal login state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func readGoogleLoginState(r *http.Request) (*googleLoginState, error) {
	cookie, err := r.Cookie(googleStateCookieName)
	if err != nil {
		return nil, fmt.Errorf("read cookie: %w", err)
	}
	data, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("decode cookie: %w", err)
	}
	var login googleLoginState
	if err := json.Unmarshal(data, &login); err != nil {
		return nil, fmt.Errorf("parse cookie: %w", err)
	}
	if login.State == "" || login.Nonce == "" || login.Verifier == "" {
		return nil, fmt.Errorf("incomplete login state")
	}
	login.Redirect = safeRedirectPath(login.Redirect)
	return &login, nil
}

// safeRedirectPath returns p if it is a path on this server and "/"
// otherwise, so the login flow can't be used as an open redirect
func safeRedirectPath(p string) string {
	// url.Parse rejects control characters. Browsers drop them from a
	// Location header, which would turn "/\t/evil.com" into "//evil.com".
	if _, err := url.Parse(p); err != nil ||
		len(p) > maxRedirectPathLength ||
		!strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, "\\") {
		return "/"
	}
	return p
}

// redirectToLoginError sends the browser to the login page with an error
// code, keeping the page the user asked for so a retry returns there
func redirectToLoginError(w http.ResponseWriter, r *http.Request, code string, redirect string) {
	query := url.Values{"error": {code}}
	if redirect != "/" {
		query.Set("redirect", redirect)
	}
	http.Redirect(w, r, "/login?"+query.Encode(), http.StatusFound)
}

// HandleGoogleLogIn starts Google sign-in by redirecting the browser to Google
func HandleGoogleLogIn(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if GlobalGoogleAuth == nil {
		http.NotFound(w, r)
		return
	}

	login := googleLoginState{
		State:    rand.Text(),
		Nonce:    rand.Text(),
		Verifier: oauth2.GenerateVerifier(),
		Redirect: safeRedirectPath(r.URL.Query().Get("redirect")),
	}
	value, err := encodeGoogleLoginState(login)
	if err != nil {
		logger.Error("encode google login state", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	cookie := googleStateCookie(r)
	cookie.Value = value
	cookie.MaxAge = int(googleStateTTL / time.Second)
	http.SetCookie(w, cookie)
	http.Redirect(w, r, GlobalGoogleAuth.AuthCodeURL(login.State, login.Nonce, login.Verifier), http.StatusFound)
}

// HandleGoogleCallback finishes Google sign-in and logs the user in
func HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if GlobalGoogleAuth == nil {
		http.NotFound(w, r)
		return
	}

	// The login state is single use, whatever the outcome
	clearCookie := googleStateCookie(r)
	clearCookie.MaxAge = -1
	http.SetCookie(w, clearCookie)

	login, err := readGoogleLoginState(r)
	if err != nil {
		logger.Warn("google login: no valid login state", "err", err)
		redirectToLoginError(w, r, loginErrorGoogle, "/")
		return
	}
	query := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(login.State)) != 1 {
		logger.Warn("google login: state mismatch")
		redirectToLoginError(w, r, loginErrorGoogle, login.Redirect)
		return
	}
	code := query.Get("code")
	if code == "" {
		logger.Warn("google login: no code", "error", query.Get("error"))
		redirectToLoginError(w, r, loginErrorGoogle, login.Redirect)
		return
	}

	email, err := GlobalGoogleAuth.Exchange(r.Context(), code, login.Verifier, login.Nonce)
	if errors.Is(err, errGoogleDomain) {
		logger.Warn("google login: rejected", "err", err)
		redirectToLoginError(w, r, loginErrorGoogleDomain, login.Redirect)
		return
	}
	if err != nil {
		logger.Error("google login", "err", err)
		redirectToLoginError(w, r, loginErrorGoogle, login.Redirect)
		return
	}

	c, err := GlobalSessionStorage.New(r, GoogleSessionTTL)
	if err != nil {
		logger.Error("create session", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Warn so the default log level keeps a record of who logged in
	logger.Warn("google login", "email", email)
	http.SetCookie(w, c)
	http.Redirect(w, r, login.Redirect, http.StatusFound)
}
