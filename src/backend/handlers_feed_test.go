package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// The feed only reads while it scans, so a key reading the feed never makes
// build updates wait for the database's write lock
func TestFeedScanDoesNotTakeWriteLock(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"read"}, "scope": {"read"}, "job": {"test_api"}})
	// Recording the key's first use writes, at most once a minute
	if rec := keyRequest(router, "GET", "/api/feed", token, nil); rec.Code != http.StatusOK {
		t.Fatalf("first request: status = %d", rec.Code)
	}

	held := make(chan struct{})
	release := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- DB.Update(func(*bolt.Tx) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	t.Cleanup(func() {
		close(release)
		if err := <-writerDone; err != nil {
			t.Errorf("writer: %v", err)
		}
	})

	done := make(chan int, 1)
	go func() {
		done <- keyRequest(router, "GET", "/api/feed", token, nil).Code
	}()
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Errorf("status = %d, want 200", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the feed waited for the write lock")
	}
}

// A build the history has as running but the queue doesn't know, e.g. after
// a restart, is shown and saved as aborted
func TestFeedMarksStaleBuildsAborted(t *testing.T) {
	newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	data, err := json.Marshal(BuildUpdateData{ID: 3, Name: "deploy", Status: StatusRunning})
	if err != nil {
		t.Fatalf("marshal build: %v", err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(HistoryBucket).Put(Itob(3), data)
	}); err != nil {
		t.Fatalf("save build: %v", err)
	}

	rec := httptest.NewRecorder()
	HandleFeedView(rec, httptest.NewRequest("GET", "/api/feed", nil))
	var builds []BuildUpdateData
	if err := json.Unmarshal(rec.Body.Bytes(), &builds); err != nil {
		t.Fatalf("decode feed %s: %v", rec.Body.String(), err)
	}
	if len(builds) == 0 || builds[0].ID != 3 || builds[0].Status != StatusAborted {
		t.Fatalf("feed = %+v, want build 3 aborted first", builds)
	}

	var stored BuildUpdateData
	if err := DB.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(HistoryBucket).Get(Itob(3)), &stored)
	}); err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.Status != StatusAborted {
		t.Errorf("stored status = %q, want %q", stored.Status, StatusAborted)
	}
}

// A build that changed between the feed's scan and its write keeps its new
// status
func TestMarkStaleBuildsAbortedRechecks(t *testing.T) {
	newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	// Build 1 finished after the scan saw it running
	if err := markStaleBuildsAborted(L, []int{1}); err != nil {
		t.Fatalf("markStaleBuildsAborted: %v", err)
	}
	var stored BuildUpdateData
	if err := DB.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(HistoryBucket).Get(Itob(1)), &stored)
	}); err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.Status != StatusFinished {
		t.Errorf("status = %q, want %q", stored.Status, StatusFinished)
	}
}

// The feed's search is q; filter, its old name, still works when q is absent
func TestFeedQuery(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	for id := 3; id <= 6; id++ {
		addTestBuild(t, id, "deploy")
	}
	token := createTestKey(t, url.Values{"name": {"read"}, "scope": {"read"}, "job": {"deploy"}})

	tests := []struct {
		target string
		want   []int
	}{
		{"/api/feed?q=id:4", []int{4}},
		{"/api/feed?filter=id:4", []int{4}},
		{"/api/feed?q=id:5&filter=id:4", []int{5}},
		// An empty q is no search, not a reason to read filter
		{"/api/feed?q=&filter=id:4", []int{6, 5, 4, 3, 1}},
	}
	for _, tt := range tests {
		rec := keyRequest(router, "GET", tt.target, token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body %q", tt.target, rec.Code, rec.Body.String())
		}
		if got := buildIDs(t, rec.Body.Bytes()); !slices.Equal(got, tt.want) {
			t.Errorf("%s builds = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestFeedCursorAndFilters(t *testing.T) {
	newAPIKeyTestEnv(t)
	// Builds 1 (deploy) and 2 (nightly) exist; odd IDs are deploy, and every
	// fifth build failed
	for id := 3; id <= 40; id++ {
		job := "nightly"
		if id%2 == 1 {
			job = "deploy"
		}
		addTestBuild(t, id, job)
	}
	for id := 5; id <= 40; id += 5 {
		job := "nightly"
		if id%2 == 1 {
			job = "deploy"
		}
		data, err := json.Marshal(BuildUpdateData{ID: id, Name: job, Status: StatusFailed})
		if err != nil {
			t.Fatalf("marshal build: %v", err)
		}
		if err := DB.Update(func(tx *bolt.Tx) error { return tx.Bucket(HistoryBucket).Put(Itob(id), data) }); err != nil {
			t.Fatalf("save build: %v", err)
		}
	}
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }

	tests := []struct {
		query string
		want  []int
	}{
		{"limit=3", []int{40, 39, 38}},
		{"before=10&limit=3", []int{9, 8, 7}},
		{"after=37", []int{40, 39, 38}},
		{"after=30&before=34", []int{33, 32, 31}},
		{"before=1000&limit=2", []int{40, 39}},
		{"after=40", []int{}},
		{"job=deploy&limit=3", []int{39, 37, 35}},
		{"job=deploy&job=nightly&limit=2", []int{40, 39}},
		{"job=deplo", []int{}},
		{"status=failed", []int{40, 35, 30, 25, 20, 15, 10, 5}},
		{"status=failed&job=deploy", []int{35, 25, 15, 5}},
		{"status=failed&before=30&limit=2", []int{25, 20}},
		{"offset=2&limit=2", []int{38, 37}},
		{"offset=1&job=deploy&limit=2", []int{37, 35}},
		{"limit=200", nil},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			rec := apiCall("GET", "/api/feed?"+tt.query, nil, nil, basicAuth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body %q", rec.Code, rec.Body.String())
			}
			got := buildIDs(t, rec.Body.Bytes())
			if tt.want == nil {
				if len(got) != 40 {
					t.Errorf("got %d builds, want all 40", len(got))
				}
				return
			}
			if len(tt.want) == 0 && rec.Body.String() != "[]" {
				t.Errorf("body = %q, want []", rec.Body.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("builds = %v, want %v", got, tt.want)
			}
		})
	}

	for _, query := range []string{"limit=0", "limit=201", "before=0", "after=x", "offset=-1", "status=done"} {
		t.Run("invalid "+query, func(t *testing.T) {
			if rec := apiCall("GET", "/api/feed?"+query, nil, nil, basicAuth); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body %q", rec.Code, rec.Body.String())
			}
		})
	}
}

// A poller can ask whether the page changed instead of downloading it again
func TestFeedETag(t *testing.T) {
	newAPIKeyTestEnv(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }
	first := apiCall("GET", "/api/feed?limit=5", nil, nil, basicAuth)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("first request: %d, ETag %q", first.Code, etag)
	}
	ifNoneMatch := func(r *http.Request) {
		basicAuth(r)
		r.Header.Set("If-None-Match", etag)
	}
	if rec := apiCall("GET", "/api/feed?limit=5", nil, nil, ifNoneMatch); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("unchanged page: %d with %d bytes, want 304 and no body", rec.Code, rec.Body.Len())
	}
	addTestBuild(t, 3, "deploy")
	rec := apiCall("GET", "/api/feed?limit=5", nil, nil, ifNoneMatch)
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag {
		t.Errorf("changed page: %d, ETag %q; want 200 and a new ETag", rec.Code, rec.Header().Get("ETag"))
	}
}
