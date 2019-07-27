package main

import (
	"encoding/json"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron"

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
# related to the workspce directory
artifacts:
  - "*.tar.gz"

# Automatically run the job every configured interval (cron expression)
# More info https://godoc.org/github.com/robfig/cron
interval: "@daily"

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
	Interval      string              `yaml:"interval" json:"interval"`
}

// AddToCron adds a job to cron
func (j *Job) AddToCron() error {
	if j.Interval == "" {
		return nil
	}
	_, err := C.AddJob(j.Interval, j)
	Logger.Printf("Add job %s to cron with interval %s\n", j.Name, j.Interval)
	return err
}

// Run is used to add job to cron
func (j *Job) Run() {
	var params url.Values
	build, err := RunJob(j.Name, params)
	if err != nil {
		build.Logger.Printf("Unable to schedule the build via cron: %s\n", err.Error())
	}
}

// Used to verify interval before saving after editing
func (j *Job) verifyInterval() error {
	if j.Interval == "" {
		return nil
	}
	_, err := cron.ParseStandard(j.Interval)
	return err
}

// Task ...
type Task struct {
	ID        int         `json:"id"`
	Name      string      `yaml:"name" json:"name"`
	Command   string      `yaml:"command" json:"command"`
	Status    ItemStatus  `json:"status"`
	Logs      interface{} `json:"logs"` // used as a container for frontend
	startedAt time.Time
	duration  time.Duration
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
	// Clean Cron entries
	Logger.Println("Cleaning all cron entries...")
	for _, entry := range C.Entries() {
		C.Remove(entry.ID)
	}
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
			err = jb.Put([]byte("interval"), []byte(job.Interval))
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			Logger.Println(err)
			continue
		}
		err = job.AddToCron()
		if err != nil {
			Logger.Println(err)
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

// RunJob creates a new build and schedules it for execution
func RunJob(name string, params url.Values) (*Build, error) {
	jobFile := *ConfigDirFlag + name + ".yaml"
	job, err := CreateJobFromFile(jobFile)
	if err != nil {
		return nil, err
	}
	build, err := CreateBuild(job, jobFile)
	if err != nil {
		return nil, err
	}

	// Update params from URL
	for idx := range build.Params {
		for pkey := range build.Params[idx] {
			value := params.Get(pkey)
			if value != "" {
				build.Params[idx][pkey] = value
				build.Logger.Printf("Updating key %s to %s", pkey, value)
			}
		}
	}

	Q.Add(build)
	Q.Take()
	build.BroadcastUpdate()
	return build, nil
}
