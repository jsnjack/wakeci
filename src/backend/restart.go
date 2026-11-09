package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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

// stopPollInterval is how often a stop checks whether the builds ended
const stopPollInterval = 200 * time.Millisecond

// queuedBuild is what a queued build needs to be queued again after a
// restart, stored in QueueBucket
type queuedBuild struct {
	Job               *Job                `json:"job"`
	Params            []map[string]string `json:"params"`
	StartedWithAPIKey bool                `json:"startedWithAPIKey"`
	// Tasks holds when the on_pending tasks ran, which Job doesn't keep.
	// They don't run again
	Tasks []*TaskStatus `json:"tasks"`
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
	if saved.Job == nil {
		return nil, fmt.Errorf("queued build has no job")
	}
	build := &Build{
		ID:                id,
		Job:               saved.Job,
		Status:            StatusPending,
		abort:             newAbortScope(),
		flushChannel:      make(chan bool),
		Params:            saved.Params,
		ETA:               GetJobETA(saved.Job.Name),
		startedWithAPIKey: saved.StartedWithAPIKey,
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

// MarkInterruptedBuilds saves the builds that were running when wakeci
// stopped as aborted. Call it after RestoreQueue, so that queued builds are
// known
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

// forwardStopSignals passes each signal from signals on to the returned
// channel, which drainQueue reads, once q noted it: a stop signal cancels a
// planned restart whenever it arrives. Like signal.Notify, it drops a signal
// while the channel is full. It ends when signals is closed
func forwardStopSignals(q *Queue, signals <-chan os.Signal) <-chan os.Signal {
	forwarded := make(chan os.Signal, cap(signals))
	go func() {
		for sig := range signals {
			q.noteStopSignal(sig)
			select {
			case forwarded <- sig:
			default:
			}
		}
	}()
	return forwarded
}

// drainQueue waits until the running builds end, or until the abort time
// BeginStop set, and then aborts the builds still running. Each value from
// interrupt ends the current wait early: the first aborts the builds now,
// the next stops waiting for their on_aborted and finally tasks. Queued
// builds stay queued, so they start after the restart
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
	if q.waitIdle(*status.AbortAt, interrupt) {
		return
	}
	L.Warn("stopping: aborting running builds")
	q.abortRunning(stopNote)
	if !q.waitIdle(time.Now().Add(hookTimeout), interrupt) {
		L.Warn("stopping before the aborted builds ended")
	}
}

// waitIdle waits until no build runs, until deadline, or until a value
// arrives from interrupt, and reports whether no build runs
func (q *Queue) waitIdle(deadline time.Time, interrupt <-chan os.Signal) bool {
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
		}
	}
	return true
}
