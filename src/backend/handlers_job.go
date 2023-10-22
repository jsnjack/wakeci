package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

// HandleRunJob adds job to queue
// @Summary      Start a job
// @Description  A new build for the job `name` is created and added to the queue. Returns build id
// @Tags         job
// @Produce      plain
// @Param        name     path       string   true   "Name of the job"
// @Param        param1   query      string   false  "Override default `params` of the job"
// @Param        param2   formData   string   false  "Override default `params` of the job"
// @Success      200      {integer}  integer
// @Failure      400      {string}   string
// @Router       /job/{name}/run [post]
func HandleRunJob(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	err := r.ParseForm()
	if err != nil {
		logger.Println(err)
	}

	build, err := RunJob(chi.URLParam(r, "name"), r.Form)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(strconv.Itoa(build.ID)))
}

// HandleJobGet returns content of a specific job file
// @Summary      Return the content of the job
// @Tags         job
// @Produce      json
// @Success      200      {object}   JobData
// @Failure      500      {string}   string
// @Router       /job/{name}/ [get]
func HandleJobGet(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	path := Config.JobDir + chi.URLParam(r, "name") + Config.jobsExt
	data, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	jd := JobData{
		Content: string(data),
	}
	payloadB, err := json.Marshal(jd)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleJobPost updates content of a specific job
// @Summary      Update the content of the job
// @Description  All parameters are available as query parameters and as formData
// @Tags         job
// @Produce      plain
// @Param        fileContent     formData    string   true   "New content of the job"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Router       /job/{name}/ [post]
func HandleJobPost(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	content := r.FormValue("fileContent")
	contentB := []byte(content)

	// Verify that it is still a valid yaml file and it is possible to create
	// a job out of it
	job := Job{}
	err := yaml.Unmarshal(contentB, &job)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	// Verify provided interval
	err = job.verifyInterval()
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	contentB = NormalizeNewlines(contentB)

	path := Config.JobDir + chi.URLParam(r, "name") + Config.jobsExt

	err = os.WriteFile(path, contentB, 0644)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	logger.Printf("Job %s was updated\n", chi.URLParam(r, "name"))
}

// HandleDeleteJob deletes the job
// @Summary      Delete the job
// @Tags         job
// @Produce      plain
// @Param        name     path    string   true   "Name of the job to delete"
// @Success      200      {string}    string
// @Failure      400      {string}    string
// @Failure      500      {string}    string
// @Router       /job/{name} [delete]
func HandleDeleteJob(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	name := chi.URLParam(r, "name")
	path := Config.JobDir + name + Config.jobsExt

	if _, err := os.Stat(path); err == nil {
		err = os.Remove(path)
		logger.Printf("Job %s was deleted\n", name)
		CleanupJobsBucket()
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		return
	} else if os.IsNotExist(err) {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	} else {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

// HandleJobSetActive sets if job is active (enabled/disabled)
// @Summary      Enable/disable the job. Returns if the job is active
// @Tags         job
// @Produce      plain
// @Param        name     path    string   true   "Name of the job"
// @Success      200      {string}    string
// @Failure      500      {string}    string
// @Router       /job/{name}/set_active [post]
func HandleJobSetActive(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	name := chi.URLParam(r, "name")

	activeStatus := r.FormValue("active")

	switch activeStatus {
	case "false", "true":
		break
	default:
		m := fmt.Sprintf("Invalid active flag for a job: %s\n", activeStatus)
		logger.Printf("Invalid active flag for a job: %s\n", activeStatus)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(m))
		return
	}

	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		jb := b.Bucket([]byte(name))
		if jb == nil {
			return fmt.Errorf("invalid job name: %s", name)
		}
		err := jb.Put([]byte("active"), []byte(activeStatus))
		if err == nil {
			logger.Printf("Change active state of job %s to %s\n", name, activeStatus)
		}
		return err
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(activeStatus))
}
