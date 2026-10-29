package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

// HandlerLogger is a special type for loggers per request
type HandlerLogger string

// HL is a handle logger
const HL HandlerLogger = "logger"

// statusRecorder wraps http.ResponseWriter to capture the status code written
// by the handler, so it can be included in the access log line. It forwards
// Hijack/Flush so it stays transparent to websocket upgrades (/ws) and
// streaming/compressed responses.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rec.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}

func (rec *statusRecorder) Flush() {
	if flusher, ok := rec.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// LogMi is a middleware that creates a new logger per request and writes an
// access log line (method, path, status, duration) unconditionally, since
// request traffic is core operational visibility for an HTTP service.
func LogMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		logID := GenerateRandomString(5)

		// Get IP address of a user
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			L.Error("split remote addr", "addr", r.RemoteAddr, "err", err)
			host = r.RemoteAddr
		}

		handlerLogger := L.With("logID", logID, "host", host)

		// Get new context with key-value "settings"
		ctx := context.WithValue(r.Context(), HL, handlerLogger)

		// Get new http.Request with the new context
		r = r.WithContext(ctx)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			handlerLogger.Warn("request completed",
				"method", r.Method, "path", r.URL.Path, "status", rec.status, "took", time.Since(startTime),
			)
		}()

		// Call actual handler
		next.ServeHTTP(rec, r)
	})
}

// CORSMi adds CORS headers
func CORSMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := "*"
		if Config.Hostname != "" {
			origin = "https://" + Config.Hostname
		}
		w.Header().Set("access-control-allow-origin", origin)
		w.Header().Set("access-control-max-age", "86400")
		next.ServeHTTP(w, r)
	})
}

// SecurityMi is a middleware which adds security headers
func SecurityMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("referrer-policy", "no-referrer")
		w.Header().Set("content-security-policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'self'")
		w.Header().Set("x-content-type-options", "nosniff")
		if Config.Hostname != "" {
			w.Header().Set("strict-transport-security", "max-age=15768000;includeSubdomains")
		}
		next.ServeHTTP(w, r)
	})
}

// storagePDFCSP is the storage policy without the sandbox. A PDF can't run
// scripts on this origin, and Chrome won't show one in a sandboxed page.
const storagePDFCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'none'; form-action 'none'; frame-ancestors 'self'"

// storageCSP lets an HTML report run its scripts and load its own files, but
// never act as the logged-in user. connect-src and form-action stop it from
// sending requests itself. It could still get hold of another page on this
// origin whose policy allows requests and send them from there: a frame or
// object, a popup, or a page made up by a service worker. frame-src,
// object-src, worker-src and the sandbox (which leaves out allow-popups)
// close those. allow-same-origin keeps the origin, so the report's CSS,
// scripts and images still get the SameSite session cookie.
const storageCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'none'; form-action 'none'; frame-src 'none'; object-src 'none'; worker-src 'none'; " +
	"frame-ancestors 'none'; sandbox allow-scripts allow-same-origin allow-downloads allow-forms allow-modals"

// StorageSecurityMi is a middleware which adds security headers specifically for storage endpoints
// It is more relaxed than the normal one, as we want to be able to preview html pages.
// Scripts in a previewed page may still run (self-contained reports commonly
// embed inline JS/CSS for interactivity), but storageCSP stops a build
// artifact - which can contain task-influenced content - from ever calling
// back into wakeci's own authenticated API, even though it's served same-site.
func StorageSecurityMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("referrer-policy", "no-referrer")
		csp := storageCSP
		// The file server picks the content type by the file's extension, and
		// with nosniff only application/pdf shows as a PDF. The path isn't
		// cleaned: "x.pdf/" may be a directory whose index is HTML.
		if strings.EqualFold(path.Ext(r.URL.Path), ".pdf") {
			csp = storagePDFCSP
		}
		w.Header().Set("content-security-policy", csp)
		w.Header().Set("x-content-type-options", "nosniff")
		if Config.Hostname != "" {
			w.Header().Set("strict-transport-security", "max-age=15768000;includeSubdomains")
		}
		// Force content-type on .log requests to prevent browsers from downloading it
		if strings.HasSuffix(r.URL.Path, ".log") {
			w.Header().Set("content-type", "text/plain")
		}
		next.ServeHTTP(w, r)
	})
}

// AuthMi checks user credentials
func AuthMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*slog.Logger)
		if !ok {
			logger = L
		}

		// Basic auth for API calls
		_, password, ok := r.BasicAuth()
		if ok {
			if globalAuthAttemptLimiter.reject(w, r, logger) {
				return
			}
			var hashedPassword []byte

			err := DB.View(func(tx *bolt.Tx) error {
				b := tx.Bucket([]byte(GlobalBucket))
				hashedPassword = b.Get([]byte("password"))
				return nil
			})

			if err != nil {
				logger.Error("read password hash", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Header().Set("Content-Type", "text/plain")
				writeBody(logger, w, []byte("Another triumph"))
				return
			}

			err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
			if err != nil {
				logger.Warn("basic auth failed", "err", err)
				if globalAuthAttemptLimiter.recordFailure(w, r, logger) {
					return
				}
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusForbidden)
				writeBody(logger, w, []byte("Forbidden"))
				return
			}
			globalAuthAttemptLimiter.success(authClient(r))
			next.ServeHTTP(w, r)
			return
		}

		// Session auth for vue calls
		sessionToken, err := r.Cookie("session")
		if err != nil {
			logger.Warn("missing session cookie", "err", err)
			w.WriteHeader(http.StatusForbidden)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte("Forbidden"))
			return
		}
		err = GlobalSessionStorage.Verify(sessionToken.Value)
		if err != nil {
			logger.Warn("session verification failed", "err", err)
			w.WriteHeader(http.StatusForbidden)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte("Forbidden"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
