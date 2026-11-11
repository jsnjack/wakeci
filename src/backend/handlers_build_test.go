package main

import (
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
		{"GET", "/api/feed?offset=-1", http.StatusBadRequest, `Invalid offset: "-1"`},
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
