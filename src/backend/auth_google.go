package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// googleEndpoint is a variable so tests can point the flow at a fake Google
var googleEndpoint = endpoints.Google

const (
	googleStateCookie     = "google_oauth_state"
	googleLoginTTL        = 10 * time.Minute
	googleExchangeTimeout = 10 * time.Second
	// maxPendingGoogleLogins caps the in-memory state store, which anyone
	// can add to by opening /auth/google/login
	maxPendingGoogleLogins          = 10000
	maxPendingGoogleLoginsPerClient = 20
	maxGoogleLoginStarts            = 30
	googleLoginStartWindow          = time.Minute
)

// Values of the `error` query parameter the login page is redirected with
const (
	googleLoginFailed     = "google_failed"
	googleLoginNotAllowed = "google_not_allowed"
)

var errGoogleEmailUnverified = errors.New("google email is missing or unverified")

// pendingGoogleLogin is what the callback needs to finish a login started by
// HandleGoogleLogin
type pendingGoogleLogin struct {
	client   string
	verifier string
	nonce    string
	redirect string
	// consent forces Google's consent screen, the only way to be sure to get
	// a new refresh token
	consent bool
	expires time.Time
}

type googleLoginStore struct {
	mu     sync.Mutex
	logins map[string]pendingGoogleLogin
	starts map[string]googleLoginStarts
}

type googleLoginStarts struct {
	count   int
	expires time.Time
}

type googleLoginLimitError struct {
	retryAfter time.Duration
}

func (e *googleLoginLimitError) Error() string {
	return "too many google login attempts from this client"
}

var googleLogins = &googleLoginStore{logins: make(map[string]pendingGoogleLogin)}

func (s *googleLoginStore) put(state string, login pendingGoogleLogin, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := 0
	firstExpiry := login.expires
	for key, l := range s.logins {
		if !now.Before(l.expires) {
			delete(s.logins, key)
		} else if l.client == login.client {
			pending++
			if l.expires.Before(firstExpiry) {
				firstExpiry = l.expires
			}
		}
	}
	for client, starts := range s.starts {
		if !now.Before(starts.expires) {
			delete(s.starts, client)
		}
	}
	starts := s.starts[login.client]
	if starts.count >= maxGoogleLoginStarts {
		return &googleLoginLimitError{retryAfter: starts.expires.Sub(now)}
	}
	if pending >= maxPendingGoogleLoginsPerClient {
		return &googleLoginLimitError{retryAfter: firstExpiry.Sub(now)}
	}
	if len(s.logins) >= maxPendingGoogleLogins {
		var oldest string
		var expiry time.Time
		for key, pending := range s.logins {
			if expiry.IsZero() || pending.expires.Before(expiry) {
				oldest, expiry = key, pending.expires
			}
		}
		delete(s.logins, oldest)
	}
	if starts.count == 0 {
		if len(s.starts) >= maxPendingGoogleLogins {
			var oldest string
			var expiry time.Time
			for client, attempts := range s.starts {
				if expiry.IsZero() || attempts.expires.Before(expiry) {
					oldest, expiry = client, attempts.expires
				}
			}
			delete(s.starts, oldest)
		}
		starts.expires = now.Add(googleLoginStartWindow)
	}
	starts.count++
	if s.starts == nil {
		s.starts = make(map[string]googleLoginStarts)
	}
	s.starts[login.client] = starts
	s.logins[state] = login
	return nil
}

// take removes and returns the login for state; each state is usable once
func (s *googleLoginStore) take(state string, now time.Time) (pendingGoogleLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	login, ok := s.logins[state]
	delete(s.logins, state)
	if !ok || !now.Before(login.expires) {
		return pendingGoogleLogin{}, false
	}
	return login, true
}

// googleRedirectURL must be identical when starting the login and when
// exchanging the code, and must be registered in the Google Cloud console
func googleRedirectURL(r *http.Request) string {
	if Config.Google.RedirectURL != "" {
		return Config.Google.RedirectURL
	}
	host := Config.Hostname
	if host == "" {
		host = r.Host
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + host + "/auth/google/callback"
}

// googleOAuthConfigFor builds the OAuth client; redirectURL may be empty
// when only refreshing tokens
func googleOAuthConfigFor(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     Config.Google.ClientID,
		ClientSecret: Config.Google.ClientSecret,
		Endpoint:     googleEndpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email"},
	}
}

// safeRedirectPath only allows same-origin paths as post-login targets.
// Control characters are rejected because browsers strip tabs and newlines
// from URLs, turning "/\t/evil.com" into "//evil.com".
func safeRedirectPath(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, "\\") || strings.ContainsFunc(p, unicode.IsControl) {
		return "/"
	}
	return p
}

func googleStateCookieFor(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     googleStateCookie,
		Path:     "/auth/google/",
		HttpOnly: true,
		// Lax, not Strict: the callback is a top-level navigation coming
		// from accounts.google.com, which wouldn't carry a Strict cookie
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	}
}

func redirectToLogin(w http.ResponseWriter, r *http.Request, loginError string) {
	http.Redirect(w, r, "/login?error="+url.QueryEscape(loginError), http.StatusFound)
}

// HandleGoogleLogin starts "Sign in with Google": it remembers a fresh
// state, nonce and PKCE verifier, then redirects to Google
func HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if !Config.Google.Enabled() {
		http.NotFound(w, r)
		return
	}

	state, err := randomHex(32)
	if err != nil {
		logger.Error("generate oauth state", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	nonce, err := randomHex(32)
	if err != nil {
		logger.Error("generate oauth nonce", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	now := time.Now()
	login := pendingGoogleLogin{
		client:   authClient(r),
		verifier: oauth2.GenerateVerifier(),
		nonce:    nonce,
		redirect: safeRedirectPath(r.URL.Query().Get("redirect")),
		consent:  r.URL.Query().Get("consent") == "1",
		expires:  now.Add(googleLoginTTL),
	}
	if err := googleLogins.put(state, login, now); err != nil {
		logger.Warn("store google login", "err", err)
		var limited *googleLoginLimitError
		if errors.As(err, &limited) {
			seconds := max(1, int((limited.retryAfter+time.Second-1)/time.Second))
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeText(w, logger, http.StatusTooManyRequests, "Too many Google login attempts, try again later")
			return
		}
		writeText(w, logger, http.StatusServiceUnavailable, "Too many pending logins, try again later")
		return
	}

	// Bind the state to this browser, so a callback URL from someone else's
	// login can't be replayed into it
	cookie := googleStateCookieFor(r)
	cookie.Value = state
	cookie.MaxAge = int(googleLoginTTL / time.Second)
	http.SetCookie(w, cookie)

	prompt := "select_account"
	if login.consent {
		prompt = "consent select_account"
	}
	opts := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(login.verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("prompt", prompt),
		// A refresh token lets RunGoogleAccountChecker notice when the
		// account is suspended or deleted
		oauth2.AccessTypeOffline,
	}
	if len(Config.Google.AllowedDomains) == 1 {
		// Only a hint to Google's account picker; the callback enforces it
		opts = append(opts, oauth2.SetAuthURLParam("hd", Config.Google.AllowedDomains[0]))
	}
	http.Redirect(w, r, googleOAuthConfigFor(googleRedirectURL(r)).AuthCodeURL(state, opts...), http.StatusFound)
}

// HandleGoogleCallback finishes "Sign in with Google": it exchanges the code
// for an ID token and a refresh token, checks the account's domain, stores
// the account for RunGoogleAccountChecker and starts a session
func HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if !Config.Google.Enabled() {
		http.NotFound(w, r)
		return
	}

	clearState := googleStateCookieFor(r)
	clearState.MaxAge = -1
	http.SetCookie(w, clearState)

	query := r.URL.Query()

	state := query.Get("state")
	cookie, err := r.Cookie(googleStateCookie)
	if err != nil || state == "" || cookie.Value != state {
		logger.Warn("google callback state doesn't match this browser", "cookieErr", err)
		redirectToLogin(w, r, googleLoginFailed)
		return
	}
	now := time.Now()
	login, ok := googleLogins.take(state, now)
	if !ok {
		logger.Warn("google callback state unknown or expired")
		redirectToLogin(w, r, googleLoginFailed)
		return
	}

	if e := query.Get("error"); e != "" {
		logger.Warn("google sign-in returned an error", "error", e, "description", query.Get("error_description"))
		redirectToLogin(w, r, googleLoginFailed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), googleExchangeTimeout)
	defer cancel()
	token, err := googleOAuthConfigFor(googleRedirectURL(r)).Exchange(ctx, query.Get("code"), oauth2.VerifierOption(login.verifier))
	if err != nil {
		logger.Warn("exchange google auth code", "err", err)
		redirectToLogin(w, r, googleLoginFailed)
		return
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	claims, err := parseGoogleIDToken(rawIDToken, Config.Google.ClientID, now)
	if err == nil && claims.Nonce != login.nonce {
		err = errors.New("nonce mismatch")
	}
	if err != nil {
		logger.Warn("invalid google id token", "err", err)
		redirectToLogin(w, r, googleLoginFailed)
		return
	}
	if !Config.Google.Allows(claims.HostedDomain) {
		logger.Warn("google account not allowed", "email", claims.Email, "hd", claims.HostedDomain)
		redirectToLogin(w, r, googleLoginNotAllowed)
		return
	}

	principal := &Principal{
		ID:       ProviderGoogle + ":" + claims.Subject,
		User:     strings.ToLower(claims.Email),
		Provider: ProviderGoogle,
		Domain:   strings.ToLower(claims.HostedDomain),
		Scopes:   slices.Clone(AllScopes),
	}

	// Google only hands out a refresh token on consent. Without one, and
	// without a stored one, the account couldn't be offboarded automatically,
	// so go round once more with the consent screen.
	refreshToken := token.RefreshToken
	if refreshToken == "" {
		existing, err := getGoogleAccount(principal.ID)
		if err != nil {
			logger.Error("read google account", "err", err)
			redirectToLogin(w, r, googleLoginFailed)
			return
		}
		switch {
		case existing != nil:
			refreshToken = existing.RefreshToken
		case !login.consent:
			http.Redirect(w, r, "/auth/google/login?consent=1&redirect="+url.QueryEscape(login.redirect), http.StatusFound)
			return
		default:
			logger.Warn("google returned no refresh token even after consent", "email", principal.User)
			redirectToLogin(w, r, googleLoginFailed)
			return
		}
	}
	account := &googleAccount{
		ID:           principal.ID,
		Email:        principal.User,
		Domain:       principal.Domain,
		RefreshToken: refreshToken,
		VerifiedAt:   now,
	}
	err = putGoogleAccount(account)
	if err != nil {
		logger.Error("store google account", "err", err)
		redirectToLogin(w, r, googleLoginFailed)
		return
	}
	c, err := newGoogleSession(r, principal, account)
	if err != nil {
		logger.Error("create session", "err", err)
		redirectToLogin(w, r, googleLoginFailed)
		return
	}
	logger.Info("google sign-in", "email", principal.User, "id", principal.ID)
	http.SetCookie(w, c)
	http.Redirect(w, r, login.redirect, http.StatusFound)
}

// googleIDClaims are the ID token claims wakeci relies on
type googleIDClaims struct {
	Issuer        string `json:"iss"`
	Audience      string `json:"aud"`
	Expiry        int64  `json:"exp"`
	Subject       string `json:"sub"`
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	HostedDomain  string `json:"hd"`
}

// parseGoogleIDToken decodes and validates an ID token received from
// Google's token endpoint. Its signature isn't checked: the token came
// straight from Google over TLS in exchange for a single-use code or a
// refresh token, which OpenID Connect Core §3.1.3.7 accepts in place of
// signature validation. The token must never be accepted from any other
// source (e.g. the browser). Callers finishing a login must check the nonce.
func parseGoogleIDToken(raw string, clientID string, now time.Time) (*googleIDClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("id token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode id token payload: %w", err)
	}
	var claims googleIDClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal id token claims: %w", err)
	}
	switch {
	case claims.Issuer != "https://accounts.google.com" && claims.Issuer != "accounts.google.com":
		return nil, fmt.Errorf("unexpected issuer %q", claims.Issuer)
	case claims.Audience != clientID:
		return nil, fmt.Errorf("token is for client %q, not this one", claims.Audience)
	case !now.Before(time.Unix(claims.Expiry, 0)):
		return nil, errors.New("token expired")
	case claims.Subject == "":
		return nil, errors.New("token has no subject")
	case claims.Email == "" || !claims.EmailVerified:
		return nil, fmt.Errorf("%w: %q", errGoogleEmailUnverified, claims.Email)
	}
	return &claims, nil
}
