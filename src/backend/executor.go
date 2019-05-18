package main

import "encoding/json"

// ExecutorStatus ...
type ExecutorStatus string

// ExecutorRunning ...
const ExecutorRunning = "running"

// ExecutorFailed ...
const ExecutorFailed = "failed"

// ExecutorFinished ...
const ExecutorFinished = "finished"

// Executor ...
type Executor struct {
	Job       *Job
	ID        string
	Status    ExecutorStatus
	DoneTasks int
}

// Start starts execution of tasks in job
func (e *Executor) Start() {
	e.BroadcastUpdate()
}

// BroadcastUpdate ...
func (e *Executor) BroadcastUpdate() {
	msg := MsgFeedUpdate{
		Type: "feed:update",
		Data: FeedUpdateData{
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
func CreateExecutor(job *Job) *Executor {
	ex := Executor{
		Job:    job,
		Status: ExecutorRunning,
		ID:     GenerateRandomString(5),
	}
	return &ex
}
