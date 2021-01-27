package main

import (
	"encoding/json"
	"time"
)

// MsgTypeInSubscribe is incoming message. Means a user has opened build page
const MsgTypeInSubscribe = "in:subscribe"

// MsgTypeInUnsubscribe is incoming message. Means a user has closed build page
const MsgTypeInUnsubscribe = "in:unsubscribe"

// MsgBroadcast ...
type MsgBroadcast struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
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
}

// JobData used for editing a job
type JobData struct {
	Content string `json:"fileContent"`
}
