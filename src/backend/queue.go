package main

import (
	"fmt"
	"sync"

	bolt "go.etcd.io/bbolt"
)

// Queue represents queued and running builds
type Queue struct {
	queued           []*Build
	running          []*Build
	mutex            sync.Mutex
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
			Logger.Printf("Inspecting build %d from queue\n", qItem.ID)
			if !qItem.Job.AllowParallel {
				// Verify that other build of the same job is not running
				for _, rItem := range q.running {
					if rItem.Job.Name == qItem.Job.Name {
						continue QLoop
					}
				}
			}
			foundItem = true
			foundItemID = id
			break
		}
		if foundItem {
			Logger.Printf("Running item %d, build %d\n", foundItemID, q.queued[foundItemID].ID)
			q.running = append(q.running, q.queued[foundItemID])
			go q.queued[foundItemID].Start()
			q.queued[foundItemID] = nil
			q.queued = append(q.queued[:foundItemID], q.queued[foundItemID+1:]...)
		} else {
			Logger.Println("Nothing to run")
		}
	}
	q.mutex.Unlock()
	if toRun && foundItem {
		q.Take()
	}
	Logger.Printf("Executing %d builds, %d in queue\n", len(q.running), len(q.queued))
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
	Logger.Printf("New build queued: %s %d\n", b.Job.Name, b.ID)
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
	Logger.Printf("Build %d was not found in Q\n", id)
}

// Verify returns true if a build with provided id is queued or running
func (q *Queue) Verify(id int) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for _, item := range q.running {
		if item.ID == id {
			return true
		}
	}
	for _, item := range q.queued {
		if item.ID == id {
			return true
		}
	}
	return false
}

// Abort schedules build to be aborted
func (q *Queue) Abort(id int) error {
	for _, item := range q.running {
		if item.ID == id {
			item.abortedChannel <- true
			return nil
		}
	}
	for _, item := range q.queued {
		if item.ID == id {
			item.SetBuildStatus(StatusAborted)
			return nil
		}
	}
	return fmt.Errorf("Build %d not found in Q", id)
}

// FlushLogs instructs to flush logs
func (q *Queue) FlushLogs(id int) error {
	for _, item := range q.running {
		if item.ID == id {
			item.flushChannel <- true
			return nil
		}
	}
	return fmt.Errorf("Build is not running")
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
		Logger.Println(err)
		return
	}
	q.mutex.Lock()
	q.concurrentBuilds = number
	q.mutex.Unlock()
	Logger.Printf("Number of concurrent builds changed to %d\n", number)
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

	Logger.Printf("Creating Queue with %d concurrent builds\n", cb)
	q := &Queue{
		concurrentBuilds: cb,
	}
	return q, nil
}
