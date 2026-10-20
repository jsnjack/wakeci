package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Artifacts run in an opaque origin. A preview URL grants only the report's
// files, and remains tied to the credential that authorized opening it.
const artifactCSP = "sandbox allow-scripts allow-downloads; " +
	"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'none'; form-action 'none'; frame-ancestors 'self'; base-uri 'none'; object-src 'none'"

const previewTTL = 15 * time.Minute

var previewKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("generate preview key: %v", err))
	}
	return key
}()

var errInvalidPreview = errors.New("invalid preview token")

type previewClaims struct {
	Build               int    `json:"b"`
	Expires             int64  `json:"e"`
	Principal           string `json:"p"`
	Provider            string `json:"v"`
	KeyID               string `json:"k,omitempty"`
	SessionDigest       string `json:"s,omitempty"`
	PasswordFingerprint string `json:"f,omitempty"`
	Directory           string `json:"d"`
}

func signPreview(payload string) string {
	mac := hmac.New(sha256.New, previewKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func previewDirectory(file string) string {
	build, _, err := storedFile(strings.TrimPrefix(file, "/"))
	if err != nil {
		return ""
	}
	artifacts := "/" + strconv.Itoa(build) + "/artifacts"
	if file == artifacts || strings.HasPrefix(file, artifacts+"/") {
		return artifacts + "/"
	}
	if isStoredDirectory(file) {
		return strings.TrimSuffix(file, "/") + "/"
	}
	return path.Dir(file) + "/"
}

func newPreviewToken(r *http.Request, build int, file string, now time.Time) (string, error) {
	p := principalFrom(r)
	if p == nil {
		return "", errCredentialRevoked
	}
	claims := previewClaims{Build: build, Expires: now.Add(previewTTL).Unix(), Principal: p.ID, Provider: p.Provider, Directory: previewDirectory(file)}
	switch p.Via {
	case viaSession:
		cookie, err := r.Cookie("session")
		if err != nil {
			return "", fmt.Errorf("preview session cookie: %w", err)
		}
		claims.SessionDigest = passwordFingerprint([]byte(cookie.Value))
	case viaBasic:
		hash, ok := r.Context().Value(verifiedPasswordHashKey{}).([]byte)
		if !ok {
			return "", errCredentialRevoked
		}
		claims.PasswordFingerprint = passwordFingerprint(hash)
	case viaAPIKey:
		claims.KeyID = p.KeyID
	default:
		return "", errCredentialRevoked
	}
	data, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal preview claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return payload + "." + signPreview(payload), nil
}

func checkPreviewToken(token string, now time.Time) (*previewClaims, error) {
	if len(token) > 4096 {
		return nil, errInvalidPreview
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(signPreview(payload))) {
		return nil, fmt.Errorf("%w: bad signature", errInvalidPreview)
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidPreview, err)
	}
	var claims previewClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidPreview, err)
	}
	if now.Unix() >= claims.Expires || claims.Directory == "" {
		return nil, fmt.Errorf("%w: expired or missing scope", errInvalidPreview)
	}
	switch {
	case claims.SessionDigest != "":
		var principal *Principal
		principal, err = GlobalSessionStorage.verifyDigest(claims.SessionDigest, now)
		if err == nil {
			if principal.ID != claims.Principal || principal.Provider != claims.Provider {
				err = errCredentialRevoked
			} else {
				err = checkSessionPrincipal(principal, now)
			}
		}
	case claims.KeyID != "":
		err = checkAPIKeyByID(claims.KeyID, now)
	case claims.PasswordFingerprint != "" && claims.Provider == ProviderPassword:
		var hash []byte
		hash, err = readPasswordHash()
		if err == nil && (!passwordLoginEnabled() || claims.PasswordFingerprint != passwordFingerprint(hash)) {
			err = errCredentialRevoked
		}
	default:
		err = errCredentialRevoked
	}
	if isRevoked(err) {
		return nil, fmt.Errorf("%w: %w", errInvalidPreview, err)
	}
	if err != nil {
		return nil, err
	}
	return &claims, nil
}

func storedFile(rest string) (int, string, error) {
	file := path.Clean("/" + rest)
	dir, _, _ := strings.Cut(strings.TrimPrefix(file, "/"), "/")
	build, err := strconv.Atoi(dir)
	if err != nil || build <= 0 || strconv.Itoa(build) != dir {
		return 0, "", errBuildNotFound
	}
	return build, file, nil
}

func isStoredDirectory(file string) bool {
	build, _, err := storedFile(strings.TrimPrefix(file, "/"))
	if err != nil {
		return false
	}
	root, err := os.OpenRoot(Config.WorkDir + "wakespace/" + strconv.Itoa(build))
	if err != nil {
		return false
	}
	defer closeBuildRoot(root)
	relative := strings.TrimPrefix(file, "/"+strconv.Itoa(build))
	info, err := root.Stat(strings.TrimPrefix(relative+"/.", "/"))
	return err == nil && info.IsDir()
}

func isPage(file string) bool {
	switch strings.ToLower(path.Ext(file)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return isStoredDirectory(file)
}

func closeBuildRoot(root *os.Root) {
	if err := root.Close(); err != nil {
		L.Log(context.Background(), LevelTrace, "close build root", "err", err)
	}
}

// hiddenFileSystem conceals filesystem failures behind the same not-found response.
type hiddenFileSystem struct{ fs.FS }

func (h hiddenFileSystem) Open(name string) (fs.File, error) {
	file, err := h.FS.Open(name)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	return file, nil
}

// serveBuildFile confines all file opens, including symlink resolution, to one build.
func serveBuildFile(w http.ResponseWriter, r *http.Request) {
	build, file, err := storedFile(strings.TrimPrefix(r.URL.Path, "/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(Config.WorkDir + "wakespace/" + strconv.Itoa(build))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer closeBuildRoot(root)
	r2 := r.Clone(r.Context())
	r2.URL.Path = strings.TrimPrefix(file, "/"+strconv.Itoa(build))
	if r2.URL.Path == "" {
		r2.URL.Path = "/"
	}
	if strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(r2.URL.Path, "/") {
		r2.URL.Path += "/"
	}
	r2.URL.RawPath = ""
	http.FileServerFS(hiddenFileSystem{root.FS()}).ServeHTTP(w, r2)
}

func serveStoredFile(handler http.Handler, w http.ResponseWriter, r *http.Request, file string) {
	r2 := r.Clone(r.Context())
	if strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(file, "/") {
		file += "/"
	}
	r2.URL.Path, r2.URL.RawPath = file, ""
	handler.ServeHTTP(w, r2)
}

// ArtifactSecurityMi isolates build output and blocks its use as app-origin code.
func ArtifactSecurityMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", artifactCSP)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Vary", "Cookie, Authorization")
		if strings.HasPrefix(r.URL.Path, "/preview/") {
			// Sandboxed reports have an opaque origin; the URL token grants access.
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		if isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=15768000;includeSubdomains")
		}
		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "same-origin") {
			switch r.Header.Get("Sec-Fetch-Dest") {
			case "script", "worker", "sharedworker", "serviceworker", "style":
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		if strings.HasSuffix(r.URL.Path, ".log") {
			w.Header().Set("Content-Type", "text/plain")
		}
		next.ServeHTTP(w, r)
	})
}

// HandleWakespaceResource redirects HTML pages to credential-bound preview URLs.
func HandleWakespaceResource(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*slog.Logger)
		if !ok {
			logger = L
		}
		build, file, err := storedFile(strings.TrimPrefix(r.URL.Path, "/storage/build/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !isPage(file) {
			serveStoredFile(handler, w, r, file)
			return
		}
		token, err := newPreviewToken(r, build, file, time.Now())
		if err != nil {
			writeText(w, logger, http.StatusForbidden, "Your session has ended")
			return
		}
		target := "/preview/" + token + "/build" + file
		if (strings.HasSuffix(r.URL.Path, "/") || isStoredDirectory(file)) && !strings.HasSuffix(target, "/") {
			target += "/"
		}
		w.Header().Set("Location", (&url.URL{Path: target, RawQuery: r.URL.RawQuery}).String())
		w.WriteHeader(http.StatusFound)
	})
}

// servePreviewFile uses two roots: the build boundary, then the signed directory.
func servePreviewFile(w http.ResponseWriter, r *http.Request, claims *previewClaims, file string) {
	buildRoot, err := os.OpenRoot(Config.WorkDir + "wakespace/" + strconv.Itoa(claims.Build))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer closeBuildRoot(buildRoot)
	prefix := "/" + strconv.Itoa(claims.Build)
	directory := strings.TrimPrefix(strings.TrimSuffix(claims.Directory, "/"), prefix+"/")
	if strings.TrimSuffix(claims.Directory, "/") == prefix {
		directory = "."
	}
	root, err := buildRoot.OpenRoot(directory)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer closeBuildRoot(root)
	relative := strings.TrimPrefix(file, strings.TrimSuffix(claims.Directory, "/"))
	if relative == "" {
		relative = "/"
	}
	serveStoredFile(http.FileServerFS(hiddenFileSystem{root.FS()}), w, r, relative)
}

// HandlePreview serves only files in the signed directory while the issuer is active.
func HandlePreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		claims, err := checkPreviewToken(token, time.Now())
		if errors.Is(err, errInvalidPreview) {
			http.Error(w, "This preview has expired, open it from wakeci again", http.StatusForbidden)
			return
		}
		if err != nil {
			L.Error("check preview token", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		build, file, err := storedFile(strings.TrimPrefix(r.URL.Path, "/preview/"+token+"/build/"))
		if err != nil || build != claims.Build || (!strings.HasPrefix(file, claims.Directory) && file != strings.TrimSuffix(claims.Directory, "/")) {
			http.NotFound(w, r)
			return
		}
		servePreviewFile(w, r, claims, file)
	})
}

func logPath(r *http.Request) string {
	if rest, ok := strings.CutPrefix(r.URL.Path, "/preview/"); ok {
		if _, file, ok := strings.Cut(rest, "/"); ok {
			return "/preview/<token>/" + file
		}
		return "/preview/<token>"
	}
	return r.URL.Path
}
