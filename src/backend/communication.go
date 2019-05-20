package main

import (
	"encoding/json"

	bolt "github.com/etcd-io/bbolt"
)

// MsgJobsList ...
type MsgJobsList struct {
	Type string          `json:"type"`
	Data []*JobsListData `json:"data"`
}

// JobsListData ...
type JobsListData struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
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

// GetAllJobsMessage returns the message with the list of available jobs
func GetAllJobsMessage() *[]byte {
	msg := MsgJobsList{Type: "jobs:list"}
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			bucket := b.Bucket(key)
			job := JobsListData{
				Name:  string(key),
				Count: btoi(bucket.Get([]byte("count"))),
			}

			msg.Data = append(msg.Data, &job)
		}
		return nil
	})
	if err != nil {
		Logger.Println(err)
	}
	msgB, _ := json.Marshal(msg)
	return &msgB
}
