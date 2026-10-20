package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/sasha-s/go-deadlock"
)

// SessionTTL is TTL for a session cookie
const SessionTTL = 24 * 5 * time.Hour

// SessionCleanupPeriod is a period to clean up expired sessions
const SessionCleanupPeriod = 1 * time.Hour

// SessionStorage is in-memory storage to keep active sessions
type SessionStorage struct {
	sessions map[string]session
	mu       deadlock.RWMutex
}

type session struct {
	created   time.Time
	expires   time.Time
	principal Principal
}

// isHTTPS reports whether the client reached wakeci over HTTPS, directly or
// through a TLS-terminating reverse proxy
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

// New creates a session for p and returns its cookie.
func (s *SessionStorage) New(r *http.Request, p *Principal) (*http.Cookie, error) {
	sessionToken, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	expires := now.Add(SessionTTL)
	principal := *p
	if principal.Provider == ProviderPassword && principal.passwordFingerprint == "" {
		hash, err := readPasswordHash()
		if err != nil {
			return nil, err
		}
		principal.passwordFingerprint = passwordFingerprint(hash)
	}
	principal.Via = viaSession
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionToken.String()] = session{created: now, expires: expires, principal: principal}
	c := sessionCookie(r)
	c.Value = sessionToken.String()
	c.Expires = expires
	return c, nil
}

// Verify returns the session's principal, or an error if the session is not valid
func (s *SessionStorage) Verify(sessionToken string) (*Principal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.sessions[sessionToken]
	if !ok {
		return nil, errors.New("session does not exist")
	}
	if val.expires.Before(time.Now()) {
		return nil, errors.New("session expired")
	}
	principal := val.principal
	return &principal, nil
}

// Delete removes session id from storage
func (s *SessionStorage) Delete(sessionToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[sessionToken]
	if !ok {
		return errors.New("session does not exist")
	}
	delete(s.sessions, sessionToken)
	return nil
}

// DeletePrincipal ends the sessions of the principal with id that were
// created before createdBefore
func (s *SessionStorage) DeletePrincipal(id string, createdBefore time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for key, val := range s.sessions {
		if val.principal.ID == id && val.created.Before(createdBefore) {
			delete(s.sessions, key)
			deleted++
		}
	}
	return deleted
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

// verifyDigest looks up a session without putting its bearer cookie in a preview URL.
func (s *SessionStorage) verifyDigest(digest string, now time.Time) (*Principal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for token, val := range s.sessions {
		if passwordFingerprint([]byte(token)) == digest && now.Before(val.expires) {
			principal := val.principal
			return &principal, nil
		}
	}
	return nil, errCredentialRevoked
}
