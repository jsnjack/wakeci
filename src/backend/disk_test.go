package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// writeTestFile writes size bytes at path, below the work dir
func writeTestFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeTestBuildFiles gives build id logs, artifacts and a workspace of the
// given sizes
func writeTestBuildFiles(t *testing.T, id, logs, artifacts, workspace int) {
	t.Helper()
	writeTestFile(t, wakespaceDir(id)+"task_0.log", logs)
	writeTestFile(t, wakespaceDir(id)+"artifacts/report/index.html", artifacts)
	writeTestFile(t, Config.WorkDir+"workspace/"+strconv.Itoa(id)+"/src/main.go", workspace)
}

func TestMeasureBuildDisk(t *testing.T) {
	newTestBuildEnv(t)
	writeTestBuildFiles(t, 5, 10, 100, 7)
	// A file named like the artifacts folder isn't an artifact
	writeTestFile(t, wakespaceDir(5)+"artifacts.txt", 3)
	if got, want := *measureBuildDisk(5), (buildDisk{Logs: 13, Artifacts: 100, Workspace: 7}); got != want {
		t.Errorf("measured %+v, want %+v", got, want)
	}
	if got := *measureBuildDisk(6); got != (buildDisk{}) {
		t.Errorf("a build without files measured %+v", got)
	}
}

func TestJobInsightsAddUpDisk(t *testing.T) {
	newAPIKeyTestEnv(t)
	useQueue(t, &Queue{})
	start := time.Now().Add(-time.Hour)
	putTestHistory(t,
		&BuildUpdateData{ID: 10, Name: "deploy", Status: StatusFinished, StartedAt: start},
		&BuildUpdateData{ID: 11, Name: "deploy", Status: StatusFailed, StartedAt: start},
		&BuildUpdateData{ID: 12, Name: "nightly", Status: StatusFinished, StartedAt: start},
	)
	putTestStats(t,
		&buildStats{ID: 10, Job: "deploy", Status: StatusFinished, Disk: &buildDisk{Logs: 1, Artifacts: 20, Workspace: 300}},
		&buildStats{ID: 11, Job: "deploy", Status: StatusFailed, Disk: &buildDisk{Logs: 2, Artifacts: 40}},
		&buildStats{ID: 12, Job: "nightly", Status: StatusFinished, Disk: &buildDisk{Logs: 1000}},
	)
	rec := adminRequest("GET", "/api/job/deploy/insights?builds=1", nil)
	var data JobInsights
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("insights: %d %s", rec.Code, rec.Body.String())
	}
	// All of the job's builds count, not only those asked for; build 1 of
	// the test environment has no size
	if want := (JobDisk{Builds: 2, Unmeasured: 1, Logs: 3, Artifacts: 60, Workspace: 300}); data.Disk != want {
		t.Errorf("disk %+v, want %+v", data.Disk, want)
	}
}

func TestBackfillBuildDisk(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	start := time.Now().Add(-time.Hour)
	putTestHistory(t,
		&BuildUpdateData{ID: 20, Name: "deploy", Status: StatusFinished, StartedAt: start},
		&BuildUpdateData{ID: 21, Name: "deploy", Status: StatusRunning, StartedAt: start},
		&BuildUpdateData{ID: 22, Name: "deploy", Status: StatusFinished, StartedAt: start},
	)
	measured := &buildDisk{Logs: 5}
	putTestStats(t,
		&buildStats{ID: 20, Job: "deploy", Status: StatusFinished},
		&buildStats{ID: 21, Job: "deploy", Status: StatusRunning},
		&buildStats{ID: 22, Job: "deploy", Status: StatusFinished, Disk: measured},
	)
	writeTestBuildFiles(t, 20, 10, 100, 7)
	writeTestBuildFiles(t, 22, 1, 1, 1)

	BackfillBuildDisk(context.Background())
	_ = DB.View(func(tx *bolt.Tx) error {
		if got := getBuildStats(tx, 20).Disk; got == nil || *got != (buildDisk{Logs: 10, Artifacts: 100, Workspace: 7}) {
			t.Errorf("build 20: %+v, want it measured", got)
		}
		if got := getBuildStats(tx, 21).Disk; got != nil {
			t.Errorf("a running build was measured: %+v", got)
		}
		if got := getBuildStats(tx, 22).Disk; got == nil || *got != *measured {
			t.Errorf("build 22 was measured again: %+v", got)
		}
		return nil
	})

	// A stopping wakeci measures nothing
	putTestStats(t, &buildStats{ID: 20, Job: "deploy", Status: StatusFinished})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	BackfillBuildDisk(ctx)
	_ = DB.View(func(tx *bolt.Tx) error {
		if got := getBuildStats(tx, 20).Disk; got != nil {
			t.Errorf("measured after the stop: %+v", got)
		}
		return nil
	})
}

// A build is measured once it has left the queue, in the background, and
// its stats record gets the size
func TestSaveDiskLater(t *testing.T) {
	newTestBuildEnv(t)
	putTestStats(t, &buildStats{ID: 5, Job: "deploy", Status: StatusFinished})
	writeTestBuildFiles(t, 5, 10, 100, 7)
	saveDiskLater(&Build{ID: 5, Logger: L})
	want := buildDisk{Logs: 10, Artifacts: 100, Workspace: 7}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got *buildDisk
		if err := DB.View(func(tx *bolt.Tx) error {
			if record := getBuildStats(tx, 5); record != nil {
				got = record.Disk
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if got != nil && *got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stats record's disk %+v, want %+v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
