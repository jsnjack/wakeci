package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	bolt "go.etcd.io/bbolt"
)

// HandleFeedView returns items in current feed - executed and queued jobs
// @Summary      Return information about the latest builds
// @Description  Returns information about 15 latest builds
// @Tags         feed
// @Produce      json
// @Param        offset   query      integer   false  "Skip `offset` latest builds"
// @Param        filter   query      string    false  "Returns only builds which ID, name, params or status contains any of the space-separated words. Requires presence of the prefixed with `+` words. Requires absence of the prefixed with `-` words. Phrases can be wrapped in single or double quotes"
// @Success      200      {array}    BuildUpdateData
// @Failure      400      {string}   string
// @Failure      500      {string}   string
// @Router       /feed/ [get]
func HandleFeedView(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	const pageSize = 15

	offsetS := r.URL.Query().Get("offset")

	// Default value to simplify REST API usage
	if offsetS == "" {
		offsetS = "0"
	}

	offset, err := strconv.Atoi(offsetS)
	if err != nil {
		errMsg := fmt.Sprintf("Invalid offset: %q", offsetS)
		logger.Warn("invalid offset", "offset", offsetS)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(errMsg))
		return
	}

	if offset < 0 {
		offset = 1
	}

	filter := CreateFilterRequest(r.URL.Query().Get("filter"))
	// An API key only sees the builds of its jobs
	apiKey := apiKeyFromContext(r.Context())
	filtered := filter != nil || apiKey != nil

	var payload []*BuildUpdateData
	// Builds the history still has as pending or running but the queue no
	// longer knows, e.g. after a restart. They are marked aborted in a
	// separate write, so the scan never holds the database's write lock.
	var stale []int
	err = DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(HistoryBucket))
		c := b.Cursor()
		count := 0
		// Check what is the last one
		lastK, _ := c.Last()
		if lastK == nil {
			return nil
		}
		// Find starting point
		fromB := make([]byte, 8)
		if !filtered {
			binary.BigEndian.PutUint64(fromB, binary.BigEndian.Uint64(lastK)-uint64(offset))
		} else {
			// If interval is specified, always iterate from the beginning to take
			// into account offset later
			fromB = lastK
		}
		for key, v := c.Seek(fromB); key != nil; key, v = c.Prev() {
			var msg BuildUpdateData
			err := json.Unmarshal(v, &msg)
			if err != nil {
				logger.Error("unmarshal build history entry", "err", err)
			} else {
				if isStaleBuild(&msg) {
					msg.Status = StatusAborted
					stale = append(stale, msg.ID)
				}
				if filtered {
					if (apiKey != nil && !apiKey.CoversJob(msg.Name)) ||
						(filter != nil && !matchesFilter(msg.ToFilterMatchString(), filter)) {
						continue
					}
					count++
					if count <= offset {
						continue
					}
				}
				payload = append(payload, &msg)
				if len(payload) >= pageSize {
					break
				}
			}
		}
		return nil
	})
	if err == nil && len(stale) > 0 {
		err = markStaleBuildsAborted(logger, stale)
	}
	if err != nil {
		logger.Error("build feed query", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	payloadB, err := json.Marshal(payload)
	if err != nil {
		logger.Error("marshal feed payload", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
}

// isStaleBuild reports whether the history has a build as pending or running
// that the queue doesn't know
func isStaleBuild(msg *BuildUpdateData) bool {
	return (msg.Status == StatusPending || msg.Status == StatusRunning) && !GlobalQueue.Verify(msg.ID)
}

// markStaleBuildsAborted saves the builds as aborted if they are still stale,
// since they may have changed after the feed read them
func markStaleBuildsAborted(logger *slog.Logger, ids []int) error {
	return DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(HistoryBucket)
		for _, id := range ids {
			var msg BuildUpdateData
			if err := json.Unmarshal(b.Get(Itob(id)), &msg); err != nil || !isStaleBuild(&msg) {
				continue
			}
			msg.Status = StatusAborted
			updatedB, err := json.Marshal(msg)
			if err != nil {
				logger.Error("marshal build history entry", "build", id, "err", err)
			} else if err := b.Put(Itob(id), updatedB); err != nil {
				logger.Error("save build history entry", "build", id, "err", err)
			}
		}
		return nil
	})
}
