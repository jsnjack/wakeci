package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/sasha-s/go-deadlock"
)

// SessionTTL is TTL for a session cookie
const SessionTTL = 24 * 5 * time.Hour

// SSOSessionTTL is TTL for a session created by single sign-on. It is
// shorter than SessionTTL because wakeci never re-checks the account: it
// bounds how long a suspended account, or one removed from the allowed
// groups, keeps access.
const SSOSessionTTL = 12 * time.Hour

// SessionCleanupPeriod is a period to clean up expired sessions
const SessionCleanupPeriod = 1 * time.Hour

// SessionStorage is in-memory storage to keep active sessions
type SessionStorage struct {
	sessions map[string]time.Time
	mu       deadlock.RWMutex
}

func isHTTPS(r *http.Request) bool {
	forwardedProto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return Config.Port == "443" || r.TLS != nil || strings.EqualFold(strings.TrimSpace(forwardedProto), "https")
}

func sessionCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     "session",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isHTTPS(r),
	}
}

// New creates new session which expires after ttl and returns a cookie.
func (s *SessionStorage) New(r *http.Request, ttl time.Duration) (*http.Cookie, error) {
	sessionToken, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionToken.String()] = expires
	c := sessionCookie(r)
	c.Value = sessionToken.String()
	c.Expires = expires
	return c, nil
}

// Verify returns error if cookie is not valid
func (s *SessionStorage) Verify(sessionToken string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.sessions[sessionToken]
	if !ok {
		return fmt.Errorf("session %s doesn't exist", sessionToken)
	}
	if val.Before(time.Now()) {
		return fmt.Errorf("session %s expired", sessionToken)
	}
	return nil
}

// Delete removes session id from storage
func (s *SessionStorage) Delete(sessionToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[sessionToken]
	if !ok {
		return fmt.Errorf("session %s doesn't exist", sessionToken)
	}
	delete(s.sessions, sessionToken)
	return nil
}

// Periodically cleanup expired sessions
func (s *SessionStorage) deleteExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := time.Now()
	for key, exp := range s.sessions {
		if exp.Before(t) {
			delete(s.sessions, key)
		}
	}
}

// Run session cleanup every t
func (s *SessionStorage) startCleanup(d time.Duration) {
	ticker := time.NewTicker(d)
	go func() {
		for range ticker.C {
			s.deleteExpired()
		}
	}()
}

// CreateSessionStorage creates and returns new session storage
func CreateSessionStorage(d time.Duration) *SessionStorage {
	s := &SessionStorage{
		sessions: make(map[string]time.Time),
	}
	s.startCleanup(d)
	return s
}
