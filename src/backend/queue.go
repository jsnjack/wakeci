package main

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/sasha-s/go-deadlock"

	bolt "go.etcd.io/bbolt"
)

// MsgTypeQueueStatus is the type of the messages with QueueStatusData
const MsgTypeQueueStatus = "queue:status"

// ErrQueueStopping is returned when a build can't start because wakeci is
// stopping. Queued builds start after the restart
var ErrQueueStopping = errors.New("wakeci is stopping; queued builds start after the restart")

// Queue represents queued and running builds
type Queue struct {
	queued  []*Build
	running []*Build
	// aborting holds builds aborted while queued, until their on_aborted and
	// finally tasks end. They hold no concurrency slot
	aborting         []*Build
	mutex            deadlock.Mutex
	concurrentBuilds int
	// stopping is closed by BeginStop: no queued build starts again, and
	// wakeci stops once the running builds end or abortAt passes
	stopping chan struct{}
	abortAt  time.Time
	// restartVersion is the version an update restarts into
	restartVersion string
}

// Take takes build from queue and starts running it
func (q *Queue) Take() {
	q.mutex.Lock()
	toRun := !q.isStoppingLocked() && len(q.running) < q.concurrentBuilds && len(q.queued) > 0
	var foundItem bool
	var foundItemID int
	if toRun {
	QLoop:
		for id, qItem := range q.queued {
			L.Debug("inspecting build from queue", "build", qItem.ID)
			if qItem.Job.Concurrency != 0 {
				// Verify number of running builds of the same job
				parallel := 0
				for _, rItem := range q.running {
					if rItem.Job.Name == qItem.Job.Name {
						parallel++
					}
				}
				if parallel >= qItem.Job.Concurrency {
					continue QLoop
				}
			}
			foundItem = true
			foundItemID = id
			break
		}
		if foundItem {
			L.Debug("running item", "item", foundItemID, "build", q.queued[foundItemID].ID)
			q.running = append(q.running, q.queued[foundItemID])
			go q.queued[foundItemID].Start()
			q.queued[foundItemID] = nil
			q.queued = append(q.queued[:foundItemID], q.queued[foundItemID+1:]...)
		} else {
			L.Debug("nothing to run")
		}
	}
	running, queued := len(q.running), len(q.queued)
	q.mutex.Unlock()
	if toRun && foundItem {
		q.Take()
		return
	}
	L.Debug("queue status", "running", running, "queued", queued)
}

// TakeNow takes the build from the queue and starts executing it now. It
// returns ErrQueueStopping while wakeci stops
func (q *Queue) TakeNow(buildID int) error {
	var foundItem bool

	q.mutex.Lock()
	if q.isStoppingLocked() {
		q.mutex.Unlock()
		return ErrQueueStopping
	}
	for id, qItem := range q.queued {
		if qItem.ID == buildID {
			L.Debug("running item immediately", "item", id, "build", q.queued[id].ID)
			q.running = append(q.running, q.queued[id])
			go q.queued[id].Start()
			q.queued[id] = nil
			q.queued = append(q.queued[:id], q.queued[id+1:]...)
			foundItem = true
			break
		}
	}
	q.mutex.Unlock()

	// Take logs the queue lengths
	q.Take()
	if !foundItem {
		return fmt.Errorf("build with id %d is not in the queue", buildID)
	}
	return nil
}

// Add adds build to the queue
func (q *Queue) Add(b *Build) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.queued = append(q.queued, b)
	// Possibly shift queue
	if b.Job.Priority != 0 {
		for id, qItem := range q.queued {
			if b.Job.Priority > qItem.Job.Priority {
				newQueue := make([]*Build, len(q.queued))
				copy(newQueue, q.queued[:id])
				newQueue[id] = q.queued[len(q.queued)-1]
				copy(newQueue[id+1:], q.queued[id:len(q.queued)-1])
				q.queued = newQueue
				break
			}
		}
	}
	L.Info("new build queued", "job", b.Job.Name, "build", b.ID)
}

// Remove removes a build from Queue
func (q *Queue) Remove(id int) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for i, ex := range q.running {
		if ex.ID == id {
			q.running = append(q.running[:i], q.running[i+1:]...)
			return
		}
	}
	for i, ex := range q.queued {
		if ex.ID == id {
			q.queued = append(q.queued[:i], q.queued[i+1:]...)
			return
		}
	}
	for i, ex := range q.aborting {
		if ex.ID == id {
			q.aborting = append(q.aborting[:i], q.aborting[i+1:]...)
			return
		}
	}
	L.Warn("build not found in queue", "build", id)
}

// Verify returns true if a build with provided id is queued or running,
// including the tasks of a queued abort
func (q *Queue) Verify(id int) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for _, item := range q.queued {
		if item.ID == id {
			return true
		}
	}
	return q.find(id) != nil
}

// Abort schedules build to be aborted. A running build records the request
// (Build.requestAbort), which never blocks and is kept until the build stops
// the tasks that run, whatever it is doing when the request arrives.
func (q *Queue) Abort(id int, reason string) error {
	q.mutex.Lock()
	build := q.find(id)
	dequeued := false
	if build == nil {
		// Leave the queue under the lock, so Take can't start it
		for i, item := range q.queued {
			if item.ID == id {
				build = item
				q.queued = append(q.queued[:i], q.queued[i+1:]...)
				q.aborting = append(q.aborting, build)
				dequeued = true
				break
			}
		}
	}
	q.mutex.Unlock()
	if build == nil {
		return fmt.Errorf("Build %d not found in Q", id)
	}

	// Stops on_pending tasks of a queued build, too
	build.requestAbort(reason, "")
	if dequeued {
		// Waits for the on_pending tasks, then runs on_aborted and finally
		go build.SetBuildStatus(StatusAborted)
	}
	return nil
}

// find returns the build with id that is running or running the tasks of a
// queued abort. The caller holds q.mutex
func (q *Queue) find(id int) *Build {
	for _, list := range [][]*Build{q.running, q.aborting} {
		for _, item := range list {
			if item.ID == id {
				return item
			}
		}
	}
	return nil
}

// FlushLogs instructs to flush logs. The send happens outside q.mutex and
// never blocks.
func (q *Queue) FlushLogs(id int) error {
	q.mutex.Lock()
	running := q.find(id)
	q.mutex.Unlock()
	if running == nil {
		return fmt.Errorf("Build is not running")
	}

	select {
	case running.flushChannel <- true:
	default:
	}
	return nil
}

// SetConcurrency saves and sets number of concurrent builds
func (q *Queue) SetConcurrency(number int) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	err := DB.Update(func(tx *bolt.Tx) error {
		return saveConcurrency(tx, number)
	})
	if err != nil {
		L.Error("save concurrency setting", "err", err)
		return
	}
	q.applyConcurrency(number)
}

// saveConcurrency saves number of concurrent builds, for applyConcurrency
// once tx commits
func saveConcurrency(tx *bolt.Tx, number int) error {
	return tx.Bucket(GlobalBucket).Put([]byte("concurrentBuilds"), IntToByte(number))
}

// applyConcurrency sets number of concurrent builds, already saved
func (q *Queue) applyConcurrency(number int) {
	q.mutex.Lock()
	q.concurrentBuilds = number
	q.mutex.Unlock()
	L.Info("concurrency changed", "concurrentBuilds", number)
	q.Take()
}

// BeginStop keeps queued builds from starting, because wakeci stops once the
// running builds end, and aborts them at abortAt (see drainQueue).
// restartVersion is the version an update restarts into, empty for a stop.
// It returns false if a stop was already planned
func (q *Queue) BeginStop(abortAt time.Time, restartVersion string) bool {
	q.mutex.Lock()
	if q.isStoppingLocked() {
		q.mutex.Unlock()
		return false
	}
	q.abortAt = abortAt
	q.restartVersion = restartVersion
	close(q.stoppingLocked())
	q.mutex.Unlock()
	q.broadcastStatus()
	return true
}

// Stopping returns a channel that BeginStop closes
func (q *Queue) Stopping() <-chan struct{} {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.stoppingLocked()
}

// stoppingLocked returns the stopping channel, creating it for a Queue made
// without CreateQueue. The caller holds q.mutex
func (q *Queue) stoppingLocked() chan struct{} {
	if q.stopping == nil {
		q.stopping = make(chan struct{})
	}
	return q.stopping
}

// isStoppingLocked reports whether BeginStop was called. The caller holds
// q.mutex
func (q *Queue) isStoppingLocked() bool {
	select {
	case <-q.stoppingLocked():
		return true
	default:
		return false
	}
}

// Status returns whether queued builds start, and how many builds run and
// wait
func (q *Queue) Status() QueueStatusData {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	status := QueueStatusData{
		Stopping: q.isStoppingLocked(),
		Running:  len(q.running) + len(q.aborting),
		Queued:   len(q.queued),
		Version:  Version,
	}
	if status.Stopping {
		abortAt := q.abortAt
		status.AbortAt = &abortAt
		status.RestartVersion = q.restartVersion
	}
	return status
}

// broadcastStatus sends Status to the subscribed clients
func (q *Queue) broadcastStatus() {
	WSHub.broadcast <- &MsgBroadcast{Type: MsgTypeQueueStatus, Data: q.Status()}
}

// idle reports whether no build runs, not counting the on_pending tasks of
// queued builds
func (q *Queue) idle() bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.running) == 0 && len(q.aborting) == 0
}

// abortRunning aborts the running builds, and the on_aborted and finally
// tasks of builds aborted while queued, and logs note in their tasks. Queued
// builds stay queued
func (q *Queue) abortRunning(note string) {
	q.mutex.Lock()
	running := slices.Concat(q.running, q.aborting)
	q.mutex.Unlock()
	for _, build := range running {
		build.Logger.Warn("aborting build", "reason", note)
		build.requestAbort(StatusAborted, note)
	}
}

// waitQueuedHooks waits for the on_pending tasks of the queued builds, at
// most timeout, and reports whether they ended
func (q *Queue) waitQueuedHooks(timeout time.Duration) bool {
	q.mutex.Lock()
	queued := slices.Clone(q.queued)
	q.mutex.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, build := range queued {
			build.pendingTasksWG.Wait()
		}
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// CreateQueue creates new Queue object
func CreateQueue() (*Queue, error) {
	var cb int
	err := DB.View(func(tx *bolt.Tx) error {
		var err error
		gb := tx.Bucket(GlobalBucket)
		cb, err = ByteToInt(gb.Get([]byte("concurrentBuilds")))
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	L.Debug("creating queue", "concurrentBuilds", cb)
	q := &Queue{
		concurrentBuilds: cb,
		stopping:         make(chan struct{}),
	}
	return q, nil
}
