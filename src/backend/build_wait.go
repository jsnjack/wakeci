package main

import (
	"net/http"
	"slices"
	"sync"
	"time"
)

// maxBuildWait is the longest a request waits for a build to end
const maxBuildWait = 300

// buildSignals wakes the requests waiting for a build when that build is
// saved, and only those, so many waiting agents don't each read their build
// again on every other build's update
type buildSignals struct {
	mu sync.Mutex
	// waiting has a signal for each build someone waits for
	waiting map[int]*buildSignal
}

// buildSignal is closed at the build's next change
type buildSignal struct {
	ch chan struct{}
	// watchers counts who still holds it, so the last one removes it
	watchers int
}

func newBuildSignals() *buildSignals {
	return &buildSignals{waiting: map[int]*buildSignal{}}
}

// watch returns a channel that is closed at build id's next change, and
// release, which the watcher calls when it no longer waits
func (s *buildSignals) watch(id int) (<-chan struct{}, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	signal := s.waiting[id]
	if signal == nil {
		signal = &buildSignal{ch: make(chan struct{})}
		s.waiting[id] = signal
	}
	signal.watchers++
	return signal.ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		signal.watchers--
		if signal.watchers == 0 && s.waiting[id] == signal {
			delete(s.waiting, id)
		}
	}
}

// notify wakes those waiting for build id
func (s *buildSignals) notify(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if signal := s.waiting[id]; signal != nil {
		close(signal.ch)
		delete(s.waiting, id)
	}
}

// buildChanges wakes the requests that wait for a build when it is saved
var buildChanges = newBuildSignals()

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

// waitForBuild reads build id and, with seconds, waits until it has ended,
// the seconds have passed, the client went away or the server shuts down.
// It returns the build as it read it last.
func waitForBuild(r *http.Request, id int, seconds int) (*BuildUpdateData, error) {
	var timeout <-chan time.Time
	if seconds > 0 {
		timer := time.NewTimer(time.Duration(seconds) * time.Second)
		defer timer.Stop()
		timeout = timer.C
	}
	for {
		// Before reading, so a change between the read and the wait wakes it
		changed, release := buildChanges.watch(id)
		build, err := readBuildStatus(id)
		if err != nil || seconds <= 0 || isBuildDone(build) {
			release()
			return build, err
		}
		select {
		case <-changed:
			release()
		case <-timeout:
			release()
			return build, nil
		case <-r.Context().Done():
			release()
			return build, nil
		case <-buildWaitsEnd.ch:
			release()
			return build, nil
		}
	}
}
