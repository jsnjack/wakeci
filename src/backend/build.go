package main

import (
	"bufio"
	"encoding/json"
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

// ItemStatus handles information about the item status (currently is used for
// both Builds and Tasks)
type ItemStatus string

// StatusRunning ...
const StatusRunning = "running"

// StatusFailed ...
const StatusFailed = "failed"

// StatusFinished ...
const StatusFinished = "finished"

// StatusPending ...
const StatusPending = "pending"

// StatusAborted ...
const StatusAborted = "aborted"

// Build ...
type Build struct {
	ID          int
	Job         *Job
	Status      ItemStatus
	Logger      *log.Logger
	Subscribers []*websocket.Conn
}

// Start starts execution of tasks in job
func (b *Build) Start() {
	b.Logger.Println("Started...")
	b.Status = StatusRunning
	b.BroadcastUpdate()
	for _, task := range b.Job.Tasks {
		// Disable output buffering, enable streaming
		cmdOptions := cmd.Options{
			Buffered:  false,
			Streaming: true,
		}

		// Create Cmd with options
		envCmd := cmd.NewCmdOptions(cmdOptions, "bash", "-c", task.Command)

		fwChannel := make(chan bool)

		// Print STDOUT and STDERR lines streaming from CmdLogger
		go func() {
			file, err := os.Create(b.GetWakespaceDir() + fmt.Sprintf("task_%d.log", task.ID))
			bw := bufio.NewWriter(file)
			defer func() {
				bw.Flush()
				file.Close()
			}()
			if err != nil {
				// Allow command to start
				time.Sleep(10 * time.Millisecond)
				b.Logger.Println(err)
				envCmd.Stop()
				return
			}
			x := 0
			for {
				select {
				case line := <-envCmd.Stdout:
					_, err := bw.WriteString(line + "\n")
					if err != nil {
						b.Logger.Println(err)
					}
					b.PublishCommandLogs(task.ID, x, line)
					x++
				case line := <-envCmd.Stderr:
					_, err := bw.WriteString(line + "\n")
					if err != nil {
						b.Logger.Println(err)
					}
					b.PublishCommandLogs(task.ID, x, line)
					x++
				case <-fwChannel:
					return
				}
			}
		}()

		// Run and wait for Cmd to return, discard Status
		status := <-envCmd.Start()

		// Cmd has finished but wait for goroutine to print all lines
		for len(envCmd.Stdout) > 0 || len(envCmd.Stderr) > 0 {
			time.Sleep(10 * time.Millisecond)
		}
		// Signal to flush the file
		fwChannel <- true

		if status.Exit != 0 {
			task.Status = StatusFailed
			b.Failed()
			return
		}
		task.Status = StatusFinished
		b.BroadcastUpdate()
	}
	b.Finished()
}

// Failed is called when job fails
func (b *Build) Failed() {
	b.Logger.Println("Failed.")
	b.Status = StatusFailed
	b.BroadcastUpdate()
	b.Cleanup()
}

// Finished is called when a job succeded
func (b *Build) Finished() {
	b.Logger.Println("Finished.")
	b.Status = StatusFinished
	b.BroadcastUpdate()
	b.Cleanup()
}

// Cleanup is called when a job finished or failed
func (b *Build) Cleanup() {
	for i, ex := range BuildList {
		if ex.ID == b.ID {
			BuildList = append(BuildList[:i], BuildList[i+1:]...)
			break
		}
	}
	TakeFromQueue()
}

// BroadcastUpdate sends update to all subscribed clients. Contains general
// information about the build
func (b *Build) BroadcastUpdate() {
	data := b.GenerateBuildUpdateData()
	msg := MsgBroadcast{
		Type: MsgTypeBuildUpdate,
		Data: data,
	}
	BroadcastChannel <- &msg

	err := DB.Update(func(tx *bolt.Tx) error {
		var err error
		hb := tx.Bucket([]byte(HistoryBucket))
		dataB, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return hb.Put([]byte(strconv.Itoa(data.ID)), dataB)
	})
	if err != nil {
		b.Logger.Println(err)
	}
}

// GenerateBuildUpdateData generates BuildUpdateData
func (b *Build) GenerateBuildUpdateData() *BuildUpdateData {
	return &BuildUpdateData{
		ID:         b.ID,
		Name:       b.Job.Name,
		Status:     b.Status,
		TotalTasks: len(b.Job.Tasks),
		DoneTasks:  b.GetNumberOfFinishedTasks(),
	}
}

// PublishCommandLogs sends log update to all subscribed users
func (b *Build) PublishCommandLogs(taskID int, id int, data string) {
	msg := MsgBroadcast{
		Type: MsgType("build:log:" + strconv.Itoa(b.ID)),
		Data: &CommandLogData{
			TaskID: taskID,
			ID:     id,
			Data:   data,
		},
	}
	BroadcastChannel <- &msg
}

// GetWorkspaceDir returns path to the workspace, where all user created files
// are stored
func (b *Build) GetWorkspaceDir() string {
	return WorkingDir + "workspace/" + strconv.Itoa(b.ID) + "/"
}

// GetWakespaceDir returns path to the data dir - there all build+wake related data is
// stored
func (b *Build) GetWakespaceDir() string {
	return WorkingDir + "wakespace/" + strconv.Itoa(b.ID) + "/"
}

// GetBuildConfigFilename returns build config filename (copy of the original job file)
func (b *Build) GetBuildConfigFilename() string {
	return b.GetWakespaceDir() + "build.yaml"
}

// GetNumberOfFinishedTasks returns number of finished tasks
func (b *Build) GetNumberOfFinishedTasks() int {
	var x int
	for _, t := range b.Job.Tasks {
		switch t.Status {
		case StatusFailed, StatusFinished:
			x++
			break
		}
	}
	return x
}

// CreateBuild ..
func CreateBuild(job *Job) (*Build, error) {
	var counti int
	err := DB.Update(func(tx *bolt.Tx) error {
		var err error
		gb := tx.Bucket([]byte(GlobalBucket))
		count := gb.Get([]byte("count"))
		if count == nil {
			counti = 1
		} else {
			counti, err = ByteToInt(count)
			if err != nil {
				return err
			}
			counti++
		}
		gb.Put([]byte("count"), []byte(strconv.Itoa(counti)))
		return nil
	})
	if err != nil {
		return nil, err
	}

	build := Build{
		Job:    job,
		Status: StatusPending,
		ID:     counti,
	}
	build.Logger = log.New(os.Stdout, strconv.Itoa(build.ID)+" ", log.Lmicroseconds|log.Lshortfile)
	return &build, nil
}

// TakeFromQueue checks if it is possible to start executing new job from queue
// and executes it
func TakeFromQueue() {
	if len(BuildList) < NumberOfConcurrentBuilds && len(BuildQueue) > 0 {
		Logger.Printf("Taking job from queue %d\n", BuildQueue[0].ID)
		BuildList = append(BuildList, BuildQueue[0])
		go BuildQueue[0].Start()
		BuildQueue[0] = nil
		BuildQueue = BuildQueue[1:]
		TakeFromQueue()
	}
	Logger.Printf("Executing %d jobs, %d in queue\n", len(BuildList), len(BuildQueue))
}
