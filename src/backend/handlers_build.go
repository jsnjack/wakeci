package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v2"
)

var errBuildNotFound = errors.New("build not found")

// getBuildConfig returns job instance of already executed build
func getBuildConfig(buildID int) (*Job, error) {
	// Collect tasks info by reconstructing job object
	buildConfigDir := Config.WorkDir + "wakespace/" + strconv.Itoa(buildID)
	newConfigFilename := buildConfigDir + "/build_plan" + Config.jobsExt
	oldConfigFilename := buildConfigDir + "/build" + Config.jobsExt

	job := &Job{}

	// Check if a config file with new format exists
	_, err := os.Stat(newConfigFilename)
	if os.IsNotExist(err) {
		// Check if a config file with old format exists
		_, err = os.Stat(oldConfigFilename)
		if os.IsNotExist(err) {
			return nil, err
		}

		job, err = CreateJobFromFile(oldConfigFilename)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		// Config file with new format might exist but an error occured
		return nil, err
	} else {
		// Config file with new format exists
		data, err := os.ReadFile(newConfigFilename)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(data, job)
		if err != nil {
			return nil, err
		}
	}
	return job, nil
}

// HandleGetBuild Returns information required to bootstrap build page
// @Summary      Return status of the build
// @Description  Contains information about the job and the latest build status
// @Tags         build
// @Produce      json
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {object}   GetBuildPayload
// @Failure      500      {string}   http.StatusInternalServerError
// @Failure      404      {string}   http.StatusNotFound
// @Router       /build/{id} [get]
func HandleGetBuild(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	idp := chi.URLParam(r, "id")
	buildID, err := strconv.Atoi(idp)
	if err != nil {
		logger.Error("parse build id", "id", idp, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	job, err := getBuildConfig(buildID)
	if err != nil {
		logger.Error("get build config", "build", buildID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	// Get build statusupdate
	var buildStatusData BuildUpdateData
	err = DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(HistoryBucket))
		ud := b.Get(Itob(buildID))
		if ud == nil {
			return errBuildNotFound
		}
		return json.Unmarshal(ud, &buildStatusData)
	})
	if err != nil {
		logger.Error("get build status", "build", buildID, "err", err)
		if errors.Is(err, errBuildNotFound) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte(err.Error()))
		}
		return
	}
	payload := GetBuildPayload{
		Job:          job,
		StatusUpdate: &buildStatusData,
	}

	payloadB, err := json.Marshal(payload)
	if err != nil {
		logger.Error("marshal build payload", "build", buildID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
}

type GetBuildPayload struct {
	Job          *Job             `json:"job"`
	StatusUpdate *BuildUpdateData `json:"status_update"`
}

// HandleAbortBuild aborts build
// @Summary      Abort the build
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {string}   string
// @Failure      500      {string}   http.StatusInternalServerError
// @Failure      404      {string}   http.StatusNotFound
// @Router       /build/{id}/abort [post]
func HandleAbortBuild(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	buildID := chi.URLParam(r, "id")
	id, err := strconv.Atoi(buildID)
	if err != nil {
		logger.Error("parse build id", "id", buildID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	err = GlobalQueue.Abort(id, StatusAborted)
	if err != nil {
		logger.Error("abort build", "build", id, "err", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

// HandleFlushTaskLogs signals to flush logs
// @Summary      Signal the build to flush its log buffer
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {string}   string
// @Failure      500      {string}   http.StatusInternalServerError
// @Failure      404      {string}   http.StatusNotFound
// @Router       /build/{id}/flush [post]
func HandleFlushTaskLogs(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	buildID := chi.URLParam(r, "id")
	id, err := strconv.Atoi(buildID)
	if err != nil {
		logger.Error("parse build id", "id", buildID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	err = GlobalQueue.FlushLogs(id)
	if err != nil {
		logger.Error("flush task logs", "build", id, "err", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

// HandleStartBuild takes the build from the queue and starts to run it immediately
// @Summary      Takes the build from the queue and starts to run it immediately, ignorring the number of allowed concurrent builds and concurrency parameter
// @Description  Not while wakeci is stopping (409)
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {string}   string
// @Failure      500      {string}   http.StatusInternalServerError
// @Failure      404      {string}   http.StatusNotFound
// @Failure      409      {string}   string
// @Router       /build/{id}/start [post]
func HandleStartBuild(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	buildID := chi.URLParam(r, "id")
	id, err := strconv.Atoi(buildID)
	if err != nil {
		logger.Error("parse build id", "id", buildID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	err = GlobalQueue.TakeNow(id)
	if errors.Is(err, ErrQueueStopping) {
		writePlainText(w, logger, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		logger.Error("start build", "build", id, "err", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
}
