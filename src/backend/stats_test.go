package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// readStats returns the build's stats record, failing the test without one
func readStats(t *testing.T, id int) *buildStats {
	t.Helper()
	var record *buildStats
	if err := DB.View(func(tx *bolt.Tx) error {
		record = getBuildStats(tx, id)
		return nil
	}); err != nil {
		t.Fatalf("read stats: %v", err)
	}
	if record == nil {
		t.Fatalf("build %d has no stats record", id)
	}
	return record
}

func putTestStats(t *testing.T, records ...*buildStats) {
	t.Helper()
	if err := DB.Update(func(tx *bolt.Tx) error {
		for _, record := range records {
			data, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if err := putBuildStats(tx, data, record.ID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("save stats: %v", err)
	}
}

func putTestHistory(t *testing.T, msgs ...*BuildUpdateData) {
	t.Helper()
	if err := DB.Update(func(tx *bolt.Tx) error {
		for _, msg := range msgs {
			data, err := json.Marshal(msg)
			if err != nil {
				return err
			}
			if err := tx.Bucket(HistoryBucket).Put(Itob(msg.ID), data); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("save history: %v", err)
	}
}

// writeTestPlan writes a build's plan with tasks named by their IDs
func writeTestPlan(t *testing.T, id int, job string, names ...string) {
	t.Helper()
	dir := wakespaceDir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create wakespace: %v", err)
	}
	plan := "name: " + job + "\ntasks:\n"
	for i, name := range names {
		plan += "  - id: " + strconv.Itoa(i) + "\n    name: " + name + "\n"
	}
	if err := os.WriteFile(dir+"build_plan.yaml", []byte(plan), 0644); err != nil {
		t.Fatalf("write build plan: %v", err)
	}
}

// A build saves its record with each update: who started it, when it was
// queued and started, and once it ended, its duration and the task that
// stopped it
func TestBuildStatsFollowTheBuild(t *testing.T) {
	build := abortTestBuild(t, &Job{Name: "stats", Tasks: []*Task{
		{ID: 0, Name: "Compile", Command: "true", Kind: KindMain},
		{ID: 1, Name: "Test", Command: "exit 1", Kind: KindMain},
		{ID: 2, Name: "Notify", Command: "sleep 0.1", Kind: StatusFailed},
	}})
	trigger := buildTrigger{Kind: TriggerAPIKey, User: "sam@example.com", APIKey: "abc123"}
	build.mutex.Lock()
	build.trigger = trigger
	build.mutex.Unlock()
	build.BroadcastUpdate()

	queued := readStats(t, build.ID)
	if queued.Status != StatusPending || queued.Ended || queued.QueuedAt.IsZero() || queued.Trigger != trigger {
		t.Errorf("queued record = %+v, want pending with the queue time and trigger", queued)
	}

	waitDone(t, startBuild(build), 20*time.Second)
	got := readStats(t, build.ID)
	if got.Status != StatusFailed || !got.Ended || got.FailedTask != "Test" || got.Job != "stats" {
		t.Errorf("record = %+v, want failed at Test and ended", got)
	}
	if got.Trigger != trigger || !got.QueuedAt.Equal(queued.QueuedAt) {
		t.Errorf("record lost the trigger or queue time: %+v", got)
	}
	// The on_failed task runs before the build ends
	if got.Duration < 100*time.Millisecond || got.StartedAt.Before(got.QueuedAt) {
		t.Errorf("duration %v, started %v, queued %v: want the whole build", got.Duration, got.StartedAt, got.QueuedAt)
	}
	if wait, ok := got.wait(); !ok || wait < 0 {
		t.Errorf("wait = %v, %t", wait, ok)
	}
}

func TestTriggerOf(t *testing.T) {
	tests := []struct {
		auth, user, apiKey string
		want               buildTrigger
	}{
		{"password", "", "", buildTrigger{Kind: TriggerPassword}},
		{"basic", "", "", buildTrigger{Kind: TriggerPassword}},
		{"sso", "sam@example.com", "", buildTrigger{Kind: TriggerSSO, User: "sam@example.com"}},
		{"apikey", "sam@example.com", "abc", buildTrigger{Kind: TriggerAPIKey, User: "sam@example.com", APIKey: "abc"}},
		{"", "", "", buildTrigger{}},
	}
	for _, tt := range tests {
		r, _ := withRequestInfo(httpRequest())
		setRequestUser(r, tt.auth, tt.user, tt.apiKey)
		if got := triggerOf(r); got != tt.want {
			t.Errorf("auth %q: trigger = %+v, want %+v", tt.auth, got, tt.want)
		}
	}
}

func httpRequest() *http.Request {
	r, _ := http.NewRequest("GET", "/", nil)
	return r
}

// Each way of starting a build records who started it
func TestRunJobRecordsTrigger(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	useQueue(t, &Queue{})
	token := createTestKey(t, url.Values{"name": {"ci"}, "scope": {"run"}, "job": {"deploy"}})
	keyID := strings.Split(token, "_")[1]

	runs := []struct {
		name string
		run  func() string
		want buildTrigger
	}{
		{"admin password", func() string {
			return adminRequest("POST", "/api/job/deploy/run", nil).Body.String()
		}, buildTrigger{Kind: TriggerPassword}},
		{"single sign-on", func() string {
			return apiCall("POST", "/api/job/deploy/run", loginAs(t, "sam@example.com"), nil, nil).Body.String()
		}, buildTrigger{Kind: TriggerSSO, User: "sam@example.com"}},
		{"API key", func() string {
			return keyRequest(router, "POST", "/api/job/deploy/run", token, nil).Body.String()
		}, buildTrigger{Kind: TriggerAPIKey, APIKey: keyID}},
		{"interval", func() string {
			(&Job{Name: "deploy"}).Run()
			GlobalQueue.mutex.Lock()
			defer GlobalQueue.mutex.Unlock()
			return strconv.Itoa(GlobalQueue.queued[len(GlobalQueue.queued)-1].ID)
		}, buildTrigger{Kind: TriggerInterval}},
	}
	for _, tt := range runs {
		t.Run(tt.name, func(t *testing.T) {
			var id int
			if err := json.Unmarshal([]byte(tt.run()), &id); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := readStats(t, id); got.Trigger != tt.want || got.Status != StatusPending {
				t.Errorf("record = %+v, want trigger %+v", got, tt.want)
			}
		})
	}
}

// A queued build keeps its trigger and queue time over a restart
func TestRestoredBuildKeepsTrigger(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	build := queueTestBuild(t, &Job{Name: "restore", Tasks: []*Task{}})
	build.mutex.Lock()
	build.trigger = buildTrigger{Kind: TriggerSSO, User: "sam@example.com"}
	build.mutex.Unlock()
	build.BroadcastUpdate()

	var record []byte
	if err := DB.View(func(tx *bolt.Tx) error {
		record = append(record, tx.Bucket(QueueBucket).Get(Itob(build.ID))...)
		return nil
	}); err != nil {
		t.Fatalf("read queued build: %v", err)
	}
	restored, err := restoreBuild(build.ID, record)
	if err != nil {
		t.Fatalf("restoreBuild: %v", err)
	}
	if restored.trigger != build.trigger || !restored.queuedAt.Equal(build.queuedAt) {
		t.Errorf("restored trigger %+v queued %v, want %+v %v", restored.trigger, restored.queuedAt, build.trigger, build.queuedAt)
	}
}

// Builds that ended before wakeci recorded stats get a record from their
// history entry, with the task that stopped them from their plan
func TestBackfillBuildStats(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	started := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	putTestHistory(t,
		&BuildUpdateData{ID: 1, Name: "deploy", Status: StatusFinished, StartedAt: started, Duration: time.Minute},
		&BuildUpdateData{ID: 2, Name: "deploy", Status: StatusFailed, StartedAt: started, Duration: time.Minute,
			Tasks: []*TaskStatus{{ID: 0, Status: StatusFinished, Kind: KindMain}, {ID: 1, Status: StatusFailed, Kind: KindMain}}},
		// Never started: no day to count it on
		&BuildUpdateData{ID: 3, Name: "deploy", Status: StatusAborted},
		// Queued again after the restart: writes its own
		&BuildUpdateData{ID: 4, Name: "deploy", Status: StatusPending},
		&BuildUpdateData{ID: 5, Name: "deploy", Status: StatusFinished, StartedAt: started},
	)
	writeTestPlan(t, 2, "deploy", "Build", "Test")
	existing := &buildStats{ID: 5, Job: "deploy", Status: StatusFinished, Trigger: buildTrigger{Kind: TriggerInterval}, Ended: true}
	putTestStats(t, existing)

	if err := BackfillBuildStats(); err != nil {
		t.Fatalf("BackfillBuildStats: %v", err)
	}
	if got := readStats(t, 1); got.Status != StatusFinished || !got.Ended || got.Duration != time.Minute || !got.StartedAt.Equal(started) || got.Trigger.Kind != "" {
		t.Errorf("finished build = %+v", got)
	}
	if got := readStats(t, 2); got.FailedTask != "Test" {
		t.Errorf("failed build = %+v, want failed at Test", got)
	}
	if got := readStats(t, 5); got.Trigger.Kind != TriggerInterval {
		t.Errorf("existing record = %+v, want it kept", got)
	}
	if err := DB.View(func(tx *bolt.Tx) error {
		for _, id := range []int{3, 4} {
			if getBuildStats(tx, id) != nil {
				t.Errorf("build %d got a record", id)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Again changes nothing
	if err := BackfillBuildStats(); err != nil {
		t.Fatalf("BackfillBuildStats again: %v", err)
	}
	if got := readStats(t, 5); got.Trigger.Kind != TriggerInterval {
		t.Errorf("existing record after a second backfill = %+v", got)
	}
}

// A build that was running when wakeci stopped is aborted in its record
// too, which keeps who started it
func TestInterruptedBuildStats(t *testing.T) {
	newTestBuildEnv(t)
	useQueue(t, &Queue{})
	started := time.Now().Add(-time.Hour)
	putTestHistory(t,
		&BuildUpdateData{ID: 1, Name: "deploy", Status: StatusRunning, StartedAt: started},
		&BuildUpdateData{ID: 2, Name: "deploy", Status: StatusRunning, StartedAt: started},
	)
	trigger := buildTrigger{Kind: TriggerSSO, User: "sam@example.com"}
	putTestStats(t, &buildStats{ID: 1, Job: "deploy", Status: StatusRunning, StartedAt: started, QueuedAt: started, Trigger: trigger})

	if err := MarkInterruptedBuilds(); err != nil {
		t.Fatalf("MarkInterruptedBuilds: %v", err)
	}
	if got := readStats(t, 1); got.Status != StatusAborted || !got.Ended || got.Trigger != trigger {
		t.Errorf("record = %+v, want aborted with its trigger", got)
	}
	// Running under an older version: made from the history
	// The history keeps the start to the microsecond
	if got := readStats(t, 2); got.Status != StatusAborted || got.StartedAt.Sub(started).Abs() > time.Millisecond {
		t.Errorf("record without one before = %+v, want aborted", got)
	}
}

func TestPruneBuildStats(t *testing.T) {
	newTestBuildEnv(t)
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	old := now.Add(-statsRetention - time.Hour)
	putTestStats(t,
		&buildStats{ID: 1, Job: "a", QueuedAt: old},
		&buildStats{ID: 2, Job: "a", StartedAt: old},
		// No time to tell its age
		&buildStats{ID: 3, Job: "a"},
		&buildStats{ID: 4, Job: "a", QueuedAt: old},
		&buildStats{ID: 5, Job: "a", QueuedAt: now.Add(-time.Hour)},
		&buildStats{ID: 6, Job: "a", QueuedAt: old},
	)
	if err := DB.Update(func(tx *bolt.Tx) error {
		return pruneBuildStats(tx, now, slog.New(slog.DiscardHandler))
	}); err != nil {
		t.Fatalf("pruneBuildStats: %v", err)
	}
	var kept []int
	if err := DB.View(func(tx *bolt.Tx) error {
		for id := 1; id <= 6; id++ {
			if getBuildStats(tx, id) != nil {
				kept = append(kept, id)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// It stops at the first new record, so an old one after it waits
	if want := []int{3, 5, 6}; !slices.Equal(kept, want) {
		t.Errorf("kept %v, want %v", kept, want)
	}
}

// The stats bucket is made for new databases and existing ones
func TestBootstrapCreatesStatsBucket(t *testing.T) {
	db, err := bolt.Open(filepath.Join(t.TempDir(), "boot.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	if err := db.Update(bootstrapDB); err != nil {
		t.Fatalf("bootstrapDB: %v", err)
	}
	if err := db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(BuildStatsBucket) == nil {
			t.Error("no stats bucket")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
