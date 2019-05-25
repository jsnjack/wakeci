package main

import (
	"encoding/json"
	"strconv"

	bolt "github.com/etcd-io/bbolt"
)

// MsgJobsList ...
type MsgJobsList struct {
	Type string          `json:"type"`
	Data []*JobsListData `json:"data"`
}

// MsgJobUpdate ...
type MsgJobUpdate struct {
	Type string        `json:"type"`
	Data *JobsListData `json:"data"`
}

// JobsListData ...
type JobsListData struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MsgBuildUpdate ...
type MsgBuildUpdate struct {
	Type string           `json:"type"`
	Data *BuildUpdateData `json:"data"`
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

// GetAllJobsMessage returns the message with the list of available jobs
func GetAllJobsMessage() *[]byte {
	msg := MsgJobsList{Type: "jobs:list"}
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
				msg.Data = append(msg.Data, &job)
			} else {
				Logger.Println(err)
			}
		}
		return nil
	})
	if err != nil {
		Logger.Println(err)
	}
	msgB, _ := json.Marshal(msg)
	return &msgB
}
