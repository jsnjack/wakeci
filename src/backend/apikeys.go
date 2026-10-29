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

type contextKey string

const apiKeyContextKey contextKey = "apiKey"

// APIKey is what wakeci keeps about an API key, apart from its secret
type APIKey struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	// AllJobs covers every job, including jobs added later. Otherwise the key
	// covers Jobs by exact name and the jobs that Patterns match.
	AllJobs    bool       `json:"allJobs"`
	Jobs       []string   `json:"jobs"`
	Patterns   []string   `json:"patterns"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
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

// HasScope reports whether the key grants scope. Write lets a key run any
// command on the server, so it only counts on a key that covers all jobs.
func (k *APIKey) HasScope(scope string) bool {
	return slices.Contains(k.Scopes, scope) && (scope != ScopeWrite || k.AllJobs)
}

// CoversJob reports whether the key can be used on the job name
func (k *APIKey) CoversJob(name string) bool {
	return k.AllJobs || slices.Contains(k.Jobs, name) ||
		slices.ContainsFunc(k.Patterns, func(pattern string) bool {
			return matchJobPattern(pattern, name)
		})
}

func (k *APIKey) expired(now time.Time) bool {
	return k.ExpiresAt != nil && !now.Before(*k.ExpiresAt)
}

// matchJobPattern reports whether name matches pattern, in which * matches
// any run of characters and every other character only itself
func matchJobPattern(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	rest, ok := strings.CutPrefix(name, parts[0])
	if !ok {
		return false
	}
	// Taking each middle part at its first match leaves the most room for
	// the ones after it
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	return strings.HasSuffix(rest, parts[len(parts)-1])
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
		Jobs:      []string{},
		Patterns:  []string{},
		CreatedAt: now,
	}
	if key.Name == "" {
		return nil, fmt.Errorf("give the key a name")
	}
	if utf8.RuneCountInString(key.Name) > maxAPIKeyNameLength {
		return nil, fmt.Errorf("the name is longer than %d characters", maxAPIKeyNameLength)
	}

	for _, scope := range form["scope"] {
		if !slices.Contains(apiKeyScopes, scope) {
			return nil, fmt.Errorf("unknown scope %q", scope)
		}
	}
	for _, scope := range apiKeyScopes {
		if slices.Contains(form["scope"], scope) {
			key.Scopes = append(key.Scopes, scope)
		}
	}
	if len(key.Scopes) == 0 {
		return nil, fmt.Errorf("select at least one scope")
	}

	switch form.Get("allJobs") {
	case "true":
		key.AllJobs = true
	case "false", "":
	default:
		return nil, fmt.Errorf("allJobs must be true or false")
	}
	if !key.AllJobs {
		var err error
		if key.Jobs, err = uniqueJobNames(form["job"]); err != nil {
			return nil, err
		}
		if key.Patterns, err = uniqueJobNames(form["pattern"]); err != nil {
			return nil, err
		}
		if len(key.Jobs) == 0 && len(key.Patterns) == 0 {
			return nil, fmt.Errorf("select a job or enter a pattern")
		}
		if slices.Contains(key.Scopes, ScopeWrite) {
			return nil, fmt.Errorf("write can run any command on the server, so it needs access to all jobs")
		}
	}

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

// uniqueJobNames checks job names or patterns and drops repeats
func uniqueJobNames(values []string) ([]string, error) {
	names := []string{}
	for _, value := range values {
		if !isValidJobName(value) {
			return nil, fmt.Errorf("invalid job name or pattern %q", value)
		}
		if !slices.Contains(names, value) {
			names = append(names, value)
		}
	}
	return names, nil
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

// HandleAPIKeysList returns all API keys, newest first
// @Summary      List API keys
// @Description  Secrets are never returned. Not available with an API key
// @Tags         keys
// @Produce      json
// @Success      200      {array}    APIKey
// @Failure      500      {string}   string
// @Router       /keys/ [get]
func HandleAPIKeysList(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	keys := []*APIKey{}
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(APIKeysBucket).ForEach(func(_, data []byte) error {
			var stored storedAPIKey
			if err := json.Unmarshal(data, &stored); err != nil {
				return err
			}
			keys = append(keys, &stored.APIKey)
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
	writeJSON(w, logger, keys)
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
// @Router       /keys/ [post]
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
	token, err := saveNewAPIKey(key)
	if err != nil {
		logger.Error("save api key", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("api key created", "apiKey", key.ID, "name", key.Name, "scopes", key.Scopes)
	// The token is in this response only
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, logger, NewAPIKeyData{Key: key, Token: token})
}

// HandleAPIKeyRevoke deletes an API key, which stops working at once
// @Summary      Revoke an API key
// @Description  Not available with an API key
// @Tags         keys
// @Produce      plain
// @Param        id       path       string   true   "ID of the key"
// @Success      200      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /keys/{id} [delete]
func HandleAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	id := chi.URLParam(r, "id")
	errNotFound := errors.New("no such API key")
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		if b.Get([]byte(id)) == nil {
			return errNotFound
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
		logger.Info("api key revoked", "apiKey", id)
	}
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
// acts on; anything else gets 403. So a route added to the main router stays
// closed to keys until it is added here too.
func newAPIKeyRouter(storage http.Handler) http.Handler {
	router := chi.NewRouter()
	// Set before the routes, so the sub-routers get them too
	router.NotFound(rejectAPIKeyRoute)
	router.MethodNotAllowed(rejectAPIKeyRoute)
	router.Use(LogMi)
	router.Use(SecurityMi)
	router.Use(CORSMi)
	router.Use(APIKeyAuthMi)

	router.Route("/api", func(router chi.Router) {
		// Lists are filtered to the key's jobs by their handlers
		router.With(requireAPIKey(ScopeRead, nil)).Get("/feed", HandleFeedView)

		router.Route("/jobs", func(router chi.Router) {
			router.With(requireAPIKey(ScopeRead, nil)).Get("/", HandleJobsView)
			router.With(requireAPIKey(ScopeWrite, nil)).Post("/create", HandleJobsCreate)
		})

		router.Route("/job", func(router chi.Router) {
			router.With(requireAPIKey(ScopeRun, jobFromName), rejectUnsafeParams).Post("/{name}/run", HandleRunJob)
			router.With(requireAPIKey(ScopeWrite, jobFromName)).Delete("/{name}", HandleDeleteJob)
			router.With(requireAPIKey(ScopeWrite, jobFromName)).Post("/{name}", HandleJobPost)
			router.With(requireAPIKey(ScopeRead, jobFromName)).Get("/{name}", HandleJobGet)
			router.With(requireAPIKey(ScopeWrite, jobFromName)).Post("/{name}/set_active", HandleJobSetActive)
		})

		router.Route("/build", func(router chi.Router) {
			router.With(requireAPIKey(ScopeRead, jobFromBuild)).Get("/{id}", HandleGetBuild)
			router.With(requireAPIKey(ScopeRun, jobFromBuild)).Post("/{id}/abort", HandleAbortBuild)
			router.With(requireAPIKey(ScopeRun, jobFromBuild)).Post("/{id}/flush", HandleFlushTaskLogs)
			router.With(requireAPIKey(ScopeRun, jobFromBuild)).Post("/{id}/start", HandleStartBuild)
		})
	})

	router.Route("/storage", func(router chi.Router) {
		router.Use(StorageSecurityMi)
		router.With(requireAPIKey(ScopeRead, jobFromStorage)).Method("GET", "/build/*", storage)
		router.With(requireAPIKey(ScopeRead, jobFromStorage)).Method("HEAD", "/build/*", storage)
	})
	return router
}

func rejectAPIKeyRoute(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	logger.Warn("api key used on a route that doesn't accept keys", "method", r.Method, "path", r.URL.Path)
	writePlainText(w, logger, http.StatusForbidden, "API keys can't be used here")
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
			// valid keys would only let bad ones lock them out
			if apiKeyAttemptLimiter.recordFailure(w, r, logger) {
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writePlainText(w, logger, http.StatusUnauthorized, "Invalid or expired API key")
			return
		}
		logger = logger.With("apiKey", key.ID)
		if err := touchAPIKey(key, now); err != nil {
			logger.Error("record api key use", "err", err)
		}
		ctx := context.WithValue(r.Context(), HL, logger)
		ctx = context.WithValue(ctx, apiKeyContextKey, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// jobResolver returns the job a request acts on, or the response status and
// error when it can't
type jobResolver func(r *http.Request) (string, int, error)

// requireAPIKey lets a request through if its API key has scope and, when
// jobOf is set, covers the job the request acts on
func requireAPIKey(scope string, jobOf jobResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := requestLogger(r)
			key := apiKeyFromContext(r.Context())
			if key == nil || !key.HasScope(scope) {
				logger.Warn("api key lacks scope", "scope", scope)
				writePlainText(w, logger, http.StatusForbidden, "This API key doesn't have the "+scope+" scope")
				return
			}
			if jobOf != nil {
				job, status, err := jobOf(r)
				if err != nil {
					logger.Warn("find job for api key", "err", err)
					writePlainText(w, logger, status, err.Error())
					return
				}
				if !key.CoversJob(job) {
					logger.Warn("api key doesn't cover job", "job", job)
					// The job of a build is only known to keys that cover it
					writePlainText(w, logger, http.StatusForbidden, "This API key doesn't cover that job")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func jobFromName(r *http.Request) (string, int, error) {
	name, err := jobNameParam(r)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	return name, 0, nil
}

func jobFromBuild(r *http.Request) (string, int, error) {
	idp := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idp)
	if err != nil {
		return "", http.StatusBadRequest, fmt.Errorf("invalid build ID %q", idp)
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
		return "", http.StatusForbidden, fmt.Errorf("API keys can only read the files of a build")
	}
	return buildJob(id)
}

// buildJob returns the name of the job that build id belongs to
func buildJob(id int) (string, int, error) {
	var build BuildUpdateData
	err := DB.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(HistoryBucket).Get(Itob(id))
		if data == nil {
			return errBuildNotFound
		}
		return json.Unmarshal(data, &build)
	})
	switch {
	case errors.Is(err, errBuildNotFound):
		return "", http.StatusNotFound, fmt.Errorf("build %d not found", id)
	case err != nil:
		return "", http.StatusInternalServerError, fmt.Errorf("read build %d: %w", id, err)
	}
	return build.Name, 0, nil
}

// rejectUnsafeParams refuses run parameters that reference a secret or hold
// a line break or NUL. Run parameters go through injectSecrets, so without
// the first a run key could send any secret wherever the job sends that
// parameter; a line break would let it add variables, e.g. BASH_ENV, when a
// job writes the parameter into build.env.
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
			if slices.ContainsFunc(values, func(v string) bool { return strings.ContainsAny(v, "\r\n\x00") }) {
				logger.Warn("api key run parameter has a line break or NUL", "param", name)
				writePlainText(w, logger, http.StatusBadRequest,
					"Parameter "+name+" has a line break or NUL, which isn't allowed with an API key")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
