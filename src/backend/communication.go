package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// Set precision of startedAt field
const TimeFormat = "2006-01-02T15:04:05.000000Z07:00"

// MsgTypeInSubscribe is incoming message. Means a user has opened build page
const MsgTypeInSubscribe = "in:subscribe"

// MsgTypeInUnsubscribe is incoming message. Means a user has closed build page
const MsgTypeInUnsubscribe = "in:unsubscribe"

// MsgBroadcast ...
type MsgBroadcast struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
	// Job is the job the message is about, so the hub only sends it to
	// clients that may read the job; empty for messages about the server
	Job string `json:"-"`
}

// MsgIncoming ...
type MsgIncoming struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// InSubscribeData ...
type InSubscribeData struct {
	To []string `json:"to"`
}

// JobsListData is a format of data that JobsView receives and JobsBucket stores
type JobsListData struct {
	Name          string              `json:"name"`
	Desc          string              `json:"desc"`
	DefaultParams []map[string]string `json:"defaultParams"`
	Interval      string              `json:"interval"`
	Active        string              `json:"active"`
}

// TaskStatus contains basic info about a task, used for status updates
type TaskStatus struct {
	// ID is the task's position in the build's tasks, which start with the
	// on_pending and on_running tasks, so the first main task's ID may not
	// be 0
	ID int `json:"id"`
	// Status is pending until the task runs, then running, and at the end
	// finished, failed, skipped (its when or if condition was false),
	// aborted or timed out. Tasks that never run, such as the on_failed
	// tasks of a build that finished, stay pending.
	Status ItemStatus `json:"status"`
	// StartedAt is 0001-01-01T00:00:00.000000Z until the task starts
	StartedAt time.Time `json:"startedAt"`
	// Duration is in nanoseconds, 0 until the task ends
	Duration time.Duration `json:"duration" swaggertype:"integer"`
	// Kind is main for the job's tasks, or pending, running, failed,
	// aborted, finished or finally for those of on_pending, on_running,
	// on_failed, on_aborted, on_finished and finally. A build that timed out
	// runs its on_aborted tasks.
	Kind string `json:"kind"`
	// Name, Log and LogSize are only filled in by GET /api/build/{id}. Log
	// is the URL of the task's log, empty if the task wrote none.
	Name    string `json:"name,omitempty"`
	Log     string `json:"log,omitempty"`
	LogSize int64  `json:"logSize,omitempty"`
}

// When StartedAt field is serialized to JSON, it has fixed second's precision
// to simplify using of Python's datetime.fromisoformat
func (x TaskStatus) MarshalJSON() ([]byte, error) {
	// re-type it to strip this method, else infinite regression
	type Alias TaskStatus

	// Embed, then mask the underlying time.Time field, allowing us to overwrite how it is marshalled
	type StartedAtFieldMarshal struct {
		Alias
		StartedAt string `json:"startedAt"`
	}

	aliased := StartedAtFieldMarshal{
		Alias:     Alias(x),
		StartedAt: x.StartedAt.Format(TimeFormat),
	}

	// Marshal the wrapped struct, letting the JSON package handle the marshaling of any fields
	// we're not specifically overwriting here
	return json.Marshal(aliased)
}

// BuildUpdateData is viewable on the feed page
type BuildUpdateData struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Status is pending while the build is queued, then running, and at
	// the end finished, failed, aborted or timed out. It turns final before
	// the build's on_* and finally tasks run and its artifacts are
	// collected; the build has ended once they are done, when duration is
	// set and a wait answers.
	Status ItemStatus `json:"status"`
	// Tasks are all the build's tasks in the order they would run:
	// on_pending, on_running, the main tasks, on_failed, on_aborted,
	// on_finished and finally. Only those the build's status calls for run.
	Tasks          []*TaskStatus       `json:"tasks"`
	Params         []map[string]string `json:"params"`
	Artifacts      []string            `json:"artifacts"` // Deprecated: file names only; use build_artifacts
	BuildArtifacts []*ArtifactInfo     `json:"build_artifacts"`
	// StartedAt is when the build started running, after its on_pending
	// tasks; 0001-01-01T00:00:00.000000Z if it never did
	StartedAt time.Time `json:"startedAt"`
	// Duration is in nanoseconds, from startedAt until the build's last
	// task ended. It is 0 until then, for a build that never started, and
	// for one that was running when wakeci was killed.
	Duration time.Duration `json:"duration" swaggertype:"integer"`
	// ETA is how long the build is expected to take, in nanoseconds: the
	// average duration of the job's last 5 finished builds when this build
	// was created; 0 until the job has 5
	ETA int `json:"eta"`
}

// When StartedAt field is serialized to JSON, it has fixed second's precision
// to simplify using of Python's datetime.fromisoformat
func (x BuildUpdateData) MarshalJSON() ([]byte, error) {
	// re-type it to strip this method, else infinite regression
	type Alias BuildUpdateData

	// Embed, then mask the underlying time.Time field, allowing us to overwrite how it is marshalled
	type StartedAtFieldMarshal struct {
		Alias
		StartedAt string `json:"startedAt"`
	}

	aliased := StartedAtFieldMarshal{
		Alias:     Alias(x),
		StartedAt: x.StartedAt.Format(TimeFormat),
	}
	// Older history entries can contain null lists; the API promises arrays.
	if aliased.Tasks == nil {
		aliased.Tasks = []*TaskStatus{}
	}
	if aliased.Params == nil {
		aliased.Params = []map[string]string{}
	}
	if aliased.Artifacts == nil {
		aliased.Artifacts = []string{}
	}
	if aliased.BuildArtifacts == nil {
		aliased.BuildArtifacts = []*ArtifactInfo{}
	}

	// Marshal the wrapped struct, letting the JSON package handle the marshaling of any fields
	// we're not specifically overwriting here
	return json.Marshal(aliased)
}

// ToFilterMatchString converts BuildUpdateData to a string suitable for filter matching
func (x BuildUpdateData) ToFilterMatchString() string {
	return fmt.Sprintf("id:%v name:%s status:%s %s", x.ID, x.Name, x.Status, x.Params)
}

// CommandLogData ...
type CommandLogData struct {
	TaskID int    `json:"taskID"`
	ID     int    `json:"id"` // ID of a log message
	Data   string `json:"data"`
}

// SettingsData used for Settings view to allow user to modify settings
type SettingsData struct {
	ConcurrentBuilds int `json:"concurrentBuilds"`
	BuildHistorySize int `json:"buildHistorySize"`
	// APIKeyRate is how many requests a second each API key may keep
	// sending, and APIKeyBurst how many at once
	APIKeyRate  int `json:"apiKeyRate"`
	APIKeyBurst int `json:"apiKeyBurst"`
	// Server is what Wakefile.yaml set, which the page shows read-only
	Server ServerConfigData `json:"server"`
	// Admin is true for the admin password, which alone may change it
	Admin bool `json:"admin"`
}

// QueueStatusData tells whether queued builds start
type QueueStatusData struct {
	// Stopping is true once wakeci stops after the running builds end.
	// Queued builds wait for the restart
	Stopping bool `json:"stopping"`
	// AbortAt is when builds still running are aborted, while stopping
	AbortAt *time.Time `json:"abortAt,omitempty"`
	// RestartVersion is the version an update restarts into, while stopping
	RestartVersion string `json:"restartVersion,omitempty"`
	// BuildsAborted is true once the stop aborted the running builds, at
	// AbortAt or earlier, and waits for their on_aborted and finally tasks
	BuildsAborted bool `json:"buildsAborted,omitempty"`
	// Running counts the builds that run tasks, also the on_aborted tasks of
	// builds aborted while queued
	Running int `json:"running"`
	Queued  int `json:"queued"`
	// Version is the running wakeci version, so pages opened before an
	// update can reload
	Version string `json:"version"`
}

// JobData used for editing a job
type JobData struct {
	Content string `json:"fileContent"`
}
