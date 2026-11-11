package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v2"
)

var errBuildNotFound = errors.New("not found")

// buildIDParam reads the build ID from the URL
func buildIDParam(r *http.Request) (int, error) {
	idp := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idp)
	if err != nil {
		return 0, fmt.Errorf("invalid build ID %q", idp)
	}
	return id, nil
}

// readBuildStatus reads a build's entry in the history
func readBuildStatus(id int) (*BuildUpdateData, error) {
	var build BuildUpdateData
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(HistoryBucket)
		data := b.Get(Itob(id))
		if data == nil {
			return buildNotFound(b.Cursor(), id)
		}
		if err := json.Unmarshal(data, &build); err != nil {
			return fmt.Errorf("read build %d: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &build, nil
}

// buildNotFound names the builds this server holds, because clients mix up
// the build IDs of different wakeci servers
func buildNotFound(c *bolt.Cursor, id int) error {
	first, _ := c.First()
	last, _ := c.Last()
	if first == nil {
		return fmt.Errorf("build %d %w on this server, which has no builds", id, errBuildNotFound)
	}
	return fmt.Errorf("build %d %w on this server, which holds builds %d to %d",
		id, errBuildNotFound, binary.BigEndian.Uint64(first), binary.BigEndian.Uint64(last))
}

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
// @Description  Contains information about the job and the latest build status. API key scope: read
// @Tags         build
// @Produce      json
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {object}   GetBuildPayload
// @Failure      400      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/build/{id} [get]
func HandleGetBuild(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	buildID, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}

	buildStatusData, err := readBuildStatus(buildID)
	if errors.Is(err, errBuildNotFound) {
		writePlainText(w, logger, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		logger.Error("get build status", "build", buildID, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}

	job, err := getBuildConfig(buildID)
	if err != nil {
		logger.Error("get build config", "build", buildID, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}

	payload := GetBuildPayload{
		Job:          job,
		StatusUpdate: buildStatusData,
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
// @Description  API key scope: run
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/build/{id}/abort [post]
func HandleAbortBuild(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	id, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
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
// @Description  API key scope: run
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/build/{id}/flush [post]
func HandleFlushTaskLogs(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)

	id, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
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
// @Description  Not while wakeci is stopping (409). API key scope: run
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true  "Build ID"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Failure      409      {string}   string
// @Router       /api/build/{id}/start [post]
func HandleStartBuild(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	id, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
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
