package main

import (
	"io/ioutil"
	"path/filepath"

	bolt "github.com/etcd-io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

// ConfigExt ...
const ConfigExt = ".yaml"

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

	_, nameExt := filepath.Split(path)
	job.Name = nameExt[0 : len(nameExt)-len(ConfigExt)]

	Logger.Printf("Read job from file %s: %v\n", path, job)
	return &job, nil
}

// ScanAllJobs scans for all available jobs and saves them in database
func ScanAllJobs() error {
	files, _ := filepath.Glob(WorkingDir + "*" + ConfigExt)
	for _, f := range files {
		job, err := ReadJob(f)
		if err != nil {
			Logger.Println(err)
			continue
		}
		err = DB.Update(func(tx *bolt.Tx) error {
			jobsBucket := tx.Bucket(JobsBucket)

			itemBucket, err := jobsBucket.CreateBucketIfNotExists([]byte(job.Name))
			if err != nil {
				return err
			}

			count := itemBucket.Get([]byte("count"))
			if count == nil {
				err = itemBucket.Put([]byte("count"), itob(0))
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
	}
	return nil
}
