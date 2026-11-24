package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
)

// stopNote is logged in the tasks of the builds a stop aborts
const stopNote = "wakeci is stopping"

// stopHookTimeout is how long builds aborted by a stop may run their
// on_aborted and finally tasks
const stopHookTimeout = 2 * time.Minute

// stopPendingTasksTimeout is how long a stop waits for the on_pending tasks
// of queued builds, which are expected to be quick
const stopPendingTasksTimeout = 30 * time.Second

// stopTasksTimeout is how long wakeci waits, as it exits, for the tasks it
// aborts last (Queue.stopTasks). A command that ignores SIGTERM gets SIGKILL
// after ABORT_TIMEOUT
const stopTasksTimeout = 10 * time.Second

// stopPollInterval is how often a stop checks whether the builds ended
const stopPollInterval = 200 * time.Millisecond

// queuedBuild is what a queued build needs to be queued again after a
// restart, stored in QueueBucket
type queuedBuild struct {
	Job             *Job                `json:"job"`
	Params          []map[string]string `json:"params"`
	UntrustedParams bool                `json:"untrustedParams"`
	// StartedWithAPIKey is the mark under its name in wakeci 2.41 and older,
	// set there for builds API keys started. It is read, so a build queued
	// before an upgrade stays untrusted, and written too, so a rollback
	// keeps the mark
	StartedWithAPIKey bool `json:"startedWithAPIKey,omitempty"`
	// Tasks holds when the on_pending tasks ran, which Job doesn't keep.
	// They don't run again
	Tasks []*TaskStatus `json:"tasks"`
	// QueuedAt and Trigger are the build's, for its stats; zero in records
	// of older versions
	QueuedAt time.Time    `json:"queuedAt,omitzero"`
	Trigger  buildTrigger `json:"trigger,omitzero"`
	// Parent and Root are the build's group, 0 if no build started it
	Parent int `json:"parent,omitempty"`
	Root   int `json:"root,omitempty"`
	// CancelKey is the cancel key the build was queued with, "" for none,
	// so a restore doesn't run cancel_unless again; nil in a record saved
	// before it was queued, or by an older version
	CancelKey *string `json:"cancelKey,omitempty"`
}

// RestoreQueue queues the builds that were queued when wakeci stopped, in
// the order they were queued. Their on_pending tasks don't run again. It
// doesn't start them: call q.Take once the server listens
func RestoreQueue(q *Queue) error {
	records := map[int][]byte{}
	var ids []int
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(QueueBucket).ForEach(func(k, v []byte) error {
			id := int(binary.BigEndian.Uint64(k))
			ids = append(ids, id)
			records[id] = append([]byte(nil), v...)
			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("read queued builds: %w", err)
	}
	restored := 0
	for _, id := range ids {
		build, err := restoreBuild(id, records[id])
		if err != nil {
			// MarkInterruptedBuilds marks it aborted
			L.Error("restore queued build", "build", id, "err", err)
			if err := DB.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(QueueBucket).Delete(Itob(id))
			}); err != nil {
				L.Error("forget queued build", "build", id, "err", err)
			}
			continue
		}
		build.setRestoredKeys()
		q.Add(build)
		build.BroadcastUpdate()
		restored++
	}
	if restored > 0 {
		L.Warn("restored queued builds", "builds", restored)
	}
	return nil
}

// restoreBuild creates the queued build saved in record
func restoreBuild(id int, record []byte) (*Build, error) {
	var saved queuedBuild
	if err := json.Unmarshal(record, &saved); err != nil {
		return nil, fmt.Errorf("decode queued build: %w", err)
	}
	return newRestoredBuild(id, &saved)
}

// newRestoredBuild creates a pending build from saved. A task that ran when
// wakeci stopped is marked aborted
func newRestoredBuild(id int, saved *queuedBuild) (*Build, error) {
	if saved.Job == nil {
		return nil, fmt.Errorf("saved build has no job")
	}
	build := &Build{
		ID:              id,
		Job:             saved.Job,
		Status:          StatusPending,
		abort:           newAbortScope(),
		Params:          saved.Params,
		ETA:             GetJobETA(saved.Job.Name),
		untrustedParams: saved.UntrustedParams || saved.StartedWithAPIKey,
		queuedAt:        saved.QueuedAt,
		trigger:         saved.Trigger,
		group:           buildGroup{Parent: saved.Parent, Root: saved.Root},
		savedCancelKey:  saved.CancelKey,
	}
	build.Logger = L.With("build", id)
	ran := map[int]*TaskStatus{}
	for _, status := range saved.Tasks {
		ran[status.ID] = status
	}
	for _, task := range build.Job.Tasks {
		if status, ok := ran[task.ID]; ok {
			task.startedAt = status.StartedAt
			task.duration = status.Duration
		}
		// The restart stopped it
		if task.Status == StatusRunning {
			task.Status = StatusAborted
		}
	}
	for _, dir := range []string{build.GetWorkspaceDir(), build.GetArtifactsDir()} {
		if err := os.MkdirAll(dir, privateBuildDirMode); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return build, nil
}

// interruptedRunNote starts the log of each task an interrupted build runs
// after the restart
const interruptedRunNote = "Running after a restart: wakeci stopped while this build ran"

// interruptedTaskNote ends the log of the task that ran when wakeci stopped
const interruptedTaskNote = "Aborted: wakeci stopped while this task ran"

// startedBuild is what a build that left the queue needs to run the tasks
// it has left if wakeci stops without ending it, stored in StartedBucket
// until it ends. A bucket of its own: an older wakeci queues every build in
// QueueBucket again, so after a rollback it would run these from the start
type startedBuild struct {
	queuedBuild
	// Status is running, or the final status while the build's on_* and
	// finally tasks run
	Status    ItemStatus `json:"status"`
	StartedAt time.Time  `json:"startedAt,omitzero"`
	// FailedTask is the ID of the main task that stopped the build
	FailedTask *int `json:"failedTask,omitempty"`
}

// FinishInterruptedBuilds adds to q the builds that wakeci stopped without
// ending, as when it was killed, to run the tasks they have left: on_aborted
// and finally for a build that was running; the rest of its on_* tasks and
// finally for one that was running those. Tasks that started before don't
// run again. Call it after RestoreQueue and before MarkInterruptedBuilds,
// which marks aborted the builds it couldn't restore. It doesn't run the
// tasks, which may call wakeci's API: call finishInterrupted on each build
// it returns once the server listens
func FinishInterruptedBuilds(q *Queue) ([]*Build, error) {
	type saved struct {
		id     int
		record []byte
		// The history entry is gone, as after a rollback that cleaned the
		// build up: there is nothing to finish
		forgotten bool
	}
	var records []saved
	err := DB.View(func(tx *bolt.Tx) error {
		history := tx.Bucket(HistoryBucket)
		return tx.Bucket(StartedBucket).ForEach(func(k, v []byte) error {
			records = append(records, saved{
				id:        int(binary.BigEndian.Uint64(k)),
				record:    append([]byte(nil), v...),
				forgotten: history.Get(k) == nil,
			})
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("read started builds: %w", err)
	}
	var builds []*Build
	for _, rec := range records {
		var build *Build
		var err error
		if rec.forgotten {
			// Restoring would create its directories again
			err = errors.New("its history entry is gone")
		} else {
			build, err = restoreStartedBuild(rec.id, rec.record)
		}
		if err != nil {
			// MarkInterruptedBuilds marks it aborted
			L.Error("restore interrupted build", "build", rec.id, "err", err)
			if err := DB.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(StartedBucket).Delete(Itob(rec.id))
			}); err != nil {
				L.Error("forget interrupted build", "build", rec.id, "err", err)
			}
			continue
		}
		// Its hooks hold its concurrency key, as after any abort
		build.setHookKeys()
		q.addInterrupted(build)
		builds = append(builds, build)
	}
	if len(builds) > 0 {
		L.Warn("finishing builds wakeci stopped without ending", "builds", len(builds))
	}
	return builds, nil
}

// restoreStartedBuild creates the interrupted build saved in record, and
// ends the log of the task that ran when wakeci stopped with why it ended
func restoreStartedBuild(id int, record []byte) (*Build, error) {
	var saved startedBuild
	if err := json.Unmarshal(record, &saved); err != nil {
		return nil, fmt.Errorf("decode started build: %w", err)
	}
	// Before newRestoredBuild marks it aborted
	var cut *Task
	if saved.Job != nil {
		for _, task := range saved.Job.Tasks {
			if task.Status == StatusRunning {
				cut = task
				break
			}
		}
	}
	build, err := newRestoredBuild(id, &saved.queuedBuild)
	if err != nil {
		return nil, err
	}
	build.Status = saved.Status
	build.StartedAt = saved.StartedAt
	build.interrupted = true
	switch {
	case saved.FailedTask != nil:
		for _, task := range build.Job.Tasks {
			if task.ID == *saved.FailedTask {
				build.failedTask = task
			}
		}
	// The main task that ran when wakeci stopped is the one that stopped
	// the build, as for an abort
	case saved.Status == StatusRunning && cut != nil && cut.Kind == KindMain:
		build.failedTask = cut
	}
	if cut != nil {
		build.noteInterrupted(cut)
	}
	return build, nil
}

// noteInterrupted ends task's log with why it ended: wakeci stopped while
// it ran
func (b *Build) noteInterrupted(task *Task) {
	file, err := os.OpenFile(b.taskLogPath(task.ID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, privateBuildFileMode)
	if err != nil {
		b.Logger.Error("open task log", "task", task.ID, "err", err)
		return
	}
	log := &taskLogWriter{file: file, w: bufio.NewWriter(file)}
	// Numbered after the lines the log has, as the live log expects
	if lines, err := countFileLines(b.taskLogPath(task.ID)); err == nil {
		log.lines = lines
	}
	// The colour log, which the log viewer shows if the task wrote one,
	// ends with the note too
	ansi, err := os.OpenFile(b.taskANSILogPath(task.ID), os.O_APPEND|os.O_WRONLY, privateBuildFileMode)
	switch {
	case err == nil:
		log.ansiFile, log.ansi = ansi, bufio.NewWriter(ansi)
	case !errors.Is(err, fs.ErrNotExist):
		b.Logger.Error("open task colour log", "task", task.ID, "err", err)
	}
	b.ProcessLogEntry("> "+interruptedTaskNote+".", log, task.ID, task.startedAt)
	// At once, as runTask does: the task doesn't run again, so nothing else
	// would send it before its batch's timer, which would outlive the call
	b.flushLog()
	if err := log.Close(); err != nil {
		b.Logger.Error("close task log", "task", task.ID, "err", err)
	}
}

// finishInterrupted runs the tasks the interrupted build has left (see
// FinishInterruptedBuilds), and ends it
func (b *Build) finishInterrupted() {
	b.mutex.Lock()
	status := b.Status
	b.mutex.Unlock()
	switch status {
	case StatusFailed, StatusFinished, StatusAborted, StatusTimedOut:
		// It was running the on_* and finally tasks of this status
	default:
		// It was running, so it was aborted
		status = StatusAborted
	}
	b.Logger.Warn("finishing build wakeci stopped without ending", "status", status)
	b.SetBuildStatus(status)
}

// MarkInterruptedBuilds saves the builds that were running when wakeci
// stopped as aborted. Call it after RestoreQueue and
// FinishInterruptedBuilds, so that the builds they restored are known
func MarkInterruptedBuilds() error {
	var stale []int
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(HistoryBucket).ForEach(func(k, v []byte) error {
			var msg BuildUpdateData
			if err := json.Unmarshal(v, &msg); err != nil {
				L.Error("decode build history entry", "build", binary.BigEndian.Uint64(k), "err", err)
				return nil
			}
			if isStaleBuild(&msg) {
				stale = append(stale, msg.ID)
			}
			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("read build history: %w", err)
	}
	if len(stale) > 0 {
		L.Warn("marking builds interrupted by the restart as aborted", "builds", stale)
	}
	return markStaleBuildsAborted(L, stale)
}

// forwardStopSignals passes each signal from signals on to forwarded, which
// drainQueue reads, once q noted it: a stop signal cancels a planned restart
// whenever it arrives. Like signal.Notify, it drops a signal while forwarded
// is full. Once signals is closed and every signal in it was noted, it closes
// done
func forwardStopSignals(q *Queue, signals <-chan os.Signal) (forwarded <-chan os.Signal, done <-chan struct{}) {
	out := make(chan os.Signal, cap(signals))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for sig := range signals {
			q.noteStopSignal(sig)
			select {
			case out <- sig:
			default:
			}
		}
	}()
	return out, finished
}

// drainOnStop waits for a stop signal, which begins a stop that aborts
// running builds after drainTimeout, or for a stop an update began, then
// drains the queue and calls stop. It returns without draining once ctx is
// done, as when serving ended without a stop: the waits on the way out read
// interrupt then
func drainOnStop(ctx context.Context, q *Queue, interrupt <-chan os.Signal, drainTimeout, hookTimeout time.Duration, stop func()) {
	select {
	case sig := <-interrupt:
		if ctx.Err() != nil {
			return
		}
		L.Warn("stopping", "signal", sig)
		q.BeginStop(time.Now().Add(drainTimeout), "")
	case <-q.Stopping():
	case <-ctx.Done():
		return
	}
	drainQueue(q, interrupt, hookTimeout)
	stop()
}

// drainQueue waits until the running builds end, or until the abort time
// BeginStop set, and then aborts the builds still running. Each value from
// interrupt ends the current wait early: the first aborts the builds now,
// the next stops waiting for their on_aborted and finally tasks. AbortNow
// ends the first wait too. Queued builds stay queued, so they start after
// the restart
func drainQueue(q *Queue, interrupt <-chan os.Signal, hookTimeout time.Duration) {
	status := q.Status()
	if status.AbortAt == nil {
		L.Error("drain queue: BeginStop was not called")
		return
	}
	if !q.idle() {
		L.Warn("stopping: waiting for running builds; send the signal again to abort them now",
			"running", status.Running, "abortAt", status.AbortAt.Format(time.DateTime))
	}
	if q.waitIdle(*status.AbortAt, interrupt, q.abortNowRequested()) {
		return
	}
	by := q.markBuildsAborted()
	L.Warn("stopping: aborting running builds", "by", by)
	q.abortRunning(stopNote, by)
	// Not abortNowRequested, which stays closed
	if !q.waitIdle(time.Now().Add(hookTimeout), interrupt, nil) {
		L.Warn("stopping before the aborted builds ended")
	}
}

// waitIdle waits until no build runs, until deadline, until a value arrives
// from interrupt, or until now is closed, and reports whether no build runs
func (q *Queue) waitIdle(deadline time.Time, interrupt <-chan os.Signal, now <-chan struct{}) bool {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()
	for !q.idle() {
		select {
		case <-ticker.C:
		case <-timer.C:
			return q.idle()
		case sig := <-interrupt:
			L.Warn("stopping: received another signal", "signal", sig)
			return q.idle()
		case <-now:
			return q.idle()
		}
	}
	return true
}
