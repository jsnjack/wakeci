package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

// VueResourcesMi checks if path needs to be stripped out before serving the location
func HandleVueResources(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*log.Logger)
		if !ok {
			logger = Logger
		}
		// First check if it is any of API, AUTH or STORAGE calls. This urls
		// should never reach this point
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"), strings.HasPrefix(r.URL.Path, "/auth/"), strings.HasPrefix(r.URL.Path, "/storage/"):
			w.WriteHeader(http.StatusInternalServerError)
			logger.Printf("vue 500 %s\n", r.URL.Path)
			return
		}

		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		switch {
		case strings.Contains(r.URL.Path, "."):
			// Static file
			r2.URL.Path = "/assets" + r.URL.Path
			w.Header().Set("cache-control", "public, max-age=604800, immutable")
		default:
			// This is "/" request or, most likely, request to one of the dynamic URLs used by frontend,
			// serve index.html (/assets/) in this case
			r2.URL.Path = "/assets/"
		}
		logger.Printf("vue %s --> %s\n", r.URL.Path, r2.URL.Path)
		h.ServeHTTP(w, r2)
	})
}

// WakespaceResourceMi serves content of wakespace/ dir
func HandleWakespaceResource(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
