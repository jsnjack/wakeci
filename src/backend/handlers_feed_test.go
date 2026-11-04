package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
