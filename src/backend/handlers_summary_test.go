package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func writeLog(t *testing.T, path string, lines int, last string) {
	t.Helper()
	var text strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&text, "line %d\n", i)
	}
	text.WriteString(last)
	if err := os.WriteFile(path, []byte(text.String()), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}
}

func TestTailLines(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name  string
		lines int
		last  string
		n     int
		want  string
	}{
		{"last lines", 10, "", 3, "line 8\nline 9\nline 10"},
		{"more than the log has", 2, "", 5, "line 1\nline 2"},
		{"no newline at the end", 3, "partial", 2, "line 3\npartial"},
		// Over several 64 KB chunks
		{"long log", 200000, "", 2, "line 199999\nline 200000"},
		{"empty log", 0, "", 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "_"))
			writeLog(t, path, tt.lines, tt.last)
			got, err := tailLines(path, tt.n)
			if err != nil {
				t.Fatalf("tailLines: %v", err)
			}
			if got != tt.want {
				t.Errorf("tailLines = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGrepLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.log")
	content := "ok\nFAILED test_login\nok\nfailed test_logout\nok\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	tests := []struct {
		text string
		tail int
		want string
	}{
		{"failed", 0, "2: FAILED test_login\n4: failed test_logout"},
		{"failed", 1, "4: failed test_logout"},
		{"nothing", 0, ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s tail %d", tt.text, tt.tail), func(t *testing.T) {
			got, err := grepLines(path, tt.text, tt.tail)
			if err != nil {
				t.Fatalf("grepLines: %v", err)
			}
			if got != tt.want {
				t.Errorf("grepLines = %q, want %q", got, tt.want)
			}
		})
	}

	many := filepath.Join(t.TempDir(), "many.log")
	writeLog(t, many, maxGrepMatches+5, "")
	got, err := grepLines(many, "line", 0)
	if err != nil {
		t.Fatalf("grepLines: %v", err)
	}
	if !strings.HasSuffix(got, "\n... 5 more matching lines") || strings.Count(got, "\n") != maxGrepMatches {
		t.Errorf("grepLines kept %d lines, ending %q; want %d and a count of the rest", strings.Count(got, "\n"), got[len(got)-40:], maxGrepMatches)
	}
}

// addFailedTestBuild adds build 7 of deploy: task 0 finished, task 1 failed
// with a 300-line log, task 2 skipped without a log
func addFailedTestBuild(t *testing.T) {
	t.Helper()
	build := BuildUpdateData{
		ID: 7, Name: "deploy", Status: StatusFailed,
		Tasks: []*TaskStatus{
			{ID: 0, Status: StatusFinished, Kind: "main"},
			{ID: 1, Status: StatusFailed, Kind: "main"},
			{ID: 2, Status: StatusSkipped, Kind: "main"},
		},
		BuildArtifacts: []*ArtifactInfo{{Filename: "report.json", Size: 2}},
		Params:         []map[string]string{{"TARGET": "staging"}},
	}
	data, err := json.Marshal(build)
	if err != nil {
		t.Fatalf("marshal build: %v", err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error { return tx.Bucket(HistoryBucket).Put(Itob(7), data) }); err != nil {
		t.Fatalf("save build: %v", err)
	}
	dir := filepath.Join(Config.WorkDir, "wakespace", "7")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create wakespace: %v", err)
	}
	plan := "name: deploy\ntasks:\n  - id: 0\n    name: Build\n  - id: 1\n    name: Test\n  - id: 2\n    name: Upload\n"
	if err := os.WriteFile(filepath.Join(dir, "build_plan.yaml"), []byte(plan), 0644); err != nil {
		t.Fatalf("write build plan: %v", err)
	}
	writeLog(t, filepath.Join(dir, "task_0.log"), 3, "")
	writeLog(t, filepath.Join(dir, "task_1.log"), 299, "FAILED test_login\n")
}

func TestBuildSummary(t *testing.T) {
	newAPIKeyTestEnv(t)
	addFailedTestBuild(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }

	rec := apiCall("GET", "/api/build/7/summary?tail=2", nil, nil, basicAuth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %q", rec.Code, rec.Body.String())
	}
	var summary BuildSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if summary.Job != "deploy" || summary.Status != StatusFailed || summary.Page != "/build/7" || summary.Params[0]["TARGET"] != "staging" {
		t.Errorf("summary = %+v, want deploy, failed, its page and params", summary)
	}
	if len(summary.Tasks) != 3 || summary.Tasks[2].Name != "Upload" || summary.Tasks[2].Log != "" {
		t.Errorf("tasks = %+v, want three, named, the skipped one without a log", summary.Tasks)
	}
	want := FailedTask{ID: 1, Name: "Test", Kind: "main", Status: StatusFailed, Log: "/storage/build/7/task_1.log", Tail: "line 299\nFAILED test_login"}
	if len(summary.Failed) != 1 || summary.Failed[0] != want {
		t.Errorf("failed = %+v, want %+v", summary.Failed, want)
	}
	if len(summary.Artifacts) != 1 || summary.Artifacts[0].URL != "/storage/build/7/artifacts/report.json" {
		t.Errorf("artifacts = %+v, want report.json with its URL", summary.Artifacts)
	}

	// Without tail, the default; with tail=0, no excerpts
	rec = apiCall("GET", "/api/build/7/summary", nil, nil, basicAuth)
	if !strings.Contains(rec.Body.String(), `line 201\n`) || strings.Contains(rec.Body.String(), `line 200\n`) {
		t.Errorf("default summary should end the log with its last 100 lines: %s", rec.Body.String())
	}
	rec = apiCall("GET", "/api/build/7/summary?tail=0", nil, nil, basicAuth)
	if strings.Contains(rec.Body.String(), "FAILED test_login") {
		t.Errorf("tail=0 summary has an excerpt: %s", rec.Body.String())
	}
}

func TestBuildSummarySendsEmptyParams(t *testing.T) {
	newAPIKeyTestEnv(t)
	addFailedTestBuild(t)
	for _, tt := range []struct {
		name   string
		params string
	}{
		{"legacy null params", "null"},
		{"empty params", "[]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{"id":7,"name":"deploy","status":"finished","params":` + tt.params + `}`)
			if err := DB.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(HistoryBucket).Put(Itob(7), data)
			}); err != nil {
				t.Fatalf("save build: %v", err)
			}
			rec := apiCall("GET", "/api/build/7/summary", nil, nil, func(r *http.Request) {
				r.SetBasicAuth("", "admin")
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body %q", rec.Code, rec.Body.String())
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
				t.Fatalf("decode summary: %v", err)
			}
			if got := string(fields["params"]); got != "[]" {
				t.Errorf("params = %s, want []", got)
			}
		})
	}
}

func TestTaskLogRoute(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	addFailedTestBuild(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }

	tests := []struct {
		target     string
		wantStatus int
		wantBody   string
	}{
		{"/api/build/7/log/1?tail=1", http.StatusOK, "FAILED test_login"},
		{"/api/build/7/log/1?grep=failed", http.StatusOK, "300: FAILED test_login"},
		{"/api/build/7/log/1?grep=line+29&tail=1", http.StatusOK, "299: line 299"},
		{"/api/build/7/log/0", http.StatusOK, "line 1\nline 2\nline 3\n"},
		{"/api/build/7/log/2", http.StatusNotFound, "task 2 of build 7 wrote no log"},
		{"/api/build/99/log/0", http.StatusNotFound, "build 99 not found"},
		{"/api/build/7/log/x", http.StatusBadRequest, `invalid task ID "x"`},
		{"/api/build/7/log/1?tail=0", http.StatusBadRequest, "invalid tail"},
		{"/api/build/7/log/1?grep=" + strings.Repeat("a", maxGrepLength+1), http.StatusBadRequest, "grep is longer"},
		{"/api/build/7/summary?tail=-1", http.StatusBadRequest, "invalid tail"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := apiCall("GET", tt.target, nil, nil, basicAuth)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}

	// API keys read both with the read scope, on their jobs only
	deploy := createTestKey(t, url.Values{"name": {"deploy"}, "scope": {"read"}, "job": {"deploy"}})
	nightly := createTestKey(t, url.Values{"name": {"nightly"}, "scope": {"read"}, "job": {"nightly"}})
	runOnly := createTestKey(t, url.Values{"name": {"run"}, "scope": {"run"}, "job": {"deploy"}})
	for _, target := range []string{"/api/build/7/summary", "/api/build/7/log/1?tail=1"} {
		if rec := keyRequest(router, "GET", target, deploy, nil); rec.Code != http.StatusOK {
			t.Errorf("%s with a deploy key: %d", target, rec.Code)
		}
		if rec := keyRequest(router, "GET", target, nightly, nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s with a nightly key: %d, want 403", target, rec.Code)
		}
		if rec := keyRequest(router, "GET", target, runOnly, nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s with a run key: %d, want 403", target, rec.Code)
		}
	}
}

// What a job writes to $WAKE_SUMMARY reaches the build JSON and the
// summary, cut at 64 KB
func TestJobSummary(t *testing.T) {
	newAPIKeyTestEnv(t)
	addFailedTestBuild(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }
	build := &Build{ID: 7, Job: &Job{Name: "deploy"}}
	var summaryPath string
	for _, env := range build.generateDefaultEnvVariables() {
		if value, ok := strings.CutPrefix(env, "WAKE_SUMMARY="); ok {
			summaryPath = value
		}
	}
	if summaryPath != filepath.Join(Config.WorkDir, "wakespace", "7", "summary.md") {
		t.Fatalf("WAKE_SUMMARY = %q, want summary.md in the build's wakespace", summaryPath)
	}

	readSummaries := func() (string, string) {
		t.Helper()
		var payload GetBuildPayload
		if err := json.Unmarshal(apiCall("GET", "/api/build/7", nil, nil, basicAuth).Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode build: %v", err)
		}
		var summary BuildSummary
		if err := json.Unmarshal(apiCall("GET", "/api/build/7/summary", nil, nil, basicAuth).Body.Bytes(), &summary); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		return payload.Summary, summary.JobSummary
	}

	if build, summary := readSummaries(); build != "" || summary != "" {
		t.Errorf("without a summary file: %q, %q; want empty", build, summary)
	}
	text := "### Failed tests\n- test_login\n"
	if err := os.WriteFile(summaryPath, []byte(text), 0644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if build, summary := readSummaries(); build != text || summary != text {
		t.Errorf("summaries = %q, %q; want %q", build, summary, text)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Repeat("x", maxJobSummary+10)), 0644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if build, _ := readSummaries(); len(build) != maxJobSummary+len("\n... (cut at 64 KB)") || !strings.HasSuffix(build, "(cut at 64 KB)") {
		t.Errorf("long summary: %d bytes ending %q, want it cut at 64 KB", len(build), build[len(build)-20:])
	}
}
