package main

import (
	"encoding/json"
)

// MsgType ...
type MsgType string

// MsgTypeBuildUpdate is sent where there is an update in status of executing build
// Used on the feed page
const MsgTypeBuildUpdate = "build:update"

// MsgTypeJobUpdate is sent when new job was triggered to update job count on the job page
const MsgTypeJobUpdate = "job:update"

// MsgTypeInSubscribe is incoming message. Means a user has opened build page
const MsgTypeInSubscribe = "in:subscribe"

// MsgTypeInUnsubscribe is incoming message. Means a user has closed build page
const MsgTypeInUnsubscribe = "in:unsubscribe"

// MsgBroadcast ...
type MsgBroadcast struct {
	Type MsgType     `json:"type"`
	Data interface{} `json:"data"`
}

// MsgIncoming ...
type MsgIncoming struct {
	Type MsgType         `json:"type"`
	Data json.RawMessage `json:"data"`
}

// InSubscribeData ...
type InSubscribeData struct {
	To string `json:"to"`
	ID string `json:"id"`
}

// JobsListData ...
type JobsListData struct {
	Name string `json:"name"`
}

// BuildUpdateData is viewable on the feed page
type BuildUpdateData struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Status     ItemStatus `json:"status"`
	TotalTasks int        `json:"total_tasks"`
	DoneTasks  int        `json:"done_tasks"`
}

// CommandLogData ...
type CommandLogData struct {
	TaskID int    `json:"task_id"`
	ID     int    `json:"id"`
	Data   string `json:"data"`
}
