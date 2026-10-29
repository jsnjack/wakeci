package main

import (
	"context"
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

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

// newAPIKeyTestEnv registers the jobs deploy, nightly and test_api, adds
// build 1 of deploy and build 2 of nightly, each with a task log, and returns
// the API key router
func newAPIKeyTestEnv(t *testing.T) http.Handler {
	t.Helper()
	newTestBuildEnv(t)
	if err := DB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(APIKeysBucket)
		return err
	}); err != nil {
		t.Fatalf("create api keys bucket: %v", err)
	}
	oldCron := GlobalCron
	GlobalCron = cron.New()
	oldLimiter := apiKeyAttemptLimiter
	apiKeyAttemptLimiter = newAuthAttemptLimiter(authMaxFailures, authFailureWindow, authBlockDuration)
	t.Cleanup(func() {
		GlobalCron = oldCron
		apiKeyAttemptLimiter = oldLimiter
	})

	for _, name := range []string{"deploy", "nightly", "test_api"} {
		path := Config.JobDir + name + Config.jobsExt
		if err := os.WriteFile(path, []byte("desc: "+name+"\nparams:\n  - TARGET: staging\ntasks: []\n"), 0644); err != nil {
			t.Fatalf("write job: %v", err)
		}
		if err := RegisterJob(path); err != nil {
			t.Fatalf("register job: %v", err)
		}
	}
	addTestBuild(t, 1, "deploy")
	addTestBuild(t, 2, "nightly")

	storage := HandleWakespaceResource(http.FileServer(http.Dir(Config.WorkDir + "wakespace")))
	return newAPIKeyRouter(storage)
}

func addTestBuild(t *testing.T, id int, job string) {
	t.Helper()
	data, err := json.Marshal(BuildUpdateData{ID: id, Name: job, Status: StatusFinished})
	if err != nil {
		t.Fatalf("marshal build: %v", err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(HistoryBucket).Put(Itob(id), data)
	}); err != nil {
		t.Fatalf("save build: %v", err)
	}
	dir := Config.WorkDir + "wakespace/" + strconv.Itoa(id) + "/"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create wakespace: %v", err)
	}
	if err := os.WriteFile(dir+"build_plan.yaml", []byte("name: "+job+"\ntasks: []\n"), 0644); err != nil {
		t.Fatalf("write build plan: %v", err)
	}
	if err := os.WriteFile(dir+"task_1.log", []byte("log of "+job), 0644); err != nil {
		t.Fatalf("write task log: %v", err)
	}
}

func createTestKey(t *testing.T, form url.Values) string {
	t.Helper()
	if form.Get("expiresInDays") == "" {
		form.Set("expiresInDays", "30")
	}
	key, err := apiKeyFromForm(form, time.Now())
	if err != nil {
		t.Fatalf("apiKeyFromForm: %v", err)
	}
	token, err := saveNewAPIKey(key)
	if err != nil {
		t.Fatalf("saveNewAPIKey: %v", err)
	}
	return token
}

func keyRequest(handler http.Handler, method, target, token string, form url.Values) *httptest.ResponseRecorder {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func jobNames(t *testing.T, body []byte) []string {
	t.Helper()
	var jobs []JobsListData
	if err := json.Unmarshal(body, &jobs); err != nil {
		t.Fatalf("decode jobs %s: %v", body, err)
	}
	var names []string
	for _, job := range jobs {
		names = append(names, job.Name)
	}
	return names
}

func buildIDs(t *testing.T, body []byte) []int {
	t.Helper()
	var builds []BuildUpdateData
	if err := json.Unmarshal(body, &builds); err != nil {
		t.Fatalf("decode feed %s: %v", body, err)
	}
	var ids []int
	for _, build := range builds {
		ids = append(ids, build.ID)
	}
	return ids
}

func TestAPIKeyRoutes(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"deploy"}, "scope": {"read", "run"}, "job": {"deploy"}})

	tests := []struct {
		method, target string
		form           url.Values
		wantStatus     int
		wantBody       string
	}{
		{"GET", "/api/job/deploy", nil, http.StatusOK, "desc: deploy"},
		{"GET", "/api/job/nightly", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/api/build/1", nil, http.StatusOK, `"name":"deploy"`},
		{"GET", "/api/build/2", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/api/build/99", nil, http.StatusNotFound, "build 99 not found"},
		{"GET", "/api/build/x", nil, http.StatusBadRequest, "invalid build ID"},
		{"GET", "/storage/build/1/task_1.log", nil, http.StatusOK, "log of deploy"},
		{"HEAD", "/storage/build/1/task_1.log", nil, http.StatusOK, ""},
		{"GET", "/storage/build/2/task_1.log", nil, http.StatusForbidden, "doesn't cover that job"},
		{"HEAD", "/storage/build/2/task_1.log", nil, http.StatusForbidden, ""},
		{"GET", "/storage/build/1/../2/task_1.log", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/storage/build/1/%2e%2e/2/task_1.log", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/storage/build/1/%2E%2E%2F2/task_1.log", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/storage/build/", nil, http.StatusForbidden, "only read the files of a build"},
		{"GET", "/storage/build/..", nil, http.StatusForbidden, "only read the files of a build"},
		{"GET", "/storage/build/01/task_1.log", nil, http.StatusForbidden, "only read the files of a build"},
		{"POST", "/api/job/nightly/run", nil, http.StatusForbidden, "doesn't cover that job"},
		{"POST", "/api/job/deploy/run", url.Values{"TARGET": {"{{ secrets.TOKEN }}"}}, http.StatusBadRequest, "references a secret"},
		{"POST", "/api/job/deploy/run?TARGET=x%7B%7Bsecrets.TOKEN%7D%7D", nil, http.StatusBadRequest, "references a secret"},
		{"POST", "/api/job/deploy/run", url.Values{"TARGET": {"x\nBASH_ENV=/tmp/x"}}, http.StatusBadRequest, "line break"},
		{"POST", "/api/job/deploy/run", url.Values{"TARGET": {"x\ry"}}, http.StatusBadRequest, "line break"},
		{"POST", "/api/job/deploy/run", url.Values{"TARGET": {"x\x00y"}}, http.StatusBadRequest, "line break"},
		{"POST", "/api/build/2/abort", nil, http.StatusForbidden, "doesn't cover that job"},
		{"POST", "/api/build/2/start", nil, http.StatusForbidden, "doesn't cover that job"},
		{"POST", "/api/build/2/flush", nil, http.StatusForbidden, "doesn't cover that job"},
		{"POST", "/api/jobs/create", url.Values{"name": {"new"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"POST", "/api/job/deploy", url.Values{"fileContent": {"tasks: []"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"DELETE", "/api/job/deploy", nil, http.StatusForbidden, "doesn't have the write scope"},
		{"POST", "/api/job/deploy/set_active", url.Values{"active": {"false"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"GET", "/api/settings", nil, http.StatusForbidden, "can't be used here"},
		{"POST", "/api/settings", url.Values{"password": {"x"}}, http.StatusForbidden, "can't be used here"},
		{"GET", "/api/keys/", nil, http.StatusForbidden, "can't be used here"},
		{"POST", "/api/keys/", nil, http.StatusForbidden, "can't be used here"},
		{"GET", "/ws", nil, http.StatusForbidden, "can't be used here"},
		{"GET", "/auth/_isLoggedIn", nil, http.StatusForbidden, "can't be used here"},
		{"GET", "/", nil, http.StatusForbidden, "can't be used here"},
		{"DELETE", "/api/feed", nil, http.StatusForbidden, "can't be used here"},
		{"GET", "/storage/other", nil, http.StatusForbidden, "can't be used here"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := keyRequest(router, tt.method, tt.target, token, tt.form)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAPIKeyListsOnlyCoveredJobs(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	tests := []struct {
		name       string
		form       url.Values
		wantJobs   []string
		wantBuilds []int
	}{
		{"exact name", url.Values{"job": {"deploy"}}, []string{"deploy"}, []int{1}},
		{"pattern", url.Values{"pattern": {"test_*"}}, []string{"test_api"}, nil},
		{"name and pattern", url.Values{"job": {"nightly"}, "pattern": {"dep*"}}, []string{"deploy", "nightly"}, []int{2, 1}},
		{"all jobs", url.Values{"allJobs": {"true"}}, []string{"deploy", "nightly", "test_api"}, []int{2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.form.Set("name", tt.name)
			tt.form.Set("scope", ScopeRead)
			token := createTestKey(t, tt.form)

			rec := keyRequest(router, "GET", "/api/jobs/", token, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("jobs status = %d; body %q", rec.Code, rec.Body.String())
			}
			if got := jobNames(t, rec.Body.Bytes()); !slices.Equal(got, tt.wantJobs) {
				t.Errorf("jobs = %q, want %q", got, tt.wantJobs)
			}

			rec = keyRequest(router, "GET", "/api/feed", token, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("feed status = %d; body %q", rec.Code, rec.Body.String())
			}
			if got := buildIDs(t, rec.Body.Bytes()); !slices.Equal(got, tt.wantBuilds) {
				t.Errorf("builds = %v, want %v", got, tt.wantBuilds)
			}
		})
	}
}

// The feed pages over the builds the key covers, so a page isn't cut short
// by builds of other jobs
func TestAPIKeyFeedPages(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	for id := 3; id <= 40; id++ {
		job := "nightly"
		if id%2 == 1 {
			job = "deploy"
		}
		addTestBuild(t, id, job)
	}
	token := createTestKey(t, url.Values{"name": {"deploy"}, "scope": {"read"}, "job": {"deploy"}})

	tests := []struct {
		target string
		want   []int
	}{
		{"/api/feed", []int{39, 37, 35, 33, 31, 29, 27, 25, 23, 21, 19, 17, 15, 13, 11}},
		{"/api/feed?offset=15", []int{9, 7, 5, 3, 1}},
		{"/api/feed?offset=1&filter=id:1", []int{17, 15, 13, 11, 1}},
	}
	for _, tt := range tests {
		rec := keyRequest(router, "GET", tt.target, token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body %q", tt.target, rec.Code, rec.Body.String())
		}
		if got := buildIDs(t, rec.Body.Bytes()); !slices.Equal(got, tt.want) {
			t.Errorf("%s builds = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestAPIKeyRunReachesHandler(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"run"}, "scope": {"run"}, "pattern": {"dep*"}})
	// A disabled job stops RunJob before it queues a build, with an error
	// that only the handler gives
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(JobsBucket).Bucket([]byte("deploy")).Put([]byte("active"), []byte("false"))
	}); err != nil {
		t.Fatalf("disable job: %v", err)
	}
	rec := keyRequest(router, "POST", "/api/job/deploy/run", token, url.Values{"TARGET": {"production"}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "job deploy is not enabled") {
		t.Errorf("status = %d, body %q; want the handler's not-enabled error", rec.Code, rec.Body.String())
	}
	// Run alone doesn't include read
	rec = keyRequest(router, "GET", "/api/build/1", token, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("read with a run key: status = %d, want 403", rec.Code)
	}
}

// A build a key starts is marked, so its build.env values skip injectSecrets
func TestAPIKeyRunMarksBuild(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	// No concurrent builds, so the builds stay queued where the test sees them
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	token := createTestKey(t, url.Values{"name": {"run"}, "scope": {"run"}, "job": {"deploy"}})

	if rec := keyRequest(router, "POST", "/api/job/deploy/run", token, url.Values{"TARGET": {"production"}}); rec.Code != http.StatusOK {
		t.Fatalf("run with a key: status = %d; body %q", rec.Code, rec.Body.String())
	}
	main := chi.NewRouter()
	main.Post("/api/job/{name}/run", HandleRunJob)
	rec := httptest.NewRecorder()
	main.ServeHTTP(rec, httptest.NewRequest("POST", "/api/job/deploy/run", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("run without a key: status = %d; body %q", rec.Code, rec.Body.String())
	}

	if len(GlobalQueue.queued) != 2 {
		t.Fatalf("queued %d builds, want 2", len(GlobalQueue.queued))
	}
	if !GlobalQueue.queued[0].startedWithAPIKey || GlobalQueue.queued[1].startedWithAPIKey {
		t.Errorf("startedWithAPIKey = %t, %t; want true for the key's build only",
			GlobalQueue.queued[0].startedWithAPIKey, GlobalQueue.queued[1].startedWithAPIKey)
	}
	if got := GlobalQueue.queued[0].Params[0]["TARGET"]; got != "production" {
		t.Errorf("TARGET = %q, want production", got)
	}
}

func TestAPIKeyWriteScope(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"admin"}, "scope": {"write"}, "allJobs": {"true"}})

	rec := keyRequest(router, "POST", "/api/jobs/create", token, url.Values{"name": {"created"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("create job: status = %d; body %q", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(Config.JobDir + "created" + Config.jobsExt); err != nil {
		t.Errorf("job file not created: %v", err)
	}
	// Write never opens settings or key management
	for _, target := range []string{"/api/settings", "/api/keys/"} {
		if rec := keyRequest(router, "GET", target, token, nil); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s: status = %d, want 403", target, rec.Code)
		}
	}
	// Write alone doesn't include read
	if rec := keyRequest(router, "GET", "/api/job/deploy", token, nil); rec.Code != http.StatusForbidden {
		t.Errorf("read with a write key: status = %d, want 403", rec.Code)
	}
}

// Each route needs its own scope, whatever jobs the key covers
func TestAPIKeyScopePerRoute(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	tests := []struct {
		scope   string
		denied  []string
		allowed []string
	}{
		{
			scope: ScopeRead,
			denied: []string{
				"POST /api/job/deploy/run", "POST /api/build/1/abort", "POST /api/build/1/start",
				"POST /api/build/1/flush", "POST /api/jobs/create", "POST /api/job/deploy",
				"DELETE /api/job/deploy", "POST /api/job/deploy/set_active",
			},
			allowed: []string{
				"GET /api/feed", "GET /api/jobs/", "GET /api/job/deploy", "GET /api/build/1",
				"GET /storage/build/1/task_1.log", "HEAD /storage/build/1/task_1.log",
			},
		},
		{
			scope: ScopeRun,
			denied: []string{
				"GET /api/feed", "GET /api/jobs/", "GET /api/job/deploy", "GET /api/build/1",
				"GET /storage/build/1/task_1.log", "HEAD /storage/build/1/task_1.log",
				"POST /api/jobs/create", "POST /api/job/deploy", "DELETE /api/job/deploy",
				"POST /api/job/deploy/set_active",
			},
		},
		{
			scope: ScopeWrite,
			denied: []string{
				"GET /api/feed", "GET /api/jobs/", "GET /api/job/deploy", "GET /api/build/1",
				"GET /storage/build/1/task_1.log", "POST /api/job/deploy/run",
				"POST /api/build/1/abort", "POST /api/build/1/start", "POST /api/build/1/flush",
			},
		},
	}
	for _, tt := range tests {
		token := createTestKey(t, url.Values{"name": {tt.scope}, "scope": {tt.scope}, "allJobs": {"true"}})
		for _, route := range tt.denied {
			method, target, _ := strings.Cut(route, " ")
			rec := keyRequest(router, method, target, token, url.Values{})
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "scope") {
				t.Errorf("%s key, %s: status = %d, body %q; want 403 for the scope", tt.scope, route, rec.Code, rec.Body.String())
			}
		}
		for _, route := range tt.allowed {
			method, target, _ := strings.Cut(route, " ")
			if rec := keyRequest(router, method, target, token, nil); rec.Code != http.StatusOK {
				t.Errorf("%s key, %s: status = %d, want 200", tt.scope, route, rec.Code)
			}
		}
	}
}

func TestAPIKeyRejected(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"read"}, "scope": {"read"}, "allJobs": {"true"}})
	id, secret, ok := parseAPIKeyToken(token)
	if !ok {
		t.Fatalf("parseAPIKeyToken(%q) failed", token)
	}
	otherSecret := strings.Repeat("A", len(secret))

	expired := createTestKey(t, url.Values{"name": {"expired"}, "scope": {"read"}, "allJobs": {"true"}})
	expiredID, _, _ := parseAPIKeyToken(expired)
	if err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(APIKeysBucket)
		var stored storedAPIKey
		if err := json.Unmarshal(b.Get([]byte(expiredID)), &stored); err != nil {
			return err
		}
		past := time.Now().Add(-time.Second)
		stored.ExpiresAt = &past
		data, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		return b.Put([]byte(expiredID), data)
	}); err != nil {
		t.Fatalf("expire key: %v", err)
	}

	revoked := createTestKey(t, url.Values{"name": {"revoked"}, "scope": {"read"}, "allJobs": {"true"}})
	if rec := keyRequest(router, "GET", "/api/jobs/", revoked, nil); rec.Code != http.StatusOK {
		t.Fatalf("before revoking: status = %d", rec.Code)
	}
	revokedID, _, _ := parseAPIKeyToken(revoked)
	revoke := httptest.NewRequest("DELETE", "/api/keys/"+revokedID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", revokedID)
	revoke = revoke.WithContext(contextWithRouteContext(revoke, rctx))
	revokeRec := httptest.NewRecorder()
	HandleAPIKeyRevoke(revokeRec, revoke)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d; body %q", revokeRec.Code, revokeRec.Body.String())
	}

	for name, bad := range map[string]string{
		"malformed":    "wk_abc",
		"wrong secret": apiKeyPrefix + id + "_" + otherSecret,
		"unknown key":  apiKeyPrefix + "000000000000_" + secret,
		"expired":      expired,
		"revoked":      revoked,
	} {
		t.Run(name, func(t *testing.T) {
			rec := keyRequest(router, "GET", "/api/jobs/", bad, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body %q", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
				t.Errorf("WWW-Authenticate = %q", got)
			}
		})
	}
	if rec := keyRequest(router, "GET", "/api/jobs/", token, nil); rec.Code != http.StatusOK {
		t.Errorf("valid key: status = %d, want 200", rec.Code)
	}
}

// API key failures have their own counter: they never block password logins,
// and a valid key never clears failed password attempts
func TestAPIKeyLimiter(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"read"}, "scope": {"read"}, "allJobs": {"true"}})
	oldPasswordLimiter := globalAuthAttemptLimiter
	globalAuthAttemptLimiter = newAuthAttemptLimiter(2, time.Minute, time.Minute)
	apiKeyAttemptLimiter = newAuthAttemptLimiter(2, time.Minute, time.Minute)
	t.Cleanup(func() { globalAuthAttemptLimiter = oldPasswordLimiter })

	client := "192.0.2.1" // httptest's RemoteAddr
	globalAuthAttemptLimiter.failure(client, time.Now())
	if rec := keyRequest(router, "GET", "/api/jobs/", token, nil); rec.Code != http.StatusOK {
		t.Fatalf("valid key: status = %d", rec.Code)
	}
	if got := globalAuthAttemptLimiter.attempts[client].failures; got != 1 {
		t.Errorf("password failures after a valid key = %d, want 1", got)
	}

	if rec := keyRequest(router, "GET", "/api/jobs/", "wk_bad", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("first bad key: status = %d, want 401", rec.Code)
	}
	if rec := keyRequest(router, "GET", "/api/jobs/", "wk_bad", nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("second bad key: status = %d, want 429", rec.Code)
	}
	// A key can't be guessed, so bad keys never lock out a valid one
	if rec := keyRequest(router, "GET", "/api/jobs/", token, nil); rec.Code != http.StatusOK {
		t.Errorf("valid key while bad ones are blocked: status = %d, want 200", rec.Code)
	}
	if rec := keyRequest(router, "GET", "/api/jobs/", "wk_bad", nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("bad key after a valid one: status = %d, want 429", rec.Code)
	}
	if _, limited := globalAuthAttemptLimiter.retryAfter(client, time.Now()); limited {
		t.Error("bad keys blocked password logins")
	}
}

func TestRouteAPIKeys(t *testing.T) {
	keyRouter := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	router := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := routeAPIKeys(keyRouter, router)
	for header, want := range map[string]int{
		"":                       http.StatusOK,
		"Basic YWRtaW46YWRtaW4=": http.StatusOK,
		"Bearer eyJhbGciOi":      http.StatusOK,
		"Bearer wk_x":            http.StatusTeapot,
		"bearer wk_x":            http.StatusTeapot,
		"Bearer  wk_x ":          http.StatusTeapot,
	} {
		req := httptest.NewRequest("GET", "/api/feed", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Authorization %q: status = %d, want %d", header, rec.Code, want)
		}
	}
}

func TestTouchAPIKey(t *testing.T) {
	newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"read"}, "scope": {"read"}, "allJobs": {"true"}})
	now := time.Now()
	key, err := verifyAPIKey(token, now)
	if err != nil {
		t.Fatalf("verifyAPIKey: %v", err)
	}

	recent := now.Add(-apiKeyTouchInterval / 2)
	key.LastUsedAt = &recent
	if err := touchAPIKey(key, now); err != nil {
		t.Fatalf("touchAPIKey: %v", err)
	}
	if stored, _ := verifyAPIKey(token, now); stored.LastUsedAt != nil {
		t.Errorf("recorded a use %s after the last one", apiKeyTouchInterval/2)
	}

	key.LastUsedAt = nil
	if err := touchAPIKey(key, now); err != nil {
		t.Fatalf("touchAPIKey: %v", err)
	}
	if stored, _ := verifyAPIKey(token, now); stored.LastUsedAt == nil || !stored.LastUsedAt.Equal(now) {
		t.Errorf("LastUsedAt = %v, want %v", stored.LastUsedAt, now)
	}

	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(APIKeysBucket).Delete([]byte(key.ID))
	}); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	key.LastUsedAt = nil
	if err := touchAPIKey(key, now); err != nil {
		t.Fatalf("touchAPIKey after revoke: %v", err)
	}
	if _, err := verifyAPIKey(token, now); err == nil {
		t.Error("touching a revoked key brought it back")
	}
}

func TestAPIKeyFromForm(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		form    url.Values
		wantErr string
		check   func(t *testing.T, key *APIKey)
	}{
		{
			name: "selected jobs",
			form: url.Values{"name": {" ci "}, "scope": {"run", "read", "run"}, "job": {"deploy", "deploy"}, "pattern": {"test_*"}, "expiresInDays": {"90"}},
			check: func(t *testing.T, key *APIKey) {
				if key.Name != "ci" || !slices.Equal(key.Scopes, []string{"read", "run"}) || key.AllJobs ||
					!slices.Equal(key.Jobs, []string{"deploy"}) || !slices.Equal(key.Patterns, []string{"test_*"}) {
					t.Errorf("key = %+v", key)
				}
				if key.ExpiresAt == nil || !key.ExpiresAt.Equal(now.AddDate(0, 0, 90)) {
					t.Errorf("ExpiresAt = %v", key.ExpiresAt)
				}
			},
		},
		{
			name: "all jobs ignores the job list",
			form: url.Values{"name": {"admin"}, "scope": {"write"}, "allJobs": {"true"}, "job": {"deploy"}, "expiresInDays": {"0"}},
			check: func(t *testing.T, key *APIKey) {
				if !key.AllJobs || len(key.Jobs) != 0 || key.ExpiresAt != nil {
					t.Errorf("key = %+v", key)
				}
			},
		},
		{name: "no name", form: url.Values{"name": {"  "}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"0"}}, wantErr: "name"},
		{name: "long name", form: url.Values{"name": {strings.Repeat("ä", 101)}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"0"}}, wantErr: "longer"},
		{name: "no scope", form: url.Values{"name": {"x"}, "allJobs": {"true"}, "expiresInDays": {"0"}}, wantErr: "scope"},
		{name: "unknown scope", form: url.Values{"name": {"x"}, "scope": {"admin"}, "allJobs": {"true"}, "expiresInDays": {"0"}}, wantErr: "unknown scope"},
		{name: "no jobs", form: url.Values{"name": {"x"}, "scope": {"read"}, "expiresInDays": {"0"}}, wantErr: "select a job"},
		{name: "bad allJobs", form: url.Values{"name": {"x"}, "scope": {"read"}, "allJobs": {"yes"}, "expiresInDays": {"0"}}, wantErr: "allJobs"},
		{name: "job with slash", form: url.Values{"name": {"x"}, "scope": {"read"}, "job": {"a/b"}, "expiresInDays": {"0"}}, wantErr: "invalid job name"},
		{name: "pattern with backslash", form: url.Values{"name": {"x"}, "scope": {"read"}, "pattern": {`a\*`}, "expiresInDays": {"0"}}, wantErr: "invalid job name"},
		{name: "write on selected jobs", form: url.Values{"name": {"x"}, "scope": {"read", "write"}, "job": {"deploy"}, "expiresInDays": {"0"}}, wantErr: "all jobs"},
		{name: "write on a * pattern", form: url.Values{"name": {"x"}, "scope": {"write"}, "pattern": {"*"}, "expiresInDays": {"0"}}, wantErr: "all jobs"},
		{name: "no expiry", form: url.Values{"name": {"x"}, "scope": {"read"}, "allJobs": {"true"}}, wantErr: "expiresInDays"},
		{name: "negative expiry", form: url.Values{"name": {"x"}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"-1"}}, wantErr: "expiresInDays"},
		{name: "expiry too long", form: url.Values{"name": {"x"}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"3651"}}, wantErr: "expiresInDays"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := apiKeyFromForm(tt.form, now)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, key)
		})
	}
}

func TestAPIKeyHasScope(t *testing.T) {
	selected := &APIKey{Scopes: []string{ScopeRead, ScopeWrite}, Jobs: []string{"deploy"}}
	if !selected.HasScope(ScopeRead) || selected.HasScope(ScopeRun) {
		t.Error("read/run scopes wrong")
	}
	if selected.HasScope(ScopeWrite) {
		t.Error("write counted on a key that doesn't cover all jobs")
	}
	all := &APIKey{Scopes: []string{ScopeWrite}, AllJobs: true}
	if !all.HasScope(ScopeWrite) {
		t.Error("write not counted on a key that covers all jobs")
	}
}

func TestMatchJobPattern(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"deploy", "deploy", true},
		{"deploy", "deploy2", false},
		{"*", "anything", true},
		{"test_*", "test_api", true},
		{"test_*", "test_", true},
		{"test_*", "api_test_x", false},
		{"*_prod", "web_prod", true},
		{"*_prod", "web_prod_old", false},
		{"a*b*b", "abb", true},
		{"a*b*b", "ab", false},
		{"a*b*c", "axxbyyc", true},
		{"a*b*c", "axxcyyb", false},
		{"a*b*c", "axxc", false},
		{"v?c", "vxc", false},
		{"v?c", "v?c", true},
		{"[ab]", "a", false},
		{"[ab]", "[ab]", true},
	}
	for _, tt := range tests {
		if got := matchJobPattern(tt.pattern, tt.name); got != tt.want {
			t.Errorf("matchJobPattern(%q, %q) = %t, want %t", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestParseAPIKeyToken(t *testing.T) {
	newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"x"}, "scope": {"read"}, "allJobs": {"true"}})
	id, secret, ok := parseAPIKeyToken(token)
	if !ok || len(id) != 12 || len(secret) != 43 || token != apiKeyPrefix+id+"_"+secret {
		t.Fatalf("parseAPIKeyToken(%q) = %q, %q, %t", token, id, secret, ok)
	}
	for _, bad := range []string{
		"",
		strings.TrimPrefix(token, apiKeyPrefix),
		"wk_" + id + secret,
		"wk_" + id[:11] + "_" + secret,
		"wk_" + id[:10] + "_" + secret,
		"wk_" + id + "_" + secret[:42],
		"wk_" + id + "_" + secret + "A",
		"wk_zzzzzzzzzzzz_" + secret,
		"wk_" + id + "_" + secret[:42] + "+",
	} {
		if _, _, ok := parseAPIKeyToken(bad); ok {
			t.Errorf("parseAPIKeyToken(%q) accepted", bad)
		}
	}
}

func TestHandleAPIKeys(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	create := func(name string) NewAPIKeyData {
		form := url.Values{"name": {name}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"30"}}
		req := httptest.NewRequest("POST", "/api/keys/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		HandleAPIKeyCreate(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("create: status = %d; body %q", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}
		var created NewAPIKeyData
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode created key: %v", err)
		}
		return created
	}
	first := create("first")
	time.Sleep(time.Millisecond)
	second := create("second")
	if rec := keyRequest(router, "GET", "/api/jobs/", second.Token, nil); rec.Code != http.StatusOK {
		t.Errorf("created token: status = %d, want 200", rec.Code)
	}

	rec := httptest.NewRecorder()
	HandleAPIKeysList(rec, httptest.NewRequest("GET", "/api/keys/", nil))
	if strings.Contains(rec.Body.String(), "ecret") || strings.Contains(rec.Body.String(), second.Token[len(apiKeyPrefix)+13:]) {
		t.Errorf("list leaks a secret: %s", rec.Body.String())
	}
	var keys []APIKey
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(keys) != 2 || keys[0].ID != second.Key.ID || keys[1].ID != first.Key.ID {
		t.Errorf("list = %+v, want second then first", keys)
	}

	req := httptest.NewRequest("POST", "/api/keys/", strings.NewReader("name=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	HandleAPIKeyCreate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid create: status = %d, want 400", rec.Code)
	}

	revoke := httptest.NewRequest("DELETE", "/api/keys/000000000000", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "000000000000")
	rec = httptest.NewRecorder()
	HandleAPIKeyRevoke(rec, revoke.WithContext(contextWithRouteContext(revoke, rctx)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("revoke unknown key: status = %d, want 404", rec.Code)
	}
}

func contextWithRouteContext(r *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
}
