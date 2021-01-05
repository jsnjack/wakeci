package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	yaml "gopkg.in/yaml.v3"

	bolt "go.etcd.io/bbolt"
)

// NewJobTemplate is a template for newly created jobs
// See configDescription.yaml for functionality demonstration
var NewJobTemplate = strings.Trim(`

desc: New job
tasks:
  - name: Print kernel information
    command: uname -a
`, "\n ")

// KindMain is a kind for main tasks - the ones which actually do the job
const KindMain = "main"

// Job represents Job
// Default params are stored as params in yaml files
type Job struct {
	Name          string              `yaml:"name" json:"name"`
	Desc          string              `yaml:"desc" json:"desc"`
	Tasks         []*Task             `yaml:"tasks" json:"tasks"`
	DefaultParams []map[string]string `yaml:"params" json:"defaultParams"`
	Artifacts     []string            `yaml:"artifacts" json:"artifacts"`
	Interval      string              `yaml:"interval" json:"interval"`
	Timeout       string              `yaml:"timeout" json:"timeout"`
	AllowParallel bool                `yaml:"allow_parallel"`
	Priority      int                 `yaml:"priority"`
}

// AddToCron adds a job to cron
func (j *Job) AddToCron() error {
	if j.Interval == "" {
		return nil
	}
	_, err := GlobalCron.AddJob(j.Interval, j)
	Logger.Printf("Add job %s to cron with interval %s\n", j.Name, j.Interval)
	return err
}

// Run is used to run a job via cron
func (j *Job) Run() {
	var params url.Values
	build, err := RunJob(j.Name, params)
	if err != nil {
		Logger.Printf("Unable to schedule a build via cron for job %s: %s\n", j.Name, err.Error())
		return
	}
	build.Logger.Printf("The build for job %s is scheduled via cron\n", j.Name)
}

// Used to verify interval before saving after editing
func (j *Job) verifyInterval() error {
	if j.Interval == "" {
		return nil
	}
	_, err := cron.ParseStandard(j.Interval)
	return err
}

// Task is a command to execute
// .Kind - Possible values: `KindMain` for main tasks; one of `StatusRunning` (and etc) for tasks that are executed when
//  job status has changed
type Task struct {
	ID        int         `json:"id"`
	Name      string      `yaml:"name" json:"name"`
	Command   string      `yaml:"command" json:"command"`
	Status    ItemStatus  `json:"status"`
	Kind      string      `json:"kind"`
	Logs      interface{} `json:"logs"` // used as a container for frontend
	startedAt time.Time
	duration  time.Duration
}

// OnTasks is a list of tasks that should be ran on status change
type OnTasks struct {
	OnPending  []*Task `yaml:"on_pending"`
	OnRunning  []*Task `yaml:"on_running"`
	OnFailed   []*Task `yaml:"on_failed"`
	OnAborted  []*Task `yaml:"on_aborted"`
	OnFinished []*Task `yaml:"on_finished"`
	Finally    []*Task `yaml:"finally"`
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

	// Assign main kind to all tasks
	for _, t := range job.Tasks {
		t.Kind = KindMain
	}

	ot := OnTasks{}
	err = yaml.Unmarshal(data, &ot)
	if err != nil {
		return nil, err
	}

	if ot.OnRunning != nil {
		for _, t := range ot.OnRunning {
			t.Kind = StatusRunning
		}
		job.Tasks = append(ot.OnRunning, job.Tasks...)
	}

	if ot.OnPending != nil {
		for _, t := range ot.OnPending {
			t.Kind = StatusPending
		}
		job.Tasks = append(ot.OnPending, job.Tasks...)
	}

	if ot.OnFailed != nil {
		for _, t := range ot.OnFailed {
			t.Kind = StatusFailed
		}
		job.Tasks = append(job.Tasks, ot.OnFailed...)
	}

	if ot.OnAborted != nil {
		for _, t := range ot.OnAborted {
			t.Kind = StatusAborted
		}
		job.Tasks = append(job.Tasks, ot.OnAborted...)
	}

	if ot.OnFinished != nil {
		for _, t := range ot.OnFinished {
			t.Kind = StatusFinished
		}
		job.Tasks = append(job.Tasks, ot.OnFinished...)
	}

	if ot.Finally != nil {
		for _, t := range ot.Finally {
			t.Kind = "finally"
		}
		job.Tasks = append(job.Tasks, ot.Finally...)
	}

	// Assign tasks ids and status
	for i, t := range job.Tasks {
		t.ID = i
		t.Status = StatusPending
	}

	_, nameExt := filepath.Split(path)
	job.Name = nameExt[0 : len(nameExt)-len(Config.jobsExt)]

	Logger.Printf("Read job from file %s: %s, tasks %d\n", path, job.Name, len(job.Tasks))
	return &job, nil
}

// ScanAllJobs scans for all available jobs and saves them in database
func ScanAllJobs() error {
	// Clean Cron entries
	Logger.Println("Cleaning all cron entries...")
	for _, entry := range GlobalCron.Entries() {
		GlobalCron.Remove(entry.ID)
	}
	files, _ := filepath.Glob(Config.JobDir + "*" + Config.jobsExt)
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
			isActive := jb.Get([]byte("active"))
			if isActive == nil {
				err = jb.Put([]byte("active"), []byte("true"))
				if err != nil {
					return err
				}
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

// RunJob creates a new build and schedules it for execution
func RunJob(name string, params url.Values) (*Build, error) {
	// Check if job is enabled
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		jb := b.Bucket([]byte(name))
		if jb == nil {
			return fmt.Errorf("Invalid job name: %s", name)
		}
		isActive := jb.Get([]byte("active"))
		if isActive == nil {
			return fmt.Errorf("Unknown if job %s is active", name)
		}
		if string(isActive) != "true" {
			return fmt.Errorf("Job %s is not enabled", name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	jobFile := Config.JobDir + name + Config.jobsExt
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

	GlobalQueue.Add(build)
	GlobalQueue.Take()
	build.BroadcastUpdate()
	return build, nil
}
