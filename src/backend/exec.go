package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	bolt "github.com/etcd-io/bbolt"
)

// ExecutorStatus ...
type ExecutorStatus string

// ExecutorRunning ...
const ExecutorRunning = "running"

// ExecutorFailed ...
const ExecutorFailed = "failed"

// ExecutorFinished ...
const ExecutorFinished = "finished"

// ExecutorPending ...
const ExecutorPending = "pending"

// Executor ...
type Executor struct {
	Job       *Job
	ID        int
	Status    ExecutorStatus
	DoneTasks int // to report progress
}

// Start starts execution of tasks in job
func (e *Executor) Start() {
	e.BroadcastUpdate()
}

// BroadcastUpdate ...
func (e *Executor) BroadcastUpdate() {
	msg := MsgFeedUpdate{
		Type: "feed:update",
		Data: &FeedUpdateData{
			ID:         e.ID,
			Name:       e.Job.Name,
			Status:     e.Status,
			TotalTasks: len(e.Job.Tasks),
			DoneTasks:  e.DoneTasks,
		},
	}
	msgB, err := json.Marshal(msg)
	if err != nil {
		Logger.Println(err)
		return
	}
	BroadcastChannel <- msgB
}

// CreateExecutor ..
func CreateExecutor(job *Job) (*Executor, error) {
	var id int
	err := DB.Update(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte(JobsBucket))
		j := b.Bucket([]byte(job.Name))
		if j == nil {
			return fmt.Errorf("No job with name %s", job.Name)
		}
		idS := string(j.Get([]byte("count")))
		id, err = strconv.Atoi(idS)
		if err != nil {
			return err
		}
		id = id + 1
		err = j.Put([]byte("count"), []byte(strconv.Itoa(id)))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Broadcast job count update
	msg := MsgJobUpdate{
		Type: "job:update",
		Data: &JobsListData{
			Name:  job.Name,
			Count: id,
		},
	}
	msgB, err := json.Marshal(msg)
	if err != nil {
		Logger.Println(err)
	}
	BroadcastChannel <- msgB

	ex := Executor{
		Job:    job,
		Status: ExecutorPending,
		ID:     id,
	}
	return &ex, nil
}
