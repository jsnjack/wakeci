package main

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// runAs starts job with form as the admin password, failing the test unless
// it gets status, and returns the new build's ID (0 if refused)
func runAs(t *testing.T, cookie *http.Cookie, job string, form url.Values, status int) (int, string) {
	t.Helper()
	prepare := func(r *http.Request) { r.SetBasicAuth("", "admin") }
	if cookie != nil {
		prepare = nil
	}
	rec := apiCall("POST", "/api/job/"+job+"/run", cookie, form, prepare)
	if rec.Code != status {
		t.Fatalf("run %s with %v: %d %q, want %d", job, form, rec.Code, rec.Body.String(), status)
	}
	if status != http.StatusOK {
		return 0, strings.TrimSpace(rec.Body.String())
	}
	id, err := strconv.Atoi(strings.TrimSpace(rec.Body.String()))
	if err != nil {
		t.Fatalf("run %s: %q isn't a build ID", job, rec.Body.String())
	}
	return id, ""
}

func groupEnv(t *testing.T) {
	t.Helper()
	newAPIKeyTestEnv(t)
	oldQueue := GlobalQueue
	// No concurrent builds: the builds stay queued, so they count as parents
	GlobalQueue = &Queue{}
	t.Cleanup(func() { GlobalQueue = oldQueue })
	// Past the builds the test environment adds
	if err := DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GlobalBucket).Put([]byte("count"), []byte("10"))
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRunUnderAParent(t *testing.T) {
	groupEnv(t)
	events, _ := captureAudit(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	child, _ := runAs(t, nil, "nightly", url.Values{parentParam: {strconv.Itoa(root)}}, http.StatusOK)
	grandchild, _ := runAs(t, nil, "deploy", url.Values{parentParam: {strconv.Itoa(child)}}, http.StatusOK)

	for _, c := range []struct {
		id   int
		want buildGroup
	}{{root, buildGroup{}}, {child, buildGroup{root, root}}, {grandchild, buildGroup{child, root}}} {
		if got := GlobalQueue.Live(c.id).Group(); got != c.want {
			t.Errorf("build %d: group %+v, want %+v", c.id, got, c.want)
		}
		saved, err := readBuildStatus(c.id)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Parent != c.want.Parent || saved.Root != c.want.Root {
			t.Errorf("history of build %d: parent %d root %d, want %+v", c.id, saved.Parent, saved.Root, c.want)
		}
	}
	_ = DB.View(func(tx *bolt.Tx) error {
		if got := groupMembers(tx, root); !slices.Equal(got, []int{child, grandchild}) {
			t.Errorf("members of %d: %v, want [%d %d]", root, got, child, grandchild)
		}
		return nil
	})

	if env := GlobalQueue.Live(grandchild).generateDefaultEnvVariables(); !slices.Contains(env, "WAKE_PARENT_ID="+strconv.Itoa(child)) {
		t.Errorf("grandchild's environment lacks WAKE_PARENT_ID=%d: %q", child, env)
	}
	for _, v := range GlobalQueue.Live(root).generateDefaultEnvVariables() {
		if strings.HasPrefix(v, "WAKE_PARENT_ID=") {
			t.Errorf("a build no build started has %s", v)
		}
	}

	var parents []string
	for _, event := range events() {
		if event["event"] == "build.run" {
			parents = append(parents, event["parent"])
		}
	}
	// A run without parent has no parent attribute
	if want := []string{"", strconv.Itoa(root), strconv.Itoa(child)}; !slices.Equal(parents, want) {
		t.Errorf("audit parents %q, want %q", parents, want)
	}
}

func TestRunUnderAParentRefusesWhatCantBeAParent(t *testing.T) {
	groupEnv(t)
	live, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	nightly, _ := runAs(t, nil, "nightly", nil, http.StatusOK)
	addTestGroup(t, "Deployers", Grant{Scopes: []string{ScopeRead, ScopeRun}, Jobs: []string{"deploy"}})
	deployer := loginInGroups(t, "dana@example.com", "Deployers")

	cases := []struct {
		name   string
		cookie *http.Cookie
		form   url.Values
		want   string
	}{
		{"not a number", nil, url.Values{parentParam: {"abc"}}, "WAKE_PARENT must be a build ID"},
		{"zero", nil, url.Values{parentParam: {"0"}}, "WAKE_PARENT must be a build ID"},
		{"no such build", nil, url.Values{parentParam: {"999"}}, "there is no build 999"},
		// Build 1 is in the history, finished
		{"ended", nil, url.Values{parentParam: {"1"}}, "build 1 has ended"},
		{"both", nil, url.Values{parentParam: {strconv.Itoa(live)}, againParam: {"1"}}, "not both"},
		// The deployer may not read nightly: as if it didn't exist
		{"unreadable", deployer, url.Values{parentParam: {strconv.Itoa(nightly)}}, "there is no build " + strconv.Itoa(nightly)},
		{"unreadable and ended", deployer, url.Values{parentParam: {"2"}}, "there is no build 2"},
	}
	queued := len(GlobalQueue.queued)
	for _, c := range cases {
		_, body := runAs(t, c.cookie, "deploy", c.form, http.StatusBadRequest)
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: %q, want it to say %q", c.name, body, c.want)
		}
	}
	if len(GlobalQueue.queued) != queued {
		t.Errorf("refused runs queued %d builds", len(GlobalQueue.queued)-queued)
	}
	// The deployer may start deploy under a deploy build
	runAs(t, deployer, "deploy", url.Values{parentParam: {strconv.Itoa(live)}}, http.StatusOK)
}

// Run again keeps the parent, after the parent ended, but only for a build
// of the same job
func TestRunAgainKeepsTheParent(t *testing.T) {
	groupEnv(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	child, _ := runAs(t, nil, "nightly", url.Values{parentParam: {strconv.Itoa(root)}}, http.StatusOK)
	GlobalQueue.Remove(root)

	again, _ := runAs(t, nil, "nightly", url.Values{againParam: {strconv.Itoa(child)}}, http.StatusOK)
	if got := GlobalQueue.Live(again).Group(); got != (buildGroup{root, root}) {
		t.Errorf("run again of %d: group %+v, want parent and root %d", child, got, root)
	}
	if _, body := runAs(t, nil, "deploy", url.Values{againParam: {strconv.Itoa(child)}}, http.StatusBadRequest); !strings.Contains(body, "not of deploy") {
		t.Errorf("run again as another job: %q", body)
	}
	if _, body := runAs(t, nil, "deploy", url.Values{againParam: {"999"}}, http.StatusBadRequest); !strings.Contains(body, "there is no build 999") {
		t.Errorf("run again of a build that doesn't exist: %q", body)
	}
	// A build no build started runs again on its own
	alone, _ := runAs(t, nil, "deploy", url.Values{againParam: {"1"}}, http.StatusOK)
	if got := GlobalQueue.Live(alone).Group(); got != (buildGroup{}) {
		t.Errorf("run again of a build without parent: group %+v", got)
	}
}

func TestQueuedBuildKeepsItsGroupOverARestart(t *testing.T) {
	groupEnv(t)
	root, _ := runAs(t, nil, "deploy", nil, http.StatusOK)
	child, _ := runAs(t, nil, "nightly", url.Values{parentParam: {strconv.Itoa(root)}}, http.StatusOK)
	var record []byte
	_ = DB.View(func(tx *bolt.Tx) error {
		record = slices.Clone(tx.Bucket(QueueBucket).Get(Itob(child)))
		return nil
	})
	restored, err := restoreBuild(child, record)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Group(); got != (buildGroup{root, root}) {
		t.Errorf("restored group %+v, want parent and root %d", got, root)
	}
}

func TestGroupForgetsDeletedMembers(t *testing.T) {
	newTestBuildEnv(t)
	err := DB.Update(func(tx *bolt.Tx) error {
		for _, id := range []int{6, 7} {
			if err := putGroupMember(tx, 5, id); err != nil {
				return err
			}
		}
		if err := deleteGroupMember(tx, 5, 6); err != nil {
			return err
		}
		if got := groupMembers(tx, 5); !slices.Equal(got, []int{7}) {
			t.Errorf("members after deleting 6: %v", got)
		}
		if err := deleteGroupMember(tx, 5, 7); err != nil {
			return err
		}
		if tx.Bucket(GroupsBucket).Bucket(Itob(5)) != nil {
			t.Error("the group outlived its last member")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
