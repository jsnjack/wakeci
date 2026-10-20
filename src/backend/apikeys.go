package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

// apiKeyPrefix starts every API key token: wcik_<id><secret>, where the
// body is apiKeyIDLength hex chars of key ID followed by apiKeySecretLength
// hex chars of secret. The prefix makes keys recognisable to secret scanners
// and lets AuthMi accept a key as a basic auth password, for tools that only
// speak basic auth.
const apiKeyPrefix = "wcik_"

// Lengths, in hex chars, of the two parts of a token's body
const (
	apiKeyIDLength     = 16
	apiKeySecretLength = 64
)

// apiKeyLastUsedGranularity bounds how often a key's LastUsedAt is written
// back, so busy keys don't turn every API call into a BoltDB write
const apiKeyLastUsedGranularity = time.Minute

const (
	maxAPIKeyNameLength = 100
	maxAPIKeyTTLDays    = 3650
)

var (
	errInvalidAPIKey  = errors.New("invalid api key")
	errAPIKeyNotFound = errors.New("api key not found")
	errScopeNotForKey = errors.New("scope can't be granted to an api key")
	// Jobs run commands on the server, so a key that can edit one can reach
	// every job anyway; limiting it would only pretend otherwise
	errWriteKeyLimited = errors.New("a key with the write scope can't be limited to jobs")
)

// APIKey is a scoped, revocable credential for API calls. It belongs to the
// user who created it and never carries more scopes than they had.
type APIKey struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// OwnerID is the creator's Principal.ID; Owner is their email, for display
	OwnerID       string  `json:"ownerID"`
	Owner         string  `json:"owner"`
	OwnerProvider string  `json:"ownerProvider"`
	Scopes        []Scope `json:"scopes"`
	// Jobs limits the key to the jobs matching one of these patterns
	// (matchJobPattern); empty means every job, including future ones
	Jobs []string `json:"jobs,omitempty"`
	// SecretHash is the SHA-256 of the token's secret part. The secret is 256
	// random bits, so a slow password hash would add per-request cost without
	// adding resistance to guessing.
	SecretHash []byte     `json:"secretHash"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// APIKeyData is an API key as the API returns it, without its secret
type APIKeyData struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Owner      string     `json:"owner"`
	Scopes     []Scope    `json:"scopes"`
	Jobs       []string   `json:"jobs"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// CreatedAPIKeyData is returned once, when a key is created. Token is the
// only copy of the key's secret; wakeci stores just its hash.
type CreatedAPIKeyData struct {
	Key   APIKeyData `json:"key"`
	Token string     `json:"token"`
}

func (k *APIKey) data() APIKeyData {
	return APIKeyData{
		ID:         k.ID,
		Name:       k.Name,
		Owner:      k.Owner,
		Scopes:     k.Scopes,
		Jobs:       k.Jobs,
		CreatedAt:  k.CreatedAt,
		ExpiresAt:  k.ExpiresAt,
		LastUsedAt: k.LastUsedAt,
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func hashAPIKeySecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// CreateAPIKey stores a new key owned by owner and returns it along with its
// token. No jobs means every job; a zero ttl means the key never expires.
// stillAllowed, if set, runs in the transaction that stores the key, to
// re-check the owner's access: offboarding revokes in a transaction too, so
// a key can't slip in after it.
func CreateAPIKey(owner *Principal, name string, scopes []Scope, jobs []string, ttl time.Duration, now time.Time, stillAllowed func(tx *bolt.Tx) error) (*APIKey, string, error) {
	for _, scope := range scopes {
		if !slices.Contains(APIKeyScopes, scope) {
			return nil, "", fmt.Errorf("%w: %q", errScopeNotForKey, scope)
		}
	}
	if len(jobs) > 0 && slices.Contains(scopes, ScopeWrite) {
		return nil, "", errWriteKeyLimited
	}
	id, err := randomHex(apiKeyIDLength / 2)
	if err != nil {
		return nil, "", fmt.Errorf("generate api key id: %w", err)
	}
	secret, err := randomHex(apiKeySecretLength / 2)
	if err != nil {
		return nil, "", fmt.Errorf("generate api key secret: %w", err)
	}
	key := &APIKey{
		ID:            id,
		Name:          name,
		OwnerID:       owner.ID,
		Owner:         owner.User,
		OwnerProvider: owner.Provider,
		Scopes:        scopes,
		Jobs:          jobs,
		SecretHash:    hashAPIKeySecret(secret),
		CreatedAt:     now,
	}
	if ttl > 0 {
		expires := now.Add(ttl)
		key.ExpiresAt = &expires
	}
	data, err := json.Marshal(key)
	if err != nil {
		return nil, "", fmt.Errorf("marshal api key: %w", err)
	}
	err = DB.Update(func(tx *bolt.Tx) error {
		if stillAllowed != nil {
			if err := stillAllowed(tx); err != nil {
				return err
			}
		}
		b := tx.Bucket(APIKeysBucket)
		if b.Get([]byte(id)) != nil {
			return fmt.Errorf("api key id %s already exists", id)
		}
		return b.Put([]byte(id), data)
	})
	if err != nil {
		return nil, "", fmt.Errorf("store api key: %w", err)
	}
	return key, apiKeyPrefix + id + secret, nil
}

// deleteOwnerKeys deletes every key of the principal with ownerID, in tx
func deleteOwnerKeys(tx *bolt.Tx, ownerID string) (int, error) {
	keys := tx.Bucket(APIKeysBucket)
	var keyIDs [][]byte
	err := keys.ForEach(func(k, v []byte) error {
		var key APIKey
		if err := json.Unmarshal(v, &key); err != nil {
			return fmt.Errorf("unmarshal api key: %w", err)
		}
		if key.OwnerID == ownerID {
			keyIDs = append(keyIDs, k)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	// Deleting inside ForEach is unsafe in bolt, hence the second pass
	for _, k := range keyIDs {
		if err := keys.Delete(k); err != nil {
			return 0, err
		}
	}
	return len(keyIDs), nil
}

// ListAPIKeys returns the keys of the principal with ownerID, newest first
func ListAPIKeys(ownerID string) ([]APIKey, error) {
	keys := []APIKey{}
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(APIKeysBucket).ForEach(func(_, v []byte) error {
			var key APIKey
			if err := json.Unmarshal(v, &key); err != nil {
				return fmt.Errorf("unmarshal api key: %w", err)
			}
			if key.OwnerID == ownerID {
				keys = append(keys, key)
			}
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	slices.SortFunc(keys, func(a, b APIKey) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return keys, nil
}

// RevokeAPIKey deletes key id of the principal with ownerID. Keys of other
// users are reported as not found rather than forbidden, so their IDs can't
// be probed.
func RevokeAPIKey(ownerID string, id string) error {
	return DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		v := b.Get([]byte(id))
		if v == nil {
			return errAPIKeyNotFound
		}
		var key APIKey
		if err := json.Unmarshal(v, &key); err != nil {
			return fmt.Errorf("unmarshal api key: %w", err)
		}
		if key.OwnerID != ownerID {
			return errAPIKeyNotFound
		}
		return b.Delete([]byte(id))
	})
}

// VerifyAPIKey returns the principal a token authenticates as. Errors
// wrapping errInvalidAPIKey mean the token was rejected; anything else is a
// server-side failure.
func VerifyAPIKey(logger *slog.Logger, token string, now time.Time) (*Principal, error) {
	rest, ok := strings.CutPrefix(token, apiKeyPrefix)
	if !ok {
		return nil, fmt.Errorf("%w: missing %q prefix", errInvalidAPIKey, apiKeyPrefix)
	}
	if len(rest) != apiKeyIDLength+apiKeySecretLength {
		return nil, fmt.Errorf("%w: malformed token", errInvalidAPIKey)
	}
	id, secret := rest[:apiKeyIDLength], rest[apiKeyIDLength:]

	var key APIKey
	found := false
	err := DB.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(APIKeysBucket).Get([]byte(id))
		if v == nil {
			return nil
		}
		found = true
		return json.Unmarshal(v, &key)
	})
	if err != nil {
		return nil, fmt.Errorf("read api key %s: %w", id, err)
	}
	if !found {
		return nil, fmt.Errorf("%w: unknown key %s", errInvalidAPIKey, id)
	}
	if subtle.ConstantTimeCompare(hashAPIKeySecret(secret), key.SecretHash) != 1 {
		return nil, fmt.Errorf("%w: wrong secret for key %s", errInvalidAPIKey, id)
	}
	if err := checkAPIKey(&key, now); err != nil {
		return nil, err
	}

	if key.LastUsedAt == nil || now.Sub(*key.LastUsedAt) >= apiKeyLastUsedGranularity {
		if err := touchAPIKey(id, now); err != nil {
			// Bookkeeping only; not a reason to fail an authenticated call
			logger.Warn("update api key last used", "key", id, "err", err)
		}
	}

	return &Principal{
		ID:       key.OwnerID,
		User:     key.Owner,
		Provider: key.OwnerProvider,
		Scopes:   key.Scopes,
		Jobs:     key.Jobs,
		Via:      viaAPIKey,
		KeyID:    key.ID,
	}, nil
}

// checkAPIKey reports whether an authentic key may still be used: it must
// not have expired, and its owner must still be allowed in
func checkAPIKey(key *APIKey, now time.Time) error {
	if len(key.Jobs) > 0 && slices.Contains(key.Scopes, ScopeWrite) {
		return fmt.Errorf("%w: legacy write key has job restrictions; recreate it", errInvalidAPIKey)
	}
	if key.ExpiresAt != nil && !now.Before(*key.ExpiresAt) {
		return fmt.Errorf("%w: key %s expired at %s", errInvalidAPIKey, key.ID, key.ExpiresAt)
	}
	// A key never outlives its owner's access
	err := checkSessionPrincipal(&Principal{ID: key.OwnerID, Provider: key.OwnerProvider}, now)
	if isRevoked(err) {
		return fmt.Errorf("%w: owner of key %s: %w", errInvalidAPIKey, key.ID, err)
	}
	return err
}

// checkAPIKeyByID re-checks a key that already authenticated, e.g. one
// holding a websocket open; a revoked key is reported as invalid
func checkAPIKeyByID(id string, now time.Time) error {
	var key APIKey
	found := false
	err := DB.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(APIKeysBucket).Get([]byte(id))
		if v == nil {
			return nil
		}
		found = true
		return json.Unmarshal(v, &key)
	})
	if err != nil {
		return fmt.Errorf("read api key %s: %w", id, err)
	}
	if !found {
		return fmt.Errorf("%w: key %s was revoked", errInvalidAPIKey, id)
	}
	return checkAPIKey(&key, now)
}

func touchAPIKey(id string, now time.Time) error {
	return DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		v := b.Get([]byte(id))
		if v == nil {
			// Revoked since it was verified
			return nil
		}
		var key APIKey
		if err := json.Unmarshal(v, &key); err != nil {
			return fmt.Errorf("unmarshal api key: %w", err)
		}
		key.LastUsedAt = &now
		data, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("marshal api key: %w", err)
		}
		return b.Put([]byte(id), data)
	})
}

// apiKeyFromRequest extracts an API key token from an `Authorization:
// Bearer` header, or from the basic auth password, when it looks like a key.
// Other bearer tokens are ignored, as they were before API keys existed:
// a proxy in front of wakeci may add its own.
func apiKeyFromRequest(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if token = strings.TrimSpace(token); ok && strings.EqualFold(scheme, "Bearer") && strings.HasPrefix(token, apiKeyPrefix) {
		return token, true
	}
	if _, password, ok := r.BasicAuth(); ok && strings.HasPrefix(password, apiKeyPrefix) {
		return password, true
	}
	return "", false
}

// HandleAPIKeysList lists the API keys of the logged-in user
// @Summary      List your API keys
// @Description  Only available to logged-in browser sessions, not to API keys or basic auth
// @Tags         keys
// @Produce      json
// @Success      200      {array}    APIKeyData
// @Failure      403      {string}   string
// @Failure      500      {string}   string
// @Router       /keys/ [get]
func HandleAPIKeysList(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	keys, err := ListAPIKeys(principalFrom(r).ID)
	if err != nil {
		logger.Error("list api keys", "err", err)
		writeText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]APIKeyData, 0, len(keys))
	for _, key := range keys {
		data = append(data, key.data())
	}
	writeJSON(w, logger, data)
}

// HandleAPIKeysCreate creates an API key for the logged-in user
// @Summary      Create an API key
// @Description  Only available to logged-in browser sessions. The token is returned once and can't be retrieved later. Use it as `Authorization: Bearer <token>`, or as the basic auth password.
// @Tags         keys
// @Produce      json
// @Param        name           formData   string    true   "Name of the key"
// @Param        scope          formData   []string  true   "Scopes: read, run, write. Repeat the field or comma-separate" collectionFormat(multi)
// @Param        job            formData   []string  false  "Limit the key to these jobs. Each is a job name or a pattern in which * matches any characters, e.g. test_*; repeat the field for more. Omit for every job, including future ones" collectionFormat(multi)
// @Param        expiresInDays  formData   integer   false  "Days until the key expires. 0 or empty: never"
// @Success      200      {object}   CreatedAPIKeyData
// @Failure      400      {string}   string
// @Failure      403      {string}   string
// @Failure      500      {string}   string
// @Router       /keys/ [post]
func HandleAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	principal := principalFrom(r)

	// FormValue parses the body, populating r.Form for the repeated scope field
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len(name) > maxAPIKeyNameLength {
		writeText(w, logger, http.StatusBadRequest, fmt.Sprintf("Name must be 1-%d characters", maxAPIKeyNameLength))
		return
	}

	scopes, err := ParseScopes(r.Form["scope"])
	if err != nil {
		writeText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	if len(scopes) == 0 {
		writeText(w, logger, http.StatusBadRequest, "Select at least one scope")
		return
	}
	for _, scope := range scopes {
		if !slices.Contains(APIKeyScopes, scope) {
			writeText(w, logger, http.StatusBadRequest, fmt.Sprintf("The %q scope can't be granted to API keys", scope))
			return
		}
		if !principal.Has(scope) {
			writeText(w, logger, http.StatusForbidden, fmt.Sprintf("You don't have the %q scope yourself", scope))
			return
		}
	}

	jobs, err := ParseJobPatterns(r.Form["job"])
	if err != nil {
		writeText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	if len(jobs) > 0 && slices.Contains(scopes, ScopeWrite) {
		writeText(w, logger, http.StatusBadRequest, "A key with the write scope can't be limited to jobs: jobs run commands on the server, so it could reach every job anyway")
		return
	}

	var ttl time.Duration
	if days := r.FormValue("expiresInDays"); days != "" {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 || n > maxAPIKeyTTLDays {
			writeText(w, logger, http.StatusBadRequest, fmt.Sprintf("expiresInDays must be 0-%d", maxAPIKeyTTLDays))
			return
		}
		ttl = time.Duration(n) * 24 * time.Hour
	}

	// Revocation can finish after AuthMi runs. Re-check in the transaction
	// that stores the key. RequireSession guarantees the cookie.
	cookie, err := r.Cookie("session")
	if err != nil {
		writeText(w, logger, http.StatusForbidden, "Forbidden")
		return
	}
	stillAllowed := func(tx *bolt.Tx) error {
		live, err := GlobalSessionStorage.Verify(cookie.Value)
		if err != nil {
			return fmt.Errorf("%w: %w", errCredentialRevoked, err)
		}
		if live.Provider == ProviderPassword && (!passwordLoginEnabled() || live.passwordFingerprint != passwordFingerprint(tx.Bucket(GlobalBucket).Get([]byte("password")))) {
			return errCredentialRevoked
		}
		if principal.Provider == ProviderGoogle && tx.Bucket(GoogleAccountsBucket).Get([]byte(principal.ID)) == nil {
			return fmt.Errorf("%w: %s was offboarded", errGoogleAccountInactive, principal.ID)
		}
		return nil
	}
	key, token, err := CreateAPIKey(principal, name, scopes, jobs, ttl, time.Now(), stillAllowed)
	if isRevoked(err) {
		logger.Warn("api key not created", "err", err)
		writeText(w, logger, http.StatusForbidden, "Your session has ended, sign in again")
		return
	}
	if err != nil {
		logger.Error("create api key", "err", err)
		writeText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("api key created", "key", key.ID, "owner", key.Owner, "scopes", key.Scopes, "jobs", key.Jobs)
	writeJSON(w, logger, CreatedAPIKeyData{Key: key.data(), Token: token})
}

// HandleAPIKeysRevoke revokes one of the logged-in user's API keys
// @Summary      Revoke an API key
// @Description  Only available to logged-in browser sessions
// @Tags         keys
// @Produce      plain
// @Param        id       path    string   true  "ID of the key"
// @Success      200      {string}   string
// @Failure      403      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /keys/{id} [delete]
func HandleAPIKeysRevoke(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	id := chi.URLParam(r, "id")
	err := RevokeAPIKey(principalFrom(r).ID, id)
	if errors.Is(err, errAPIKeyNotFound) {
		writeText(w, logger, http.StatusNotFound, "API key not found")
		return
	}
	if err != nil {
		logger.Error("revoke api key", "key", id, "err", err)
		writeText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	WSHub.Recheck()
	logger.Info("api key revoked", "key", id)
}
