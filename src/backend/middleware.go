package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

// HandlerLogger is a special type for loggers per request
type HandlerLogger string

// HL is a handle logger
const HL HandlerLogger = "logger"

// LogMi is a middleware that creates a new logger per request and logs total time that took to process a request
func LogMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		logID := GenerateRandomString(5)

		// Get IP address of a user
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			Logger.Println(err)
			host = r.RemoteAddr
		}

		// Get the settings
		handlerLogger := log.New(os.Stdout, "["+logID+" "+host+"] ", log.Lmicroseconds|log.Lshortfile)

		// Get new context with key-value "settings"
		ctx := context.WithValue(r.Context(), HL, handlerLogger)

		// Get new http.Request with the new context
		r = r.WithContext(ctx)

		// Call actuall handler
		next.ServeHTTP(w, r.WithContext(ctx))

		defer func() {
			duration := time.Since(startTime)
			handlerLogger.Printf("%s %s [took %s]\n", r.Method, r.URL, duration)
		}()
	})
}

// CORSMi adds CORS headers
func CORSMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Call actuall handler
		next.ServeHTTP(w, r)
		origin := "*"
		if Config.Hostname != "" {
			origin = "https://" + Config.Hostname
		}
		w.Header().Set("access-control-allow-origin", origin)
		w.Header().Set("access-control-max-age", "86400")
	})
}

// SecurityMi is a middleware which adds security headers
func SecurityMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Call actuall handler
		w.Header().Set("referrer-policy", "no-referrer")
		w.Header().Set("content-security-policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		w.Header().Set("x-content-type-options", "nosniff")
		if Config.Hostname != "" {
			w.Header().Set("strict-transport-security", "max-age=15768000;includeSubdomains")
		}
		next.ServeHTTP(w, r)
	})
}

// AuthMi checks user credentials
func AuthMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*log.Logger)
		if !ok {
			logger = Logger
		}

		// Basic auth for API calls
		_, password, ok := r.BasicAuth()
		if ok {
			var hashedPassword []byte

			err := DB.View(func(tx *bolt.Tx) error {
				b := tx.Bucket([]byte(GlobalBucket))
				hashedPassword = b.Get([]byte("password"))
				return nil
			})

			if err != nil {
				logger.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
			if err != nil {
				logger.Println(err)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Session auth for vue calls
		sessionToken, err := r.Cookie("session")
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		err = GlobalSessionStorage.Verify(sessionToken.Value)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// InternalAuthMi requires calls to be made from localhost only
func InternalAuthMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*log.Logger)
		if !ok {
			logger = Logger
		}

		// Get IP address of a user
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		err = EnsureLocalIP(ip)

		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
