package main

import (
	"encoding/json"
	"strconv"

	bolt "github.com/etcd-io/bbolt"
)

// MsgType ...
type MsgType string

// MsgTypeBuildUpdate ...
const MsgTypeBuildUpdate = "build:update"

// MsgTypeJobUpdate ...
const MsgTypeJobUpdate = "job:update"

// MsgTypeInSubscribe ...
const MsgTypeInSubscribe = "in:subscribe"

// MsgTypeInUnsubscribe ...
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
}

// JobsListData ...
type JobsListData struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BuildUpdateData ...
type BuildUpdateData struct {
	ID         string      `json:"id"`
	Count      int         `json:"count"`
	Name       string      `json:"name"`
	Status     BuildStatus `json:"status"`
	TotalTasks int         `json:"total_tasks"`
	DoneTasks  int         `json:"done_tasks"`
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
