package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

// TestConfigAndSecretsAreNotJobs: with the documented `jobdir: ./`, the
// config and secrets files sit among the job files, and so may other YAML.
// No key may list, read, overwrite, delete or run anything but a real job,
// also not through a link.
func TestConfigAndSecretsAreNotJobs(t *testing.T) {
	newAuthTestEnv(t)
	if err := DB.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{JobsBucket, HistoryBucket} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("create buckets: %v", err)
	}
	originalCron := GlobalCron
	GlobalCron = cron.New()
	t.Cleanup(func() { GlobalCron = originalCron })

	dir := t.TempDir()
	files := map[string]string{
		"Wakefile.yaml": fmt.Sprintf("workdir: %s/workdir\njobdir: %s\nsecretsfile: %s/secrets.yaml\ngoogle:\n  client_id: id\n  client_secret: CLIENT-SECRET\n  allowed_domains: [example.com]\n", dir, dir, dir),
		"secrets.yaml":  "db_password: SECRET-VALUE\n",
		"build.yaml":    "desc: a real job\ntasks:\n  - name: hello\n    run: echo hello\n",
		// Other YAML that happens to live in jobdir
		"compose.yaml":   "services:\n  db:\n    environment:\n      PASSWORD: SECRET-VALUE\n",
		"inventory.yaml": "- host: SECRET-VALUE\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "secrets.yaml"), filepath.Join(dir, "leak.yaml")); err != nil {
		t.Fatal(err)
	}
	config, err := CreateWakeConfig(filepath.Join(dir, "Wakefile.yaml"))
	if err != nil {
		t.Fatalf("CreateWakeConfig: %v", err)
	}
	Config = config
	// As registered by a version that didn't protect it
	if err := DB.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"secrets", "compose", "inventory"} {
			jb, err := tx.Bucket(JobsBucket).CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return err
			}
			if err := jb.Put([]byte("defaultParams"), []byte("[]")); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	CleanupJobsBucket()
	if err := DB.View(func(tx *bolt.Tx) error {
		for _, name := range []string{"secrets", "compose", "inventory"} {
			if tx.Bucket(JobsBucket).Bucket([]byte(name)) != nil {
				t.Errorf("startup kept stale registration %q", name)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ScanAllJobs()

	_, readKey := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
	_, allKey := mustCreateAPIKey(t, passwordPrincipal(viaSession), APIKeyScopes, 0, time.Now())

	rec := serveWithKey(readKey, http.MethodGet, "/api/jobs/", nil)
	var jobs []JobsListData
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("decode jobs (status %d): %v", rec.Code, err)
	}
	var names []string
	for _, job := range jobs {
		names = append(names, job.Name)
	}
	if !slices.Equal(names, []string{"build"}) {
		t.Errorf("jobs list = %q, want only the real job", names)
	}

	for _, name := range []string{"Wakefile", "secrets", "leak", "compose", "inventory"} {
		requests := []struct {
			token, method, path string
			form                url.Values
		}{
			{readKey, http.MethodGet, "/api/job/" + name, nil},
			{allKey, http.MethodPost, "/api/job/" + name, url.Values{"fileContent": {"tasks: []\n"}}},
			{allKey, http.MethodPost, "/api/job/" + name + "/run", nil},
			{allKey, http.MethodPost, "/api/job/" + name + "/set_active", url.Values{"active": {"false"}}},
			{allKey, http.MethodDelete, "/api/job/" + name, nil},
		}
		for _, rq := range requests {
			rec := serveWithKey(rq.token, rq.method, rq.path, rq.form)
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SECRET") {
				t.Errorf("%s %s: status = %d body = %q, want 404", rq.method, rq.path, rec.Code, rec.Body.String())
			}
		}
	}
	for name, content := range files {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != content {
			t.Errorf("%s changed or is gone: %v", name, err)
		}
	}
}

func TestJobFileChangesInvalidateRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		registered    bool
	}{
		{"non-job mapping", "services:\n  db:\n    password: PRIVATE-CREDENTIAL\n", false},
		{"non-job sequence", "- password: PRIVATE-CREDENTIAL\n", false},
		{"transient parse failure", "tasks: [", true},
		{"empty job", "tasks: []\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newJobAccessTestEnv(t)
			file := Config.JobDir + "changing.yaml"
			valid := "tasks: []\ninterval: '@every 1h'\n"
			if err := os.WriteFile(file, []byte(valid), 0600); err != nil {
				t.Fatal(err)
			}
			if err := RegisterJob(file); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			_, readKey := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
			if tc.name != "empty job" {
				rec := serveWithKey(readKey, http.MethodGet, "/api/job/changing", nil)
				if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "PRIVATE-CREDENTIAL") {
					t.Errorf("before watcher: status %d body %q; want 404", rec.Code, rec.Body.String())
				}
			}
			handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Write}, Config.jobsExt)
			registered := false
			if err := DB.View(func(tx *bolt.Tx) error {
				registered = tx.Bucket(JobsBucket).Bucket([]byte("changing")) != nil
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if registered != tc.registered {
				t.Errorf("registered = %t, want %t", registered, tc.registered)
			}
			scheduled := false
			for _, entry := range GlobalCron.Entries() {
				if job, ok := entry.Job.(*Job); ok && job.Name == "changing" {
					scheduled = true
				}
			}
			if tc.name != "empty job" && scheduled != tc.registered {
				t.Errorf("scheduled = %t, want %t", scheduled, tc.registered)
			}
			if !tc.registered {
				rec := serveWithKey(readKey, http.MethodGet, "/api/job/changing", nil)
				if rec.Code != http.StatusNotFound {
					t.Errorf("after watcher: status %d, want 404", rec.Code)
				}
			}
			if err := os.WriteFile(file, []byte(valid), 0600); err != nil {
				t.Fatal(err)
			}
			handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Write}, Config.jobsExt)
			if rec := serveWithKey(readKey, http.MethodGet, "/api/job/changing", nil); rec.Code != http.StatusOK {
				t.Errorf("recovered job: status %d", rec.Code)
			}
		})
	}
}
