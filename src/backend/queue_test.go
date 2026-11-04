package main

import (
	"testing"
	"time"
)

// TestQueueAbortDoesNotBlockWithoutARunningTask is a regression test for a
// deadlock where Abort held q.mutex while sending on abortedChannel, which is
// only received while a task is actively executing. A build between tasks
// (or one that hasn't started/has finished) has no listener, so the send -
// and every other Queue method waiting on q.mutex - would block forever.
func TestQueueAbortDoesNotBlockWithoutARunningTask(t *testing.T) {
	build := &Build{ID: 1}
	q := &Queue{running: []*Build{build}}

	done := make(chan error, 1)
	go func() {
		done <- q.Abort(1, StatusAborted)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Abort blocked instead of returning when no task was listening")
	}

	// The mutex must not still be held: every other Queue method must work.
	verifyDone := make(chan bool, 1)
	go func() { verifyDone <- q.Verify(1) }()
	select {
	case ok := <-verifyDone:
		if !ok {
			t.Error("expected build 1 to still be verifiable after Abort")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("q.mutex is still held after Abort returned")
	}
}

// TestQueueFlushLogsDoesNotBlockWithoutARunningTask mirrors the Abort
// regression test above for flushChannel.
func TestQueueFlushLogsDoesNotBlockWithoutARunningTask(t *testing.T) {
	build := &Build{
		ID:           2,
		flushChannel: make(chan bool),
	}
	q := &Queue{running: []*Build{build}}

	done := make(chan error, 1)
	go func() {
		done <- q.FlushLogs(2)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FlushLogs blocked instead of returning when no task was listening")
	}

	verifyDone := make(chan bool, 1)
	go func() { verifyDone <- q.Verify(2) }()
	select {
	case ok := <-verifyDone:
		if !ok {
			t.Error("expected build 2 to still be verifiable after FlushLogs")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("q.mutex is still held after FlushLogs returned")
	}
}

// TestQueueAbortDeliversToRunningTask verifies that a running build gets the
// abort, and keeps it until the build checks it: Abort used to drop the
// signal when no task was receiving at that moment
func TestQueueAbortDeliversToRunningTask(t *testing.T) {
	build := &Build{ID: 3}
	q := &Queue{running: []*Build{build}}
	abort := build.currentAbort()

	if err := q.Abort(3, StatusAborted); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A later abort, e.g. the timeout, keeps the first reason
	if err := q.Abort(3, StatusTimedOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-abort.c:
	case <-time.After(2 * time.Second):
		t.Fatal("abort signal was not delivered to the running build")
	}
	if reason := abort.stopped(); reason != StatusAborted {
		t.Errorf("expected reason %q, got %q", StatusAborted, reason)
	}
}

func TestQueueAbortReturnsErrorForUnknownBuild(t *testing.T) {
	q := &Queue{}
	if err := q.Abort(99, StatusAborted); err == nil {
		t.Error("expected an error for an unknown build id")
	}
}

// TestQueueTakeWhileAborting is a regression test for a data race, caught
// with -race: Take and TakeNow logged the queue lengths after releasing
// q.mutex, while Remove and Abort change the queue under it
func TestQueueTakeWhileAborting(t *testing.T) {
	newTestBuildEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	for i := 1; i <= 20; i++ {
		GlobalQueue.Add(&Build{ID: i, Job: &Job{Name: "take-race"}, Logger: L})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			GlobalQueue.Take()
			if err := GlobalQueue.TakeNow(1000); err == nil {
				t.Error("TakeNow of an unknown build: want an error")
			}
		}
	}()
	for i := 1; i <= 20; i++ {
		GlobalQueue.Remove(i)
	}
	<-done
}
