package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// saveTestBuildStatus saves a build of deploy with status. It reports
// errors with Errorf, so goroutines may call it.
func saveTestBuildStatus(t *testing.T, id int, status ItemStatus) {
	t.Helper()
	data, err := json.Marshal(BuildUpdateData{ID: id, Name: "deploy", Status: status})
	if err == nil {
		err = DB.Update(func(tx *bolt.Tx) error { return tx.Bucket(HistoryBucket).Put(Itob(id), data) })
	}
	if err != nil {
		t.Errorf("save build %d: %v", id, err)
	}
}

// waitTime runs waitForBuild for a request with ctx and returns how long it
// took
func waitTime(t *testing.T, ctx context.Context, id, seconds int) time.Duration {
	t.Helper()
	started := time.Now()
	req := httptest.NewRequest("GET", "/api/build/1", nil).WithContext(ctx)
	build, err := waitForBuild(req, id, seconds)
	if err != nil || build == nil || build.ID != id {
		t.Fatalf("waitForBuild = %+v, %v; want build %d", build, err, id)
	}
	return time.Since(started)
}

func TestWaitForBuild(t *testing.T) {
	newTestBuildEnv(t)
	oldQueue, oldEnd := GlobalQueue, buildWaitsEnd
	// No queue: only the status says whether a build ended
	GlobalQueue = nil
	t.Cleanup(func() { GlobalQueue, buildWaitsEnd = oldQueue, oldEnd })
	buildWaitsEnd = newStopSignal()

	saveTestBuildStatus(t, 1, StatusFinished)
	if took := waitTime(t, context.Background(), 1, 5); took > time.Second {
		t.Errorf("an ended build waited %v", took)
	}

	saveTestBuildStatus(t, 2, StatusRunning)
	if took := waitTime(t, context.Background(), 2, 1); took < time.Second || took > 3*time.Second {
		t.Errorf("a running build waited %v, want the 1 s asked for", took)
	}

	// It answers as soon as the build ends
	saveTestBuildStatus(t, 3, StatusRunning)
	go func() {
		time.Sleep(200 * time.Millisecond)
		saveTestBuildStatus(t, 3, StatusFailed)
		buildChanges.notify(3)
	}()
	if took := waitTime(t, context.Background(), 3, 10); took > 2*time.Second {
		t.Errorf("waited %v after the build ended", took)
	}

	// A client that goes away stops the wait
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if took := waitTime(t, ctx, 2, 10); took > 2*time.Second {
		t.Errorf("waited %v after the client went away", took)
	}

	// So does the server's shutdown
	go func() {
		time.Sleep(200 * time.Millisecond)
		buildWaitsEnd.stop()
	}()
	if took := waitTime(t, context.Background(), 2, 10); took > 2*time.Second {
		t.Errorf("waited %v after the shutdown began", took)
	}
}

func TestBuildWaitParameter(t *testing.T) {
	newAPIKeyTestEnv(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }
	for _, target := range []string{"/api/build/1?wait=301", "/api/build/1?wait=x", "/api/build/1/summary?wait=-1"} {
		if rec := apiCall("GET", target, nil, nil, basicAuth); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", target, rec.Code)
		}
	}
	// Build 1 has ended, so waiting returns it at once
	started := time.Now()
	if rec := apiCall("GET", "/api/build/1?wait=30", nil, nil, basicAuth); rec.Code != http.StatusOK || time.Since(started) > time.Second {
		t.Errorf("wait for an ended build: %d after %v", rec.Code, time.Since(started))
	}
	if rec := apiCall("GET", "/api/build/99?wait=30", nil, nil, basicAuth); rec.Code != http.StatusNotFound {
		t.Errorf("wait for an unknown build: %d, want 404", rec.Code)
	}
}

// A build's update wakes only those waiting for that build, so waiting
// agents don't each read their build again on every other build's update;
// signals nobody holds any more are dropped
func TestBuildSignalsWakeOnlyTheirBuild(t *testing.T) {
	s := newBuildSignals()
	two, releaseTwo := s.watch(2)
	three, releaseThree := s.watch(3)
	alsoThree, releaseAlsoThree := s.watch(3)
	s.notify(3)
	select {
	case <-two:
		t.Error("build 3's update woke a wait for build 2")
	default:
	}
	for i, ch := range []<-chan struct{}{three, alsoThree} {
		select {
		case <-ch:
		default:
			t.Errorf("build 3's update didn't wake wait %d for it", i)
		}
	}
	// A wait that starts after the update waits for the next one
	again, releaseAgain := s.watch(3)
	select {
	case <-again:
		t.Error("a new wait for build 3 woke at once")
	default:
	}
	for _, release := range []func(){releaseTwo, releaseThree, releaseAlsoThree, releaseAgain} {
		release()
	}
	if len(s.waiting) != 0 {
		t.Errorf("signals left after every wait ended: %v", s.waiting)
	}
}

// TestWaitForBuildHooks is a regression test: a build's status is final
// before its on_finished and finally tasks run and its artifacts are
// collected, and a wait answered then, with duration 0, no artifacts and
// the hooks still running
func TestWaitForBuildHooks(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "wait-hooks", Artifacts: []string{"report.txt"}, Tasks: []*Task{
		{ID: 0, Command: "echo ok > report.txt", Kind: KindMain},
		{ID: 1, Command: "sleep 0.5", Kind: StatusFinished},
		{ID: 2, Command: "sleep 0.5", Kind: "finally"},
	}})
	oldEnd := buildWaitsEnd
	buildWaitsEnd = newStopSignal()
	t.Cleanup(func() { buildWaitsEnd = oldEnd })

	done := startBuild(build)
	got, err := waitForBuild(httptest.NewRequest("GET", "/api/build/1", nil), build.ID, 10)
	if err != nil {
		t.Fatalf("waitForBuild: %v", err)
	}
	if got.Status != StatusFinished || got.Duration == 0 || len(got.BuildArtifacts) != 1 {
		t.Errorf("wait answered %s with duration %v and %d artifacts, want finished with its duration and 1 artifact",
			got.Status, got.Duration, len(got.BuildArtifacts))
	}
	for _, task := range got.Tasks {
		if task.Status != StatusFinished {
			t.Errorf("task %d (%s) is %q, want finished", task.ID, task.Kind, task.Status)
		}
	}
	waitDone(t, done, 10*time.Second)
}
