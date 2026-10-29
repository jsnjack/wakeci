package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	bolt "go.etcd.io/bbolt"
)

// Routes of the auth router, which main.go mounts at authRoutePrefix
const (
	authRoutePrefix = "/auth"
	ssoRoutePrefix  = "/sso/"
	ssoLoginRoute   = ssoRoutePrefix + "login"
	ssoACSRoute     = ssoRoutePrefix + "acs"
)

// SSOACSPath is where the identity provider posts the SAML response
const SSOACSPath = authRoutePrefix + ssoACSRoute

const ssoLoginPath = authRoutePrefix + ssoLoginRoute

const ssoStateCookieName = "sso_login"

const ssoStateTTL = 10 * time.Minute

// maxSAMLResponseSize bounds the form posted to the ACS endpoint. Google's
// responses are a few KB even with the 75 groups it sends at most.
const maxSAMLResponseSize = 1 << 20

// maxRedirectPathLength bounds the page to return to after login
const maxRedirectPathLength = 1024

// maxLoginStateCookieLength leaves room for the cookie name and attributes
// within the 4096 bytes browsers store per cookie
const maxLoginStateCookieLength = 3500

// Error codes passed to the login page as ?error=
const (
	loginErrorSSO           = "sso"
	loginErrorSSONotAllowed = "sso_not_allowed"
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
	if globalAuthAttemptLimiter.reject(w, r, logger) {
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
		if globalAuthAttemptLimiter.recordFailure(w, r, logger) {
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
	SSO bool `json:"sso"`
}

// HandleAuthMethods returns the available login methods
func HandleAuthMethods(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	payloadB, err := json.Marshal(AuthMethods{SSO: GlobalSSOAuth != nil})
	if err != nil {
		logger.Error("marshal auth methods", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
}

func ssoStateCookie() *http.Cookie {
	return &http.Cookie{
		Name:     ssoStateCookieName,
		Path:     authRoutePrefix + ssoRoutePrefix,
		HttpOnly: true,
		// None, not Lax or Strict: the identity provider posts the response
		// cross-site, and browsers leave Lax cookies off cross-site POSTs.
		// None requires Secure, which browsers also accept on http://localhost.
		SameSite: http.SameSiteNoneMode,
		Secure:   true,
	}
}

func readSSOLoginState(r *http.Request) (*ssoLoginState, error) {
	cookie, err := r.Cookie(ssoStateCookieName)
	if err != nil {
		return nil, fmt.Errorf("read cookie: %w", err)
	}
	return GlobalSSOAuth.openLoginState(cookie.Value, time.Now())
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
	http.Redirect(w, r, "/login?"+query.Encode(), http.StatusSeeOther)
}

// HandleSSOLogIn starts single sign-on by redirecting the browser to the
// identity provider
func HandleSSOLogIn(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if GlobalSSOAuth == nil {
		http.NotFound(w, r)
		return
	}

	authURL, requestID, err := GlobalSSOAuth.AuthURL()
	if err != nil {
		logger.Error("sso login request", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	login := ssoLoginState{
		RequestID: requestID,
		Redirect:  safeRedirectPath(r.URL.Query().Get("redirect")),
		Expires:   time.Now().Add(ssoStateTTL).Unix(),
	}
	value, err := GlobalSSOAuth.sealLoginState(login)
	if err == nil && len(value) > maxLoginStateCookieLength {
		// JSON escaping can make a valid redirect several times longer (each
		// "&" becomes "\u0026"). Drop it rather than set a cookie the browser
		// refuses, which would fail the login.
		login.Redirect = "/"
		value, err = GlobalSSOAuth.sealLoginState(login)
	}
	if err != nil {
		logger.Error("seal sso login state", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	cookie := ssoStateCookie()
	cookie.Value = value
	cookie.MaxAge = int(ssoStateTTL / time.Second)
	http.SetCookie(w, cookie)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleSSOACS receives the identity provider's SAML response (the assertion
// consumer service) and logs the user in
func HandleSSOACS(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if GlobalSSOAuth == nil {
		http.NotFound(w, r)
		return
	}

	// The login state is single use, whatever the outcome
	clearCookie := ssoStateCookie()
	clearCookie.MaxAge = -1
	http.SetCookie(w, clearCookie)

	r.Body = http.MaxBytesReader(w, r.Body, maxSAMLResponseSize)
	if err := r.ParseForm(); err != nil {
		logger.Warn("sso login: read form", "err", err)
		redirectToLoginError(w, r, loginErrorSSO, "/")
		return
	}

	// Without a valid login state the response is still checked: an
	// unsolicited one restarts the login below
	requestID, redirect := "", "/"
	login, err := readSSOLoginState(r)
	if err != nil {
		logger.Warn("sso login: no valid login state", "err", err)
	} else {
		requestID, redirect = login.RequestID, login.Redirect
	}

	email, err := GlobalSSOAuth.VerifyResponse(r.PostForm.Get("SAMLResponse"), requestID)
	switch {
	case errors.Is(err, errSSOUnsolicited):
		// Opened from the identity provider (e.g. the Google apps menu), so
		// there is no request to bind the response to. Start one instead.
		logger.Info("sso login: unsolicited response, starting a new login")
		http.Redirect(w, r, ssoLoginPath, http.StatusSeeOther)
		return
	case errors.Is(err, errSSOGroup):
		logger.Warn("sso login: rejected", "err", err)
		redirectToLoginError(w, r, loginErrorSSONotAllowed, redirect)
		return
	case err != nil:
		logger.Warn("sso login failed", "err", err)
		redirectToLoginError(w, r, loginErrorSSO, redirect)
		return
	}

	c, err := GlobalSessionStorage.New(r, SSOSessionTTL)
	if err != nil {
		logger.Error("create session", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Warn so the default log level keeps a record of who logged in
	logger.Warn("sso login", "email", email)
	http.SetCookie(w, c)
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
