package main

import (
	"fmt"
	"sync"
)

// Queue represents queued and running builds
type Queue struct {
	queued           []*Build
	running          []*Build
	mutex            *sync.Mutex
	concurrentBuilds int
}

// Take takes build from queue and starts running it
func (q *Queue) Take() {
	q.mutex.Lock()
	toRun := len(q.running) < q.concurrentBuilds && len(q.queued) > 0
	if toRun {
		Logger.Printf("Taking build from queue %d\n", q.queued[0].ID)
		q.running = append(q.running, q.queued[0])
		go q.queued[0].Start()
		q.queued[0] = nil
		q.queued = q.queued[1:]
	}
	q.mutex.Unlock()
	if toRun {
		q.Take()
	}
	Logger.Printf("Executing %d builds, %d in queue\n", len(q.running), len(q.queued))
}

// Add adds build to the queue
func (q *Queue) Add(b *Build) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.queued = append(q.queued, b)
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
			go func() {
				item.abortedChannel <- true
			}()
			return nil
		}
	}
	for _, item := range q.queued {
		if item.ID == id {
			item.Abort()
			return nil
		}
	}
	return fmt.Errorf("Build %d not found in Q", id)
}
