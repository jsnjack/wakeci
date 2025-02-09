package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
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
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
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
		logger.Println(errMsg)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(errMsg))
		return
	}

	if offset < 0 {
		offset = 1
	}

	filter := CreateFilterRequest(r.URL.Query().Get("filter"))

	var payload []*BuildUpdateData
	err = DB.Update(func(tx *bolt.Tx) error {
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
		if filter == nil {
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
				logger.Println(err)
			} else {
				switch msg.Status {
				case StatusPending, StatusRunning:
					if !GlobalQueue.Verify(msg.ID) {
						msg.Status = StatusAborted
						updatedB, err := json.Marshal(msg)
						if err != nil {
							logger.Println(err)
						}
						b.Put(Itob(msg.ID), updatedB)
					}
				}
				if filter != nil {
					if matchesFilter(fmt.Sprintf("%v %s %s %s", msg.ID, msg.Name, msg.Status, msg.Params), filter) {
						count++
						if count <= offset {
							continue
						}
					} else {
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
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
		return
	}
	payloadB, err := json.Marshal(payload)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(payloadB)
}
