package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/sasha-s/go-deadlock"
	bolt "go.etcd.io/bbolt"
)

// API key scopes
const (
	ScopeRead  = "read"
	ScopeRun   = "run"
	ScopeWrite = "write"
)

// apiKeyScopes are all scopes, in the order they are shown
var apiKeyScopes = []string{ScopeRead, ScopeRun, ScopeWrite}

const (
	// apiKeyPrefix starts every token, so a leaked one is easy to search for
	apiKeyPrefix = "wk_"
	// A token is wk_<id>_<secret>: the ID is hex, the secret base64url
	apiKeyIDBytes     = 6
	apiKeySecretBytes = 32

	maxAPIKeyNameLength = 100
	maxAPIKeyExpiryDays = 3650

	// apiKeyTouchInterval is how stale a key's last use may get before a
	// request records it, so a busy script doesn't write to the database on
	// every request
	apiKeyTouchInterval = time.Minute
)

// apiKeyAttemptLimiter counts failed API key attempts apart from password
// attempts, so neither can clear or use up the other's counter
var apiKeyAttemptLimiter = newAuthAttemptLimiter(
	authMaxFailures,
	authFailureWindow,
	authBlockDuration,
)

// apiKeyOwnersMu makes creating a key and revoking a person's keys take
// turns, so a key their session creates meanwhile can't outlive the revoke
var apiKeyOwnersMu deadlock.Mutex

type contextKey string

const apiKeyContextKey contextKey = "apiKey"

// APIKey is what wakeci keeps about an API key, apart from its secret
type APIKey struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Grant is what the key may do; its fields sit next to the others in JSON
	Grant
	// CreatedBy is the single sign-on email of whoever created the key, empty
	// if they used the password. It lets offboarding revoke a person's keys.
	CreatedBy string `json:"createdBy"`
	// CreatorGroups are the creator's single sign-on groups as of their last
	// login; the key can't do more than these groups allow. Nil for a key made
	// with the password or before wakeci recorded them.
	CreatorGroups []string   `json:"creatorGroups"`
	CreatedAt     time.Time  `json:"createdAt"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt    *time.Time `json:"lastUsedAt,omitempty"`
}

// storedAPIKey is an API key as stored in APIKeysBucket. Only a hash of the
// secret is kept: the secret is 256 random bits, so a fast hash is enough.
type storedAPIKey struct {
	APIKey
	SecretHash []byte `json:"secretHash"`
}

// NewAPIKeyData is a new API key with its token, which is only shown once
type NewAPIKeyData struct {
	Key   *APIKey `json:"key"`
	Token string  `json:"token"`
}

func (k *APIKey) expired(now time.Time) bool {
	return k.ExpiresAt != nil && !now.Before(*k.ExpiresAt)
}

// apiKeyFromContext returns the API key a request was authorized with, or nil
func apiKeyFromContext(ctx context.Context) *APIKey {
	key, _ := ctx.Value(apiKeyContextKey).(*APIKey)
	return key
}

// apiKeyToken returns the API key a request carries as a Bearer token. Other
// Bearer tokens, e.g. from a proxy in front of wakeci, are left alone.
func apiKeyToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(token, apiKeyPrefix) {
		return "", false
	}
	return token, true
}

// parseAPIKeyToken splits a token into its key ID and secret
func parseAPIKeyToken(token string) (string, string, bool) {
	rest, ok := strings.CutPrefix(token, apiKeyPrefix)
	if !ok {
		return "", "", false
	}
	// The ID is hex, so the first _ ends it; the secret may contain more
	id, secret, ok := strings.Cut(rest, "_")
	if !ok ||
		len(id) != hex.EncodedLen(apiKeyIDBytes) ||
		len(secret) != base64.RawURLEncoding.EncodedLen(apiKeySecretBytes) {
		return "", "", false
	}
	if _, err := hex.DecodeString(id); err != nil {
		return "", "", false
	}
	if _, err := base64.RawURLEncoding.DecodeString(secret); err != nil {
		return "", "", false
	}
	return id, secret, true
}

func hashAPIKeySecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// verifyAPIKey returns the key a token belongs to, if the token is valid and
// the key hasn't expired
func verifyAPIKey(token string, now time.Time) (*APIKey, error) {
	id, secret, ok := parseAPIKeyToken(token)
	if !ok {
		return nil, fmt.Errorf("malformed token")
	}
	var stored storedAPIKey
	err := DB.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(APIKeysBucket).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("no key %s", id)
		}
		return json.Unmarshal(data, &stored)
	})
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(hashAPIKeySecret(secret), stored.SecretHash) != 1 {
		return nil, fmt.Errorf("wrong secret for key %s", id)
	}
	if stored.expired(now) {
		return nil, fmt.Errorf("key %s expired at %s", id, stored.ExpiresAt.Format(time.RFC3339))
	}
	return &stored.APIKey, nil
}

// touchAPIKey records that the key was used, unless that was recorded less
// than apiKeyTouchInterval ago
func touchAPIKey(key *APIKey, now time.Time) error {
	if key.LastUsedAt != nil && now.Sub(*key.LastUsedAt) < apiKeyTouchInterval {
		return nil
	}
	return DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		data := b.Get([]byte(key.ID))
		// Revoked since it was checked: writing it would bring it back
		if data == nil {
			return nil
		}
		var stored storedAPIKey
		if err := json.Unmarshal(data, &stored); err != nil {
			return err
		}
		// Another request may have recorded a later use since key was read
		if stored.LastUsedAt != nil && now.Sub(*stored.LastUsedAt) < apiKeyTouchInterval {
			return nil
		}
		stored.LastUsedAt = &now
		updated, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		return b.Put([]byte(key.ID), updated)
	})
}

// apiKeyFromForm validates a request to create a key and returns the key,
// without its ID
func apiKeyFromForm(form url.Values, now time.Time) (*APIKey, error) {
	key := &APIKey{
		Name:      strings.TrimSpace(form.Get("name")),
		CreatedAt: now,
	}
	if key.Name == "" {
		return nil, fmt.Errorf("give the key a name")
	}
	if utf8.RuneCountInString(key.Name) > maxAPIKeyNameLength {
		return nil, fmt.Errorf("the name is longer than %d characters", maxAPIKeyNameLength)
	}
	grant, err := grantFromForm(form)
	if err != nil {
		return nil, err
	}
	key.Grant = grant

	days, err := strconv.Atoi(form.Get("expiresInDays"))
	if err != nil || days < 0 || days > maxAPIKeyExpiryDays {
		return nil, fmt.Errorf("expiresInDays must be 0 (never) to %d", maxAPIKeyExpiryDays)
	}
	if days > 0 {
		expires := now.AddDate(0, 0, days)
		key.ExpiresAt = &expires
	}
	return key, nil
}

// refreshKeyCreatorGroups records groups as the creator groups of the keys
// email created, so that the keys follow the groups the person is in now
func refreshKeyCreatorGroups(email string, groups []string) error {
	apiKeyOwnersMu.Lock()
	defer apiKeyOwnersMu.Unlock()
	return DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		updates := map[string][]byte{}
		err := b.ForEach(func(k, v []byte) error {
			var stored storedAPIKey
			if err := json.Unmarshal(v, &stored); err != nil {
				return fmt.Errorf("decode api key: %w", err)
			}
			if !strings.EqualFold(stored.CreatedBy, email) || slices.Equal(stored.CreatorGroups, groups) {
				return nil
			}
			stored.CreatorGroups = append([]string{}, groups...)
			data, err := json.Marshal(stored)
			if err != nil {
				return fmt.Errorf("encode api key: %w", err)
			}
			updates[string(k)] = data
			return nil
		})
		if err != nil {
			return err
		}
		// Not inside ForEach: Bolt doesn't allow changing a bucket it iterates
		for k, data := range updates {
			if err := b.Put([]byte(k), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// saveNewAPIKey stores key under a new ID and returns its token
func saveNewAPIKey(key *APIKey) (string, error) {
	secretBytes := make([]byte, apiKeySecretBytes)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		idBytes := make([]byte, apiKeyIDBytes)
		for {
			if _, err := rand.Read(idBytes); err != nil {
				return fmt.Errorf("generate ID: %w", err)
			}
			key.ID = hex.EncodeToString(idBytes)
			if b.Get([]byte(key.ID)) == nil {
				break
			}
		}
		data, err := json.Marshal(storedAPIKey{APIKey: *key, SecretHash: hashAPIKeySecret(secret)})
		if err != nil {
			return err
		}
		return b.Put([]byte(key.ID), data)
	})
	if err != nil {
		return "", err
	}
	return apiKeyPrefix + key.ID + "_" + secret, nil
}

func requestLogger(r *http.Request) *slog.Logger {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		return L
	}
	return logger
}

func writePlainText(w http.ResponseWriter, logger *slog.Logger, status int, text string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	writeBody(logger, w, []byte(text))
}

func writeJSON(w http.ResponseWriter, logger *slog.Logger, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		logger.Error("marshal response", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payload)
}

// RevokedData says how many keys and sessions revoking a person ended
type RevokedData struct {
	Keys     int `json:"keys"`
	Sessions int `json:"sessions"`
}

// APIKeysData lists the API keys the requester may see
type APIKeysData struct {
	// Admin is true for the admin password, which sees and revokes every key
	Admin bool      `json:"admin"`
	Keys  []*APIKey `json:"keys"`
	// People, for the admin, are everyone with a key or a single sign-on
	// session: whose access can be revoked
	People []string `json:"people,omitempty"`
}

// HandleAPIKeysList returns the API keys the requester may see, newest
// first: all of them for the admin password, their own for an SSO user
// @Summary      List API keys
// @Description  The admin password sees every key, a single sign-on user only their own. Secrets are never returned. Not available with an API key
// @Tags         keys
// @Produce      json
// @Success      200      {object}   APIKeysData
// @Failure      403      {string}   string
// @Failure      500      {string}   string
// @Router       /api/keys/ [get]
func HandleAPIKeysList(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	user, err := requestUser(r)
	if err != nil {
		logger.Warn("find who lists api keys", "err", err)
		writePlainText(w, logger, http.StatusForbidden, "Forbidden")
		return
	}
	keys := []*APIKey{}
	err = DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(APIKeysBucket).ForEach(func(_, data []byte) error {
			var stored storedAPIKey
			if err := json.Unmarshal(data, &stored); err != nil {
				return err
			}
			if user == "" || strings.EqualFold(stored.CreatedBy, user) {
				keys = append(keys, &stored.APIKey)
			}
			return nil
		})
	})
	if err != nil {
		logger.Error("list api keys", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	slices.SortFunc(keys, func(a, b *APIKey) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	data := APIKeysData{Admin: user == "", Keys: keys}
	if data.Admin {
		data.People = keyPeople(keys, GlobalSessionStorage.Users())
	}
	writeJSON(w, logger, data)
}

// keyPeople returns everyone who created a key or is signed in with single
// sign-on, once each ignoring case, sorted
func keyPeople(keys []*APIKey, signedIn []string) []string {
	seen := map[string]bool{}
	people := []string{}
	add := func(person string) {
		if person != "" && !seen[strings.ToLower(person)] {
			seen[strings.ToLower(person)] = true
			people = append(people, person)
		}
	}
	for _, key := range keys {
		add(key.CreatedBy)
	}
	for _, person := range signedIn {
		add(person)
	}
	slices.Sort(people)
	return people
}

// HandleAPIKeyCreate creates an API key
// @Summary      Create an API key
// @Description  The response holds the token, which is never shown again. Not available with an API key. All parameters are available as query parameters and as formData
// @Tags         keys
// @Produce      json
// @Param        name           formData  string    true   "Name of the key"
// @Param        scope          formData  []string  true   "read, run or write; repeat for several"  collectionFormat(multi)
// @Param        allJobs        formData  boolean   false  "Cover all jobs, including jobs added later; required for write"
// @Param        job            formData  []string  false  "Job the key covers; repeat for several"  collectionFormat(multi)
// @Param        pattern        formData  []string  false  "Job name pattern the key covers, * matches anything; repeat for several"  collectionFormat(multi)
// @Param        expiresInDays  formData  integer   true   "Days until the key expires, 0 for never"
// @Success      200      {object}   NewAPIKeyData
// @Failure      400      {string}   string
// @Failure      500      {string}   string
// @Router       /api/keys/ [post]
func HandleAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	if err := r.ParseForm(); err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	key, err := apiKeyFromForm(r.Form, time.Now())
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	apiKeyOwnersMu.Lock()
	defer apiKeyOwnersMu.Unlock()
	key.CreatedBy, err = requestUser(r)
	if err != nil {
		logger.Warn("find who creates an api key", "err", err)
		writePlainText(w, logger, http.StatusForbidden, "Forbidden")
		return
	}
	if key.CreatedBy != "" {
		access := accessFromContext(r.Context())
		if access == nil {
			writePlainText(w, logger, http.StatusForbidden, "Forbidden")
			return
		}
		if err := access.checkWithin(&key.Grant); err != nil {
			writePlainText(w, logger, http.StatusForbidden, "The key can't do more than you: "+err.Error())
			return
		}
		// Never nil, which would leave the key unlimited by groups
		key.CreatorGroups = append([]string{}, access.groups...)
	}
	token, err := saveNewAPIKey(key)
	if err != nil {
		logger.Error("save api key", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("api key created", "apiKey", key.ID, "name", key.Name, "scopes", key.Scopes, "createdBy", key.CreatedBy)
	addAuditAttrs(r, apiKeyAuditAttrs(key)...)
	// The token is in this response only
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, logger, NewAPIKeyData{Key: key, Token: token})
}

// HandleAPIKeysRevokeBy deletes all API keys one person created and ends
// their sessions, e.g. when they leave. Only the admin password may do this.
// @Summary      Revoke all API keys of a person and sign them out
// @Description  Only with the admin password; not available with an API key
// @Tags         keys
// @Produce      plain
// @Param        createdBy  query      string   true   "Single sign-on email the keys were created with"
// @Success      200        {object}   RevokedData
// @Failure      400        {string}   string
// @Failure      403        {string}   string
// @Failure      500        {string}   string
// @Router       /api/keys/ [delete]
func HandleAPIKeysRevokeBy(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	if user, err := requestUser(r); err != nil || user != "" {
		logger.Warn("revoke a person's api keys without the admin password", "user", user, "err", err)
		writePlainText(w, logger, http.StatusForbidden, "Only the admin password can revoke another person's keys")
		return
	}
	createdBy := strings.TrimSpace(r.URL.Query().Get("createdBy"))
	// Required, so a bare DELETE can't revoke the keys made with the password
	if createdBy == "" {
		writePlainText(w, logger, http.StatusBadRequest, "createdBy is required")
		return
	}
	apiKeyOwnersMu.Lock()
	defer apiKeyOwnersMu.Unlock()
	// Sessions first: while one is left, it could create a new key
	result := RevokedData{Sessions: GlobalSessionStorage.DeleteUser(createdBy)}
	// Recorded now: the sessions have ended even if deleting the keys fails
	addAuditAttrs(r, "sessions", result.Sessions)
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		var ids [][]byte
		err := b.ForEach(func(id, data []byte) error {
			var stored storedAPIKey
			if err := json.Unmarshal(data, &stored); err != nil {
				return err
			}
			if strings.EqualFold(stored.CreatedBy, createdBy) {
				ids = append(ids, id)
			}
			return nil
		})
		if err != nil {
			return err
		}
		// Deleted after ForEach, which must not change the bucket it walks
		for _, id := range ids {
			if err := b.Delete(id); err != nil {
				return err
			}
		}
		result.Keys = len(ids)
		return nil
	})
	if err != nil {
		logger.Error("revoke api keys", "createdBy", createdBy, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("api keys revoked", "createdBy", createdBy, "keys", result.Keys, "sessions", result.Sessions)
	addAuditAttrs(r, "keys", result.Keys)
	writeJSON(w, logger, result)
}

// HandleAPIKeyRevoke deletes an API key, which stops working at once. The
// admin password may revoke any key, an SSO user only their own.
// @Summary      Revoke an API key
// @Description  The admin password may revoke any key, a single sign-on user only their own. Not available with an API key
// @Tags         keys
// @Produce      plain
// @Param        id       path       string   true   "ID of the key"
// @Success      200      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/keys/{id} [delete]
func HandleAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	user, err := requestUser(r)
	if err != nil {
		logger.Warn("find who revokes an api key", "err", err)
		writePlainText(w, logger, http.StatusForbidden, "Forbidden")
		return
	}
	id := chi.URLParam(r, "id")
	errNotFound := errors.New("no such API key")
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		data := b.Get([]byte(id))
		if data == nil {
			return errNotFound
		}
		var stored storedAPIKey
		decodeErr := json.Unmarshal(data, &stored)
		if user != "" {
			if decodeErr != nil {
				return decodeErr
			}
			// An SSO user doesn't see other people's keys, so to them
			// someone else's key doesn't exist either
			if !strings.EqualFold(stored.CreatedBy, user) {
				return errNotFound
			}
		}
		// The admin password can delete a key that can't be read, which
		// then has no creator to record
		if decodeErr == nil && stored.CreatedBy != "" {
			addAuditAttrs(r, "createdBy", stored.CreatedBy)
		}
		return b.Delete([]byte(id))
	})
	switch {
	case errors.Is(err, errNotFound):
		writePlainText(w, logger, http.StatusNotFound, err.Error())
	case err != nil:
		logger.Error("revoke api key", "apiKey", id, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
	default:
		logger.Info("api key revoked", "apiKey", id, "by", user)
	}
}

// apiKeyAuditAttrs describe a new key in the audit log: what it may do, never
// its secret
func apiKeyAuditAttrs(key *APIKey) []any {
	expires := "never"
	if key.ExpiresAt != nil {
		expires = key.ExpiresAt.Format(time.RFC3339)
	}
	attrs := []any{"key", key.ID, "name", key.Name, "scopes", strings.Join(key.Scopes, ","), "allJobs", key.AllJobs}
	if !key.AllJobs {
		// JSON lists: a job name can hold a comma, or be "all"
		attrs = append(attrs, auditListAttrs("jobs", key.Jobs)...)
		attrs = append(attrs, auditListAttrs("patterns", key.Patterns)...)
	}
	return append(attrs, "expires", expires)
}

// routeAPIKeys sends requests that carry an API key to keyRouter and all
// others to router, so a key never reaches the password and session checks
func routeAPIKeys(keyRouter, router http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := apiKeyToken(r); ok {
			keyRouter.ServeHTTP(w, r)
			return
		}
		router.ServeHTTP(w, r)
	})
}

// newAPIKeyRouter routes requests that carry an API key. Only the routes
// registered here accept keys, each with the scope it needs and the job it
// acts on; any other route gets 403. So a route added to the main router
// stays closed to keys until it is added here too.
func newAPIKeyRouter(storage http.Handler) http.Handler {
	router := chi.NewRouter()
	// Only to tell an /api route that keys can't use from one that doesn't
	// exist; it never serves a request
	passwordRoutes := chi.NewRouter()
	passwordRoutes.Route("/api", apiRoutes)
	reject := rejectAPIKeyRoute(passwordRoutes, router)
	// Set before the routes, so the sub-routers get them too
	router.NotFound(reject)
	router.MethodNotAllowed(reject)
	router.Use(LogMi)
	router.Use(SecurityMi)
	router.Use(CORSMi)
	// Before the key is checked, so refused changes are audited too
	router.Use(AuditMi)
	router.Use(APIKeyAuthMi)

	router.Route("/api", func(router chi.Router) {
		// Lists are filtered to the key's jobs by their handlers
		router.With(requireAccess(ScopeRead, nil)).Get("/feed", HandleFeedView)

		router.Route("/jobs", func(router chi.Router) {
			router.With(requireAccess(ScopeRead, nil)).Get("/", HandleJobsView)
			router.With(requireAccess(ScopeWrite, nil)).Post("/create", HandleJobsCreate)
		})

		router.Route("/job", func(router chi.Router) {
			router.With(requireAccess(ScopeRun, jobFromName), rejectUnsafeParams).Post("/{name}/run", HandleRunJob)
			router.With(requireAccess(ScopeWrite, jobFromName)).Delete("/{name}", HandleDeleteJob)
			router.With(requireAccess(ScopeWrite, jobFromName)).Post("/{name}", HandleJobPost)
			router.With(requireAccess(ScopeRead, jobFromName)).Get("/{name}", HandleJobGet)
			router.With(requireAccess(ScopeWrite, jobFromName)).Post("/{name}/set_active", HandleJobSetActive)
		})

		router.Route("/build", func(router chi.Router) {
			router.With(requireAccess(ScopeRead, jobFromBuild)).Get("/{id}", HandleGetBuild)
			router.With(requireAccess(ScopeRun, jobFromBuild)).Post("/{id}/abort", HandleAbortBuild)
			router.With(requireAccess(ScopeRun, jobFromBuild)).Post("/{id}/flush", HandleFlushTaskLogs)
			router.With(requireAccess(ScopeRun, jobFromBuild)).Post("/{id}/start", HandleStartBuild)
		})
	})

	router.Route("/storage", func(router chi.Router) {
		router.Use(StorageSecurityMi)
		router.With(requireAccess(ScopeRead, jobFromStorage)).Method("GET", "/build/*", storage)
		router.With(requireAccess(ScopeRead, jobFromStorage)).Method("HEAD", "/build/*", storage)
	})
	return router
}

// rejectAPIKeyRoute answers a key's request that the key router has no route
// for. Under /api, a path that doesn't exist gets 404 and one that exists
// only with other methods gets 405, so that a client can tell a mistyped URL
// from a route closed to keys. Everything else gets 403.
func rejectAPIKeyRoute(passwordRoutes, keyRoutes chi.Routes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := requestLogger(r)
		routePath := r.URL.Path
		if r.URL.RawPath != "" {
			routePath = r.URL.RawPath
		}
		if !strings.HasPrefix(routePath, "/api/") || hasRoute(passwordRoutes, r.Method, routePath) {
			logger.Warn("api key used on a route that doesn't accept keys", "method", r.Method, "path", r.URL.Path)
			writePlainText(w, logger, http.StatusForbidden, "API keys can't be used here")
			return
		}
		var allowed []string
		for _, method := range routeMethods {
			if hasRoute(keyRoutes, method, routePath) || hasRoute(passwordRoutes, method, routePath) {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			writePlainText(w, logger, http.StatusMethodNotAllowed, r.Method+" isn't allowed on "+r.URL.Path)
			return
		}
		writePlainText(w, logger, http.StatusNotFound, "No API route "+r.URL.Path)
	}
}

// routeMethods are the methods the API's routes use
var routeMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete}

func hasRoute(routes chi.Routes, method, routePath string) bool {
	return routes.Find(chi.NewRouteContext(), method, routePath) != ""
}

// APIKeyAuthMi checks the API key a request carries and adds it to the context
func APIKeyAuthMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := requestLogger(r)
		token, _ := apiKeyToken(r)
		now := time.Now()
		key, err := verifyAPIKey(token, now)
		if err != nil {
			logger.Warn("api key rejected", "err", err)
			// Only failures are limited: a key can't be guessed, so blocking
			// valid keys would only let bad ones lock them out. reject also
			// clears a block that has expired, which recordFailure doesn't.
			if apiKeyAttemptLimiter.reject(w, r, logger) {
				return
			}
			blocked := apiKeyAttemptLimiter.recordFailure(w, r, logger)
			// The error names the key ID at most, never the secret
			audit(r, "login.apikey", "failed", append([]any{"reason", err.Error()}, blockedAttrs(blocked)...)...)
			if blocked {
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writePlainText(w, logger, http.StatusUnauthorized, "Invalid or expired API key")
			return
		}
		logger = logger.With("apiKey", key.ID)
		setRequestUser(r, "apikey", key.CreatedBy, key.ID)
		if err := touchAPIKey(key, now); err != nil {
			logger.Error("record api key use", "err", err)
		}
		access, err := apiKeyAccess(key)
		if err != nil {
			logger.Error("find api key access", "err", err)
			writePlainText(w, logger, http.StatusInternalServerError, "Can't read the access of this API key")
			return
		}
		ctx := context.WithValue(r.Context(), HL, logger)
		ctx = context.WithValue(ctx, apiKeyContextKey, key)
		ctx = context.WithValue(ctx, accessContextKey{}, access)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// apiKeyAccess is what a request with key may do: what the key grants and,
// for a key made by a single sign-on user, also what the creator's groups
// allow now. Keys made with the admin password, and keys made before wakeci
// recorded the creator's groups, are limited by their grant only.
func apiKeyAccess(key *APIKey) (*Access, error) {
	access := &Access{Grants: []Grant{key.Grant}, subject: "This API key"}
	if key.CreatedBy != "" && key.CreatorGroups != nil {
		within, err := ssoGroupsAccess(key.CreatorGroups)
		if err != nil {
			return nil, err
		}
		access.Within = within
	}
	return access, nil
}

// jobResolver returns the job a request acts on, or the response status and
// error when it can't
type jobResolver func(r *http.Request) (string, int, error)

func jobFromName(r *http.Request) (string, int, error) {
	name, err := jobNameParam(r)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	return name, 0, nil
}

func jobFromBuild(r *http.Request) (string, int, error) {
	id, err := buildIDParam(r)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	return buildJob(id)
}

// jobFromStorage finds the build a storage request reads from, in the path
// that the file server will serve
func jobFromStorage(r *http.Request) (string, int, error) {
	// http.FileServer cleans the path the same way before serving it
	cleaned := path.Clean("/" + wakespacePath(r))
	first, _, _ := strings.Cut(strings.TrimPrefix(cleaned, "/"), "/")
	id, err := strconv.Atoi(first)
	if err != nil || strconv.Itoa(id) != first {
		return "", http.StatusForbidden, fmt.Errorf("only the files of a build can be read here")
	}
	return buildJob(id)
}

// buildJob returns the name of the job that build id belongs to
func buildJob(id int) (string, int, error) {
	build, err := readBuildStatus(id)
	switch {
	case errors.Is(err, errBuildNotFound):
		return "", http.StatusNotFound, err
	case err != nil:
		return "", http.StatusInternalServerError, err
	}
	return build.Name, 0, nil
}

// rejectUnsafeParams refuses run parameters that reference a secret or hold
// a quote, line break or NUL. Run parameters go through injectSecrets, so
// without the first a run key could send any secret wherever the job sends
// that parameter. The others would let it add variables, e.g. BASH_ENV, when
// a job writes the parameter into build.env: godotenv starts a new variable
// after a quoted value on the same line (TARGET="a" BASH_ENV=...), and
// without quotes it reads the rest of the line as one value. The backtick is
// a quote to some dotenv parsers.
func rejectUnsafeParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := requestLogger(r)
		if err := r.ParseForm(); err != nil {
			writePlainText(w, logger, http.StatusBadRequest, err.Error())
			return
		}
		for name, values := range r.Form {
			if slices.ContainsFunc(values, secretsRegex.MatchString) {
				logger.Warn("api key run parameter references a secret", "param", name)
				writePlainText(w, logger, http.StatusBadRequest,
					"Parameter "+name+" references a secret, which isn't allowed with an API key")
				return
			}
			if slices.ContainsFunc(values, func(v string) bool { return strings.ContainsAny(v, "\"'`\r\n\x00") }) {
				logger.Warn("api key run parameter has a quote, line break or NUL", "param", name)
				writePlainText(w, logger, http.StatusBadRequest,
					"Parameter "+name+" has a quote, line break or NUL, which isn't allowed with an API key")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
