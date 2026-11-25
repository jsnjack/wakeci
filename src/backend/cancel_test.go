package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// cancelEnv is tokenEnv with jobs that share the cancel group tests/$BRANCH,
// spared on prod
func cancelEnv(t *testing.T) {
	t.Helper()
	tokenEnv(t)
	group := "params:\n  - BRANCH: \"\"\ncancel_group: tests/$BRANCH\ncancel_unless: '[[ $BRANCH == prod ]]'\ntasks: []\n"
	for name, content := range map[string]string{"tests": group, "postdep": group} {
		path := Config.JobDir + name + Config.jobsExt
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RegisterJob(path); err != nil {
			t.Fatal(err)
		}
	}
}

func runBranch(t *testing.T, job, branch string, more url.Values) int {
	t.Helper()
	form := url.Values{"BRANCH": {branch}}
	for k, v := range more {
		form[k] = v
	}
	id, _ := runAs(t, nil, job, form, http.StatusOK)
	return id
}

// ended returns the status of build id once it has left the queue
func ended(t *testing.T, id int) ItemStatus {
	t.Helper()
	waitLeftQueue(t, id)
	build, err := readBuildStatus(id)
	if err != nil {
		t.Fatal(err)
	}
	return build.Status
}

func TestCancelGroupCancelsOlderBuilds(t *testing.T) {
	cancelEnv(t)
	events, _ := captureAudit(t)
	old := runBranch(t, "tests", "main", nil)
	other := runBranch(t, "tests", "feature", nil)
	newer := runBranch(t, "postdep", "main", nil)

	if status := ended(t, old); status != StatusAborted {
		t.Errorf("older build of main: %s, want aborted", status)
	}
	if GlobalQueue.Live(other) == nil || GlobalQueue.Live(newer) == nil {
		t.Error("the build of another branch, or the newer build, left the queue")
	}
	var cancelled string
	for _, event := range events() {
		if event["event"] == "build.run" && event["build"] == strconv.Itoa(newer) {
			cancelled = event["cancelled"]
		}
	}
	if want := `["` + strconv.Itoa(old) + `"]`; cancelled != want {
		t.Errorf("audit cancelled = %q, want %q", cancelled, want)
	}
}

func TestCancelGroupSparesWhatItShould(t *testing.T) {
	cancelEnv(t)
	// cancel_unless is true for prod; an empty branch is no key
	for _, branch := range []string{"prod", ""} {
		first := runBranch(t, "tests", branch, nil)
		second := runBranch(t, "tests", branch, nil)
		if GlobalQueue.Live(first) == nil || GlobalQueue.Live(second) == nil {
			t.Errorf("branch %q: a build was cancelled", branch)
		}
	}
}

func TestCancelGroupTakesTheBuildsItStarted(t *testing.T) {
	cancelEnv(t)
	root := runBranch(t, "tests", "main", nil)
	child := runBranch(t, "deploy", "", url.Values{parentParam: {strconv.Itoa(root)}})
	// Same key in the same group: a build's children don't cancel it
	sameKey := runBranch(t, "postdep", "main", url.Values{parentParam: {strconv.Itoa(root)}})
	grandchild := runBranch(t, "nightly", "", url.Values{parentParam: {strconv.Itoa(sameKey)}})
	if GlobalQueue.Live(root) == nil {
		t.Fatal("a child with its root's key cancelled the root")
	}
	unrelated := runBranch(t, "deploy", "", nil)

	newer := GlobalQueue.Live(runBranch(t, "tests", "main", nil))
	for _, id := range []int{root, child, sameKey, grandchild} {
		if status := ended(t, id); status != StatusAborted {
			t.Errorf("build %d of the old group: %s, want aborted", id, status)
		}
	}
	if newer == nil || GlobalQueue.Live(unrelated) == nil {
		t.Fatal("the newer build, or a build of no group, left the queue")
	}
	// sameKey is an older build with the key and one root started: once
	cancelled := slices.Clone(newer.cancelled)
	slices.Sort(cancelled)
	if want := []int{root, child, sameKey, grandchild}; !slices.Equal(cancelled, want) {
		t.Errorf("cancelled %v, want %v, each once", newer.cancelled, want)
	}
}

func TestCancelGroupLaterBuildWins(t *testing.T) {
	cancelEnv(t)
	job, err := CreateJobFromFile(Config.JobDir + "tests" + Config.jobsExt)
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"BRANCH": {"main"}}
	older, err := createBuild(job, "", false, values, buildGroup{})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := createBuild(job, "", false, values, buildGroup{})
	if err != nil {
		t.Fatal(err)
	}
	// The newer build is queued first, as when its request was faster
	newer.setQueueKeys()
	GlobalQueue.Add(newer)
	older.setQueueKeys()
	GlobalQueue.Add(older)
	if older.supersededBy != newer.ID || len(newer.cancelled) != 0 {
		t.Errorf("older superseded by %d, newer cancelled %v; want older superseded by %d", older.supersededBy, newer.cancelled, newer.ID)
	}
	if status := ended(t, older.ID); status != StatusAborted {
		t.Errorf("older build: %s, want aborted", status)
	}
	if GlobalQueue.Live(newer.ID) == nil {
		t.Error("the newer build left the queue")
	}
}

func TestExpandKey(t *testing.T) {
	values := map[string]string{"BRANCH": "main", "NAME": "pr42", "EMPTY": ""}
	for _, c := range []struct {
		key, want string
		ok        bool
	}{
		{"tests/$BRANCH", "tests/main", true},
		{"pr/${NAME}-$BRANCH", "pr/pr42-main", true},
		{"tests/$EMPTY", "tests/", false},
		{"tests/$MISSING", "tests/", false},
		{"", "", false},
		{"fixed", "fixed", true},
	} {
		if got, ok := expandKey(c.key, values); got != c.want || ok != c.ok {
			t.Errorf("expandKey(%q) = %q, %v; want %q, %v", c.key, got, ok, c.want, c.ok)
		}
	}
}

func TestConcurrencyKeyCountsHooksOfAbortedBuilds(t *testing.T) {
	keyed := func(id int, key string, limit int) *Build {
		return &Build{ID: id, Job: &Job{Name: "deploy_pr" + strconv.Itoa(id), Concurrency: limit}, keys: queueKeys{concurrencyKey: key}}
	}
	old := keyed(1, "pr/42", 0)
	q := &Queue{running: []*Build{old}}
	if !q.concurrencyFull(keyed(2, "pr/42", 0)) {
		t.Error("a build of another job with the same key may start while one runs")
	}
	if q.concurrencyFull(keyed(3, "pr/43", 0)) {
		t.Error("a build with another key must not wait")
	}
	if q.concurrencyFull(keyed(4, "pr/42", 2)) {
		t.Error("with concurrency 2, a second build with the key may start")
	}
	// Aborted, it runs its hooks outside the running list
	q.running, q.aborting = nil, []*Build{old}
	if !q.concurrencyFull(keyed(2, "pr/42", 0)) {
		t.Error("a build may start while the one with its key runs its hooks")
	}
	q.aborting = nil
	if q.concurrencyFull(keyed(2, "pr/42", 0)) {
		t.Error("a build waits after the one with its key ended")
	}

	// Without a key, concurrency counts the job's own running builds
	plain := &Build{ID: 5, Job: &Job{Name: "nightly", Concurrency: 1}}
	q = &Queue{running: []*Build{{ID: 6, Job: &Job{Name: "nightly"}}}, aborting: []*Build{{ID: 7, Job: &Job{Name: "nightly"}}}}
	if !q.concurrencyFull(plain) {
		t.Error("job concurrency 1 lets a second build start")
	}
}

// A build that only runs its hooks after a restart cancels no other, so
// cancel_unless doesn't run for it; its concurrency key still holds
func TestHookKeysRunNoCancelUnless(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	build := &Build{ID: 3, Logger: L, Params: []map[string]string{{"BRANCH": "main"}}, Job: &Job{
		Name:           "tests",
		CancelGroup:    "tests/$BRANCH",
		CancelUnless:   "touch " + marker + "; false",
		ConcurrencyKey: "env/$BRANCH",
	}}
	build.setHookKeys()
	if build.keys.cancelKey != "" || build.keys.concurrencyKey != "env/main" {
		t.Errorf("keys %+v, want only the concurrency key env/main", build.keys)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("cancel_unless ran")
	}
	build.setQueueKeys()
	if build.keys.cancelKey != "tests/main" {
		t.Errorf("queued, cancel key %q, want tests/main", build.keys.cancelKey)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("queued, cancel_unless didn't run: %v", err)
	}
}

// Cleanup may remove a chain's root from the history while the chain runs;
// a superseded member still finds the builds it started
func TestStartedUnderWithoutTheRootsHistory(t *testing.T) {
	groupEnv(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	child, _ := runAs(t, nil, "nightly", url.Values{parentParam: {strconv.Itoa(root)}}, http.StatusOK)
	grandchild, _ := runAs(t, nil, "deploy", url.Values{parentParam: {strconv.Itoa(child)}}, http.StatusOK)
	other, _ := runAs(t, nil, "deploy", url.Values{parentParam: {strconv.Itoa(root)}}, http.StatusOK)
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(HistoryBucket).Delete(Itob(root))
	}); err != nil {
		t.Fatal(err)
	}
	if got := startedUnder(GlobalQueue.Live(child)); !slices.Equal(got, []int{grandchild}) {
		t.Errorf("started under %d: %v, want [%d], not %d", child, got, grandchild, other)
	}
}

// A build saved as pending is known to the queue until it is queued, while
// its cancel_unless runs, so the feed doesn't mark it aborted as stale
func TestBuildBeingAddedIsntStale(t *testing.T) {
	cancelEnv(t)
	job, err := CreateJobFromFile(Config.JobDir + "tests" + Config.jobsExt)
	if err != nil {
		t.Fatal(err)
	}
	build, err := createBuild(job, "", false, url.Values{"BRANCH": {"main"}}, buildGroup{})
	if err != nil {
		t.Fatal(err)
	}
	if isStaleBuild(&BuildUpdateData{ID: build.ID, Status: StatusPending}) {
		t.Error("a build being added counts as stale")
	}
	build.setQueueKeys()
	GlobalQueue.Add(build)
	if !GlobalQueue.Verify(build.ID) || GlobalQueue.adding[build.ID] {
		t.Error("the queued build isn't known as queued")
	}
}

// A build aborted while queued runs its hooks outside the queue; they hold
// its job's concurrency, with a key or without, but no concurrent build
func TestAbortedBuildsHooksHoldTheirJobsConcurrency(t *testing.T) {
	job := &Job{Name: "deploy", Concurrency: 1}
	cleaning := &Build{ID: 1, Job: job}
	next := &Build{ID: 2, Job: job}
	q := &Queue{aborting: []*Build{cleaning}}
	if !q.concurrencyFull(next) {
		t.Error("the next build may start while the aborted one cleans up")
	}
	q.aborting = nil
	if q.concurrencyFull(next) {
		t.Error("the next build waits with nothing else of its job")
	}
	other := &Build{ID: 3, Job: &Job{Name: "other", Concurrency: 1}}
	q.aborting = []*Build{cleaning}
	if q.concurrencyFull(other) {
		t.Error("another job waits for this one's cleanup")
	}
}

// A build that runs its hooks has done its work: a newer build of its key
// leaves it, so its cleanup isn't stopped
func TestCancelGroupSparesABuildInItsHooks(t *testing.T) {
	cancelEnv(t)
	old := runBranch(t, "tests", "main", nil)
	// As once its main tasks ended, while finally runs
	GlobalQueue.Live(old).mainEnded.Store(true)
	newer := runBranch(t, "postdep", "main", nil)
	if GlobalQueue.Live(old) == nil || GlobalQueue.Live(newer) == nil {
		t.Error("a build running its hooks was cancelled, or the newer one")
	}
	if cancelled := GlobalQueue.Live(newer).cancelled; len(cancelled) != 0 {
		t.Errorf("cancelled %v, want none", cancelled)
	}
}
