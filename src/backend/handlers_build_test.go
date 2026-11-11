package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

func TestHandleGetBuildReturnsNotFoundForMissingHistory(t *testing.T) {
	dir := t.TempDir()
	originalConfig := Config
	originalDB := DB
	Config = &WakeConfig{
		WorkDir: dir + string(os.PathSeparator),
		jobsExt: ".yaml",
	}
	t.Cleanup(func() {
		Config = originalConfig
		DB = originalDB
	})

	buildDir := filepath.Join(dir, "wakespace", "42")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatalf("create build directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "build_plan.yaml"), []byte("name: test\ntasks: []\n"), 0644); err != nil {
		t.Fatalf("write build plan: %v", err)
	}

	db, err := bolt.Open(filepath.Join(dir, "build.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open build test db: %v", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(HistoryBucket)
		return err
	}); err != nil {
		t.Fatalf("create history bucket: %v", err)
	}
	DB = db
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close build test db: %v", err)
		}
	})

	router := chi.NewRouter()
	router.Get("/api/build/{id}", HandleGetBuild)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/build/42", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestBuildRoutesClientErrors(t *testing.T) {
	newAPIKeyTestEnv(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }

	tests := []struct {
		method, target string
		wantStatus     int
		wantBody       string
	}{
		{"GET", "/api/build/1", http.StatusOK, `"name":"deploy"`},
		{"GET", "/api/build/99", http.StatusNotFound, "build 99 not found on this server, which holds builds 1 to 2"},
		{"GET", "/api/build/x", http.StatusBadRequest, `invalid build ID "x"`},
		{"POST", "/api/build/x/abort", http.StatusBadRequest, `invalid build ID "x"`},
		{"POST", "/api/build/x/flush", http.StatusBadRequest, `invalid build ID "x"`},
		{"POST", "/api/build/x/start", http.StatusBadRequest, `invalid build ID "x"`},
		{"GET", "/api/feed?offset=-1", http.StatusBadRequest, `invalid offset: "-1", want 0 to`},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := apiCall(tt.method, tt.target, nil, nil, basicAuth)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantStatus != http.StatusOK && rec.Header().Get("Content-Type") != "text/plain" {
				t.Errorf("Content-Type = %q, want text/plain", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestReadBuildStatusWithEmptyHistory(t *testing.T) {
	newTestBuildEnv(t)
	_, err := readBuildStatus(5)
	if err == nil || err.Error() != "build 5 not found on this server, which has no builds" {
		t.Fatalf("err = %v, want build 5 not found on this server, which has no builds", err)
	}
}

// A build's JSON names its tasks and says where their logs and its
// artifacts are, so clients don't put the paths together themselves
func TestBuildFileURLs(t *testing.T) {
	newAPIKeyTestEnv(t)
	build := BuildUpdateData{
		ID: 7, Name: "deploy", Status: StatusFailed,
		Tasks:          []*TaskStatus{{ID: 0, Status: StatusFailed}, {ID: 1, Status: StatusSkipped}},
		BuildArtifacts: []*ArtifactInfo{{Filename: "artifacts/selenium/report #1.json", Size: 12}},
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
	plan := "name: deploy\ntasks:\n  - id: 0\n    name: Run the tests\n  - id: 1\n    name: Upload the report\n"
	if err := os.WriteFile(filepath.Join(dir, "build_plan.yaml"), []byte(plan), 0644); err != nil {
		t.Fatalf("write build plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task_0.log"), []byte("FAILED test_login\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }

	rec := apiCall("GET", "/api/build/7", nil, nil, basicAuth)
	var payload struct {
		StatusUpdate BuildUpdateData `json:"status_update"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	tasks := payload.StatusUpdate.Tasks
	if len(tasks) != 2 {
		t.Fatalf("tasks = %+v, want 2", tasks)
	}
	if tasks[0].Name != "Run the tests" || tasks[0].Log != "/storage/build/7/task_0.log" || tasks[0].LogSize != 18 {
		t.Errorf("task 0 = %+v, want its name, log URL and size", tasks[0])
	}
	if tasks[1].Name != "Upload the report" || tasks[1].Log != "" || tasks[1].LogSize != 0 {
		t.Errorf("task 1 = %+v, want its name and no log", tasks[1])
	}
	wantURL := "/storage/build/7/artifacts/artifacts/selenium/report%20%231.json"
	if got := payload.StatusUpdate.BuildArtifacts[0].URL; got != wantURL {
		t.Errorf("artifact URL = %q, want %q", got, wantURL)
	}

	rec = apiCall("GET", "/api/feed?limit=1", nil, nil, basicAuth)
	if !strings.Contains(rec.Body.String(), `"url":"`+wantURL+`"`) {
		t.Errorf("feed = %s, want the artifact URL", rec.Body.String())
	}
	// The history keeps none of it
	if err := DB.View(func(tx *bolt.Tx) error {
		if stored := string(tx.Bucket(HistoryBucket).Get(Itob(7))); strings.Contains(stored, "url") || strings.Contains(stored, "Run the tests") {
			t.Errorf("history entry = %s, want no URLs or names", stored)
		}
		return nil
	}); err != nil {
		t.Fatalf("read history: %v", err)
	}
}
