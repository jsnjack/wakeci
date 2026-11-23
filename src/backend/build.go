package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	pendingTasksWG sync.WaitGroup
	Params         []map[string]string
	Artifacts      []string // Deprecate
	BuildArtifacts []*ArtifactInfo
	StartedAt      time.Time
	Duration       time.Duration // ns
	ETA            int           // ns, see BuildUpdateData.ETA
	timer          *time.Timer   // A timer for Job.Timeout
	mutex          deadlock.Mutex
	// untrustedParams means the params came from someone who may not edit
	// jobs (an API key, or a single sign-on user without write), so values
	// the job writes into build.env may be built from them
	untrustedParams bool
	// queuedAt is when the build was created, and trigger who or what
	// started it, both for its stats. Guarded by mutex
	queuedAt time.Time
	trigger  buildTrigger
	// ended is set once the build's last tasks ran and Duration is final.
	// Guarded by mutex
	ended bool
	// failedTask is the main task that stopped the build, exposed to the
	// on_failed, on_aborted and finally tasks
	failedTask *Task
	// liveLog is the log of the task that runs now, nil between tasks
	liveLog atomic.Pointer[taskLogWriter]
	// logBatch holds the log lines that wait to be sent to the browsers
	logBatch logBatch
	// abort is the abort request for the tasks that run now, guarded by mutex
	abort *abortScope
	// abortedWhileQueued is set by Queue.Abort for a queued build: it has no
	// queue record from then on, although it is pending until its on_pending
	// tasks end. Guarded by mutex
	abortedWhileQueued bool
	// interrupted is set for a build that wakeci stopped without ending,
	// which FinishInterruptedBuilds restored to run the tasks it has left.
	// Set before the build is shared, never changed
	interrupted bool
	// keys are the build's cancel and concurrency keys (cancel.go), set
	// before it is queued, never changed
	keys queueKeys
	// cancelled are the builds this one cancelled as it was queued, and
	// supersededBy the newer build that cancelled it then; for the run's
	// audit event, written by the goroutine that queues the build
	cancelled    []int
	supersededBy int
	// disk is the build's size on disk, measured as it ends. Guarded by mutex
	disk *buildDisk
	// group is the build that started this one and the root of its chain,
	// set before the build's tasks start. Guarded by mutex
	group buildGroup
	// token is the build's WAKE_TOKEN, created when its first task starts,
	// and tokenRevoked is set once the build left the queue (buildtoken.go).
	// Guarded by mutex
	token        string
	tokenRevoked bool
	// updateMu is held by BroadcastUpdate from taking the build's state until
	// it is saved, so updates are saved in the order they were taken: an
	// update taken while queued, saved after a later one, would queue the
	// build again after a restart. Not a deadlock.Mutex, whose 30 s limit
	// would stop wakeci while a slow database write holds it
	updateMu sync.Mutex
}

// abortScope is one abort request. Its channel is closed when the abort is
// requested, and a closed channel stays readable: an abort that arrives while
// no command runs (setup, between tasks) is seen by the next check instead of
// being lost
type abortScope struct {
	reason string // Set once, before c is closed
	// note says why, when neither a user nor the job timeout asked for it.
	// Set with reason
	note string
	// by completes "Aborted …" with who asked for it, e.g. "by
	// sam@example.com"; empty if unknown. Set with reason
	by string
	c  chan struct{}
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
	return b.abortLocked()
}

// abortLocked returns the current scope, creating it for a Build made
// without CreateBuild. The caller holds b.mutex
func (b *Build) abortLocked() *abortScope {
	if b.abort == nil {
		b.abort = newAbortScope()
	}
	return b.abort
}

// requestAbort asks the build to stop the tasks that run now, with reason
// StatusAborted or StatusTimedOut, and note for the task log if neither a
// user nor the timeout asked for it. It never blocks; the first reason wins
func (b *Build) requestAbort(reason, note, by string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	scope := b.abortLocked()
	if scope.reason != "" {
		return
	}
	scope.reason = reason
	scope.note = note
	scope.by = by
	close(scope.c)
}

// nextAbortScope starts a new scope if the current one was aborted, so the
// next group of tasks (on_aborted, finally) runs. A scope that wasn't aborted
// is kept, so an abort that arrives between two groups stops the next one
func (b *Build) nextAbortScope() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.abortLocked().reason != "" {
		b.abort = newAbortScope()
	}
}

// logAbort writes why the task was stopped to its log. The scope must be
// aborted
func (b *Build) logAbort(scope *abortScope, bw *taskLogWriter, task *Task) {
	switch {
	case scope.note != "" && scope.by != "":
		b.ProcessLogEntry("> Aborted "+scope.by+": "+scope.note+".", bw, task.ID, task.startedAt)
	case scope.note != "":
		b.ProcessLogEntry("> Aborted: "+scope.note+".", bw, task.ID, task.startedAt)
	case scope.reason == StatusTimedOut:
		b.ProcessLogEntry("> Timed out.", bw, task.ID, task.startedAt)
	case scope.reason == StatusAborted && scope.by != "":
		b.ProcessLogEntry("> Aborted "+scope.by+".", bw, task.ID, task.startedAt)
	case scope.reason == StatusAborted:
		b.ProcessLogEntry("> Aborted by a user.", bw, task.ID, task.startedAt)
	default:
		b.Logger.Warn("unhandled abort method", "task", task.ID, "reason", scope.reason)
	}
}

// Start starts execution of tasks in job
func (b *Build) Start() {
	b.SetBuildStatus(StatusRunning)
	for _, task := range b.Job.Tasks {
		if task.Kind != KindMain {
			continue
		}
		if b.stopIfAborted() {
			return
		}
		b.taskStarted(task)
		b.BroadcastUpdate()

		status := b.runTask(task)

		b.taskEnded(task, status)
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
	// After the last main task, or in a job without main tasks
	if b.stopIfAborted() {
		return
	}
	b.SetBuildStatus(StatusFinished)
}

// stopIfAborted ends the build with the abort reason if an abort arrived
// during the on_running tasks or outside a main task's command
func (b *Build) stopIfAborted() bool {
	reason := b.currentAbort().stopped()
	if reason == "" {
		return false
	}
	b.Logger.Info("build aborted between tasks", "reason", reason)
	b.SetBuildStatus(ItemStatus(reason))
	return true
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
		// A task of an interrupted build that started before wakeci stopped
		// doesn't run again: it may have done part of its work
		if b.interrupted && !task.startedAt.IsZero() {
			continue
		}
		if task.Kind == string(status) {
			// An abort stops the rest of the group
			if reason := abort.stopped(); reason != "" {
				b.Logger.Info("skipping tasks after abort", "kind", status, "reason", reason)
				return
			}
			b.taskStarted(task)
			b.BroadcastUpdate()

			status := b.runTask(task)

			b.taskEnded(task, status)
			b.BroadcastUpdate()
		}
	}
}

// taskStarted marks task as running. Under b.mutex, since an update from
// another goroutine (RunJob, RestoreQueue) reads it during the on_pending
// tasks
func (b *Build) taskStarted(task *Task) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	task.Status = StatusRunning
	task.startedAt = time.Now()
}

// taskEnded records how task ended and how long it ran, under b.mutex like
// taskStarted
func (b *Build) taskEnded(task *Task, status ItemStatus) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	task.Status = status
	task.duration = time.Since(task.startedAt)
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
	if err != nil {
		b.Logger.Error("create task log", "task", task.ID, "err", err)
		return StatusFailed
	}
	bw := &taskLogWriter{file: file, w: bufio.NewWriter(file), path: b.taskLogPath(task.ID), ansiPath: b.taskANSILogPath(task.ID)}
	b.liveLog.Store(bw)
	defer func() {
		// The task's last lines go out before its status changes
		b.flushLog()
		b.liveLog.CompareAndSwap(bw, nil)
		if closeErr := bw.Close(); closeErr != nil {
			b.Logger.Error("close task log", "task", task.ID, "err", closeErr)
		}
	}()
	// wakeci exits next (Queue.stopTasks), and nothing would wait for it
	if GlobalQueue.tasksStoppedNow() {
		b.ProcessLogEntry("> Not started: "+stopNote+".", bw, task.ID, task.startedAt)
		return StatusAborted
	}
	if b.interrupted {
		b.ProcessLogEntry("> "+interruptedRunNote+".", bw, task.ID, task.startedAt)
	}

	// Construct environment for the task
	taskCmd.Env = os.Environ()
	taskCmd.Dir = b.GetWorkspaceDir()
	taskCmd.Env = append(taskCmd.Env, b.generateDefaultEnvVariables()...)
	// Before the params and the task's env, which may set them otherwise
	if b.Job.Color || task.Color {
		taskCmd.Env = append(taskCmd.Env, colorEnv...)
	}
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
			if !b.untrustedParams {
				value = injectSecrets(value)
			}
			taskCmd.Env = append(taskCmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	taskCmd.Env = append(taskCmd.Env, secretEnv...)
	// Last, so a task's env or build.env can't change them
	taskCmd.Env = append(taskCmd.Env, b.buildStateEnvVariables()...)

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
		b.logAbort(abort, bw, task)
		return ItemStatus(reason)
	}

	// Add executed command to logs
	b.ProcessLogEntry("> Running command: "+task.Command, bw, task.ID, task.startedAt)
	expandedTaskCmd := os.Expand(task.Command, logEnvMapper(taskCmd.Env))
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
	var abortedAt time.Time
	// The abort's kill timer closes killed when it sends SIGKILL. The loop
	// below writes the note, since it alone writes to bw while the command
	// runs; the timer used to write from its own goroutine.
	killed := make(chan struct{})
	go func() {
		defer close(doneChan)
		killedC := killed
		for taskCmd.Stdout != nil || taskCmd.Stderr != nil {
			select {
			case <-killedC:
				killedC = nil
				b.ProcessLogEntry("> Killing the command...", bw, task.ID, task.startedAt)
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
				abortedAt = time.Now()
				b.Logger.Info("aborting task", "task", task.ID, "reason", abortedReason)
				b.logAbort(abort, bw, task)
				// taskCmd.Stop() send SIGTERM signal to the command. Most of the time it works just fine, however
				// there are applications which will just ignore it or are in busy state and can't handle the signal.
				// Here we start a timer for SIGTERM to succeed and if it doesn't, SIGKILL is sent
				abortTimer := time.AfterFunc(ABORT_TIMEOUT*time.Second, func() {
					close(killed)
					// PID 0 means the command never started; kill(0) would
					// signal wakeci's own process group
					pid := taskCmd.Status().PID
					if pid <= 0 {
						return
					}
					// The whole group (go-cmd starts the shell as its leader):
					// a child that ignores SIGTERM outlives the shell, and one
					// holding the output open keeps the task from ending
					if killErr := syscall.Kill(-pid, syscall.SIGKILL); killErr != nil {
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
		b.killTaskGroup(taskCmd.Status().PID, abortedAt.Add(ABORT_TIMEOUT*time.Second), task.ID)
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

// killTaskGroup ends what is left of an aborted task once its shell (pid,
// the leader of the task's process group) ended: a child that ignores
// SIGTERM lives on in the group, without holding the task's output open. It
// gets until deadline, like the shell, then SIGKILL
func (b *Build) killTaskGroup(pid int, deadline time.Time, taskID int) {
	// PID 0 means the command never started; kill(0) would signal wakeci's
	// own process group
	if pid <= 0 {
		return
	}
	// Only while the group has members: once it is empty, its ID may be
	// reused by another task's group
	for syscall.Kill(-pid, 0) == nil {
		if !time.Now().Before(deadline) {
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				b.Logger.Error("kill what is left of an aborted task", "task", taskID, "err", err)
			}
			return
		}
		time.Sleep(stopPollInterval)
	}
}

// checkCondition evaluates a task's `when` or `if` condition, with script
// wrapping the condition into a bash command. It returns an empty status when
// the task should run, otherwise the status the task ends with
func (b *Build) checkCondition(task *Task, kind, expr, script string, env []string, dir string, bw *taskLogWriter, abort *abortScope) ItemStatus {
	condition, conditionSecretEnv := injectCommandSecrets(expr)
	condCmd := conditionCommand(fmt.Sprintf(script, condition))
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
		b.logAbort(abort, bw, task)
		return ItemStatus(abort.stopped())
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

// conditionCommand returns the command that evaluates a condition script, in
// a process group of its own, so waitForCondition's kill stops its children
// too
func conditionCommand(script string) *exec.Cmd {
	condCmd := exec.Command("bash", "-c", script)
	condCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return condCmd
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
	for !timedOut && !aborted {
		select {
		case err := <-waitDone:
			return false, false, err
		case <-timer.C:
			timedOut = true
		case <-abort:
			aborted = true
		}
	}
	select {
	case err = <-waitDone:
		// It ended just as the timer fired or the abort arrived. Wait reaped
		// it, so its group ID may already belong to another task's group
	default:
		// Not reaped yet, so its group ID is still its own
		if err := syscall.Kill(-condCmd.Process.Pid, syscall.SIGKILL); err != nil {
			b.Logger.Log(context.Background(), LevelTrace, "kill condition process", "task", taskID, "err", err)
		}
		err = <-waitDone
	}
	// The condition exited on its own just as the timer fired
	if timedOut && err == nil {
		return false, false, nil
	}
	return timedOut, aborted, err
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
	b.mutex.Unlock()
	var evs = []string{
		fmt.Sprintf("WAKE_BUILD_ID=%d", b.ID),
		fmt.Sprintf("WAKE_BUILD_WORKSPACE=%s", b.GetWorkspaceDir()),
		fmt.Sprintf("WAKE_JOB_NAME=%s", b.Job.Name),
		fmt.Sprintf("WAKE_JOB_PARAMS=%s", params.Encode()),
		fmt.Sprintf("WAKE_CONFIG_DIR=%s", Config.JobDir),
		fmt.Sprintf("WAKE_SUMMARY=%s", b.GetWakespaceDir()+jobSummaryFile),
	}
	if parent := b.Group().Parent; parent != 0 {
		evs = append(evs, fmt.Sprintf("WAKE_PARENT_ID=%d", parent))
	}
	if Config.Port == "443" {
		evs = append(evs, fmt.Sprintf("WAKE_URL=https://%s/", Config.Hostname))
	} else {
		evs = append(evs, fmt.Sprintf("WAKE_URL=http://localhost:%s/", Config.Port))
	}
	if token, err := b.buildToken(); err != nil {
		b.Logger.Error("create build token", "err", err)
	} else if token != "" {
		evs = append(evs, "WAKE_TOKEN="+token)
	}
	// The wake command, first, so another wake on the PATH doesn't hide it
	if wakeCommandDir != "" {
		path := wakeCommandDir
		// An empty entry would add the workspace to the PATH
		if inherited := os.Getenv("PATH"); inherited != "" {
			path += string(os.PathListSeparator) + inherited
		}
		evs = append(evs, "PATH="+path)
	}
	return evs
}

// buildStateEnvVariables returns the build's status and the task that stopped
// it, which hooks use to decide what to report
func (b *Build) buildStateEnvVariables() []string {
	b.mutex.Lock()
	status := b.Status
	failedTask := b.failedTask
	b.mutex.Unlock()
	evs := []string{
		fmt.Sprintf("WAKE_BUILD_STATUS=%s", status),
		fmt.Sprintf("WAKE_INTERRUPTED=%t", b.interrupted),
	}
	if failedTask != nil {
		evs = append(evs,
			fmt.Sprintf("WAKE_FAILED_TASK_ID=%d", failedTask.ID),
			fmt.Sprintf("WAKE_FAILED_TASK_NAME=%s", failedTask.Name),
			fmt.Sprintf("WAKE_FAILED_TASK_LOG=%s", b.taskLogPath(failedTask.ID)),
		)
	}
	return evs
}

// Cleanup is called when a job finished, failed or aborted
func (b *Build) Cleanup() {
	if b.timer != nil {
		b.timer.Stop()
	}
	GlobalQueue.Remove(b.ID)
	b.revokeBuildToken()
	// Waiting requests take a build as ended once it left the queue
	buildChanges.notify(b.ID)
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
				b.mutex.Lock()
				b.BuildArtifacts = append(b.BuildArtifacts, &ArtifactInfo{
					Size:     artifactSize,
					Filename: relPath,
				})
				b.Artifacts = append(b.Artifacts, relPath) // Deprecate
				b.mutex.Unlock()
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
// information about the build. It saves the build in the history, in
// QueueBucket while the build is queued, and in StartedBucket from then
// until it ends
func (b *Build) BroadcastUpdate() {
	b.updateMu.Lock()
	defer b.updateMu.Unlock()
	bID := b.ID
	data, queued, started, stats, err := b.records()
	if err != nil {
		b.Logger.Error("generate build update", "err", err)
		return
	}
	root := b.Group().Root
	msg := MsgBroadcast{
		Type: "build:update:" + strconv.Itoa(bID),
		Data: json.RawMessage(data),
		Job:  b.Job.Name,
	}
	WSHub.broadcast <- &msg

	err = DB.Update(func(tx *bolt.Tx) error {
		hb := tx.Bucket([]byte(HistoryBucket))
		if err := hb.Put(Itob(bID), data); err != nil {
			return err
		}
		if err := putBuildStats(tx, stats, bID); err != nil {
			return err
		}
		if err := putGroupMember(tx, root, bID); err != nil {
			return err
		}
		if err := putOrDelete(tx.Bucket(QueueBucket), bID, queued); err != nil {
			return err
		}
		return putOrDelete(tx.Bucket(StartedBucket), bID, started)
	})
	if err != nil {
		b.Logger.Error("save build history", "err", err)
	}
	buildChanges.notify(bID)
}

// putOrDelete saves value under the build's ID in bucket, or deletes the key
// if value is nil
func putOrDelete(bucket *bolt.Bucket, id int, value []byte) error {
	if value == nil {
		return bucket.Delete(Itob(id))
	}
	return bucket.Put(Itob(id), value)
}

// records returns the build's history entry (BuildUpdateData), its stats
// record and, while it is queued, what RestoreQueue needs to queue it again,
// nil once it left the queue. From then until the build ended, started is
// what FinishInterruptedBuilds needs to run the tasks it has left. All
// describe the same moment
func (b *Build) records() (data, queued, started, stats []byte, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	// JSON, so the caller doesn't read the build's maps while it changes
	data, err = json.Marshal(&BuildUpdateData{
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
		QueuedAt:       b.queuedAt,
		Parent:         b.group.Parent,
		Root:           b.group.Root,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	failedTask := ""
	if b.failedTask != nil {
		failedTask = displayTaskName(b.failedTask)
	}
	stats, err = json.Marshal(&buildStats{
		ID:         b.ID,
		Job:        b.Job.Name,
		Status:     b.Status,
		QueuedAt:   b.queuedAt,
		StartedAt:  b.StartedAt,
		Duration:   b.Duration,
		Ended:      b.ended,
		Trigger:    b.trigger,
		FailedTask: failedTask,
		Parent:     b.group.Parent,
		Root:       b.group.Root,
		Disk:       b.disk,
	})
	if err != nil {
		return data, nil, nil, stats, err
	}
	saved := queuedBuild{
		Job:             b.Job,
		Params:          b.Params,
		UntrustedParams: b.untrustedParams,
		QueuedAt:        b.queuedAt,
		Trigger:         b.trigger,
		// So a rollback to wakeci 2.41, which reads only this name, keeps it
		StartedWithAPIKey: b.untrustedParams,
		Tasks:             b.GetTasksStatus(),
		Parent:            b.group.Parent,
		Root:              b.group.Root,
	}
	switch {
	case b.Status == StatusPending && !b.abortedWhileQueued:
		queued, err = json.Marshal(saved)
	// A build aborted while queued is pending until its on_pending tasks
	// end, and has no record then
	case b.Status != StatusPending && !b.ended:
		record := startedBuild{queuedBuild: saved, Status: b.Status, StartedAt: b.StartedAt}
		if b.failedTask != nil {
			record.FailedTask = &b.failedTask.ID
		}
		started, err = json.Marshal(record)
	}
	return data, queued, started, stats, err
}

// taskLogWriter is a task's log file behind a buffer. The task writes its lines
// and FlushLogs writes the buffer out for a request, so both lock. A flush
// used to be a message to the task's output loop, dropped while the loop was
// busy, which it is while output is heavy: it waits on the hub to take
// each line.
type taskLogWriter struct {
	mu     sync.Mutex
	file   *os.File
	w      *bufio.Writer
	closed bool
	// lines counts the lines written, so each write knows its line number
	lines int
	// path is the log's, and ansiPath the colour log's, which ansi writes
	// once a line has colours; empty ansiPath writes none
	path      string
	ansiPath  string
	ansiFile  *os.File
	ansi      *bufio.Writer
	ansiFails bool
}

// writeLines writes plain, whole lines, to the log, and ansi, the same
// lines with their colours, to the colour log, which starts at the first
// line that has any (colored) with the lines before it. It returns the
// number of plain's first line, counting from 1: one entry can be several
// lines, such as a command of several lines that the log repeats.
func (l *taskLogWriter) writeLines(plain, ansi string, colored bool) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	if colored && l.ansi == nil && l.ansiPath != "" && !l.ansiFails {
		if err := l.startANSILocked(); err != nil {
			// The plain log goes on; the page shows it without colours
			l.ansiFails = true
			L.Error("start colour log", "path", l.ansiPath, "err", err)
		}
	}
	first := l.lines + 1
	l.lines += strings.Count(plain, "\n")
	if _, err := l.w.WriteString(plain); err != nil {
		return first, err
	}
	if l.ansi != nil {
		if _, err := l.ansi.WriteString(ansi); err != nil {
			return first, err
		}
	}
	return first, nil
}

// startANSILocked creates the colour log with the lines the log has so far
func (l *taskLogWriter) startANSILocked() error {
	if err := l.w.Flush(); err != nil {
		return err
	}
	before, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(l.ansiPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, privateBuildFileMode)
	if err != nil {
		return err
	}
	l.ansiFile, l.ansi = file, bufio.NewWriter(file)
	_, err = l.ansi.Write(before)
	return err
}

// Flush writes out the buffered lines; it does nothing once the log is
// closed, since Close wrote them
func (l *taskLogWriter) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	if l.ansi != nil {
		if err := l.ansi.Flush(); err != nil {
			return err
		}
	}
	return l.w.Flush()
}

// Close writes out the buffered lines and closes the file
func (l *taskLogWriter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	err := errors.Join(l.w.Flush(), l.file.Close())
	if l.ansi != nil {
		err = errors.Join(err, l.ansi.Flush(), l.ansiFile.Close())
	}
	return err
}

// flushLiveLog writes out the log of the task that runs now, so the file
// holds every line sent so far
func (b *Build) flushLiveLog() {
	if log := b.liveLog.Load(); log != nil {
		if err := log.Flush(); err != nil {
			b.Logger.Error("flush task log", "err", err)
		}
	}
}

// ProcessLogEntry handles log messages from tasks
func (b *Build) ProcessLogEntry(line string, buffer *taskLogWriter, taskID int, startedAt time.Time) {
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
	prefix := fmt.Sprintf("[%10s] ", time.Since(startedAt).Truncate(time.Millisecond).String())
	plain, ansi, colored := colorLine(line)
	pline, aline := prefix+plain+"\n", prefix+ansi+"\n"
	// Write to the task's log files
	first, err := buffer.writeLines(pline, aline, colored)
	if err != nil {
		b.Logger.Error("write task log", "task", taskID, "err", err)
		return
	}
	// Send the log to all subscribed users
	b.queueLog(taskID, first, pline, aline, colored)
}

// logBatchDelay is how long a task's log lines wait to be sent together,
// and logBatchMax how much text a message holds at most
const (
	logBatchDelay = 100 * time.Millisecond
	logBatchMax   = 64 << 10
)

// logBatch collects a task's log lines for logBatchDelay, so a burst of
// output reaches the browsers in a few messages instead of one a line, which
// filled a client's queue of 1024 and disconnected it
type logBatch struct {
	mu    sync.Mutex
	task  int
	first int // the number of the batch's first line in the task's log
	text  strings.Builder
	// ansi is text with its colours, sent only if a line had any (colored)
	ansi    strings.Builder
	colored bool
	timer   *time.Timer
}

// queueLog adds whole lines of task's log, the first numbered first, to the
// lines that wait to be sent: text, and ansi with their colours if colored
func (b *Build) queueLog(task, first int, text, ansi string, colored bool) {
	batch := &b.logBatch
	var ready []*MsgBroadcast
	batch.mu.Lock()
	if batch.text.Len() > 0 && batch.task != task {
		ready = append(ready, b.takeLogBatchLocked())
	}
	if batch.text.Len() == 0 {
		batch.task, batch.first = task, first
	}
	batch.text.WriteString(text)
	batch.ansi.WriteString(ansi)
	batch.colored = batch.colored || colored
	if batch.text.Len() >= logBatchMax {
		ready = append(ready, b.takeLogBatchLocked())
	} else if batch.timer == nil {
		batch.timer = time.AfterFunc(logBatchDelay, b.flushLog)
	}
	batch.mu.Unlock()
	for _, msg := range ready {
		WSHub.broadcast <- msg
	}
}

// flushLog sends the lines that wait, at once
func (b *Build) flushLog() {
	batch := &b.logBatch
	batch.mu.Lock()
	if batch.text.Len() == 0 {
		batch.timer = nil
		batch.mu.Unlock()
		return
	}
	msg := b.takeLogBatchLocked()
	batch.mu.Unlock()
	WSHub.broadcast <- msg
}

// takeLogBatchLocked empties the batch into a message; batch.mu is held
func (b *Build) takeLogBatchLocked() *MsgBroadcast {
	batch := &b.logBatch
	if batch.timer != nil {
		batch.timer.Stop()
		batch.timer = nil
	}
	data := &CommandLogData{TaskID: batch.task, ID: batch.first, Data: batch.text.String()}
	if batch.colored {
		data.ANSI = batch.ansi.String()
	}
	msg := &MsgBroadcast{Type: "build:log:" + strconv.Itoa(b.ID), Data: data, Job: b.Job.Name}
	batch.text.Reset()
	batch.ansi.Reset()
	batch.colored = false
	return msg
}

// GetWorkspaceDir returns path to the workspace, where all user created files
// are stored
func (b *Build) GetWorkspaceDir() string {
	return Config.WorkDir + "workspace/" + strconv.Itoa(b.ID) + "/"
}

// wakespaceDir is where build id keeps its task logs, artifacts, plan and
// summary, ending in a slash. Every path into it starts here.
func wakespaceDir(id int) string {
	return Config.WorkDir + "wakespace/" + strconv.Itoa(id) + "/"
}

// taskLogName is the file of a task's log in its build's wakespace
func taskLogName(taskID int) string {
	return fmt.Sprintf("task_%d.log", taskID)
}

// GetWakespaceDir returns path to the data dir - there all build+wake related data is
// stored
func (b *Build) GetWakespaceDir() string {
	return wakespaceDir(b.ID)
}

// taskLogPath returns the path to the output log of the task
func (b *Build) taskLogPath(taskID int) string {
	return wakespaceDir(b.ID) + taskLogName(taskID)
}

// taskANSILogName is the task's colour log, task_N.ansi.log: task_N.log line
// for line, with the colour codes and hyperlinks of the task's output. Only
// tasks that printed colours have one.
func taskANSILogName(taskID int) string {
	return fmt.Sprintf("task_%d.ansi.log", taskID)
}

func (b *Build) taskANSILogPath(taskID int) string {
	return wakespaceDir(b.ID) + taskANSILogName(taskID)
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

// setDuration records how long the build ran, and that it ended. A build
// aborted while queued never started, and time.Since of a zero StartedAt is
// the maximum duration. An interrupted build's is 0, like that of a build
// MarkInterruptedBuilds marks aborted
func (b *Build) setDuration() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.ended = true
	// An interrupted build's would include the time wakeci was stopped
	if b.StartedAt.IsZero() || b.interrupted {
		b.Duration = 0
		return
	}
	b.Duration = time.Since(b.StartedAt)
}

// SetBuildStatus sets the status of the builds
func (b *Build) SetBuildStatus(status ItemStatus) {
	// Pending status tasks must finish before any fields for the next status
	// are changed or broadcast.
	b.pendingTasksWG.Wait()
	b.Logger.Info("build status", "status", status)
	// Under b.mutex, like every field records() reads: RunJob and
	// RestoreQueue send an update from their own goroutine
	b.mutex.Lock()
	b.Status = status
	if status == StatusRunning {
		b.StartedAt = time.Now()
	}
	b.mutex.Unlock()
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
		b.setDuration()
		b.setDisk()
		// Saved before the build leaves the queue: a stop waits only for
		// builds in the queue, then closes the database
		b.BroadcastUpdate()
		b.Cleanup()
	case StatusFailed:
		b.runOnStatusTasks(status)
		b.CollectArtifacts()
		// An abort during the on_failed tasks stops them, not finally
		b.nextAbortScope()
		b.runOnStatusTasks(FinalTask)
		b.setDuration()
		b.setDisk()
		b.BroadcastUpdate()
		b.Cleanup()
	case StatusFinished:
		b.runOnStatusTasks(status)
		b.CollectArtifacts()
		// An abort during the on_finished tasks stops them, not finally
		b.nextAbortScope()
		b.runOnStatusTasks(FinalTask)
		b.setDuration()
		b.setDisk()
		// An interrupted build's duration is unknown (setDuration)
		if !b.interrupted {
			if err := RecordBuildDuration(b.Job.Name, int(b.Duration)); err != nil {
				b.Logger.Error("record build duration", "err", err)
			}
		}
		b.BroadcastUpdate()
		b.Cleanup()
	}

}

// CreateBuild creates Build instance and all necessary files and folders in
// wakespace, with the job's default params
func CreateBuild(job *Job, jobPath string, untrustedParams bool) (*Build, error) {
	return createBuild(job, jobPath, untrustedParams, nil, buildGroup{})
}

// runParams returns a copy of the job's default params with the run's
// non-empty values in place of the defaults; an empty value keeps the
// default. A copy, so the job's defaults stay the defaults.
func runParams(defaults []map[string]string, values url.Values) []map[string]string {
	if defaults == nil {
		return nil
	}
	params := make([]map[string]string, len(defaults))
	for idx, param := range defaults {
		params[idx] = make(map[string]string, len(param))
		for pkey, pval := range param {
			if value := values.Get(pkey); value != "" {
				pval = value
			}
			params[idx][pkey] = pval
		}
	}
	return params
}

// createBuild creates a build whose params are the run's values (runParams).
// They are set before the build is marked pending, because that sends the
// build's first update and starts its on_pending tasks: both must see the
// values the build runs with. RunJob used to set them afterwards, so the
// feed got the defaults first and an on_pending task could run with them.
func createBuild(job *Job, jobPath string, untrustedParams bool, values url.Values, group buildGroup) (*Build, error) {
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
		Job:      job,
		ID:       counti,
		abort:    newAbortScope(),
		Params:   runParams(job.DefaultParams, values),
		ETA:      GetJobETA(job.Name),
		queuedAt: time.Now(),
		// Set before SetBuildStatus below starts the pending tasks
		untrustedParams: untrustedParams,
		group:           group,
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
	// URL is where to download it, filled in by the API, not stored
	URL string `json:"url,omitempty"`
}

// Used to expand env variables in commands
// logEnvMapper expands the variables a task's environment has, for the
// "Expanded command" log line, and leaves the others as written: bash runs
// the command as written, where $1 in a shell function or a variable the
// command sets itself has a value that the log line can't know
func logEnvMapper(env []string) func(string) string {
	known := map[string]bool{}
	for _, pair := range env {
		name, _, _ := strings.Cut(pair, "=")
		known[name] = true
	}
	mapper := getEnvMapper(env)
	return func(name string) string {
		if !known[name] {
			return "$" + name
		}
		return mapper(name)
	}
}

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
