package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	bolt "github.com/etcd-io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

// NewJobTemplate is a template for newly created jobs. Suppose to also
// demonstrate all possible functionality
var NewJobTemplate = strings.Trim(`

desc: Ask a cow to say something smart
# 'params' are injected as environmetal variables
params:
  - SLEEP: 5

tasks:
  - name: Waking up a cow
    command: sleep ${SLEEP}

  - name: Cow says
	command: fortune | cowsay

# List of patterns according to https://golang.org/pkg/path/filepath/#Match
artifacts:
  - ./*.tar.gz

`, "\n ")

// ConfigExt ...
const ConfigExt = ".yaml"

// Job represents Job
// Default params are stored as params in yaml files
type Job struct {
	Name          string              `yaml:"name" json:"name"`
	Desc          string              `yaml:"desc" json:"desc"`
	Tasks         []*Task             `yaml:"tasks" json:"tasks"`
	DefaultParams []map[string]string `yaml:"params" json:"defaultParams"`
	Artifacts     []string            `yaml:"artifacts" json:"artifacts"`
}

// Task ...
type Task struct {
	ID      int         `json:"id"`
	Name    string      `yaml:"name" json:"name"`
	Command string      `yaml:"command" json:"command"`
	Status  ItemStatus  `json:"status"`
	Logs    interface{} `json:"logs"` // used as a container for frontend
}

// CreateJobFromFile reads job from a file
func CreateJobFromFile(path string) (*Job, error) {
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
	files, _ := filepath.Glob(*ConfigDirFlag + "*" + ConfigExt)
	for _, f := range files {
		job, err := CreateJobFromFile(f)
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
			err = jb.Put([]byte("defaultParams"), paramsB)
			if err != nil {
				return err
			}
			err = jb.Put([]byte("desc"), []byte(job.Desc))
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			Logger.Println(err)
			continue
		}
	}
	return nil
}

// CleanUpJobs verifies that jobs bucket is up to date
func CleanUpJobs() {
	DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(JobsBucket)
		c := b.Cursor()
		var toRemove [][]byte
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			name := string(key)
			path := *ConfigDirFlag + name + ".yaml"
			_, err := os.Stat(path)
			if err != nil {
				Logger.Printf("Removing %s: %s\n", name, err.Error())
				toRemove = append(toRemove, key)
			}
		}
		for _, rk := range toRemove {
			err := b.DeleteBucket(rk)
			if err != nil {
				Logger.Println(err)
			}
		}
		return nil
	})
}
