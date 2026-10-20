package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieSecure(t *testing.T) {
	newAuthTestEnv(t)
	originalConfig := Config
	t.Cleanup(func() { Config = originalConfig })

	tests := []struct {
		name             string
		port             string
		forwardedProto   string
		directTLS        bool
		wantSecureCookie bool
	}{
		{name: "plain HTTP", port: "8081"},
		{name: "built-in HTTPS", port: "443", wantSecureCookie: true},
		{name: "direct TLS", port: "8081", directTLS: true, wantSecureCookie: true},
		{name: "TLS reverse proxy", port: "8081", forwardedProto: "https", wantSecureCookie: true},
		{name: "case-insensitive proxy header", port: "8081", forwardedProto: "HTTPS", wantSecureCookie: true},
		{name: "first proxy protocol", port: "8081", forwardedProto: "https, http", wantSecureCookie: true},
		{name: "plain reverse proxy", port: "8081", forwardedProto: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Config = &WakeConfig{Port: tt.port}
			req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
			req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			if tt.directTLS {
				req.TLS = &tls.ConnectionState{}
			}

			storage := &SessionStorage{sessions: make(map[string]session)}
			cookie, err := storage.New(req, passwordPrincipal(viaSession))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if cookie.Secure != tt.wantSecureCookie {
				t.Errorf("cookie.Secure = %t, want %t", cookie.Secure, tt.wantSecureCookie)
			}
		})
	}
}

// TestDeletePrincipalKeepsNewerSessions: offboarding ends the sessions that
// existed when it decided to, not ones a later login created
func TestDeletePrincipalKeepsNewerSessions(t *testing.T) {
	newAuthTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	older, err := GlobalSessionStorage.New(req, googleUser)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	cutoff := time.Now()
	time.Sleep(time.Millisecond)
	newer, err := GlobalSessionStorage.New(req, googleUser)
	if err != nil {
		t.Fatal(err)
	}
	if n := GlobalSessionStorage.DeletePrincipal(googleUser.ID, cutoff); n != 1 {
		t.Errorf("deleted %d sessions, want 1", n)
	}
	if _, err := GlobalSessionStorage.Verify(older.Value); err == nil {
		t.Error("the older session survived")
	}
	if _, err := GlobalSessionStorage.Verify(newer.Value); err != nil {
		t.Errorf("the newer session was deleted: %v", err)
	}
}
