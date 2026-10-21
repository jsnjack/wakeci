package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

func TestRunJobUsesCurrentDefaults(t *testing.T) {
	for _, tt := range []struct {
		name           string
		current, value string
		defaults       []string
		want           string
		caller         bool
	}{
		{name: "unchanged default", current: "staging", value: "staging", defaults: []string{"VALUE"}, want: "staging"},
		{name: "stale default", current: "production", value: "staging", defaults: []string{"VALUE"}, want: "staging", caller: true},
		{name: "unchanged secret", current: "{{ secrets.A }}", value: "{{ secrets.A }}", defaults: []string{"VALUE"}, want: "{{ secrets.A }}"},
		{name: "stale secret", current: "{{ secrets.B }}", value: "{{ secrets.A }}", defaults: []string{"VALUE"}, want: "{{ secrets.A }}", caller: true},
		{name: "caller matches secret default", current: "{{ secrets.B }}", value: "{{ secrets.B }}", defaults: []string{}, want: "{{ secrets.B }}", caller: true},
		{name: "API secret override", current: "{{ secrets.B }}", value: "{{ secrets.B }}", want: "{{ secrets.B }}", caller: true},
		{name: "empty stale default", current: "production", defaults: []string{"VALUE"}, want: "", caller: true},
		{name: "empty caller override", current: "production", defaults: []string{}, want: "", caller: true},
		{name: "API empty value", current: "production", want: "production"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newTestBuildEnv(t)
			oldQueue := GlobalQueue
			GlobalQueue = &Queue{}
			t.Cleanup(func() { GlobalQueue = oldQueue })
			path := filepath.Join(Config.JobDir, "defaults.yaml")
			content := "params:\n  - VALUE: '" + tt.current + "'\ntasks: []\n"
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if err := RegisterJob(path); err != nil {
				t.Fatal(err)
			}
			build, err := runJob("defaults", url.Values{"VALUE": {tt.value}}, tt.defaults)
			if err != nil {
				t.Fatal(err)
			}
			if got := build.Params[0]["VALUE"]; got != tt.want {
				t.Errorf("parameter = %q, want %q", got, tt.want)
			}
			if got := build.callerParams["VALUE"]; got != tt.caller {
				t.Errorf("caller parameter = %t, want %t", got, tt.caller)
			}
		})
	}
}

func TestRunJobRejectsMalformedDefaultNames(t *testing.T) {
	for _, header := range []string{"not JSON", "null", "{}", `["VALUE", 1]`} {
		t.Run(header, func(t *testing.T) {
			newJobAccessTestEnv(t)
			req := httptest.NewRequest(http.MethodPost, "/api/job/test_unit/run", nil)
			req.SetBasicAuth("", "correct")
			req.Header.Set(defaultParamsHeader, header)
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("malformed defaults: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

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

	withTestDB(t)
	oldConfig := Config
	Config = &WakeConfig{JobDir: t.TempDir() + "/", jobsExt: ".yaml"}
	t.Cleanup(func() { Config = oldConfig })
	for _, tc := range tests {
		if tc.want != "" {
			if err := os.WriteFile(Config.JobDir+tc.want+Config.jobsExt, []byte("tasks: []\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Only registered jobs have names
	if err := DB.Update(func(tx *bolt.Tx) error {
		for _, tt := range tests {
			if tt.want != "" {
				if _, err := tx.Bucket(JobsBucket).CreateBucket([]byte(tt.want)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
