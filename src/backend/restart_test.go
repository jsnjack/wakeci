package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// queuedRecordIDs returns the builds saved in QueueBucket
func queuedRecordIDs(t *testing.T) []int {
	t.Helper()
	var ids []int
	if err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(QueueBucket).ForEach(func(k, _ []byte) error {
			ids = append(ids, int(binary.BigEndian.Uint64(k)))
			return nil
		})
	}); err != nil {
		t.Fatalf("read queued builds: %v", err)
	}
	return ids
}

// historyStatus returns the status the history has for the build
func historyStatus(t *testing.T, id int) ItemStatus {
	t.Helper()
	var msg BuildUpdateData
	if err := DB.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(HistoryBucket).Get(Itob(id)), &msg)
	}); err != nil {
		t.Fatalf("read history of build %d: %v", id, err)
	}
	return msg.Status
}

// queueTestBuild creates a build of job and adds it to GlobalQueue
func queueTestBuild(t *testing.T, job *Job) *Build {
	t.Helper()
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, job.Name+".yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	GlobalQueue.Add(build)
	return build
}

func useQueue(t *testing.T, q *Queue) {
	t.Helper()
	oldQueue := GlobalQueue
	GlobalQueue = q
	t.Cleanup(func() { GlobalQueue = oldQueue })
}

// A stopping queue starts no queued build, since the restart would cut it
// short
func TestStoppingQueueHoldsBuilds(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{concurrentBuilds: 1})
	if !GlobalQueue.BeginStop(time.Now().Add(time.Hour), "") {
		t.Fatal("BeginStop: want true for the first stop")
	}
	queueTestBuild(t, &Job{Name: "held", Tasks: []*Task{{ID: 0, Command: "touch main", Kind: KindMain}}})
	GlobalQueue.Take()
	if status := GlobalQueue.Status(); status.Queued != 1 || status.Running != 0 {
		t.Fatalf("status = %+v, want the build held in the queue", status)
	}
}

func TestTakeNow(t *testing.T) {
	tests := []struct {
		name    string
		hold    func(q *Queue)
		wantErr error
	}{
		{"no free slot", func(q *Queue) { q.concurrentBuilds = 0 }, nil},
		{"stopping", func(q *Queue) { q.BeginStop(time.Now().Add(time.Hour), "") }, ErrQueueStopping},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newTestBuildEnv(t)
			useQueue(t, &Queue{concurrentBuilds: 1})
			tt.hold(GlobalQueue)
			build := queueTestBuild(t, &Job{Name: "now", Tasks: []*Task{{ID: 0, Command: "touch main", Kind: KindMain}}})
			err := GlobalQueue.TakeNow(build.ID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("TakeNow = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if !GlobalQueue.Verify(build.ID) || GlobalQueue.Status().Queued != 1 {
					t.Error("a refused build must stay queued")
				}
				return
			}
			waitGone(t, build)
			if !workspaceHas(build, "main") {
				t.Error("the build didn't run")
			}
		})
	}
}

func TestQueuedBuildIsRestoredAfterRestart(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	job := &Job{
		Name:          "restore",
		DefaultParams: []map[string]string{{"TARGET": "default"}},
		Tasks: []*Task{
			{ID: 0, Command: "echo x >> pending", Kind: StatusPending},
			{ID: 1, Command: `echo "$TARGET" > main`, Kind: KindMain},
		},
	}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "restore.yaml"), true)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	build.mutex.Lock()
	build.Params[0]["TARGET"] = "custom"
	build.mutex.Unlock()
	GlobalQueue.Add(build)
	build.BroadcastUpdate()
	build.pendingTasksWG.Wait()

	restarted := &Queue{concurrentBuilds: 1}
	GlobalQueue = restarted
	if err := RestoreQueue(restarted); err != nil {
		t.Fatalf("RestoreQueue: %v", err)
	}
	if status := restarted.Status(); status.Queued != 1 {
		t.Fatalf("queued = %d, want the build queued again", status.Queued)
	}
	restored := restarted.queued[0]
	if restored.ID != build.ID || !restored.startedWithAPIKey || restored.Params[0]["TARGET"] != "custom" {
		t.Errorf("restored build %d, startedWithAPIKey %v, params %v; want build %d started with a key and TARGET=custom",
			restored.ID, restored.startedWithAPIKey, restored.Params, build.ID)
	}
	if status := restored.Job.Tasks[0].Status; status != StatusFinished {
		t.Errorf("on_pending task status = %q, want %q kept from before the restart", status, StatusFinished)
	}

	restarted.Take()
	waitGone(t, restored)
	main, err := os.ReadFile(restored.GetWorkspaceDir() + "main")
	if err != nil || string(main) != "custom\n" {
		t.Errorf("main task wrote %q, %v; want the restored param", main, err)
	}
	pending, err := os.ReadFile(restored.GetWorkspaceDir() + "pending")
	if err != nil || string(pending) != "x\n" {
		t.Errorf("on_pending output = %q, %v; want it to run once, before the restart", pending, err)
	}
	if ids := queuedRecordIDs(t); len(ids) != 0 {
		t.Errorf("queued records = %v, want none once the build started", ids)
	}
	if status := historyStatus(t, restored.ID); status != StatusFinished {
		t.Errorf("history status = %q, want %q", status, StatusFinished)
	}
}

// TestBuildUpdatesSaveInOrder is a regression test: two updates of a build
// could be saved in the other order, and an update taken while the build was
// queued, saved after one taken once it ran, queued it again for the restart
func TestBuildUpdatesSaveInOrder(t *testing.T) {
	build := queuedAbortBuild(t, &Job{Name: "order", Tasks: []*Task{{ID: 0, Command: "true", Kind: KindMain}}})
	build.pendingTasksWG.Wait()
	hub := &Hub{broadcast: make(chan *MsgBroadcast)}
	WSHub = hub
	// Holding the database's write lock keeps an update between being sent
	// and being saved
	tx, err := DB.Begin(true)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	locked := true
	t.Cleanup(func() {
		if locked {
			if err := tx.Rollback(); err != nil {
				t.Errorf("rollback: %v", err)
			}
		}
	})
	broadcast := func() <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			build.BroadcastUpdate()
		}()
		return done
	}

	queuedDone := broadcast()
	<-hub.broadcast
	build.mutex.Lock()
	build.Status = StatusRunning
	build.mutex.Unlock()
	runningDone := broadcast()
	select {
	case <-hub.broadcast:
		t.Error("the running update was sent before the queued one was saved")
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	locked = false
	for queuedDone != nil || runningDone != nil {
		select {
		case <-hub.broadcast:
		case <-queuedDone:
			queuedDone = nil
		case <-runningDone:
			runningDone = nil
		case <-time.After(5 * time.Second):
			t.Fatal("updates never saved")
		}
	}
	if ids := queuedRecordIDs(t); len(ids) != 0 {
		t.Errorf("queued records = %v, want none once the build runs", ids)
	}
	if status := historyStatus(t, build.ID); status != StatusRunning {
		t.Errorf("history status = %q, want %q", status, StatusRunning)
	}
}

func TestQueuedRecordFollowsBuild(t *testing.T) {
	tests := []struct {
		name  string
		leave func(t *testing.T, build *Build)
	}{
		{"started", func(t *testing.T, _ *Build) { GlobalQueue.SetConcurrency(1) }},
		{"aborted while queued", func(t *testing.T, build *Build) {
			if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
				t.Fatalf("Abort: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := queuedAbortBuild(t, &Job{Name: "record", Tasks: []*Task{{ID: 0, Command: "true", Kind: KindMain}}})
			if ids := queuedRecordIDs(t); !slices.Equal(ids, []int{build.ID}) {
				t.Fatalf("queued records = %v, want [%d]", ids, build.ID)
			}
			tt.leave(t, build)
			waitGone(t, build)
			if ids := queuedRecordIDs(t); len(ids) != 0 {
				t.Errorf("queued records = %v, want none once the build left the queue", ids)
			}
		})
	}
}

// useSlowHub replaces WSHub with a hub that waits delay before it takes each
// message, so a build's updates are slow to send
func useSlowHub(t *testing.T, delay time.Duration) {
	t.Helper()
	hub := &Hub{broadcast: make(chan *MsgBroadcast)}
	WSHub = hub
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-time.After(delay):
			case <-stop:
				return
			}
			select {
			case <-hub.broadcast:
			case <-stop:
				return
			}
		}
	}()
}

// TestBuildIsSavedBeforeLeavingTheQueue is a regression test: a build left
// the queue before its final state was saved, so a stop that saw no build
// running could close the database first. The build then stayed running in
// the history, or, aborted while queued, was queued again after the restart
func TestBuildIsSavedBeforeLeavingTheQueue(t *testing.T) {
	job := func(command string) *Job {
		return &Job{Name: "final", Tasks: []*Task{{ID: 0, Command: command, Kind: KindMain}}}
	}
	tests := []struct {
		name    string
		command string
		// abort aborts the build once it runs, queued aborts it while queued
		abort, queued bool
		want          ItemStatus
	}{
		{name: "finished", command: "true", want: StatusFinished},
		{name: "failed", command: "false", want: StatusFailed},
		{name: "aborted while running", command: "touch started; sleep 30", abort: true, want: StatusAborted},
		{name: "aborted while queued", command: "true", queued: true, want: StatusAborted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var build *Build
			if tt.queued {
				build = queuedAbortBuild(t, job(tt.command))
			} else {
				build = abortTestBuild(t, job(tt.command))
			}
			build.pendingTasksWG.Wait()
			// Leaving the queue before the final update is sent then shows
			// as a gap of about this long
			useSlowHub(t, 100*time.Millisecond)

			switch {
			case tt.queued:
				if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
					t.Fatalf("Abort: %v", err)
				}
			case tt.abort:
				startBuild(build)
				waitForFile(t, build, "started")
				if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
					t.Fatalf("Abort: %v", err)
				}
			default:
				startBuild(build)
			}

			waitGone(t, build)
			if status := historyStatus(t, build.ID); status != tt.want {
				t.Errorf("history status = %q once the build left the queue, want %q", status, tt.want)
			}
			if ids := queuedRecordIDs(t); len(ids) != 0 {
				t.Errorf("queued records = %v once the build left the queue, want none", ids)
			}
		})
	}
}

func TestRestoreQueueDropsBrokenRecord(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	entry, err := json.Marshal(BuildUpdateData{ID: 7, Name: "broken", Status: StatusPending})
	if err != nil {
		t.Fatalf("marshal history entry: %v", err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(HistoryBucket).Put(Itob(7), entry); err != nil {
			return err
		}
		return tx.Bucket(QueueBucket).Put(Itob(7), []byte("not json"))
	}); err != nil {
		t.Fatalf("save broken build: %v", err)
	}

	if err := RestoreQueue(GlobalQueue); err != nil {
		t.Fatalf("RestoreQueue: %v", err)
	}
	if queued := GlobalQueue.Status().Queued; queued != 0 {
		t.Errorf("queued = %d, want the broken build dropped", queued)
	}
	if ids := queuedRecordIDs(t); len(ids) != 0 {
		t.Errorf("queued records = %v, want the broken one deleted", ids)
	}
	if err := MarkInterruptedBuilds(); err != nil {
		t.Fatalf("MarkInterruptedBuilds: %v", err)
	}
	if status := historyStatus(t, 7); status != StatusAborted {
		t.Errorf("history status = %q, want %q", status, StatusAborted)
	}
}

func TestMarkInterruptedBuilds(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{queued: []*Build{{ID: 2}}})
	before := map[int]ItemStatus{1: StatusRunning, 2: StatusPending, 3: StatusFinished, 4: StatusPending}
	if err := DB.Update(func(tx *bolt.Tx) error {
		for id, status := range before {
			entry, err := json.Marshal(BuildUpdateData{ID: id, Name: "job", Status: status})
			if err != nil {
				return err
			}
			if err := tx.Bucket(HistoryBucket).Put(Itob(id), entry); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("save history: %v", err)
	}
	if err := MarkInterruptedBuilds(); err != nil {
		t.Fatalf("MarkInterruptedBuilds: %v", err)
	}
	tests := []struct {
		name string
		id   int
		want ItemStatus
	}{
		{"running when wakeci stopped", 1, StatusAborted},
		{"queued again", 2, StatusPending},
		{"finished", 3, StatusFinished},
		{"pending but not queued", 4, StatusAborted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := historyStatus(t, tt.id); got != tt.want {
				t.Errorf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDrainQueue(t *testing.T) {
	tests := []struct {
		name    string
		main    string
		abortIn time.Duration
		signals int
		want    ItemStatus
		// aborted means the stop aborted the build and on_aborted ran
		aborted bool
	}{
		{"build ends before the abort time", "touch started; sleep 0.3", time.Minute, 0, StatusFinished, false},
		{"aborted at the abort time", "touch started; sleep 30", 300 * time.Millisecond, 0, StatusAborted, true},
		{"a signal aborts at once", "touch started; sleep 30", time.Hour, 1, StatusAborted, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := abortTestBuild(t, &Job{Name: "drain", Tasks: []*Task{
				{ID: 0, Command: tt.main, Kind: KindMain},
				{ID: 1, Command: "touch hook", Kind: StatusAborted},
			}})
			done := startBuild(build)
			waitForFile(t, build, "started")
			queued := queueTestBuild(t, &Job{Name: "waiting", Tasks: []*Task{{ID: 0, Command: "true", Kind: KindMain}}})
			interrupt := make(chan os.Signal, 1)
			for range tt.signals {
				interrupt <- os.Interrupt
			}

			GlobalQueue.BeginStop(time.Now().Add(tt.abortIn), "")
			start := time.Now()
			drainQueue(GlobalQueue, interrupt, time.Minute)
			if took := time.Since(start); took > 10*time.Second {
				t.Errorf("drain took %v", took)
			}
			waitDone(t, done, 5*time.Second)

			if build.Status != tt.want {
				t.Errorf("build status = %q, want %q", build.Status, tt.want)
			}
			if got := workspaceHas(build, "hook"); got != tt.aborted {
				t.Errorf("on_aborted ran = %v, want %v", got, tt.aborted)
			}
			if got := strings.Contains(taskLog(t, build, 0), "> Aborted: "+stopNote+"."); got != tt.aborted {
				t.Errorf("task log says the stop aborted it = %v, want %v", got, tt.aborted)
			}
			if !GlobalQueue.Verify(queued.ID) || !slices.Contains(queuedRecordIDs(t), queued.ID) {
				t.Error("the queued build must stay queued and saved for the restart")
			}
		})
	}
}

// A second signal stops the wait for the on_aborted tasks of the builds the
// first one aborted
func TestDrainQueueSecondSignal(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "drain-hooks", Tasks: []*Task{
		{ID: 0, Command: "touch started; sleep 30", Kind: KindMain},
		{ID: 1, Command: "touch hook; sleep 30", Kind: StatusAborted},
	}})
	done := startBuild(build)
	waitForFile(t, build, "started")
	interrupt := make(chan os.Signal, 2)
	interrupt <- os.Interrupt
	interrupt <- os.Interrupt

	GlobalQueue.BeginStop(time.Now().Add(time.Hour), "")
	start := time.Now()
	drainQueue(GlobalQueue, interrupt, time.Hour)
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("drain took %v, want the second signal to end it", took)
	}

	waitForFile(t, build, "hook")
	if GlobalQueue.idle() {
		t.Error("on_aborted should still run")
	}
	// Stops the hook, so it doesn't outlive the test
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	waitDone(t, done, 10*time.Second)
}

// TestDrainQueueAbortsBuildsAbortedWhileQueued is a regression test: the
// stop aborted only running builds, so the on_aborted tasks of a build
// aborted while queued ran on at the abort time, and the drain waited the
// whole hook timeout for them
func TestDrainQueueAbortsBuildsAbortedWhileQueued(t *testing.T) {
	build := queuedAbortBuild(t, &Job{Name: "queued-hooks", Tasks: []*Task{
		{ID: 0, Command: "true", Kind: KindMain},
		{ID: 1, Command: "touch hook; sleep 30", Kind: StatusAborted},
		{ID: 2, Command: "touch final", Kind: FinalTask},
	}})
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	waitForFile(t, build, "hook")

	GlobalQueue.BeginStop(time.Now().Add(300*time.Millisecond), "")
	start := time.Now()
	drainQueue(GlobalQueue, make(chan os.Signal), 20*time.Second)
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("drain took %v, want the stop to abort the on_aborted task", took)
	}
	waitGone(t, build)
	if !strings.Contains(taskLog(t, build, 1), "> Aborted: "+stopNote+".") {
		t.Errorf("on_aborted log = %q, want the stop's note", taskLog(t, build, 1))
	}
	if !workspaceHas(build, "final") {
		t.Error("finally didn't run")
	}
	if status := historyStatus(t, build.ID); status != StatusAborted {
		t.Errorf("history status = %q, want %q", status, StatusAborted)
	}
}

// TestDrainQueueAbortsHooksAfterUserAbort is a regression test: when a user
// aborted a build just before the stop's abort time, the stop's abort found
// the build already aborted and was dropped, so its on_aborted tasks ran
// unaborted and the drain waited the whole hook timeout for them
func TestDrainQueueAbortsHooksAfterUserAbort(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "user-then-stop", Tasks: []*Task{
		// Ends only with the SIGKILL ABORT_TIMEOUT after the abort
		{ID: 0, Command: "trap '' TERM; touch started; exec sleep 30", Kind: KindMain},
		{ID: 1, Command: "touch hook; sleep 30", Kind: StatusAborted},
		{ID: 2, Command: "touch final", Kind: FinalTask},
	}})
	done := startBuild(build)
	waitForFile(t, build, "started")
	if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	GlobalQueue.BeginStop(time.Now().Add(200*time.Millisecond), "")
	start := time.Now()
	drainQueue(GlobalQueue, make(chan os.Signal), 20*time.Second)
	if took := time.Since(start); took > 15*time.Second {
		t.Errorf("drain took %v, want the stop to abort the on_aborted tasks", took)
	}
	waitDone(t, done, 5*time.Second)
	if workspaceHas(build, "hook") {
		t.Error("on_aborted ran after the stop's abort")
	}
	if !workspaceHas(build, "final") {
		t.Error("finally didn't run")
	}
}

// TestStopSignalCancelsRestart is a regression test: a stop signal during an
// update's drain only skipped a wait, and wakeci restarted instead of
// stopping, while systemctl stop or a reboot waited for it to end
func TestStopSignalCancelsRestart(t *testing.T) {
	tests := []struct {
		name string
		// signalFirst sends the signal before the update plans its restart
		signal, signalFirst bool
		want                string
	}{
		{name: "signal during the update's drain", signal: true, want: ""},
		{name: "signal before the update", signal: true, signalFirst: true, want: ""},
		{name: "no signal", want: "2.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newTestBuildEnv(t)
			hub := &Hub{broadcast: make(chan *MsgBroadcast, 8)}
			WSHub = hub
			q := &Queue{}
			signals := make(chan os.Signal, 1)
			defer close(signals)
			forwarded, _ := forwardStopSignals(q, signals)
			send := func() {
				signals <- syscall.SIGTERM
				select {
				case <-forwarded:
				case <-time.After(5 * time.Second):
					t.Fatal("the signal wasn't forwarded")
				}
			}

			if tt.signalFirst {
				send()
			}
			q.BeginStop(time.Now().Add(time.Hour), "2.0.0")
			if tt.signal && !tt.signalFirst {
				send()
			}

			if got := q.Status().RestartVersion; got != tt.want {
				t.Errorf("restart version = %q, want %q", got, tt.want)
			}
			// The banner shows the last status sent
			var last *QueueStatusData
			for len(hub.broadcast) > 0 {
				if msg := <-hub.broadcast; msg.Type == MsgTypeQueueStatus {
					status := msg.Data.(QueueStatusData)
					last = &status
				}
			}
			if last == nil || last.RestartVersion != tt.want {
				t.Errorf("last status sent = %+v, want restart version %q", last, tt.want)
			}
		})
	}
}

// stubbornTask starts a sleep that only a signal ends and saves its PID in
// pid, then creates started in the workspace
const stubbornTask = "sleep 300 & echo $! > pid; touch started; wait"

// waitProcessGone waits until the process whose PID a task saved in pid has
// ended
func waitProcessGone(t *testing.T, build *Build) {
	t.Helper()
	data, err := os.ReadFile(build.GetWorkspaceDir() + "pid")
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse pid %q: %v", data, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		// A zombie has ended too; its new parent reaps it
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil || strings.Contains(string(stat), ") Z ") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still runs", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestWaitQueuedHooksEndsOnSignal checks that a further signal on the way out
// ends the wait for on_pending tasks, as it ends each wait of the drain
func TestWaitQueuedHooksEndsOnSignal(t *testing.T) {
	build := queuedAbortBuild(t, &Job{Name: "pending-wait", Tasks: []*Task{
		{ID: 0, Command: stubbornTask, Kind: StatusPending},
		{ID: 1, Command: "true", Kind: KindMain},
	}})
	waitForFile(t, build, "started")
	interrupt := make(chan os.Signal, 1)
	interrupt <- syscall.SIGTERM

	start := time.Now()
	if GlobalQueue.waitQueuedHooks(20*time.Second, interrupt) {
		t.Error("waitQueuedHooks = true, want false while the on_pending task runs")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("waitQueuedHooks took %v, want the signal to end it", took)
	}
	if !GlobalQueue.stopTasks(stopNote, 10*time.Second) {
		t.Error("stopTasks: the on_pending task still runs")
	}
}

// TestStopTasksEndsLeftoverTasks is a regression test: when wakeci stopped
// waiting for builds (a hook past the hook timeout, another signal, an
// on_pending task past its wait), it exited or re-executed with their tasks
// still running, and nothing waited for them any more
func TestStopTasksEndsLeftoverTasks(t *testing.T) {
	t.Run("hook of an aborted build", func(t *testing.T) {
		build := abortTestBuild(t, &Job{Name: "leftover-hook", Tasks: []*Task{
			{ID: 0, Command: "sleep 30", Kind: KindMain},
			{ID: 1, Command: stubbornTask, Kind: StatusAborted},
			{ID: 2, Command: "touch final", Kind: FinalTask},
		}})
		startBuild(build)
		if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
			t.Fatalf("Abort: %v", err)
		}
		waitForFile(t, build, "started")

		start := time.Now()
		if !GlobalQueue.stopTasks(stopNote, 10*time.Second) {
			t.Fatalf("stopTasks: tasks still ran after %v", time.Since(start))
		}
		waitProcessGone(t, build)
		if !strings.Contains(taskLog(t, build, 1), "> Aborted: "+stopNote+".") {
			t.Errorf("on_aborted log = %q, want the stop's note", taskLog(t, build, 1))
		}
		if workspaceHas(build, "final") {
			t.Error("finally ran after the tasks were stopped")
		}
		if log := taskLog(t, build, 2); !strings.Contains(log, "> Not started: "+stopNote+".") {
			t.Errorf("finally log = %q, want it not started", log)
		}
		if GlobalQueue.Verify(build.ID) {
			t.Error("the build is still in the queue")
		}
		if status := historyStatus(t, build.ID); status != StatusAborted {
			t.Errorf("history status = %q, want %q", status, StatusAborted)
		}
	})

	t.Run("on_pending task of a queued build", func(t *testing.T) {
		build := queuedAbortBuild(t, &Job{Name: "leftover-pending", Tasks: []*Task{
			{ID: 0, Command: stubbornTask, Kind: StatusPending},
			{ID: 1, Command: "true", Kind: KindMain},
		}})
		waitForFile(t, build, "started")

		if !GlobalQueue.stopTasks(stopNote, 10*time.Second) {
			t.Fatal("stopTasks: tasks still ran")
		}
		waitProcessGone(t, build)
		if !GlobalQueue.Verify(build.ID) || !slices.Contains(queuedRecordIDs(t), build.ID) {
			t.Error("the queued build must stay queued and saved for the restart")
		}
	})
}

// TestStopSignalNotedWhenHandlingEnds is a regression test: a signal still
// waiting in the channel when run stopped handling signals could be noted
// after main had already decided to restart
func TestStopSignalNotedWhenHandlingEnds(t *testing.T) {
	newTestBuildEnv(t)
	q := &Queue{}
	q.BeginStop(time.Now().Add(time.Hour), "2.0.0")
	signals := make(chan os.Signal, 1)
	// Holding the queue's lock keeps the signal from being noted
	q.mutex.Lock()
	signals <- syscall.SIGTERM
	_, done := forwardStopSignals(q, signals)
	close(signals)
	select {
	case <-done:
		t.Error("done closed before the signal was noted")
	case <-time.After(200 * time.Millisecond):
	}
	q.mutex.Unlock()
	<-done
	if got := q.Status().RestartVersion; got != "" {
		t.Errorf("restart version = %q once the signals were handled, want none", got)
	}
}

// TestBeginStopAfterStopSignal is a regression test: an update whose
// BeginStop came after a stop signal reported a planned restart, although
// the signal had refused it
func TestBeginStopAfterStopSignal(t *testing.T) {
	newTestBuildEnv(t)
	q := &Queue{}
	q.noteStopSignal(syscall.SIGTERM)
	if q.BeginStop(time.Now().Add(time.Hour), "2.0.0") {
		t.Error("BeginStop = true, want false: the signal refused the restart")
	}
	if status := q.Status(); !status.Stopping || status.RestartVersion != "" {
		t.Errorf("status = %+v, want stopping without a restart", status)
	}
}

func TestBeginStopOnce(t *testing.T) {
	newTestBuildEnv(t)
	q := &Queue{}
	abortAt := time.Now().Add(time.Minute)
	if !q.BeginStop(abortAt, "2.0.0") {
		t.Fatal("first BeginStop: want true")
	}
	if q.BeginStop(time.Now(), "") {
		t.Error("second BeginStop: want false")
	}
	select {
	case <-q.Stopping():
	default:
		t.Error("Stopping channel is not closed")
	}
	status := q.Status()
	if !status.Stopping || status.RestartVersion != "2.0.0" || status.AbortAt == nil || !status.AbortAt.Equal(abortAt) {
		t.Errorf("status = %+v, want the first stop's plan", status)
	}
}

func TestRestoreBuild(t *testing.T) {
	newTestBuildEnv(t)
	startedAt := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	record, err := json.Marshal(queuedBuild{
		Job: &Job{Name: "restored", Tasks: []*Task{
			{ID: 0, Kind: StatusPending, Status: StatusFinished},
			{ID: 1, Kind: StatusPending, Status: StatusRunning},
			{ID: 2, Kind: KindMain, Status: StatusPending},
		}},
		Params: []map[string]string{{"A": "1"}},
		Tasks:  []*TaskStatus{{ID: 0, StartedAt: startedAt, Duration: time.Second}},
	})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	build, err := restoreBuild(5, record)
	if err != nil {
		t.Fatalf("restoreBuild: %v", err)
	}
	tests := []struct {
		name string
		task int
		want ItemStatus
	}{
		{"finished on_pending task", 0, StatusFinished},
		{"on_pending task the restart stopped", 1, StatusAborted},
		{"main task", 2, StatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := build.Job.Tasks[tt.task].Status; got != tt.want {
				t.Errorf("status = %q, want %q", got, tt.want)
			}
		})
	}
	if task := build.Job.Tasks[0]; !task.startedAt.Equal(startedAt) || task.duration != time.Second {
		t.Errorf("on_pending task started %v and took %v, want the saved values", task.startedAt, task.duration)
	}
	if build.ID != 5 || build.Status != StatusPending || build.Params[0]["A"] != "1" {
		t.Errorf("build %d status %q params %v, want build 5 pending with the saved params", build.ID, build.Status, build.Params)
	}
	if _, err := os.Stat(build.GetArtifactsDir()); err != nil {
		t.Errorf("artifacts dir: %v", err)
	}
	if _, err := restoreBuild(6, []byte(`{"params":[]}`)); err == nil {
		t.Error("restoreBuild without a job: want an error")
	}
}
