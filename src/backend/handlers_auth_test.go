package main

import (
	"context"
	"log/slog"
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

type passwordRotationLogHandler struct {
	slog.Handler
	afterInvalidation func()
}

func (h passwordRotationLogHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Message == "shared password changed" {
		h.afterInvalidation()
	}
	return h.Handler.Handle(ctx, record)
}

func TestPasswordRotationPreservesNewSessions(t *testing.T) {
	newAuthTestEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	oldCookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	invalidated := make(chan struct{})
	resume := make(chan struct{})
	logger := slog.New(passwordRotationLogHandler{Handler: L.Handler(), afterInvalidation: func() {
		close(invalidated)
		<-resume
	}})
	form := url.Values{"password": {"replacement"}, "concurrentBuilds": {"1"}, "buildHistorySize": {"10"}}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), HL, logger))
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		HandleSettingsPost(rec, req)
		close(done)
	}()
	<-invalidated

	// Model overlapping issuance with the newly committed hash. Capture its
	// start before resuming so either scheduling order of a later sweep is covered.
	issuedAt := time.Now()
	GlobalSessionStorage.mu.Lock()
	committed := make(chan []byte, 1)
	dbErr := make(chan error, 1)
	go func() {
		dbErr <- DB.Update(func(tx *bolt.Tx) error {
			committed <- append([]byte(nil), tx.Bucket(GlobalBucket).Get([]byte("password"))...)
			return nil
		})
	}()
	close(resume)
	hash := <-committed
	principal := passwordPrincipal(viaSession)
	principal.passwordFingerprint = passwordFingerprint(hash)
	const newToken = "new-password-session"
	now := time.Now()
	GlobalSessionStorage.sessions[newToken] = session{created: issuedAt, expires: now.Add(SessionTTL), principal: *principal}
	GlobalSessionStorage.mu.Unlock()
	<-done
	if err := <-dbErr; err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body.String())
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte("replacement")); err != nil {
		t.Fatalf("session did not use the committed replacement hash: %v", err)
	}
	if got := sessionStatus(oldCookie); got != http.StatusForbidden {
		t.Errorf("old session: %d, want 403", got)
	}
	if got := sessionStatus(&http.Cookie{Name: "session", Value: newToken}); got != http.StatusOK {
		t.Errorf("new-password session: %d, want 200", got)
	}
}

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
		for _, b := range [][]byte{APIKeysBucket, GoogleAccountsBucket} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return bucket.Put([]byte("password"), hash)
	}); err != nil {
		t.Fatalf("store test password: %v", err)
	}

	originalDB := DB
	originalConfig := Config
	originalSessions := GlobalSessionStorage
	originalLimiter := globalAuthAttemptLimiter
	originalGoogleLogins := googleLogins
	googleLogins = &googleLoginStore{logins: make(map[string]pendingGoogleLogin)}
	DB = db
	Config = &WakeConfig{Port: "8081"}
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	globalAuthAttemptLimiter = newAuthAttemptLimiter(2, time.Minute, time.Minute)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close auth test db: %v", err)
		}
		DB = originalDB
		Config = originalConfig
		GlobalSessionStorage = originalSessions
		globalAuthAttemptLimiter = originalLimiter
		googleLogins = originalGoogleLogins
	})
}

func TestAuthenticationRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		request func() *http.Request
	}{
		{
			name:    "form login",
			handler: http.HandlerFunc(HandleLogIn),
			request: func() *http.Request {
				form := url.Values{"password": {"wrong"}}
				req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req
			},
		},
		{
			name: "basic auth",
			handler: AuthMi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
				req.SetBasicAuth("", "wrong")
				return req
			},
		},
		{
			name: "api key",
			handler: AuthMi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
				req.Header.Set("Authorization", "Bearer wcik_"+strings.Repeat("0", apiKeyIDLength+apiKeySecretLength))
				return req
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			for attempt, wantStatus := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
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

	GlobalSessionStorage.sessions["session-token"] = session{expires: time.Now().Add(time.Hour)}
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
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
	if _, err := GlobalSessionStorage.Verify("session-token"); err == nil {
		t.Error("session remains valid after logout")
	}
}

func TestPasswordChangeRevokesPasswordSessions(t *testing.T) {
	for _, actor := range []string{ProviderPassword, ProviderGoogle} {
		t.Run(actor, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
			mustPutGoogleAccount(t, time.Now())
			oldQueue := GlobalQueue
			GlobalQueue = &Queue{}
			t.Cleanup(func() { GlobalQueue = oldQueue })
			newSession := func(p *Principal) *http.Cookie {
				cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), p)
				if err != nil {
					t.Fatal(err)
				}
				return cookie
			}
			first := newSession(passwordPrincipal(viaSession))
			second := newSession(passwordPrincipal(viaSession))
			google := newSession(googleUser)
			cookie := first
			if actor == ProviderGoogle {
				cookie = google
			}
			form := url.Values{"password": {"replacement"}, "concurrentBuilds": {"1"}, "buildHistorySize": {"10"}}
			req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("rotate: %d %s", rec.Code, rec.Body.String())
			}
			for i, old := range []*http.Cookie{first, second} {
				if got := sessionStatus(old); got != http.StatusForbidden {
					t.Errorf("old session %d: %d, want 403", i, got)
				}
			}
			if got := sessionStatus(google); got != http.StatusOK {
				t.Errorf("Google session: %d, want 200", got)
			}
			for _, password := range []string{"correct", "replacement"} {
				form := url.Values{"password": {password}}
				req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				newRouter().ServeHTTP(rec, req)
				want := http.StatusForbidden
				if password == "replacement" {
					want = http.StatusNoContent
				}
				if rec.Code != want {
					t.Errorf("login with %q: %d, want %d", password, rec.Code, want)
				}
			}
		})
	}
}

// TestAPIKeyDoesNotResetPasswordLockout: someone holding any valid key
// could otherwise guess the shared password forever, using the key to wipe
// out each failed guess before the lockout kicks in
func TestAPIKeyDoesNotResetPasswordLockout(t *testing.T) {
	newAuthTestEnv(t)
	_, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
	send := func(auth func(req *http.Request)) int {
		req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
		auth(req)
		rec := httptest.NewRecorder()
		newRouter().ServeHTTP(rec, req)
		return rec.Code
	}
	wrongPassword := func(req *http.Request) { req.SetBasicAuth("", "wrong") }
	validKey := func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) }

	for i, step := range []struct {
		auth       func(req *http.Request)
		wantStatus int
	}{
		{wrongPassword, http.StatusForbidden},
		{validKey, http.StatusOK},
		{wrongPassword, http.StatusTooManyRequests},
	} {
		if got := send(step.auth); got != step.wantStatus {
			t.Errorf("request %d: status = %d, want %d", i+1, got, step.wantStatus)
		}
	}
}
