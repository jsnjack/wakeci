package main

import (
	"encoding/json"
	"path/filepath"
)

// WelcomeMessage ...
type WelcomeMessage struct {
	Type string `json:"type"`
	Data []*Job `json:"data"`
}

// GenerateWelcomeMessage returns the message with the list of available jobs
func GenerateWelcomeMessage() *[]byte {
	msg := WelcomeMessage{Type: "jobs:list"}
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
