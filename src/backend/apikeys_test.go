package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

var googleUser = &Principal{
	ID:       "google:1001",
	User:     "dev@example.com",
	Provider: ProviderGoogle,
	Domain:   "example.com",
	Scopes:   AllScopes,
	Via:      viaSession,
}

// mustPutGoogleAccount records that Google confirmed googleUser at verifiedAt
func mustPutGoogleAccount(t *testing.T, verifiedAt time.Time) {
	t.Helper()
	err := putGoogleAccount(&googleAccount{
		ID:           googleUser.ID,
		Email:        googleUser.User,
		Domain:       googleUser.Domain,
		RefreshToken: "refresh-1",
		VerifiedAt:   verifiedAt,
	})
	if err != nil {
		t.Fatalf("putGoogleAccount: %v", err)
	}
}

func mustCreateAPIKey(t *testing.T, owner *Principal, scopes []Scope, ttl time.Duration, now time.Time) (*APIKey, string) {
	t.Helper()
	key, token, err := CreateAPIKey(owner, "test key", scopes, nil, ttl, now, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	return key, token
}

func TestVerifyAPIKey(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// token builds the presented token from a freshly created key's token
		token          func(token string) string
		ttl            time.Duration
		at             time.Time
		allowedDomains []string
		wantErr        bool
	}{
		{name: "valid", token: func(tok string) string { return tok }, at: now},
		{name: "wrong secret", token: func(tok string) string { return tok[:len(tok)-1] + "x" }, at: now, wantErr: true},
		{name: "unknown id", token: func(string) string { return "wcik_0123456789abcdef" + strings.Repeat("a", apiKeySecretLength) }, at: now, wantErr: true},
		{name: "missing prefix", token: func(tok string) string { return strings.TrimPrefix(tok, "wcik_") }, at: now, wantErr: true},
		{name: "too short", token: func(tok string) string { return tok[:len(tok)-1] }, at: now, wantErr: true},
		{name: "too long", token: func(tok string) string { return tok + "0" }, at: now, wantErr: true},
		{name: "not expired yet", token: func(tok string) string { return tok }, ttl: time.Hour, at: now.Add(59 * time.Minute)},
		{name: "expired", token: func(tok string) string { return tok }, ttl: time.Hour, at: now.Add(time.Hour), wantErr: true},
		{name: "owner's domain no longer allowed", token: func(tok string) string { return tok }, at: now, allowedDomains: []string{"other.com"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			domains := []string{"example.com"}
			if tt.allowedDomains != nil {
				domains = tt.allowedDomains
			}
			Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: domains}
			mustPutGoogleAccount(t, tt.at)

			key, token := mustCreateAPIKey(t, googleUser, []Scope{ScopeRead}, tt.ttl, now)
			principal, err := VerifyAPIKey(L, tt.token(token), tt.at)
			if tt.wantErr {
				if !errors.Is(err, errInvalidAPIKey) {
					t.Fatalf("VerifyAPIKey error = %v, want errInvalidAPIKey", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyAPIKey: %v", err)
			}
			if principal.ID != googleUser.ID || principal.User != googleUser.User || principal.KeyID != key.ID || principal.Via != viaAPIKey {
				t.Errorf("principal = %+v, want user %s via api key %s", principal, googleUser.User, key.ID)
			}
			if !slices.Equal(principal.Scopes, []Scope{ScopeRead}) {
				t.Errorf("principal scopes = %q, want only read", principal.Scopes)
			}
		})
	}
}

func TestVerifyAPIKeyThrottlesLastUsedWrites(t *testing.T) {
	newAuthTestEnv(t)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	_, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, now)

	lastUsed := func() *time.Time {
		keys, err := ListAPIKeys(passwordPrincipal(viaSession).ID)
		if err != nil || len(keys) != 1 {
			t.Fatalf("ListAPIKeys = %v, %v; want one key", keys, err)
		}
		return keys[0].LastUsedAt
	}
	for _, step := range []struct {
		at   time.Time
		want time.Time
	}{
		{at: now, want: now},
		{at: now.Add(30 * time.Second), want: now},
		{at: now.Add(time.Minute), want: now.Add(time.Minute)},
	} {
		if _, err := VerifyAPIKey(L, token, step.at); err != nil {
			t.Fatalf("VerifyAPIKey at %v: %v", step.at, err)
		}
		if got := lastUsed(); got == nil || !got.Equal(step.want) {
			t.Errorf("after use at %v LastUsedAt = %v, want %v", step.at, got, step.want)
		}
	}
}

func TestRevokeAPIKey(t *testing.T) {
	newAuthTestEnv(t)
	Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
	mustPutGoogleAccount(t, time.Now())
	key, token := mustCreateAPIKey(t, googleUser, []Scope{ScopeRead}, 0, time.Now())

	// A different Google account that has since been given the same email
	reassigned := *googleUser
	reassigned.ID = "google:2002"
	if err := putGoogleAccount(&googleAccount{ID: reassigned.ID, Email: reassigned.User, Domain: reassigned.Domain, VerifiedAt: time.Now()}); err != nil {
		t.Fatalf("putGoogleAccount: %v", err)
	}
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), &reassigned)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	asReassigned := func(method string, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		newRouter().ServeHTTP(rec, req)
		return rec
	}
	if rec := asReassigned(http.MethodGet, "/api/keys/"); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("same email, other account lists keys: %d %s, want 200 []", rec.Code, rec.Body.String())
	}
	if rec := asReassigned(http.MethodDelete, "/api/keys/"+key.ID); rec.Code != http.StatusNotFound {
		t.Errorf("same email, other account revokes key: status = %d, want 404", rec.Code)
	}
	if _, err := VerifyAPIKey(L, token, time.Now()); err != nil {
		t.Fatalf("key stopped working after a rejected revoke: %v", err)
	}

	if err := RevokeAPIKey(googleUser.ID, key.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, err := VerifyAPIKey(L, token, time.Now()); !errors.Is(err, errInvalidAPIKey) {
		t.Errorf("revoked key: error = %v, want errInvalidAPIKey", err)
	}
}

func TestHandleAPIKeysCreate(t *testing.T) {
	readOnly := &Principal{ID: "google:1002", User: "ro@example.com", Provider: ProviderGoogle, Domain: "example.com", Scopes: []Scope{ScopeRead}, Via: viaSession}
	tests := []struct {
		name       string
		principal  *Principal
		form       url.Values
		wantStatus int
		wantScopes []Scope
		wantJobs   []string
		wantExpiry bool
	}{
		{name: "repeated scopes", form: url.Values{"name": {"ci"}, "scope": {"run", "read"}}, wantStatus: http.StatusOK, wantScopes: []Scope{ScopeRead, ScopeRun}},
		{name: "comma separated scopes", form: url.Values{"name": {"ci"}, "scope": {"read,write"}}, wantStatus: http.StatusOK, wantScopes: []Scope{ScopeRead, ScopeWrite}},
		{name: "with expiry", form: url.Values{"name": {"ci"}, "scope": {"read"}, "expiresInDays": {"30"}}, wantStatus: http.StatusOK, wantScopes: []Scope{ScopeRead}, wantExpiry: true},
		{name: "limited to jobs", form: url.Values{"name": {"ci"}, "scope": {"run"}, "job": {"test_*", "deploy"}}, wantStatus: http.StatusOK, wantScopes: []Scope{ScopeRun}, wantJobs: []string{"test_*", "deploy"}},
		{name: "invalid job pattern", form: url.Values{"name": {"ci"}, "scope": {"run"}, "job": {"a/b"}}, wantStatus: http.StatusBadRequest},
		{name: "missing name", form: url.Values{"scope": {"read"}}, wantStatus: http.StatusBadRequest},
		{name: "name too long", form: url.Values{"name": {strings.Repeat("n", maxAPIKeyNameLength+1)}, "scope": {"read"}}, wantStatus: http.StatusBadRequest},
		{name: "no scopes", form: url.Values{"name": {"ci"}}, wantStatus: http.StatusBadRequest},
		{name: "unknown scope", form: url.Values{"name": {"ci"}, "scope": {"sudo"}}, wantStatus: http.StatusBadRequest},
		{name: "admin scope", form: url.Values{"name": {"ci"}, "scope": {"read", "admin"}}, wantStatus: http.StatusBadRequest},
		{name: "negative expiry", form: url.Values{"name": {"ci"}, "scope": {"read"}, "expiresInDays": {"-1"}}, wantStatus: http.StatusBadRequest},
		{name: "scope the creator lacks", principal: readOnly, form: url.Values{"name": {"ci"}, "scope": {"read", "run"}}, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
			mustPutGoogleAccount(t, time.Now())
			principal := tt.principal
			if principal == nil {
				principal = googleUser
			}
			req := httptest.NewRequest(http.MethodPost, "/api/keys/", strings.NewReader(tt.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			cookie, err := GlobalSessionStorage.New(req, principal)
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			HandleAPIKeysCreate(rec, withPrincipal(req, principal))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var created CreatedAPIKeyData
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if strings.Contains(rec.Body.String(), "secretHash") {
				t.Error("response leaks the stored secret hash")
			}
			if !slices.Equal(created.Key.Scopes, tt.wantScopes) {
				t.Errorf("key scopes = %q, want %q", created.Key.Scopes, tt.wantScopes)
			}
			if (created.Key.ExpiresAt != nil) != tt.wantExpiry {
				t.Errorf("key expiresAt = %v, want set: %t", created.Key.ExpiresAt, tt.wantExpiry)
			}
			got, err := VerifyAPIKey(L, created.Token, time.Now())
			if err != nil {
				t.Fatalf("returned token doesn't verify: %v", err)
			}
			if !slices.Equal(got.Scopes, tt.wantScopes) {
				t.Errorf("token scopes = %q, want %q", got.Scopes, tt.wantScopes)
			}
			if !slices.Equal(created.Key.Jobs, tt.wantJobs) || !slices.Equal(got.Jobs, tt.wantJobs) {
				t.Errorf("key jobs = %q, token jobs = %q, want %q", created.Key.Jobs, got.Jobs, tt.wantJobs)
			}
		})
	}
}

// TestRouteScopes checks every protected route against the scope it needs,
// by presenting a key that has every other scope a key can hold. Admin
// routes therefore also prove that no API key can reach them.
func TestRouteScopes(t *testing.T) {
	routes := []struct {
		method string
		path   string
		scope  Scope
	}{
		{http.MethodGet, "/ws", ScopeRead},
		{http.MethodGet, "/api/feed", ScopeRead},
		{http.MethodGet, "/api/jobs/", ScopeRead},
		{http.MethodPost, "/api/jobs/create", ScopeWrite},
		{http.MethodPost, "/api/job/myjob/run", ScopeRun},
		{http.MethodDelete, "/api/job/myjob", ScopeWrite},
		{http.MethodPost, "/api/job/myjob", ScopeWrite},
		{http.MethodGet, "/api/job/myjob", ScopeRead},
		{http.MethodPost, "/api/job/myjob/set_active", ScopeWrite},
		{http.MethodGet, "/api/build/1", ScopeRead},
		{http.MethodPost, "/api/build/1/abort", ScopeRun},
		{http.MethodPost, "/api/build/1/flush", ScopeRead},
		{http.MethodPost, "/api/build/1/start", ScopeRun},
		{http.MethodGet, "/api/settings", ScopeAdmin},
		{http.MethodPost, "/api/settings", ScopeAdmin},
		{http.MethodGet, "/storage/build/1/task_0.log", ScopeRead},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			newAuthTestEnv(t)
			others := slices.DeleteFunc(slices.Clone(APIKeyScopes), func(s Scope) bool { return s == route.scope })
			_, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), others, 0, time.Now())

			req := httptest.NewRequest(route.method, route.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), string(route.scope)) {
				t.Errorf("without %q: status = %d body = %q, want 403 naming the scope", route.scope, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestForeignBearerTokensAreIgnored: wakeci ignored Authorization: Bearer
// before API keys existed, and a proxy in front of it may add its own, so a
// bearer token that isn't a wakeci key must not shadow the session cookie
func TestForeignBearerTokensAreIgnored(t *testing.T) {
	newAuthTestEnv(t)
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodPost, "/auth/login", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tests := []struct {
		name       string
		bearer     string
		wantStatus int
	}{
		{name: "proxy token", bearer: "eyJhbGciOiJSUzI1NiJ9.e30.sig", wantStatus: http.StatusOK},
		{name: "malformed wakeci key", bearer: apiKeyPrefix + "nope", wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
			req.Header.Set("Authorization", "Bearer "+tt.bearer)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
		})
	}
}

func TestAPIKeyCredentials(t *testing.T) {
	newAuthTestEnv(t)
	key, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())

	tests := []struct {
		name string
		auth func(req *http.Request)
	}{
		{name: "bearer", auth: func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) }},
		{name: "lowercase bearer", auth: func(req *http.Request) { req.Header.Set("Authorization", "bearer "+token) }},
		{name: "basic auth password", auth: func(req *http.Request) { req.SetBasicAuth("", token) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
			tt.auth(req)
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
			}
			var p Principal
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("decode principal: %v", err)
			}
			if p.KeyID != key.ID || p.Via != viaAPIKey || !slices.Equal(p.Scopes, []Scope{ScopeRead}) {
				t.Errorf("principal = %+v, want read-only key %s", p, key.ID)
			}
		})
	}
}

func TestAPIKeysAreManagedOnlyFromSessions(t *testing.T) {
	newAuthTestEnv(t)
	_, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), APIKeyScopes, 0, time.Now())
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodPost, "/auth/login", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	tests := []struct {
		name       string
		auth       func(req *http.Request)
		wantStatus int
	}{
		{name: "session", auth: func(req *http.Request) { req.AddCookie(cookie) }, wantStatus: http.StatusOK},
		{name: "api key with every scope", auth: func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) }, wantStatus: http.StatusForbidden},
		{name: "shared password basic auth", auth: func(req *http.Request) { req.SetBasicAuth("", "correct") }, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/keys/", nil)
			tt.auth(req)
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
		})
	}
}

func TestCreateAPIKeyRejectsAdminScope(t *testing.T) {
	newAuthTestEnv(t)
	_, _, err := CreateAPIKey(passwordPrincipal(viaSession), "admin key", []Scope{ScopeRead, ScopeAdmin}, nil, 0, time.Now(), nil)
	if !errors.Is(err, errScopeNotForKey) {
		t.Fatalf("CreateAPIKey with admin scope: error = %v, want errScopeNotForKey", err)
	}
	if keys, err := ListAPIKeys(passwordPrincipal(viaSession).ID); err != nil || len(keys) != 0 {
		t.Errorf("rejected key was stored: %v (err %v)", keys, err)
	}
}

// TestKeysCannotOutliveTheirSession: a request is authenticated before its
// body arrives. If the session ends in between, by offboarding or logout, the
// key must not be created: one created after offboarding would come back to
// life when the owner signs in again.
func TestKeysCannotOutliveTheirSession(t *testing.T) {
	tests := []struct {
		name string
		end  func(t *testing.T, cookie *http.Cookie)
	}{
		{name: "owner offboarded", end: func(t *testing.T, cookie *http.Cookie) {
			account, err := getGoogleAccount(googleUser.ID)
			if err != nil || account == nil {
				t.Fatalf("getGoogleAccount: %v %v", account, err)
			}
			if err := offboardGoogleAccount(L, *account, "test"); err != nil {
				t.Fatalf("offboard: %v", err)
			}
		}},
		{name: "logged out", end: func(t *testing.T, cookie *http.Cookie) {
			if err := GlobalSessionStorage.Delete(cookie.Value); err != nil {
				t.Fatalf("log out: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			Config.Google = GoogleConfig{ClientID: "id", ClientSecret: "secret", AllowedDomains: []string{"example.com"}}
			mustPutGoogleAccount(t, time.Now())
			req := httptest.NewRequest(http.MethodPost, "/api/keys/", strings.NewReader(url.Values{"name": {"late"}, "scope": {"read"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			cookie, err := GlobalSessionStorage.New(req, googleUser)
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			req.AddCookie(cookie)

			// AuthMi let the request in; then the session ends
			tt.end(t, cookie)
			rec := httptest.NewRecorder()
			HandleAPIKeysCreate(rec, withPrincipal(req, googleUser))

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d (%s), want 403", rec.Code, rec.Body.String())
			}
			if keys, err := ListAPIKeys(googleUser.ID); err != nil || len(keys) != 0 {
				t.Errorf("keys = %v (%v), want none", keys, err)
			}
		})
	}
}
