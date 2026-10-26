package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const (
	testGoogleClientID     = "client-id.apps.googleusercontent.com"
	testGoogleClientSecret = "client-secret"
	testGoogleDomain       = "example.com"
	testGoogleRedirectURL  = "https://wakeci.example.com/auth/google/callback"
	testGoogleCode         = "auth-code"
	testLoginRedirect      = "/build/7"
)

// Login page URLs the callback redirects to on failure
var (
	wantGoogleError       = "/login?" + url.Values{"error": {loginErrorGoogle}, "redirect": {testLoginRedirect}}.Encode()
	wantGoogleDomainError = "/login?" + url.Values{"error": {loginErrorGoogleDomain}, "redirect": {testLoginRedirect}}.Encode()
	wantGoogleErrorNoPage = "/login?error=" + loginErrorGoogle
)

func makeTestIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc.EncodeToString(payload) + ".sig"
}

// fakeGoogle stands in for Google's token endpoint. It only issues tokens
// for the expected code, client credentials and PKCE verifier.
type fakeGoogle struct {
	t             *testing.T
	challenge     string
	claims        map[string]any // nil leaves id_token out of the response
	rawIDToken    string         // replaces the token built from claims
	status        int            // non-zero makes every token request fail
	tokenRequests int
}

func (f *fakeGoogle) writeJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		f.t.Errorf("marshal token response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		f.t.Errorf("write token response: %v", err)
	}
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.tokenRequests++
	if f.status != 0 {
		f.writeJSON(w, f.status, map[string]string{"error": "internal_failure"})
		return
	}
	if err := r.ParseForm(); err != nil {
		f.t.Errorf("parse token request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	verifierOK := base64.RawURLEncoding.EncodeToString(sum[:]) == f.challenge
	if r.PostForm.Get("grant_type") != "authorization_code" ||
		r.PostForm.Get("code") != testGoogleCode ||
		r.PostForm.Get("client_id") != testGoogleClientID ||
		r.PostForm.Get("client_secret") != testGoogleClientSecret ||
		r.PostForm.Get("redirect_uri") != testGoogleRedirectURL ||
		!verifierOK {
		f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	resp := map[string]any{
		"access_token": "access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	}
	switch {
	case f.rawIDToken != "":
		resp["id_token"] = f.rawIDToken
	case f.claims != nil:
		resp["id_token"] = makeTestIDToken(f.t, f.claims)
	}
	f.writeJSON(w, http.StatusOK, resp)
}

func newGoogleTestEnv(t *testing.T) *fakeGoogle {
	t.Helper()
	fake := &fakeGoogle{t: t}
	server := httptest.NewServer(fake)

	originalConfig := Config
	originalSessions := GlobalSessionStorage
	originalGoogle := GlobalGoogleAuth
	Config = &WakeConfig{Port: "8081"}
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]time.Time)}
	GlobalGoogleAuth = NewGoogleAuth(GoogleConfig{
		ClientID:     testGoogleClientID,
		ClientSecret: testGoogleClientSecret,
		Domain:       testGoogleDomain,
		RedirectURL:  testGoogleRedirectURL,
	})
	GlobalGoogleAuth.oauth.Endpoint = oauth2.Endpoint{
		AuthURL:   googleEndpoint.AuthURL,
		TokenURL:  server.URL + "/token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	t.Cleanup(func() {
		server.Close()
		Config = originalConfig
		GlobalSessionStorage = originalSessions
		GlobalGoogleAuth = originalGoogle
	})
	return fake
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// decodeLoginCookie reads the login state as stored, without the checks
// readGoogleLoginState applies
func decodeLoginCookie(t *testing.T, c *http.Cookie) googleLoginState {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		t.Fatalf("decode login cookie: %v", err)
	}
	var login googleLoginState
	if err := json.Unmarshal(data, &login); err != nil {
		t.Fatalf("parse login cookie: %v", err)
	}
	return login
}

// forgeLoginCookie returns a copy of c with its login state changed by edit
func forgeLoginCookie(t *testing.T, c *http.Cookie, edit func(*googleLoginState)) *http.Cookie {
	t.Helper()
	login := decodeLoginCookie(t, c)
	edit(&login)
	value, err := encodeGoogleLoginState(login)
	if err != nil {
		t.Fatalf("encode login cookie: %v", err)
	}
	forged := *c
	forged.Value = value
	return &forged
}

func startGoogleLogin(t *testing.T, redirect string, forwardedProto string) (*url.URL, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, authRoutePrefix+googleLoginRoute+"?redirect="+url.QueryEscape(redirect), nil)
	req.Header.Set("X-Forwarded-Proto", forwardedProto)
	rec := httptest.NewRecorder()
	HandleGoogleLogIn(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want %d", rec.Code, http.StatusFound)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	cookie := findCookie(rec.Result().Cookies(), googleStateCookieName)
	if cookie == nil {
		t.Fatal("no login state cookie")
	}
	return location, cookie
}

func TestGoogleLogInRedirect(t *testing.T) {
	newGoogleTestEnv(t)
	location, cookie := startGoogleLogin(t, testLoginRedirect, "")
	login := decodeLoginCookie(t, cookie)
	challenge := sha256.Sum256([]byte(login.Verifier))

	q := location.Query()
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"auth endpoint", location.Scheme + "://" + location.Host + location.Path, googleEndpoint.AuthURL},
		{"client_id", q.Get("client_id"), testGoogleClientID},
		{"redirect_uri", q.Get("redirect_uri"), testGoogleRedirectURL},
		{"response_type", q.Get("response_type"), "code"},
		{"scope", q.Get("scope"), "openid email"},
		{"hd", q.Get("hd"), testGoogleDomain},
		{"state", q.Get("state"), login.State},
		{"nonce", q.Get("nonce"), login.Nonce},
		{"code_challenge", q.Get("code_challenge"), base64.RawURLEncoding.EncodeToString(challenge[:])},
		{"code_challenge_method", q.Get("code_challenge_method"), "S256"},
		{"cookie path", cookie.Path, "/auth/google/"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("got %q, want %q", c.got, c.want)
			}
		})
	}
	t.Run("cookie attributes", func(t *testing.T) {
		if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 {
			t.Errorf("cookie HttpOnly=%t SameSite=%v MaxAge=%d, want HttpOnly, Lax, positive MaxAge",
				cookie.HttpOnly, cookie.SameSite, cookie.MaxAge)
		}
	})
}

func TestGoogleLoginStateCookie(t *testing.T) {
	tests := []struct {
		name           string
		redirect       string
		forwardedProto string
		wantRedirect   string
		wantSecure     bool
	}{
		{name: "page on this server", redirect: testLoginRedirect, wantRedirect: testLoginRedirect},
		{name: "other site", redirect: "//evil.example.com", wantRedirect: "/"},
		{name: "too long for a cookie", redirect: "/jobs?q=" + strings.Repeat("a", 3000), wantRedirect: "/"},
		{name: "behind TLS proxy", redirect: testLoginRedirect, forwardedProto: "https", wantRedirect: testLoginRedirect, wantSecure: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newGoogleTestEnv(t)
			_, cookie := startGoogleLogin(t, tt.redirect, tt.forwardedProto)
			if got := decodeLoginCookie(t, cookie).Redirect; got != tt.wantRedirect {
				t.Errorf("stored redirect = %q, want %q", got, tt.wantRedirect)
			}
			if cookie.Secure != tt.wantSecure {
				t.Errorf("cookie Secure = %t, want %t", cookie.Secure, tt.wantSecure)
			}
		})
	}
}

func TestGoogleCallback(t *testing.T) {
	tests := []struct {
		name         string
		fake         func(f *fakeGoogle)
		request      func(t *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie)
		wantLocation string
		wantSession  bool
		noExchange   bool // the callback must give up before calling Google
	}{
		{name: "workspace account", wantLocation: testLoginRedirect, wantSession: true},
		{
			name:         "domain in other case",
			fake:         func(f *fakeGoogle) { f.claims["hd"] = "EXAMPLE.com" },
			wantLocation: testLoginRedirect,
			wantSession:  true,
		},
		{
			name: "forged redirect to other site",
			request: func(t *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				return q, forgeLoginCookie(t, c, func(l *googleLoginState) { l.Redirect = "https://evil.example.com" })
			},
			wantLocation: "/",
			wantSession:  true,
		},
		{
			name:         "other workspace domain",
			fake:         func(f *fakeGoogle) { f.claims["hd"] = "other.com"; f.claims["email"] = "a@other.com" },
			wantLocation: wantGoogleDomainError,
		},
		{
			name:         "consumer account without hd",
			fake:         func(f *fakeGoogle) { delete(f.claims, "hd"); f.claims["email"] = "a@gmail.com" },
			wantLocation: wantGoogleDomainError,
		},
		{
			name:         "unverified email",
			fake:         func(f *fakeGoogle) { f.claims["email_verified"] = false },
			wantLocation: wantGoogleError,
		},
		{
			name:         "no email",
			fake:         func(f *fakeGoogle) { delete(f.claims, "email") },
			wantLocation: wantGoogleError,
		},
		{
			name:         "token for another client",
			fake:         func(f *fakeGoogle) { f.claims["aud"] = "someone-else.apps.googleusercontent.com" },
			wantLocation: wantGoogleError,
		},
		{
			name:         "wrong issuer",
			fake:         func(f *fakeGoogle) { f.claims["iss"] = "https://evil.example.com" },
			wantLocation: wantGoogleError,
		},
		{
			name:         "expired token",
			fake:         func(f *fakeGoogle) { f.claims["exp"] = time.Now().Add(-time.Minute).Unix() },
			wantLocation: wantGoogleError,
		},
		{
			name:         "replayed nonce",
			fake:         func(f *fakeGoogle) { f.claims["nonce"] = "nonce-from-another-login" },
			wantLocation: wantGoogleError,
		},
		{
			name:         "no id_token",
			fake:         func(f *fakeGoogle) { f.claims = nil },
			wantLocation: wantGoogleError,
		},
		{
			name:         "id_token not a JWT",
			fake:         func(f *fakeGoogle) { f.rawIDToken = "not-a-jwt" },
			wantLocation: wantGoogleError,
		},
		{
			name:         "id_token payload not base64",
			fake:         func(f *fakeGoogle) { f.rawIDToken = "header.!!!.sig" },
			wantLocation: wantGoogleError,
		},
		{
			name:         "token endpoint down",
			fake:         func(f *fakeGoogle) { f.status = http.StatusInternalServerError },
			wantLocation: wantGoogleError,
		},
		{
			name: "wrong code",
			request: func(_ *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				q.Set("code", "stolen-code")
				return q, c
			},
			wantLocation: wantGoogleError,
		},
		{
			name: "state mismatch",
			request: func(_ *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				q.Set("state", "forged")
				return q, c
			},
			wantLocation: wantGoogleError,
			noExchange:   true,
		},
		{
			name: "no login state cookie",
			request: func(_ *testing.T, q url.Values, _ *http.Cookie) (url.Values, *http.Cookie) {
				return q, nil
			},
			wantLocation: wantGoogleErrorNoPage,
			noExchange:   true,
		},
		{
			name: "login state cookie without nonce",
			request: func(t *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				return q, forgeLoginCookie(t, c, func(l *googleLoginState) { l.Nonce = "" })
			},
			wantLocation: wantGoogleErrorNoPage,
			noExchange:   true,
		},
		{
			name: "login state cookie not base64",
			request: func(_ *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				broken := *c
				broken.Value = "!!!"
				return q, &broken
			},
			wantLocation: wantGoogleErrorNoPage,
			noExchange:   true,
		},
		{
			name: "user cancelled at google",
			request: func(_ *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				q.Del("code")
				q.Set("error", "access_denied")
				return q, c
			},
			wantLocation: wantGoogleError,
			noExchange:   true,
		},
		{
			name: "no code",
			request: func(_ *testing.T, q url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				q.Del("code")
				return q, c
			},
			wantLocation: wantGoogleError,
			noExchange:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newGoogleTestEnv(t)
			location, stateCookie := startGoogleLogin(t, testLoginRedirect, "")

			fake.challenge = location.Query().Get("code_challenge")
			fake.claims = map[string]any{
				"iss":            "https://accounts.google.com",
				"aud":            testGoogleClientID,
				"exp":            time.Now().Add(time.Hour).Unix(),
				"nonce":          location.Query().Get("nonce"),
				"email":          "alice@example.com",
				"email_verified": true,
				"hd":             testGoogleDomain,
			}
			if tt.fake != nil {
				tt.fake(fake)
			}

			q := url.Values{"code": {testGoogleCode}, "state": {location.Query().Get("state")}}
			cookie := stateCookie
			if tt.request != nil {
				q, cookie = tt.request(t, q, cookie)
			}
			req := httptest.NewRequest(http.MethodGet, GoogleCallbackPath+"?"+q.Encode(), nil)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()
			HandleGoogleCallback(rec, req)

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}
			if tt.noExchange && fake.tokenRequests != 0 {
				t.Errorf("token endpoint called %d times, want 0", fake.tokenRequests)
			}
			cleared := findCookie(rec.Result().Cookies(), googleStateCookieName)
			if cleared == nil || cleared.MaxAge >= 0 {
				t.Errorf("login state cookie not cleared: %+v", cleared)
			}

			session := findCookie(rec.Result().Cookies(), "session")
			if !tt.wantSession {
				if session != nil {
					t.Errorf("unexpected session cookie")
				}
				return
			}
			if session == nil {
				t.Fatal("no session cookie")
			}
			if err := GlobalSessionStorage.Verify(session.Value); err != nil {
				t.Errorf("session not stored: %v", err)
			}
			if ttl := time.Until(session.Expires); ttl > GoogleSessionTTL || ttl < GoogleSessionTTL-time.Minute {
				t.Errorf("session expires in %v, want about %v", ttl, GoogleSessionTTL)
			}
		})
	}
}

func TestVerifyIDTokenRequiresNonce(t *testing.T) {
	g := NewGoogleAuth(GoogleConfig{ClientID: testGoogleClientID, Domain: testGoogleDomain})
	token := makeTestIDToken(t, map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            testGoogleClientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"email":          "alice@example.com",
		"email_verified": true,
		"hd":             testGoogleDomain,
	})
	if _, err := g.verifyIDToken(token, "", time.Now()); err == nil {
		t.Error("token without nonce accepted when no nonce was expected")
	}
}

func TestGoogleHandlersDisabled(t *testing.T) {
	originalGoogle := GlobalGoogleAuth
	GlobalGoogleAuth = nil
	t.Cleanup(func() { GlobalGoogleAuth = originalGoogle })

	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"login", HandleGoogleLogIn, authRoutePrefix + googleLoginRoute},
		{"callback", HandleGoogleCallback, GoogleCallbackPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandleAuthMethods(t *testing.T) {
	originalGoogle := GlobalGoogleAuth
	t.Cleanup(func() { GlobalGoogleAuth = originalGoogle })

	tests := []struct {
		name   string
		google *GoogleAuth
		want   string
	}{
		{"password only", nil, `{"google":false}`},
		{"google enabled", NewGoogleAuth(GoogleConfig{ClientID: testGoogleClientID}), `{"google":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GlobalGoogleAuth = tt.google
			rec := httptest.NewRecorder()
			HandleAuthMethods(rec, httptest.NewRequest(http.MethodGet, "/auth/methods", nil))
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"page", "/build/7", "/build/7"},
		{"page with query", "/jobs?q=a", "/jobs?q=a"},
		{"empty", "", "/"},
		{"relative path", "build/7", "/"},
		{"absolute URL", "https://evil.example.com/", "/"},
		{"scheme-relative URL", "//evil.example.com", "/"},
		{"triple slash", "///evil.example.com", "/"},
		{"backslash", "/\\evil.example.com", "/"},
		{"tab", "/\t/evil.example.com", "/"},
		{"newline", "/\n/evil.example.com", "/"},
		{"longest allowed", "/" + strings.Repeat("a", maxRedirectPathLength-1), "/" + strings.Repeat("a", maxRedirectPathLength-1)},
		{"too long", "/" + strings.Repeat("a", maxRedirectPathLength), "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRedirectPath(tt.in); got != tt.want {
				t.Errorf("safeRedirectPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
