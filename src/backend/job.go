package main

import (
	"fmt"
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

// Job represents Job
type Job struct {
	Name  string  `yaml:"name" json:"name"`
	Tasks []*Task `yaml:"tasks"`
}

// Task ...
type Task struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// ReadJob reads job from a file
func ReadJob(path string) (*Job, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	job := Job{}
	err = yaml.Unmarshal(data, &job)
	if err != nil {
		return nil, err
	}
	if job.Name == "" {
		return nil, fmt.Errorf("Job name is empty: " + path)
	}
	Logger.Printf("Read job from file %s: %v\n", path, job)
	return &job, nil
}
