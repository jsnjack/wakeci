package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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
	// bytes is the size of the body written
	bytes int
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Write(data []byte) (int, error) {
	n, err := rec.ResponseWriter.Write(data)
	rec.bytes += n
	return n, err
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
// access log line (method, path, status, duration, and who made the request
// once the auth middleware knows) unconditionally, since request traffic is
// core operational visibility for an HTTP service.
func LogMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		r, info := withRequestInfo(r)
		handlerLogger := L.With("logID", info.logID, "host", info.ip)

		// Get new context with key-value "settings"
		ctx := context.WithValue(r.Context(), HL, handlerLogger)

		// Get new http.Request with the new context
		r = r.WithContext(ctx)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			attrs := []any{"logID", info.logID, "host", info.ip, "method", r.Method, "path", r.URL.Path}
			if query := loggedQuery(r.URL.Query()); query != "" {
				attrs = append(attrs, "query", query)
			}
			attrs = append(attrs, "status", rec.status, "bytes", rec.bytes, "took", time.Since(startTime))
			attrs = append(attrs, info.who()...)
			if info.forwardedFor != "" {
				attrs = append(attrs, "forwardedFor", info.forwardedFor)
			}
			if ua := r.UserAgent(); ua != "" {
				attrs = append(attrs, "ua", cutLogValue(ua))
			}
			requestLog.Info("request completed", attrs...)
		}()

		// Call actual handler
		next.ServeHTTP(rec, r)
	})
}

// loggedQueryParams are the query parameters the request line shows: those
// that say what a client reads. Others can carry run parameters, and so
// secrets.
var loggedQueryParams = []string{"q", "filter", "offset", "limit", "before", "after", "job", "status"}

// loggedQuery returns the query parameters of loggedQueryParams that are set
func loggedQuery(query url.Values) string {
	shown := url.Values{}
	for _, name := range loggedQueryParams {
		if values, ok := query[name]; ok {
			shown[name] = values
		}
	}
	return cutLogValue(shown.Encode())
}

// maxLogValue is the most of a client's value a log line shows, so a client
// can't make huge lines
const maxLogValue = 256

func cutLogValue(value string) string {
	if len(value) <= maxLogValue {
		return value
	}
	return strings.ToValidUTF8(value[:maxLogValue], "") + "..."
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
				audit(r, "login.basic", "failed", "reason", "read password hash: "+err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				w.Header().Set("Content-Type", "text/plain")
				writeBody(logger, w, []byte("Another triumph"))
				return
			}

			err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
			if err != nil {
				logger.Warn("basic auth failed", "err", err)
				blocked := globalAuthAttemptLimiter.recordFailure(w, r, logger)
				audit(r, "login.basic", "failed", blockedAttrs(blocked)...)
				if blocked {
					return
				}
				refuseCredentials(w, r, logger, "Wrong password")
				return
			}
			globalAuthAttemptLimiter.success(authClient(r))
			setRequestUser(r, "basic", "", "")
			next.ServeHTTP(w, withAccess(r, fullAccess))
			return
		}

		// Session auth for vue calls
		sessionToken, err := r.Cookie("session")
		if err != nil {
			logger.Warn("missing session cookie", "err", err)
			refuseCredentials(w, r, logger, "Not logged in: send an API key as a Bearer token, or the password with Basic auth")
			return
		}
		user, groups, err := GlobalSessionStorage.Lookup(sessionToken.Value)
		if err != nil {
			logger.Warn("session verification failed", "err", err)
			refuseCredentials(w, r, logger, "The session ended; log in again")
			return
		}
		setRequestUser(r, sessionAuth(user), user, "")
		access := fullAccess
		if user != "" {
			// From the groups as they are now, so a change applies at once
			access, err = ssoGroupsAccess(groups)
			if err != nil {
				logger.Error("find the access of sso groups", "err", err)
				writePlainText(w, logger, http.StatusInternalServerError, "Can't read the permissions of your groups")
				return
			}
			if access.Empty() {
				logger.Warn("sso user's groups have no access", "user", user)
				writePlainText(w, logger, http.StatusForbidden, "None of your groups has access")
				return
			}
		}
		next.ServeHTTP(w, withAccess(r, access))
	})
}

// refuseCredentials answers a request whose credentials are missing or wrong
// with 401, so that a client can tell it from a refusal of what it asked for
// (403). Only a request that sent Basic auth gets a Basic challenge: one on
// a page's own requests would make the browser ask for a password itself.
func refuseCredentials(w http.ResponseWriter, r *http.Request, logger *slog.Logger, text string) {
	if _, _, ok := r.BasicAuth(); ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="wakeci", charset="UTF-8"`)
	} else {
		w.Header().Set("WWW-Authenticate", `Bearer realm="wakeci"`)
	}
	writePlainText(w, logger, http.StatusUnauthorized, text)
}

// requestUser returns who made a request: the single sign-on email, or empty
// for the admin password, which alone may change the password and manage
// every API key. AuthMi has checked the credentials already, Basic auth
// before the session cookie, and this follows the same order.
func requestUser(r *http.Request) (string, error) {
	if _, _, ok := r.BasicAuth(); ok {
		return "", nil
	}
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", err
	}
	return GlobalSessionStorage.User(cookie.Value)
}
