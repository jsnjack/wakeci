package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

func newAuthTestEnv(t *testing.T) {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "auth.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open auth test db: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(GlobalBucket)
		if err != nil {
			return err
		}
		return bucket.Put([]byte("password"), hash)
	}); err != nil {
		t.Fatalf("store test password: %v", err)
	}

	originalDB := DB
	originalSessions := GlobalSessionStorage
	originalLimiter := globalAuthAttemptLimiter
	DB = db
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	globalAuthAttemptLimiter = newAuthAttemptLimiter(2, time.Minute, time.Minute)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close auth test db: %v", err)
		}
		DB = originalDB
		GlobalSessionStorage = originalSessions
		globalAuthAttemptLimiter = originalLimiter
	})
}

func TestAuthenticationRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		request func() *http.Request
		// wantRefused is the answer to a wrong password before the limit
		wantRefused int
	}{
		{
			name:        "form login",
			wantRefused: http.StatusForbidden,
			handler:     http.HandlerFunc(HandleLogIn),
			request: func() *http.Request {
				form := url.Values{"password": {"wrong"}}
				req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req
			},
		},
		{
			name:        "basic auth",
			wantRefused: http.StatusUnauthorized,
			handler: AuthMi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
				req.SetBasicAuth("", "wrong")
				return req
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			for attempt, wantStatus := range []int{tt.wantRefused, http.StatusTooManyRequests} {
				req := tt.request()
				req.RemoteAddr = "192.0.2.10:1234"
				response := httptest.NewRecorder()
				tt.handler.ServeHTTP(response, req)
				if response.Code != wantStatus {
					t.Errorf("attempt %d status = %d, want %d", attempt+1, response.Code, wantStatus)
				}
				if wantStatus == http.StatusTooManyRequests {
					if response.Header().Get("Retry-After") == "" {
						t.Error("rate-limited response has no Retry-After header")
					}
					reset, err := strconv.ParseInt(response.Header().Get("X-RateLimit-Reset"), 10, 64)
					if err != nil {
						t.Errorf("X-RateLimit-Reset is not a Unix timestamp: %v", err)
					} else if reset <= time.Now().Unix() {
						t.Errorf("X-RateLimit-Reset = %d, want a future timestamp", reset)
					}
					if response.Header().Get("X-RateLimit-Limit") != "2" {
						t.Errorf("X-RateLimit-Limit = %q, want 2", response.Header().Get("X-RateLimit-Limit"))
					}
				}
			}
		})
	}
}

func TestHandleLogOutClearsCookieWithSessionAttributes(t *testing.T) {
	newAuthTestEnv(t)
	originalConfig := Config
	Config = &WakeConfig{Port: "8081"}
	t.Cleanup(func() { Config = originalConfig })

	GlobalSessionStorage.sessions[sessionKey("session-token")] = session{expires: time.Now().Add(time.Hour)}
	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})
	rec := httptest.NewRecorder()

	HandleLogOut(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != -1 {
		t.Errorf("logout cookie attributes = Path:%q HttpOnly:%t Secure:%t SameSite:%d MaxAge:%d", cookie.Path, cookie.HttpOnly, cookie.Secure, cookie.SameSite, cookie.MaxAge)
	}
	if !cookie.Expires.Before(time.Now()) {
		t.Errorf("logout cookie expiry = %v, want a past time", cookie.Expires)
	}
	if err := GlobalSessionStorage.Verify("session-token"); err == nil {
		t.Error("session remains valid after logout")
	}
}

// Missing or wrong credentials get 401, and a browser is never asked for a
// password by a challenge on the page's own requests; a valid login that
// isn't allowed in gets 403
func TestAuthMiRefusals(t *testing.T) {
	newAPIKeyTestEnv(t)
	addTestGroup(t, "Nothing", Grant{Scopes: []string{ScopeWrite}, Jobs: []string{"deploy"}})
	ended := loginAs(t, "alice@example.com")
	GlobalSessionStorage.DeleteUser("alice@example.com")
	nothing := loginInGroups(t, "bob@example.com", "Nothing")

	tests := []struct {
		name          string
		prepare       func(*http.Request)
		wantStatus    int
		wantChallenge string
		wantBody      string
	}{
		{"no credentials", nil, http.StatusUnauthorized, `Bearer realm="wakeci"`, "Not logged in"},
		{"wrong password", func(r *http.Request) { r.SetBasicAuth("", "wrong") }, http.StatusUnauthorized, `Basic realm="wakeci", charset="UTF-8"`, "Wrong password"},
		{"ended session", func(r *http.Request) { r.AddCookie(ended) }, http.StatusUnauthorized, `Bearer realm="wakeci"`, "The session ended"},
		{"groups that allow nothing", func(r *http.Request) { r.AddCookie(nothing) }, http.StatusForbidden, "", "None of your groups has access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := apiCall("GET", "/api/jobs/", nil, nil, tt.prepare)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tt.wantChallenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tt.wantChallenge)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
