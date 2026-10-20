package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []Scope
		wantErr bool
	}{
		{name: "single", values: []string{"read"}, want: []Scope{ScopeRead}},
		{name: "repeated field", values: []string{"run", "read"}, want: []Scope{ScopeRead, ScopeRun}},
		{name: "comma separated with duplicates", values: []string{"admin, read", "read"}, want: []Scope{ScopeRead, ScopeAdmin}},
		{name: "empty", values: []string{"", " , "}, want: nil},
		{name: "unknown scope", values: []string{"read", "root"}, wantErr: true},
		{name: "case sensitive", values: []string{"READ"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScopes(tt.values)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseScopes(%q) error = %v, wantErr %t", tt.values, err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseScopes(%q) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestRequireScopeAndSession(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	readKey := &Principal{User: "a@example.com", Scopes: []Scope{ScopeRead}, Via: viaAPIKey}
	session := &Principal{User: "a@example.com", Scopes: AllScopes, Via: viaSession}

	tests := []struct {
		name       string
		handler    http.Handler
		principal  *Principal
		wantStatus int
	}{
		{name: "scope held", handler: RequireScope(ScopeRead)(ok), principal: readKey, wantStatus: http.StatusNoContent},
		{name: "scope missing", handler: RequireScope(ScopeRun)(ok), principal: readKey, wantStatus: http.StatusForbidden},
		{name: "no principal", handler: RequireScope(ScopeRead)(ok), wantStatus: http.StatusForbidden},
		{name: "session for session-only route", handler: RequireSession(ok), principal: session, wantStatus: http.StatusNoContent},
		{name: "api key for session-only route", handler: RequireSession(ok), principal: readKey, wantStatus: http.StatusForbidden},
		{name: "basic auth for session-only route", handler: RequireSession(ok), principal: passwordPrincipal(viaBasic), wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
			if tt.principal != nil {
				req = withPrincipal(req, tt.principal)
			}
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestSharedPasswordNextToGoogle: the shared password keeps working next to
// Google sign-in until google.disable_password turns it off. Then nothing
// behind it may work: a password can't be offboarded, so a departed employee
// who knew it would otherwise keep access.
func TestSharedPasswordNextToGoogle(t *testing.T) {
	tests := []struct {
		name   string
		google GoogleConfig
		wantOK bool
	}{
		{name: "google off", wantOK: true},
		{name: "google on", google: GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}, wantOK: true},
		{name: "google on, password disabled", google: GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}, DisablePassword: true}},
		// disable_password alone must not lock everyone out
		{name: "password disabled without google", google: GoogleConfig{DisablePassword: true}, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			_, passwordKey := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
			passwordSession, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodPost, "/auth/login", nil), passwordPrincipal(viaSession))
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			Config.Google = tt.google
			wantOK := tt.wantOK

			requests := []struct {
				name    string
				request func() *http.Request
			}{
				{name: "login form", request: func() *http.Request {
					req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(url.Values{"password": {"correct"}}.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return req
				}},
				{name: "basic auth", request: func() *http.Request {
					req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
					req.SetBasicAuth("", "correct")
					return req
				}},
				{name: "key created with the password", request: func() *http.Request {
					req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
					req.Header.Set("Authorization", "Bearer "+passwordKey)
					return req
				}},
				{name: "password session", request: func() *http.Request {
					req := httptest.NewRequest(http.MethodGet, "/auth/_isLoggedIn", nil)
					req.AddCookie(passwordSession)
					return req
				}},
			}
			for _, rq := range requests {
				rec := httptest.NewRecorder()
				newRouter().ServeHTTP(rec, rq.request())
				ok := rec.Code < 300
				if ok != wantOK {
					t.Errorf("%s: status = %d, want success %t", rq.name, rec.Code, wantOK)
				}
			}

			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/methods", nil))
			if want := fmt.Sprintf(`"password":%t`, wantOK); !strings.Contains(rec.Body.String(), want) {
				t.Errorf("/auth/methods = %s, want %s", rec.Body.String(), want)
			}

			if !wantOK {
				// Nor can a Google user set a password to sign in with later
				google := &Principal{ID: "google:1", User: "dev@example.com", Provider: ProviderGoogle, Scopes: AllScopes, Via: viaSession}
				req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(url.Values{"password": {"new"}}.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				HandleSettingsPost(rec, withPrincipal(req, google))
				if rec.Code != http.StatusBadRequest {
					t.Errorf("setting a password: status = %d, want 400", rec.Code)
				}
			}
		})
	}
}
