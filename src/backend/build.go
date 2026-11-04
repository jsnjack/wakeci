package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/sasha-s/go-deadlock"
	"gopkg.in/yaml.v2"

	"github.com/go-cmd/cmd"
	"github.com/joho/godotenv"
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

// StatusSkipped indicates that `when` condition is false and the task won't be
// executed
const StatusSkipped = "skipped"

// FinalTask is the task that is executed no matter what is the result of the build
const FinalTask = "finally"

// WHEN_EVAL_TIMEOUT is the timeout for evaluating `when` condition in tasks, s
const WHEN_EVAL_TIMEOUT = 3

// ABORT_TIMEOUT is the timeout for aborting the task, s
const ABORT_TIMEOUT = 5

const (
	privateBuildDirMode  os.FileMode = 0700
	privateBuildFileMode os.FileMode = 0600
)

// Build ...
type Build struct {
	ID             int
	Job            *Job
	Status         ItemStatus
	Logger         *slog.Logger
	flushChannel   chan bool // Instructs to flush bw
	pendingTasksWG sync.WaitGroup
	Params         []map[string]string
	Artifacts      []string // Deprecate
	BuildArtifacts []*ArtifactInfo
	StartedAt      time.Time
	Duration       time.Duration // ns
	ETA            int           // seconds
	timer          *time.Timer   // A timer for Job.Timeout
	mutex          deadlock.Mutex
	// startedWithAPIKey means the params came from an API key, so values the
	// job writes into build.env may be built from them
	startedWithAPIKey bool
	// failedTask is the main task that stopped the build, exposed to the
	// on_failed, on_aborted and finally tasks
	failedTask *Task
	// abort is the abort request for the tasks that run now, guarded by mutex
	abort *abortScope
}

// abortScope is one abort request. Its channel is closed when the abort is
// requested, and a closed channel stays readable: an abort that arrives while
// no command runs (setup, between tasks) is seen by the next check instead of
// being lost
type abortScope struct {
	reason string // Set once, before c is closed
	c      chan struct{}
}

func newAbortScope() *abortScope {
	return &abortScope{c: make(chan struct{})}
}

// stopped returns the abort reason, or "" if no abort was requested. Reading
// reason after receiving from the closed channel needs no lock
func (s *abortScope) stopped() string {
	select {
	case <-s.c:
		return s.reason
	default:
		return ""
	}
}

// currentAbort returns the abort scope of the tasks that run now
func (b *Build) currentAbort() *abortScope {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.abort == nil {
		b.abort = newAbortScope()
	}
	return b.abort
}

// requestAbort asks the build to stop the tasks that run now, with reason
// StatusAborted or StatusTimedOut. It never blocks; the first reason wins
func (b *Build) requestAbort(reason string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.abort == nil {
		b.abort = newAbortScope()
	}
	if b.abort.reason != "" {
		return
	}
	b.abort.reason = reason
	close(b.abort.c)
}

// nextAbortScope starts a new scope if the current one was aborted, so the
// next group of tasks (on_aborted, finally) runs. A scope that wasn't aborted
// is kept, so an abort that arrives between two groups stops the next one
func (b *Build) nextAbortScope() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.abort == nil || b.abort.reason != "" {
		b.abort = newAbortScope()
	}
}

// logAbort writes why the task was stopped to its log
func (b *Build) logAbort(reason string, bw *bufio.Writer, task *Task) {
	switch reason {
	case StatusTimedOut:
		b.ProcessLogEntry("> Timed out.", bw, task.ID, task.startedAt)
	case StatusAborted:
		b.ProcessLogEntry("> Aborted by a user.", bw, task.ID, task.startedAt)
	default:
		b.Logger.Warn("unhandled abort method", "task", task.ID, "reason", reason)
	}
}

// Start starts execution of tasks in job
func (b *Build) Start() {
	b.SetBuildStatus(StatusRunning)
	for _, task := range b.Job.Tasks {
		if task.Kind != KindMain {
			continue
		}
		// An abort that arrived during on_running tasks or between tasks
		if reason := b.currentAbort().stopped(); reason != "" {
			b.Logger.Info("build aborted between tasks", "reason", reason)
			b.SetBuildStatus(ItemStatus(reason))
			return
		}
		task.Status = StatusRunning
		task.startedAt = time.Now()
		b.BroadcastUpdate()

		status := b.runTask(task)

		task.Status = status
		task.duration = time.Since(task.startedAt)
		switch status {
		case StatusFailed, StatusAborted, StatusTimedOut:
			b.mutex.Lock()
			b.failedTask = task
			b.mutex.Unlock()
			b.SetBuildStatus(status)
			return
		}
		b.BroadcastUpdate()
	}
	b.SetBuildStatus(StatusFinished)
}

// runOnStatusTasks runs tasks on status change. When status is StatusPending,
// callers must call pendingTasksWG.Add(1) themselves before starting this in
// a goroutine (see SetBuildStatus) - a WaitGroup's Add must happen-before the
// matching Wait, which isn't guaranteed if Add runs inside the goroutine
// Wait is racing against.
func (b *Build) runOnStatusTasks(status ItemStatus) {
	if status == StatusPending {
		defer b.pendingTasksWG.Done()
	}
	abort := b.currentAbort()
	for _, task := range b.Job.Tasks {
		if task.Kind == string(status) {
			// An abort stops the rest of the group
			if reason := abort.stopped(); reason != "" {
				b.Logger.Info("skipping tasks after abort", "kind", status, "reason", reason)
				return
			}
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
	b.Logger.Debug("task started", "task", task.ID)
	defer b.Logger.Debug("task completed", "task", task.ID)
	abort := b.currentAbort()
	// Disable output buffering, enable streaming
	// Modify default streaming buffer size (thanks, webpack)
	cmdOptions := cmd.Options{
		Buffered:       false,
		Streaming:      true,
		LineBufferSize: 491520,
	}
	command, secretEnv := injectCommandSecrets(task.Command)
	taskCmd := cmd.NewCmdOptions(cmdOptions, "bash", "-c", command)

	// Configure task logs
	file, err := os.Create(b.taskLogPath(task.ID))
	bw := bufio.NewWriter(file)
	defer func() {
		if flushErr := bw.Flush(); flushErr != nil {
			b.Logger.Error("flush task log", "task", task.ID, "err", flushErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			b.Logger.Error("close task log", "task", task.ID, "err", closeErr)
		}
	}()
	if err != nil {
		b.Logger.Error("create task log", "task", task.ID, "err", err)
		return StatusFailed
	}

	// Construct environment for the task
	taskCmd.Env = os.Environ()
	taskCmd.Dir = b.GetWorkspaceDir()
	taskCmd.Env = append(taskCmd.Env, b.generateDefaultEnvVariables()...)
	b.mutex.Lock()
	for idx := range b.Params {
		for pkey, pval := range b.Params[idx] {
			taskCmd.Env = append(taskCmd.Env, fmt.Sprintf("%s=%s", pkey, injectSecrets(pval)))
		}
	}
	b.mutex.Unlock()

	for key, value := range task.Env {
		taskCmd.Env = append(taskCmd.Env, fmt.Sprintf("%s=%s", key, injectSecrets(value)))
	}

	envFile := b.GetWorkspaceDir() + "build.env"
	buidEnv, err := godotenv.Read(envFile)
	if err != nil {
		if !os.IsNotExist(err) {
			b.ProcessLogEntry("> Error in build.env file: "+err.Error(), bw, task.ID, task.startedAt)
			return StatusFailed
		}
	} else {
		for key, value := range buidEnv {
			// A job may write a param into build.env, and godotenv expands
			// variables, so a key could turn a param that isn't a secret
			// reference into one here
			if !b.startedWithAPIKey {
				value = injectSecrets(value)
			}
			taskCmd.Env = append(taskCmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	taskCmd.Env = append(taskCmd.Env, secretEnv...)

	// `when` is a bash test expression, `if` is a command
	if task.When != "" {
		if status := b.checkCondition(task, "when", task.When, "[[ %s ]]", taskCmd.Env, taskCmd.Dir, bw, abort); status != "" {
			return status
		}
	}
	if task.If != "" {
		if status := b.checkCondition(task, "if", task.If, "%s", taskCmd.Env, taskCmd.Dir, bw, abort); status != "" {
			return status
		}
	}

	// An abort that arrived during setup or the conditions
	if reason := abort.stopped(); reason != "" {
		b.logAbort(reason, bw, task)
		return ItemStatus(reason)
	}

	// Add executed command to logs
	b.ProcessLogEntry("> Running command: "+task.Command, bw, task.ID, task.startedAt)
	expandedTaskCmd := os.Expand(task.Command, getEnvMapper(taskCmd.Env))
	if expandedTaskCmd != task.Command {
		b.ProcessLogEntry(
			"> Expanded command: "+injectSecrets(expandedTaskCmd), bw, task.ID, task.startedAt,
		)
	}

	// Print STDOUT and STDERR lines streaming from Cmd
	// See example https://github.com/go-cmd/cmd/blob/master/examples/blocking-streaming/main.go
	doneChan := make(chan struct{})
	abortC := abort.c
	// Written by the goroutine below, read after doneChan is closed
	var abortedReason string
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
			case <-abortC:
				// The channel stays closed; stop selecting on it
				abortC = nil
				abortedReason = abort.reason
				b.Logger.Info("aborting task", "task", task.ID, "reason", abortedReason)
				b.logAbort(abortedReason, bw, task)
				// taskCmd.Stop() send SIGTERM signal to the command. Most of the time it works just fine, however
				// there are applications which will just ignore it or are in busy state and can't handle the signal.
				// Here we start a timer for SIGTERM to succeed and if it doesn't, SIGKILL is sent
				abortTimer := time.AfterFunc(ABORT_TIMEOUT*time.Second, func() {
					b.ProcessLogEntry("> Killing the command...", bw, task.ID, task.startedAt)
					// PID 0 means the command never started; kill(0) would
					// signal wakeci's own process group
					pid := taskCmd.Status().PID
					if pid <= 0 {
						return
					}
					if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
						b.Logger.Error("kill aborted task", "task", task.ID, "err", killErr)
					}
				})
				if err := taskCmd.Stop(); err != nil {
					b.Logger.Log(context.Background(), LevelTrace, "stop aborted task", "task", task.ID, "err", err)
				}
				go func() {
					// This call is blocking and should be executed outside of channel message handler
					<-taskCmd.Done()
					abortTimer.Stop()
				}()
			case <-b.flushChannel:
				b.Logger.Debug("flushing task log", "task", task.ID)
				if err := bw.Flush(); err != nil {
					b.Logger.Error("flush task log", "task", task.ID, "err", err)
				}
			}
		}
	}()

	// Run and wait for Cmd to return
	status := <-taskCmd.Start()
	b.Logger.Debug("task result",
		"task", task.ID, "completed", status.Complete, "exit", status.Exit, "err", status.Error,
	)

	// Cmd has finished but wait for goroutine to print all lines
	<-doneChan

	// The command was stopped because of an abort
	if abortedReason != "" {
		return ItemStatus(abortedReason)
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

// checkCondition evaluates a task's `when` or `if` condition, with script
// wrapping the condition into a bash command. It returns an empty status when
// the task should run, otherwise the status the task ends with
func (b *Build) checkCondition(task *Task, kind, expr, script string, env []string, dir string, bw *bufio.Writer, abort *abortScope) ItemStatus {
	condition, conditionSecretEnv := injectCommandSecrets(expr)
	condCmd := exec.Command("bash", "-c", fmt.Sprintf(script, condition))
	condCmd.Env = append(env, conditionSecretEnv...)
	condCmd.Dir = dir
	b.ProcessLogEntry("> Checking `"+kind+"` condition: "+expr, bw, task.ID, task.startedAt)
	expanded := os.Expand(expr, getEnvMapper(condCmd.Env))
	if expanded != expr {
		b.ProcessLogEntry("> Expanded condition: "+expanded, bw, task.ID, task.startedAt)
	}
	condErr := condCmd.Start()
	if condErr != nil {
		b.ProcessLogEntry(
			fmt.Sprintf("> Unable to evaluate the condition: %s", condErr.Error()),
			bw, task.ID, task.startedAt,
		)
		return StatusFailed
	}
	condTimedOut, condAborted, condErr := b.waitForCondition(condCmd, task.ID, WHEN_EVAL_TIMEOUT*time.Second, abort.c)
	if condAborted {
		reason := abort.stopped()
		b.logAbort(reason, bw, task)
		return ItemStatus(reason)
	}
	if condTimedOut {
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
		return StatusSkipped
	}
	b.ProcessLogEntry("> Condition is true", bw, task.ID, task.startedAt)
	return ""
}

// waitForCondition waits for the condition command. It kills the command
// when timeout passes (timedOut) or abort is closed (aborted)
func (b *Build) waitForCondition(condCmd *exec.Cmd, taskID int, timeout time.Duration, abort <-chan struct{}) (timedOut, aborted bool, err error) {
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- condCmd.Wait()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-waitDone:
		return false, false, err
	case <-timer.C:
		timedOut = true
	case <-abort:
		aborted = true
	}
	if err := condCmd.Process.Kill(); err != nil {
		b.Logger.Log(context.Background(), LevelTrace, "kill condition process", "task", taskID, "err", err)
	}
	return timedOut, aborted, <-waitDone
}

// Generate default set of environmental variables that are injected before
// running a task, for example WAKE_BUILD_ID
func (b *Build) generateDefaultEnvVariables() []string {
	params := url.Values{}
	b.mutex.Lock()
	for idx := range b.Params {
		for pkey, pval := range b.Params[idx] {
			params.Set(pkey, pval)
		}
	}
	failedTask := b.failedTask
	b.mutex.Unlock()
	var evs = []string{
		fmt.Sprintf("WAKE_BUILD_ID=%d", b.ID),
		fmt.Sprintf("WAKE_BUILD_WORKSPACE=%s", b.GetWorkspaceDir()),
		fmt.Sprintf("WAKE_JOB_NAME=%s", b.Job.Name),
		fmt.Sprintf("WAKE_JOB_PARAMS=%s", params.Encode()),
		fmt.Sprintf("WAKE_CONFIG_DIR=%s", Config.JobDir),
	}
	if failedTask != nil {
		evs = append(evs,
			fmt.Sprintf("WAKE_FAILED_TASK_ID=%d", failedTask.ID),
			fmt.Sprintf("WAKE_FAILED_TASK_NAME=%s", failedTask.Name),
			fmt.Sprintf("WAKE_FAILED_TASK_LOG=%s", b.taskLogPath(failedTask.ID)),
		)
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
		files, err := doublestar.FilepathGlob(pattern)
		if err != nil {
			b.Logger.Error("glob artifacts", "pattern", pattern, "err", err)
			continue
		}

		for _, f := range files {
			// Skip directories
			fi, err := os.Stat(f)
			if err != nil {
				b.Logger.Error("stat artifact", "file", f, "err", err)
				continue
			}
			if fi.IsDir() {
				continue
			}
			relPath := strings.TrimPrefix(f, b.GetWorkspaceDir())
			relDir, _ := filepath.Split(relPath)

			// Recreate folder structure relative to artifacts directory
			err = os.MkdirAll(b.GetArtifactsDir()+relDir, privateBuildDirMode)
			if err != nil {
				b.Logger.Error("create artifacts dir", "dir", b.GetArtifactsDir()+relDir, "err", err)
				continue
			}
			b.Logger.Debug("copying artifact", "path", relPath)
			artifactSize, err := copyArtifact(f, b.GetArtifactsDir()+relPath, fi.Mode())
			if err != nil {
				b.Logger.Error("copy artifact", "file", f, "err", err)
			} else {
				b.BuildArtifacts = append(b.BuildArtifacts, &ArtifactInfo{
					Size:     artifactSize,
					Filename: relPath,
				})
				b.Artifacts = append(b.Artifacts, relPath) // Deprecate
			}
		}
	}
}

func copyArtifact(source string, destination string, mode os.FileMode) (size int64, err error) {
	src, err := os.Open(source)
	if err != nil {
		return 0, fmt.Errorf("open artifact source: %w", err)
	}
	defer func() {
		if closeErr := src.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close artifact source: %w", closeErr)
		}
	}()

	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return 0, fmt.Errorf("open artifact destination: %w", err)
	}
	defer func() {
		if closeErr := dst.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close artifact destination: %w", closeErr)
		}
	}()

	bufferedDst := bufio.NewWriter(dst)
	if err := copyRedactedSecrets(bufferedDst, src); err != nil {
		return 0, fmt.Errorf("redact artifact: %w", err)
	}
	if err := bufferedDst.Flush(); err != nil {
		return 0, fmt.Errorf("flush artifact destination: %w", err)
	}
	info, err := dst.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat artifact destination: %w", err)
	}
	return info.Size(), nil
}

// BroadcastUpdate sends update to all subscribed clients. Contains general
// information about the build
func (b *Build) BroadcastUpdate() {
	bID := b.ID
	data, err := b.GenerateBuildUpdateData()
	if err != nil {
		b.Logger.Error("generate build update data", "err", err)
		return
	}
	msg := MsgBroadcast{
		Type: "build:update:" + strconv.Itoa(bID),
		Data: json.RawMessage(data),
	}
	WSHub.broadcast <- &msg

	err = DB.Update(func(tx *bolt.Tx) error {
		hb := tx.Bucket([]byte(HistoryBucket))
		return hb.Put(Itob(bID), data)
	})
	if err != nil {
		b.Logger.Error("save build history", "err", err)
	}
}

// GenerateBuildUpdateData generates BuildUpdateData
func (b *Build) GenerateBuildUpdateData() ([]byte, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	updData := &BuildUpdateData{
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
	// Return JSON to prevent concurrent map read/write issues
	return json.Marshal(updData)
}

// ProcessLogEntry handles log messages from tasks
func (b *Build) ProcessLogEntry(line string, buffer *bufio.Writer, taskID int, startedAt time.Time) {
	// Some commands (e.g. progress bars) redraw a single terminal line using
	// carriage returns instead of newlines. go-cmd only splits stdout/stderr
	// on newlines, so all of those redraws arrive here as one "line" with
	// embedded \r. Keep only the text after the last \r, i.e. what a real
	// terminal would end up displaying, instead of dumping every intermediate
	// redraw into the log.
	//
	// A trailing \r terminates the line rather than starting a redraw: under a
	// PTY the tty translates \n to \r\n, so treating it as a redraw marker
	// would discard the whole line.
	line = strings.TrimSuffix(line, "\r")
	if idx := strings.LastIndexByte(line, '\r'); idx != -1 {
		line = line[idx+1:]
	}

	// Format and clean up the log line:
	// - add duration and a new line to the log entry
	// - stip out color info
	// - redact servers from the log
	//
	// Note: Internal logs start with `>`
	pline := fmt.Sprintf("[%10s] ", time.Since(startedAt).Truncate(time.Millisecond).String()) + StripColor(redactSecrets(line)) + "\n"
	// Write to the task's log file
	_, err := buffer.WriteString(pline)
	if err != nil {
		b.Logger.Error("write task log", "task", taskID, "err", err)
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

// taskLogPath returns the path to the output log of the task
func (b *Build) taskLogPath(taskID int) string {
	return b.GetWakespaceDir() + fmt.Sprintf("task_%d.log", taskID)
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
	// Pending status tasks must finish before any fields for the next status
	// are changed or broadcast.
	b.pendingTasksWG.Wait()
	b.Logger.Info("build status", "status", status)
	b.Status = status
	if status == StatusRunning {
		b.StartedAt = time.Now()
	}
	switch status {
	case StatusPending:
		b.BroadcastUpdate()
		// Run onStatusTasks of kind pending in separate goroutine so it doesn't
		// slow down putting build into queue. Also it is expected to be something
		// really simple, like setting commit status in VCS
		b.pendingTasksWG.Add(1)
		go b.runOnStatusTasks(status)
	case StatusRunning:
		b.BroadcastUpdate()
		// Start timeout if available
		if b.Job.Timeout != "" {
			duration, err := time.ParseDuration(b.Job.Timeout)
			if err != nil {
				b.Logger.Error("parse job timeout", "timeout", b.Job.Timeout, "err", err)
			} else {
				// AfterFunc leaves no goroutine behind when Cleanup stops it
				b.timer = time.AfterFunc(duration, func() {
					b.Logger.Warn("build timed out", "build", b.ID)
					if abortErr := GlobalQueue.Abort(b.ID, StatusTimedOut); abortErr != nil {
						b.Logger.Error("abort timed out build", "build", b.ID, "err", abortErr)
					}
				})
			}
		}
		b.runOnStatusTasks(status)
	case StatusAborted, StatusTimedOut:
		// We run on_aborted handlers for builds aborted by a user or timed out.
		// The abort that stopped the build mustn't stop them too
		b.nextAbortScope()
		b.runOnStatusTasks(StatusAborted)
		b.nextAbortScope()
		b.runOnStatusTasks(FinalTask)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		b.BroadcastUpdate()
	case StatusFailed:
		b.runOnStatusTasks(status)
		b.CollectArtifacts()
		// An abort during the on_failed tasks stops them, not finally
		b.nextAbortScope()
		b.runOnStatusTasks(FinalTask)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		b.BroadcastUpdate()
	case StatusFinished:
		b.runOnStatusTasks(status)
		b.CollectArtifacts()
		// An abort during the on_finished tasks stops them, not finally
		b.nextAbortScope()
		b.runOnStatusTasks(FinalTask)
		b.Duration = time.Since(b.StartedAt)
		b.Cleanup()
		err := RecordBuildDuration(b.Job.Name, int(b.Duration))
		if err != nil {
			b.Logger.Error("record build duration", "err", err)
		}
		b.BroadcastUpdate()
	}

}

// CreateBuild creates Build instance and all necessary files and folders in wakespace
func CreateBuild(job *Job, jobPath string, startedWithAPIKey bool) (*Build, error) {
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
		return gb.Put([]byte("count"), []byte(strconv.Itoa(counti)))
	})
	if err != nil {
		return nil, err
	}

	build := Build{
		Job:          job,
		ID:           counti,
		abort:        newAbortScope(),
		flushChannel: make(chan bool),
		Params:       job.DefaultParams,
		ETA:          GetJobETA(job.Name),
		// Set before SetBuildStatus below starts the pending tasks
		startedWithAPIKey: startedWithAPIKey,
	}
	build.Logger = L.With("build", build.ID)

	// Create workspace
	err = os.MkdirAll(build.GetWorkspaceDir(), privateBuildDirMode)
	if err != nil {
		build.Logger.Error("create workspace", "dir", build.GetWorkspaceDir(), "err", err)
		return nil, err
	}
	build.Logger.Debug("workspace created", "dir", build.GetWorkspaceDir())

	// Create wakespace
	err = os.MkdirAll(build.GetWakespaceDir(), privateBuildDirMode)
	if err != nil {
		build.Logger.Error("create wakespace", "dir", build.GetWakespaceDir(), "err", err)
		return nil, err
	}
	build.Logger.Debug("wakespace created", "dir", build.GetWakespaceDir())

	// Create artifacts dir
	err = os.MkdirAll(build.GetArtifactsDir(), privateBuildDirMode)
	if err != nil {
		build.Logger.Error("create artifacts dir", "dir", build.GetArtifactsDir(), "err", err)
		return nil, err
	}

	input, err := yaml.Marshal(build.Job)
	if err != nil {
		build.Logger.Error("marshal build config", "err", err)
		return nil, err
	}

	err = os.WriteFile(build.GetBuildConfigFilename(), input, privateBuildFileMode)
	if err != nil {
		build.Logger.Error("write build config", "file", build.GetBuildConfigFilename(), "err", err)
		return nil, err
	}
	build.Logger.Debug("build config created", "file", build.GetBuildConfigFilename())

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
		// Iterate backwards as the last value will be the actual value
		for i := len(env) - 1; i >= 0; i-- {
			pair := strings.SplitN(env[i], "=", 2)
			if pair[0] == evar {
				return pair[1]
			}
		}
		return ""
	}
	return mapper
}
