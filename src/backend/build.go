package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	bolt "github.com/etcd-io/bbolt"
	"github.com/go-cmd/cmd"
	"github.com/gorilla/websocket"
)

// NumberOfConcurrentBuilds maximum amount of tasks that are being executed at the same time
const NumberOfConcurrentBuilds = 2

// BuildList contains all tasks that are being executed at the moment
var BuildList []*Build

// BuildQueue ...
var BuildQueue []*Build

// BuildStatus ...
type BuildStatus string

// BuildRunning ...
const BuildRunning = "running"

// BuildFailed ...
const BuildFailed = "failed"

// BuildFinished ...
const BuildFinished = "finished"

// BuildPending ...
const BuildPending = "pending"

// Build ...
type Build struct {
	ID          string // job.Name + Count
	Job         *Job
	Count       int
	Status      BuildStatus
	DoneTasks   int // to report progress
	Logger      *log.Logger
	Subscribers []*websocket.Conn
}

// Start starts execution of tasks in job
func (b *Build) Start() {
	b.Logger.Println("Started...")
	b.Status = BuildRunning
	b.BroadcastUpdate()
	err := os.MkdirAll(WorkspaceDir+b.ID+"/", os.ModePerm)
	if err != nil {
		b.Logger.Println(err)
		b.Failed()
	}
	for _, task := range b.Job.Tasks {
		// Disable output buffering, enable streaming
		cmdOptions := cmd.Options{
			Buffered:  false,
			Streaming: true,
		}

		// Create Cmd with options
		envCmd := cmd.NewCmdOptions(cmdOptions, "bash", "-c", task.Command)

		// Print STDOUT and STDERR lines streaming from Cmd
		go func() {
			x := 0
			for {
				select {
				case line := <-envCmd.Stdout:
					b.PublishCommandLogs(0, x, line)
					x++
				case line := <-envCmd.Stderr:
					b.PublishCommandLogs(0, x, line)
					x++
				}
			}
		}()

		// Run and wait for Cmd to return, discard Status
		status := <-envCmd.Start()
		fmt.Println(status)

		// Cmd has finished but wait for goroutine to print all lines
		for len(envCmd.Stdout) > 0 || len(envCmd.Stderr) > 0 {
			time.Sleep(10 * time.Millisecond)
		}

		if status.Exit != 0 {
			b.Failed()
			return
		}
		b.DoneTasks++
		b.BroadcastUpdate()
	}
	b.Finished()
}

// Failed is called when job fails
func (b *Build) Failed() {
	b.Logger.Println("Failed.")
	b.Status = BuildFailed
	b.BroadcastUpdate()
	b.Cleanup()
}

// Finished is called when a job succeded
func (b *Build) Finished() {
	b.Logger.Println("Finished.")
	b.Status = BuildFinished
	b.BroadcastUpdate()
	b.Cleanup()
}

// Cleanup is called when a job finished or filed
func (b *Build) Cleanup() {
	for i, ex := range BuildList {
		if ex.ID == b.ID {
			BuildList = append(BuildList[:i], BuildList[i+1:]...)
			break
		}
	}
	TakeFromQueue()
}

// BroadcastUpdate ...
func (b *Build) BroadcastUpdate() {
	msg := MsgBroadcast{
		Type: MsgTypeBuildUpdate,
		Data: &BuildUpdateData{
			ID:         b.ID,
			Count:      b.Count,
			Name:       b.Job.Name,
			Status:     b.Status,
			TotalTasks: len(b.Job.Tasks),
			DoneTasks:  b.DoneTasks,
		},
	}
	BroadcastChannel <- &msg
}

// PublishCommandLogs ...
func (b *Build) PublishCommandLogs(taskID int, id int, data string) {
	msg := MsgBroadcast{
		Type: MsgType("build:log:" + b.ID),
		Data: &CommandLogData{
			TaskID: taskID,
			ID:     id,
			Data:   data,
		},
	}
	BroadcastChannel <- &msg
}

// CreateBuild ..
func CreateBuild(job *Job) (*Build, error) {
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
	msg := MsgBroadcast{
		Type: MsgTypeJobUpdate,
		Data: &JobsListData{
			Name:  job.Name,
			Count: id,
		},
	}
	BroadcastChannel <- &msg

	build := Build{
		Job:    job,
		Status: BuildPending,
		Count:  id,
		ID:     fmt.Sprintf("%s_%d", job.Name, id),
	}
	build.Logger = log.New(os.Stdout, build.ID, log.Lmicroseconds|log.Lshortfile)
	return &build, nil
}

// TakeFromQueue checks if it is possible to start executing new job from queue
// and executes it
func TakeFromQueue() {
	if len(BuildList) < NumberOfConcurrentBuilds && len(BuildQueue) > 0 {
		Logger.Printf("Taking job from queue %s\n", BuildQueue[0].ID)
		BuildList = append(BuildList, BuildQueue[0])
		go BuildQueue[0].Start()
		BuildQueue[0] = nil
		BuildQueue = BuildQueue[1:]
		TakeFromQueue()
	}
	Logger.Printf("Executing %d jobs, %d in queue\n", len(BuildList), len(BuildQueue))
}
