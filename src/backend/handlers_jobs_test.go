package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

func withTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(JobsBucket)
		return err
	})
	if err != nil {
		t.Fatalf("create jobs bucket: %v", err)
	}

	old := DB
	DB = db
	oldCron := GlobalCron
	GlobalCron = cron.New()
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test db: %v", err)
		}
		DB = old
		GlobalCron = oldCron
	})
}

func postJobsCreate(name string) *httptest.ResponseRecorder {
	form := url.Values{"name": {name}}
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	HandleJobsCreate(rec, req)
	return rec
}

// TestHandleJobsCreateRejectsPathTraversal is a regression test: name used
// to be concatenated into Config.JobDir+name+ext with no validation, unlike
// every other job handler (which gets a slash-free name for free from chi's
// routing). A name containing ".." and "/" could write a job file outside
// JobDir entirely.
func TestHandleJobsCreateRejectsPathTraversal(t *testing.T) {
	jobDir := filepath.Join(t.TempDir(), "jobdir")
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		t.Fatalf("mkdir jobdir: %v", err)
	}
	Config = &WakeConfig{JobDir: jobDir + string(os.PathSeparator), jobsExt: ".yaml"}
	withTestDB(t)

	rec := postJobsCreate("../pwn")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	escaped := filepath.Join(filepath.Dir(jobDir), "pwn.yaml")
	if _, err := os.Stat(escaped); err == nil {
		t.Errorf("expected no file to be created outside JobDir, but found %q", escaped)
	}
}

// TestHandleJobsCreateStillAcceptsNormalNames verifies the fix doesn't
// regress the ordinary, valid case.
func TestHandleJobsCreateStillAcceptsNormalNames(t *testing.T) {
	jobDir := filepath.Join(t.TempDir(), "jobdir")
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		t.Fatalf("mkdir jobdir: %v", err)
	}
	Config = &WakeConfig{JobDir: jobDir + string(os.PathSeparator), jobsExt: ".yaml"}
	withTestDB(t)

	rec := postJobsCreate("my-normal-job")

	if rec.Code != 0 && rec.Code != http.StatusOK {
		t.Errorf("expected success, got status %d body %q", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(jobDir, "my-normal-job.yaml")); err != nil {
		t.Errorf("expected job file to be created: %v", err)
	}
}

// The job list is [] without jobs, and a job without parameters has
// defaultParams [], so clients don't have to handle null
func TestJobListHasNoNulls(t *testing.T) {
	newAPIKeyTestEnv(t)
	for _, name := range []string{"deploy", "nightly", "test_api"} {
		if rec := adminRequest("DELETE", "/api/job/"+name, nil); rec.Code != http.StatusOK {
			t.Fatalf("delete %s: %d", name, rec.Code)
		}
	}
	rec := adminRequest("GET", "/api/jobs/", nil)
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("no jobs: got %q, want []", body)
	}

	path := Config.JobDir + "plain" + Config.jobsExt
	if err := os.WriteFile(path, []byte("desc: plain\ntasks: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RegisterJob(path); err != nil {
		t.Fatal(err)
	}
	var jobs []map[string]any
	if err := json.Unmarshal(adminRequest("GET", "/api/jobs/", nil).Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0]["defaultParams"] == nil {
		t.Errorf("a job without parameters: got %v, want defaultParams []", jobs)
	}
}
