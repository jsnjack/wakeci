package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// writeCachedJSON writes data as JSON with an ETag, and answers a request
// whose If-None-Match has that tag with 304 and no body, so a client that
// polls downloads only what changed. Clients may keep the response but must
// ask again before using it.
func writeCachedJSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		logger.Error("marshal response", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	sum := sha256.Sum256(payload)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payload)
}

// etagMatches reports whether an If-None-Match header names etag. It may
// list several tags, weak ones (W/"...") included, or be *.
func etagMatches(ifNoneMatch, etag string) bool {
	for _, tag := range strings.Split(ifNoneMatch, ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == etag || tag == "*" {
			return true
		}
	}
	return false
}
