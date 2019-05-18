package main

import (
	"encoding/json"
	"path/filepath"
)

// MsgJobsList ...
type MsgJobsList struct {
	Type string `json:"type"`
	Data []*Job `json:"data"`
}

// MsgFeedUpdate ...
type MsgFeedUpdate struct {
	Type string         `json:"type"`
	Data FeedUpdateData `json:"data"`
}

// FeedUpdateData ...
type FeedUpdateData struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     ExecutorStatus `json:"status"`
	TotalTasks int            `json:"total_tasks"`
	DoneTasks  int            `json:"done_tasks"`
}

// GenerateWelcomeMessage returns the message with the list of available jobs
func GenerateWelcomeMessage() *[]byte {
	msg := MsgJobsList{Type: "jobs:list"}
	files, _ := filepath.Glob(WorkingDir + "*.yaml")
	for _, f := range files {
		job, err := ReadJob(f)
		if err != nil {
			Logger.Println(err)
		} else {
			msg.Data = append(msg.Data, job)
		}
	}
	msgB, _ := json.Marshal(msg)
	return &msgB
}
