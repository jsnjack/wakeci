package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func fakeIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + enc.EncodeToString(payload) + "." + enc.EncodeToString([]byte("sig"))
}

func validGoogleClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            "client-id",
		"exp":            now.Add(time.Hour).Unix(),
		"nonce":          "nonce",
		"sub":            "110169484474386276334",
		"email":          "Dev@Example.com",
		"email_verified": true,
		"hd":             "example.com",
	}
}

func TestParseGoogleIDToken(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		modify  func(claims map[string]any)
		raw     string
		wantErr bool
	}{
		{name: "valid", modify: func(map[string]any) {}},
		{name: "short issuer", modify: func(c map[string]any) { c["iss"] = "accounts.google.com" }},
		{name: "other issuer", modify: func(c map[string]any) { c["iss"] = "https://evil.example.com" }, wantErr: true},
		{name: "other client", modify: func(c map[string]any) { c["aud"] = "other-client" }, wantErr: true},
		{name: "expired", modify: func(c map[string]any) { c["exp"] = now.Add(-time.Second).Unix() }, wantErr: true},
		{name: "unverified email", modify: func(c map[string]any) { c["email_verified"] = false }, wantErr: true},
		{name: "no email", modify: func(c map[string]any) { delete(c, "email") }, wantErr: true},
		{name: "no subject", modify: func(c map[string]any) { delete(c, "sub") }, wantErr: true},
		{name: "not a jwt", raw: "garbage", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.raw
			if raw == "" {
				claims := validGoogleClaims(now)
				tt.modify(claims)
				raw = fakeIDToken(t, claims)
			}
			_, err := parseGoogleIDToken(raw, "client-id", now)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGoogleIDToken error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestGoogleConfigAllows(t *testing.T) {
	g := GoogleConfig{ClientID: "id", ClientSecret: "s", AllowedDomains: []string{"example.com"}}
	tests := []struct {
		name   string
		hd     string
		config GoogleConfig
		want   bool
	}{
		{name: "workspace domain", hd: "example.com", config: g, want: true},
		{name: "domain is case-insensitive", hd: "Example.COM", config: g, want: true},
		// Personal accounts, gmail.com or a custom address, never carry hd
		{name: "personal account", config: g},
		{name: "other workspace", hd: "other.com", config: g},
		{name: "google sign-in disabled", hd: "example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.Allows(tt.hd); got != tt.want {
				t.Errorf("Allows(%q) = %t, want %t", tt.hd, got, tt.want)
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
		{name: "path with query", in: "/build/5?x=1", want: "/build/5?x=1"},
		{name: "empty", in: "", want: "/"},
		{name: "absolute URL", in: "https://evil.example.com", want: "/"},
		{name: "protocol relative", in: "//evil.example.com", want: "/"},
		{name: "backslash", in: "/\\evil.example.com", want: "/"},
		{name: "tab browsers strip", in: "/\t/evil.example.com", want: "/"},
		{name: "newline browsers strip", in: "/\n/evil.example.com", want: "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRedirectPath(tt.in); got != tt.want {
				t.Errorf("safeRedirectPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGoogleRedirectURL(t *testing.T) {
	tests := []struct {
		name           string
		config         WakeConfig
		host           string
		forwardedProto string
		want           string
	}{
		{name: "request host", config: WakeConfig{Port: "8081"}, host: "ci.local:8081", want: "http://ci.local:8081/auth/google/callback"},
		{name: "TLS proxy", config: WakeConfig{Port: "8081"}, host: "ci.example.com", forwardedProto: "https", want: "https://ci.example.com/auth/google/callback"},
		{name: "configured hostname", config: WakeConfig{Port: "443", Hostname: "ci.example.com"}, host: "spoofed.example.org", want: "https://ci.example.com/auth/google/callback"},
		{name: "explicit override", config: WakeConfig{Port: "8081", Google: GoogleConfig{RedirectURL: "https://proxy.example.com/ci/auth/google/callback"}}, host: "ci.local", want: "https://proxy.example.com/ci/auth/google/callback"},
	}
	originalConfig := Config
	t.Cleanup(func() { Config = originalConfig })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Config = &tt.config
			req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
			req.Host = tt.host
			req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			if got := googleRedirectURL(req); got != tt.want {
				t.Errorf("googleRedirectURL = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeGoogle is Google's token endpoint. For a code it checks the PKCE
// verifier and returns an ID token for claims plus refreshToken; for a
// refresh token it returns refreshClaims[token], or invalid_grant for tokens
// it doesn't know, like Google does for suspended or deleted accounts.
type fakeGoogle struct {
	t             *testing.T
	server        *httptest.Server
	challenge     string
	claims        map[string]any
	refreshToken  string
	refreshClaims map[string]map[string]any
	// unavailable makes every refresh fail with a 503
	unavailable bool
	// omitIDToken leaves the ID token out of refresh responses, which
	// Google may do; the checker then has to ask the userinfo endpoint
	omitIDToken bool
	// userInfo is served by the userinfo endpoint; nil makes it fail
	userInfo map[string]any
	// onRefresh runs while a refresh request is in flight
	onRefresh func()
}

func (f *fakeGoogle) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		f.t.Errorf("fake google: write response: %v", err)
	}
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	f := &fakeGoogle{t: t, refreshToken: "refresh-1", refreshClaims: map[string]map[string]any{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/userinfo" {
			if f.userInfo == nil || r.Header.Get("Authorization") != "Bearer access" {
				f.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
				return
			}
			f.writeJSON(w, http.StatusOK, f.userInfo)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("fake google: parse form: %v", err)
		}
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			if r.PostForm.Get("code") != "auth-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
				return
			}
			resp := map[string]any{"access_token": "access", "token_type": "Bearer", "expires_in": 3600, "id_token": fakeIDToken(t, f.claims)}
			if f.refreshToken != "" {
				resp["refresh_token"] = f.refreshToken
			}
			f.writeJSON(w, http.StatusOK, resp)
		case "refresh_token":
			if f.onRefresh != nil {
				f.onRefresh()
			}
			if f.unavailable {
				f.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
				return
			}
			claims, ok := f.refreshClaims[r.PostForm.Get("refresh_token")]
			if !ok {
				f.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "Token has been expired or revoked."})
				return
			}
			resp := map[string]any{"access_token": "access", "token_type": "Bearer", "expires_in": 3600}
			if !f.omitIDToken {
				resp["id_token"] = fakeIDToken(t, claims)
			}
			f.writeJSON(w, http.StatusOK, resp)
		default:
			t.Errorf("fake google: unexpected grant_type %q", r.PostForm.Get("grant_type"))
		}
	}))
	t.Cleanup(f.server.Close)

	originalEndpoint, originalUserInfo := googleEndpoint, googleUserInfoURL
	googleEndpoint = oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: f.server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}
	googleUserInfoURL = f.server.URL + "/userinfo"
	t.Cleanup(func() { googleEndpoint, googleUserInfoURL = originalEndpoint, originalUserInfo })
	return f
}

// startLogin opens /auth/google/login and returns the state cookie and the
// parameters wakeci sent to Google
func (f *fakeGoogle) startLogin(router http.Handler, redirect string) (*http.Cookie, url.Values) {
	f.t.Helper()
	return f.startLoginAt(router, "/auth/google/login?redirect="+url.QueryEscape(redirect))
}

func (f *fakeGoogle) startLoginAt(router http.Handler, path string) (*http.Cookie, url.Values) {
	f.t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusFound {
		f.t.Fatalf("login status = %d, want 302", rec.Code)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(location.String(), "https://accounts.example/auth?") {
		f.t.Fatalf("login redirects to %q, want the Google auth URL", rec.Header().Get("Location"))
	}
	params := location.Query()
	f.challenge = params.Get("code_challenge")
	f.claims = validGoogleClaims(time.Now())
	f.claims["aud"] = Config.Google.ClientID
	f.claims["nonce"] = params.Get("nonce")
	for _, c := range rec.Result().Cookies() {
		if c.Name == googleStateCookie {
			return c, params
		}
	}
	f.t.Fatal("login set no state cookie")
	return nil, nil
}

func callback(router http.Handler, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=auth-code&state="+url.QueryEscape(state), nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func sessionPrincipal(t *testing.T, rec *httptest.ResponseRecorder) *Principal {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" {
			p, err := GlobalSessionStorage.Verify(c.Value)
			if err != nil {
				t.Fatalf("callback session doesn't verify: %v", err)
			}
			return p
		}
	}
	return nil
}

func TestGoogleLoginFlow(t *testing.T) {
	tests := []struct {
		name string
		// tamper adjusts the flow after login started; it returns the state
		// and cookie to present to the callback
		tamper       func(f *fakeGoogle, state string, cookie *http.Cookie) (string, *http.Cookie)
		redirect     string
		wantLocation string
		wantUser     string
	}{
		{
			name:         "allowed workspace account",
			redirect:     "/build/5",
			wantLocation: "/build/5",
			wantUser:     "dev@example.com",
		},
		{
			name:         "offsite redirect is dropped",
			redirect:     "//evil.example.com",
			wantLocation: "/",
			wantUser:     "dev@example.com",
		},
		{
			name: "account outside the allowlist",
			tamper: func(f *fakeGoogle, state string, cookie *http.Cookie) (string, *http.Cookie) {
				f.claims["hd"] = "other.com"
				return state, cookie
			},
			wantLocation: "/login?error=" + googleLoginNotAllowed,
		},
		{
			name: "personal google account",
			tamper: func(f *fakeGoogle, state string, cookie *http.Cookie) (string, *http.Cookie) {
				delete(f.claims, "hd")
				return state, cookie
			},
			wantLocation: "/login?error=" + googleLoginNotAllowed,
		},
		{
			name: "state from another browser",
			tamper: func(f *fakeGoogle, state string, _ *http.Cookie) (string, *http.Cookie) {
				return state, &http.Cookie{Name: googleStateCookie, Value: "attacker-state"}
			},
			wantLocation: "/login?error=" + googleLoginFailed,
		},
		{
			name: "no state cookie",
			tamper: func(f *fakeGoogle, state string, _ *http.Cookie) (string, *http.Cookie) {
				return state, nil
			},
			wantLocation: "/login?error=" + googleLoginFailed,
		},
		{
			name: "replayed id token nonce",
			tamper: func(f *fakeGoogle, state string, cookie *http.Cookie) (string, *http.Cookie) {
				f.claims["nonce"] = "old-nonce"
				return state, cookie
			},
			wantLocation: "/login?error=" + googleLoginFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
			f := newFakeGoogle(t)
			router := newRouter()

			cookie, params := f.startLogin(router, tt.redirect)
			if params.Get("hd") != "example.com" || params.Get("code_challenge_method") != "S256" || params.Get("access_type") != "offline" {
				t.Errorf("auth URL params = %v, want hd hint, S256 PKCE and offline access", params)
			}
			state := params.Get("state")
			if tt.tamper != nil {
				state, cookie = tt.tamper(f, state, cookie)
			}

			rec := callback(router, state, cookie)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != tt.wantLocation {
				t.Fatalf("callback = %d to %q, want 302 to %q", rec.Code, rec.Header().Get("Location"), tt.wantLocation)
			}
			p := sessionPrincipal(t, rec)
			account, err := getGoogleAccount("google:110169484474386276334")
			if err != nil {
				t.Fatalf("getGoogleAccount: %v", err)
			}
			if tt.wantUser == "" {
				if p != nil || account != nil {
					t.Errorf("failed login created a session %+v / account %+v", p, account)
				}
				return
			}
			if p == nil || p.ID != "google:110169484474386276334" || p.User != tt.wantUser || p.Provider != ProviderGoogle || p.Domain != "example.com" {
				t.Errorf("session principal = %+v, want google user %s", p, tt.wantUser)
			}
			if account == nil || account.RefreshToken != "refresh-1" || account.Domain != "example.com" {
				t.Errorf("stored account = %+v, want one holding the refresh token", account)
			}
		})
	}
}

func TestGoogleCallbackStateIsSingleUse(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
	f := newFakeGoogle(t)
	router := newRouter()

	cookie, params := f.startLogin(router, "/")
	if rec := callback(router, params.Get("state"), cookie); rec.Header().Get("Location") != "/" {
		t.Fatalf("first callback redirects to %q, want /", rec.Header().Get("Location"))
	}
	rec := callback(router, params.Get("state"), cookie)
	if rec.Header().Get("Location") != "/login?error="+googleLoginFailed || sessionPrincipal(t, rec) != nil {
		t.Errorf("replayed callback redirects to %q, want a failed login", rec.Header().Get("Location"))
	}
}

func TestGoogleStateCookieSurvivesTheCrossSiteCallback(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	HandleGoogleLogin(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}
	c := cookies[0]
	if c.SameSite != http.SameSiteLaxMode || !c.HttpOnly || !c.Secure || c.Path != "/auth/google/" {
		t.Errorf("state cookie = SameSite:%d HttpOnly:%t Secure:%t Path:%q, want Lax, HttpOnly, Secure, /auth/google/", c.SameSite, c.HttpOnly, c.Secure, c.Path)
	}
}

func TestGoogleRoutesAreOffUnlessConfigured(t *testing.T) {
	newAuthTestEnv(t)
	for _, path := range []string{"/auth/google/login", "/auth/google/callback?code=x&state=y"} {
		rec := httptest.NewRecorder()
		newRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, rec.Code)
		}
	}
}

func TestGoogleLoginClientLimits(t *testing.T) {
	for _, finish := range []bool{false, true} {
		name := "pending logins"
		limit := maxPendingGoogleLoginsPerClient
		if finish {
			name, limit = "completed logins still count towards start rate", maxGoogleLoginStarts
		}
		t.Run(name, func(t *testing.T) {
			store := &googleLoginStore{logins: make(map[string]pendingGoogleLogin)}
			now := time.Now()
			for i := 0; i < limit; i++ {
				state := strconv.Itoa(i)
				if err := store.put(state, pendingGoogleLogin{client: "attacker", expires: now.Add(googleLoginTTL)}, now); err != nil {
					t.Fatalf("start %d: %v", i, err)
				}
				if finish {
					if _, ok := store.take(state, now); !ok {
						t.Fatal("could not complete admitted login")
					}
				}
			}
			var limited *googleLoginLimitError
			if err := store.put("blocked", pendingGoogleLogin{client: "attacker", expires: now.Add(googleLoginTTL)}, now); !errors.As(err, &limited) || limited.retryAfter <= 0 {
				t.Fatalf("excess start: %v, want a client limit with retry time", err)
			}
			if err := store.put("victim", pendingGoogleLogin{client: "victim", expires: now.Add(googleLoginTTL)}, now); err != nil {
				t.Fatalf("other client blocked: %v", err)
			}
			if !finish {
				if _, ok := store.take("0", now); !ok {
					t.Fatal("could not finish pending login")
				}
				if err := store.put("replacement", pendingGoogleLogin{client: "attacker", expires: now.Add(googleLoginTTL)}, now); err != nil {
					t.Fatalf("finished login did not release pending slot: %v", err)
				}
			}
			minuteLater := now.Add(googleLoginStartWindow)
			err := store.put("after-window", pendingGoogleLogin{client: "attacker", expires: minuteLater.Add(googleLoginTTL)}, minuteLater)
			if finish {
				if err != nil {
					t.Fatalf("start window did not reset: %v", err)
				}
				store.take("after-window", minuteLater)
			} else if err == nil {
				t.Fatal("pending limit reset before states expired")
			}
			later := now.Add(googleLoginTTL)
			if err := store.put("later", pendingGoogleLogin{client: "attacker", expires: later.Add(googleLoginTTL)}, later); err != nil {
				t.Fatalf("limit did not expire: %v", err)
			}
			if len(store.logins) != 1 || len(store.starts) != 1 {
				t.Fatalf("expired entries retained: %d logins, %d clients", len(store.logins), len(store.starts))
			}
		})
	}
}

func TestGoogleLoginFloodLeavesOtherClientsAccess(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
	router := newRouter()
	for i := 0; i < maxPendingGoogleLoginsPerClient+5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		want := http.StatusFound
		if i >= maxPendingGoogleLoginsPerClient {
			want = http.StatusTooManyRequests
			if rec.Header().Get("Retry-After") == "" || len(rec.Result().Cookies()) != 0 {
				t.Fatal("limited response must have Retry-After and no state cookie")
			}
		}
		if rec.Code != want {
			t.Fatalf("request %d: %d, want %d", i, rec.Code, want)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	req.RemoteAddr = "192.0.2.2:2345"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("other client: %d, want 302", rec.Code)
	}
}
