package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar"

	bolt "github.com/etcd-io/bbolt"
	"github.com/jsnjack/cmd"
)

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
	ID             int
	Job            *Job
	Status         ItemStatus
	Logger         *log.Logger
	abortedChannel chan bool
	pendingTasksWG sync.WaitGroup
	aborted        bool
	Params         []map[string]string
	Artifacts      []string
	StartedAt      time.Time
	Duration       time.Duration
	timer          *time.Timer // A timer for Job.Timeout
}

// Start starts execution of tasks in job
func (b *Build) Start() {
	b.SetBuildStatus(StatusRunning)
	for _, task := range b.Job.Tasks {
		if task.Kind != KindMain {
			continue
		}
		task.Status = StatusRunning
		task.startedAt = time.Now()
		b.BroadcastUpdate()

		status := b.runTask(task)

		task.Status = status
		task.duration = time.Since(task.startedAt)
		switch status {
		case StatusFinished:
			break
		case StatusFailed:
			b.SetBuildStatus(StatusFailed)
			return
		case StatusAborted:
			b.SetBuildStatus(StatusAborted)
			return
		}
		b.BroadcastUpdate()
	}
	b.SetBuildStatus(StatusFinished)
}

// runOnStatusTasks runs tasks on status change
func (b *Build) runOnStatusTasks(status ItemStatus) {
	if status == StatusPending {
		b.pendingTasksWG.Add(1)
		defer b.pendingTasksWG.Done()
	}
	for _, task := range b.Job.Tasks {
		if task.Kind == string(status) {
			task.Status = StatusRunning
			task.startedAt = time.Now()

			status := b.runTask(task)

			task.Status = status
			task.duration = time.Since(task.startedAt)
		}
	}
}

// runTask is responsible for running one task and return it's status
func (b *Build) runTask(task *Task) ItemStatus {
	b.Logger.Printf("Task %d has been started\n", task.ID)
	defer b.Logger.Printf("Task %d is completed\n", task.ID)
	// Disable output buffering, enable streaming
	cmdOptions := cmd.Options{
		Buffered:  false,
		Streaming: true,
	}

	// Create Cmd with options
	// Modify default streaming buffer size (thanks, webpack)
	cmd.DEFAULT_LINE_BUFFER_SIZE = 491520
	taskCmd := cmd.NewCmdOptions(cmdOptions, "bash", "-c", task.Command)

	// Construct environment from params
	taskCmd.Env = os.Environ()
	taskCmd.Dir = b.GetWorkspaceDir()
	taskCmd.Env = append(taskCmd.Env, b.generateDefaultEnvVariables()...)
	for idx := range b.Params {
		for pkey, pval := range b.Params[idx] {
			taskCmd.Env = append(taskCmd.Env, fmt.Sprintf("%s=%s", pkey, pval))
		}
	}

	fwChannel := make(chan bool)

	// Print STDOUT and STDERR lines streaming from CmdLogger
	go func() {
		file, err := os.Create(b.GetWakespaceDir() + fmt.Sprintf("task_%d.log", task.ID))
		bw := bufio.NewWriter(file)
		defer func() {
			err = bw.Flush()
			if err != nil {
				b.Logger.Println(err)
			}
			err = file.Close()
			if err != nil {
				b.Logger.Println(err)
			}
		}()
		if err != nil {
			// Allow command to start
			time.Sleep(10 * time.Millisecond)
			b.Logger.Println(err)
			taskCmd.Stop()
			return
		}

		// Add executed command to logs
		commandInfo := b.ProcessLogEntry(task.Command, task.startedAt)
		_, err = bw.WriteString(commandInfo)
		if err != nil {
			b.Logger.Println(err)
		}
		b.PublishCommandLogs(task.ID, commandInfo)

		for {
			select {
			case line := <-taskCmd.Stdout:
				line = b.ProcessLogEntry(line, task.startedAt)
				_, err := bw.WriteString(line)
				if err != nil {
					b.Logger.Println(err)
				}
				b.PublishCommandLogs(task.ID, line)
			case line := <-taskCmd.Stderr:
				line = b.ProcessLogEntry(line, task.startedAt)
				_, err := bw.WriteString(line)
				if err != nil {
					b.Logger.Println(err)
				}
				b.PublishCommandLogs(task.ID, line)
			case <-fwChannel:
				return
			case toAbort := <-b.abortedChannel:
				b.Logger.Println("Aborting via abortedChannel")
				if toAbort {
					taskCmd.Stop()
					b.aborted = true
				}
			}
		}
	}()

	// Run and wait for Cmd to return
	status := <-taskCmd.Start()
	b.Logger.Printf(
		"Task %d result: Completed: %v, Exit code %d, Error %s",
		task.ID, status.Complete, status.Exit, status.Error,
	)

	// Cmd has finished but wait for goroutine to print all lines
	for len(taskCmd.Stdout) > 0 || len(taskCmd.Stderr) > 0 {
		time.Sleep(10 * time.Millisecond)
	}
	// Signal to flush the file
	fwChannel <- true

	// Abort message was recieved via channel
	if b.aborted {
		return StatusAborted
	}

	if !status.Complete || status.Exit != 0 || status.Error != nil {
		return StatusFailed
	}

	// Start new job from this task
	if task.QueueJob != "" {
		// New job is going to be started with params from the current build
		newParams := url.Values{}
		for idx := range b.Params {
			for pkey, pval := range b.Params[idx] {
				newParams.Set(pkey, pval)
			}
		}
		newParams.Set("WAKE_PARENT_BUILD_ID", strconv.Itoa(b.ID))

		newBuild, err := RunJob(task.QueueJob, newParams)
		if err != nil {
			newBuildMsg := fmt.Sprintf("Unable to add job %s to the queue: %s\n", task.QueueJob, err)
			b.Logger.Printf(newBuildMsg)
			return StatusFailed
		}
		b.Logger.Printf(
			"New job %s has been scheduled with build ID %d\n",
			task.QueueJob, newBuild.ID,
		)
	}

	return StatusFinished
}

// Generate default set of environmental variables that are injected before
// running a task, for example WAKE_BUILD_ID
func (b *Build) generateDefaultEnvVariables() []string {
	var evs = []string{
		fmt.Sprintf("WAKE_BUILD_ID=%d", b.ID),
		fmt.Sprintf("WAKE_BUILD_WORKSPACE=%s", b.GetWorkspaceDir()),
		fmt.Sprintf("WAKE_JOB_NAME=%s", b.Job.Name),
		fmt.Sprintf("WAKE_CONFIG_DIR=%s", *ConfigDirFlag),
	}
	if *PortFlag == "443" {
		evs = append(evs, fmt.Sprintf("WAKE_URL=https://%s/", *HostnameFlag))
	} else {
		evs = append(evs, fmt.Sprintf("WAKE_URL=http://localhost:%s/", *PortFlag))
	}
	return evs
}

// Cleanup is called when a job finished or failed
func (b *Build) Cleanup() {
	if b.timer != nil {
		b.timer.Stop()
	}
	Q.Remove(b.ID)
	Q.Take()
}

// CollectArtifacts copies artifacts from workspace to wakespace
func (b *Build) CollectArtifacts() {
	for _, artPattern := range b.Job.Artifacts {
		pattern := b.GetWorkspaceDir() + artPattern
		files, err := doublestar.Glob(pattern)
		if err != nil {
			b.Logger.Println(err)
			continue
		}

		for _, f := range files {
			// Skip directories
			fi, err := os.Stat(f)
			if err != nil {
				b.Logger.Println(err)
				continue
			}
			if fi.IsDir() {
				continue
			}
			relPath := strings.TrimPrefix(f, b.GetWorkspaceDir())
			relDir, _ := filepath.Split(relPath)

			// Recreate folder structure relative to artifacts directory
			err = os.MkdirAll(b.GetArtifactsDir()+relDir, os.ModePerm)
			if err != nil {
				b.Logger.Println(err)
				continue
			}
			b.Logger.Printf("Copying artifact %s...\n", relPath)
			c := cmd.NewCmd("cp", f, b.GetArtifactsDir()+relPath)
			s := <-c.Start()
			if s.Exit != 0 {
				b.Logger.Printf("Unable to copy %s, code %d\n", f, s.Exit)
			} else {
				b.Artifacts = append(b.Artifacts, relPath)
			}
		}
	}
}

// BroadcastUpdate sends update to all subscribed clients. Contains general
// information about the build
func (b *Build) BroadcastUpdate() {
	data := b.GenerateBuildUpdateData()
	msg := MsgBroadcast{
		Type: "build:update:" + strconv.Itoa(b.ID),
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
		return hb.Put(Itob(data.ID), dataB)
	})
	if err != nil {
		b.Logger.Println(err)
	}
}

// GenerateBuildUpdateData generates BuildUpdateData
func (b *Build) GenerateBuildUpdateData() *BuildUpdateData {
	return &BuildUpdateData{
		ID:        b.ID,
		Name:      b.Job.Name,
		Status:    b.Status,
		Tasks:     b.GetTasksStatus(),
		Params:    b.Params,
		Artifacts: b.Artifacts,
		StartedAt: b.StartedAt,
		Duration:  b.Duration,
	}
}

// PublishCommandLogs sends log update to all subscribed users
func (b *Build) PublishCommandLogs(taskID int, data string) {
	msg := MsgBroadcast{
		Type: "build:log:" + strconv.Itoa(b.ID),
		Data: &CommandLogData{
			TaskID: taskID,
			Data:   data,
		},
	}
	BroadcastChannel <- &msg
}

// ProcessLogEntry adds duration and a new line to the log entry; stips out color info
func (b *Build) ProcessLogEntry(line string, startedAt time.Time) string {
	return fmt.Sprintf("[%10s] ", time.Since(startedAt).Truncate(time.Millisecond).String()) + StripColor(line) + "\n"
}

// GetWorkspaceDir returns path to the workspace, where all user created files
// are stored
func (b *Build) GetWorkspaceDir() string {
	return *WorkingDirFlag + "workspace/" + strconv.Itoa(b.ID) + "/"
}

// GetWakespaceDir returns path to the data dir - there all build+wake related data is
// stored
func (b *Build) GetWakespaceDir() string {
	return *WorkingDirFlag + "wakespace/" + strconv.Itoa(b.ID) + "/"
}

// GetArtifactsDir returns location of artifacts folder
func (b *Build) GetArtifactsDir() string {
	return b.GetWakespaceDir() + "artifacts/"
}

// GetBuildConfigFilename returns build config filename (copy of the original job file)
func (b *Build) GetBuildConfigFilename() string {
	return b.GetWakespaceDir() + "build.yaml"
}

// GetTasksStatus list of tasks with their status
func (b *Build) GetTasksStatus() []*TaskStatus {
	var info []*TaskStatus
	for _, t := range b.Job.Tasks {
		info = append(info, &TaskStatus{
			ID:        t.ID,
			Status:    t.Status,
			StartedAt: t.startedAt,
			Duration:  t.duration,
			Kind:      t.Kind,
		})
	}
	return info
}

// SetBuildStatus sets the status of the builds
func (b *Build) SetBuildStatus(status ItemStatus) {
	b.Logger.Printf("Status: %s\n", status)
	b.Status = status
	defer b.BroadcastUpdate()
	// Wait for pending task to finish before running anything else
	b.pendingTasksWG.Wait()
	switch status {
	case StatusPending:
		// Run onStatusTasks of kind pending in separate goroutine so it doesn't
		// slow down putting build into queue. Also it is expected to be something
		// really simple, like setting commit status in VCS
		go b.runOnStatusTasks(status)
		break
	case StatusRunning:
		b.StartedAt = time.Now()
		// Start timeout if available
		if b.Job.Timeout != "" {
			duration, err := time.ParseDuration(b.Job.Timeout)
			if err != nil {
				b.Logger.Println(err)
			} else {
				b.timer = time.NewTimer(duration)
				go func() {
					<-b.timer.C
					b.Logger.Printf("Build %d has timed out\n", b.ID)
					err = Q.Abort(b.ID)
					if err != nil {
						b.Logger.Println(err)
					}
				}()
			}
		}
		b.runOnStatusTasks(status)
		break
	case StatusAborted:
		b.runOnStatusTasks(status)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		break
	case StatusFailed:
		b.runOnStatusTasks(status)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		break
	case StatusFinished:
		b.runOnStatusTasks(status)
		b.Duration = time.Since(b.StartedAt)
		b.CollectArtifacts()
		b.Cleanup()
		break
	}
}

// CreateBuild creates Build instance and all necessary files and folders in wakespace
func CreateBuild(job *Job, jobPath string) (*Build, error) {
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
		Job:            job,
		ID:             counti,
		abortedChannel: make(chan bool),
		Params:         job.DefaultParams,
	}
	build.Logger = log.New(os.Stdout, fmt.Sprintf("[build #%d] ", build.ID), log.Lmicroseconds|log.Lshortfile)

	// Create workspace
	err = os.MkdirAll(build.GetWorkspaceDir(), os.ModePerm)
	if err != nil {
		build.Logger.Println(err)
		return nil, err
	}
	build.Logger.Printf("Workspace %s has been created\n", build.GetWorkspaceDir())

	// Create wakespace
	err = os.MkdirAll(build.GetWakespaceDir(), os.ModePerm)
	if err != nil {
		build.Logger.Println(err)
		return nil, err
	}
	build.Logger.Printf("Wakespace %s has been created\n", build.GetWakespaceDir())

	// Create artifacts dir
	err = os.MkdirAll(build.GetArtifactsDir(), os.ModePerm)
	if err != nil {
		build.Logger.Println(err)
		return nil, err
	}

	// Copy job config
	input, err := ioutil.ReadFile(jobPath)
	if err != nil {
		build.Logger.Println(err)
		return nil, err
	}

	err = ioutil.WriteFile(build.GetBuildConfigFilename(), input, os.ModePerm)
	if err != nil {
		build.Logger.Println(err)
		return nil, err
	}
	build.Logger.Printf("Build config %s has been created\n", build.GetBuildConfigFilename())

	build.SetBuildStatus(StatusPending)
	return &build, nil
}
