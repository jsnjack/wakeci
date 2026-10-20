package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func googleSessionCookie(t *testing.T) *http.Cookie {
	t.Helper()
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), googleUser)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return cookie
}

func sessionStatus(cookie *http.Cookie) int {
	req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	return rec.Code
}

// TestGoogleAccountChecker covers automatic offboarding: whatever Google
// says about an account decides whether its keys and sessions survive
func TestGoogleAccountChecker(t *testing.T) {
	activeClaims := func() map[string]any {
		claims := validGoogleClaims(time.Now())
		claims["aud"] = "client-id"
		claims["sub"] = "1001"
		claims["email"] = googleUser.User
		return claims
	}
	tests := []struct {
		name string
		// google sets up what the fake Google answers for "refresh-1"
		google        func(f *fakeGoogle)
		wantOffboard  bool
		wantVerifyNow bool
	}{
		{
			name:          "still active",
			google:        func(f *fakeGoogle) { f.refreshClaims["refresh-1"] = activeClaims() },
			wantVerifyNow: true,
		},
		{
			name:         "suspended, deleted or app access revoked",
			google:       func(f *fakeGoogle) {},
			wantOffboard: true,
		},
		{
			name: "moved to another workspace",
			google: func(f *fakeGoogle) {
				claims := activeClaims()
				claims["hd"] = "other.com"
				f.refreshClaims["refresh-1"] = claims
			},
			wantOffboard: true,
		},
		{
			name: "token for a different account",
			google: func(f *fakeGoogle) {
				claims := activeClaims()
				claims["sub"] = "9999"
				f.refreshClaims["refresh-1"] = claims
			},
			wantOffboard: true,
		},
		{
			name:   "google unreachable",
			google: func(f *fakeGoogle) { f.unavailable = true },
		},
		{
			name: "no id token, userinfo confirms",
			google: func(f *fakeGoogle) {
				f.refreshClaims["refresh-1"] = activeClaims()
				f.omitIDToken = true
				f.userInfo = map[string]any{"sub": "1001", "email": googleUser.User, "email_verified": true, "hd": "example.com"}
			},
			wantVerifyNow: true,
		},
		{
			name: "no id token, userinfo shows another workspace",
			google: func(f *fakeGoogle) {
				f.refreshClaims["refresh-1"] = activeClaims()
				f.omitIDToken = true
				f.userInfo = map[string]any{"sub": "1001", "email": "dev@other.com", "email_verified": true, "hd": "other.com"}
			},
			wantOffboard: true,
		},
		{
			// The stored domain must not be re-confirmed without current claims
			name: "no id token, userinfo unavailable",
			google: func(f *fakeGoogle) {
				f.refreshClaims["refresh-1"] = activeClaims()
				f.omitIDToken = true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
			f := newFakeGoogle(t)
			tt.google(f)

			lastVerified := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
			mustPutGoogleAccount(t, lastVerified)
			_, token := mustCreateAPIKey(t, googleUser, []Scope{ScopeRead}, 0, time.Now())
			cookie := googleSessionCookie(t)

			now := time.Now().Truncate(time.Second)
			verifyGoogleAccounts(context.Background(), L, now)

			account, err := getGoogleAccount(googleUser.ID)
			if err != nil {
				t.Fatalf("getGoogleAccount: %v", err)
			}
			keys, err := ListAPIKeys(googleUser.ID)
			if err != nil {
				t.Fatalf("ListAPIKeys: %v", err)
			}
			_, keyErr := VerifyAPIKey(L, token, now)

			if tt.wantOffboard {
				if account != nil || len(keys) != 0 {
					t.Errorf("after offboarding: account %+v, keys %v; want both gone", account, keys)
				}
				if !errors.Is(keyErr, errInvalidAPIKey) {
					t.Errorf("offboarded owner's key: error = %v, want errInvalidAPIKey", keyErr)
				}
				if status := sessionStatus(cookie); status != http.StatusForbidden {
					t.Errorf("offboarded owner's session: status = %d, want 403", status)
				}
			} else {
				if account == nil || len(keys) != 1 || keyErr != nil {
					t.Fatalf("account %+v, keys %v, key error %v; want the account and its working key kept", account, keys, keyErr)
				}
				wantVerified := lastVerified
				if tt.wantVerifyNow {
					wantVerified = now
				}
				if !account.VerifiedAt.Equal(wantVerified) {
					t.Errorf("VerifiedAt = %v, want %v", account.VerifiedAt, wantVerified)
				}
				if status := sessionStatus(cookie); status != http.StatusOK {
					t.Errorf("session: status = %d, want 200", status)
				}
			}
		})
	}
}

// TestGoogleAccountFailsClosed checks that keys and sessions are refused
// whenever Google hasn't recently vouched for the owner, even before the
// checker gets to offboard them
func TestGoogleAccountFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr bool
	}{
		{name: "confirmed recently", setup: func(t *testing.T) { mustPutGoogleAccount(t, time.Now().Add(-time.Minute)) }},
		{name: "not confirmed for too long", setup: func(t *testing.T) {
			mustPutGoogleAccount(t, time.Now().Add(-googleAccountMaxUnverified-time.Minute))
		}, wantErr: true},
		{name: "never signed in or offboarded", setup: func(t *testing.T) {}, wantErr: true},
		{name: "domain removed from config", setup: func(t *testing.T) {
			mustPutGoogleAccount(t, time.Now())
			Config.Google.AllowedDomains = []string{"other.com"}
		}, wantErr: true},
		{name: "google sign-in turned off", setup: func(t *testing.T) {
			mustPutGoogleAccount(t, time.Now())
			Config.Google = GoogleConfig{}
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
			_, token := mustCreateAPIKey(t, googleUser, []Scope{ScopeRead}, 0, time.Now())
			cookie := googleSessionCookie(t)
			tt.setup(t)

			_, err := VerifyAPIKey(L, token, time.Now())
			status := sessionStatus(cookie)
			if tt.wantErr {
				if !errors.Is(err, errInvalidAPIKey) || status != http.StatusForbidden {
					t.Errorf("key error = %v, session status = %d; want both refused", err, status)
				}
				return
			}
			if err != nil || status != http.StatusOK {
				t.Errorf("key error = %v, session status = %d; want both accepted", err, status)
			}
		})
	}
}

func TestGoogleLoginGetsARefreshToken(t *testing.T) {
	tests := []struct {
		name string
		// storedAccount pre-stores an account for the user, as a previous
		// login would have
		storedAccount bool
		// tokens are the refresh tokens Google returns on each code exchange
		tokens       []string
		wantLogins   int
		wantLocation string
		wantRefresh  string
	}{
		{name: "first login returns one", tokens: []string{"refresh-1"}, wantLogins: 1, wantLocation: "/build/5", wantRefresh: "refresh-1"},
		{name: "none returned, none stored: retry with consent", tokens: []string{"", "refresh-2"}, wantLogins: 2, wantLocation: "/build/5", wantRefresh: "refresh-2"},
		{name: "none even after consent", tokens: []string{"", ""}, wantLogins: 2, wantLocation: "/login?error=" + googleLoginFailed},
		{name: "none returned, stored one reused", storedAccount: true, tokens: []string{""}, wantLogins: 1, wantLocation: "/build/5", wantRefresh: "refresh-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
			f := newFakeGoogle(t)
			router := newRouter()
			const id = "google:110169484474386276334"
			if tt.storedAccount {
				if err := putGoogleAccount(&googleAccount{ID: id, Email: "dev@example.com", Domain: "example.com", RefreshToken: "refresh-1", VerifiedAt: time.Now()}); err != nil {
					t.Fatalf("putGoogleAccount: %v", err)
				}
			}

			redirect := "/build/5"
			var rec *httptest.ResponseRecorder
			for i, refreshToken := range tt.tokens {
				f.refreshToken = refreshToken
				loginPath := "/auth/google/login?redirect=" + url.QueryEscape(redirect)
				if i > 0 {
					// Follow wakeci's redirect back to the login
					loginPath = rec.Header().Get("Location")
				}
				cookie, params := f.startLoginAt(router, loginPath)
				wantPrompt := "select_account"
				if i > 0 {
					wantPrompt = "consent select_account"
				}
				if params.Get("prompt") != wantPrompt {
					t.Errorf("login %d prompt = %q, want %q", i+1, params.Get("prompt"), wantPrompt)
				}
				rec = callback(router, params.Get("state"), cookie)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLocation {
				t.Fatalf("after %d logins redirected to %q, want %q", len(tt.tokens), got, tt.wantLocation)
			}
			account, err := getGoogleAccount(id)
			if err != nil {
				t.Fatalf("getGoogleAccount: %v", err)
			}
			if tt.wantRefresh == "" {
				if account != nil || sessionPrincipal(t, rec) != nil {
					t.Errorf("failed login stored %+v or created a session", account)
				}
				return
			}
			if account == nil || account.RefreshToken != tt.wantRefresh {
				t.Errorf("stored account = %+v, want refresh token %q", account, tt.wantRefresh)
			}
			if sessionPrincipal(t, rec) == nil {
				t.Error("login created no session")
			}
		})
	}
}

// TestOffboardingIgnoresStaleVerdicts: when someone signs in again while the
// checker is asking Google about their account, what Google said about the
// older login must not take down the new one, its session or their keys.
// That holds even when the new login reused the stored refresh token, which
// it does whenever Google returns none.
func TestOffboardingIgnoresStaleVerdicts(t *testing.T) {
	for _, newToken := range []string{"refresh-2", "refresh-1"} {
		t.Run("new login with "+newToken, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
			// Knows no refresh tokens, so it rejects "refresh-1"
			f := newFakeGoogle(t)
			mustPutGoogleAccount(t, time.Now())
			_, token := mustCreateAPIKey(t, googleUser, []Scope{ScopeRead}, 0, time.Now())

			var cookie *http.Cookie
			f.onRefresh = func() {
				err := putGoogleAccount(&googleAccount{ID: googleUser.ID, Email: googleUser.User, Domain: googleUser.Domain, RefreshToken: newToken, VerifiedAt: time.Now()})
				if err != nil {
					t.Errorf("store the new login: %v", err)
				}
				cookie, err = GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), googleUser)
				if err != nil {
					t.Errorf("create the new session: %v", err)
				}
			}
			verifyGoogleAccounts(context.Background(), L, time.Now())

			account, err := getGoogleAccount(googleUser.ID)
			if err != nil || account == nil || account.RefreshToken != newToken {
				t.Fatalf("account = %+v (%v), want the new login kept", account, err)
			}
			if _, err := VerifyAPIKey(L, token, time.Now()); err != nil {
				t.Errorf("key: %v, want it kept", err)
			}
			if status := sessionStatus(cookie); status != http.StatusOK {
				t.Errorf("new session: status = %d, want 200", status)
			}
		})
	}
}

// TestCheckerKeepsNewerLoginData: a successful check must not overwrite what
// a login stored while Google was being asked
func TestCheckerKeepsNewerLoginData(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
	f := newFakeGoogle(t)
	claims := validGoogleClaims(time.Now())
	claims["aud"], claims["sub"], claims["email"] = "client-id", "1001", googleUser.User
	f.refreshClaims["refresh-1"] = claims
	mustPutGoogleAccount(t, time.Now().Add(-10*time.Minute))
	var login googleAccount
	f.onRefresh = func() {
		// The address was renamed; the checker still holds the old claims
		login = googleAccount{ID: googleUser.ID, Email: "renamed@example.com", Domain: googleUser.Domain, RefreshToken: "refresh-2", VerifiedAt: time.Now()}
		if err := putGoogleAccount(&login); err != nil {
			t.Errorf("store the new login: %v", err)
		}
	}
	verifyGoogleAccounts(context.Background(), L, time.Now())

	account, err := getGoogleAccount(googleUser.ID)
	if err != nil || account == nil {
		t.Fatalf("account = %+v (%v)", account, err)
	}
	if account.LoginID != login.LoginID || account.RefreshToken != "refresh-2" || account.Email != "renamed@example.com" {
		t.Errorf("account = %+v, want the newer login's data", account)
	}
}
