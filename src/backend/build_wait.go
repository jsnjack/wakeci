package main

import (
	"net/http"
	"slices"
	"sync"
	"time"
)

// maxBuildWait is the longest a request waits for a build to end
const maxBuildWait = 300

// changeSignal wakes everyone waiting for a change: notify closes the
// channel that changed returned and makes a new one
type changeSignal struct {
	mu sync.Mutex
	ch chan struct{}
}

func newChangeSignal() *changeSignal {
	return &changeSignal{ch: make(chan struct{})}
}

// changed returns a channel that is closed at the next change
func (s *changeSignal) changed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ch
}

func (s *changeSignal) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.ch)
	s.ch = make(chan struct{})
}

// buildChanges wakes the requests that wait for a build whenever a build is
// saved
var buildChanges = newChangeSignal()

// stopSignal is closed once, by stop
type stopSignal struct {
	once sync.Once
	ch   chan struct{}
}

func newStopSignal() *stopSignal {
	return &stopSignal{ch: make(chan struct{})}
}

func (s *stopSignal) stop() {
	s.once.Do(func() { close(s.ch) })
}

// buildWaitsEnd stops when the HTTP server shuts down, so waiting requests
// answer at once instead of holding up a restart
var buildWaitsEnd = newStopSignal()

// buildDoneStatuses are the statuses of a build that has ended
var buildDoneStatuses = []ItemStatus{StatusFinished, StatusFailed, StatusAborted, StatusTimedOut, StatusSkipped}

// isBuildDone reports whether the build has ended. A build the queue no
// longer knows will never change, so waiting for it ends too.
func isBuildDone(build *BuildUpdateData) bool {
	return slices.Contains(buildDoneStatuses, build.Status) || (GlobalQueue != nil && isStaleBuild(build))
}

// waitForBuild waits until build id has ended, the wait query parameter's
// seconds have passed, the client went away or the server shuts down. It
// returns the error of reading the build, if any; the caller reads it again.
func waitForBuild(r *http.Request, id int, seconds int) error {
	if seconds <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	for {
		// Before reading, so a change between the read and the wait wakes it
		changed := buildChanges.changed()
		build, err := readBuildStatus(id)
		if err != nil {
			return err
		}
		if isBuildDone(build) {
			return nil
		}
		select {
		case <-changed:
		case <-timer.C:
			return nil
		case <-r.Context().Done():
			return nil
		case <-buildWaitsEnd.ch:
			return nil
		}
	}
}
