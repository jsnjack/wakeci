package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

// syncBuffer is a bytes.Buffer that builds started by a test can write to
// while the test reads it
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// auditEvent is one line of the audit log, its values as strings
type auditEvent map[string]string

// captureAudit sends the audit log to a buffer for the rest of the test and
// returns a function that reads the events so far, and the raw text
func captureAudit(t *testing.T) (func() []auditEvent, *syncBuffer) {
	t.Helper()
	buf := &syncBuffer{}
	old := auditLogger
	auditLogger = slog.New(slog.NewJSONHandler(buf, nil)).With("server", "test-server")
	t.Cleanup(func() { auditLogger = old })
	return func() []auditEvent {
		var events []auditEvent
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				t.Fatalf("audit line isn't JSON: %q: %v", line, err)
			}
			event := auditEvent{}
			for k, v := range raw {
				event[k] = fmt.Sprint(v)
			}
			events = append(events, event)
		}
		return events
	}, buf
}

// onlyAuditEvent returns the one event in events, failing the test if there
// is another number
func onlyAuditEvent(t *testing.T, events []auditEvent) auditEvent {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("got %d audit events, want 1: %v", len(events), events)
	}
	return events[0]
}

// checkAuditEvent fails the test unless event has the wanted values. An
// empty wanted value means the attr must be missing.
func checkAuditEvent(t *testing.T, event auditEvent, want map[string]string) {
	t.Helper()
	if event["msg"] != "audit" || event["server"] != "test-server" || event["level"] != "INFO" {
		t.Errorf("msg, server, level = %q, %q, %q; want audit, test-server, INFO", event["msg"], event["server"], event["level"])
	}
	for k, v := range want {
		got, ok := event[k]
		if v == "" && ok {
			t.Errorf("%s = %q, want no %s", k, got, k)
		}
		if v != "" && got != v {
			t.Errorf("%s = %q, want %q (event %v)", k, got, v, event)
		}
	}
}

// newAuditTestEnv is newAPIKeyTestEnv with a queue that keeps new builds
// queued
func newAuditTestEnv(t *testing.T) http.Handler {
	t.Helper()
	router := newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	return router
}

// createKeyFor saves a key made with form as created by user and returns
// its ID and token
func createKeyFor(t *testing.T, user string, form url.Values) (string, string) {
	t.Helper()
	form.Set("expiresInDays", "30")
	key, err := apiKeyFromForm(form, time.Now())
	if err != nil {
		t.Fatalf("apiKeyFromForm: %v", err)
	}
	key.CreatedBy = user
	token, err := saveNewAPIKey(key)
	if err != nil {
		t.Fatalf("saveNewAPIKey: %v", err)
	}
	return key.ID, token
}

// mutatingRoutes returns "METHOD pattern" of the routes of handler that can
// change something
func mutatingRoutes(t *testing.T, handler http.Handler) []string {
	t.Helper()
	var routes []string
	err := chi.Walk(handler.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !isReadOnly(method) {
			routes = append(routes, method+" "+route)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	return routes
}

func TestAuditedRoutes(t *testing.T) {
	main := mutatingRoutes(t, apiRouter())
	for _, route := range main {
		if _, ok := auditedRoutes[route]; !ok {
			t.Errorf("%s isn't in auditedRoutes", route)
		}
	}
	for _, route := range mutatingRoutes(t, newAPIKeyRouter(http.NotFoundHandler())) {
		if !slices.Contains(main, route) {
			t.Errorf("%s is only on the API key router", route)
		}
	}
	for route := range auditedRoutes {
		if !slices.Contains(main, route) {
			t.Errorf("auditedRoutes has %s, which isn't a route", route)
		}
	}
}

func TestAuditActions(t *testing.T) {
	const alice = "alice@example.com"
	jobForm := url.Values{"fileContent": {"desc: edited\ntasks: []\n"}}
	settingsForm := url.Values{"concurrentBuilds": {"2"}, "buildHistorySize": {"50"}}
	withPassword := url.Values{"password": {"new-secret-password"}, "concurrentBuilds": {"2"}, "buildHistorySize": {"50"}}

	tests := []struct {
		name   string
		user   string // "" for no session, adminUser is ":admin"
		method string
		target string
		form   url.Values
		basic  bool
		want   map[string]string
	}{
		{
			name: "run", user: alice, method: "POST", target: "/api/job/deploy/run",
			form: url.Values{"TARGET": {"prod"}, "VERSION": {"1"}},
			want: map[string]string{
				"event": "build.run", "outcome": "ok", "status": "200", "auth": "sso", "user": alice,
				"job": "deploy", "params": `["TARGET","VERSION"]`, "paramsOmitted": "", "method": "POST", "path": "/api/job/deploy/run",
			},
		},
		{
			// Not the same as parameters A and B
			name: "run with a comma in a parameter name", user: alice, method: "POST", target: "/api/job/deploy/run",
			form: url.Values{"A,B": {"1"}},
			want: map[string]string{"event": "build.run", "outcome": "ok", "params": `["A,B"]`},
		},
		{
			name: "run with Basic auth", method: "POST", target: "/api/job/deploy/run", basic: true,
			want: map[string]string{"event": "build.run", "outcome": "ok", "auth": "basic", "user": ""},
		},
		{
			name: "run without logging in", method: "POST", target: "/api/job/deploy/run",
			want: map[string]string{"event": "build.run", "outcome": "failed", "status": "403", "auth": "", "user": "", "job": "deploy"},
		},
		{
			name: "edit", user: alice, method: "POST", target: "/api/job/nightly", form: jobForm,
			want: map[string]string{"event": "job.edit", "outcome": "ok", "job": "nightly"},
		},
		{
			name: "edit with invalid yaml", user: alice, method: "POST", target: "/api/job/nightly",
			form: url.Values{"fileContent": {"tasks: ["}},
			want: map[string]string{"event": "job.edit", "outcome": "failed", "status": "400", "job": "nightly"},
		},
		{
			name: "disable", user: alice, method: "POST", target: "/api/job/nightly/set_active",
			form: url.Values{"active": {"false"}},
			want: map[string]string{"event": "job.set_active", "outcome": "ok", "job": "nightly", "active": "false"},
		},
		{
			name: "create", user: alice, method: "POST", target: "/api/jobs/create",
			form: url.Values{"name": {"audit_new"}},
			want: map[string]string{"event": "job.create", "outcome": "ok", "job": "audit_new"},
		},
		{
			name: "create an existing job", user: alice, method: "POST", target: "/api/jobs/create",
			form: url.Values{"name": {"deploy"}},
			want: map[string]string{"event": "job.create", "outcome": "failed", "status": "400", "job": "deploy"},
		},
		{
			name: "delete", user: alice, method: "DELETE", target: "/api/job/test_api",
			want: map[string]string{"event": "job.delete", "outcome": "ok", "job": "test_api"},
		},
		{
			// Build 1 of deploy has finished, so there is nothing to abort
			name: "abort", user: alice, method: "POST", target: "/api/build/1/abort",
			want: map[string]string{"event": "build.abort", "outcome": "failed", "status": "404", "build": "1", "job": "deploy"},
		},
		{
			name: "start", user: alice, method: "POST", target: "/api/build/2/start",
			want: map[string]string{"event": "build.start", "build": "2", "job": "nightly"},
		},
		{
			name: "flush", user: alice, method: "POST", target: "/api/build/2/flush",
			want: map[string]string{"event": "build.flush", "build": "2", "job": "nightly"},
		},
		{
			name: "abort a build that doesn't exist", user: alice, method: "POST", target: "/api/build/99/abort",
			want: map[string]string{"event": "build.abort", "outcome": "failed", "build": "99", "job": ""},
		},
		{
			name: "settings", user: alice, method: "POST", target: "/api/settings", form: settingsForm,
			want: map[string]string{
				"event": "settings.change", "outcome": "ok", "passwordChange": "false",
				"concurrentBuilds": "2", "buildHistorySize": "50",
			},
		},
		{
			name: "password change by the admin password", user: ":admin", method: "POST", target: "/api/settings", form: withPassword,
			want: map[string]string{"event": "settings.change", "outcome": "ok", "auth": "password", "user": "", "passwordChange": "true"},
		},
		{
			name: "password change by an SSO user", user: alice, method: "POST", target: "/api/settings", form: withPassword,
			want: map[string]string{"event": "settings.change", "outcome": "failed", "status": "403", "user": alice, "passwordChange": "true"},
		},
		{
			name: "create a key", user: alice, method: "POST", target: "/api/keys/",
			form: url.Values{"name": {"ci"}, "scope": {"read", "run"}, "job": {"deploy"}, "pattern": {"night*"}, "expiresInDays": {"0"}},
			want: map[string]string{
				"event": "key.create", "outcome": "ok", "name": "ci", "scopes": "read,run",
				"allJobs": "false", "jobs": `["deploy"]`, "patterns": `["night*"]`, "expires": "never",
			},
		},
		{
			// Neither can pass for a key that covers all jobs, or for two jobs
			name: "create a key for a job called all and one with a comma", user: alice, method: "POST", target: "/api/keys/",
			form: url.Values{"name": {"odd"}, "scope": {"read"}, "job": {"all", "a,b"}, "expiresInDays": {"0"}},
			want: map[string]string{"event": "key.create", "outcome": "ok", "allJobs": "false", "jobs": `["all","a,b"]`, "patterns": "[]"},
		},
		{
			name: "revoke access", user: ":admin", method: "DELETE", target: "/api/keys/?createdBy=bob%40example.com",
			want: map[string]string{"event": "access.revoke", "outcome": "ok", "person": "bob@example.com", "keys": "0", "sessions": "0"},
		},
		{
			name: "revoke access as an SSO user", user: alice, method: "DELETE", target: "/api/keys/?createdBy=bob%40example.com",
			want: map[string]string{"event": "access.revoke", "outcome": "failed", "status": "403", "person": "bob@example.com"},
		},
		{
			// Trimmed like the handler, so it names the account revoked
			name: "revoke access with spaces", user: ":admin", method: "DELETE", target: "/api/keys/?createdBy=%20bob%40example.com%20",
			want: map[string]string{"event": "access.revoke", "outcome": "ok", "person": "bob@example.com"},
		},
		{
			name: "revoke access without logging in", method: "DELETE", target: "/api/keys/?createdBy=bob%40example.com",
			want: map[string]string{"event": "access.revoke", "outcome": "failed", "status": "403", "person": "bob@example.com"},
		},
		{
			// A router mounted at /keys serves /keys as /keys/
			name: "create a key without the slash", user: alice, method: "POST", target: "/api/keys",
			form: url.Values{"name": {"ci"}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"0"}},
			want: map[string]string{"event": "key.create", "outcome": "ok", "name": "ci", "path": "/api/keys"},
		},
		{
			name: "revoke access without the slash", user: ":admin", method: "DELETE", target: "/api/keys?createdBy=bob%40example.com",
			want: map[string]string{"event": "access.revoke", "outcome": "ok", "person": "bob@example.com", "keys": "0"},
		},
		{
			name: "a route that only exists with the slash", user: alice, method: "POST", target: "/api/settings/",
			form: settingsForm,
			want: map[string]string{"event": "request", "outcome": "failed", "status": "404"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh, so builds 1 and 2 are the env's and nothing is queued
			newAuditTestEnv(t)
			events, raw := captureAudit(t)
			var cookie *http.Cookie
			switch tt.user {
			case "":
			case ":admin":
				cookie = loginAs(t, adminUser)
			default:
				cookie = loginAs(t, tt.user)
			}
			rec := apiCall(tt.method, tt.target, cookie, tt.form, func(r *http.Request) {
				r.RemoteAddr = "192.0.2.7:4321"
				if tt.basic {
					r.SetBasicAuth("", "admin")
				}
			})
			event := onlyAuditEvent(t, events())
			want := map[string]string{"ip": "192.0.2.7", "forwardedFor": ""}
			for k, v := range tt.want {
				want[k] = v
			}
			checkAuditEvent(t, event, want)
			if got := event["status"]; got != fmt.Sprint(rec.Code) {
				t.Errorf("status = %s, response %d", got, rec.Code)
			}
			if event["logID"] == "" {
				t.Error("no logID")
			}
			if strings.Contains(raw.String(), "new-secret-password") {
				t.Errorf("the audit log has the new password: %s", raw)
			}
		})
	}
}

// A request can't write a huge audit line, e.g. with a long parameter name
// or URL, and lists stay valid JSON
func TestAuditValuesAreBounded(t *testing.T) {
	newAuditTestEnv(t)
	cookie := loginAs(t, "alice@example.com")
	long := strings.Repeat("P", 1<<20)

	events, _ := captureAudit(t)
	form := url.Values{long: {"x"}}
	for i := range 300 {
		form.Set(fmt.Sprintf("A%03d", i), "y")
	}
	if rec := apiCall("POST", "/api/job/deploy/run", cookie, form, nil); rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	event := onlyAuditEvent(t, events())
	var params []string
	if err := json.Unmarshal([]byte(event["params"]), &params); err != nil {
		t.Fatalf("params isn't a JSON list: %v", err)
	}
	if len(event["params"]) > maxAuditValueLength || len(params) == 0 || params[0] != "A000" {
		t.Errorf("params has %d bytes and %d names, want at most %d bytes from A000", len(event["params"]), len(params), maxAuditValueLength)
	}
	if want := fmt.Sprint(len(form) - len(params)); event["paramsOmitted"] != want {
		t.Errorf("paramsOmitted = %q, want %s", event["paramsOmitted"], want)
	}

	events, _ = captureAudit(t)
	apiCall("POST", "/api/job/"+long+"/run", cookie, nil, nil)
	event = onlyAuditEvent(t, events())
	if want := long[:maxAuditValueLength] + "..."; event["job"] != want {
		t.Errorf("job has %d bytes, want %d ending in ...", len(event["job"]), len(want))
	}
	if len(event["path"]) != maxAuditValueLength+len("...") {
		t.Errorf("path has %d bytes, want %d", len(event["path"]), maxAuditValueLength+len("..."))
	}
}

func TestAuditListAttrs(t *testing.T) {
	if got := fmt.Sprint(auditListAttrs("jobs", nil)); got != "[jobs []]" {
		t.Errorf("no names: %s", got)
	}
	if got := fmt.Sprint(auditListAttrs("jobs", []string{"all", "a,b", `"q"`})); got != `[jobs ["all","a,b","\"q\""]]` {
		t.Errorf("names: %s", got)
	}
	var many []string
	for i := range 500 {
		many = append(many, fmt.Sprintf("job_%03d", i))
	}
	attrs := auditListAttrs("jobs", many)
	var list []string
	if err := json.Unmarshal([]byte(attrs[1].(string)), &list); err != nil || len(attrs[1].(string)) > maxAuditValueLength {
		t.Fatalf("list of %d bytes isn't JSON within the limit: %v", len(attrs[1].(string)), err)
	}
	if len(attrs) != 4 || attrs[2] != "jobsOmitted" || attrs[3] != len(many)-len(list) {
		t.Errorf("omitted: %v, kept %d", attrs[2:], len(list))
	}
}

// A proxy appends the address it saw to X-Forwarded-For, so a long header
// keeps its end as well as its start
func TestForwardedForKeepsBothEnds(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("X-Forwarded-For", "203.0.113.1, "+strings.Repeat("9", 1000))
	r.Header.Add("X-Forwarded-For", "198.51.100.4")
	got := forwardedFor(r)
	if !strings.HasPrefix(got, "203.0.113.1, 999") || !strings.HasSuffix(got, "999, 198.51.100.4") || !strings.Contains(got, "...") {
		t.Errorf("forwardedFor = %q, want its start, ... and its end", got)
	}
	if len(got) != maxForwardedForLength+len("...") {
		t.Errorf("forwardedFor has %d bytes, want %d", len(got), maxForwardedForLength+len("..."))
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.4")
	if got := forwardedFor(r); got != "203.0.113.1, 198.51.100.4" {
		t.Errorf("short forwardedFor = %q, want it unchanged", got)
	}
}

// A create that fails after writing the job file says so: the file stays,
// and the watcher or a restart can still add the job
func TestAuditJobCreateLeavesFile(t *testing.T) {
	newAuditTestEnv(t)
	// A value where the job's bucket goes makes registering it fail
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(JobsBucket).Put([]byte("audit_broken"), []byte("x"))
	}); err != nil {
		t.Fatalf("break the job's bucket: %v", err)
	}
	events, _ := captureAudit(t)
	rec := apiCall("POST", "/api/jobs/create", loginAs(t, "alice@example.com"), url.Values{"name": {"audit_broken"}}, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("create: %d %s, want 500", rec.Code, rec.Body)
	}
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{
		"event": "job.create", "outcome": "failed", "job": "audit_broken", "fileCreated": "true",
	})
	if _, err := os.Stat(Config.JobDir + "audit_broken" + Config.jobsExt); err != nil {
		t.Errorf("the job file: %v", err)
	}
}

// Sessions end before the keys are deleted, so a revoke that fails after
// ending them still says how many it ended
func TestAuditAccessRevokeFailsAfterSessions(t *testing.T) {
	newAuditTestEnv(t)
	loginAs(t, "bob@example.com")
	// A key that can't be read makes deleting the keys fail
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(APIKeysBucket).Put([]byte("broken"), []byte("not json"))
	}); err != nil {
		t.Fatalf("break a key: %v", err)
	}
	events, _ := captureAudit(t)
	rec := apiCall("DELETE", "/api/keys/?createdBy=bob%40example.com", loginAs(t, adminUser), nil, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("revoke: %d %s, want 500", rec.Code, rec.Body)
	}
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{
		"event": "access.revoke", "outcome": "failed", "person": "bob@example.com", "sessions": "1", "keys": "",
	})
}

func TestAuditRunBuildID(t *testing.T) {
	newAuditTestEnv(t)
	events, _ := captureAudit(t)
	rec := apiCall("POST", "/api/job/deploy/run", loginAs(t, "alice@example.com"), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{"build": rec.Body.String(), "params": ""})
}

func TestAuditKeyRevoke(t *testing.T) {
	newAuditTestEnv(t)
	id, _ := createKeyFor(t, "bob@example.com", url.Values{"name": {"bob-ci"}, "scope": {"read"}, "allJobs": {"true"}})
	events, _ := captureAudit(t)
	rec := apiCall("DELETE", "/api/keys/"+id, loginAs(t, adminUser), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
	}
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{
		"event": "key.revoke", "outcome": "ok", "auth": "password", "key": id, "createdBy": "bob@example.com",
	})
}

// The admin password can revoke a key that can't be read; to an SSO user
// it isn't theirs
func TestAuditKeyRevokeUnreadable(t *testing.T) {
	newAuditTestEnv(t)
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(APIKeysBucket).Put([]byte("broken"), []byte("not json"))
	}); err != nil {
		t.Fatalf("break a key: %v", err)
	}
	if rec := apiCall("DELETE", "/api/keys/broken", loginAs(t, "alice@example.com"), nil, nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("SSO user: %d, want 500", rec.Code)
	}
	events, _ := captureAudit(t)
	if rec := apiCall("DELETE", "/api/keys/broken", loginAs(t, adminUser), nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s, want 200", rec.Code, rec.Body)
	}
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{
		"event": "key.revoke", "outcome": "ok", "key": "broken", "createdBy": "",
	})
}

func TestAuditKeyCreateHasNoSecret(t *testing.T) {
	newAuditTestEnv(t)
	events, raw := captureAudit(t)
	created := createKeyAs(t, "alice@example.com", "ci")
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{
		"event": "key.create", "key": created.Key.ID, "allJobs": "true", "jobs": "", "patterns": "", "scopes": "read",
	})
	_, secret, _ := parseAPIKeyToken(created.Token)
	if strings.Contains(raw.String(), secret) {
		t.Errorf("the audit log has the key's secret: %s", raw)
	}
}

func TestAuditAPIKey(t *testing.T) {
	router := newAuditTestEnv(t)
	readID, readToken := createKeyFor(t, "alice@example.com", url.Values{"name": {"read"}, "scope": {"read"}, "allJobs": {"true"}})
	runID, runToken := createKeyFor(t, "", url.Values{"name": {"run"}, "scope": {"run"}, "job": {"deploy"}})
	_, readSecret, _ := parseAPIKeyToken(readToken)
	badToken := strings.TrimSuffix(readToken, readSecret) + strings.Repeat("A", len(readSecret))

	tests := []struct {
		name   string
		method string
		target string
		token  string
		form   url.Values
		want   map[string]string
	}{
		{
			name: "run", method: "POST", target: "/api/job/deploy/run", token: runToken,
			want: map[string]string{
				"event": "build.run", "outcome": "ok", "auth": "apikey", "apiKey": runID, "user": "", "job": "deploy",
			},
		},
		{
			name: "run without the run scope", method: "POST", target: "/api/job/deploy/run", token: readToken,
			want: map[string]string{
				"event": "build.run", "outcome": "failed", "status": "403", "auth": "apikey", "apiKey": readID,
				"user": "alice@example.com", "job": "deploy",
			},
		},
		{
			name: "run a job the key doesn't cover", method: "POST", target: "/api/job/nightly/run", token: runToken,
			want: map[string]string{"event": "build.run", "outcome": "failed", "status": "403", "job": "nightly"},
		},
		{
			name: "run with a parameter that references a secret", method: "POST", target: "/api/job/deploy/run", token: runToken,
			form: url.Values{"TARGET": {"{{ secrets.A }}"}},
			want: map[string]string{"event": "build.run", "outcome": "failed", "status": "400"},
		},
		{
			name: "a route that doesn't take keys", method: "POST", target: "/api/settings", token: readToken,
			want: map[string]string{"event": "settings.change", "outcome": "failed", "status": "403", "apiKey": readID},
		},
		{
			name: "a route that doesn't exist", method: "DELETE", target: "/api/nothing", token: readToken,
			want: map[string]string{"event": "request", "outcome": "failed", "status": "403", "method": "DELETE", "path": "/api/nothing"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, _ := captureAudit(t)
			keyRequest(router, tt.method, tt.target, tt.token, tt.form)
			checkAuditEvent(t, onlyAuditEvent(t, events()), tt.want)
		})
	}

	t.Run("reading isn't audited", func(t *testing.T) {
		events, _ := captureAudit(t)
		if rec := keyRequest(router, "GET", "/api/build/1", readToken, nil); rec.Code != http.StatusOK {
			t.Fatalf("read build: %d", rec.Code)
		}
		if got := events(); len(got) != 0 {
			t.Errorf("audit events for a read: %v", got)
		}
	})

	t.Run("bad key", func(t *testing.T) {
		events, raw := captureAudit(t)
		rec := keyRequest(router, "POST", "/api/job/deploy/run", badToken, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		got := events()
		if len(got) != 2 {
			t.Fatalf("got %d audit events, want login.apikey and build.run: %v", len(got), got)
		}
		checkAuditEvent(t, got[0], map[string]string{"event": "login.apikey", "outcome": "failed", "apiKey": "", "blocked": ""})
		if !strings.Contains(got[0]["reason"], "wrong secret for key "+readID) {
			t.Errorf("reason = %q", got[0]["reason"])
		}
		checkAuditEvent(t, got[1], map[string]string{"event": "build.run", "outcome": "failed", "status": "401", "auth": ""})
		if strings.Contains(raw.String(), strings.Repeat("A", len(readSecret))) {
			t.Errorf("the audit log has the token: %s", raw)
		}
	})

	t.Run("bad key that blocks the client", func(t *testing.T) {
		apiKeyAttemptLimiter = newAuthAttemptLimiter(1, authFailureWindow, authBlockDuration)
		events, _ := captureAudit(t)
		rec := keyRequest(router, "GET", "/api/jobs/", badToken, nil)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429", rec.Code)
		}
		checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{"event": "login.apikey", "blocked": "true"})

		// Requests while blocked are in the request line only
		keyRequest(router, "GET", "/api/jobs/", badToken, nil)
		if got := events(); len(got) != 1 {
			t.Errorf("audit events while blocked: %v", got[1:])
		}
	})
}

func TestAuditPasswordLogin(t *testing.T) {
	newAuditTestEnv(t)
	globalAuthAttemptLimiter = newAuthAttemptLimiter(2, authFailureWindow, authBlockDuration)
	login := func(password string) int {
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(url.Values{"password": {password}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		rec := httptest.NewRecorder()
		HandleLogIn(rec, req)
		return rec.Code
	}

	events, _ := captureAudit(t)
	if code := login("admin"); code != http.StatusNoContent {
		t.Fatalf("login: %d", code)
	}
	checkAuditEvent(t, onlyAuditEvent(t, events()), map[string]string{
		"event": "login.password", "outcome": "ok", "auth": "password", "forwardedFor": "203.0.113.9", "ip": "192.0.2.1",
	})

	for _, want := range []map[string]string{
		{"event": "login.password", "outcome": "failed", "auth": "", "blocked": ""},
		{"event": "login.password", "outcome": "failed", "blocked": "true"},
	} {
		events, raw := captureAudit(t)
		login("wrong-password-123")
		checkAuditEvent(t, onlyAuditEvent(t, events()), want)
		if strings.Contains(raw.String(), "wrong-password-123") {
			t.Errorf("the audit log has the password: %s", raw)
		}
	}

	// Blocked attempts aren't checked, and are in the request line only
	events, _ = captureAudit(t)
	if code := login("admin"); code != http.StatusTooManyRequests {
		t.Fatalf("login while blocked: %d", code)
	}
	if got := events(); len(got) != 0 {
		t.Errorf("audit events while blocked: %v", got)
	}
}

func TestAuditBasicAuthFailure(t *testing.T) {
	newAuditTestEnv(t)
	events, _ := captureAudit(t)
	apiCall("POST", "/api/job/deploy/run", nil, nil, func(r *http.Request) { r.SetBasicAuth("", "wrong") })
	got := events()
	if len(got) != 2 {
		t.Fatalf("got %d audit events, want login.basic and build.run: %v", len(got), got)
	}
	checkAuditEvent(t, got[0], map[string]string{"event": "login.basic", "outcome": "failed", "auth": ""})
	checkAuditEvent(t, got[1], map[string]string{"event": "build.run", "outcome": "failed", "status": "403", "auth": ""})
}

func TestAuditLogout(t *testing.T) {
	newAuditTestEnv(t)
	logout := func(cookie *http.Cookie) []auditEvent {
		events, _ := captureAudit(t)
		req := httptest.NewRequest("GET", "/auth/logout", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		HandleLogOut(httptest.NewRecorder(), req)
		return events()
	}
	cookie := loginAs(t, "alice@example.com")
	checkAuditEvent(t, onlyAuditEvent(t, logout(cookie)), map[string]string{
		"event": "logout", "outcome": "ok", "auth": "sso", "user": "alice@example.com",
	})
	// Logging out again, or without a session, isn't someone logging out
	if got := logout(cookie); len(got) != 0 {
		t.Errorf("audit events for an ended session: %v", got)
	}
	if got := logout(nil); len(got) != 0 {
		t.Errorf("audit events without a session: %v", got)
	}
}

func TestAuditScheduledBuild(t *testing.T) {
	newAuditTestEnv(t)
	events, _ := captureAudit(t)
	(&Job{Name: "deploy"}).Run()
	(&Job{Name: "missing"}).Run()
	got := events()
	if len(got) != 2 {
		t.Fatalf("got %d audit events, want 2: %v", len(got), got)
	}
	checkAuditEvent(t, got[0], map[string]string{
		"event": "build.run", "outcome": "ok", "trigger": "interval", "job": "deploy", "auth": "", "ip": "", "logID": "",
	})
	if got[0]["build"] == "" {
		t.Error("no build")
	}
	checkAuditEvent(t, got[1], map[string]string{"event": "build.run", "outcome": "failed", "trigger": "interval", "job": "missing"})
}

func TestRequestLineSaysWho(t *testing.T) {
	newAuditTestEnv(t)
	buf := &syncBuffer{}
	oldL := L
	L = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	t.Cleanup(func() { L = oldL })

	router := chi.NewRouter()
	router.Use(LogMi)
	router.Route("/api", apiRoutes)
	request := func(cookie *http.Cookie) map[string]any {
		buf.buf.Reset()
		req := httptest.NewRequest("GET", "/api/jobs/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		router.ServeHTTP(httptest.NewRecorder(), req)
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("log line isn't JSON: %q", line)
			}
			if record["msg"] == "request completed" {
				return record
			}
		}
		t.Fatalf("no request line in %s", buf)
		return nil
	}

	line := request(loginAs(t, "alice@example.com"))
	if line["auth"] != "sso" || line["user"] != "alice@example.com" || line["forwardedFor"] != "203.0.113.9" {
		t.Errorf("request line = %v", line)
	}
	line = request(loginAs(t, adminUser))
	if line["auth"] != "password" || line["user"] != nil {
		t.Errorf("request line = %v", line)
	}

	// A session token never reaches the logs
	invalid := &http.Cookie{Name: "session", Value: "not-a-real-session-token"}
	line = request(invalid)
	if line["auth"] != nil || line["status"] != float64(http.StatusForbidden) {
		t.Errorf("request line = %v", line)
	}
	if strings.Contains(buf.String(), invalid.Value) {
		t.Errorf("the log has the session token: %s", buf)
	}
}
