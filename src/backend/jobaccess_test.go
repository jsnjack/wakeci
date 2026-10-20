package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

func TestMatchJobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"deploy", "deploy", true},
		{"deploy", "deploy_prod", false},
		{"test_*", "test_unit_js", true},
		{"test_*", "test_", true},
		{"test_*", "unit_test", false},
		{"*_integration", "test_studio_integration", true},
		{"*_integration", "integration", false},
		{"vrec_*_test", "vrec_node_test", true},
		{"vrec_*_test", "vrec_test", false},
		{"*a*b*", "xaybz", true},
		{"*a*b*", "xbya", false},
		{"a*a", "a", false},
		{"a*a", "aa", true},
		{"*", "anything", true},
		// Only * is special
		{"test_?", "test_1", false},
		{"test_[ab]", "test_[ab]", true},
		{"test_[ab]", "test_a", false},
	}
	for _, tt := range tests {
		if got := matchJobPattern(tt.pattern, tt.name); got != tt.want {
			t.Errorf("matchJobPattern(%q, %q) = %t, want %t", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestParseJobPatterns(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []string
		wantErr bool
	}{
		{name: "none", values: nil, want: nil},
		{name: "names and patterns, deduplicated", values: []string{"test_*", "deploy", "test_*"}, want: []string{"test_*", "deploy"}},
		{name: "empty", values: []string{""}, wantErr: true},
		{name: "slash", values: []string{"../secret"}, wantErr: true},
		{name: "too long", values: []string{strings.Repeat("a", maxJobPatternLength+1)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJobPatterns(tt.values)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseJobPatterns(%q) error = %v, wantErr %t", tt.values, err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseJobPatterns(%q) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

// testBuilds are the builds newJobAccessTestEnv creates, by ID
var testBuilds = map[int]string{1: "test_unit", 2: "deploy", 3: "test_e2e"}

// newJobAccessTestEnv sets up the jobs of testBuilds, each with a job file,
// and their builds, each with a task log
func newJobAccessTestEnv(t *testing.T) {
	t.Helper()
	newAuthTestEnv(t)
	Config.WorkDir = t.TempDir() + "/"
	Config.JobDir = t.TempDir() + "/"
	Config.jobsExt = ".yaml"
	originalCron := GlobalCron
	GlobalCron = cron.New()
	t.Cleanup(func() { GlobalCron = originalCron })

	err := DB.Update(func(tx *bolt.Tx) error {
		history, err := tx.CreateBucketIfNotExists(HistoryBucket)
		if err != nil {
			return err
		}
		jobs, err := tx.CreateBucketIfNotExists(JobsBucket)
		if err != nil {
			return err
		}
		for id, name := range testBuilds {
			data, err := json.Marshal(BuildUpdateData{ID: id, Name: name, Status: StatusFinished})
			if err != nil {
				return err
			}
			if err := history.Put(Itob(id), data); err != nil {
				return err
			}
			job, err := jobs.CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return err
			}
			if err := job.Put([]byte("defaultParams"), []byte("[]")); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("store jobs and builds: %v", err)
	}
	for id, name := range testBuilds {
		dir := Config.WorkDir + "wakespace/" + strconv.Itoa(id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/task_0.log", []byte(name+" log"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(Config.JobDir+name+Config.jobsExt, []byte("desc: "+name+"\ntasks: []\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// mustCreateJobKey creates a key limited to jobs, with every scope such a
// key can have: read and run. Without jobs, it gets write too.
func mustCreateJobKey(t *testing.T, jobs []string) string {
	t.Helper()
	scopes := []Scope{ScopeRead, ScopeRun}
	if len(jobs) == 0 {
		scopes = APIKeyScopes
	}
	_, token, err := CreateAPIKey(passwordPrincipal(viaSession), "jobs key", scopes, jobs, 0, time.Now(), nil)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	return token
}

func serveWithKey(token, method, target string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	newRouter().ServeHTTP(rec, req)
	return rec
}

// TestJobLimitedKeyRoutes checks that every route about a job or a build
// turns away a key limited to other jobs. Builds of other jobs look like
// missing builds, so guessing build IDs doesn't reveal those jobs.
func TestJobLimitedKeyRoutes(t *testing.T) {
	routes := []struct {
		method string
		// path is about job deploy or its build 2, which the key can't access
		path string
		form url.Values
	}{
		{http.MethodPost, "/api/job/deploy/run", nil},
		{http.MethodGet, "/api/job/deploy", nil},
		{http.MethodGet, "/api/build/2", nil},
		{http.MethodPost, "/api/build/2/abort", nil},
		{http.MethodPost, "/api/build/2/flush", nil},
		{http.MethodPost, "/api/build/2/start", nil},
		{http.MethodGet, "/storage/build/2/task_0.log", nil},
		{http.MethodHead, "/storage/build/2/task_0.log", nil},
		// The file server would resolve both of these to build 2's log
		{http.MethodGet, "/storage/build/1/../2/task_0.log", nil},
		{http.MethodGet, "/storage/build/1%2F..%2F2/task_0.log", nil},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			newJobAccessTestEnv(t)
			rec := serveWithKey(mustCreateJobKey(t, []string{"test_*"}), route.method, route.path, route.form)
			if strings.HasPrefix(route.path, "/api/job/") {
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "no access to job") {
					t.Errorf("status = %d body = %q, want 403 for job deploy", rec.Code, rec.Body.String())
				}
				return
			}
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "deploy") {
				t.Errorf("status = %d body = %q, want a 404 that doesn't name the job", rec.Code, rec.Body.String())
			}
		})
	}

	t.Run("allowed jobs", func(t *testing.T) {
		newJobAccessTestEnv(t)
		token := mustCreateJobKey(t, []string{"test_*"})
		if rec := serveWithKey(token, http.MethodGet, "/api/job/test_unit", nil); rec.Code != http.StatusOK {
			t.Errorf("job of the key: status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		if rec := serveWithKey(token, http.MethodGet, "/storage/build/1/task_0.log", nil); rec.Code != http.StatusOK || rec.Body.String() != "test_unit log" {
			t.Errorf("log of the key's build: status = %d body = %q, want 200", rec.Code, rec.Body.String())
		}
		// Getting this far means access was granted; there's no build plan
		if rec := serveWithKey(token, http.MethodGet, "/api/build/1", nil); rec.Code == http.StatusForbidden || rec.Code == http.StatusNotFound {
			t.Errorf("build of the key's job: status = %d (%s)", rec.Code, rec.Body.String())
		}
		// A pattern also covers jobs created later
		newJob := Config.JobDir + "test_new" + Config.jobsExt
		if err := os.WriteFile(newJob, []byte("desc: new\ntasks: []\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := RegisterJob(newJob); err != nil {
			t.Fatal(err)
		}
		if rec := serveWithKey(token, http.MethodGet, "/api/job/test_new", nil); rec.Code != http.StatusOK {
			t.Errorf("job added later that matches the pattern: status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		if rec := serveWithKey(token, http.MethodGet, "/api/build/99", nil); rec.Code != http.StatusNotFound {
			t.Errorf("unknown build: status = %d, want 404", rec.Code)
		}
		if rec := serveWithKey(token, http.MethodGet, "/storage/build/", nil); rec.Code != http.StatusNotFound {
			t.Errorf("listing every build's files: status = %d, want 404", rec.Code)
		}
	})

	t.Run("key for every job", func(t *testing.T) {
		newJobAccessTestEnv(t)
		if rec := serveWithKey(mustCreateJobKey(t, nil), http.MethodGet, "/api/job/deploy", nil); rec.Code != http.StatusOK {
			t.Errorf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
	})
}

// TestJobLimitedKeyListings: listings leave out the jobs a key can't access
func TestJobLimitedKeyListings(t *testing.T) {
	feedIDs := func(t *testing.T, token, target string) []int {
		t.Helper()
		rec := serveWithKey(token, http.MethodGet, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d (%s)", target, rec.Code, rec.Body.String())
		}
		var builds []BuildUpdateData
		if err := json.Unmarshal(rec.Body.Bytes(), &builds); err != nil {
			t.Fatalf("decode feed: %v", err)
		}
		var ids []int
		for _, b := range builds {
			ids = append(ids, b.ID)
		}
		return ids
	}
	jobNames := func(t *testing.T, token string) []string {
		t.Helper()
		rec := serveWithKey(token, http.MethodGet, "/api/jobs/", nil)
		var jobs []JobsListData
		if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
			t.Fatalf("decode jobs (status %d): %v", rec.Code, err)
		}
		var names []string
		for _, j := range jobs {
			names = append(names, j.Name)
		}
		return names
	}

	newJobAccessTestEnv(t)
	limited := mustCreateJobKey(t, []string{"test_*"})
	all := mustCreateJobKey(t, nil)

	tests := []struct {
		name  string
		token string
		path  string
		want  []int
	}{
		{name: "every job", token: all, path: "/api/feed", want: []int{3, 2, 1}},
		{name: "limited", token: limited, path: "/api/feed", want: []int{3, 1}},
		{name: "limited, offset counts only visible builds", token: limited, path: "/api/feed?offset=1", want: []int{1}},
		{name: "limited and filtered", token: limited, path: "/api/feed?filter=unit", want: []int{1}},
		{name: "limited, filter for a hidden job", token: limited, path: "/api/feed?filter=deploy", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := feedIDs(t, tt.token, tt.path); !slices.Equal(got, tt.want) {
				t.Errorf("%s: builds %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	if got, want := jobNames(t, limited), []string{"test_e2e", "test_unit"}; !slices.Equal(got, want) {
		t.Errorf("jobs list = %q, want %q", got, want)
	}
	if got, want := jobNames(t, all), []string{"deploy", "test_e2e", "test_unit"}; !slices.Equal(got, want) {
		t.Errorf("jobs list = %q, want %q", got, want)
	}
}

// TestJobLimitedKeyWebSocket: live updates and logs of other jobs never
// reach a websocket opened with a key limited to some jobs
func TestJobLimitedKeyWebSocket(t *testing.T) {
	tests := []struct {
		name       string
		jobs       []string
		wantDeploy bool
	}{
		{name: "limited", jobs: []string{"test_*"}},
		{name: "every job", wantDeploy: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newJobAccessTestEnv(t)
			originalHub := WSHub
			WSHub = newHub()
			go WSHub.run()
			t.Cleanup(func() { WSHub = originalHub })
			server := httptest.NewServer(newRouter())
			t.Cleanup(server.Close)

			conn := dialWS(t, server, http.Header{"Authorization": {"Bearer " + mustCreateJobKey(t, tt.jobs)}})
			if err := conn.WriteJSON(map[string]any{"type": MsgTypeInSubscribe, "data": map[string]any{"to": []string{"build:update:", "build:log:"}}}); err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			types := make(chan string, 100)
			go func() {
				for {
					_, frame, err := conn.ReadMessage()
					if err != nil {
						return
					}
					// The hub batches queued messages into one frame
					for _, line := range bytes.Split(frame, newline) {
						var msg MsgBroadcast
						if json.Unmarshal(line, &msg) == nil {
							types <- msg.Type
						}
					}
				}
			}()
			broadcast := func() {
				WSHub.broadcast <- &MsgBroadcast{Type: "build:update:2", job: "deploy"}
				WSHub.broadcast <- &MsgBroadcast{Type: "build:log:2", job: "deploy"}
				WSHub.broadcast <- &MsgBroadcast{Type: "build:update:1", job: "test_unit"}
			}

			// The subscription is handled asynchronously: wait until it is
			subscribed := false
			for deadline := time.Now().Add(2 * time.Second); !subscribed && time.Now().Before(deadline); {
				broadcast()
				select {
				case <-types:
					subscribed = true
				case <-time.After(20 * time.Millisecond):
				}
			}
			if !subscribed {
				t.Fatal("no updates arrived at all")
			}
			broadcast()
			var got []string
			for timeout := time.After(300 * time.Millisecond); ; {
				select {
				case msgType := <-types:
					got = append(got, msgType)
					continue
				case <-timeout:
				}
				break
			}
			if !slices.Contains(got, "build:update:1") {
				t.Errorf("received %q, want build 1's update", got)
			}
			if gotDeploy := slices.Contains(got, "build:update:2") || slices.Contains(got, "build:log:2"); gotDeploy != tt.wantDeploy {
				t.Errorf("received %q; want deploy's messages: %t", got, tt.wantDeploy)
			}
		})
	}
}
