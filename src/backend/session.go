package main

import (
	"errors"
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

// revokedLoginLeeway extends the cutoff of a revoke, so a single sign-on
// login issued just before it isn't let in by a clock that is slightly off
const revokedLoginLeeway = time.Minute

// errLoginRevoked means a person's access was revoked after the identity
// provider issued their login
var errLoginRevoked = errors.New("login issued before the person's access was revoked")

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
	// OnEnd, if set, gets the tokens of sessions that were deleted or
	// expired, so connections opened with them can be closed. It is called
	// without the lock held.
	OnEnd func(tokens []string)
	// revoked holds when each single sign-on user's access was revoked, by
	// lowercased email, for as long as a login issued before could arrive
	revoked map[string]time.Time
}

func (s *SessionStorage) ended(tokens []string) {
	if s.OnEnd != nil && len(tokens) > 0 {
		s.OnEnd(tokens)
	}
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
	return s.create(r, ttl, user, nil)
}

// NewSSO creates a session for a single sign-on login that the identity
// provider issued at issuedAt, unless the person's access was revoked after
// that: a login obtained before an offboarding mustn't undo it.
func (s *SessionStorage) NewSSO(r *http.Request, ttl time.Duration, user string, issuedAt time.Time) (*http.Cookie, error) {
	return s.create(r, ttl, user, &issuedAt)
}

func (s *SessionStorage) create(r *http.Request, ttl time.Duration, user string, issuedAt *time.Time) (*http.Cookie, error) {
	sessionToken, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Under the lock DeleteUser takes, so a login can't slip in between a
	// revoke's cutoff and its deletion of the sessions
	if revokedAt, ok := s.revoked[strings.ToLower(user)]; ok && issuedAt != nil &&
		!issuedAt.After(revokedAt.Add(revokedLoginLeeway)) {
		return nil, errLoginRevoked
	}
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
	_, ok := s.sessions[sessionToken]
	delete(s.sessions, sessionToken)
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s doesn't exist", sessionToken)
	}
	s.ended([]string{sessionToken})
	return nil
}

// DeleteUser ends every session of a single sign-on user and returns how
// many. Logins the identity provider issued before stay refused (NewSSO).
func (s *SessionStorage) DeleteUser(user string) int {
	if user == "" {
		return 0
	}
	var ended []string
	s.mu.Lock()
	now := time.Now()
	if s.revoked == nil {
		s.revoked = make(map[string]time.Time)
	}
	// A login issued before a revoke is useless once its login state expired
	for key, revokedAt := range s.revoked {
		if now.Sub(revokedAt) > ssoStateTTL+revokedLoginLeeway {
			delete(s.revoked, key)
		}
	}
	s.revoked[strings.ToLower(user)] = now
	for key, val := range s.sessions {
		if strings.EqualFold(val.user, user) {
			delete(s.sessions, key)
			ended = append(ended, key)
		}
	}
	s.mu.Unlock()
	s.ended(ended)
	return len(ended)
}

// Users returns the single sign-on users with a valid session
func (s *SessionStorage) Users() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var users []string
	for _, val := range s.sessions {
		if val.user != "" && !val.expires.Before(now) {
			users = append(users, val.user)
		}
	}
	return users
}

// Periodically cleanup expired sessions
func (s *SessionStorage) deleteExpired() {
	var ended []string
	s.mu.Lock()
	t := time.Now()
	for key, val := range s.sessions {
		if val.expires.Before(t) {
			delete(s.sessions, key)
			ended = append(ended, key)
		}
	}
	s.mu.Unlock()
	s.ended(ended)
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
