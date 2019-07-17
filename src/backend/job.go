package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	bolt "github.com/etcd-io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

// ConfigExt ...
const ConfigExt = ".yaml"

// Job represents Job
// Default params are stored as params in yaml files
type Job struct {
	Name          string              `yaml:"name" json:"name"`
	Tasks         []*Task             `yaml:"tasks" json:"tasks"`
	DefaultParams []map[string]string `yaml:"params" json:"defaultParams"`
}

// Task ...
type Task struct {
	ID      int         `json:"id"`
	Name    string      `yaml:"name" json:"name"`
	Command string      `yaml:"command" json:"command"`
	Status  ItemStatus  `json:"status"`
	Logs    interface{} `json:"logs"` // used as a container for frontend
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

	// Assign tasks ids and status
	for i, t := range job.Tasks {
		t.ID = i
		t.Status = StatusPending
	}

	_, nameExt := filepath.Split(path)
	job.Name = nameExt[0 : len(nameExt)-len(ConfigExt)]

	Logger.Printf("Read job from file %s: %s, tasks %d\n", path, job.Name, len(job.Tasks))
	return &job, nil
}

// ScanAllJobs scans for all available jobs and saves them in database
func ScanAllJobs() error {
	files, _ := filepath.Glob(*WorkingDirFlag + "*" + ConfigExt)
	for _, f := range files {
		job, err := ReadJob(f)
		if err != nil {
			Logger.Println(err)
			continue
		}
		err = DB.Update(func(tx *bolt.Tx) error {
			jobsBucket := tx.Bucket(JobsBucket)

			jb, err := jobsBucket.CreateBucketIfNotExists([]byte(job.Name))
			if err != nil {
				return err
			}
			paramsB, err := json.Marshal(job.DefaultParams)
			if err != nil {
				return err
			}
			return jb.Put([]byte("defaultParams"), paramsB)
		})
		if err != nil {
			Logger.Println(err)
			continue
		}
	}
	return nil
}
