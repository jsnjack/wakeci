package main

import (
	"net/http"
	"net/url"
	"testing"
)

// Someone who may run jobs but not edit them gets the rules of API keys for
// run parameters: no secret references, quotes, line breaks or NUL, and
// their builds' build.env values aren't expanded. Whoever may edit jobs,
// and so could make a job do anything, keeps the old rules.
func TestRunParametersOfPeopleWhoCantEditJobs(t *testing.T) {
	newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	// No concurrent builds, so the builds stay queued where the test sees them
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	addTestGroup(t, "Runners", Grant{Scopes: []string{ScopeRead, ScopeRun}, Jobs: []string{"deploy"}})
	addTestGroup(t, "Editors", Grant{Scopes: []string{ScopeRead, ScopeRun, ScopeWrite}, AllJobs: true})
	runner := loginInGroups(t, "rita@example.com", "Runners")
	editor := loginInGroups(t, "ed@example.com", "Editors")
	admin := func(r *http.Request) { r.SetBasicAuth("", "admin") }

	for _, target := range []string{"{{ secrets.TOKEN }}", "x{{secrets.TOKEN}}", "x\nBASH_ENV=/tmp/x", `"a" BASH_ENV="/tmp/evil"`, "x\x00y"} {
		if rec := apiCall("POST", "/api/job/deploy/run", runner, url.Values{"TARGET": {target}}, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("run-only member with TARGET=%q: %d, want 400", target, rec.Code)
		}
	}
	if rec := apiCall("POST", "/api/job/deploy/run?TARGET=%7B%7Bsecrets.TOKEN%7D%7D", runner, nil, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("run-only member with a secret in the query: %d, want 400", rec.Code)
	}
	if len(GlobalQueue.queued) != 0 {
		t.Fatalf("refused runs queued %d builds", len(GlobalQueue.queued))
	}

	runs := []struct {
		name      string
		cookie    *http.Cookie
		prepare   func(*http.Request)
		target    string
		untrusted bool
	}{
		{"run-only member", runner, nil, "production", true},
		{"member who may edit jobs", editor, nil, "{{ secrets.TOKEN }}", false},
		{"admin password", nil, admin, "x\ny", false},
	}
	for i, run := range runs {
		rec := apiCall("POST", "/api/job/deploy/run", run.cookie, url.Values{"TARGET": {run.target}}, run.prepare)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %q", run.name, rec.Code, rec.Body.String())
		}
		if got := GlobalQueue.queued[i].untrustedParams; got != run.untrusted {
			t.Errorf("%s: untrustedParams = %t, want %t", run.name, got, run.untrusted)
		}
	}
}
