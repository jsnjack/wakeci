package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

func jobNameParam(r *http.Request) (string, error) {
	name := chi.URLParam(r, "name")
	if r.URL.RawPath != "" {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			return "", fmt.Errorf("decode job name: %w", err)
		}
		name = decoded
	}
	if !isValidJobName(name) {
		return "", fmt.Errorf("invalid job name: %s", name)
	}
	return name, nil
}

func requireJobName(w http.ResponseWriter, r *http.Request, logger *slog.Logger) (string, bool) {
	name, err := jobNameParam(r)
	if err == nil {
		return name, true
	}
	logger.Warn("invalid job name", "err", err)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusBadRequest)
	writeBody(logger, w, []byte(err.Error()))
	return "", false
}

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
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	name, ok := requireJobName(w, r, logger)
	if !ok {
		return
	}

	err := r.ParseForm()
	if err != nil {
		logger.Error("parse form", "err", err)
	}

	build, err := RunJob(name, r.Form, apiKeyFromContext(r.Context()) != nil)
	if err != nil {
		logger.Error("run job", "job", name, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	writeBody(logger, w, []byte(strconv.Itoa(build.ID)))
}

// HandleJobGet returns content of a specific job file
// @Summary      Return the content of the job
// @Tags         job
// @Produce      json
// @Success      200      {object}   JobData
// @Failure      500      {string}   string
// @Router       /job/{name}/ [get]
func HandleJobGet(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	name, ok := requireJobName(w, r, logger)
	if !ok {
		return
	}

	path := Config.JobDir + name + Config.jobsExt
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("read job file", "path", path, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	jd := JobData{
		Content: string(data),
	}
	payloadB, err := json.Marshal(jd)
	if err != nil {
		logger.Error("marshal job data", "path", path, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
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
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	name, ok := requireJobName(w, r, logger)
	if !ok {
		return
	}

	content := r.FormValue("fileContent")
	contentB := []byte(content)

	// Verify that it is still a valid yaml file and it is possible to create
	// a job out of it
	job := Job{}
	err := yaml.Unmarshal(contentB, &job)
	if err != nil {
		logger.Warn("invalid job yaml", "job", name, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	// Verify provided interval
	err = job.verifyInterval()
	if err != nil {
		logger.Warn("invalid job interval", "job", name, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	contentB = NormalizeNewlines(contentB)

	path := Config.JobDir + name + Config.jobsExt

	err = os.WriteFile(path, contentB, 0644)
	if err != nil {
		logger.Error("write job file", "path", path, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	logger.Info("job updated", "job", name)
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
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	name, ok := requireJobName(w, r, logger)
	if !ok {
		return
	}
	path := Config.JobDir + name + Config.jobsExt

	if _, err := os.Stat(path); err == nil {
		err = os.Remove(path)
		logger.Info("job deleted", "job", name)
		CleanupJobsBucket()
		if err != nil {
			logger.Error("remove job file", "path", path, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte(err.Error()))
			return
		}
		return
	} else if os.IsNotExist(err) {
		logger.Warn("delete missing job", "job", name, "err", err)
		w.WriteHeader(http.StatusNotFound)
		return
	} else {
		logger.Error("stat job file", "path", path, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
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
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	name, ok := requireJobName(w, r, logger)
	if !ok {
		return
	}

	activeStatus := r.FormValue("active")

	switch activeStatus {
	case "false", "true":
		break
	default:
		m := fmt.Sprintf("Invalid active flag for a job: %s\n", activeStatus)
		logger.Warn("invalid active flag for a job", "job", name, "active", activeStatus)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(m))
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
			logger.Info("job active state changed", "job", name, "active", activeStatus)
		}
		return err
	})
	if err != nil {
		logger.Error("set job active state", "job", name, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	writeBody(logger, w, []byte(activeStatus))
}
