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

// session is an active login
type session struct {
	expires time.Time
	// user is the email of a single sign-on login, empty for the password
	user string
}

// SessionStorage is in-memory storage to keep active sessions
type SessionStorage struct {
	sessions map[string]session
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

// New creates new session for user (empty for the password) which expires
// after ttl and returns a cookie.
func (s *SessionStorage) New(r *http.Request, ttl time.Duration, user string) (*http.Cookie, error) {
	sessionToken, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionToken.String()] = session{expires: expires, user: user}
	c := sessionCookie(r)
	c.Value = sessionToken.String()
	c.Expires = expires
	return c, nil
}

// Verify returns error if cookie is not valid
func (s *SessionStorage) Verify(sessionToken string) error {
	_, err := s.User(sessionToken)
	return err
}

// User returns who logged in with a valid session: the single sign-on email,
// or empty for the password
func (s *SessionStorage) User(sessionToken string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.sessions[sessionToken]
	if !ok {
		return "", fmt.Errorf("session %s doesn't exist", sessionToken)
	}
	if val.expires.Before(time.Now()) {
		return "", fmt.Errorf("session %s expired", sessionToken)
	}
	return val.user, nil
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

// DeleteUser ends every session of a single sign-on user and returns how many
func (s *SessionStorage) DeleteUser(user string) int {
	if user == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ended := 0
	for key, val := range s.sessions {
		if strings.EqualFold(val.user, user) {
			delete(s.sessions, key)
			ended++
		}
	}
	return ended
}

// Periodically cleanup expired sessions
func (s *SessionStorage) deleteExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := time.Now()
	for key, val := range s.sessions {
		if val.expires.Before(t) {
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
		sessions: make(map[string]session),
	}
	s.startCleanup(d)
	return s
}
