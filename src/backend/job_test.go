package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
)

func TestJobNameParamDecodesOneSafePathSegment(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		want       string
		wantStatus int
	}{
		{name: "reserved characters", path: "/api/job/job%23%3F%25%20name", want: "job#?% name", wantStatus: http.StatusOK},
		{name: "literal escape sequence", path: "/api/job/job%2523", want: "job%23", wantStatus: http.StatusOK},
		{name: "encoded separator", path: "/api/job/job%2Fescape", wantStatus: http.StatusBadRequest},
	}

	router := chi.NewRouter()
	router.Get("/api/job/{name}", func(w http.ResponseWriter, r *http.Request) {
		name, err := jobNameParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(name))
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.want != "" && rec.Body.String() != tt.want {
				t.Errorf("decoded job name = %q, want %q", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestAddToCronReplacesAllDuplicateEntries(t *testing.T) {
	oldCron := GlobalCron
	oldConfig := Config
	GlobalCron = cron.New()
	Config = &WakeConfig{}
	t.Cleanup(func() {
		GlobalCron = oldCron
		Config = oldConfig
	})

	interval := "0 0 * * *"
	for i := 0; i < 2; i++ {
		if _, err := GlobalCron.AddJob(interval, &Job{Name: "nightly"}); err != nil {
			t.Fatalf("add duplicate cron entry: %v", err)
		}
	}
	if _, err := GlobalCron.AddJob(interval, &Job{Name: "unrelated"}); err != nil {
		t.Fatalf("add unrelated cron entry: %v", err)
	}

	if err := (&Job{Name: "nightly", Interval: interval}).AddToCron(); err != nil {
		t.Fatalf("AddToCron: %v", err)
	}

	counts := make(map[string]int)
	for _, entry := range GlobalCron.Entries() {
		job, ok := entry.Job.(*Job)
		if !ok {
			continue
		}
		counts[job.Name]++
	}
	if counts["nightly"] != 1 {
		t.Errorf("nightly cron entries = %d, want 1", counts["nightly"])
	}
	if counts["unrelated"] != 1 {
		t.Errorf("unrelated cron entries = %d, want 1", counts["unrelated"])
	}
}

func TestIsValidJobName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "SimpleName", input: "my-job", wantErr: false},
		{name: "NameWithUnderscoreAndDigits", input: "job_123", wantErr: false},
		{name: "Empty", input: "", wantErr: true},
		{name: "ForwardSlash", input: "../../../../tmp/pwn", wantErr: true},
		{name: "SingleSlash", input: "a/b", wantErr: true},
		{name: "Backslash", input: `a\b`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidJobName(tt.input)
			if got == tt.wantErr {
				t.Errorf("isValidJobName(%q) = %v, wanted valid=%v", tt.input, got, !tt.wantErr)
			}
		})
	}
}

func TestGetJobNameFromPath(t *testing.T) {
	Config = &WakeConfig{jobsExt: ".yaml"}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{name: "SimpleName", path: "/jobs/build.yaml", expected: "build"},
		{name: "RelativePath", path: "build.yaml", expected: "build"},
		{name: "NameWithDashes", path: "/jobs/nightly-build.yaml", expected: "nightly-build"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetJobNameFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCreateJobFromFile(t *testing.T) {
	dir := t.TempDir()
	Config = &WakeConfig{JobDir: dir, jobsExt: ".yaml"}

	tests := []struct {
		name          string
		content       string
		expectErr     bool
		expectedTasks int
	}{
		{
			name: "SimpleJob",
			content: `desc: A test job
tasks:
  - name: step one
    run: echo one
  - name: step two
    run: echo two
`,
			expectedTasks: 2,
		},
		{
			name: "WithOnFailedAndFinally",
			content: `desc: A job with hooks
tasks:
  - name: main step
    run: echo main
on_failed:
  - name: notify
    run: echo failed
finally:
  - name: cleanup
    run: echo cleanup
`,
			expectedTasks: 3,
		},
		{
			name:      "InvalidYaml",
			content:   "tasks: [this is not valid",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := tt.name + ".yaml"
			path := filepath.Join(dir, filename)
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			job, err := CreateJobFromFile(path)
			if tt.expectErr {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if job.Name != tt.name {
				t.Errorf("expected job name %q, got %q", tt.name, job.Name)
			}
			if len(job.Tasks) != tt.expectedTasks {
				t.Errorf("expected %d tasks, got %d", tt.expectedTasks, len(job.Tasks))
			}
			for i, task := range job.Tasks {
				if task.ID != i {
					t.Errorf("expected task %d to have ID %d, got %d", i, i, task.ID)
				}
				if task.Status != StatusPending {
					t.Errorf("expected task %d to start pending, got %s", i, task.Status)
				}
			}
		})
	}
}

// RunJob sets the run's params before the build's first update and its
// on_pending tasks. It used to set them afterwards, so the feed got the
// job's defaults first (and dropped the build under a search for the run's
// value), and an on_pending task could run with the defaults.
func TestRunJobGivesTheFirstUpdateAndPendingTasksTheRunParams(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{}) // starts nothing, so the build stays queued
	oldCron := GlobalCron
	GlobalCron = cron.New()
	t.Cleanup(func() { GlobalCron = oldCron })
	var (
		mu   sync.Mutex
		sent []*MsgBroadcast
	)
	hub := newHub()
	oldHub := WSHub
	WSHub = hub
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop); WSHub = oldHub })
	go func() {
		for {
			select {
			case msg := <-hub.broadcast:
				mu.Lock()
				sent = append(sent, msg)
				mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
	path := Config.JobDir + "pending-params" + Config.jobsExt
	content := "desc: pending params\nparams:\n  - CITY: nowhere\non_pending:\n  - name: Record the city\n    run: echo \"$CITY\" > city\ntasks: []\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write job: %v", err)
	}
	if err := RegisterJob(path); err != nil {
		t.Fatalf("register job: %v", err)
	}

	build, err := RunJob("pending-params", url.Values{"CITY": {"minsk"}}, false, buildTrigger{Kind: TriggerPassword}, buildGroup{})
	if err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	build.pendingTasksWG.Wait()

	mu.Lock()
	var first *BuildUpdateData
	for _, msg := range sent {
		if msg.Type == "build:update:"+strconv.Itoa(build.ID) {
			first = &BuildUpdateData{}
			if err := json.Unmarshal(msg.Data.(json.RawMessage), first); err != nil {
				t.Fatalf("decode the first update: %v", err)
			}
			break
		}
	}
	mu.Unlock()
	if first == nil || len(first.Params) != 1 || first.Params[0]["CITY"] != "minsk" {
		t.Errorf("first update params = %v, want CITY=minsk", first)
	}
	city, err := os.ReadFile(filepath.Join(build.GetWorkspaceDir(), "city"))
	if err != nil || strings.TrimSpace(string(city)) != "minsk" {
		t.Errorf("on_pending task saw CITY = %q (%v), want minsk", city, err)
	}
	if build.Job.DefaultParams[0]["CITY"] != "nowhere" {
		t.Errorf("the job's default became %q; the run's value must go to a copy", build.Job.DefaultParams[0]["CITY"])
	}
}

// A run of a job whose file no longer parses says which file it is
func TestRunJobNamesTheFileThatDoesntParse(t *testing.T) {
	newAPIKeyTestEnv(t)
	path := Config.JobDir + "deploy" + Config.jobsExt
	if err := os.WriteFile(path, []byte("tasks:\n  - name: x\n    run: curl -H \"a: b\"\n  bad: [\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := RunJob("deploy", nil, false, buildTrigger{Kind: TriggerPassword}, buildGroup{})
	if err == nil || !strings.HasPrefix(err.Error(), "deploy"+Config.jobsExt+" can't be read: ") {
		t.Errorf("run of a broken job: %v", err)
	}
}
