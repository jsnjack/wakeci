package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

func TestPreviewCredentialRevocation(t *testing.T) {
	for _, kind := range []string{viaSession, viaBasic, viaAPIKey} {
		t.Run(kind, func(t *testing.T) {
			newAuthTestEnv(t)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			var revoke func()
			switch kind {
			case viaSession:
				cookie, err := GlobalSessionStorage.New(req, passwordPrincipal(viaSession))
				if err != nil {
					t.Fatal(err)
				}
				req.AddCookie(cookie)
				req = withPrincipal(req, passwordPrincipal(viaSession))
				revoke = func() {
					if err := GlobalSessionStorage.Delete(cookie.Value); err != nil {
						t.Fatal(err)
					}
				}
			case viaBasic:
				req = withPrincipal(req, passwordPrincipal(viaBasic))
				hash, err := readPasswordHash()
				if err != nil {
					t.Fatal(err)
				}
				req = req.WithContext(context.WithValue(req.Context(), verifiedPasswordHashKey{}, hash))
				revoke = func() {
					if err := DB.Update(func(tx *bolt.Tx) error { return tx.Bucket(GlobalBucket).Put([]byte("password"), []byte("changed")) }); err != nil {
						t.Fatal(err)
					}
				}
			case viaAPIKey:
				key, _ := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
				req = withPrincipal(req, &Principal{ID: key.OwnerID, Provider: key.OwnerProvider, Via: viaAPIKey, KeyID: key.ID})
				revoke = func() {
					if err := RevokeAPIKey(key.OwnerID, key.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			token, err := newPreviewToken(req, 1, "/1/artifacts/a.html", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := checkPreviewToken(token, time.Now()); err != nil {
				t.Fatalf("fresh token: %v", err)
			}
			revoke()
			if _, err := checkPreviewToken(token, time.Now()); !errors.Is(err, errInvalidPreview) {
				t.Fatalf("revoked token accepted: %v", err)
			}
		})
	}
}

func TestStoredFileIsolation(t *testing.T) {
	newJobAccessTestEnv(t)
	root := Config.WorkDir + "wakespace/1/"
	for _, dir := range []string{"artifacts/report", "page", "private"} {
		if err := os.MkdirAll(root+dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"artifacts/report/index.html", "page/index.html", "private/hidden.html"} {
		if err := os.WriteFile(root+file, []byte("content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("OUTSIDE-SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root+"artifacts/escape.txt"); err != nil {
		t.Fatal(err)
	}
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	get := func(target string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if authenticated {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		newRouter().ServeHTTP(rec, req)
		return rec
	}
	for _, page := range []string{"artifacts/report/index.html", "page/index.html"} {
		t.Run(page, func(t *testing.T) {
			redirect := get("/storage/build/1/"+page, true)
			location := redirect.Header().Get("Location")
			if redirect.Code != http.StatusFound {
				t.Fatalf("redirect: %d", redirect.Code)
			}
			token := strings.Split(location, "/")[2]
			claims, err := checkPreviewToken(token, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			want := "/1/page/"
			if strings.HasPrefix(page, "artifacts/") {
				want = "/1/artifacts/"
			}
			if claims.Directory != want {
				t.Fatalf("scope = %q want %q", claims.Directory, want)
			}
			for _, target := range []string{"/preview/" + token + "/build/1/private/hidden.html", "/preview/" + token + "/build/1/artifacts/escape.txt"} {
				rec := get(target, false)
				if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "OUTSIDE-SECRET") {
					t.Fatalf("escaped scope: %d %s", rec.Code, rec.Body.String())
				}
			}
			pageResponse := get(location, false)
			if pageResponse.Code == http.StatusMovedPermanently {
				base, err := url.Parse(location)
				if err != nil {
					t.Fatal(err)
				}
				relative, err := url.Parse(pageResponse.Header().Get("Location"))
				if err != nil {
					t.Fatal(err)
				}
				pageResponse = get(base.ResolveReference(relative).String(), false)
			}
			if pageResponse.Code != http.StatusOK {
				t.Fatalf("report: %d", pageResponse.Code)
			}
		})
	}
	escaped := get("/storage/build/1/artifacts/escape.txt", true)
	if escaped.Code != http.StatusNotFound {
		t.Fatalf("external symlink: %d", escaped.Code)
	}
	if err := os.WriteFile(root+"page/a #?%.html", []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	escapedLocation := get("/storage/build/1/page/"+url.PathEscape("a #?%.html"), true).Header().Get("Location")
	if !strings.Contains(escapedLocation, "%23%3F%25.html") {
		t.Fatalf("unescaped redirect: %q", escapedLocation)
	}
}

func TestArtifactHeadersAndSubresources(t *testing.T) {
	for _, dest := range []string{"script", "worker", "sharedworker", "serviceworker", "style", "document", "image"} {
		t.Run(dest, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/preview/dummy/build/1/x", nil)
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			req.Header.Set("Sec-Fetch-Dest", dest)
			rec := httptest.NewRecorder()
			ArtifactSecurityMi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(rec, req)
			want := http.StatusForbidden
			if dest == "document" || dest == "image" {
				want = http.StatusNoContent
			}
			if rec.Code != want {
				t.Fatalf("status %d want %d", rec.Code, want)
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(rec.Header().Get("Vary"), "Authorization") {
				t.Fatal("missing private cache policy")
			}
			for _, unsafe := range []string{"allow-same-origin", "allow-popups", "allow-modals"} {
				if strings.Contains(rec.Header().Get("Content-Security-Policy"), unsafe) {
					t.Fatalf("unsafe sandbox permission %s", unsafe)
				}
			}
		})
	}
	for _, target := range []string{"/preview/SECRET", "/preview/SECRET/build/1/a"} {
		if strings.Contains(logPath(httptest.NewRequest(http.MethodGet, target, nil)), "SECRET") {
			t.Fatal("token leaked to access log")
		}
	}
}

func TestRequestSecurity(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, origin, fetchSite string
		body                                  []byte
		want                                  int
	}{
		{name: "cross-origin login", method: http.MethodPost, path: "/auth/login", origin: "https://evil.test", fetchSite: "cross-site", body: []byte("password=correct"), want: http.StatusForbidden},
		{name: "same-site foreign origin", method: http.MethodPost, path: "/auth/login", origin: "http://other.example.com", body: []byte("password=correct"), want: http.StatusForbidden},
		{name: "oversized", method: http.MethodPost, path: "/auth/login", body: bytes.Repeat([]byte("x"), maxRequestBodyBytes+1), want: http.StatusRequestEntityTooLarge},
		{name: "password only in query", method: http.MethodPost, path: "/auth/login?password=correct", want: http.StatusForbidden},
		{name: "form login", method: http.MethodPost, path: "/auth/login", body: []byte("password=correct"), want: http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status %d want %d", rec.Code, tt.want)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("missing no-store: %v", rec.Header())
			}
		})
	}
}

func TestPasswordSessionFingerprint(t *testing.T) {
	newAuthTestEnv(t)
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GlobalBucket).Put([]byte("password"), []byte("changed hash"))
	}); err != nil {
		t.Fatal(err)
	}
	if status := sessionStatus(cookie); status != http.StatusForbidden {
		t.Fatalf("old session survived fingerprint change: %d", status)
	}
	if err := checkSessionPrincipal(&Principal{Provider: "unknown"}, time.Now()); !isRevoked(err) {
		t.Fatalf("unknown provider accepted: %v", err)
	}
}

func TestDefaultPasswordWithGoogle(t *testing.T) {
	for _, basic := range []bool{false, true} {
		t.Run(map[bool]string{false: "login", true: "basic"}[basic], func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
			hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
			if err != nil {
				t.Fatal(err)
			}
			if err := DB.Update(func(tx *bolt.Tx) error { return tx.Bucket(GlobalBucket).Put([]byte("password"), hash) }); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("password=admin"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if basic {
				req = httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
				req.SetBasicAuth("", "admin")
			}
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("default password accepted: %d", rec.Code)
			}
		})
	}
}

func TestGoogleErrorConsumesState(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
	now := time.Now()
	if err := googleLogins.put("state", pendingGoogleLogin{client: "x", expires: now.Add(googleLoginTTL)}, now); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=state&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: googleStateCookie, Value: "state"})
	newRouter().ServeHTTP(httptest.NewRecorder(), req)
	if _, ok := googleLogins.take("state", now); ok {
		t.Fatal("error callback left replayable state")
	}
}

func TestStaleGoogleSessionCannotBeCreated(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
	mustPutGoogleAccount(t, time.Now())
	account, err := getGoogleAccount(googleUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := offboardGoogleAccount(L, *account, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := newGoogleSession(httptest.NewRequest(http.MethodGet, "/", nil), googleUser, account); !errors.Is(err, errGoogleAccountInactive) {
		t.Fatalf("offboarded login issued session: %v", err)
	}
}

func TestLegacyWriteKeyRestrictions(t *testing.T) {
	newAuthTestEnv(t)
	key := &APIKey{OwnerID: "password:admin", OwnerProvider: ProviderPassword, Scopes: []Scope{ScopeWrite}, Jobs: []string{"limited"}}
	if err := checkAPIKey(key, time.Now()); !errors.Is(err, errInvalidAPIKey) {
		t.Fatalf("legacy restricted write key accepted: %v", err)
	}
}

func TestGoogleInvalidTokenDoesNotDeleteAccount(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
	fake := newFakeGoogle(t)
	claims := validGoogleClaims(time.Now())
	claims["aud"] = "wrong-client"
	fake.refreshClaims["refresh-1"] = claims
	mustPutGoogleAccount(t, time.Now())
	checked, err := getGoogleAccount(googleUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyGoogleAccount(context.Background(), L, *checked, time.Now()); err == nil {
		t.Fatal("invalid token accepted")
	}
	account, err := getGoogleAccount(googleUser.ID)
	if err != nil || account == nil {
		t.Fatalf("parse failure deleted account: %v", err)
	}
}

func TestPendingGoogleLoginEviction(t *testing.T) {
	now := time.Now()
	store := &googleLoginStore{logins: map[string]pendingGoogleLogin{}}
	for i := 0; i < maxPendingGoogleLogins; i++ {
		state := string(rune(i + 1))
		store.logins[state] = pendingGoogleLogin{client: "other", expires: now.Add(googleLoginTTL + time.Duration(i)*time.Second)}
	}
	if err := store.put("new", pendingGoogleLogin{client: "new-client", expires: now.Add(googleLoginTTL)}, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.logins[string(rune(1))]; ok {
		t.Fatal("oldest entry was not evicted")
	}
	if len(store.logins) != maxPendingGoogleLogins {
		t.Fatal("store is no longer bounded")
	}
}

func TestPreviewDoesNotContainSessionCookie(t *testing.T) {
	newAuthTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	cookie, err := GlobalSessionStorage.New(req, passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	req = withPrincipal(req, passwordPrincipal(viaSession))
	token, err := newPreviewToken(req, 1, "/1/artifacts/index.html", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	claims, err := checkPreviewToken(token, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(cookie.Value)) {
		t.Fatal("session cookie leaked into signed URL")
	}
	if claims.Expires > time.Now().Add(15*time.Minute).Unix() {
		t.Fatal("preview lifetime exceeds fifteen minutes")
	}
}

type bodyChangeReader struct {
	io.Reader
	change  func()
	changed bool
}

func (r *bodyChangeReader) Read(data []byte) (int, error) {
	if !r.changed {
		r.changed = true
		r.change()
	}
	return r.Reader.Read(data)
}

func TestRequestBodyArrivesBeforeAuthentication(t *testing.T) {
	newJobAccessTestEnv(t)
	body := &bodyChangeReader{Reader: strings.NewReader("active=false"), change: func() {
		hash, err := bcrypt.GenerateFromPassword([]byte("replacement"), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		if err := DB.Update(func(tx *bolt.Tx) error { return tx.Bucket(GlobalBucket).Put([]byte("password"), hash) }); err != nil {
			t.Fatal(err)
		}
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/job/test_unit/set_active", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("", "correct")
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	if !body.changed || rec.Code != http.StatusForbidden {
		t.Fatalf("authenticated before body arrival: changed=%t status=%d", body.changed, rec.Code)
	}
}

func TestSettingsPasswordQueryIgnored(t *testing.T) {
	newAuthTestEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	before, err := readPasswordHash()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/settings?password=unexpected", strings.NewReader("concurrentBuilds=1&buildHistorySize=10"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("", "correct")
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	after, err := readPasswordHash()
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !bytes.Equal(before, after) {
		t.Fatalf("query changed password: %d", rec.Code)
	}
}

func TestLogoutRequiresPost(t *testing.T) {
	newAuthTestEnv(t)
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET logout: %d", rec.Code)
	}
	if _, err := GlobalSessionStorage.Verify(cookie.Value); err != nil {
		t.Fatal("GET ended the session")
	}
}

func TestBarePreviewNeverLogsBearerToken(t *testing.T) {
	newAuthTestEnv(t)
	var output bytes.Buffer
	original := L
	L = slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: LevelTrace}))
	t.Cleanup(func() { L = original })
	req := httptest.NewRequest(http.MethodGet, "/preview/REVIEW-PRIVATE-PREVIEW", nil)
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	if strings.Contains(output.String(), "REVIEW-PRIVATE-PREVIEW") {
		t.Fatal("debug log exposed preview bearer token")
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("malformed preview response is cacheable")
	}
}

func TestJobEditsRequireExplicitTasks(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		status        int
	}{
		{"missing tasks", "desc: not a job\n", http.StatusBadRequest},
		{"empty job", "desc: empty\ntasks: []\n", http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newJobAccessTestEnv(t)
			form := url.Values{"fileContent": {tt.content}}
			req := httptest.NewRequest(http.MethodPost, "/api/job/test_unit", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.SetBasicAuth("", "correct")
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status %d want %d", rec.Code, tt.status)
			}
		})
	}
}

func TestCredentialErrorsDoNotExposeSecrets(t *testing.T) {
	for _, state := range []string{"unknown", "expired", "delete", "principal log"} {
		t.Run(state, func(t *testing.T) {
			secret := "sensitive-test-credential"
			storage := &SessionStorage{sessions: make(map[string]session)}
			var output string
			switch state {
			case "unknown":
				_, err := storage.Verify(secret)
				output = err.Error()
			case "expired":
				storage.sessions[secret] = session{expires: time.Now().Add(-time.Hour)}
				_, err := storage.Verify(secret)
				output = err.Error()
			case "delete":
				output = storage.Delete(secret).Error()
			case "principal log":
				var buf bytes.Buffer
				logger := slog.New(slog.NewTextHandler(&buf, nil))
				logger.Warn("denied", "principal", &Principal{ID: "password:admin", passwordFingerprint: secret})
				output = buf.String()
				if !strings.Contains(output, "password:admin") {
					t.Fatal("principal identity missing from log")
				}
			}
			if strings.Contains(output, secret) {
				t.Fatal("credential exposed in error or log")
			}
		})
	}
}
