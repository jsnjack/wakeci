package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// tokenEnv is groupEnv that also returns the API key router, which takes
// build tokens
func tokenEnv(t *testing.T) http.Handler {
	t.Helper()
	router := newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	// No concurrent builds: the builds stay queued, and their tokens work
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GlobalBucket).Put([]byte("count"), []byte("10"))
	}); err != nil {
		t.Fatal(err)
	}
	return router
}

// liveToken returns the token of build id, which is in the queue
func liveToken(t *testing.T, id int) string {
	t.Helper()
	build := GlobalQueue.Live(id)
	if build == nil {
		t.Fatalf("build %d isn't in the queue", id)
	}
	token, err := build.buildToken()
	if err != nil || token == "" {
		t.Fatalf("build token: %q, %v", token, err)
	}
	t.Cleanup(build.revokeBuildToken)
	return token
}

func tokenCall(router http.Handler, method, target, token string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestBuildTokenStartsBuildsUnderItsBuild(t *testing.T) {
	router := tokenEnv(t)
	events, _ := captureAudit(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	token := liveToken(t, root)

	rec := tokenCall(router, "POST", "/api/job/nightly/run", token, url.Values{"TARGET": {"prod"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("run with the token: %d %q", rec.Code, rec.Body.String())
	}
	child, _ := strconv.Atoi(strings.TrimSpace(rec.Body.String()))
	build := GlobalQueue.Live(child)
	if build == nil {
		t.Fatalf("build %d isn't queued", child)
	}
	if got, want := build.Group(), (buildGroup{root, root}); got != want {
		t.Errorf("group %+v, want %+v", got, want)
	}
	if want := (buildTrigger{Kind: TriggerBuild, Build: root}); build.trigger != want {
		t.Errorf("trigger %+v, want %+v", build.trigger, want)
	}
	if !build.untrustedParams {
		t.Error("a build started with a token must follow the rules for untrusted parameters")
	}
	var run auditEvent
	for _, event := range events() {
		if event["event"] == "build.run" {
			run = event
		}
	}
	if run["auth"] != "build" || run["byBuild"] != strconv.Itoa(root) || run["build"] != strconv.Itoa(child) {
		t.Errorf("audit event %v, want auth=build byBuild=%d build=%d", run, root, child)
	}

	// Its own build as WAKE_PARENT is fine, another isn't
	if rec := tokenCall(router, "POST", "/api/job/nightly/run", token, url.Values{parentParam: {strconv.Itoa(root)}}); rec.Code != http.StatusOK {
		t.Errorf("run with its own build as parent: %d %q", rec.Code, rec.Body.String())
	}
	if rec := tokenCall(router, "POST", "/api/job/nightly/run", token, url.Values{parentParam: {strconv.Itoa(child)}}); rec.Code != http.StatusBadRequest {
		t.Errorf("run with another parent: %d %q, want 400", rec.Code, rec.Body.String())
	}
}

func TestBuildTokenMayReadAndRunOnly(t *testing.T) {
	router := tokenEnv(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	token := liveToken(t, root)

	rec := tokenCall(router, "GET", "/api/whoami", token, nil)
	var who WhoAmIData
	if err := json.Unmarshal(rec.Body.Bytes(), &who); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("whoami: %d %q", rec.Code, rec.Body.String())
	}
	if who.Auth != "build" || who.Build != root || who.Key != nil || who.Admin {
		t.Errorf("whoami %+v, want auth build of build %d", who, root)
	}
	for _, c := range []struct {
		method, target string
		form           url.Values
		status         int
	}{
		{"GET", "/api/feed", nil, http.StatusOK},
		{"GET", "/api/job/nightly", nil, http.StatusOK},
		{"GET", "/api/build/" + strconv.Itoa(root), nil, http.StatusOK},
		{"POST", "/api/jobs/create", url.Values{"name": {"made_by_a_build"}}, http.StatusForbidden},
		{"POST", "/api/job/nightly/set_active", url.Values{"active": {"false"}}, http.StatusForbidden},
		{"GET", "/api/settings", nil, http.StatusForbidden},
		{"GET", "/api/keys/", nil, http.StatusForbidden},
		// Untrusted parameters, as with a run key
		{"POST", "/api/job/nightly/run", url.Values{"TARGET": {`prod"; touch x`}}, http.StatusBadRequest},
		{"POST", "/api/job/nightly/run", url.Values{"TARGET": {"{{ secrets.TOKEN }}"}}, http.StatusBadRequest},
	} {
		if rec := tokenCall(router, c.method, c.target, token, c.form); rec.Code != c.status {
			t.Errorf("%s %s: %d %q, want %d", c.method, c.target, rec.Code, rec.Body.String(), c.status)
		}
	}
}

func TestBuildTokenStopsWhenItsBuildLeavesTheQueue(t *testing.T) {
	router := tokenEnv(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	other, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	token := liveToken(t, root)
	otherToken := liveToken(t, other)
	if token == otherToken {
		t.Fatal("two builds got the same token")
	}
	_, secret, _ := strings.Cut(strings.TrimPrefix(token, buildTokenPrefix), "_")
	for _, bad := range []string{
		buildTokenPrefix + strconv.Itoa(other) + "_" + secret, // another build's ID
		buildTokenPrefix + strconv.Itoa(root) + "_x" + secret,
		buildTokenPrefix + "0" + strconv.Itoa(root) + "_" + secret,
		buildTokenPrefix + strconv.Itoa(root),
		buildTokenPrefix,
	} {
		if rec := tokenCall(router, "GET", "/api/whoami", bad, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: %d, want 401", bad, rec.Code)
		}
	}

	GlobalQueue.Live(root).revokeBuildToken()
	if rec := tokenCall(router, "GET", "/api/whoami", token, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("token of a build that left the queue: %d, want 401", rec.Code)
	}
	if again, err := GlobalQueue.Live(root).buildToken(); again != "" || err != nil {
		t.Errorf("a build that left the queue got a new token: %q, %v", again, err)
	}
	if rec := tokenCall(router, "GET", "/api/whoami", otherToken, nil); rec.Code != http.StatusOK {
		t.Errorf("the other build's token: %d, want 200", rec.Code)
	}
}

func TestBuildTokenInTasksAndRedacted(t *testing.T) {
	tokenEnv(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	build := GlobalQueue.Live(root)
	env := build.generateDefaultEnvVariables()
	token := liveToken(t, root)
	if !slices.Contains(env, "WAKE_TOKEN="+token) {
		t.Errorf("task environment lacks WAKE_TOKEN: %q", env)
	}

	line := "curl -H 'Authorization: Bearer " + token + "' $WAKE_URL"
	if got, want := redactSecrets(line), "curl -H 'Authorization: Bearer "+redactedSecret+"' $WAKE_URL"; got != want {
		t.Errorf("redacted %q, want %q", got, want)
	}
	if plain, _, _ := colorLine("\x1b[31m" + token + "\x1b[0m"); strings.Contains(plain, token) {
		t.Errorf("task log line shows the token: %q", plain)
	}

	build.revokeBuildToken()
	for _, v := range build.generateDefaultEnvVariables() {
		if strings.HasPrefix(v, "WAKE_TOKEN=") {
			t.Errorf("a build that left the queue has %s", v)
		}
	}
}
