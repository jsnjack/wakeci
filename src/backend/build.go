package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/sasha-s/go-deadlock"
	"gopkg.in/yaml.v2"

	"github.com/go-cmd/cmd"
	bolt "go.etcd.io/bbolt"
)

// ItemStatus handles information about the item status (currently is used for
// both Builds and Tasks) and type of OnStatus changed tasks
type ItemStatus string

// StatusRunning indicates the the build is in progress
const StatusRunning = "running"

// StatusFailed indicates that the build failed
const StatusFailed = "failed"

// StatusFinished indicates that the build is finished and is a success!
const StatusFinished = "finished"

// StatusPending indicates that the build is in the queue
const StatusPending = "pending"

// StatusAborted indicates that a build was manually aborted by user
const StatusAborted = "aborted"

// StatusTimedOut indicates that a build was automatically aborted because the
// `timeout` value was reached
const StatusTimedOut = "timed out"

// FinalTask is the task that is executed no matter what is the result of the build
const FinalTask = "finally"

// WHEN_EVAL_TIMEOUT is the timeout for evaluating `when` condition in tasks
const WHEN_EVAL_TIMEOUT = 3

// Build ...
type Build struct {
	ID             int
	Job            *Job
	Status         ItemStatus
	Logger         *log.Logger
	abortedChannel chan string
	flushChannel   chan bool // Instructs to flush bw
	pendingTasksWG sync.WaitGroup
	abortedReason  string
	Params         []map[string]string
	Artifacts      []string // Deprecate
	BuildArtifacts []*ArtifactInfo
	StartedAt      time.Time
	Duration       time.Duration // ns
	ETA            int           // seconds
	timer          *time.Timer   // A timer for Job.Timeout
	mutex          deadlock.Mutex
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
		case StatusFailed:
			b.SetBuildStatus(StatusFailed)
			return
		case StatusAborted:
			b.SetBuildStatus(StatusAborted)
			return
		case StatusTimedOut:
			b.SetBuildStatus(StatusTimedOut)
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
			b.BroadcastUpdate()

			status := b.runTask(task)

			task.Status = status
			task.duration = time.Since(task.startedAt)
			b.BroadcastUpdate()
		}
	}
}

// runTask is responsible for running one task and return it's status
func (b *Build) runTask(task *Task) ItemStatus {
	b.Logger.Printf("Task %d has been started\n", task.ID)
	defer b.Logger.Printf("Task %d is completed\n", task.ID)
	// Disable output buffering, enable streaming
	// Modify default streaming buffer size (thanks, webpack)
	cmdOptions := cmd.Options{
		Buffered:       false,
		Streaming:      true,
		LineBufferSize: 491520,
	}
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

	for key, value := range task.Env {
		taskCmd.Env = append(taskCmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Configure task logs
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
		b.Logger.Println(err)
		return StatusFailed
	}

	// Checking condition in when
	if task.When != "" {
		condCmd := exec.Command("bash", "-c", fmt.Sprintf("[[ %s ]]", task.When))
		condCmd.Env = taskCmd.Env
		condCmd.Dir = taskCmd.Dir
		b.ProcessLogEntry("> Checking `when` condition: "+task.When, bw, task.ID, task.startedAt)
		expandedCondCmd := os.Expand(task.When, getEnvMapper(condCmd.Env))
		if expandedCondCmd != task.When {
			b.ProcessLogEntry(
				"> Expanded condition: "+os.Expand(task.When, getEnvMapper(condCmd.Env)), bw, task.ID, task.startedAt,
			)
		}
		condErr := condCmd.Start()
		if condErr != nil {
			b.ProcessLogEntry(
				fmt.Sprintf("> Unable to evaluate the condition: %s", condErr.Error()),
				bw, task.ID, task.startedAt,
			)
			return StatusFailed
		}
		condKilled := false
		condTimer := time.AfterFunc(WHEN_EVAL_TIMEOUT*time.Second, func() {
			condKilled = true
			condCmd.Process.Kill()
		})
		condErr = condCmd.Wait()
		condTimer.Stop()
		if condKilled {
			b.ProcessLogEntry(
				fmt.Sprintf("> Condition timeouted: %s", condErr.Error()),
				bw, task.ID, task.startedAt,
			)
			return StatusFailed
		}
		if condErr != nil {
			b.ProcessLogEntry(
				fmt.Sprintf("> Condition is false: %s. Skipping the task", condErr.Error()),
				bw, task.ID, task.startedAt,
			)
			return StatusFinished
		} else {
			b.ProcessLogEntry("> Condition is true", bw, task.ID, task.startedAt)
		}
	}

	// Add executed command to logs
	b.ProcessLogEntry("> Running command: "+task.Command, bw, task.ID, task.startedAt)
	expandedTaskCmd := os.Expand(task.Command, getEnvMapper(taskCmd.Env))
	if expandedTaskCmd != task.Command {
		b.ProcessLogEntry(
			"> Expanded command: "+os.Expand(task.Command, getEnvMapper(taskCmd.Env)), bw, task.ID, task.startedAt,
		)
	}

	// Print STDOUT and STDERR lines streaming from Cmd
	// See example https://github.com/go-cmd/cmd/blob/master/examples/blocking-streaming/main.go
	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		for taskCmd.Stdout != nil || taskCmd.Stderr != nil {
			select {
			case line, open := <-taskCmd.Stdout:
				if !open {
					taskCmd.Stdout = nil
					continue
				}
				b.ProcessLogEntry(line, bw, task.ID, task.startedAt)
			case line, open := <-taskCmd.Stderr:
				if !open {
					taskCmd.Stderr = nil
					continue
				}
				b.ProcessLogEntry(line, bw, task.ID, task.startedAt)
			case abortedDetails := <-b.abortedChannel:
				b.abortedReason = abortedDetails
				b.Logger.Printf("Aborting via abortedChannel: %s\n", abortedDetails)
				switch abortedDetails {
				case StatusTimedOut:
					b.ProcessLogEntry("> Timed out.", bw, task.ID, task.startedAt)
				case StatusAborted:
					b.ProcessLogEntry("> Aborted by a user.", bw, task.ID, task.startedAt)
				default:
					b.Logger.Printf("Unhandled abort method: %s\n", abortedDetails)
				}
				taskCmd.Stop()
			case <-b.flushChannel:
				b.Logger.Println("Flushing log file...")
				bw.Flush()
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
	<-doneChan

	// Abort message was recieved via channel
	if b.abortedReason != "" {
		reason := b.abortedReason
		// Toggle status back for OnStatus tasks
		b.abortedReason = ""
		return ItemStatus(reason)
	}

	b.ProcessLogEntry(fmt.Sprintf("> Exit code: %d", status.Exit), bw, task.ID, task.startedAt)

	if !status.Complete || status.Exit != 0 || status.Error != nil {
		if task.IgnoreErrors {
			b.ProcessLogEntry("> Ignorring exit code", bw, task.ID, task.startedAt)
			return StatusFinished
		}
		return StatusFailed
	}

	return StatusFinished
}

// Generate default set of environmental variables that are injected before
// running a task, for example WAKE_BUILD_ID
func (b *Build) generateDefaultEnvVariables() []string {
	params := url.Values{}
	for idx := range b.Params {
		for pkey, pval := range b.Params[idx] {
			params.Set(pkey, pval)
		}
	}
	var evs = []string{
		fmt.Sprintf("WAKE_BUILD_ID=%d", b.ID),
		fmt.Sprintf("WAKE_BUILD_WORKSPACE=%s", b.GetWorkspaceDir()),
		fmt.Sprintf("WAKE_JOB_NAME=%s", b.Job.Name),
		fmt.Sprintf("WAKE_JOB_PARAMS=%s", params.Encode()),
		fmt.Sprintf("WAKE_CONFIG_DIR=%s", Config.JobDir),
	}
	if Config.Port == "443" {
		evs = append(evs, fmt.Sprintf("WAKE_URL=https://%s/", Config.Hostname))
	} else {
		evs = append(evs, fmt.Sprintf("WAKE_URL=http://localhost:%s/", Config.Port))
	}
	return evs
}

// Cleanup is called when a job finished, failed or aborted
func (b *Build) Cleanup() {
	if b.timer != nil {
		b.timer.Stop()
	}
	GlobalQueue.Remove(b.ID)
	GlobalQueue.Take()
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
				b.BuildArtifacts = append(b.BuildArtifacts, &ArtifactInfo{
					Size:     fi.Size(),
					Filename: relPath,
				})
				b.Artifacts = append(b.Artifacts, relPath) // Deprecate
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
	WSHub.broadcast <- &msg

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
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return &BuildUpdateData{
		ID:             b.ID,
		Name:           b.Job.Name,
		Status:         b.Status,
		Tasks:          b.GetTasksStatus(),
		Params:         b.Params,
		Artifacts:      b.Artifacts, // Deprecate
		BuildArtifacts: b.BuildArtifacts,
		StartedAt:      b.StartedAt,
		Duration:       b.Duration,
		ETA:            b.ETA,
	}
}

// ProcessLogEntry handles log messages from tasks
func (b *Build) ProcessLogEntry(line string, buffer *bufio.Writer, taskID int, startedAt time.Time) {
	// Format and clean up the log line:
	// - add duration and a new line to the log entry
	// - stip out color info
	//
	// Note: Internal logs start with `>`
	pline := fmt.Sprintf("[%10s] ", time.Since(startedAt).Truncate(time.Millisecond).String()) + StripColor(line) + "\n"
	// Write to the task's log file
	_, err := buffer.WriteString(pline)
	if err != nil {
		b.Logger.Println(err)
	}

	// Send the log to all subscribed users
	msg := MsgBroadcast{
		Type: "build:log:" + strconv.Itoa(b.ID),
		Data: &CommandLogData{
			TaskID: taskID,
			Data:   pline,
		},
	}
	WSHub.broadcast <- &msg
}

// GetWorkspaceDir returns path to the workspace, where all user created files
// are stored
func (b *Build) GetWorkspaceDir() string {
	return Config.WorkDir + "workspace/" + strconv.Itoa(b.ID) + "/"
}

// GetWakespaceDir returns path to the data dir - there all build+wake related data is
// stored
func (b *Build) GetWakespaceDir() string {
	return Config.WorkDir + "wakespace/" + strconv.Itoa(b.ID) + "/"
}

// GetArtifactsDir returns location of artifacts folder
func (b *Build) GetArtifactsDir() string {
	return b.GetWakespaceDir() + "artifacts/"
}

// GetBuildConfigFilename returns build config filename (copy of the original job file)
func (b *Build) GetBuildConfigFilename() string {
	return b.GetWakespaceDir() + "build_plan" + Config.jobsExt
}

// GetTasksStatus list of tasks with their status
func (b *Build) GetTasksStatus() []*TaskStatus {
	info := make([]*TaskStatus, 0)
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
	if status == StatusRunning {
		b.StartedAt = time.Now()
	}
	// Wait for pending task to finish before running anything else
	b.pendingTasksWG.Wait()
	switch status {
	case StatusPending:
		b.BroadcastUpdate()
		// Run onStatusTasks of kind pending in separate goroutine so it doesn't
		// slow down putting build into queue. Also it is expected to be something
		// really simple, like setting commit status in VCS
		go b.runOnStatusTasks(status)
	case StatusRunning:
		b.BroadcastUpdate()
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
					err = GlobalQueue.Abort(b.ID, StatusTimedOut)
					if err != nil {
						b.Logger.Println(err)
					}
				}()
			}
		}
		b.runOnStatusTasks(status)
	case StatusAborted, StatusTimedOut:
		// We run on_aborted handlers for builds aborted by a user or timed out
		b.runOnStatusTasks(StatusAborted)
		b.runOnStatusTasks(FinalTask)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		b.BroadcastUpdate()
	case StatusFailed:
		b.runOnStatusTasks(status)
		b.CollectArtifacts()
		b.runOnStatusTasks(FinalTask)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		b.BroadcastUpdate()
	case StatusFinished:
		b.runOnStatusTasks(status)
		b.CollectArtifacts()
		b.runOnStatusTasks(FinalTask)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		err := RecordBuildDuration(b.Job.Name, int(b.Duration))
		if err != nil {
			b.Logger.Println(err)
		}
		b.BroadcastUpdate()
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
		abortedChannel: make(chan string),
		flushChannel:   make(chan bool),
		Params:         job.DefaultParams,
		ETA:            GetJobETA(job.Name),
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

	input, err := yaml.Marshal(build.Job)
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

// ArtifactInfo represents build artifacts
type ArtifactInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// Used to expand env variables in commands
func getEnvMapper(env []string) func(string) string {
	mapper := func(evar string) string {
		for _, e := range env {
			pair := strings.SplitN(e, "=", 2)
			if pair[0] == evar {
				return pair[1]
			}
		}
		return ""
	}
	return mapper
}
