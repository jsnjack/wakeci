package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

// sessionRouter has the routes a browser uses, with their middleware
func sessionRouter() http.Handler {
	router := chi.NewRouter()
	router.Route("/api", apiRoutes)
	storage := HandleWakespaceResource(http.FileServer(http.Dir(Config.WorkDir + "wakespace")))
	router.Route("/storage", storageRoutes(storage))
	router.With(AuthMi).Get("/auth/_isLoggedIn", HandleIsLoggedIn)
	return router
}

// sessionRequest sends a request through sessionRouter with the cookie
func sessionRequest(method, target string, cookie *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	sessionRouter().ServeHTTP(rec, req)
	return rec
}

func adminRequest(method, target string, form url.Values) *httptest.ResponseRecorder {
	return apiCall(method, target, nil, form, func(r *http.Request) { r.SetBasicAuth("", "admin") })
}

func readGroups(t *testing.T) []SSOGroup {
	t.Helper()
	groups, err := listSSOGroups()
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	return groups
}

func groupNames(groups []SSOGroup) []string {
	names := []string{}
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func TestSeedSSOGroups(t *testing.T) {
	newTestBuildEnv(t)

	// Nothing to add doesn't count as the first start with groups
	if err := seedSSOGroups(nil, time.Now()); err != nil {
		t.Fatalf("seed nothing: %v", err)
	}
	if err := seedSSOGroups([]string{"Developers", " Ops "}, time.Now()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	groups := readGroups(t)
	if !slices.Equal(groupNames(groups), []string{"Developers", "Ops"}) {
		t.Fatalf("groups = %q, want Developers and Ops", groupNames(groups))
	}
	for _, group := range groups {
		if !group.AllJobs || !slices.Equal(group.Scopes, []string{ScopeRead, ScopeRun, ScopeWrite}) {
			t.Errorf("group %s = %+v, want every scope on all jobs", group.Name, group.Grant)
		}
	}

	// Later starts leave the groups to the settings: a deleted group stays
	// deleted and a group added to the config file isn't added
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(SSOGroupsBucket).Delete(ssoGroupKey("Ops"))
	}); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	if err := seedSSOGroups([]string{"Developers", "Ops", "QA"}, time.Now()); err != nil {
		t.Fatalf("seed again: %v", err)
	}
	if names := groupNames(readGroups(t)); !slices.Equal(names, []string{"Developers"}) {
		t.Errorf("groups after a restart = %q, want only Developers", names)
	}
}

func TestSSOGroupsAccess(t *testing.T) {
	newTestBuildEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})
	addTestGroup(t, "Releasers", Grant{Scopes: []string{ScopeRun}, Patterns: []string{"release_*"}})

	tests := []struct {
		name    string
		groups  []string
		allowed map[string]bool
	}{
		{"name in another case", []string{"service team"}, map[string]bool{"read deploy": true, "run deploy": false, "read nightly": false}},
		{"groups that don't exist", []string{"Strangers", "Service Team"}, map[string]bool{"read deploy": true}},
		{"two groups", []string{"Service Team", "Releasers"}, map[string]bool{"read deploy": true, "run release_web": true, "read release_web": false}},
		{"no known group", []string{"Strangers"}, map[string]bool{"read deploy": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access, err := ssoGroupsAccess(tt.groups)
			if err != nil {
				t.Fatalf("ssoGroupsAccess: %v", err)
			}
			for check, want := range tt.allowed {
				scope, job, _ := strings.Cut(check, " ")
				if got := access.Allows(scope, job); got != want {
					t.Errorf("Allows(%s, %s) = %t, want %t", scope, job, got, want)
				}
			}
		})
	}
	if !ssoAdmits([]string{"Strangers", "SERVICE TEAM"}) || ssoAdmits([]string{"Strangers"}) || ssoAdmits(nil) {
		t.Error("ssoAdmits doesn't match the stored groups ignoring case")
	}
}

func TestSSOGroupHandlers(t *testing.T) {
	newAPIKeyTestEnv(t)
	changes := 0
	onSSOAccessChange = func() { changes++ }
	t.Cleanup(func() { onSSOAccessChange = nil })

	steps := []struct {
		name       string
		method     string
		target     string
		form       url.Values
		wantStatus int
		wantBody   string
		wantChange bool
	}{
		{"add a group", "POST", "/api/groups/", url.Values{"name": {" Service Team "}, "scope": {"read"}, "job": {"deploy"}}, http.StatusOK, `"name":"Service Team"`, true},
		{"add it again in another case", "POST", "/api/groups/", url.Values{"name": {"service team"}, "scope": {"read"}, "allJobs": {"true"}}, http.StatusConflict, "exists already", false},
		{"add one without a name", "POST", "/api/groups/", url.Values{"name": {" "}, "scope": {"read"}, "allJobs": {"true"}}, http.StatusBadRequest, "enter the group's name", false},
		{"add one without scopes", "POST", "/api/groups/", url.Values{"name": {"QA"}, "allJobs": {"true"}}, http.StatusBadRequest, "select at least one scope", false},
		{"add write on some jobs", "POST", "/api/groups/", url.Values{"name": {"QA"}, "scope": {"write"}, "job": {"deploy"}}, http.StatusBadRequest, "needs access to all jobs", false},
		{"add one with a control character", "POST", "/api/groups/", url.Values{"name": {"Q\x01A"}, "scope": {"read"}, "allJobs": {"true"}}, http.StatusBadRequest, "control characters", false},
		{"add another group", "POST", "/api/groups/", url.Values{"name": {"QA"}, "scope": {"read"}, "allJobs": {"true"}}, http.StatusOK, `"name":"QA"`, true},
		{"change a group, name in the URL encoded", "POST", "/api/groups/service%20team", url.Values{"scope": {"read", "run"}, "pattern": {"dep*"}}, http.StatusOK, `"patterns":["dep*"]`, true},
		{"change a group that doesn't exist", "POST", "/api/groups/Nobody", url.Values{"scope": {"read"}, "allJobs": {"true"}}, http.StatusNotFound, "No group named Nobody", false},
		{"change with invalid permissions", "POST", "/api/groups/QA", url.Values{"scope": {"admin"}, "allJobs": {"true"}}, http.StatusBadRequest, "unknown scope", false},
		{"remove a group", "DELETE", "/api/groups/QA", nil, http.StatusOK, "", true},
		{"remove it again", "DELETE", "/api/groups/QA", nil, http.StatusNotFound, "No group named QA", false},
	}
	for _, step := range steps {
		before := changes
		rec := adminRequest(step.method, step.target, step.form)
		if rec.Code != step.wantStatus {
			t.Fatalf("%s: status = %d, want %d; body %q", step.name, rec.Code, step.wantStatus, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), step.wantBody) {
			t.Errorf("%s: body = %q, want it to contain %q", step.name, rec.Body.String(), step.wantBody)
		}
		if changed := changes > before; changed != step.wantChange {
			t.Errorf("%s: live connections closed = %t, want %t", step.name, changed, step.wantChange)
		}
	}

	groups := readGroups(t)
	if len(groups) != 1 || groups[0].Name != "Service Team" || !slices.Equal(groups[0].Scopes, []string{ScopeRead, ScopeRun}) ||
		groups[0].AllJobs || len(groups[0].Jobs) != 0 || !slices.Equal(groups[0].Patterns, []string{"dep*"}) {
		t.Errorf("groups = %+v, want Service Team with read and run on dep*", groups)
	}

	// Two people signed in with the group, one of them twice, and one with
	// another group
	loginInGroups(t, "sam@example.com", "service team")
	loginInGroups(t, "sam@example.com", "Service Team")
	loginInGroups(t, "kim@example.com", "Service Team", "Elsewhere")
	loginInGroups(t, "lee@example.com", "Elsewhere")
	rec := adminRequest("GET", "/api/groups/", nil)
	var data SSOGroupsData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if len(data.Groups) != 1 || data.Groups[0].Name != "Service Team" || data.Groups[0].SignedIn != 2 || data.SSOEnabled {
		t.Errorf("admin list = %+v, want Service Team with 2 signed in, single sign-on not set up", data)
	}
}

func TestSSOGroupHandlersOnlyForTheAdmin(t *testing.T) {
	newAPIKeyTestEnv(t)
	addTestGroup(t, "Readers", Grant{Scopes: []string{ScopeRead}, AllJobs: true})
	writer := loginAs(t, "alice@example.com")
	reader := loginInGroups(t, "bob@example.com", "Readers")

	tests := []struct {
		name   string
		cookie *http.Cookie
		method string
		target string
		form   url.Values
	}{
		{"list without write", reader, "GET", "/api/groups/", nil},
		// Only the admin password sees the groups at all
		{"list with write", writer, "GET", "/api/groups/", nil},
		{"add with write", writer, "POST", "/api/groups/", url.Values{"name": {"Mine"}, "scope": {"read"}, "allJobs": {"true"}}},
		{"change with write", writer, "POST", "/api/groups/Readers", url.Values{"scope": {"read", "run"}, "allJobs": {"true"}}},
		{"remove with write", writer, "DELETE", "/api/groups/Readers", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := sessionRequest(tt.method, tt.target, tt.cookie, tt.form); rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403; body %q", rec.Code, rec.Body.String())
			}
		})
	}
	if groups := readGroups(t); len(groups) != 2 || !slices.Equal(groups[1].Scopes, []string{ScopeRead}) {
		t.Errorf("groups = %+v, want them unchanged", groups)
	}
}

func TestLimitedSSOUser(t *testing.T) {
	newAPIKeyTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})
	cookie := loginInGroups(t, "sam@example.com", "service team")

	tests := []struct {
		method, target string
		form           url.Values
		wantStatus     int
		wantBody       string
	}{
		{"GET", "/api/job/deploy", nil, http.StatusOK, "desc: deploy"},
		{"GET", "/api/job/nightly", nil, http.StatusForbidden, "Your account doesn't cover that job"},
		{"GET", "/api/build/1", nil, http.StatusOK, `"name":"deploy"`},
		{"GET", "/api/build/2", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/storage/build/1/task_1.log", nil, http.StatusOK, "log of deploy"},
		{"GET", "/storage/build/2/task_1.log", nil, http.StatusForbidden, "doesn't cover that job"},
		{"GET", "/storage/build/", nil, http.StatusForbidden, "only the files of a build"},
		{"POST", "/api/job/deploy/run", nil, http.StatusForbidden, "doesn't have the run scope"},
		{"POST", "/api/build/1/abort", nil, http.StatusForbidden, "doesn't have the run scope"},
		{"POST", "/api/job/deploy", url.Values{"fileContent": {"tasks: []"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"POST", "/api/jobs/create", url.Values{"name": {"new"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"GET", "/api/settings", nil, http.StatusForbidden, "doesn't have the write scope"},
		{"POST", "/api/settings", url.Values{"concurrentBuilds": {"9"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"POST", "/api/update/", url.Values{"version": {"9.9.9"}}, http.StatusForbidden, "doesn't have the write scope"},
		{"GET", "/auth/_isLoggedIn", nil, http.StatusOK, `"grants":[{"scopes":["read"],"allJobs":false,"jobs":["deploy"],"patterns":[]}]`},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := sessionRequest(tt.method, tt.target, cookie, tt.form)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}

	rec := sessionRequest("GET", "/api/feed", cookie, nil)
	if ids := buildIDs(t, rec.Body.Bytes()); !slices.Equal(ids, []int{1}) {
		t.Errorf("feed = %v, want only build 1 of deploy", ids)
	}
	rec = sessionRequest("GET", "/api/jobs/", cookie, nil)
	if names := jobNames(t, rec.Body.Bytes()); !slices.Equal(names, []string{"deploy"}) {
		t.Errorf("jobs = %q, want only deploy", names)
	}
}

func TestSSOUserInSeveralGroups(t *testing.T) {
	newAPIKeyTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})
	addTestGroup(t, "Night Shift", Grant{Scopes: []string{ScopeRead, ScopeRun}, Jobs: []string{"nightly"}})
	cookie := loginInGroups(t, "sam@example.com", "Service Team", "Night Shift")
	// A disabled job stops RunJob before it queues a build, with an error
	// that only the handler gives
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(JobsBucket).Bucket([]byte("nightly")).Put([]byte("active"), []byte("false"))
	}); err != nil {
		t.Fatalf("disable job: %v", err)
	}

	tests := []struct {
		method, target string
		wantStatus     int
	}{
		{"GET", "/api/build/1", http.StatusOK},
		{"GET", "/api/build/2", http.StatusOK},
		{"POST", "/api/job/deploy/run", http.StatusForbidden},
		{"POST", "/api/job/nightly/run", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			if rec := sessionRequest(tt.method, tt.target, cookie, nil); rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
	rec := sessionRequest("GET", "/api/jobs/", cookie, nil)
	if names := jobNames(t, rec.Body.Bytes()); !slices.Equal(names, []string{"deploy", "nightly"}) {
		t.Errorf("jobs = %q, want deploy and nightly", names)
	}
}

// A change to a group applies to its members' next request, without a new
// login
func TestSSOGroupChangeAppliesAtOnce(t *testing.T) {
	newAPIKeyTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})
	cookie := loginInGroups(t, "sam@example.com", "Service Team")

	if rec := sessionRequest("GET", "/api/build/2", cookie, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("before the change: status = %d, want 403", rec.Code)
	}
	if rec := adminRequest("POST", "/api/groups/Service%20Team", url.Values{"scope": {"read"}, "job": {"deploy", "nightly"}}); rec.Code != http.StatusOK {
		t.Fatalf("change group: %d %q", rec.Code, rec.Body.String())
	}
	if rec := sessionRequest("GET", "/api/build/2", cookie, nil); rec.Code != http.StatusOK {
		t.Errorf("after adding nightly: status = %d, want 200", rec.Code)
	}
	if rec := adminRequest("DELETE", "/api/groups/Service%20Team", nil); rec.Code != http.StatusOK {
		t.Fatalf("remove group: %d %q", rec.Code, rec.Body.String())
	}
	rec := sessionRequest("GET", "/auth/_isLoggedIn", cookie, nil)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "None of your groups has access") {
		t.Errorf("after removing the group: %d %q, want 403", rec.Code, rec.Body.String())
	}
}

// Every /api route checks what a single sign-on user may do, unless it is
// listed here. A new route without a check fails this test.
func TestEveryAPIRouteChecksAccess(t *testing.T) {
	newAPIKeyTestEnv(t)
	// Read on a job that doesn't exist: logged in, but allowed nothing real
	addTestGroup(t, "Outsiders", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"no_such_job"}})
	cookie := loginInGroups(t, "eve@example.com", "Outsiders")

	open := map[string]bool{
		// Filtered to the user's jobs
		"GET /api/feed":  true,
		"GET /api/jobs/": true,
		// The state of the server, for the banner, and the user's own access
		"GET /api/queue/": true,
		"GET /api/whoami": true,
		// Their own keys; creating one checks the user's access itself
		"GET /api/keys/":        true,
		"POST /api/keys/":       true,
		"DELETE /api/keys/{id}": true,
	}
	router := chi.NewRouter()
	router.Route("/api", apiRoutes)
	seen := map[string]bool{}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.ReplaceAll(route, "/*/", "/")
		pattern := method + " " + route
		seen[pattern] = true
		if open[pattern] {
			return nil
		}
		target := strings.NewReplacer("{name}", "deploy", "{id}", "1").Replace(route)
		rec := sessionRequest(method, target, cookie, url.Values{"scope": {"read"}, "allJobs": {"true"}, "fileContent": {"tasks: []"}})
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403; body %q", pattern, rec.Code, rec.Body.String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	for pattern := range open {
		if !seen[pattern] {
			t.Errorf("%s is listed as open but isn't a route", pattern)
		}
	}
	if len(seen) <= len(open) {
		t.Errorf("walked %d routes, want the closed ones too", len(seen))
	}
}

func TestAPIKeyOfSSOUserFollowsTheirGroups(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead, ScopeRun}, Jobs: []string{"deploy"}})
	cookie := loginInGroups(t, "sam@example.com", "Service Team")

	create := func(form url.Values) *httptest.ResponseRecorder {
		form.Set("name", "sam-ci")
		form.Set("expiresInDays", "30")
		return sessionRequest("POST", "/api/keys/", cookie, form)
	}
	refused := []struct {
		name     string
		form     url.Values
		wantBody string
	}{
		{"write", url.Values{"scope": {"write"}, "allJobs": {"true"}}, "your groups don't have the write scope"},
		{"a job the groups don't cover", url.Values{"scope": {"read"}, "job": {"nightly"}}, "your groups don't allow read on nightly"},
	}
	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			rec := create(tt.form)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("create = %d %q, want 403 with %q", rec.Code, rec.Body.String(), tt.wantBody)
			}
		})
	}

	// A key on all jobs only reaches what the creator's groups allow
	rec := create(url.Values{"scope": {"read"}, "allJobs": {"true"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("create key: %d %q", rec.Code, rec.Body.String())
	}
	var created NewAPIKeyData
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode key: %v", err)
	}
	if !slices.Equal(created.Key.CreatorGroups, []string{"Service Team"}) {
		t.Errorf("creatorGroups = %q, want [Service Team]", created.Key.CreatorGroups)
	}
	token := created.Token
	if rec := keyRequest(router, "GET", "/api/build/1", token, nil); rec.Code != http.StatusOK {
		t.Errorf("key on deploy's build: %d, want 200", rec.Code)
	}
	if rec := keyRequest(router, "GET", "/api/build/2", token, nil); rec.Code != http.StatusForbidden {
		t.Errorf("key on nightly's build: %d, want 403", rec.Code)
	}
	if ids := buildIDs(t, keyRequest(router, "GET", "/api/feed", token, nil).Body.Bytes()); !slices.Equal(ids, []int{1}) {
		t.Errorf("key's feed = %v, want only build 1", ids)
	}

	// When the group loses the job, so does the key
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"nightly"}})
	if rec := keyRequest(router, "GET", "/api/build/1", token, nil); rec.Code != http.StatusForbidden {
		t.Errorf("key after the group lost deploy: %d, want 403", rec.Code)
	}

	// A new login records the groups the person is in now
	if err := refreshKeyCreatorGroups("SAM@example.com", []string{"Elsewhere"}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rec := keyRequest(router, "GET", "/api/build/2", token, nil); rec.Code != http.StatusForbidden {
		t.Errorf("key after its creator left the group: %d, want 403", rec.Code)
	}
}

func TestAPIKeyAccessWithoutCreatorGroups(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})
	// Keys made with the password, or by a person before wakeci recorded
	// the creator's groups, are limited by their own grant only
	admin := createTestKey(t, url.Values{"name": {"admin-ci"}, "scope": {"read"}, "allJobs": {"true"}})
	key, err := apiKeyFromForm(url.Values{"name": {"old"}, "scope": {"read"}, "allJobs": {"true"}, "expiresInDays": {"30"}}, time.Now())
	if err != nil {
		t.Fatalf("apiKeyFromForm: %v", err)
	}
	key.CreatedBy = "sam@example.com"
	old, err := saveNewAPIKey(key)
	if err != nil {
		t.Fatalf("save key: %v", err)
	}
	for name, token := range map[string]string{"admin": admin, "old": old} {
		if rec := keyRequest(router, "GET", "/api/build/2", token, nil); rec.Code != http.StatusOK {
			t.Errorf("%s key: %d, want 200", name, rec.Code)
		}
	}
	// Until its creator logs in again
	if err := refreshKeyCreatorGroups("sam@example.com", []string{"Service Team"}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rec := keyRequest(router, "GET", "/api/build/2", old, nil); rec.Code != http.StatusForbidden {
		t.Errorf("old key after its creator logged in: %d, want 403", rec.Code)
	}
	if rec := keyRequest(router, "GET", "/api/build/2", admin, nil); rec.Code != http.StatusOK {
		t.Errorf("admin key after sam logged in: %d, want 200", rec.Code)
	}
}
