package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// wakeRun runs the wake command with env and stdin, and returns its exit
// status and output
func wakeRun(env map[string]string, stdin string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	status := runWake(args, func(name string) string { return env[name] }, strings.NewReader(stdin), &stdout, &stderr)
	return status, stdout.String(), stderr.String()
}

// wakeAgainstServer starts root, a build of deploy, and returns the
// environment its tasks get, pointed at an HTTP server with wakeci's API
func wakeAgainstServer(t *testing.T) (map[string]string, int) {
	t.Helper()
	router := tokenEnv(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	root, _ := runAs(t, nil, "deploy", url.Values{"TARGET": {"from-root"}}, http.StatusOK)
	return map[string]string{
		"WAKE_URL":        server.URL,
		"WAKE_TOKEN":      liveToken(t, root),
		"WAKE_JOB_PARAMS": "TARGET=from-root&UNKNOWN=x",
	}, root
}

var startedRE = regexp.MustCompile(`Started (\w+) #(\d+): (\S+)`)

// startedBuilds returns the builds wake's output says it started
func startedBuilds(t *testing.T, stdout string) []*Build {
	t.Helper()
	var builds []*Build
	for _, m := range startedRE.FindAllStringSubmatch(stdout, -1) {
		id, _ := strconv.Atoi(m[2])
		build := GlobalQueue.Live(id)
		if build == nil || build.Job.Name != m[1] {
			t.Fatalf("wake says it started %s #%d, which isn't queued", m[1], id)
		}
		if !strings.HasSuffix(m[3], "/build/"+m[2]) {
			t.Errorf("URL %s doesn't end with /build/%s", m[3], m[2])
		}
		builds = append(builds, build)
	}
	return builds
}

func target(b *Build) string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for _, param := range b.Params {
		if v, ok := param["TARGET"]; ok {
			return v
		}
	}
	return ""
}

func TestWakeStartsChildren(t *testing.T) {
	env, root := wakeAgainstServer(t)

	status, stdout, stderr := wakeRun(env, "", "start", "nightly", "test_api", "TARGET=prod")
	if status != 0 {
		t.Fatalf("wake start: %d %q %q", status, stdout, stderr)
	}
	builds := startedBuilds(t, stdout)
	if len(builds) != 2 {
		t.Fatalf("started %d builds, want 2: %q", len(builds), stdout)
	}
	for _, b := range builds {
		if target(b) != "prod" || b.Group().Parent != root {
			t.Errorf("%s #%d: TARGET %q, parent %d; want prod under %d", b.Job.Name, b.ID, target(b), b.Group().Parent, root)
		}
	}
}

func TestWakeInherits(t *testing.T) {
	env, _ := wakeAgainstServer(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--inherit"}, "from-root"},
		{[]string{"--inherit", "TARGET=other"}, "other"},
		// Empty: the job's default
		{[]string{"--inherit", "TARGET="}, "staging"},
		{nil, "staging"},
	} {
		status, stdout, stderr := wakeRun(env, "", append([]string{"start", "nightly"}, c.args...)...)
		builds := startedBuilds(t, stdout)
		if status != 0 || len(builds) != 1 {
			t.Fatalf("wake start nightly %v: %d %q %q", c.args, status, stdout, stderr)
		}
		if got := target(builds[0]); got != c.want {
			t.Errorf("wake start nightly %v: TARGET %q, want %q", c.args, got, c.want)
		}
	}
}

func TestWakeStartsOneBuildPerLine(t *testing.T) {
	env, root := wakeAgainstServer(t)
	status, stdout, stderr := wakeRun(env, "eu-1\n\n  us-2 \r\n", "start", "nightly", "--each", "TARGET")
	builds := startedBuilds(t, stdout)
	if status != 0 || len(builds) != 2 {
		t.Fatalf("wake start --each: %d %q %q", status, stdout, stderr)
	}
	if target(builds[0]) != "eu-1" || target(builds[1]) != "us-2" || builds[1].Group().Parent != root {
		t.Errorf("targets %q %q", target(builds[0]), target(builds[1]))
	}

	status, stdout, stderr = wakeRun(env, "", "start", "nightly", "--each", "TARGET")
	if status != 0 || stdout != "" || !strings.Contains(stderr, "nothing was started") {
		t.Errorf("wake start --each without lines: %d %q %q", status, stdout, stderr)
	}
}

func TestWakeChecksEverythingBeforeItStarts(t *testing.T) {
	env, _ := wakeAgainstServer(t)
	paramsFile := filepath.Join(t.TempDir(), "params")
	if err := os.WriteFile(paramsFile, []byte("# from the build\nNOPE=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		stdin string
		args  []string
		want  string
	}{
		{"", []string{"nightly", "no_such_job"}, "There is no job no_such_job."},
		{"", []string{"nightly", "NOPE=1"}, "nightly has no parameter NOPE."},
		{"", []string{"nightly", "--params-file", paramsFile}, "nightly has no parameter NOPE."},
		{"a\nb\n", []string{"nightly", "--each", "NOPE"}, "nightly has no parameter NOPE (--each)."},
		{"", []string{"nightly", `TARGET=a"b`}, "parameter TARGET has a quote, line break or NUL"},
		{"ok\nnot'ok\n", []string{"nightly", "--each", "TARGET"}, "nightly (TARGET=not'ok): parameter TARGET has a quote"},
		{"", []string{"nightly", "TARGET={{ secrets.TOKEN }}"}, "parameter TARGET references a secret"},
	} {
		before := len(GlobalQueue.queued)
		status, stdout, stderr := wakeRun(env, c.stdin, append([]string{"start"}, c.args...)...)
		if status != 1 || stdout != "" || !strings.Contains(stderr, c.want) || !strings.Contains(stderr, "Nothing was started.") {
			t.Errorf("wake start %v: %d %q %q, want 1 and %q", c.args, status, stdout, stderr, c.want)
		}
		if len(GlobalQueue.queued) != before {
			t.Errorf("wake start %v queued builds", c.args)
		}
	}
}

func TestWakeRunsABuildAgain(t *testing.T) {
	env, root := wakeAgainstServer(t)
	_, stdout, _ := wakeRun(env, "", "start", "nightly", "TARGET=prod")
	first := startedBuilds(t, stdout)[0]

	status, stdout, stderr := wakeRun(env, "", "start", "--again", strconv.Itoa(first.ID))
	builds := startedBuilds(t, stdout)
	if status != 0 || len(builds) != 1 {
		t.Fatalf("wake start --again: %d %q %q", status, stdout, stderr)
	}
	if b := builds[0]; b.Job.Name != "nightly" || target(b) != "prod" || b.Group().Parent != root {
		t.Errorf("run again: %s TARGET %q parent %d", b.Job.Name, target(b), b.Group().Parent)
	}
	status, stdout, _ = wakeRun(env, "", "start", "--again", strconv.Itoa(first.ID), "TARGET=dev")
	if builds := startedBuilds(t, stdout); status != 0 || target(builds[0]) != "dev" {
		t.Errorf("run again with TARGET=dev: %d %q", status, stdout)
	}
}

func TestWakeAPI(t *testing.T) {
	env, root := wakeAgainstServer(t)
	status, stdout, stderr := wakeRun(env, "", "api", "/api/whoami")
	var who WhoAmIData
	if status != 0 || json.Unmarshal([]byte(stdout), &who) != nil || who.Build != root {
		t.Errorf("wake api /api/whoami: %d %q %q", status, stdout, stderr)
	}
	status, _, stderr = wakeRun(env, "", "api", "api/settings")
	if status != 1 || !strings.Contains(stderr, "403") {
		t.Errorf("wake api api/settings: %d %q, want 1 and 403", status, stderr)
	}
}

func TestWakeUsage(t *testing.T) {
	env := map[string]string{"WAKE_URL": "http://localhost:1/", "WAKE_TOKEN": "wb_1_x"}
	for _, c := range []struct {
		env    map[string]string
		args   []string
		status int
		want   string
	}{
		{env, nil, 2, "wake start JOB"},
		{env, []string{"help"}, 0, "wake api PATH"},
		{env, []string{"stop"}, 2, `no command "stop"`},
		{map[string]string{"WAKE_TOKEN": "x"}, []string{"api", "/api/feed"}, 2, "set WAKE_URL"},
		{map[string]string{"WAKE_URL": "ftp://x/", "WAKE_TOKEN": "x"}, []string{"api", "/api/feed"}, 2, "set WAKE_URL"},
		{map[string]string{"WAKE_URL": "http://x/"}, []string{"api", "/api/feed"}, 2, "set WAKE_TOKEN"},
		{env, []string{"api"}, 2, "give one path"},
		{env, []string{"start"}, 2, "give a job"},
		{env, []string{"start", "a", "--again", "3"}, 2, "--again runs a build's job again"},
		{env, []string{"start", "a", "--each"}, 2, "--each needs a value"},
		{env, []string{"start", "a", "--each", "a-b"}, 2, "--each needs a parameter name"},
		{env, []string{"start", "a", "--again", "x"}, 2, "--again needs a build ID"},
		{env, []string{"start", "a", "--wait=yes"}, 2, "--wait takes no value"},
		{env, []string{"start", "a", "--force"}, 2, "no option --force"},
		{env, []string{"start", "a", "A-B=1"}, 2, "isn't NAME=VALUE"},
	} {
		status, stdout, stderr := wakeRun(c.env, "", c.args...)
		if status != c.status || !strings.Contains(stdout+stderr, c.want) {
			t.Errorf("wake %v: %d %q %q, want %d and %q", c.args, status, stdout, stderr, c.status, c.want)
		}
	}
}

func TestParseStartArgs(t *testing.T) {
	opts, err := parseStartArgs([]string{"--inherit", "a", "X=1", "--each=NODE", "b", "--params-file", "f", "--wait", "--", "--c", "Y=a=b"})
	if err != nil {
		t.Fatal(err)
	}
	want := &startOptions{
		jobs:       []string{"a", "b", "--c"},
		params:     []wakeParam{{"X", "1"}, {"Y", "a=b"}},
		inherit:    true,
		each:       "NODE",
		paramsFile: "f",
		wait:       true,
	}
	if fmt.Sprint(opts) != fmt.Sprint(want) {
		t.Errorf("parsed %+v, want %+v", opts, want)
	}
}

// fakeWake is a server with the routes wake uses, whose answers a test sets
type fakeWake struct {
	mu sync.Mutex
	// runs answers each run in turn: a status, or 0 to close the connection
	// once the request was read
	runs   []int
	nextID int
	// builds holds the statuses each GET of a build returns in turn, with
	// "" for one that hasn't ended
	builds map[int][]ItemStatus
	ran    []string
	sent   []url.Values
	// odd answers {} for a build and puts a null in the job list, as a
	// proxy's page or another server might
	odd bool
	// old answers a build without ended, as a wakeci before v3 does
	old bool
}

func (f *fakeWake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/api/jobs/":
		jobs := []*JobsListData{
			{Name: "deploy", Active: "true", DefaultParams: []map[string]string{{"NODE": ""}, {"MSG": "it's ok"}}},
		}
		if f.odd {
			jobs = append(jobs, nil)
		}
		_ = json.NewEncoder(w).Encode(jobs)
	case r.URL.Path == "/api/whoami":
		_ = json.NewEncoder(w).Encode(WhoAmIData{Auth: "build", Build: 1})
	case strings.HasSuffix(r.URL.Path, "/run"):
		_ = r.ParseForm()
		status := http.StatusOK
		if len(f.runs) > 0 {
			status, f.runs = f.runs[0], f.runs[1:]
		}
		switch status {
		case 0:
			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()
		case http.StatusOK:
			f.nextID++
			f.ran = append(f.ran, r.Form.Get("NODE"))
			f.sent = append(f.sent, r.Form)
			_, _ = fmt.Fprint(w, f.nextID)
		case http.StatusTooManyRequests:
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(status)
		case http.StatusBadRequest:
			http.Error(w, ErrQueueStopping.Error(), status)
		default:
			w.WriteHeader(status)
		}
	case strings.HasPrefix(r.URL.Path, "/api/build/") && f.odd:
		_, _ = io.WriteString(w, "{}")
	case strings.HasPrefix(r.URL.Path, "/api/build/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/build/"))
		statuses := f.builds[id]
		status := ItemStatus(StatusFinished)
		if len(statuses) > 0 {
			status, f.builds[id] = statuses[0], statuses[1:]
		}
		if f.old {
			_ = json.NewEncoder(w).Encode(map[string]any{"status_update": &BuildUpdateData{ID: id, Name: "deploy", Status: status}})
			return
		}
		_ = json.NewEncoder(w).Encode(GetBuildPayload{
			StatusUpdate: &BuildUpdateData{ID: id, Name: "deploy", Status: status, Params: []map[string]string{{"NODE": "web-1"}, {"MSG": "it's ok"}}},
			Ended:        status != "",
		})
	default:
		http.NotFound(w, r)
	}
}

// fakeWakeRun runs wake against f and returns its exit status, output and
// the waits between retries
func fakeWakeRun(t *testing.T, f *fakeWake, stdin string, args ...string) (int, string, string, []time.Duration) {
	t.Helper()
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	client, err := newWakeClient(func(name string) string {
		return map[string]string{"WAKE_URL": server.URL, "WAKE_TOKEN": "wb_1_x"}[name]
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var waits []time.Duration
	client.sleep = func(d time.Duration) { waits = append(waits, d) }
	opts, err := parseStartArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	status := client.start(opts, func(string) string { return "" }, strings.NewReader(stdin))
	return status, stdout.String(), stderr.String(), waits
}

func TestWakeRetriesWhileTheServerTookNothing(t *testing.T) {
	f := &fakeWake{runs: []int{http.StatusServiceUnavailable, http.StatusTooManyRequests, http.StatusBadRequest, http.StatusOK}}
	status, stdout, stderr, waits := fakeWakeRun(t, f, "", "deploy")
	if status != 0 || !strings.Contains(stdout, "Started deploy #1") {
		t.Fatalf("wake start: %d %q %q", status, stdout, stderr)
	}
	// Retry-After 7 is longer than the second wait, 4 s
	if want := []time.Duration{2 * time.Second, 7 * time.Second, 8 * time.Second}; !slices.Equal(waits, want) {
		t.Errorf("waits %v, want %v", waits, want)
	}

	f = &fakeWake{runs: []int{503, 503, 503, 503, 503, 503}}
	status, _, stderr, waits = fakeWakeRun(t, f, "", "deploy")
	if status != 1 || len(waits) != len(wakeRetryDelays) || !strings.Contains(stderr, "Could not start deploy: 503") {
		t.Errorf("wake start against a server that never takes it: %d %q, %d retries", status, stderr, len(waits))
	}
}

func TestWakeDoesntRunTwice(t *testing.T) {
	// 502 and 504 come from a proxy that may have passed the run on
	for _, run := range []int{0, http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout} {
		f := &fakeWake{runs: []int{run}}
		status, stdout, stderr, waits := fakeWakeRun(t, f, "a\nb\n", "deploy", "--each", "NODE")
		if status != 1 || stdout != "" || len(waits) != 0 || len(f.ran) != 0 {
			t.Errorf("answer %d: %d %q %q, %d retries, ran %v; want no retry", run, status, stdout, stderr, len(waits), f.ran)
		}
		if !strings.Contains(stderr, "Not started: deploy (NODE=b)") {
			t.Errorf("answer %d: stderr %q doesn't say what wasn't started", run, stderr)
		}
	}
	for _, run := range []int{0, http.StatusBadGateway, http.StatusGatewayTimeout} {
		f := &fakeWake{runs: []int{run}}
		if _, _, stderr, _ := fakeWakeRun(t, f, "", "deploy"); !strings.Contains(stderr, "the build may have started") {
			t.Errorf("answer %d: %q doesn't say the build may have started", run, stderr)
		}
	}
}

func TestWakeWaitsForEachBuild(t *testing.T) {
	f := &fakeWake{builds: map[int][]ItemStatus{1: {"", "", StatusFinished}, 2: {"", StatusFailed}}}
	status, stdout, stderr, _ := fakeWakeRun(t, f, "web-1\nweb-2\nweb-3\n", "deploy", "--each", "NODE", "--wait")
	if status != 1 {
		t.Errorf("exit status %d, want 1", status)
	}
	if !slices.Equal(f.ran, []string{"web-1", "web-2"}) {
		t.Errorf("ran %v, want web-1 and web-2 only", f.ran)
	}
	for _, want := range []string{"deploy #1: finished", "deploy #2: failed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q lacks %q", stdout, want)
		}
	}
	if !strings.Contains(stderr, "Not started: deploy (NODE=web-3)") {
		t.Errorf("stderr %q doesn't say web-3 wasn't started", stderr)
	}
}

func TestWakeStopsAtAnAnswerWithoutABuild(t *testing.T) {
	f := &fakeWake{odd: true}
	status, stdout, stderr, _ := fakeWakeRun(t, f, "web-1\nweb-2\n", "deploy", "--each", "NODE", "--wait")
	if status != 1 || !strings.Contains(stderr, "Could not follow deploy #1: build 1: the answer has no status_update") {
		t.Errorf("wake start --wait: %d %q %q", status, stdout, stderr)
	}
	if !slices.Equal(f.ran, []string{"web-1"}) {
		t.Errorf("ran %v, want web-1 only", f.ran)
	}

	f = &fakeWake{odd: true}
	status, stdout, stderr, _ = fakeWakeRun(t, f, "", "--again", "5")
	if status != 1 || len(f.ran) != 0 || !strings.Contains(stderr, "read build 5: the answer has no status_update") {
		t.Errorf("wake start --again: %d %q %q, ran %v", status, stdout, stderr, f.ran)
	}
}

// A wakeci before v3 sends no ended and doesn't wait: a final status ends
// the wait, and wake asks again only after a pause
func TestWakeWaitsOnAnOldServer(t *testing.T) {
	f := &fakeWake{old: true, builds: map[int][]ItemStatus{1: {StatusRunning, StatusRunning, StatusFailed}}}
	status, stdout, stderr, waits := fakeWakeRun(t, f, "", "deploy", "--wait")
	if status != 1 || !strings.Contains(stdout, "deploy #1: failed") {
		t.Errorf("wake start --wait: %d %q %q", status, stdout, stderr)
	}
	if want := []time.Duration{wakeOldServerPoll, wakeOldServerPoll}; !slices.Equal(waits, want) {
		t.Errorf("waits %v, want %v", waits, want)
	}
}

// A build token may not send a value with a quote, but a value at the
// job's default isn't sent at all, so running a build again works
func TestWakeLeavesOutDefaults(t *testing.T) {
	f := &fakeWake{}
	status, stdout, stderr, _ := fakeWakeRun(t, f, "", "--again", "5")
	if status != 0 || !slices.Equal(f.ran, []string{"web-1"}) {
		t.Fatalf("wake start --again: %d %q %q, ran %v", status, stdout, stderr, f.ran)
	}
	if f.sent[0].Has("MSG") {
		t.Errorf("sent MSG=%q, the job's default", f.sent[0].Get("MSG"))
	}
}

func TestWakeSendsTheTokenNowhereElse(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Host+r.URL.Path)
		_, _ = io.WriteString(w, "{}")
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	env := map[string]string{"WAKE_URL": server.URL + "/ci/", "WAKE_TOKEN": "wb_1_x"}
	for _, path := range []string{"//evil.example/x", "http://evil.example/x"} {
		wakeRun(env, "", "api", path)
	}
	for _, g := range got {
		if !strings.HasPrefix(g, host+"/ci/") {
			t.Errorf("wake sent a request to %s", g)
		}
	}
}

func TestInstallWakeCommand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	for _, binary := range []string{"/usr/local/bin/wakeci", "/opt/wakeci/wakeci"} {
		if err := installWakeCommand(dir, binary); err != nil {
			t.Fatalf("install: %v", err)
		}
		if target, err := os.Readlink(filepath.Join(dir, "wake")); err != nil || target != binary {
			t.Errorf("wake links to %q (%v), want %q", target, err, binary)
		}
	}
	if !isWakeCommand(filepath.Join(dir, "wake")) || isWakeCommand("/usr/local/bin/wakeci") {
		t.Error("isWakeCommand tells the names apart wrongly")
	}
}
