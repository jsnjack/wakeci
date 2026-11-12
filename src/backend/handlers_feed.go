package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// Feed sizes
const (
	defaultFeedLimit = 15
	maxFeedLimit     = 200
)

// feedStatuses are the statuses the feed's status parameter takes
var feedStatuses = []string{StatusPending, StatusRunning, StatusFinished, StatusFailed, StatusAborted, StatusTimedOut}

// feedRequest is what a feed request asks for
type feedRequest struct {
	// offset skips that many matching builds
	offset int
	limit  int
	// before and after bound the build IDs; 0 when not set
	before, after int
	// jobs and statuses, if set, are the only ones shown, by exact value
	jobs, statuses []string
	filter         *FilterRequest
}

func positiveParam(query url.Values, name string, min, max, fallback int) (int, error) {
	value := query.Get(name)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("invalid %s: %q, want %d to %d", name, value, min, max)
	}
	return n, nil
}

// feedRequestFrom reads a feed request from its query parameters
func feedRequestFrom(query url.Values) (*feedRequest, error) {
	f := &feedRequest{jobs: query["job"], statuses: query["status"]}
	var err error
	if f.offset, err = positiveParam(query, "offset", 0, math.MaxInt32, 0); err != nil {
		return nil, err
	}
	if f.limit, err = positiveParam(query, "limit", 1, maxFeedLimit, defaultFeedLimit); err != nil {
		return nil, err
	}
	if f.before, err = positiveParam(query, "before", 1, math.MaxInt32, 0); err != nil {
		return nil, err
	}
	if f.after, err = positiveParam(query, "after", 1, math.MaxInt32, 0); err != nil {
		return nil, err
	}
	for _, status := range f.statuses {
		if !slices.Contains(feedStatuses, status) {
			return nil, fmt.Errorf("invalid status %q, want one of %s", status, strings.Join(feedStatuses, ", "))
		}
	}
	// q is the search; filter, its old name, still works for clients that
	// send it, unless q is there
	search := query.Get("q")
	if !query.Has("q") {
		search = query.Get("filter")
	}
	f.filter = CreateFilterRequest(search)
	return f, nil
}

// matches reports whether the feed shows msg to a request with access
func (f *feedRequest) matches(msg *BuildUpdateData, access *Access) bool {
	return access.Reads(msg.Name) &&
		(len(f.jobs) == 0 || slices.Contains(f.jobs, msg.Name)) &&
		(len(f.statuses) == 0 || slices.Contains(f.statuses, string(msg.Status))) &&
		(f.filter == nil || matchesFilter(msg.ToFilterMatchString(), f.filter))
}

// seekAtOrBefore moves c to the newest build whose ID is id or lower
func seekAtOrBefore(c *bolt.Cursor, id int) ([]byte, []byte) {
	key, value := c.Seek(Itob(id))
	if key == nil {
		return c.Last()
	}
	if int(binary.BigEndian.Uint64(key)) > id {
		return c.Prev()
	}
	return key, value
}

// HandleFeedView returns builds, newest first
// @Summary      Return builds, newest first
// @Description  API key scope: read; a key or a single sign-on user only sees the builds of their jobs. To read the feed incrementally, keep the newest ID you have and ask for after=<it>; page through more with before=<the oldest ID you got>. An unchanged page answers If-None-Match with 304.
// @Tags         feed
// @Produce      json
// @Param        limit    query      integer   false  "How many builds, 1 to 200; 15 if not set"
// @Param        before   query      integer   false  "Only builds with a lower ID"
// @Param        after    query      integer   false  "Only builds with a higher ID"
// @Param        job      query      []string  false  "Only builds of these jobs, by exact name" collectionFormat(multi)
// @Param        status   query      []string  false  "Only builds with these statuses: pending, running, finished, failed, aborted, timed out" collectionFormat(multi)
// @Param        offset   query      integer   false  "Deprecated: skip that many matching builds; page with before=<the oldest ID you got> instead"
// @Param        q        query      string    false  "Space-separated words matched as substrings of id:<id> name:<job> status:<status> and the params, so +name:test_unit also matches test_unit_js. Builds must contain the words prefixed with +, must not contain those prefixed with -, and must contain one of the others. Phrases can be wrapped in single or double quotes"
// @Param        filter   query      string    false  "Deprecated: the old name of `q`, read only when `q` is absent"
// @Success      200      {array}    BuildUpdateData
// @Success      304      {string}   string
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/feed [get]
func HandleFeedView(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	f, err := feedRequestFrom(r.URL.Query())
	if err != nil {
		logger.Warn("invalid feed request", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	// An API key or a single sign-on user only sees the builds of their jobs
	access := accessFromContext(r.Context())
	// Without anything that hides builds, offset can skip IDs instead of
	// reading each build
	narrowed := f.filter != nil || len(f.jobs) > 0 || len(f.statuses) > 0 || (access != nil && !access.Full)

	payload := []*BuildUpdateData{}
	// Builds the history still has as pending or running but the queue no
	// longer knows, e.g. after a restart. They are marked aborted in a
	// separate write, so the scan never holds the database's write lock.
	var stale []int
	err = DB.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(HistoryBucket).Cursor()
		lastK, _ := c.Last()
		if lastK == nil {
			return nil
		}
		start := int(binary.BigEndian.Uint64(lastK))
		if f.before > 0 {
			start = min(start, f.before-1)
		}
		skip := f.offset
		if !narrowed {
			start -= skip
			skip = 0
		}
		for key, v := seekAtOrBefore(c, start); key != nil; key, v = c.Prev() {
			if f.after > 0 && int(binary.BigEndian.Uint64(key)) <= f.after {
				break
			}
			var msg BuildUpdateData
			if err := json.Unmarshal(v, &msg); err != nil {
				logger.Error("unmarshal build history entry", "err", err)
				continue
			}
			if isStaleBuild(&msg) {
				msg.Status = StatusAborted
				stale = append(stale, msg.ID)
			}
			if !f.matches(&msg, access) {
				continue
			}
			if skip > 0 {
				skip--
				continue
			}
			addArtifactURLs(&msg)
			payload = append(payload, &msg)
			if len(payload) >= f.limit {
				break
			}
		}
		return nil
	})
	if err == nil && len(stale) > 0 {
		err = markStaleBuildsAborted(logger, stale)
	}
	if err != nil {
		logger.Error("build feed query", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	writeCachedJSON(w, r, logger, payload)
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
