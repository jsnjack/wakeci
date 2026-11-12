package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/sasha-s/go-deadlock"
	bolt "go.etcd.io/bbolt"
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
	// groups are the groups the identity provider sent at login
	groups []string
}

// storedSession is a session as SessionsBucket keeps it
type storedSession struct {
	Expires time.Time `json:"expires"`
	User    string    `json:"user"`
	Groups  []string  `json:"groups,omitempty"`
}

// sessionKey identifies the session with token in SessionStorage and
// SessionsBucket. It is a hash, so the stored sessions can't be used to log in
func sessionKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SessionStorage keeps the active sessions in memory and, with a db, in
// SessionsBucket, so they survive a restart
type SessionStorage struct {
	// sessions by the sessionKey of their token
	sessions map[string]session
	mu       deadlock.RWMutex
	// db keeps the sessions across restarts; nil keeps them in memory only
	db *bolt.DB
	// saveMu is held from a change of sessions until it is saved, so changes
	// reach db in the order they happen: a session deleted by a revoke
	// can't be saved again after the deletion and come back after a restart
	saveMu deadlock.Mutex
	// unsaved holds the keys of ended sessions whose deletion failed to
	// save; every later save deletes them too, so they don't come back after
	// a restart. Guarded by saveMu
	unsaved []string
	// OnEnd, if set, gets the keys of sessions that were deleted or
	// expired, so connections opened with them can be closed. It is called
	// without the lock held.
	OnEnd func(keys []string)
	// revoked holds when each single sign-on user's access was revoked, by
	// lowercased email, for as long as a login issued before could arrive
	revoked map[string]time.Time
}

func (s *SessionStorage) ended(keys []string) {
	if s.OnEnd != nil && len(keys) > 0 {
		s.OnEnd(keys)
	}
}

// save writes saved and deletes deleted in db, with the deletions an earlier
// save failed to write. The caller holds saveMu
func (s *SessionStorage) save(saved map[string]session, deleted []string) {
	if s.db == nil {
		return
	}
	deleted = append(slices.Clone(deleted), s.unsaved...)
	if len(saved) == 0 && len(deleted) == 0 {
		return
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(SessionsBucket)
		for key, val := range saved {
			data, err := json.Marshal(storedSession{Expires: val.expires, User: val.user, Groups: val.groups})
			if err != nil {
				return err
			}
			if err := b.Put([]byte(key), data); err != nil {
				return err
			}
		}
		for _, key := range deleted {
			if err := b.Delete([]byte(key)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.unsaved = deleted
		L.Error("save sessions; the deletions are tried again with the next save",
			"saved", len(saved), "deleted", len(deleted), "err", err)
		return
	}
	s.unsaved = nil
}

// saveUnsaved tries again to save the deletions that failed. wakeci calls it
// as it stops, so they don't wait for a save after the restart, which loads
// the sessions again first
func (s *SessionStorage) saveUnsaved() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.save(nil, nil)
}

// load reads the sessions saved in db and deletes the expired ones
func (s *SessionStorage) load() error {
	now := time.Now()
	var expired []string
	withoutGroups := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(SessionsBucket).ForEach(func(k, v []byte) error {
			var stored storedSession
			if err := json.Unmarshal(v, &stored); err != nil {
				L.Error("decode saved session", "err", err)
				expired = append(expired, string(k))
				return nil
			}
			if stored.Expires.Before(now) {
				expired = append(expired, string(k))
				return nil
			}
			// Saved before sessions kept the groups: what it may do is
			// unknown, so its user logs in again
			if stored.User != "" && len(stored.Groups) == 0 {
				withoutGroups++
				expired = append(expired, string(k))
				return nil
			}
			s.sessions[string(k)] = session{expires: stored.Expires, user: stored.User, groups: stored.Groups}
			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("load sessions: %w", err)
	}
	if withoutGroups > 0 {
		L.Info("ended single sign-on sessions saved without their groups; their users log in again", "sessions", withoutGroups)
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.save(nil, expired)
	return nil
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
	return s.create(r, ttl, user, nil, nil)
}

// NewSSO creates a session for a single sign-on login of a member of groups
// that the identity provider issued at issuedAt, unless the person's access
// was revoked after that: a login obtained before an offboarding mustn't
// undo it.
func (s *SessionStorage) NewSSO(r *http.Request, ttl time.Duration, user string, groups []string, issuedAt time.Time) (*http.Cookie, error) {
	return s.create(r, ttl, user, groups, &issuedAt)
}

func (s *SessionStorage) create(r *http.Request, ttl time.Duration, user string, groups []string, issuedAt *time.Time) (*http.Cookie, error) {
	sessionToken, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(ttl)
	key := sessionKey(sessionToken.String())
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	// Under the lock DeleteUser takes, so a login can't slip in between a
	// revoke's cutoff and its deletion of the sessions
	if revokedAt, ok := s.revoked[strings.ToLower(user)]; ok && issuedAt != nil &&
		!issuedAt.After(revokedAt.Add(revokedLoginLeeway)) {
		s.mu.Unlock()
		return nil, errLoginRevoked
	}
	created := session{expires: expires, user: user, groups: groups}
	s.sessions[key] = created
	s.mu.Unlock()
	s.save(map[string]session{key: created}, nil)
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
	user, _, err := s.Lookup(sessionToken)
	return user, err
}

// Lookup returns who logged in with a valid session and, for single sign-on,
// the groups the identity provider sent at login
func (s *SessionStorage) Lookup(sessionToken string) (string, []string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.sessions[sessionKey(sessionToken)]
	if !ok {
		// Never put the token in the error: it is logged
		return "", nil, errors.New("session doesn't exist")
	}
	if val.expires.Before(time.Now()) {
		return "", nil, errors.New("session expired")
	}
	return val.user, val.groups, nil
}

// Delete removes session id from storage
func (s *SessionStorage) Delete(sessionToken string) error {
	key := sessionKey(sessionToken)
	s.saveMu.Lock()
	s.mu.Lock()
	_, ok := s.sessions[key]
	delete(s.sessions, key)
	s.mu.Unlock()
	if ok {
		s.save(nil, []string{key})
	}
	s.saveMu.Unlock()
	if !ok {
		return errors.New("session doesn't exist")
	}
	s.ended([]string{key})
	return nil
}

// DeletePasswordSessions ends every session logged in with the password,
// except the one with token keep, and returns how many. Someone who knew a
// changed password may still be logged in with it.
func (s *SessionStorage) DeletePasswordSessions(keep string) int {
	keepKey := sessionKey(keep)
	var ended []string
	s.saveMu.Lock()
	s.mu.Lock()
	for key, val := range s.sessions {
		if val.user == "" && key != keepKey {
			delete(s.sessions, key)
			ended = append(ended, key)
		}
	}
	s.mu.Unlock()
	s.save(nil, ended)
	s.saveMu.Unlock()
	s.ended(ended)
	return len(ended)
}

// DeleteUser ends every session of a single sign-on user and returns how
// many. Logins the identity provider issued before stay refused (NewSSO).
func (s *SessionStorage) DeleteUser(user string) int {
	if user == "" {
		return 0
	}
	var ended []string
	s.saveMu.Lock()
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
	s.save(nil, ended)
	s.saveMu.Unlock()
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

// GroupMembers returns, by lowercased group name, how many people with a
// valid single sign-on session are in each group
func (s *SessionStorage) GroupMembers() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	people := map[string]map[string]bool{}
	for _, val := range s.sessions {
		if val.user == "" || val.expires.Before(now) {
			continue
		}
		for _, group := range val.groups {
			group = strings.ToLower(group)
			if people[group] == nil {
				people[group] = map[string]bool{}
			}
			people[group][strings.ToLower(val.user)] = true
		}
	}
	counts := make(map[string]int, len(people))
	for group, members := range people {
		counts[group] = len(members)
	}
	return counts
}

// CountSSOPeople counts the people with a current single sign-on session
// whose groups match
func (s *SessionStorage) CountSSOPeople(match func(groups []string) bool) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	people := map[string]bool{}
	for _, val := range s.sessions {
		if val.user != "" && !val.expires.Before(now) && match(val.groups) {
			people[strings.ToLower(val.user)] = true
		}
	}
	return len(people)
}

// Periodically cleanup expired sessions
func (s *SessionStorage) deleteExpired() {
	var ended []string
	s.saveMu.Lock()
	s.mu.Lock()
	t := time.Now()
	for key, val := range s.sessions {
		if val.expires.Before(t) {
			delete(s.sessions, key)
			ended = append(ended, key)
		}
	}
	s.mu.Unlock()
	s.save(nil, ended)
	s.saveMu.Unlock()
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

// CreateSessionStorage creates session storage that keeps the sessions in
// db across restarts, or in memory only if db is nil, and deletes expired
// sessions every d
func CreateSessionStorage(d time.Duration, db *bolt.DB) (*SessionStorage, error) {
	s := &SessionStorage{
		sessions: make(map[string]session),
		db:       db,
	}
	if db != nil {
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	s.startCleanup(d)
	return s, nil
}
