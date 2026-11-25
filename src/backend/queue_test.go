package main

import (
	"slices"
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
// regression test above for FlushLogs.
func TestQueueFlushLogsDoesNotBlockWithoutARunningTask(t *testing.T) {
	build := &Build{ID: 2}
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

// The status lists the queued builds in the order they start: a job with a
// higher priority goes before those queued earlier
func TestQueueStatusListsQueuedBuildsInOrder(t *testing.T) {
	q := &Queue{}
	for _, b := range []*Build{
		{ID: 1, Job: &Job{Name: "a"}},
		{ID: 2, Job: &Job{Name: "b"}},
		{ID: 3, Job: &Job{Name: "urgent", Priority: 5}},
	} {
		q.Add(b)
	}
	if got := q.Status().QueuedIDs; !slices.Equal(got, []int{3, 1, 2}) {
		t.Errorf("QueuedIDs = %v, want [3 1 2]", got)
	}
	if got := (&Queue{}).Status().QueuedIDs; got == nil || len(got) != 0 {
		t.Errorf("empty queue: QueuedIDs = %#v, want []", got)
	}
}

// A burst of queue changes starts one status sender, which takes the
// status when it sends, not one sender each
func TestQueueChangesShareOneStatusSender(t *testing.T) {
	old := WSHub
	hub := &Hub{broadcast: make(chan *MsgBroadcast)}
	WSHub = hub
	t.Cleanup(func() { WSHub = old })
	q := &Queue{}
	for range 100 {
		q.queueChanged()
	}
	<-hub.broadcast
	select {
	case <-hub.broadcast:
		t.Error("a second status was sent for the same burst")
	case <-time.After(200 * time.Millisecond):
	}
	// A change after the status was taken is sent too
	q.queueChanged()
	select {
	case <-hub.broadcast:
	case <-time.After(2 * time.Second):
		t.Error("a change after the burst sent no status")
	}
}
