package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	bolt "go.etcd.io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

// HandleJobsView returns all available jobs
// @Summary      Returns list of available jobs
// @Tags         jobs
// @Produce      json
// @Success      200      {array}    JobsListData
// @Failure      500      {string}   string
// @Router       /jobs/ [get]
func HandleJobsView(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	var data []*JobsListData
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			job := JobsListData{
				Name: string(key),
			}
			jb := b.Bucket(key)
			if jb != nil {
				params := jb.Get([]byte("defaultParams"))
				err := json.Unmarshal(params, &job.DefaultParams)
				if err != nil {
					return err
				}
				desc := jb.Get([]byte("desc"))
				job.Desc = string(desc)
				interval := jb.Get([]byte("interval"))
				job.Interval = string(interval)
				active := jb.Get([]byte("active"))
				job.Active = string(active)
			}
			data = append(data, &job)
		}
		return nil
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	payloadB, err := json.Marshal(data)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleJobsCreate creates a new job file from default template
// @Summary      Create new empty job
// @Description  The job is created from the default template. All parameters are available as query parameters and as formData
// @Tags         jobs
// @Produce      plain
// @Param        name     formData    string   true   "Name of the job (also the name of the file in which the job is stored)"
// @Success      200      {string}    string
// @Failure      500      {string}    string
// @Router       /jobs/create [post]
func HandleJobsCreate(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	name := r.FormValue("name")
	path := Config.JobDir + name + Config.jobsExt

	if _, err := os.Stat(path); err == nil {
		logger.Printf("File %s already exists\n", path)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Job with this name already exists"))
		return
	} else if os.IsNotExist(err) {
		// Verify that it is still a valid yaml file and it is possible to create
		// a job out of it
		job := Job{}
		err := yaml.Unmarshal([]byte(NewJobTemplate), &job)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		err = os.WriteFile(path, []byte(NewJobTemplate), 0644)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		CleanupJobsBucket()
		err = ScanAllJobs()
		if err != nil {
			logger.Println(err)
		}
	} else {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

// HandleJobsRefresh deletes all jobs and reads them again from config directory
// @Summary      Refresh all jobs from the configuration folder; removes non-existing jobs
// @Tags         jobs
// @Produce      plain
// @Success      200      {string}    string
// @Failure      500      {string}    string
// @Router       /jobs/refresh [post]
func HandleJobsRefresh(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	CleanupJobsBucket()
	err := ScanAllJobs()
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}
