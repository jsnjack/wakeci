package main

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// VueResourcesMi checks if path needs to be stripped out before serving the location
func HandleVueResources(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*slog.Logger)
		if !ok {
			logger = L
		}
		// First check if it is any of API, AUTH or STORAGE calls. This urls
		// should never reach this point
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"), strings.HasPrefix(r.URL.Path, "/auth/"), strings.HasPrefix(r.URL.Path, "/storage/"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			logger.Error("vue 500", "path", r.URL.Path)
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
		logger.Debug("vue rewrite", "from", r.URL.Path, "to", r2.URL.Path)
		h.ServeHTTP(w, r2)
	})
}

// WakespaceResourceMi serves content of wakespace/ dir
func HandleWakespaceResource(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger, ok := r.Context().Value(HL).(*slog.Logger)
		if !ok {
			logger = L
		}

		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = wakespacePath(r)
		logger.Debug("storage rewrite", "from", r.URL.Path, "to", r2.URL.Path)
		h.ServeHTTP(w, r2)
	})
}

// wakespacePath returns the path in the wakespace dir that a storage request
// asks for
func wakespacePath(r *http.Request) string {
	return strings.TrimPrefix(r.URL.Path, "/storage/build/")
}
