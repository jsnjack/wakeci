package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	ID        string // job.Name + Count
	Job       *Job
	Count     int
	Status    ExecutorStatus
	DoneTasks int // to report progress
	Logger    *log.Logger
}

// Start starts execution of tasks in job
func (e *Executor) Start() {
	e.Logger.Println("Started...")
	e.Status = ExecutorRunning
	e.BroadcastUpdate()
	err := os.MkdirAll(WorkspaceDir+e.ID+"/", os.ModePerm)
	if err != nil {
		e.Logger.Println(err)
		e.Failed()
	}
	for _, task := range e.Job.Tasks {
		args := append([]string{"-c", task.Command})
		cmd := exec.Command("sh", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			e.Logger.Println(err)
			e.Failed()
			return
		}
		e.Logger.Println(string(out))
		e.DoneTasks++
		e.BroadcastUpdate()
	}
	e.Finished()
}

// Failed is called when job fails
func (e *Executor) Failed() {
	e.Logger.Println("Failed.")
	e.Status = ExecutorFailed
	e.BroadcastUpdate()
	e.Cleanup()
}

// Finished is called when a job succeded
func (e *Executor) Finished() {
	e.Logger.Println("Finished.")
	e.Status = ExecutorFinished
	e.BroadcastUpdate()
	e.Cleanup()
}

// Cleanup is called when a job finished or filed
func (e *Executor) Cleanup() {
	for i, ex := range FeedList {
		if ex.ID == e.ID {
			FeedList = append(FeedList[:i], FeedList[i+1:]...)
			break
		}
	}
	TakeFromQueue()
}

// BroadcastUpdate ...
func (e *Executor) BroadcastUpdate() {
	msg := MsgFeedUpdate{
		Type: "feed:update",
		Data: &FeedUpdateData{
			ID:         e.ID,
			Count:      e.Count,
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
		Count:  id,
		ID:     fmt.Sprintf("%s_%d", job.Name, id),
	}
	ex.Logger = log.New(os.Stdout, ex.ID, log.Lmicroseconds|log.Lshortfile)
	return &ex, nil
}
