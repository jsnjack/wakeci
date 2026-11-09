package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeBinary is a release binary that prints version for --version
func fakeBinary(version string) []byte {
	return []byte("#!/bin/sh\necho 'wakeci " + version + "'\n")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fakeGitHub serves a latest release with binary as its wakeci asset, and
// points githubAPIURL at itself
func fakeGitHub(t *testing.T, tag string, binary []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	mux.HandleFunc("/repos/"+releaseRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintf(w, `{"tag_name": %q, "html_url": %q, "assets": [{"name": "wakeci", "browser_download_url": %q, "digest": "sha256:%s"}]}`,
			tag, releasePagePrefix+"tag/"+tag, server.URL+"/download/wakeci", sha256Hex(binary)); err != nil {
			t.Errorf("serve release: %v", err)
		}
	})
	mux.HandleFunc("/download/wakeci", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(binary); err != nil {
			t.Errorf("serve binary: %v", err)
		}
	})
	t.Cleanup(server.Close)
	useGitHub(t, server.URL)
	return server
}

// useGitHub points githubAPIURL at url and forgets the cached release, also
// after the test
func useGitHub(t *testing.T, url string) {
	t.Helper()
	oldAPI := githubAPIURL
	githubAPIURL = url
	releaseCache.release = nil
	t.Cleanup(func() {
		githubAPIURL = oldAPI
		releaseCache.release = nil
	})
}

// runningBinary creates the binary an update replaces
func runningBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wakeci")
	if err := os.WriteFile(path, []byte("old"), 0755); err != nil {
		t.Fatalf("write running binary: %v", err)
	}
	// Not left to the umask, so TestDownloadRelease can check the mode
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatalf("chmod running binary: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		version, current string
		want, wantErr    bool
	}{
		{"2.40.0", "2.39.14", true, false},
		{"2.39.14", "2.39.14", false, false},
		{"2.39.9", "2.39.14", false, false},
		{"10.0.0", "9.99.99", true, false},
		{"2.40.0", "dev", false, true},
		{"2.40", "2.39.0", false, true},
		{"2.40.x", "2.39.0", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.version+" vs "+tt.current, func(t *testing.T) {
			got, err := isNewerVersion(tt.version, tt.current)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("newer = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLatestRelease(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    *release
		wantErr string
	}{
		{"release", http.StatusOK,
			`{"tag_name": "v2.40.1", "html_url": "https://github.com/jsnjack/wakeci/releases/tag/v2.40.1", "assets": [{"name": "other"}, {"name": "wakeci", "browser_download_url": "https://x/wakeci", "digest": "sha256:abc"}]}`,
			&release{Version: "2.40.1", Page: "https://github.com/jsnjack/wakeci/releases/tag/v2.40.1", URL: "https://x/wakeci", SHA256: "abc"}, ""},
		{"release page elsewhere", http.StatusOK,
			`{"tag_name": "v2.40.1", "html_url": "javascript:alert(1)", "assets": [{"name": "wakeci", "browser_download_url": "https://x/wakeci", "digest": "sha256:abc"}]}`,
			&release{Version: "2.40.1", URL: "https://x/wakeci", SHA256: "abc"}, ""},
		{"no digest", http.StatusOK,
			`{"tag_name": "v2.40.1", "assets": [{"name": "wakeci", "browser_download_url": "https://x/wakeci"}]}`,
			nil, "no sha256 digest"},
		{"no binary", http.StatusOK, `{"tag_name": "v2.40.1", "assets": []}`, nil, "no wakeci binary"},
		{"not a version", http.StatusOK, `{"tag_name": "nightly", "assets": []}`, nil, "not major.minor.patch"},
		{"rate limited", http.StatusForbidden, `{}`, nil, "403"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/jsnjack/wakeci/releases/latest" {
					t.Errorf("path = %q", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				if _, err := fmt.Fprint(w, tt.body); err != nil {
					t.Errorf("serve release: %v", err)
				}
			}))
			defer server.Close()
			oldAPI := githubAPIURL
			githubAPIURL = server.URL
			defer func() { githubAPIURL = oldAPI }()

			got, err := latestRelease(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("latestRelease: %v", err)
			}
			if *got != *tt.want {
				t.Errorf("release = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDownloadRelease(t *testing.T) {
	good := fakeBinary("2.0.0")
	tests := []struct {
		name    string
		served  []byte
		digest  string
		wantErr string
	}{
		{"checked binary", good, sha256Hex(good), ""},
		{"digest mismatch", good, sha256Hex([]byte("other")), "sha256"},
		{"other version", fakeBinary("1.0.0"), sha256Hex(fakeBinary("1.0.0")), "--version"},
		{"doesn't run", []byte("not a binary"), sha256Hex([]byte("not a binary")), "run release"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := w.Write(tt.served); err != nil {
					t.Errorf("serve binary: %v", err)
				}
			}))
			defer server.Close()
			running := runningBinary(t)
			rel := &release{Version: "2.0.0", URL: server.URL, SHA256: tt.digest}

			downloaded, err := downloadRelease(context.Background(), rel, running)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				entries, err := os.ReadDir(filepath.Dir(running))
				if err != nil {
					t.Fatalf("read dir: %v", err)
				}
				if len(entries) != 1 {
					t.Errorf("dir has %d files, want the failed download removed", len(entries))
				}
				return
			}
			if err != nil {
				t.Fatalf("downloadRelease: %v", err)
			}
			if filepath.Dir(downloaded) != filepath.Dir(running) {
				t.Errorf("downloaded to %s, want it next to %s", downloaded, running)
			}
			if got := readFile(t, downloaded); got != string(good) {
				t.Errorf("downloaded %q, want the served binary", got)
			}
			info, err := os.Stat(downloaded)
			if err != nil || info.Mode().Perm() != 0755 {
				t.Errorf("mode = %v, %v, want 0755 like the running binary", info.Mode(), err)
			}
		})
	}
}

func TestInstallRelease(t *testing.T) {
	running := runningBinary(t)
	for _, version := range []string{"new", "newer"} {
		previous := readFile(t, running)
		downloaded := filepath.Join(filepath.Dir(running), ".wakeci-update-"+version)
		if err := os.WriteFile(downloaded, []byte(version), 0755); err != nil {
			t.Fatalf("write download: %v", err)
		}
		if err := installRelease(downloaded, running); err != nil {
			t.Fatalf("installRelease: %v", err)
		}
		if got := readFile(t, running); got != version {
			t.Errorf("binary = %q, want %q", got, version)
		}
		if got := readFile(t, running+".previous"); got != previous {
			t.Errorf("previous binary = %q, want %q", got, previous)
		}
		if _, err := os.Stat(downloaded); !os.IsNotExist(err) {
			t.Errorf("download file still exists: %v", err)
		}
	}
}

// Without the previous binary there is nothing to roll back to, so the
// update must not replace the binary
func TestInstallReleaseNeedsThePreviousBinary(t *testing.T) {
	running := runningBinary(t)
	current := readFile(t, running)
	// The backup an earlier update kept is the only way back until a new one
	// exists
	if err := os.WriteFile(running+".previous", []byte("older"), 0755); err != nil {
		t.Fatalf("write previous binary: %v", err)
	}
	downloaded := filepath.Join(filepath.Dir(running), ".wakeci-update-new")
	if err := os.WriteFile(downloaded, []byte("new"), 0755); err != nil {
		t.Fatalf("write download: %v", err)
	}
	oldLink := linkBinary
	// As fs.protected_hardlinks does for a binary the service user doesn't own
	linkBinary = func(string, string) error { return os.ErrPermission }
	t.Cleanup(func() { linkBinary = oldLink })

	if err := installRelease(downloaded, running); err == nil {
		t.Fatal("installRelease succeeded without keeping the previous binary")
	}
	if got := readFile(t, running); got != current {
		t.Errorf("binary = %q, want it unchanged", got)
	}
	if got := readFile(t, running+".previous"); got != "older" {
		t.Errorf("previous binary = %q, want the earlier backup kept", got)
	}
	if _, err := os.Stat(downloaded); err != nil {
		t.Errorf("download file: %v, want it left for the caller to remove", err)
	}
}

func TestUpdateProblem(t *testing.T) {
	if platform := runtime.GOOS + "/" + runtime.GOARCH; platform != releasePlatform {
		t.Skipf("releases are built for %s, not %s", releasePlatform, platform)
	}
	running := runningBinary(t)
	if problem := updateProblem(running); problem != "" {
		t.Errorf("problem = %q, want none for a binary in a writable directory", problem)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}
	dir := filepath.Dir(running)
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("make dir read-only: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0755); err != nil {
			t.Errorf("make dir writable: %v", err)
		}
	})
	if problem := updateProblem(running); !strings.Contains(problem, "can't write to "+dir) {
		t.Errorf("problem = %q, want it to name the read-only directory", problem)
	}
}

// useUpdateEnv sets up a logged-in API with wakeci current running from a
// temporary binary
func useUpdateEnv(t *testing.T, current string) (running string, cookie *http.Cookie) {
	t.Helper()
	newSettingsTestEnv(t)
	Config.drainTimeout = time.Minute
	oldVersion, oldExecutable := Version, executablePath
	Version = current
	executablePath = runningBinary(t)
	t.Cleanup(func() { Version, executablePath = oldVersion, oldExecutable })
	return executablePath, loginAs(t, adminUser)
}

func TestHandleUpdateGet(t *testing.T) {
	tests := []struct {
		name    string
		current string
		want    UpdateData
	}{
		{"newer release", "1.0.0", UpdateData{Current: "1.0.0", Latest: "2.0.0", Available: true,
			ReleaseNotes: releasePagePrefix + "tag/v2.0.0"}},
		{"up to date", "2.0.0", UpdateData{Current: "2.0.0", Latest: "2.0.0", ReleaseNotes: releasePagePrefix + "tag/v2.0.0"}},
		{"development build", "dev", UpdateData{Current: "dev", Latest: "2.0.0", ReleaseNotes: releasePagePrefix + "tag/v2.0.0",
			Problem: `compare with the running version: version "dev" is not major.minor.patch`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cookie := useUpdateEnv(t, tt.current)
			fakeGitHub(t, "v2.0.0", fakeBinary("2.0.0"))
			rec := apiCall("GET", "/api/update/", cookie, nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}
			var got UpdateData
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.CheckedAt == nil || time.Since(*got.CheckedAt) > time.Minute {
				t.Errorf("checkedAt = %v, want the time of this check", got.CheckedAt)
			}
			got.CheckedAt = nil
			if got != tt.want {
				t.Errorf("update = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHandleUpdatePost(t *testing.T) {
	running, cookie := useUpdateEnv(t, "1.0.0")
	binary := fakeBinary("2.0.0")
	fakeGitHub(t, "v2.0.0", binary)
	update := func(version string) *httptest.ResponseRecorder {
		return apiCall("POST", "/api/update/", cookie, url.Values{"version": {version}}, nil)
	}

	if rec := update("1.5.0"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "latest release is 2.0.0") {
		t.Errorf("update to a version that isn't the latest: %d %q, want 409", rec.Code, rec.Body.String())
	}
	if got := readFile(t, running); got != "old" {
		t.Fatalf("binary = %q, want it unchanged after a refused update", got)
	}

	rec := update("2.0.0")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Installed 2.0.0") {
		t.Fatalf("update: %d %q, want 200", rec.Code, rec.Body.String())
	}
	if got := readFile(t, running); got != string(binary) {
		t.Errorf("binary = %q, want the release", got)
	}
	if got := readFile(t, running+".previous"); got != "old" {
		t.Errorf("previous binary = %q, want the old one", got)
	}
	status := GlobalQueue.Status()
	if !status.Stopping || status.RestartVersion != "2.0.0" {
		t.Errorf("queue status = %+v, want stopping to restart into 2.0.0", status)
	}

	if rec := update("2.0.0"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already stopping") {
		t.Errorf("second update: %d %q, want 409", rec.Code, rec.Body.String())
	}
}

// getUpdate calls GET /api/update/ and decodes the answer
func getUpdate(t *testing.T, cookie *http.Cookie) UpdateData {
	t.Helper()
	rec := apiCall("GET", "/api/update/", cookie, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var data UpdateData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return data
}

func TestHandleUpdateGetCachesRelease(t *testing.T) {
	_, cookie := useUpdateEnv(t, "1.0.0")
	checks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks++
		if _, err := fmt.Fprint(w, `{"tag_name": "v2.0.0", "assets": [{"name": "wakeci", "digest": "sha256:abc"}]}`); err != nil {
			t.Errorf("serve release: %v", err)
		}
	}))
	defer server.Close()
	useGitHub(t, server.URL)

	var checkedAt []time.Time
	for range 2 {
		data := getUpdate(t, cookie)
		if data.Latest != "2.0.0" || data.CheckedAt == nil {
			t.Fatalf("update = %+v, want 2.0.0 and when it was checked", data)
		}
		checkedAt = append(checkedAt, *data.CheckedAt)
	}
	if !checkedAt[0].Equal(checkedAt[1]) {
		t.Errorf("checkedAt = %v, want the time of the cached check", checkedAt)
	}
	if checks != 1 {
		t.Errorf("GitHub was asked %d times, want once within %v", checks, releaseCacheTTL)
	}
	releaseCache.checked = time.Now().Add(-releaseCacheTTL)
	getUpdate(t, cookie)
	if checks != 2 {
		t.Errorf("GitHub was asked %d times, want again once the cache expired", checks)
	}
}

// The settings page checks on every visit, so a failed check is part of the
// answer, not an error, and isn't cached
func TestHandleUpdateGetReportsFailedCheck(t *testing.T) {
	_, cookie := useUpdateEnv(t, "1.0.0")
	checks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	useGitHub(t, server.URL)

	for range 2 {
		data := getUpdate(t, cookie)
		if data.Current != "1.0.0" || data.Latest != "" || data.Available || !strings.Contains(data.Error, "403") {
			t.Errorf("update = %+v, want the running version and the failed check", data)
		}
	}
	if checks != 2 {
		t.Errorf("GitHub was asked %d times, want a failed check not cached", checks)
	}
}
