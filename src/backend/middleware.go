package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	bolt "github.com/etcd-io/bbolt"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

// HandlerLogger is a special type for loggers per request
type HandlerLogger string

// HL is a handle logger
const HL HandlerLogger = "logger"

// LogMi is a middleware that creates a new logger per request and logs total time that took to process a request
func LogMi(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		startTime := time.Now()

		// Take the context out from the request
		ctx := r.Context()

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
		ctx = context.WithValue(ctx, HL, handlerLogger)

		// Get new http.Request with the new context
		r = r.WithContext(ctx)

		// Call actuall handler
		next(w, r, ps)

		defer func() {
			duration := time.Now().Sub(startTime)
			handlerLogger.Printf("%s %s [took %s]\n", r.Method, r.URL, duration)
		}()
	})
}

// CORSMi adds CORS headers
func CORSMi(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Call actuall handler
		next(w, r, ps)
		origin := "*"
		if Config.Hostname != "" {
			origin = "https://" + Config.Hostname
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Max-Age", "86400")
	})
}

// AuthMi checks user credentials
func AuthMi(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
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

			err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
			if err != nil {
				logger.Println(err)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next(w, r, ps)
			return
		}

		// Session auth for vue calls
		sessionToken, err := r.Cookie("session")
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		err = S.Verify(sessionToken.Value)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r, ps)
	})
}

// InternalAuthMi requires calls to be made from localhost only
func InternalAuthMi(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
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
		next(w, r, ps)
	})
}

// VueResourcesMi checks if path needs to be stripped out before serving the location
func VueResourcesMi(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*log.Logger)
		if !ok {
			logger = Logger
		}
		// First check if it is any of API or AUTH calls
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			w.WriteHeader(http.StatusNotFound)
			logger.Printf("vue 404 %s\n", r.URL.Path)
			return
		}
		// Static file or root address
		if strings.Contains(r.URL.Path, ".") || r.URL.Path == "/" {
			logger.Printf("vue GET %s\n", r.URL.Path)
			h.ServeHTTP(w, r)
			return
		}
		// Most likely this is request to one of the dynamic URLs used by frontend,
		// serve index.html (/) in this case
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = "/"
		logger.Printf("vue %s --> %s\n", r.URL.Path, r2.URL.Path)
		h.ServeHTTP(w, r2)
	})
}

// WakespaceResourceMi serves content of wakespace/ dir
func WakespaceResourceMi(h http.Handler) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		logger, ok := r.Context().Value(HL).(*log.Logger)
		if !ok {
			logger = Logger
		}

		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/storage/build/")
		logger.Printf("storage %s --> %s\n", r.URL.Path, r2.URL.Path)
		h.ServeHTTP(w, r2)
	})
}
