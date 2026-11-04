package main

import (
	"os"
	"path/filepath"
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
				b.requestAbort(StatusTimedOut)
				b.requestAbort(StatusAborted)
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
				b.requestAbort(StatusAborted)
				return s
			},
			wantReason: StatusAborted,
			wantSame:   true,
		},
		{
			name: "next scope after an abort is fresh",
			run: func(b *Build) *abortScope {
				b.requestAbort(StatusAborted)
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
