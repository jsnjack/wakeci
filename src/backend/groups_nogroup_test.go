package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

// ssoLoginWithGroups signs testSSOEmail in with a signed response that
// lists groups, and returns the session cookie, nil if the login was refused
func ssoLoginWithGroups(t *testing.T, groups []string) *http.Cookie {
	t.Helper()
	_, stateCookie := startSSOLogin(t, testLoginRedirect)
	response := newTestSAMLResponse(openTestLoginState(t, stateCookie).RequestID)
	response.groups = groups
	req := httptest.NewRequest(http.MethodPost, SSOACSPath, strings.NewReader(url.Values{"SAMLResponse": {response.encode(t)}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	HandleSSOACS(rec, req)
	return findCookie(rec.Result().Cookies(), "session")
}

// newNoGroupTestEnv is the single sign-on test env with the admin password
// "admin", which the rule's routes need
func newNoGroupTestEnv(t *testing.T) {
	t.Helper()
	newSSOTestEnv(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GlobalBucket).Put([]byte("password"), hash)
	}); err != nil {
		t.Fatal(err)
	}
}

func listGroups(t *testing.T) SSOGroupsData {
	t.Helper()
	rec := adminRequest("GET", "/api/groups/", nil)
	var data SSOGroupsData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode groups %d %s: %v", rec.Code, rec.Body.String(), err)
	}
	return data
}

// People in none of the groups sign in only while the admin password allows
// it, and then get the rule's permissions; members of a group get only
// their groups'
func TestSSONoGroupRule(t *testing.T) {
	newNoGroupTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRun}, Jobs: []string{"deploy"}})

	if ssoLoginWithGroups(t, []string{"Sales"}) != nil || ssoLoginWithGroups(t, nil) != nil {
		t.Fatal("people in no group signed in before the rule allowed it")
	}
	if listGroups(t).NoGroup != nil {
		t.Error("the list has a rule before one was set")
	}

	readAll := url.Values{"scope": {"read"}, "allJobs": {"true"}}
	if rec := apiCall("POST", "/api/nogroup", loginInGroups(t, "dev@example.com", "Developers"), readAll, nil); rec.Code != http.StatusForbidden {
		t.Errorf("a single sign-on user set the rule: %d", rec.Code)
	}
	if rec := adminRequest("POST", "/api/nogroup", url.Values{"scope": {"write"}, "job": {"deploy"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("write on some jobs: %d, want 400", rec.Code)
	}
	if rec := adminRequest("POST", "/api/nogroup", readAll); rec.Code != http.StatusOK {
		t.Fatalf("set the rule: %d %s", rec.Code, rec.Body.String())
	}

	var cookie *http.Cookie
	for _, groups := range [][]string{{"Sales"}, nil} {
		if cookie = ssoLoginWithGroups(t, groups); cookie == nil {
			t.Fatalf("groups %q: refused although the rule allows people in no group", groups)
		}
		access, err := ssoGroupsAccess(groups)
		if err != nil {
			t.Fatal(err)
		}
		if !access.Allows(ScopeRead, "nightly") || access.HasScope(ScopeRun) {
			t.Errorf("groups %q: want the rule's read on all jobs and nothing more", groups)
		}
	}
	if rec := apiCall("GET", "/api/jobs/", cookie, nil, nil); rec.Code != http.StatusOK {
		t.Errorf("a person in no group reads jobs: %d", rec.Code)
	}
	if rec := apiCall("POST", "/api/job/deploy/run", cookie, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("a person in no group runs a job: %d, want 403", rec.Code)
	}

	// A member of a group gets only the group's permissions, even narrower
	access, err := ssoGroupsAccess([]string{"Sales", "service team"})
	if err != nil {
		t.Fatal(err)
	}
	if access.HasScope(ScopeRead) || !access.Allows(ScopeRun, "deploy") {
		t.Error("a member of Service Team got the rule's permissions or lost the group's")
	}

	// The list shows the rule and who it covers now: Alice, not the developer
	data := listGroups(t)
	if data.NoGroup == nil || !data.NoGroup.AllJobs || data.NoGroup.SignedIn != 1 || data.NoGroup.Name != "" {
		t.Errorf("rule in the list = %+v, want read on all jobs with 1 signed in", data.NoGroup)
	}

	// The keys of people in no group follow the rule
	key := &APIKey{Grant: Grant{Scopes: []string{ScopeRead}, AllJobs: true}, CreatedBy: testSSOEmail, CreatorGroups: []string{"Sales"}}
	if keyAccess, err := apiKeyAccess(key); err != nil || !keyAccess.HasScope(ScopeRead) {
		t.Errorf("a key of a person in no group: %v, want read", err)
	}

	if rec := adminRequest("DELETE", "/api/nogroup", nil); rec.Code != http.StatusOK {
		t.Fatalf("turn the rule off: %d", rec.Code)
	}
	if listGroups(t).NoGroup != nil {
		t.Error("the rule is still listed")
	}
	if rec := apiCall("GET", "/api/jobs/", cookie, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("after the rule was turned off: %d, want 403", rec.Code)
	}
	if keyAccess, err := apiKeyAccess(key); err != nil || keyAccess.HasScope(ScopeRead) {
		t.Errorf("a key of a person in no group after the rule went: %v, want nothing", err)
	}
	if ssoLoginWithGroups(t, []string{"Sales"}) != nil {
		t.Error("people in no group signed in after the rule was turned off")
	}
	if rec := adminRequest("DELETE", "/api/nogroup", nil); rec.Code != http.StatusOK {
		t.Errorf("turning it off again: %d, want 200", rec.Code)
	}
}

// Setting and removing the rule is in the audit log, with its permissions
func TestSSONoGroupRuleAudit(t *testing.T) {
	newNoGroupTestEnv(t)
	events, _ := captureAudit(t)
	adminRequest("POST", "/api/nogroup", url.Values{"scope": {"read", "run"}, "job": {"deploy"}, "pattern": {"test_*"}})
	adminRequest("DELETE", "/api/nogroup", nil)
	got := events()
	if len(got) != 2 {
		t.Fatalf("audit events = %+v, want 2", got)
	}
	if got[0]["event"] != "group.nogroup.set" || got[0]["scopes"] != "read,run" || got[0]["jobs"] != `["deploy"]` || got[0]["outcome"] != "ok" {
		t.Errorf("set = %+v", got[0])
	}
	if got[1]["event"] != "group.nogroup.delete" || got[1]["outcome"] != "ok" {
		t.Errorf("delete = %+v", got[1])
	}
}
