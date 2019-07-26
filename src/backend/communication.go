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
}

// TaskStatus contains basic info about a task, used for status updates
type TaskStatus struct {
	ID     int        `json:"id"`
	Status ItemStatus `json:"status"`
}

// BuildUpdateData is viewable on the feed page
type BuildUpdateData struct {
	ID        int                 `json:"id"`
	Name      string              `json:"name"`
	Status    ItemStatus          `json:"status"`
	Tasks     []*TaskStatus       `json:"tasks"`
	Params    []map[string]string `json:"params"`
	Artifacts []string            `json:"artifacts"`
	StartedAt time.Time           `json:"startedAt"`
	Duration  time.Duration       `json:"duration"`
}

// CommandLogData ...
type CommandLogData struct {
	TaskID int    `json:"task_id"`
	ID     int    `json:"id"`
	Data   string `json:"data"`
}

// SettingsData used for Settings view to allow user to modify settings
type SettingsData struct {
	ConcurrentBuilds int
}

// JobData used for editing a job
type JobData struct {
	Content string `json:"fileContent"`
}
