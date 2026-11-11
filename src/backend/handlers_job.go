package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
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
// @Description  Creates a build of the job and adds it to the queue; returns the build ID. Send the job's parameters as form fields or query parameters with their names; others are ignored. API key scope: run. With a key, parameters can't reference a secret or contain quotes, line breaks or NUL.
// @Tags         job
// @Produce      plain
// @Param        name     path       string   true   "Name of the job"
// @Success      200      {integer}  integer
// @Failure      400      {string}   string
// @Router       /api/job/{name}/run [post]
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
	addAuditAttrs(r, "build", build.ID)
	used, ignored := usedParams(build, r.Form)
	if len(used) > 0 {
		addAuditAttrs(r, auditListAttrs("params", used)...)
	}
	if ignored > 0 {
		addAuditAttrs(r, "ignoredParams", ignored)
	}
	w.Header().Set("Content-Type", "text/plain")
	writeBody(logger, w, []byte(strconv.Itoa(build.ID)))
}

// usedParams returns the names of the job's parameters that params set, and
// how many other names params has. Only names go in the audit log, since
// values can hold anything, e.g. passwords; and only the job's own names,
// since a name the job doesn't have can hold anything too: curl -d with a
// JSON body sends the whole body as one name.
func usedParams(build *Build, params url.Values) ([]string, int) {
	build.mutex.Lock()
	defer build.mutex.Unlock()
	var used []string
	for _, group := range build.Params {
		for name := range group {
			if params.Get(name) != "" {
				used = append(used, name)
			}
		}
	}
	slices.Sort(used)
	used = slices.Compact(used)
	return used, len(params) - len(used)
}

// HandleJobGet returns content of a specific job file
// @Summary      Return the content of the job
// @Description  API key scope: read
// @Tags         job
// @Produce      json
// @Param        name     path       string   true   "Name of the job"
// @Success      200      {object}   JobData
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/job/{name} [get]
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
// @Description  API key scope: write
// @Tags         job
// @Produce      plain
// @Param        name            path        string   true   "Name of the job"
// @Param        fileContent     formData    string   true   "New content of the job"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Router       /api/job/{name} [post]
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
// @Description  API key scope: write
// @Tags         job
// @Produce      plain
// @Param        name     path    string   true   "Name of the job to delete"
// @Success      200      {string}    string
// @Failure      400      {string}    string
// @Failure      500      {string}    string
// @Router       /api/job/{name} [delete]
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
// @Description  API key scope: write
// @Tags         job
// @Produce      plain
// @Param        name     path    string   true   "Name of the job"
// @Success      200      {string}    string
// @Failure      500      {string}    string
// @Router       /api/job/{name}/set_active [post]
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
		addAuditAttrs(r, "active", activeStatus)
	default:
		// Not the value in the logs: it could be a secret typed into the
		// wrong field
		addAuditAttrs(r, "active", "invalid")
		m := fmt.Sprintf("Invalid active flag for a job: %s\n", activeStatus)
		logger.Warn("invalid active flag for a job", "job", name)
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
