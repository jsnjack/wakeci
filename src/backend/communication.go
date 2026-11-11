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
	ID        int           `json:"id"`
	Status    ItemStatus    `json:"status"`
	StartedAt time.Time     `json:"startedAt"`
	Duration  time.Duration `json:"duration"`
	Kind      string        `json:"kind"`
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
	ID             int                 `json:"id"`
	Name           string              `json:"name"`
	Status         ItemStatus          `json:"status"`
	Tasks          []*TaskStatus       `json:"tasks"`
	Params         []map[string]string `json:"params"`
	Artifacts      []string            `json:"artifacts"` // Deprecate in favor of BuildArtifacts
	BuildArtifacts []*ArtifactInfo     `json:"build_artifacts"`
	StartedAt      time.Time           `json:"startedAt"`
	Duration       time.Duration       `json:"duration"`
	ETA            int                 `json:"eta"`
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
