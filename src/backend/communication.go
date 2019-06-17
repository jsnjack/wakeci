package main

import (
	"encoding/json"
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
	To string `json:"to"`
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
