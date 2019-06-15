package main

import (
	"encoding/json"
	"strconv"

	bolt "github.com/etcd-io/bbolt"
)

// MsgType ...
type MsgType string

// MsgTypeBuildUpdate is sent where there is an update in status of executing build
// Used on the feed page
const MsgTypeBuildUpdate = "build:update"

// MsgTypeBuildInfo contains information to bootstrap initial build page
// Number and name of tasks to create a placeholder for logs
const MsgTypeBuildInfo = "build:info"

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

// BuildInfoData ...
type BuildInfoData struct {
	ID string `json:"id"`
}

// JobsListData ...
type JobsListData struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
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

// GetAllJobsMessage returns the message with the list of available jobs
func GetAllJobsMessage() *[]byte {
	msg := MsgBroadcast{Type: "jobs:list"}
	var data []*JobsListData
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			bucket := b.Bucket(key)
			countS := string(bucket.Get([]byte("count")))
			count, err := strconv.Atoi(countS)
			if err == nil {
				job := JobsListData{
					Name:  string(key),
					Count: count,
				}
				data = append(data, &job)
			} else {
				Logger.Println(err)
			}
		}
		return nil
	})
	if err != nil {
		Logger.Println(err)
	}
	msg.Data = data
	msgB, _ := json.Marshal(msg)
	return &msgB
}
