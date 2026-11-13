package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Job insights read a job's builds from the history, which has each task's
// status and duration, and the task names from the builds' plans. They
// reach back as far as the history: the last buildHistorySize builds of all
// jobs.

const (
	defaultInsightBuilds = 40
	maxInsightBuilds     = 500
	// changeRuns is how many of a task's last runs Change compares with
	// the runs before; each side needs minChangeRuns
	changeRuns    = 10
	minChangeRuns = 5
	// maxParamValues is how many values of a parameter are listed
	maxParamValues = 10
)

// JobInsights is what a job's builds say about its tasks and failures
type JobInsights struct {
	Job string `json:"job"`
	// Builds are the job's last builds that ended, oldest first, as many as
	// asked for
	Builds []InsightBuild `json:"builds"`
	// InHistory is how many builds of the job that ended the history has
	InHistory int `json:"inHistory"`
	// Totals count Builds by status
	Totals StatusCounts `json:"totals"`
	// Duration is that of the builds that finished
	Duration Percentiles `json:"duration"`
	// Flaky counts the builds that failed or timed out and whose next build
	// with the same parameters finished; FlakyChecked those that had a next
	// build with the same parameters
	Flaky        int `json:"flaky"`
	FlakyChecked int `json:"flakyChecked"`
	// TimeToFix is from the first build of each failing streak to the end of
	// the build that finished after it
	TimeToFix Percentiles `json:"timeToFix"`
	// Failing is set if the job's last build that wasn't aborted failed or
	// timed out
	Failing *FailingStreak `json:"failing" extensions:"x-nullable"`
	// Tasks are the main tasks by name, in the order of the last build
	Tasks []TaskInsights `json:"tasks"`
	// StoppedAt counts the builds that failed or timed out by the main task
	// that stopped them
	StoppedAt []TaskCount `json:"stoppedAt"`
	// Params are the parameters with more than one value, and the success
	// rate of their values, the most used first
	Params []ParamInsights `json:"params"`
}

// InsightBuild is a build with its tasks
type InsightBuild struct {
	BuildOutcome
	Params map[string]string `json:"params"`
	// Tasks are the tasks that ran or were skipped
	Tasks []InsightTask `json:"tasks"`
}

// InsightTask is one task of a build
type InsightTask struct {
	Name string `json:"name"`
	// Kind is that of TaskStatus: main, or the hook it belongs to
	Kind   string     `json:"kind"`
	Status ItemStatus `json:"status"`
	// Duration is in nanoseconds
	Duration time.Duration `json:"duration" swaggertype:"integer"`
}

// TaskInsights is a main task's runs in the builds
type TaskInsights struct {
	Name string `json:"name"`
	// Runs counts the builds the task ran in; Failed those where it failed
	// or timed out, Skipped those where its condition was false
	Runs    int `json:"runs"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	// Duration is that of the runs that finished
	Duration Percentiles `json:"duration"`
	// Share is its part of the builds' time, from 0 to 1
	Share float64 `json:"share"`
	// Change is how much longer (above 0) or shorter the median of its last
	// 10 finished runs is than that of the runs before, from 0.25 for a
	// quarter longer; null with fewer than 5 runs before them
	Change *float64 `json:"change" extensions:"x-nullable"`
}

// TaskCount is how many builds a task stopped
type TaskCount struct {
	Task   string `json:"task"`
	Builds int    `json:"builds"`
}

// ParamInsights are a parameter's values
type ParamInsights struct {
	Name   string       `json:"name"`
	Values []ParamValue `json:"values"`
}

// ParamValue is how a parameter value's builds went
type ParamValue struct {
	Value string `json:"value"`
	StatusCounts
}

// historyBuild is a history entry with its task names
type historyBuild struct {
	msg   *BuildUpdateData
	names map[int]string
}

// flatParams returns a build's parameters by name
func flatParams(params []map[string]string) map[string]string {
	out := map[string]string{}
	for _, group := range params {
		maps.Copy(out, group)
	}
	return out
}

// computeInsights returns the insights of a job's builds, oldest first
func computeInsights(job string, builds []historyBuild, inHistory int) *JobInsights {
	data := &JobInsights{
		Job:       job,
		Builds:    make([]InsightBuild, 0, len(builds)),
		InHistory: inHistory,
		Tasks:     []TaskInsights{},
		StoppedAt: []TaskCount{},
		Params:    []ParamInsights{},
	}
	var durations []time.Duration
	var stats []*buildStats
	var totalTime time.Duration
	type taskAccumulator struct {
		TaskInsights
		durations []time.Duration
		time      time.Duration
	}
	tasks := map[string]*taskAccumulator{}
	stopped := map[string]int{}
	for _, b := range builds {
		msg := b.msg
		params := flatParams(msg.Params)
		build := InsightBuild{
			BuildOutcome: BuildOutcome{ID: msg.ID, Status: msg.Status, StartedAt: msg.StartedAt, Duration: msg.Duration},
			Params:       params,
			Tasks:        []InsightTask{},
		}
		for _, task := range msg.Tasks {
			if task.Status == StatusPending || task.Status == StatusRunning {
				continue
			}
			name := b.names[task.ID]
			if name == "" {
				name = fmt.Sprintf("task %d", task.ID)
			}
			build.Tasks = append(build.Tasks, InsightTask{Name: name, Kind: task.Kind, Status: task.Status, Duration: task.Duration})
			if task.Kind != KindMain {
				continue
			}
			acc := tasks[name]
			if acc == nil {
				acc = &taskAccumulator{TaskInsights: TaskInsights{Name: name}}
				tasks[name] = acc
			}
			switch task.Status {
			case StatusSkipped:
				acc.Skipped++
				continue
			case StatusFinished:
				acc.durations = append(acc.durations, task.Duration)
			case StatusFailed, StatusTimedOut:
				acc.Failed++
			}
			acc.Runs++
			acc.time += task.Duration
		}
		if id, ok := stoppedTask(msg.Tasks); ok {
			name := b.names[id]
			if name == "" {
				name = fmt.Sprintf("task %d", id)
			}
			build.FailedTask = name
			if msg.Status == StatusFailed || msg.Status == StatusTimedOut {
				stopped[name]++
			}
		}
		data.Builds = append(data.Builds, build)
		data.Totals.add(msg.Status)
		if msg.Status == StatusFinished && msg.Duration > 0 {
			durations = append(durations, msg.Duration)
		}
		totalTime += msg.Duration
		stats = append(stats, &buildStats{ID: msg.ID, Status: msg.Status, StartedAt: msg.StartedAt, Duration: msg.Duration, FailedTask: build.FailedTask})
	}
	data.Totals.rate()
	data.Duration = percentiles(durations)
	data.Failing = failingStreak(stats)
	data.Flaky, data.FlakyChecked = flakyFailures(data.Builds)
	data.TimeToFix = percentiles(timesToFix(data.Builds))

	// Tasks in the order of the last build, then those it no longer has
	order := map[string]int{}
	for i := len(data.Builds) - 1; i >= 0; i-- {
		for _, task := range data.Builds[i].Tasks {
			if _, ok := order[task.Name]; !ok && task.Kind == KindMain {
				order[task.Name] = len(order)
			}
		}
	}
	for _, acc := range tasks {
		acc.Duration = percentiles(slices.Clone(acc.durations))
		if totalTime > 0 {
			acc.Share = float64(acc.time) / float64(totalTime)
		}
		acc.Change = taskChange(acc.durations)
		data.Tasks = append(data.Tasks, acc.TaskInsights)
	}
	slices.SortFunc(data.Tasks, func(a, b TaskInsights) int { return cmp.Compare(order[a.Name], order[b.Name]) })

	for task, n := range stopped {
		data.StoppedAt = append(data.StoppedAt, TaskCount{Task: task, Builds: n})
	}
	slices.SortFunc(data.StoppedAt, func(a, b TaskCount) int {
		return cmp.Or(cmp.Compare(b.Builds, a.Builds), cmp.Compare(a.Task, b.Task))
	})
	data.Params = paramInsights(data.Builds)
	return data
}

// taskChange compares the median of a task's last runs with that of the runs
// before, given its durations in the order the builds ran
func taskChange(durations []time.Duration) *float64 {
	split := len(durations) - changeRuns
	if split < minChangeRuns {
		return nil
	}
	before := percentiles(slices.Clone(durations[:split]))
	last := percentiles(slices.Clone(durations[split:]))
	if before.P50 <= 0 {
		return nil
	}
	change := float64(last.P50)/float64(before.P50) - 1
	return &change
}

// flakyFailures counts the builds that failed or timed out and whose next
// build with the same parameters, aborted ones left out, finished
func flakyFailures(builds []InsightBuild) (flaky, checked int) {
	for i, b := range builds {
		if b.Status != StatusFailed && b.Status != StatusTimedOut {
			continue
		}
		for _, next := range builds[i+1:] {
			if next.Status == StatusAborted || !maps.Equal(next.Params, b.Params) {
				continue
			}
			checked++
			if next.Status == StatusFinished {
				flaky++
			}
			break
		}
	}
	return flaky, checked
}

// timesToFix returns, for each failing streak followed by a finished build,
// the time from the start of its first build to the end of that build
func timesToFix(builds []InsightBuild) []time.Duration {
	var fixes []time.Duration
	var first *InsightBuild
	for i := range builds {
		b := &builds[i]
		switch b.Status {
		case StatusFailed, StatusTimedOut:
			if first == nil {
				first = b
			}
		case StatusFinished:
			if first != nil && !first.StartedAt.IsZero() && !b.StartedAt.IsZero() {
				fixes = append(fixes, b.StartedAt.Add(b.Duration).Sub(first.StartedAt))
			}
			first = nil
		}
	}
	return fixes
}

// paramInsights returns the parameters that had more than one value, with
// how the builds of each value went
func paramInsights(builds []InsightBuild) []ParamInsights {
	byName := map[string]map[string]*StatusCounts{}
	for _, b := range builds {
		for name, value := range b.Params {
			if byName[name] == nil {
				byName[name] = map[string]*StatusCounts{}
			}
			counts := byName[name][value]
			if counts == nil {
				counts = &StatusCounts{}
				byName[name][value] = counts
			}
			counts.add(b.Status)
		}
	}
	out := []ParamInsights{}
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		values := byName[name]
		if len(values) < 2 {
			continue
		}
		param := ParamInsights{Name: name, Values: []ParamValue{}}
		for value, counts := range values {
			counts.rate()
			param.Values = append(param.Values, ParamValue{Value: value, StatusCounts: *counts})
		}
		slices.SortFunc(param.Values, func(a, b ParamValue) int {
			return cmp.Or(cmp.Compare(b.Builds, a.Builds), cmp.Compare(a.Value, b.Value))
		})
		if len(param.Values) > maxParamValues {
			param.Values = param.Values[:maxParamValues]
		}
		out = append(out, param)
	}
	return out
}

// readJobHistory returns the job's last builds that ended, at most limit,
// oldest first, and how many such builds the history has
func readJobHistory(tx *bolt.Tx, job string, limit int) ([]*BuildUpdateData, int) {
	var builds []*BuildUpdateData
	count := 0
	c := tx.Bucket(HistoryBucket).Cursor()
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		var msg BuildUpdateData
		if err := json.Unmarshal(v, &msg); err != nil || msg.Name != job {
			continue
		}
		switch msg.Status {
		case StatusFinished, StatusFailed, StatusAborted, StatusTimedOut:
		default:
			continue
		}
		// Its on_* and finally tasks still run
		if GlobalQueue.Verify(msg.ID) {
			continue
		}
		count++
		if len(builds) < limit {
			builds = append(builds, &msg)
		}
	}
	slices.Reverse(builds)
	return builds, count
}

// HandleJobInsights returns what a job's last builds say about its tasks
// and failures
// @Summary      Insights into a job's last builds: its tasks, failures and parameters
// @Description  Reads the job's builds still in the history (the last buildHistorySize builds of all jobs): each build's tasks and their durations, each main task's runs, failures and share of the build time, the task that stopped failed builds, flaky failures (a failed build whose next build with the same parameters finished), the time from a failure to the next finished build, and the success rate of each parameter value. API key scope: read
// @Tags         job
// @Produce      json
// @Param        name     path    string    true   "Name of the job"
// @Param        builds   query   integer   false  "How many of the last builds, 1 to 500; 40 if not set"
// @Success      200      {object}   JobInsights
// @Success      304      {string}   string
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/job/{name}/insights [get]
func HandleJobInsights(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	name, ok := requireJobName(w, r, logger)
	if !ok {
		return
	}
	limit := defaultInsightBuilds
	if s := r.URL.Query().Get("builds"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxInsightBuilds {
			writePlainText(w, logger, http.StatusBadRequest, fmt.Sprintf("builds must be a number from 1 to %d", maxInsightBuilds))
			return
		}
		limit = n
	}
	var msgs []*BuildUpdateData
	var inHistory int
	if err := DB.View(func(tx *bolt.Tx) error {
		msgs, inHistory = readJobHistory(tx, name, limit)
		return nil
	}); err != nil {
		logger.Error("read build history", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	// The plans are files, read outside the transaction
	builds := make([]historyBuild, 0, len(msgs))
	for _, msg := range msgs {
		names := map[int]string{}
		if job, err := getBuildConfig(msg.ID); err == nil {
			for _, task := range job.Tasks {
				names[task.ID] = displayTaskName(task)
			}
		}
		builds = append(builds, historyBuild{msg: msg, names: names})
	}
	writeCachedJSON(w, r, logger, computeInsights(name, builds, inHistory))
}
