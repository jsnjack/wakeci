package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Scope is a permission carried by an authenticated request. Browser
// sessions get every scope; API keys get the scopes chosen when they were
// created.
type Scope string

const (
	// ScopeRead allows viewing the feed, jobs, builds, logs and artifacts
	ScopeRead Scope = "read"
	// ScopeRun allows running jobs and starting or aborting builds
	ScopeRun Scope = "run"
	// ScopeWrite allows creating, editing, deleting and (de)activating jobs
	ScopeWrite Scope = "write"
	// ScopeAdmin allows changing server settings, including the password
	ScopeAdmin Scope = "admin"
)

// AllScopes lists every scope in display order
var AllScopes = []Scope{ScopeRead, ScopeRun, ScopeWrite, ScopeAdmin}

// APIKeyScopes are the scopes an API key can be granted. ScopeAdmin is held
// only by browser sessions and the shared password: it can change that
// password, which would turn a leaked key into full control.
var APIKeyScopes = []Scope{ScopeRead, ScopeRun, ScopeWrite}

// ParseScopes validates scope names, accepting repeated and comma-separated
// values, and returns them deduplicated in AllScopes order
func ParseScopes(values []string) ([]Scope, error) {
	requested := map[Scope]bool{}
	for _, value := range values {
		for name := range strings.SplitSeq(value, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if !slices.Contains(AllScopes, Scope(name)) {
				return nil, fmt.Errorf("unknown scope %q", name)
			}
			requested[Scope(name)] = true
		}
	}
	var scopes []Scope
	for _, scope := range AllScopes {
		if requested[scope] {
			scopes = append(scopes, scope)
		}
	}
	return scopes, nil
}

// Identity providers a Principal can come from
const (
	ProviderPassword = "password"
	ProviderGoogle   = "google"
)

// How a request proved its Principal
const (
	viaSession = "session"
	viaBasic   = "basic"
	viaAPIKey  = "apikey"
)

// passwordUser is the user name of everyone who logs in with the shared password
const passwordUser = "admin"

var (
	errPasswordDisabled = errors.New("the shared password is disabled by google.disable_password")
	// errCredentialRevoked: a session ended or the shared password changed
	errCredentialRevoked = errors.New("credential revoked")
)

// passwordLoginEnabled reports whether the shared password works at all. It
// does, next to Google sign-in, unless google.disable_password is set; then
// login, basic auth, password sessions and keys created with the password all
// stop. To recover from a broken Google setup, unset it and restart.
func passwordLoginEnabled() bool {
	return !Config.Google.Enabled() || !Config.Google.DisablePassword
}

// checkSessionPrincipal reports whether a logged-in principal may still act.
// Errors wrapping errGoogleAccountInactive or errPasswordDisabled mean no;
// anything else is a server-side failure.
func checkSessionPrincipal(p *Principal, now time.Time) error {
	switch p.Provider {
	case ProviderGoogle:
		return checkGoogleAccount(p.ID, now)
	case ProviderPassword:
		if !passwordLoginEnabled() {
			return errPasswordDisabled
		}
		if p.Via == viaSession {
			hash, err := readPasswordHash()
			if err != nil {
				return err
			}
			if p.passwordFingerprint == "" || p.passwordFingerprint != passwordFingerprint(hash) {
				return errCredentialRevoked
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown identity provider", errCredentialRevoked)
	}
}

func passwordFingerprint(hash []byte) string {
	sum := sha256.Sum256(hash)
	return hex.EncodeToString(sum[:])
}

func readPasswordHash() ([]byte, error) {
	var hash []byte
	err := DB.View(func(tx *bolt.Tx) error {
		hash = bytes.Clone(tx.Bucket(GlobalBucket).Get([]byte("password")))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read password hash: %w", err)
	}
	return hash, nil
}

// credentialCheck returns a function that re-checks the credential r
// authenticated with, for connections that outlive the request (websockets).
// It never keeps the secret itself: a session token is only a lookup key,
// an API key is re-checked by ID, and basic auth by whether the password
// hash changed since.
func credentialCheck(r *http.Request) (func(now time.Time) error, error) {
	p := principalFrom(r)
	if p == nil {
		return nil, errors.New("request is not authenticated")
	}
	switch p.Via {
	case viaSession:
		cookie, err := r.Cookie("session")
		if err != nil {
			return nil, fmt.Errorf("read session cookie: %w", err)
		}
		token := cookie.Value
		return func(now time.Time) error {
			principal, err := GlobalSessionStorage.Verify(token)
			if err != nil {
				return fmt.Errorf("%w: %w", errCredentialRevoked, err)
			}
			return checkSessionPrincipal(principal, now)
		}, nil
	case viaAPIKey:
		keyID := p.KeyID
		return func(now time.Time) error {
			return checkAPIKeyByID(keyID, now)
		}, nil
	case viaBasic:
		// The hash AuthMi checked the password against, not the current one:
		// the password may have changed since
		snapshot, ok := r.Context().Value(verifiedPasswordHashKey{}).([]byte)
		if !ok {
			return nil, errors.New("basic auth request carries no verified password hash")
		}
		return func(time.Time) error {
			if !passwordLoginEnabled() {
				return errPasswordDisabled
			}
			current, err := readPasswordHash()
			if err != nil {
				return err
			}
			if !bytes.Equal(current, snapshot) {
				return fmt.Errorf("%w: the shared password changed", errCredentialRevoked)
			}
			return nil
		}, nil
	}
	return nil, fmt.Errorf("unknown authentication %q", p.Via)
}

// isRevoked reports whether err from a credential check means the credential
// is no longer good, rather than that the check itself failed
func isRevoked(err error) bool {
	for _, revoked := range []error{errGoogleAccountInactive, errPasswordDisabled, errInvalidAPIKey, errCredentialRevoked} {
		if errors.Is(err, revoked) {
			return true
		}
	}
	return false
}

// Principal is the authenticated identity behind a request
type Principal struct {
	passwordFingerprint string
	// ID is the stable identity that owns API keys: "google:<sub>" or
	// "password:admin". Google's `sub` is used rather than the email, since
	// an email address can be reassigned to a different account.
	ID string `json:"id"`
	// User is passwordUser for the shared password, otherwise a Google email
	User     string `json:"user"`
	Provider string `json:"provider"`
	// Domain is the Google Workspace domain (`hd` claim), if any
	Domain string  `json:"domain,omitempty"`
	Scopes []Scope `json:"scopes"`
	// Jobs limits an API key to the jobs matching one of these patterns
	// (matchJobPattern); empty means every job
	Jobs []string `json:"jobs,omitempty"`
	Via  string   `json:"via"`
	// KeyID is the API key the request authenticated with
	KeyID string `json:"keyID,omitempty"`
}

// LogValue exposes identity metadata without the credential fingerprint.
func (p *Principal) LogValue() slog.Value {
	if p == nil {
		return slog.StringValue("<nil>")
	}
	return slog.GroupValue(
		slog.String("id", p.ID),
		slog.String("provider", p.Provider),
		slog.String("via", p.Via),
		slog.String("keyID", p.KeyID),
	)
}

// Has reports whether the principal holds scope
func (p *Principal) Has(scope Scope) bool {
	return slices.Contains(p.Scopes, scope)
}

func passwordPrincipal(via string) *Principal {
	return &Principal{
		ID:       ProviderPassword + ":" + passwordUser,
		User:     passwordUser,
		Provider: ProviderPassword,
		Scopes:   slices.Clone(AllScopes),
		Via:      via,
	}
}

type principalKey struct{}

// verifiedPasswordHashKey holds, for basic auth requests, the password hash
// AuthMi verified the password against. It's kept out of Principal, which
// gets logged and returned to clients.
type verifiedPasswordHashKey struct{}

func withPrincipal(r *http.Request, p *Principal) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
}

// principalFrom returns the principal AuthMi attached to the request, or nil
func principalFrom(r *http.Request) *Principal {
	p, _ := r.Context().Value(principalKey{}).(*Principal)
	return p
}

// RequireScope rejects requests whose principal lacks scope. It must run
// after AuthMi.
func RequireScope(scope Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger, ok := r.Context().Value(HL).(*slog.Logger)
			if !ok {
				logger = L
			}
			p := principalFrom(r)
			if p == nil || !p.Has(scope) {
				logger.Warn("missing scope", "scope", scope, "principal", p)
				writeText(w, logger, http.StatusForbidden, fmt.Sprintf("Forbidden: requires %q scope", scope))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSession rejects requests that did not authenticate with a browser
// session, so API keys and basic auth can't be used to mint more API keys.
// It must run after AuthMi.
func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*slog.Logger)
		if !ok {
			logger = L
		}
		p := principalFrom(r)
		if p == nil || p.Via != viaSession {
			logger.Warn("session required", "principal", p)
			writeText(w, logger, http.StatusForbidden, "Forbidden: API keys can only be managed from a logged-in browser session")
			return
		}
		next.ServeHTTP(w, r)
	})
}
