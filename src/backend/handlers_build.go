package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

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
// @Description  Contains the job and the build's status. Each task in status_update.tasks has its name, and the URL (log) and size (logSize) of its log if it wrote one; each artifact in status_update.build_artifacts has its download url. API key scope: read
// @Tags         build
// @Produce      json
// @Param        id       path    integer   true  "Build ID"
// @Param        wait     query   integer   false "Wait up to that many seconds, at most 300, for the build to end; it answers as soon as it does"
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
	wait, err := positiveParam(r.URL.Query(), "wait", 0, maxBuildWait, 0)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	if err := waitForBuild(r, buildID, wait); err != nil && !errors.Is(err, errBuildNotFound) {
		logger.Error("wait for build", "build", buildID, "err", err)
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
	addTaskLogs(buildStatusData, job)
	addArtifactURLs(buildStatusData)
	summary, err := readJobSummary(buildID)
	if err != nil {
		logger.Error("read job summary", "build", buildID, "err", err)
	}

	payload := GetBuildPayload{
		Job:          job,
		StatusUpdate: buildStatusData,
		Summary:      summary,
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

// addTaskLogs names the build's tasks and says where their logs are and how
// big, so clients neither join the task lists by index nor ask for logs of
// tasks that never ran
func addTaskLogs(build *BuildUpdateData, job *Job) {
	names := make(map[int]string, len(job.Tasks))
	for _, task := range job.Tasks {
		names[task.ID] = task.Name
	}
	dir := Config.WorkDir + "wakespace/" + strconv.Itoa(build.ID) + "/"
	for _, task := range build.Tasks {
		task.Name = names[task.ID]
		file := fmt.Sprintf("task_%d.log", task.ID)
		if info, err := os.Stat(dir + file); err == nil {
			task.Log = storageURL(build.ID, file)
			task.LogSize = info.Size()
		}
	}
}

// addArtifactURLs says where the build's artifacts can be downloaded: under
// artifacts/ in its storage, with the path the job collected them from
func addArtifactURLs(build *BuildUpdateData) {
	for _, artifact := range build.BuildArtifacts {
		artifact.URL = storageURL(build.ID, "artifacts/"+artifact.Filename)
	}
}

// storageURL is the URL of file, a path in a build's storage
func storageURL(id int, file string) string {
	parts := strings.Split(file, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/storage/build/" + strconv.Itoa(id) + "/" + strings.Join(parts, "/")
}

type GetBuildPayload struct {
	Job          *Job             `json:"job"`
	StatusUpdate *BuildUpdateData `json:"status_update"`
	// Summary is what the job wrote to $WAKE_SUMMARY, empty if nothing
	Summary string `json:"summary"`
}

// jobSummaryFile is where a build's tasks write its summary, in its
// wakespace; tasks get the path as WAKE_SUMMARY
const jobSummaryFile = "summary.md"

// maxJobSummary caps the summary a job wrote, which pages and agents read
// whole
const maxJobSummary = 64 * 1024

// readJobSummary returns what the build's tasks wrote to $WAKE_SUMMARY, at
// most maxJobSummary bytes, or "" if they wrote nothing
func readJobSummary(buildID int) (string, error) {
	f, err := os.Open(Config.WorkDir + "wakespace/" + strconv.Itoa(buildID) + "/" + jobSummaryFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open job summary: %w", err)
	}
	defer closeLog(f, jobSummaryFile)
	data, err := io.ReadAll(io.LimitReader(f, maxJobSummary+1))
	if err != nil {
		return "", fmt.Errorf("read job summary: %w", err)
	}
	if len(data) > maxJobSummary {
		return strings.ToValidUTF8(string(data[:maxJobSummary]), "") + "\n... (cut at 64 KB)", nil
	}
	return strings.ToValidUTF8(string(data), ""), nil
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
