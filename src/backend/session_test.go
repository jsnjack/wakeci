package main

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
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

// Ended sessions are reported, so the WebSockets opened with them can close
func TestSessionOnEnd(t *testing.T) {
	Config = &WakeConfig{}
	storage := &SessionStorage{sessions: make(map[string]session)}
	var ended []string
	storage.OnEnd = func(keys []string) { ended = append(ended, keys...) }
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	newSession := func(user string, ttl time.Duration) string {
		cookie, err := storage.New(req, ttl, user)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		return cookie.Value
	}
	loggedOut := newSession("", time.Hour)
	alice := newSession("alice@example.com", time.Hour)
	expired := newSession("bob@example.com", -time.Second)
	carol := newSession("carol@example.com", time.Hour)

	if err := storage.Delete(loggedOut); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	storage.DeleteUser("alice@example.com")
	storage.deleteExpired()
	if want := []string{sessionKey(loggedOut), sessionKey(alice), sessionKey(expired)}; !slices.Equal(ended, want) {
		t.Errorf("ended = %q, want %q", ended, want)
	}
	if users := storage.Users(); !slices.Equal(users, []string{"carol@example.com"}) {
		t.Errorf("Users() = %q, want carol only", users)
	}
	if storage.Verify(carol) != nil {
		t.Error("carol's session ended")
	}
}

func TestNewSSORefusesLoginsFromBeforeRevoke(t *testing.T) {
	Config = &WakeConfig{}
	storage := &SessionStorage{sessions: make(map[string]session)}
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/acs", nil)
	before := time.Now()
	storage.DeleteUser("alice@example.com")

	for _, issuedAt := range []time.Time{before, time.Now(), time.Now().Add(revokedLoginLeeway / 2)} {
		if _, err := storage.NewSSO(req, time.Hour, "Alice@Example.com", issuedAt); !errors.Is(err, errLoginRevoked) {
			t.Errorf("login issued %v: err = %v, want errLoginRevoked", issuedAt, err)
		}
	}
	if _, err := storage.NewSSO(req, time.Hour, "alice@example.com", time.Now().Add(2*revokedLoginLeeway)); err != nil {
		t.Errorf("login issued after the revoke: %v", err)
	}
	if _, err := storage.NewSSO(req, time.Hour, "bob@example.com", before); err != nil {
		t.Errorf("someone else's login: %v", err)
	}
}

// A login checked just as the revoke runs never leaves a session behind:
// either the revoke ends it, or the login sees the cutoff
func TestNewSSORacingRevoke(t *testing.T) {
	Config = &WakeConfig{}
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/acs", nil)
	for i := 0; i < 200; i++ {
		storage := &SessionStorage{sessions: make(map[string]session)}
		issuedAt := time.Now()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := storage.NewSSO(req, time.Hour, "alice@example.com", issuedAt); err != nil && !errors.Is(err, errLoginRevoked) {
				t.Errorf("NewSSO: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			storage.DeleteUser("alice@example.com")
		}()
		wg.Wait()
		if users := storage.Users(); len(users) != 0 {
			t.Fatalf("round %d: sessions left for %q", i, users)
		}
	}
}

// openSessionsDB opens a db with SessionsBucket
func openSessionsDB(t *testing.T) *bolt.DB {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "sessions.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(SessionsBucket)
		return err
	}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	return db
}

func TestSessionsSurviveRestart(t *testing.T) {
	Config = &WakeConfig{}
	db := openSessionsDB(t)
	storage, err := CreateSessionStorage(time.Hour, db)
	if err != nil {
		t.Fatalf("CreateSessionStorage: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	newSession := func(user string, ttl time.Duration) string {
		cookie, err := storage.New(req, ttl, user)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		return cookie.Value
	}
	admin := newSession("", time.Hour)
	alice := newSession("alice@example.com", time.Hour)
	loggedOut := newSession("", time.Hour)
	revoked := newSession("bob@example.com", time.Hour)
	expired := newSession("carol@example.com", time.Millisecond)
	if err := storage.Delete(loggedOut); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	storage.DeleteUser("bob@example.com")
	time.Sleep(5 * time.Millisecond)

	restarted, err := CreateSessionStorage(time.Hour, db)
	if err != nil {
		t.Fatalf("CreateSessionStorage after restart: %v", err)
	}
	tests := []struct {
		name     string
		token    string
		wantUser string
		wantErr  bool
	}{
		{"password session", admin, "", false},
		{"single sign-on session", alice, "alice@example.com", false},
		{"logged out", loggedOut, "", true},
		{"revoked", revoked, "", true},
		{"expired", expired, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := restarted.User(tt.token)
			if (err != nil) != tt.wantErr || user != tt.wantUser {
				t.Errorf("User() = %q, %v; want %q, error %v", user, err, tt.wantUser, tt.wantErr)
			}
		})
	}

	// Only the two live sessions stay saved, and not by their token
	var stored []string
	if err := db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(SessionsBucket).ForEach(func(k, v []byte) error {
			stored = append(stored, string(k))
			for _, token := range []string{admin, alice} {
				if strings.Contains(string(k), token) || strings.Contains(string(v), token) {
					t.Errorf("a token is saved in the db")
				}
			}
			return nil
		})
	}); err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	slices.Sort(stored)
	want := []string{sessionKey(admin), sessionKey(alice)}
	slices.Sort(want)
	if !slices.Equal(stored, want) {
		t.Errorf("saved sessions = %q, want the keys of the live ones %q", stored, want)
	}
}

func TestDeletePasswordSessions(t *testing.T) {
	Config = &WakeConfig{}
	db := openSessionsDB(t)
	storage, err := CreateSessionStorage(time.Hour, db)
	if err != nil {
		t.Fatalf("CreateSessionStorage: %v", err)
	}
	var ended []string
	storage.OnEnd = func(keys []string) { ended = append(ended, keys...) }
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	tokens := map[string]string{}
	for _, name := range []string{"current", "other", "alice"} {
		user := ""
		if name == "alice" {
			user = "alice@example.com"
		}
		cookie, err := storage.New(req, time.Hour, user)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		tokens[name] = cookie.Value
	}

	if n := storage.DeletePasswordSessions(tokens["current"]); n != 1 {
		t.Errorf("ended %d sessions, want 1", n)
	}
	if !slices.Equal(ended, []string{sessionKey(tokens["other"])}) {
		t.Errorf("OnEnd got %q, want the other password session", ended)
	}
	restarted, err := CreateSessionStorage(time.Hour, db)
	if err != nil {
		t.Fatalf("CreateSessionStorage after restart: %v", err)
	}
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"current", false},
		{"other", true},
		{"alice", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for label, storage := range map[string]*SessionStorage{"running": storage, "restarted": restarted} {
				if err := storage.Verify(tokens[tt.name]); (err != nil) != tt.wantErr {
					t.Errorf("%s: Verify() = %v, want error %v", label, err, tt.wantErr)
				}
			}
		})
	}
}
