package main

import (
	"fmt"

	"github.com/sasha-s/go-deadlock"

	bolt "go.etcd.io/bbolt"
)

// Queue represents queued and running builds
type Queue struct {
	queued  []*Build
	running []*Build
	// aborting holds builds aborted while queued, until their on_aborted and
	// finally tasks end. They hold no concurrency slot
	aborting         []*Build
	mutex            deadlock.Mutex
	concurrentBuilds int
}

// Take takes build from queue and starts running it
func (q *Queue) Take() {
	q.mutex.Lock()
	toRun := len(q.running) < q.concurrentBuilds && len(q.queued) > 0
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
	q.mutex.Unlock()
	if toRun && foundItem {
		q.Take()
	}
	L.Debug("queue status", "running", len(q.running), "queued", len(q.queued))
}

// TakeNow takes the build from the queue and starts executing it now
func (q *Queue) TakeNow(buildID int) error {
	var foundItem bool

	q.mutex.Lock()
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

	q.Take()
	L.Debug("queue status", "running", len(q.running), "queued", len(q.queued))
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
	build.requestAbort(reason)
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

// SetConcurrency sets number of concurrent builds
func (q *Queue) SetConcurrency(number int) {
	err := DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		err := gb.Put([]byte("concurrentBuilds"), IntToByte(number))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		L.Error("save concurrency setting", "err", err)
		return
	}
	q.mutex.Lock()
	q.concurrentBuilds = number
	q.mutex.Unlock()
	L.Info("concurrency changed", "concurrentBuilds", number)
	q.Take()
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
	}
	return q, nil
}
