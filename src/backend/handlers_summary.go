package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Sizes of log excerpts
const (
	defaultSummaryTail = 100
	maxTailLines       = 10000
	// maxExcerptBytes caps an excerpt, since one line can be huge
	maxExcerptBytes = 256 * 1024
	maxGrepMatches  = 1000
	maxGrepLength   = 200
	// maxSummaryTails is how many failed tasks get the end of their log
	maxSummaryTails = 10
)

// failedTaskStatuses are the statuses of tasks whose logs a summary shows
var failedTaskStatuses = []ItemStatus{StatusFailed, StatusTimedOut, StatusAborted}

// BuildSummary is a build in one response, for agents and scripts: its
// status, its tasks and artifacts, and the end of the logs of the tasks that
// went wrong
type BuildSummary struct {
	ID     int        `json:"id"`
	Job    string     `json:"job"`
	Status ItemStatus `json:"status"`
	// Page is the build's page in the web interface
	Page         string              `json:"page"`
	StartedAt    time.Time           `json:"startedAt"`
	Duration     time.Duration       `json:"duration"`
	DurationText string              `json:"durationText"`
	Params       []map[string]string `json:"params"`
	// JobSummary is what the job wrote to $WAKE_SUMMARY, empty if nothing
	JobSummary string        `json:"jobSummary"`
	Tasks      []*TaskStatus `json:"tasks"`
	// Failed are the tasks that failed, timed out or were aborted, with the
	// end of their logs
	Failed    []FailedTask    `json:"failed"`
	Artifacts []*ArtifactInfo `json:"artifacts"`
}

// FailedTask is a task that went wrong, with the end of its log
type FailedTask struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Kind   string     `json:"kind"`
	Status ItemStatus `json:"status"`
	Log    string     `json:"log"`
	// Tail is the end of the log, at most the lines asked for
	Tail string `json:"tail"`
}

// HandleBuildSummary returns a build with the end of the logs of its failed
// tasks
// @Summary      Summarize a build: status, tasks, artifacts and the end of the logs of failed tasks
// @Description  One request for "why did this build fail". jobSummary is what the job wrote to $WAKE_SUMMARY; failed lists the tasks that failed, timed out or were aborted, with the last lines of their logs. API key scope: read
// @Tags         build
// @Produce      json
// @Param        id       path    integer   true   "Build ID"
// @Param        tail     query   integer   false  "Lines from the end of each failed task's log, 0 to 10000; 100 if not set"
// @Param        wait     query   integer   false  "Wait up to that many seconds, at most 300, for the build to end; it answers as soon as it does"
// @Success      200      {object}   BuildSummary
// @Failure      400      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/build/{id}/summary [get]
func HandleBuildSummary(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	id, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	tail, err := positiveParam(r.URL.Query(), "tail", 0, maxTailLines, defaultSummaryTail)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	wait, err := positiveParam(r.URL.Query(), "wait", 0, maxBuildWait, 0)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	if err := waitForBuild(r, id, wait); err != nil && !errors.Is(err, errBuildNotFound) {
		logger.Error("wait for build", "build", id, "err", err)
	}
	build, err := readBuildStatus(id)
	if errors.Is(err, errBuildNotFound) {
		writePlainText(w, logger, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		logger.Error("get build status", "build", id, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	job, err := getBuildConfig(id)
	if err != nil {
		logger.Error("get build config", "build", id, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	addTaskLogs(build, job)
	addArtifactURLs(build)
	jobSummary, err := readJobSummary(id)
	if err != nil {
		logger.Error("read job summary", "build", id, "err", err)
	}

	summary := BuildSummary{
		JobSummary:   jobSummary,
		ID:           build.ID,
		Job:          build.Name,
		Status:       build.Status,
		Page:         "/build/" + strconv.Itoa(build.ID),
		StartedAt:    build.StartedAt,
		Duration:     build.Duration,
		DurationText: build.Duration.Round(time.Second).String(),
		Params:       build.Params,
		Tasks:        build.Tasks,
		Failed:       []FailedTask{},
		Artifacts:    build.BuildArtifacts,
	}
	if summary.Tasks == nil {
		summary.Tasks = []*TaskStatus{}
	}
	if summary.Artifacts == nil {
		summary.Artifacts = []*ArtifactInfo{}
	}
	for _, task := range build.Tasks {
		if !slices.Contains(failedTaskStatuses, task.Status) || len(summary.Failed) >= maxSummaryTails {
			continue
		}
		failed := FailedTask{ID: task.ID, Name: task.Name, Kind: task.Kind, Status: task.Status, Log: task.Log}
		if task.Log != "" && tail > 0 {
			failed.Tail, err = tailLines(taskLogFile(id, task.ID), tail)
			if err != nil {
				logger.Error("read task log", "build", id, "task", task.ID, "err", err)
			}
		}
		summary.Failed = append(summary.Failed, failed)
	}
	writeJSON(w, logger, summary)
}

// HandleTaskLog returns a task's log, its end, or the lines that contain a
// text
// @Summary      Read a task's log, its last lines, or the lines that contain a text
// @Description  Without tail or grep, the whole log (Range works). grep returns matching lines, ignoring case, each after its line number and a colon, at most 1000. With both, the last tail matching lines. API key scope: read
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true   "Build ID"
// @Param        task     path    integer   true   "Task ID, from status_update.tasks"
// @Param        tail     query   integer   false  "Only the last lines, 1 to 10000"
// @Param        grep     query   string    false  "Only lines that contain this text, ignoring case"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Failure      404      {string}   string
// @Failure      500      {string}   string
// @Router       /api/build/{id}/log/{task} [get]
func HandleTaskLog(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	id, err := buildIDParam(r)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	taskParam := chi.URLParam(r, "task")
	task, err := strconv.Atoi(taskParam)
	if err != nil || task < 0 {
		writePlainText(w, logger, http.StatusBadRequest, fmt.Sprintf("invalid task ID %q", taskParam))
		return
	}
	query := r.URL.Query()
	tail, err := positiveParam(query, "tail", 1, maxTailLines, 0)
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	grep := query.Get("grep")
	if len(grep) > maxGrepLength {
		writePlainText(w, logger, http.StatusBadRequest, fmt.Sprintf("grep is longer than %d bytes", maxGrepLength))
		return
	}

	if _, err := readBuildStatus(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errBuildNotFound) {
			status = http.StatusNotFound
		}
		writePlainText(w, logger, status, err.Error())
		return
	}
	path := taskLogFile(id, task)
	if _, err := os.Stat(path); err != nil {
		writePlainText(w, logger, http.StatusNotFound, fmt.Sprintf("task %d of build %d wrote no log", task, id))
		return
	}
	var text string
	switch {
	case grep != "":
		text, err = grepLines(path, grep, tail)
	case tail > 0:
		text, err = tailLines(path, tail)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeFile(w, r, path)
		return
	}
	if err != nil {
		logger.Error("read task log", "build", id, "task", task, "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	writePlainText(w, logger, http.StatusOK, text)
}

func taskLogFile(build, task int) string {
	return Config.WorkDir + "wakespace/" + strconv.Itoa(build) + "/" + fmt.Sprintf("task_%d.log", task)
}

// tailLines returns the last n lines of the file at path, reading it from
// the end, at most maxExcerptBytes of them
func tailLines(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open log: %w", err)
	}
	defer closeLog(f, path)
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat log: %w", err)
	}
	const chunk = 64 * 1024
	var data []byte
	offset := info.Size()
	// One newline more than lines, since the file usually ends with one
	for offset > 0 && bytes.Count(data, []byte("\n")) <= n && len(data) < maxExcerptBytes {
		read := min(int64(chunk), offset)
		offset -= read
		part := make([]byte, read)
		if _, err := f.ReadAt(part, offset); err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read log: %w", err)
		}
		data = append(part, data...)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if offset > 0 && len(lines) > 0 {
		// The first line read is only part of a line
		lines = lines[1:]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return cutExcerpt(strings.Join(lines, "\n")), nil
}

// grepLines returns the lines of the file at path that contain text,
// ignoring case, after their line numbers: the first maxGrepMatches, or the
// last tail if tail is set
func grepLines(path, text string, tail int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open log: %w", err)
	}
	defer closeLog(f, path)
	wanted := strings.ToLower(text)
	limit := maxGrepMatches
	if tail > 0 {
		limit = tail
	}
	var matches []string
	more := 0
	reader := bufio.NewReader(f)
	for number := 1; ; number++ {
		line, err := reader.ReadString('\n')
		if line != "" && strings.Contains(strings.ToLower(line), wanted) {
			match := strconv.Itoa(number) + ": " + strings.TrimRight(line, "\n")
			switch {
			case tail > 0:
				// Keep the last ones
				matches = append(matches, match)
				if len(matches) > limit {
					matches = matches[1:]
					more++
				}
			case len(matches) < limit:
				matches = append(matches, match)
			default:
				more++
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read log: %w", err)
		}
	}
	result := strings.Join(matches, "\n")
	if more > 0 && tail == 0 {
		result += fmt.Sprintf("\n... %d more matching lines", more)
	}
	return cutExcerpt(result), nil
}

// cutExcerpt keeps the end of text within maxExcerptBytes
func cutExcerpt(text string) string {
	if len(text) <= maxExcerptBytes {
		return text
	}
	return "...\n" + strings.ToValidUTF8(text[len(text)-maxExcerptBytes:], "")
}

func closeLog(f *os.File, path string) {
	if err := f.Close(); err != nil {
		L.Log(context.Background(), LevelTrace, "close log", "path", path, "err", err)
	}
}
