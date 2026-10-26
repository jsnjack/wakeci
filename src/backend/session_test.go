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

			storage := &SessionStorage{sessions: make(map[string]time.Time)}
			cookie, err := storage.New(req, SessionTTL)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if cookie.Secure != tt.wantSecureCookie {
				t.Errorf("cookie.Secure = %t, want %t", cookie.Secure, tt.wantSecureCookie)
			}
		})
	}
}
