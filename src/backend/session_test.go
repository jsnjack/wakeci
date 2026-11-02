package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieSecure(t *testing.T) {
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
			cookie, err := storage.New(req, SessionTTL, "")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if cookie.Secure != tt.wantSecureCookie {
				t.Errorf("cookie.Secure = %t, want %t", cookie.Secure, tt.wantSecureCookie)
			}
		})
	}
}

func TestSessionUser(t *testing.T) {
	Config = &WakeConfig{}
	storage := &SessionStorage{sessions: make(map[string]session)}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)

	for _, user := range []string{"", "alice@example.com"} {
		cookie, err := storage.New(req, time.Hour, user)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if got, err := storage.User(cookie.Value); err != nil || got != user {
			t.Errorf("User() = %q, %v; want %q", got, err, user)
		}
	}

	expired, err := storage.New(req, -time.Second, "bob@example.com")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := storage.User(expired.Value); err == nil {
		t.Error("User() of an expired session: no error")
	}
	if _, err := storage.User("missing"); err == nil {
		t.Error("User() of a missing session: no error")
	}
}

func TestSessionDeleteUser(t *testing.T) {
	Config = &WakeConfig{}
	storage := &SessionStorage{sessions: make(map[string]session)}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	newSession := func(user string) string {
		cookie, err := storage.New(req, time.Hour, user)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		return cookie.Value
	}
	alice1, alice2 := newSession("alice@example.com"), newSession("Alice@Example.com")
	bob, admin := newSession("bob@example.com"), newSession("")

	// The admin password's sessions have no user and are never ended by one
	if ended := storage.DeleteUser(""); ended != 0 || storage.Verify(admin) != nil {
		t.Errorf("DeleteUser(\"\") ended %d sessions, admin still valid: %v", ended, storage.Verify(admin))
	}
	if ended := storage.DeleteUser("ALICE@example.com"); ended != 2 {
		t.Errorf("DeleteUser(alice) ended %d sessions, want 2", ended)
	}
	for name, token := range map[string]string{"alice": alice1, "Alice": alice2} {
		if storage.Verify(token) == nil {
			t.Errorf("%s's session still valid", name)
		}
	}
	for name, token := range map[string]string{"bob": bob, "admin": admin} {
		if err := storage.Verify(token); err != nil {
			t.Errorf("%s's session ended: %v", name, err)
		}
	}
}
