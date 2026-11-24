package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// startedRecordIDs returns the builds saved in StartedBucket
func startedRecordIDs(t *testing.T) []int {
	t.Helper()
	var ids []int
	if err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(StartedBucket).ForEach(func(k, _ []byte) error {
			ids = append(ids, int(binary.BigEndian.Uint64(k)))
			return nil
		})
	}); err != nil {
		t.Fatalf("read started builds: %v", err)
	}
	return ids
}

// historyEntry returns the build's history entry
func historyEntry(t *testing.T, id int) BuildUpdateData {
	t.Helper()
	var msg BuildUpdateData
	if err := DB.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(HistoryBucket).Get(Itob(id)), &msg)
	}); err != nil {
		t.Fatalf("read history of build %d: %v", id, err)
	}
	return msg
}

// interruptBuild saves build as wakeci does while it runs with status, in
// which the done tasks ended and cut runs (-1 for none), and leaves it: what
// a killed wakeci leaves behind
func interruptBuild(t *testing.T, build *Build, status ItemStatus, done []int, cut int, failed *Task) {
	t.Helper()
	build.mutex.Lock()
	build.Status = status
	build.StartedAt = time.Now().Add(-time.Minute)
	build.failedTask = failed
	build.mutex.Unlock()
	for _, id := range done {
		build.taskStarted(build.Job.Tasks[id])
		build.taskEnded(build.Job.Tasks[id], StatusFinished)
	}
	if cut >= 0 {
		build.taskStarted(build.Job.Tasks[cut])
		if err := os.WriteFile(build.taskLogPath(cut), []byte("[ 1s] working\n"), 0600); err != nil {
			t.Fatalf("write task log: %v", err)
		}
	}
	build.BroadcastUpdate()
	if ids := startedRecordIDs(t); !slices.Contains(ids, build.ID) {
		t.Fatalf("started records = %v, want build %d", ids, build.ID)
	}
}

// restartAndFinish restores the builds into a new queue, as wakeci does when
// it starts again, runs the tasks the interrupted ones have left and waits
// until they end
func restartAndFinish(t *testing.T) []*Build {
	t.Helper()
	restarted := &Queue{concurrentBuilds: 1}
	GlobalQueue = restarted
	if err := RestoreQueue(restarted); err != nil {
		t.Fatalf("RestoreQueue: %v", err)
	}
	builds, err := FinishInterruptedBuilds(restarted)
	if err != nil {
		t.Fatalf("FinishInterruptedBuilds: %v", err)
	}
	if err := MarkInterruptedBuilds(); err != nil {
		t.Fatalf("MarkInterruptedBuilds: %v", err)
	}
	for _, build := range builds {
		if status := historyStatus(t, build.ID); status != build.Status {
			t.Errorf("history status of build %d = %q before its tasks ran, want %q kept", build.ID, status, build.Status)
		}
		go build.finishInterrupted()
	}
	for _, build := range builds {
		waitGone(t, build)
	}
	return builds
}

// wakeci was killed while a main task ran. When it starts again, the build
// ends aborted after its on_aborted and finally tasks, which can tell they
// run after a restart. Tasks that ran before don't run again, and no later
// main task runs
func TestInterruptedBuildRunsItsHooks(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{concurrentBuilds: 1})
	job := &Job{
		Name:          "killed",
		DefaultParams: []map[string]string{{"TARGET": "default"}},
		Tasks: []*Task{
			{ID: 0, Name: "Prepare", Command: "echo again >> prepared", Kind: KindMain},
			{ID: 1, Name: "Test", Command: "touch tested", Kind: KindMain},
			{ID: 2, Name: "Report", Command: "touch reported", Kind: KindMain},
			{ID: 3, Command: "touch failed", Kind: StatusFailed},
			{ID: 4, Command: `echo "$WAKE_BUILD_STATUS $WAKE_INTERRUPTED $WAKE_FAILED_TASK_NAME $TARGET" > aborted`, Kind: StatusAborted},
			{ID: 5, Command: "touch finally", Kind: FinalTask},
		},
	}
	// As ReadJob sets them
	for _, task := range job.Tasks {
		task.Status = StatusPending
	}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "killed.yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	build.mutex.Lock()
	build.Params[0]["TARGET"] = "custom"
	build.mutex.Unlock()
	interruptBuild(t, build, StatusRunning, []int{0}, 1, nil)
	// The cut task wrote colours, so the log viewer shows its colour log
	const colourLog = "[ 1s] \x1b[1mworking\x1b[0m\n"
	if err := os.WriteFile(build.taskANSILogPath(1), []byte(colourLog), 0600); err != nil {
		t.Fatal(err)
	}
	// An older wakeci queues every build in QueueBucket again after a
	// rollback, so it would run this one twice
	if ids := queuedRecordIDs(t); len(ids) != 0 {
		t.Fatalf("queued records = %v, want none for a running build", ids)
	}

	builds := restartAndFinish(t)
	if len(builds) != 1 {
		t.Fatalf("finished %d builds, want 1", len(builds))
	}
	ws := build.GetWorkspaceDir()
	aborted, err := os.ReadFile(ws + "aborted")
	if err != nil || string(aborted) != "aborted true Test custom\n" {
		t.Errorf("on_aborted wrote %q, %v; want the status, WAKE_INTERRUPTED, the cut task and the param", aborted, err)
	}
	if _, err := os.Stat(ws + "finally"); err != nil {
		t.Errorf("finally task didn't run: %v", err)
	}
	for _, file := range []string{"prepared", "tested", "reported", "failed"} {
		if _, err := os.Stat(ws + file); err == nil {
			t.Errorf("%s exists: a task ran that shouldn't have", file)
		}
	}
	msg := historyEntry(t, build.ID)
	if msg.Status != StatusAborted || msg.Duration != 0 {
		t.Errorf("history status %q, duration %v; want aborted with an unknown (0) duration", msg.Status, msg.Duration)
	}
	want := map[int]ItemStatus{
		0: StatusFinished, 1: StatusAborted, 2: StatusPending,
		3: StatusPending, 4: StatusFinished, 5: StatusFinished,
	}
	for _, task := range msg.Tasks {
		if task.Status != want[task.ID] {
			t.Errorf("task %d status = %q, want %q", task.ID, task.Status, want[task.ID])
		}
	}
	if ids := startedRecordIDs(t); len(ids) != 0 {
		t.Errorf("started records = %v, want none once the build ended", ids)
	}
	cutLog, err := os.ReadFile(build.taskLogPath(1))
	if err != nil || !strings.HasPrefix(string(cutLog), "[ 1s] working\n") ||
		!strings.Contains(string(cutLog), "> "+interruptedTaskNote+".") {
		t.Errorf("log of the cut task = %q, %v; want its output, then the note", cutLog, err)
	}
	cutColours, err := os.ReadFile(build.taskANSILogPath(1))
	if err != nil || !strings.HasPrefix(string(cutColours), colourLog) ||
		!strings.Contains(string(cutColours), "> "+interruptedTaskNote+".") {
		t.Errorf("colour log of the cut task = %q, %v; want its output, then the note", cutColours, err)
	}
	hookLog, err := os.ReadFile(build.taskLogPath(4))
	if err != nil || !strings.Contains(string(hookLog), "> "+interruptedRunNote+".") {
		t.Errorf("on_aborted log = %q, %v; want the note that it runs after a restart", hookLog, err)
	}
}

// wakeci was killed during the on_failed tasks. The build stays failed, and
// only the on_failed tasks that never started run, then finally
func TestInterruptedBuildFinishesItsOnFailedTasks(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{concurrentBuilds: 1})
	job := &Job{Name: "killed-in-hooks", Tasks: []*Task{
		{ID: 0, Name: "Test", Command: "false", Kind: KindMain},
		{ID: 1, Command: "echo again >> notified", Kind: StatusFailed},
		{ID: 2, Command: "echo again >> half-done", Kind: StatusFailed},
		{ID: 3, Command: `echo "$WAKE_BUILD_STATUS $WAKE_FAILED_TASK_NAME" > status`, Kind: StatusFailed},
		{ID: 4, Command: "touch aborted", Kind: StatusAborted},
		{ID: 5, Command: "touch finally", Kind: FinalTask},
	}}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "killed-in-hooks.yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	interruptBuild(t, build, StatusFailed, []int{0, 1}, 2, job.Tasks[0])

	restartAndFinish(t)
	ws := build.GetWorkspaceDir()
	status, err := os.ReadFile(ws + "status")
	if err != nil || string(status) != "failed Test\n" {
		t.Errorf("the on_failed task left wrote %q, %v; want the failed status and task", status, err)
	}
	if _, err := os.Stat(ws + "finally"); err != nil {
		t.Errorf("finally task didn't run: %v", err)
	}
	for _, file := range []string{"notified", "half-done", "aborted"} {
		if _, err := os.Stat(ws + file); err == nil {
			t.Errorf("%s exists: a task ran that shouldn't have", file)
		}
	}
	if status := historyStatus(t, build.ID); status != StatusFailed {
		t.Errorf("history status = %q, want %q kept", status, StatusFailed)
	}
}

// The started record exists from when a build leaves the queue until it
// ends, and never next to the queue record
func TestStartedRecordFollowsBuild(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{concurrentBuilds: 1})
	build := queueTestBuild(t, &Job{Name: "follow", Tasks: []*Task{
		{ID: 0, Command: "until [ -e go ]; do sleep 0.01; done", Kind: KindMain},
	}})
	build.BroadcastUpdate()
	if ids := startedRecordIDs(t); len(ids) != 0 {
		t.Fatalf("started records = %v while queued, want none", ids)
	}
	GlobalQueue.Take()
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Equal(startedRecordIDs(t), []int{build.ID}) {
		if time.Now().After(deadline) {
			t.Fatalf("started records = %v, want build %d once it runs", startedRecordIDs(t), build.ID)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ids := queuedRecordIDs(t); len(ids) != 0 {
		t.Errorf("queued records = %v while running, want none", ids)
	}
	if err := os.WriteFile(build.GetWorkspaceDir()+"go", nil, 0600); err != nil {
		t.Fatal(err)
	}
	waitGone(t, build)
	if ids := startedRecordIDs(t); len(ids) != 0 {
		t.Errorf("started records = %v once the build ended, want none", ids)
	}
}

// An interrupted build holds no concurrency slot: a restored queued build
// starts next to it
func TestInterruptedBuildHoldsNoSlot(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{concurrentBuilds: 1})
	interrupted, err := CreateBuild(&Job{Name: "interrupted", Tasks: []*Task{
		{ID: 0, Command: "true", Kind: KindMain},
	}}, filepath.Join(Config.JobDir, "interrupted.yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	interruptBuild(t, interrupted, StatusRunning, nil, 0, nil)
	queued := queueTestBuild(t, &Job{Name: "queued", Tasks: []*Task{{ID: 0, Command: "true", Kind: KindMain}}})
	queued.BroadcastUpdate()

	restarted := &Queue{concurrentBuilds: 1}
	GlobalQueue = restarted
	if err := RestoreQueue(restarted); err != nil {
		t.Fatalf("RestoreQueue: %v", err)
	}
	builds, err := FinishInterruptedBuilds(restarted)
	if err != nil || len(builds) != 1 {
		t.Fatalf("FinishInterruptedBuilds = %d builds, %v; want 1", len(builds), err)
	}
	restarted.Take()
	if status := restarted.Status(); status.Queued != 0 {
		t.Errorf("queued = %d, want the restored build started next to the interrupted one", status.Queued)
	}
	go builds[0].finishInterrupted()
	for !restarted.idle() {
		time.Sleep(10 * time.Millisecond)
	}
}

// A record that can't be read, or whose build has no history any more, is
// dropped, and the build is marked aborted as before
func TestFinishInterruptedBuildsDropsBrokenRecords(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	entry, err := json.Marshal(BuildUpdateData{ID: 7, Name: "broken", Status: StatusRunning})
	if err != nil {
		t.Fatalf("marshal history entry: %v", err)
	}
	noHistory := `{"job":{"name":"gone","tasks":[{"id":0,"run":"touch ran","kind":"finally","status":"pending"}]},"status":"running"}`
	if err := DB.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(HistoryBucket).Put(Itob(7), entry); err != nil {
			return err
		}
		if err := tx.Bucket(StartedBucket).Put(Itob(7), []byte("not json")); err != nil {
			return err
		}
		return tx.Bucket(StartedBucket).Put(Itob(8), []byte(noHistory))
	}); err != nil {
		t.Fatalf("save broken builds: %v", err)
	}

	builds, err := FinishInterruptedBuilds(GlobalQueue)
	if err != nil {
		t.Fatalf("FinishInterruptedBuilds: %v", err)
	}
	if len(builds) != 0 {
		t.Errorf("restored %d builds, want none", len(builds))
	}
	if ids := startedRecordIDs(t); len(ids) != 0 {
		t.Errorf("started records = %v, want the broken ones deleted", ids)
	}
	if _, err := os.Stat((&Build{ID: 8}).GetWorkspaceDir()); !os.IsNotExist(err) {
		t.Errorf("workspace of the forgotten build: %v, want it not created again", err)
	}
	if err := MarkInterruptedBuilds(); err != nil {
		t.Fatalf("MarkInterruptedBuilds: %v", err)
	}
	if status := historyStatus(t, 7); status != StatusAborted {
		t.Errorf("history status = %q, want %q", status, StatusAborted)
	}
}

// The note goes out numbered after the lines the task's log has, so a
// viewer that has them takes it
func TestInterruptedNoteIsNumberedAfterTheLog(t *testing.T) {
	newTestBuildEnv(t)
	build, err := CreateBuild(&Job{Name: "cut", Tasks: []*Task{{ID: 0, Command: "true", Kind: KindMain}}}, filepath.Join(Config.JobDir, "cut.yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	build.pendingTasksWG.Wait()
	if err := os.WriteFile(build.taskLogPath(0), []byte("[ 1s] a\n[ 2s] b\n[ 3s] c\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldHub := WSHub
	WSHub = &Hub{broadcast: make(chan *MsgBroadcast, 1)}
	t.Cleanup(func() { WSHub = oldHub })
	build.noteInterrupted(build.Job.Tasks[0])
	msg := <-WSHub.broadcast
	if data, ok := msg.Data.(*CommandLogData); !ok || data.ID != 4 {
		t.Errorf("note sent as %+v, want line 4", msg.Data)
	}
}
