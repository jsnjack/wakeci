package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

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
				"method", r.Method, "path", logPath(r), "status", rec.status, "took", time.Since(startTime),
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
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if strings.HasPrefix(r.URL.Path, "/storage/") || strings.HasPrefix(r.URL.Path, "/preview/") {
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Vary", "Cookie, Authorization")
		}
		w.Header().Set("referrer-policy", "no-referrer")
		w.Header().Set("content-security-policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'self'")
		w.Header().Set("x-content-type-options", "nosniff")
		if Config.Hostname != "" {
			w.Header().Set("strict-transport-security", "max-age=15768000;includeSubdomains")
		}
		next.ServeHTTP(w, r)
	})
}

// StorageSecurityMi supplies default headers for API documentation.
func StorageSecurityMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("referrer-policy", "no-referrer")
		w.Header().Set("content-security-policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'none'; form-action 'none'; frame-ancestors 'self'")
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

// AuthMi authenticates the request and attaches its Principal to the
// context, for RequireScope and the handlers. It accepts, in order: an API
// key (bearer token, or basic auth password starting with "wcik_"), the shared
// password via basic auth, and a browser session cookie.
func AuthMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*slog.Logger)
		if !ok {
			logger = L
		}

		// Scoped API keys. A valid key is never held back by the limiter: it
		// is 256 random bits, so blocking it would protect nothing and only
		// let anyone sharing the address lock out automation.
		if token, ok := apiKeyFromRequest(r); ok {
			principal, err := VerifyAPIKey(logger, token, time.Now())
			if errors.Is(err, errInvalidAPIKey) {
				logger.Warn("api key auth failed", "err", err)
				if recordAuthFailure(w, r, logger) {
					return
				}
				writeText(w, logger, http.StatusForbidden, "Forbidden")
				return
			}
			if err != nil {
				logger.Error("verify api key", "err", err)
				writeText(w, logger, http.StatusInternalServerError, "Another triumph")
				return
			}
			next.ServeHTTP(w, withPrincipal(r, principal))
			return
		}

		// Basic auth with the shared password, for API calls
		_, password, ok := r.BasicAuth()
		if ok {
			if Config.Google.Enabled() && password == "admin" {
				writeText(w, logger, http.StatusForbidden, "Forbidden")
				return
			}
			if !passwordLoginEnabled() {
				logger.Warn("basic auth refused", "err", errPasswordDisabled)
				writeText(w, logger, http.StatusForbidden, "Forbidden")
				return
			}
			if rejectRateLimited(w, r, logger) {
				return
			}
			// A copy: it outlives the transaction, in the request context
			hashedPassword, err := readPasswordHash()
			if err != nil {
				logger.Error("read password hash", "err", err)
				writeText(w, logger, http.StatusInternalServerError, "Another triumph")
				return
			}

			err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
			if err != nil {
				logger.Warn("basic auth failed", "err", err)
				if recordAuthFailure(w, r, logger) {
					return
				}
				writeText(w, logger, http.StatusForbidden, "Forbidden")
				return
			}
			r = withPrincipal(r, passwordPrincipal(viaBasic))
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), verifiedPasswordHashKey{}, hashedPassword)))
			return
		}

		// Session auth for vue calls
		sessionToken, err := r.Cookie("session")
		if err != nil {
			logger.Warn("missing session cookie", "err", err)
			writeText(w, logger, http.StatusForbidden, "Forbidden")
			return
		}
		principal, err := GlobalSessionStorage.Verify(sessionToken.Value)
		if err != nil {
			logger.Warn("session verification failed", "err", err)
			writeText(w, logger, http.StatusForbidden, "Forbidden")
			return
		}
		err = checkSessionPrincipal(principal, time.Now())
		if isRevoked(err) {
			logger.Warn("session refused", "err", err)
			writeText(w, logger, http.StatusForbidden, "Forbidden")
			return
		}
		if err != nil {
			logger.Error("check session principal", "err", err)
			writeText(w, logger, http.StatusInternalServerError, "Another triumph")
			return
		}
		next.ServeHTTP(w, withPrincipal(r, principal))
	})
}

const maxRequestBodyBytes = 4 << 20

// RequestSecurityMi bounds and buffers unsafe request bodies before authentication.
func RequestSecurityMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Body != nil {
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
			if closeErr := r.Body.Close(); closeErr != nil {
				L.Log(r.Context(), LevelTrace, "close request body", "err", closeErr)
			}
			if err != nil {
				status := http.StatusBadRequest
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					status = http.StatusRequestEntityTooLarge
				}
				http.Error(w, http.StatusText(status), status)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			r.ContentLength = int64(len(data))
		}
		next.ServeHTTP(w, r)
	})
}
