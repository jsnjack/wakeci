package main

import (
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// HandleVueResources serves the embedded frontend with h: Vite's static
// files, and the app's index.html for every other path
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
		// Static files are Vite's chunks under /assets/ and the files at the
		// top level (favicon.ico), all with a dot in their name; every other
		// path is a page of the app, which may hold a dot too (a job named
		// deploy.prod). /assets/ itself would be a directory listing
		static := strings.Contains(path.Base(r.URL.Path), ".") &&
			(strings.HasPrefix(r.URL.Path, "/assets/") || strings.Count(r.URL.Path, "/") == 1)
		switch {
		case static && strings.Contains(r.URL.Path, "/."):
			// all:assets embeds hidden files too, and none is the app's
			http.NotFound(w, r)
			return
		case static:
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
		// The job summary isn't redacted on disk: the API serves it redacted
		if clean := path.Clean("/" + r2.URL.Path); strings.Count(clean, "/") == 2 && strings.HasSuffix(clean, "/"+jobSummaryFile) {
			http.NotFound(w, r)
			return
		}
		logger.Debug("storage rewrite", "from", r.URL.Path, "to", r2.URL.Path)
		h.ServeHTTP(w, r2)
	})
}

// wakespacePath returns the path in the wakespace dir that a storage request
// asks for
func wakespacePath(r *http.Request) string {
	return strings.TrimPrefix(r.URL.Path, "/storage/build/")
}
