package main

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	bolt "go.etcd.io/bbolt"
)

func TestJobFilePermissions(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "new job", mode: 0},
		{name: "existing private job", mode: 0600},
		{name: "existing group-readable job", mode: 0640},
		{name: "existing world-readable job", mode: 0644},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "job.yaml")
			want := tt.mode
			if tt.mode == 0 {
				want = 0600
			} else {
				if err := os.WriteFile(path, []byte("tasks: []\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, tt.mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeJobFile(path, []byte("tasks: []\n"), tt.mode == 0); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != want {
				t.Errorf("job mode = %04o, want %04o", got, want)
			}
		})
	}
}

func TestDisabledJobsSurviveTransientFileChanges(t *testing.T) {
	for _, phase := range []string{"watcher empty", "cleanup empty", "watcher removed", "watcher renamed", "stale rename", "application save"} {
		t.Run(phase, func(t *testing.T) {
			newJobAccessTestEnv(t)
			file := Config.JobDir + "disabled.yaml"
			content := []byte("tasks: []\ninterval: '@every 1h'\n")
			if err := os.WriteFile(file, content, 0640); err != nil {
				t.Fatal(err)
			}
			if err := RegisterJob(file); err != nil {
				t.Fatal(err)
			}
			// Include disabled jobs created before disabled-state persistence existed.
			if err := DB.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(JobsBucket).Bucket([]byte("disabled")).Put([]byte("active"), []byte("false"))
			}); err != nil {
				t.Fatal(err)
			}
			_, key := mustCreateAPIKey(t, passwordPrincipal(viaSession), APIKeyScopes, 0, time.Now())
			switch phase {
			case "watcher empty", "cleanup empty":
				if err := os.WriteFile(file, nil, 0640); err != nil {
					t.Fatal(err)
				}
				if phase == "cleanup empty" {
					CleanupJobsBucket()
				} else {
					handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Write}, Config.jobsExt)
				}
			case "watcher removed":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Remove}, Config.jobsExt)
			case "watcher renamed":
				if err := os.Rename(file, file+".bak"); err != nil {
					t.Fatal(err)
				}
				handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Rename}, Config.jobsExt)
			case "stale rename":
				handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Rename}, Config.jobsExt)
			case "application save":
				response := serveWithKey(key, http.MethodPost, "/api/job/disabled", url.Values{"fileContent": {string(content)}})
				if response.Code != http.StatusOK {
					t.Fatalf("save: %d %s", response.Code, response.Body.String())
				}
			}
			if phase != "stale rename" && phase != "application save" {
				response := serveWithKey(key, http.MethodGet, "/api/job/disabled", nil)
				if response.Code != http.StatusNotFound {
					t.Fatalf("invalid current file accessible: %d", response.Code)
				}
			}
			if err := os.WriteFile(file, content, 0640); err != nil {
				t.Fatal(err)
			}
			handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Create}, Config.jobsExt)
			assertActive := func(want string) {
				t.Helper()
				if err := DB.View(func(tx *bolt.Tx) error {
					job := tx.Bucket(JobsBucket).Bucket([]byte("disabled"))
					if job == nil {
						t.Fatal("restored job is not registered")
					}
					if got := string(job.Get([]byte("active"))); got != want {
						t.Fatalf("active = %q, want %q", got, want)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			assertActive("false")
			if _, err := RunJob("disabled", nil); err == nil {
				t.Fatal("disabled job was allowed to run")
			}
			// An explicit enable must clear the preserved state as well.
			response := serveWithKey(key, http.MethodPost, "/api/job/disabled/set_active", url.Values{"active": {"true"}})
			if response.Code != http.StatusOK {
				t.Fatalf("enable: %d %s", response.Code, response.Body.String())
			}
			if err := os.WriteFile(file, nil, 0640); err != nil {
				t.Fatal(err)
			}
			CleanupJobsBucket()
			if err := os.WriteFile(file, content, 0640); err != nil {
				t.Fatal(err)
			}
			if err := RegisterJob(file); err != nil {
				t.Fatal(err)
			}
			assertActive("true")
		})
	}
}

func TestJobSavesReplaceFilesAtomically(t *testing.T) {
	newJobAccessTestEnv(t)
	path := Config.JobDir + "saved.yaml"
	original := "tasks: []\ndesc: original\n"
	replacement := "tasks: []\ndesc: replacement\n"
	if err := os.WriteFile(path, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	if err := RegisterJob(path); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	_, key := mustCreateAPIKey(t, passwordPrincipal(viaSession), APIKeyScopes, 0, time.Now())
	response := serveWithKey(key, http.MethodPost, "/api/job/saved", url.Values{"fileContent": {replacement}})
	if response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	old, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != original {
		t.Fatalf("save modified the file already open by a reader: %q", old)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != replacement {
		t.Fatalf("replacement = %q, error %v", current, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Errorf("mode = %o, want 640", info.Mode().Perm())
	}
}

func TestJobCreationDoesNotReplaceExistingFile(t *testing.T) {
	newJobAccessTestEnv(t)
	path := Config.JobDir + "existing.yaml"
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeJobFile(path, []byte("replacement"), true); !errors.Is(err, os.ErrExist) {
		t.Fatalf("create returned %v, want existing-file error", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatalf("existing file changed: %q %v", data, err)
	}
}

func TestExplicitJobDeletionAllowsFreshCreation(t *testing.T) {
	for _, order := range []string{"watcher before recreate", "watcher after recreate"} {
		t.Run(order, func(t *testing.T) {
			newJobAccessTestEnv(t)
			file := Config.JobDir + "replace.yaml"
			if err := os.WriteFile(file, []byte("tasks: []\ninterval: '@every 1h'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := RegisterJob(file); err != nil {
				t.Fatal(err)
			}
			_, key := mustCreateAPIKey(t, passwordPrincipal(viaSession), APIKeyScopes, 0, time.Now())
			response := serveWithKey(key, http.MethodPost, "/api/job/replace/set_active", url.Values{"active": {"false"}})
			if response.Code != http.StatusOK {
				t.Fatalf("disable: %d %s", response.Code, response.Body.String())
			}
			response = serveWithKey(key, http.MethodDelete, "/api/job/replace", nil)
			if response.Code != http.StatusOK {
				t.Fatalf("delete: %d %s", response.Code, response.Body.String())
			}
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Fatalf("job file remains after delete: %v", err)
			}
			if err := DB.View(func(tx *bolt.Tx) error {
				if tx.Bucket(JobsBucket).Bucket([]byte("replace")) != nil {
					t.Error("deleted registration remains")
				}
				if disabled := tx.Bucket(disabledJobsBucket); disabled != nil && disabled.Get([]byte("replace")) != nil {
					t.Error("explicit deletion retained disabled state")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if order == "watcher before recreate" {
				handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Remove}, Config.jobsExt)
			}
			response = serveWithKey(key, http.MethodPost, "/api/jobs/create", url.Values{"name": {"replace"}})
			if response.Code != http.StatusOK {
				t.Fatalf("recreate: %d %s", response.Code, response.Body.String())
			}
			if order == "watcher after recreate" {
				handleJobFileEvent(fsnotify.Event{Name: file, Op: fsnotify.Remove}, Config.jobsExt)
			}
			if err := DB.View(func(tx *bolt.Tx) error {
				job := tx.Bucket(JobsBucket).Bucket([]byte("replace"))
				if job == nil {
					t.Fatal("new job registration is missing")
				}
				if active := string(job.Get([]byte("active"))); active != "true" {
					t.Errorf("fresh job active = %q, want true", active)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
