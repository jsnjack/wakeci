package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	if len(jobs) != 1 || jobs[0]["defaultParams"] == nil || jobs[0]["labels"] == nil {
		t.Errorf("a job without parameters: got %v, want defaultParams and labels []", jobs)
	}
}

func TestJobLabels(t *testing.T) {
	params := []map[string]string{{"COMMIT": ""}, {"AUTHOR_LABEL": "unknown"}, {"TARGET": "staging"}, {"BRANCH_LABEL": "main"}}
	tests := []struct {
		name   string
		labels []string
		want   []string
	}{
		{"params named *_LABEL, in the order of params", nil, []string{"AUTHOR_LABEL", "BRANCH_LABEL"}},
		{"the labels list, in its order", []string{"BRANCH_LABEL", "COMMIT"}, []string{"BRANCH_LABEL", "COMMIT"}},
		{"names that aren't params left out, and repeats", []string{"MISSING", "TARGET", "TARGET"}, []string{"TARGET"}},
		{"a list of only unknown names: none, not the *_LABEL ones", []string{"MISSING"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := Job{DefaultParams: params, Labels: tt.labels}
			if got := job.labelNames(); !slices.Equal(got, tt.want) {
				t.Errorf("labelNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The jobs list sends each job's labels and param_info, as registering its
// file found them
func TestJobListSendsLabels(t *testing.T) {
	newAPIKeyTestEnv(t)
	path := Config.JobDir + "labelled" + Config.jobsExt
	content := "labels: [BRANCH, AUTHOR]\nparams:\n  - AUTHOR: unknown\n  - BRANCH: main\nparam_info:\n  BRANCH: {desc: Branch to test, options: [main, dev]}\n  GONE: {type: bool}\ngroup: Tests\ntasks: []\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RegisterJob(path); err != nil {
		t.Fatal(err)
	}
	var jobs []JobsListData
	if err := json.Unmarshal(adminRequest("GET", "/api/jobs/", nil).Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(jobs, func(job JobsListData) bool { return job.Name == "labelled" })
	if i < 0 || !slices.Equal(jobs[i].Labels, []string{"BRANCH", "AUTHOR"}) {
		t.Errorf("jobs = %+v, want labelled with labels [BRANCH AUTHOR]", jobs)
	}
	// What the run form says about params, for params only
	if info := jobs[i].ParamInfo; len(info) != 1 || info["BRANCH"].Desc != "Branch to test" || !slices.Equal(info["BRANCH"].Options, []string{"main", "dev"}) {
		t.Errorf("paramInfo = %+v, want BRANCH's only", info)
	}
	if jobs[i].Group != "Tests" {
		t.Errorf("group = %q, want Tests", jobs[i].Group)
	}
}

// With board=1 the jobs list adds each job's last builds, oldest first, its
// failing streak and its next scheduled run (J1)
func TestJobListBoard(t *testing.T) {
	newAPIKeyTestEnv(t)
	err := DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(BuildStatsBucket)
		if err != nil {
			return err
		}
		for i, status := range []ItemStatus{StatusFinished, StatusFailed, StatusAborted, StatusFailed} {
			record, _ := json.Marshal(buildStats{ID: 100 + i, Job: "deploy", Status: status})
			if err := b.Put(Itob(100+i), record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var jobs []JobsListData
	if err := json.Unmarshal(adminRequest("GET", "/api/jobs/?board=1", nil).Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(jobs, func(job JobsListData) bool { return job.Name == "deploy" })
	if i < 0 {
		t.Fatalf("no deploy in %+v", jobs)
	}
	ids := []int{}
	for _, b := range jobs[i].Recent {
		ids = append(ids, b.ID)
	}
	if !slices.Equal(ids, []int{100, 101, 102, 103}) {
		t.Errorf("recent = %v, want 100..103 oldest first", ids)
	}
	if f := jobs[i].Failing; f == nil || f.Builds != 2 || f.First.ID != 101 || f.Last.ID != 103 {
		t.Errorf("failing = %+v, want 2 builds from 101 to 103", f)
	}
	// Without board the list stays as it was
	var plain []map[string]any
	if err := json.Unmarshal(adminRequest("GET", "/api/jobs/", nil).Body.Bytes(), &plain); err != nil {
		t.Fatal(err)
	}
	for _, job := range plain {
		if _, ok := job["recent"]; ok {
			t.Errorf("recent without board: %v", job)
		}
	}
}

// The board shows 20 builds, but the failing streak goes on to its start:
// 25 failures in a row, and a streak behind 20 aborted builds
func TestJobListBoardStreakPastTheShownBuilds(t *testing.T) {
	newAPIKeyTestEnv(t)
	put := func(job string, statuses []ItemStatus, from int) {
		t.Helper()
		if err := DB.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(BuildStatsBucket)
			if err != nil {
				return err
			}
			for i, status := range statuses {
				record, _ := json.Marshal(buildStats{ID: from + i, Job: job, Status: status})
				if err := b.Put(Itob(from+i), record); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	failures := []ItemStatus{StatusFinished}
	for range 25 {
		failures = append(failures, StatusFailed)
	}
	put("deploy", failures, 100)
	behind := []ItemStatus{StatusFinished, StatusFailed, StatusTimedOut}
	for range 20 {
		behind = append(behind, StatusAborted)
	}
	put("nightly", behind, 200)
	var jobs []JobsListData
	if err := json.Unmarshal(adminRequest("GET", "/api/jobs/?board=1", nil).Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	failing := map[string]*FailingStreak{}
	for _, job := range jobs {
		failing[job.Name] = job.Failing
	}
	if f := failing["deploy"]; f == nil || f.Builds != 25 || f.First.ID != 101 || f.Last.ID != 125 {
		t.Errorf("deploy failing = %+v, want 25 builds from 101 to 125", f)
	}
	if f := failing["nightly"]; f == nil || f.Builds != 2 || f.First.ID != 201 || f.Last.ID != 202 {
		t.Errorf("nightly failing = %+v, want 2 builds from 201 to 202 behind the aborted ones", f)
	}
}
