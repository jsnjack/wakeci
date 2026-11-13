package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// abortTestBuild creates a build of job that GlobalQueue sees as running
func abortTestBuild(t *testing.T, job *Job) *Build {
	t.Helper()
	newTestBuildEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{concurrentBuilds: 1}
	t.Cleanup(func() { GlobalQueue = oldQueue })

	build, err := CreateBuild(job, filepath.Join(Config.JobDir, job.Name+".yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	GlobalQueue.mutex.Lock()
	GlobalQueue.running = append(GlobalQueue.running, build)
	GlobalQueue.mutex.Unlock()
	return build
}

// startBuild runs the build in the background; the returned channel closes
// when Start returns
func startBuild(build *Build) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		build.Start()
	}()
	return done
}

func waitDone(t *testing.T, done <-chan struct{}, limit time.Duration) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("build still running after %v", limit)
	}
}

// waitForFile waits until a task creates name in the build's workspace
func waitForFile(t *testing.T, build *Build, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if workspaceHas(build, name) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared in the workspace", name)
}

func workspaceHas(build *Build, name string) bool {
	_, err := os.Stat(build.GetWorkspaceDir() + name)
	return err == nil
}

func taskLog(t *testing.T, build *Build, taskID int) string {
	t.Helper()
	log, err := os.ReadFile(build.taskLogPath(taskID))
	if err != nil {
		t.Fatalf("read task %d log: %v", taskID, err)
	}
	return string(log)
}

// TestAbortReachesBuildOutsideRunningCommand is a regression test: Queue.Abort
// dropped the signal unless a task's command was running, so an abort or a
// timeout during setup, a condition, an on_running task or between tasks
// left the build running and it finished
func TestAbortReachesBuildOutsideRunningCommand(t *testing.T) {
	hook := &Task{ID: 9, Name: "Hook", Command: "touch hook", Kind: StatusAborted}
	tests := []struct {
		name string
		job  *Job
		// abort is called once the build is started, unless nil
		abort func(t *testing.T, b *Build)
		// abortFirst sends the abort before the build starts, while no
		// command is running, as between two tasks
		abortFirst bool
		want       ItemStatus
		// failed is the ID of the task expected to stop the build, -1 for none
		failed int
	}{
		{
			name: "no command running",
			job: &Job{Tasks: []*Task{
				{ID: 0, Command: "touch main", Kind: KindMain},
			}},
			abortFirst: true,
			want:       StatusAborted,
			failed:     -1,
		},
		{
			name: "if condition",
			job: &Job{Tasks: []*Task{
				{ID: 0, If: "touch started; sleep 10", Command: "touch main", Kind: KindMain},
			}},
			abort: func(t *testing.T, b *Build) {
				waitForFile(t, b, "started")
				if err := GlobalQueue.Abort(b.ID, StatusAborted); err != nil {
					t.Errorf("Abort: %v", err)
				}
			},
			want:   StatusAborted,
			failed: 0,
		},
		{
			name: "when condition",
			job: &Job{Tasks: []*Task{
				{ID: 0, When: "$(touch started; sleep 10) == x", Command: "touch main", Kind: KindMain},
			}},
			abort: func(t *testing.T, b *Build) {
				waitForFile(t, b, "started")
				if err := GlobalQueue.Abort(b.ID, StatusAborted); err != nil {
					t.Errorf("Abort: %v", err)
				}
			},
			want:   StatusAborted,
			failed: 0,
		},
		{
			name: "timeout in if condition",
			job: &Job{Timeout: "300ms", Tasks: []*Task{
				{ID: 0, If: "sleep 10", Command: "touch main", Kind: KindMain},
			}},
			want:   StatusTimedOut,
			failed: 0,
		},
		{
			name: "on_running task",
			job: &Job{Tasks: []*Task{
				{ID: 0, Command: "touch started; sleep 10", Kind: StatusRunning},
				{ID: 1, Command: "touch on_running", Kind: StatusRunning},
				{ID: 2, Command: "touch main", Kind: KindMain},
			}},
			abort: func(t *testing.T, b *Build) {
				waitForFile(t, b, "started")
				if err := GlobalQueue.Abort(b.ID, StatusAborted); err != nil {
					t.Errorf("Abort: %v", err)
				}
			},
			want:   StatusAborted,
			failed: -1,
		},
		{
			// No main task checks the abort, so Start must check it after them
			name: "on_running task without main tasks",
			job: &Job{Tasks: []*Task{
				{ID: 0, Command: "touch started; sleep 10", Kind: StatusRunning},
				{ID: 1, Command: "touch on_running", Kind: StatusRunning},
			}},
			abort: func(t *testing.T, b *Build) {
				waitForFile(t, b, "started")
				if err := GlobalQueue.Abort(b.ID, StatusAborted); err != nil {
					t.Errorf("Abort: %v", err)
				}
			},
			want:   StatusAborted,
			failed: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.job.Name = "abort-stage"
			tt.job.Tasks = append(tt.job.Tasks, hook)
			build := abortTestBuild(t, tt.job)
			if tt.abortFirst {
				if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
					t.Fatalf("Abort: %v", err)
				}
			}
			start := time.Now()
			done := startBuild(build)
			if tt.abort != nil {
				tt.abort(t, build)
			}
			waitDone(t, done, 5*time.Second)

			if build.Status != tt.want {
				t.Errorf("build status = %q, want %q", build.Status, tt.want)
			}
			if took := time.Since(start); took > 3*time.Second {
				t.Errorf("build took %v, want it stopped promptly", took)
			}
			for _, name := range []string{"main", "on_running"} {
				if workspaceHas(build, name) {
					t.Errorf("%s task ran after the abort", name)
				}
			}
			if !workspaceHas(build, "hook") {
				t.Error("on_aborted task didn't run")
			}
			switch {
			case tt.failed < 0 && build.failedTask != nil:
				t.Errorf("failed task = %d, want none", build.failedTask.ID)
			case tt.failed >= 0 && (build.failedTask == nil || build.failedTask.ID != tt.failed):
				t.Errorf("failed task = %v, want %d", build.failedTask, tt.failed)
			case tt.failed >= 0:
				if got := build.failedTask.Status; got != tt.want {
					t.Errorf("failed task status = %q, want %q", got, tt.want)
				}
				want := "> Aborted by a user."
				if tt.want == StatusTimedOut {
					want = "> Timed out."
				}
				if log := taskLog(t, build, tt.failed); !strings.Contains(log, want) {
					t.Errorf("task log = %q, want %q", log, want)
				}
			}
		})
	}
}

func TestAbortScope(t *testing.T) {
	tests := []struct {
		name string
		// run gets a build without a scope yet
		run        func(b *Build) (before *abortScope)
		wantReason string
		wantSame   bool
	}{
		{
			name: "first reason wins",
			run: func(b *Build) *abortScope {
				b.requestAbort(StatusTimedOut, "", "")
				b.requestAbort(StatusAborted, "", "")
				return b.currentAbort()
			},
			wantReason: StatusTimedOut,
			wantSame:   true,
		},
		{
			name:     "no abort keeps the scope",
			run:      func(b *Build) *abortScope { s := b.currentAbort(); b.nextAbortScope(); return s },
			wantSame: true,
		},
		{
			name: "abort before the next group carries over",
			run: func(b *Build) *abortScope {
				s := b.currentAbort()
				b.nextAbortScope()
				b.requestAbort(StatusAborted, "", "")
				return s
			},
			wantReason: StatusAborted,
			wantSame:   true,
		},
		{
			name: "next scope after an abort is fresh",
			run: func(b *Build) *abortScope {
				b.requestAbort(StatusAborted, "", "")
				s := b.currentAbort()
				b.nextAbortScope()
				return s
			},
			wantSame: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Build{}
			before := tt.run(b)
			now := b.currentAbort()
			if got := now.stopped(); got != tt.wantReason {
				t.Errorf("reason = %q, want %q", got, tt.wantReason)
			}
			if (before == now) != tt.wantSame {
				t.Errorf("same scope = %t, want %t", before == now, tt.wantSame)
			}
		})
	}
}

// TestAbortDuringHooks checks that an abort during the tasks that run after
// the main tasks stops the rest of their group, still runs finally, and
// keeps the build result
func TestAbortDuringHooks(t *testing.T) {
	abort := func(t *testing.T, b *Build, marker string) {
		waitForFile(t, b, marker)
		if err := GlobalQueue.Abort(b.ID, StatusAborted); err != nil {
			t.Errorf("Abort: %v", err)
		}
	}
	tests := []struct {
		name    string
		timeout string
		main    string
		tasks   []*Task
		// trigger stops the build at the right moment, if set
		trigger func(t *testing.T, b *Build)
		want    ItemStatus
		ran     []string
		notRan  []string
	}{
		{
			name: "on_finished",
			main: "true",
			tasks: []*Task{
				{ID: 1, Command: "touch hook1; sleep 10", Kind: StatusFinished},
				{ID: 2, Command: "touch hook2", Kind: StatusFinished},
				{ID: 3, Command: "touch final", Kind: FinalTask},
			},
			trigger: func(t *testing.T, b *Build) { abort(t, b, "hook1") },
			want:    StatusFinished,
			ran:     []string{"final"},
			notRan:  []string{"hook2"},
		},
		{
			name: "on_failed",
			main: "false",
			tasks: []*Task{
				{ID: 1, Command: "touch hook1; sleep 10", Kind: StatusFailed},
				{ID: 2, Command: "touch hook2", Kind: StatusFailed},
				{ID: 3, Command: "touch final", Kind: FinalTask},
			},
			trigger: func(t *testing.T, b *Build) { abort(t, b, "hook1") },
			want:    StatusFailed,
			ran:     []string{"final"},
			notRan:  []string{"hook2"},
		},
		{
			name: "timeout in on_finished",
			// The timer runs until cleanup, so it also covers these tasks
			timeout: "300ms",
			main:    "true",
			tasks: []*Task{
				{ID: 1, Command: "sleep 10", Kind: StatusFinished},
				{ID: 2, Command: "touch hook2", Kind: StatusFinished},
				{ID: 3, Command: "touch final", Kind: FinalTask},
			},
			want:   StatusFinished,
			ran:    []string{"final"},
			notRan: []string{"hook2"},
		},
		{
			name: "second abort in on_aborted",
			main: "touch main; sleep 10",
			tasks: []*Task{
				{ID: 1, Command: "touch hook1; sleep 10", Kind: StatusAborted},
				{ID: 2, Command: "touch hook2", Kind: StatusAborted},
				{ID: 3, Command: "touch final", Kind: FinalTask},
			},
			trigger: func(t *testing.T, b *Build) {
				abort(t, b, "main")
				abort(t, b, "hook1")
			},
			want:   StatusAborted,
			ran:    []string{"final"},
			notRan: []string{"hook2"},
		},
		{
			name: "finally",
			main: "true",
			tasks: []*Task{
				{ID: 1, Command: "touch final1; sleep 10", Kind: FinalTask},
				{ID: 2, Command: "touch final2", Kind: FinalTask},
			},
			trigger: func(t *testing.T, b *Build) { abort(t, b, "final1") },
			want:    StatusFinished,
			notRan:  []string{"final2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{Name: "abort-hooks", Timeout: tt.timeout, Tasks: append(
				[]*Task{{ID: 0, Command: tt.main, Kind: KindMain}}, tt.tasks...,
			)}
			build := abortTestBuild(t, job)
			done := startBuild(build)
			if tt.trigger != nil {
				tt.trigger(t, build)
			}
			waitDone(t, done, 5*time.Second)

			if build.Status != tt.want {
				t.Errorf("build status = %q, want %q", build.Status, tt.want)
			}
			for _, name := range tt.ran {
				if !workspaceHas(build, name) {
					t.Errorf("%s task didn't run", name)
				}
			}
			for _, name := range tt.notRan {
				if workspaceHas(build, name) {
					t.Errorf("%s task ran after the abort", name)
				}
			}
		})
	}
}

// queuedAbortBuild creates a build of job and adds it to a GlobalQueue that
// runs nothing, so it stays queued
func queuedAbortBuild(t *testing.T, job *Job) *Build {
	t.Helper()
	newTestBuildEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, job.Name+".yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	GlobalQueue.Add(build)
	return build
}

// waitGone waits until the build leaves the queue, which Cleanup does last
func waitGone(t *testing.T, build *Build) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for GlobalQueue.Verify(build.ID) {
		if time.Now().After(deadline) {
			t.Fatal("build never left the queue")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAbortQueuedBuild is a regression test: aborting a queued build ran its
// on_aborted tasks in the background but left it queued, so the queue could
// still start it, and a running on_pending task wasn't stopped
func TestAbortQueuedBuild(t *testing.T) {
	build := queuedAbortBuild(t, &Job{Name: "abort-queued", Tasks: []*Task{
		{ID: 0, Command: "touch pending; sleep 10", Kind: StatusPending},
		{ID: 1, Command: "touch main", Kind: KindMain},
		{ID: 2, Command: "echo x >> hook", Kind: StatusAborted},
	}})
	waitForFile(t, build, "pending")

	start := time.Now()
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	// Free a slot: the aborted build must not start
	GlobalQueue.SetConcurrency(1)
	waitGone(t, build)

	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("abort took %v, want the on_pending task stopped promptly", took)
	}
	if build.Status != StatusAborted {
		t.Errorf("build status = %q, want %q", build.Status, StatusAborted)
	}
	if workspaceHas(build, "main") {
		t.Error("main task of an aborted queued build ran")
	}
	// It never started, so it has no duration
	if build.Duration != 0 {
		t.Errorf("duration = %v, want 0 for a build that never started", build.Duration)
	}
	hook, err := os.ReadFile(build.GetWorkspaceDir() + "hook")
	if err != nil || string(hook) != "x\n" {
		t.Errorf("on_aborted output = %q, %v, want it to run once", hook, err)
	}
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err == nil {
		t.Error("Abort after cleanup: want an error for a build that is gone")
	}
}

// TestAbortQueuedBuildTwice checks that a build aborted while queued is still
// found while its on_aborted tasks run, so a second abort can stop them
func TestAbortQueuedBuildTwice(t *testing.T) {
	build := queuedAbortBuild(t, &Job{Name: "abort-queued-twice", Tasks: []*Task{
		{ID: 0, Command: "touch main", Kind: KindMain},
		{ID: 1, Command: "touch hook1; sleep 10", Kind: StatusAborted},
		{ID: 2, Command: "touch hook2", Kind: StatusAborted},
		{ID: 3, Command: "touch final", Kind: FinalTask},
	}})
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	waitForFile(t, build, "hook1")
	if !GlobalQueue.Verify(build.ID) {
		t.Error("Verify = false while the on_aborted tasks run")
	}
	if err := GlobalQueue.FlushLogs(build.ID); err != nil {
		t.Errorf("FlushLogs while the on_aborted tasks run: %v", err)
	}
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("second Abort: %v", err)
	}
	waitGone(t, build)

	if workspaceHas(build, "hook2") {
		t.Error("on_aborted task ran after the second abort")
	}
	if !workspaceHas(build, "final") {
		t.Error("finally task didn't run")
	}
	if workspaceHas(build, "main") {
		t.Error("main task of an aborted queued build ran")
	}
}

// TestAbortBuildThroughAPI aborts a build through the HTTP API while it waits
// in an `if` condition, where the abort used to be dropped
func TestAbortBuildThroughAPI(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "abort-api", Tasks: []*Task{
		{ID: 0, If: "touch started; sleep 10", Command: "touch main", Kind: KindMain},
	}})
	oldSessions := GlobalSessionStorage
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	t.Cleanup(func() { GlobalSessionStorage = oldSessions })

	done := startBuild(build)
	waitForFile(t, build, "started")
	rec := apiCall("POST", "/api/build/"+strconv.Itoa(build.ID)+"/abort", loginAs(t, adminUser), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("abort: status %d, body %q", rec.Code, rec.Body.String())
	}
	waitDone(t, done, 2*time.Second)
	if build.Status != StatusAborted {
		t.Errorf("build status = %q, want %q", build.Status, StatusAborted)
	}
	if workspaceHas(build, "main") {
		t.Error("main task ran after the abort")
	}
}

// TestTimeoutTimerLeavesNoGoroutine is a regression test: the goroutine that
// waited for the job timeout blocked forever once Cleanup stopped the timer
func TestTimeoutTimerLeavesNoGoroutine(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "timer-leak", Timeout: "1h", Tasks: []*Task{
		{ID: 0, Command: "true", Kind: KindMain},
	}})
	build.Start()
	if build.Status != StatusFinished {
		t.Fatalf("build status = %q, want %q", build.Status, StatusFinished)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		buf := make([]byte, 1<<20)
		stacks := string(buf[:runtime.Stack(buf, true)])
		if !strings.Contains(stacks, "SetBuildStatus.func") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("a timeout goroutine is still waiting:\n%s", stacks)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFlushLogsDuringCondition is a regression test: flush requests were read
// only while a command ran, so the log of a task waiting in its `if`
// condition stayed empty
func TestFlushLogsDuringCondition(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "flush-condition", Tasks: []*Task{
		{ID: 0, If: "touch started; sleep 10", Command: "true", Kind: KindMain},
	}})
	done := startBuild(build)
	waitForFile(t, build, "started")

	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := GlobalQueue.FlushLogs(build.ID); err != nil {
			t.Fatalf("FlushLogs: %v", err)
		}
		log, err := os.ReadFile(build.taskLogPath(0))
		if err == nil && strings.Contains(string(log), "> Checking `if` condition") {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("task log = %q after flushing, want the condition line", log)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	waitDone(t, done, 2*time.Second)
}

// TestAbortKillsTheWholeTask is a regression test: when a command ignored
// SIGTERM, the abort's SIGKILL reached only the task's shell, and a timed out
// condition killed only its shell. A child that kept the output open made the
// build wait for it; any other child outlived the build, and wakeci
func TestAbortKillsTheWholeTask(t *testing.T) {
	tests := []struct {
		name string
		task *Task
		// abort aborts the build once the task started; a condition times
		// out by itself
		abort bool
	}{
		{
			name:  "command that ignores SIGTERM",
			task:  &Task{ID: 0, Command: "trap '' TERM; sleep 300 & echo $! > pid; touch started; wait", Kind: KindMain},
			abort: true,
		},
		{
			// The shell ends at once, and the child holds no output open
			name:  "child that ignores SIGTERM after the shell ended",
			task:  &Task{ID: 0, Command: "bash -c \"trap '' TERM; exec sleep 300\" >/dev/null 2>&1 & echo $! > pid; touch started; sleep 100", Kind: KindMain},
			abort: true,
		},
		{
			name: "condition that times out",
			task: &Task{ID: 0, If: "sleep 300 & echo $! > pid; touch started; wait", Command: "true", Kind: KindMain},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := abortTestBuild(t, &Job{Name: "whole-task", Tasks: []*Task{tt.task}})
			done := startBuild(build)
			waitForFile(t, build, "started")
			if tt.abort {
				if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
					t.Fatalf("Abort: %v", err)
				}
			}
			waitDone(t, done, 15*time.Second)
			waitProcessGone(t, build)
		})
	}
}

func TestAbortedBy(t *testing.T) {
	key := &APIKey{ID: "059d6642b7ee", Name: "deploy bot"}
	tests := []struct {
		name               string
		auth, user, apiKey string
		key                *APIKey
		want               string
	}{
		{"password session", "password", "", "", nil, "with the admin password"},
		{"basic auth", "basic", "", "", nil, "with the admin password"},
		{"single sign-on", "sso", "sam@example.com", "", nil, "by sam@example.com"},
		{"single sign-on, line break", "sso", "sam@example.com\n> Finished", "", nil, "by sam@example.com> Finished"},
		{"API key", "apikey", "sam@example.com", key.ID, key, `with API key "deploy bot" (059d6642b7ee)`},
		{"API key, line break in its name", "apikey", "", "abc", &APIKey{ID: "abc", Name: "a\nb"}, `with API key "a\nb" (abc)`},
		{"API key not in the context", "apikey", "", "abc", nil, "with API key abc"},
		{"no one", "", "", "", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := withRequestInfo(httpRequest())
			setRequestUser(r, tt.auth, tt.user, tt.apiKey)
			if tt.key != nil {
				r = r.WithContext(context.WithValue(r.Context(), apiKeyContextKey, tt.key))
			}
			if got := abortedBy(r); got != tt.want {
				t.Errorf("abortedBy = %q, want %q", got, tt.want)
			}
		})
	}
}

// The task log says who aborted the build
func TestAbortByNamesWhoInTheLog(t *testing.T) {
	build := queuedAbortBuild(t, &Job{Name: "abort-by", Tasks: []*Task{
		{ID: 0, Command: "touch pending; sleep 10", Kind: StatusPending},
		{ID: 1, Command: "touch main", Kind: KindMain},
	}})
	waitForFile(t, build, "pending")
	if err := GlobalQueue.AbortBy(build.ID, StatusAborted, "by sam@example.com"); err != nil {
		t.Fatalf("AbortBy: %v", err)
	}
	waitGone(t, build)
	if log := taskLog(t, build, 0); !strings.Contains(log, "> Aborted by sam@example.com.") || strings.Contains(log, "by a user") {
		t.Errorf("task log = %q, want it to name sam@example.com", log)
	}
}
