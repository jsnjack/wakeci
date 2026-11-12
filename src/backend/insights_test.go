package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// insightTestBuilds are six builds of a job with the tasks checkout and
// test and an on_failed hook, an hour apart
func insightTestBuilds() []historyBuild {
	start := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	names := map[int]string{0: "checkout", 1: "test", 2: "notify"}
	m := time.Minute
	build := func(id int, status ItemStatus, branch string, duration time.Duration, tasks ...*TaskStatus) historyBuild {
		return historyBuild{names: names, msg: &BuildUpdateData{
			ID: id, Name: "e2e", Status: status, StartedAt: start.Add(time.Duration(id-1) * time.Hour), Duration: duration,
			// env has one value, so it says nothing
			Params: []map[string]string{{"branch": branch}, {"env": "ci"}},
			Tasks:  tasks,
		}}
	}
	task := func(id int, status ItemStatus, d time.Duration, kind string) *TaskStatus {
		return &TaskStatus{ID: id, Status: status, Duration: d, Kind: kind}
	}
	return []historyBuild{
		build(1, StatusFinished, "master", 10*m, task(0, StatusFinished, m, KindMain), task(1, StatusFinished, 9*m, KindMain), task(2, StatusPending, 0, StatusFailed)),
		build(2, StatusFailed, "feat", 5*m, task(0, StatusFinished, m, KindMain), task(1, StatusFailed, 3*m, KindMain), task(2, StatusFinished, m, StatusFailed)),
		// Same params as 2, and finished: 2 was flaky
		build(3, StatusFinished, "feat", 12*m, task(0, StatusFinished, m, KindMain), task(1, StatusFinished, 11*m, KindMain), task(2, StatusPending, 0, StatusFailed)),
		build(4, StatusFailed, "master", m, task(0, StatusFailed, m, KindMain), task(1, StatusPending, 0, KindMain), task(2, StatusFinished, 0, StatusFailed)),
		build(5, StatusAborted, "master", m, task(0, StatusAborted, m, KindMain), task(1, StatusPending, 0, KindMain)),
		build(6, StatusFailed, "master", 4*m, task(0, StatusFinished, m, KindMain), task(1, StatusFailed, 3*m, KindMain)),
	}
}

func TestComputeInsights(t *testing.T) {
	data := computeInsights("e2e", insightTestBuilds(), 9)
	if data.Builds[0].ID != 1 || len(data.Builds) != 6 || data.InHistory != 9 {
		t.Fatalf("builds = %d from %d, in history %d", len(data.Builds), data.Builds[0].ID, data.InHistory)
	}
	if data.Totals != (StatusCounts{Builds: 6, Finished: 2, Failed: 3, Aborted: 1, SuccessRate: data.Totals.SuccessRate}) || *data.Totals.SuccessRate != 0.4 {
		t.Errorf("totals = %+v", data.Totals)
	}
	// 4 is followed by 6, which failed too; 6 has no next build
	if data.Flaky != 1 || data.FlakyChecked != 2 {
		t.Errorf("flaky %d of %d, want 1 of 2", data.Flaky, data.FlakyChecked)
	}
	// From the start of 2 to the end of 3: an hour and 12 minutes
	if data.TimeToFix != (Percentiles{Count: 1, P50: 72 * time.Minute, P95: 72 * time.Minute}) {
		t.Errorf("time to fix = %+v", data.TimeToFix)
	}
	if data.Failing == nil || data.Failing.Builds != 2 || data.Failing.First.ID != 4 || data.Failing.Last.FailedTask != "test" {
		t.Errorf("failing = %+v, want 4 and 6, aborted 5 skipped", data.Failing)
	}
	// The aborted build doesn't count
	if !equalJSON(data.StoppedAt, []TaskCount{{"test", 2}, {"checkout", 1}}) {
		t.Errorf("stopped at = %+v", data.StoppedAt)
	}
	if len(data.Tasks) != 2 || data.Tasks[0].Name != "checkout" || data.Tasks[1].Name != "test" {
		t.Fatalf("tasks = %+v, want the main tasks in order", data.Tasks)
	}
	checkout, test := data.Tasks[0], data.Tasks[1]
	if checkout.Runs != 6 || checkout.Failed != 1 || checkout.Duration.P50 != time.Minute {
		t.Errorf("checkout = %+v", checkout)
	}
	if test.Runs != 4 || test.Failed != 2 || test.Duration.Count != 2 || test.Change != nil {
		t.Errorf("test = %+v", test)
	}
	// 26 of the builds' 33 minutes
	if share := test.Share; share < 0.787 || share > 0.788 {
		t.Errorf("test share = %v, want 26/33", share)
	}
	wantParams := []ParamInsights{{Name: "branch", Values: []ParamValue{
		{Value: "master", StatusCounts: StatusCounts{Builds: 4, Finished: 1, Failed: 2, Aborted: 1, SuccessRate: ptr(1.0 / 3)}},
		{Value: "feat", StatusCounts: StatusCounts{Builds: 2, Finished: 1, Failed: 1, SuccessRate: ptr(0.5)}},
	}}}
	if !equalJSON(data.Params, wantParams) {
		t.Errorf("params = %+v, want %+v", data.Params, wantParams)
	}
	// Hooks that ran are listed with their kind, tasks that didn't run aren't
	second := data.Builds[1]
	if len(second.Tasks) != 3 || second.Tasks[2].Name != "notify" || second.Tasks[2].Kind != StatusFailed || second.FailedTask != "test" {
		t.Errorf("second build = %+v", second)
	}
	if len(data.Builds[3].Tasks) != 2 {
		t.Errorf("fourth build's tasks = %+v, want checkout and the hook", data.Builds[3].Tasks)
	}
}

func TestTaskChange(t *testing.T) {
	durations := func(before, after int, a, b time.Duration) []time.Duration {
		var out []time.Duration
		for range before {
			out = append(out, a)
		}
		for range after {
			out = append(out, b)
		}
		return out
	}
	if got := taskChange(durations(5, 10, 10*time.Second, 15*time.Second)); got == nil || *got != 0.5 {
		t.Errorf("15 runs, last 10 half as long again: %v", got)
	}
	if got := taskChange(durations(4, 10, 10*time.Second, 15*time.Second)); got != nil {
		t.Errorf("14 runs: %v, want null", *got)
	}
}

func TestJobInsightsAPI(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	start := time.Now().Add(-time.Hour)
	running := &Build{ID: 13, Job: &Job{Name: "deploy"}}
	useQueue(t, &Queue{running: []*Build{running}})
	tasks := func(status ItemStatus) []*TaskStatus {
		return []*TaskStatus{{ID: 0, Status: StatusFinished, Duration: time.Second, Kind: KindMain}, {ID: 1, Status: status, Duration: time.Second, Kind: KindMain}}
	}
	putTestHistory(t,
		&BuildUpdateData{ID: 10, Name: "deploy", Status: StatusFinished, StartedAt: start, Duration: 2 * time.Second, Tasks: tasks(StatusFinished)},
		&BuildUpdateData{ID: 11, Name: "deploy", Status: StatusFailed, StartedAt: start, Duration: 2 * time.Second, Tasks: tasks(StatusFailed)},
		&BuildUpdateData{ID: 12, Name: "nightly", Status: StatusFinished, StartedAt: start},
		// Failed, but its hooks still run
		&BuildUpdateData{ID: 13, Name: "deploy", Status: StatusFailed, StartedAt: start, Tasks: tasks(StatusFailed)},
	)
	writeTestPlan(t, 10, "deploy", "Build", "Ship")
	writeTestPlan(t, 11, "deploy", "Build", "Ship")

	rec := adminRequest("GET", "/api/job/deploy/insights?builds=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var data JobInsights
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Build 1 of newAPIKeyTestEnv is deploy's too
	if data.Job != "deploy" || data.InHistory != 3 || len(data.Builds) != 2 || data.Builds[0].ID != 10 || data.Builds[1].ID != 11 {
		t.Errorf("insights: job %s, in history %d, builds %+v", data.Job, data.InHistory, data.Builds)
	}
	if !equalJSON(data.StoppedAt, []TaskCount{{"Ship", 1}}) {
		t.Errorf("stopped at = %+v, want the name from the plan", data.StoppedAt)
	}

	addTestGroup(t, "Night shift", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"nightly"}})
	cookie := loginInGroups(t, "nat@example.com", "Night shift")
	if rec := apiCall("GET", "/api/job/deploy/insights", cookie, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("another group's job: %d, want 403", rec.Code)
	}
	if rec := apiCall("GET", "/api/job/nightly/insights", cookie, nil, nil); rec.Code != http.StatusOK {
		t.Errorf("own job: %d", rec.Code)
	}
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"read"}, "job": {"deploy"}})
	if rec := keyRequest(router, "GET", "/api/job/deploy/insights", token, nil); rec.Code != http.StatusOK {
		t.Errorf("API key: %d", rec.Code)
	}
	for _, bad := range []string{"0", "501", "x"} {
		if rec := adminRequest("GET", "/api/job/deploy/insights?builds="+bad, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("builds=%s: %d, want 400", bad, rec.Code)
		}
	}
}
