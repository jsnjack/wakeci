package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// releaseRepo is the GitHub repository wakeci updates itself from
const releaseRepo = "jsnjack/wakeci"

// releaseAsset is the name of the binary in a release
const releaseAsset = "wakeci"

// releasePagePrefix starts the page of every release of releaseRepo
const releasePagePrefix = "https://github.com/" + releaseRepo + "/releases/"

// releasePlatform is the platform the release binary is built for
const releasePlatform = "linux/amd64"

// releaseMaxSize limits the download of a release binary
const releaseMaxSize = 256 << 20

// releaseCheckTimeout limits the request for the latest release
const releaseCheckTimeout = 30 * time.Second

// releaseDownloadTimeout limits the download and check of a release binary
const releaseDownloadTimeout = 5 * time.Minute

// releaseCacheTTL is how long HandleUpdateGet reuses the latest release it
// found, so opening the settings page doesn't ask GitHub every time
const releaseCacheTTL = 10 * time.Minute

// githubAPIURL is the GitHub API that lists the releases
var githubAPIURL = "https://api.github.com"

// executablePath is the running binary, found at startup, before an update
// can replace it
var executablePath string

// updateMutex lets one update run at a time
var updateMutex sync.Mutex

// releaseCache holds the latest release HandleUpdateGet found
var releaseCache struct {
	mu      sync.Mutex
	release *release
	checked time.Time
}

// release is a wakeci release on GitHub
type release struct {
	Version string
	// Page is the release page with its notes, empty unless it is on
	// releaseRepo's GitHub pages
	Page string
	URL  string
	// SHA256 is the hex digest GitHub computed for the binary
	SHA256 string
}

// UpdateData tells whether a newer wakeci release can be installed
type UpdateData struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	// Available is true if Latest is newer than Current
	Available bool `json:"available"`
	// Problem says why wakeci can't replace its binary, if it can't
	Problem string `json:"problem,omitempty"`
	// Error says why the check for the latest release failed, if it did
	Error string `json:"error,omitempty"`
	// CheckedAt is when GitHub was asked, which may be up to
	// releaseCacheTTL ago
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
	// ReleaseNotes is the page of the latest release
	ReleaseNotes string `json:"releaseNotes,omitempty"`
}

// resolveExecutable returns the path of the running binary, without symlinks
func resolveExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return path, nil
}

// latestRelease asks GitHub for the latest wakeci release
func latestRelease(ctx context.Context) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		githubAPIURL+"/repos/"+releaseRepo+"/releases/latest", nil)
	if err != nil {
		return nil, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wakeci/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get latest release: %w", err)
	}
	defer closeBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get latest release: GitHub answered %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode latest release: %w", err)
	}
	rel := &release{Version: strings.TrimPrefix(payload.TagName, "v")}
	if _, err := parseVersion(rel.Version); err != nil {
		return nil, fmt.Errorf("latest release: %w", err)
	}
	// The settings page links to it, so only GitHub's own page will do
	if strings.HasPrefix(payload.HTMLURL, releasePagePrefix) {
		rel.Page = payload.HTMLURL
	}
	for _, asset := range payload.Assets {
		if asset.Name != releaseAsset {
			continue
		}
		digest, ok := strings.CutPrefix(asset.Digest, "sha256:")
		if !ok {
			return nil, fmt.Errorf("release %s has no sha256 digest for %s", rel.Version, releaseAsset)
		}
		rel.URL = asset.URL
		rel.SHA256 = digest
		return rel, nil
	}
	return nil, fmt.Errorf("release %s has no %s binary", rel.Version, releaseAsset)
}

// cachedLatestRelease returns the latest release and when GitHub was asked
// for it, at most once per releaseCacheTTL
func cachedLatestRelease(ctx context.Context) (*release, time.Time, error) {
	releaseCache.mu.Lock()
	defer releaseCache.mu.Unlock()
	if releaseCache.release != nil && time.Since(releaseCache.checked) < releaseCacheTTL {
		return releaseCache.release, releaseCache.checked, nil
	}
	rel, err := latestRelease(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	releaseCache.release, releaseCache.checked = rel, time.Now()
	return rel, releaseCache.checked, nil
}

// parseVersion parses a version such as 2.39.14
func parseVersion(version string) ([3]int, error) {
	var parsed [3]int
	parts := strings.Split(version, ".")
	if len(parts) != len(parsed) {
		return parsed, fmt.Errorf("version %q is not major.minor.patch", version)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return parsed, fmt.Errorf("version %q is not major.minor.patch", version)
		}
		parsed[i] = n
	}
	return parsed, nil
}

// isNewerVersion reports whether version is newer than current
func isNewerVersion(version, current string) (bool, error) {
	v, err := parseVersion(version)
	if err != nil {
		return false, err
	}
	c, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("compare with the running version: %w", err)
	}
	for i := range v {
		if v[i] != c[i] {
			return v[i] > c[i], nil
		}
	}
	return false, nil
}

// updateProblem says why wakeci can't replace its binary at path, or
// returns "" if it can
func updateProblem(path string) string {
	if platform := runtime.GOOS + "/" + runtime.GOARCH; platform != releasePlatform {
		return fmt.Sprintf("Releases are built for %s, and this server runs %s.", releasePlatform, platform)
	}
	dir := filepath.Dir(path)
	if err := unix.Access(dir, unix.W_OK); err != nil {
		return fmt.Sprintf("wakeci can't write to %s, the directory of its binary.", dir)
	}
	// Capabilities belong to the file, so a new binary wouldn't have them,
	// and wakeci may need them to listen on port 443
	if size, err := unix.Getxattr(path, "security.capability", nil); err == nil && size > 0 {
		return "The binary has file capabilities (set with setcap), which a new binary wouldn't have. " +
			"Update it by hand and run setcap again."
	}
	return ""
}

// downloadRelease downloads the binary of rel next to the running binary at
// path, checks its digest and that it runs, and returns the file
func downloadRelease(ctx context.Context, rel *release, path string) (_ string, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat running binary: %w", err)
	}
	// In the same directory, so the rename that installs it is atomic
	file, err := os.CreateTemp(filepath.Dir(path), ".wakeci-update-*")
	if err != nil {
		return "", fmt.Errorf("create download file: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			L.Log(ctx, LevelTrace, "close download file", "err", closeErr)
		}
		if removeErr := os.Remove(file.Name()); removeErr != nil {
			L.Error("remove download file", "file", file.Name(), "err", removeErr)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "wakeci/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download release %s: %w", rel.Version, err)
	}
	defer closeBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download release %s: GitHub answered %s", rel.Version, resp.Status)
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, releaseMaxSize+1))
	if err != nil {
		return "", fmt.Errorf("download release %s: %w", rel.Version, err)
	}
	if size > releaseMaxSize {
		return "", fmt.Errorf("release %s is larger than %d bytes", rel.Version, releaseMaxSize)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("save release %s: %w", rel.Version, err)
	}
	if digest := hex.EncodeToString(hash.Sum(nil)); digest != rel.SHA256 {
		return "", fmt.Errorf("release %s: sha256 is %s, GitHub lists %s", rel.Version, digest, rel.SHA256)
	}
	if err := os.Chmod(file.Name(), info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("make release %s executable: %w", rel.Version, err)
	}
	out, err := exec.CommandContext(ctx, file.Name(), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("run release %s: %w", rel.Version, err)
	}
	if got, want := strings.TrimSpace(string(out)), "wakeci "+rel.Version; got != want {
		return "", fmt.Errorf("release %s prints %q for --version, want %q", rel.Version, got, want)
	}
	return file.Name(), nil
}

// linkBinary hard-links the running binary to its backup; tests replace it
var linkBinary = os.Link

// installRelease replaces the binary at path with downloaded, and keeps the
// old one as path.previous. It refuses if it can't keep it, since that is
// the only way back
func installRelease(downloaded, path string) error {
	previous := path + ".previous"
	if err := os.Remove(previous); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", previous, err)
	}
	if err := linkBinary(path, previous); err != nil {
		return fmt.Errorf("keep the previous binary as %s: %w", previous, err)
	}
	if err := os.Rename(downloaded, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// closeBody closes a response body whose content was read or isn't needed
func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		L.Log(context.Background(), LevelTrace, "close response body", "err", err)
	}
}

// HandleUpdateGet checks for a newer release
// @Summary      Check for a newer wakeci release
// @Description  Asks GitHub for the latest release, at most every 10 minutes. problem says why wakeci can't update itself, and error why the check failed
// @Tags         update
// @Produce      json
// @Success      200      {object}   UpdateData
// @Router       /update/ [get]
func HandleUpdateGet(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	ctx, cancel := context.WithTimeout(r.Context(), releaseCheckTimeout)
	defer cancel()
	data := UpdateData{Current: Version}
	rel, checkedAt, err := cachedLatestRelease(ctx)
	if err != nil {
		// The settings page checks on every visit; a server without access
		// to GitHub shows why instead of an error each time
		logger.Warn("check for update", "err", err)
		data.Error = err.Error()
		writeJSON(w, logger, data)
		return
	}
	data.Latest = rel.Version
	data.CheckedAt = &checkedAt
	data.ReleaseNotes = rel.Page
	data.Available, err = isNewerVersion(rel.Version, Version)
	if err != nil {
		data.Problem = err.Error()
	} else if data.Available {
		data.Problem = updateProblem(executablePath)
	}
	writeJSON(w, logger, data)
}

// HandleUpdatePost installs a newer release and restarts
// @Summary      Install a newer wakeci release and restart
// @Description  Downloads the release, checks its sha256 digest and that it runs, and replaces the binary, keeping the old one as <binary>.previous. Then wakeci stops like on SIGTERM: queued builds wait, running builds may end until draintimeout, and wakeci starts the new binary. Queued builds start after the restart
// @Tags         update
// @Produce      plain
// @Param        version  formData   string   true  "The version to install, which must be the latest release"
// @Success      200      {string}   string
// @Failure      409      {string}   string
// @Failure      500      {string}   string
// @Failure      502      {string}   string
// @Router       /update/ [post]
func HandleUpdatePost(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	if !updateMutex.TryLock() {
		writePlainText(w, logger, http.StatusConflict, "An update is already running.")
		return
	}
	defer updateMutex.Unlock()
	if GlobalQueue.Status().Stopping {
		writePlainText(w, logger, http.StatusConflict, "wakeci is already stopping.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), releaseDownloadTimeout)
	defer cancel()
	rel, err := latestRelease(ctx)
	if err != nil {
		logger.Error("check for update", "err", err)
		writePlainText(w, logger, http.StatusBadGateway, err.Error())
		return
	}
	if version := r.FormValue("version"); version != rel.Version {
		writePlainText(w, logger, http.StatusConflict,
			fmt.Sprintf("The latest release is %s, not %q. Check for updates again.", rel.Version, version))
		return
	}
	addAuditAttrs(r, "to", rel.Version)
	newer, err := isNewerVersion(rel.Version, Version)
	if err != nil || !newer {
		writePlainText(w, logger, http.StatusConflict, fmt.Sprintf("wakeci %s can't update to %s.", Version, rel.Version))
		return
	}
	if problem := updateProblem(executablePath); problem != "" {
		writePlainText(w, logger, http.StatusConflict, problem)
		return
	}

	logger.Warn("installing update", "from", Version, "to", rel.Version)
	downloaded, err := downloadRelease(ctx, rel, executablePath)
	if err != nil {
		logger.Error("download update", "err", err)
		writePlainText(w, logger, http.StatusBadGateway, err.Error())
		return
	}
	if err := installRelease(downloaded, executablePath); err != nil {
		logger.Error("install update", "err", err)
		if removeErr := os.Remove(downloaded); removeErr != nil {
			logger.Error("remove download file", "file", downloaded, "err", removeErr)
		}
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	if !GlobalQueue.BeginStop(time.Now().Add(Config.drainTimeout), rel.Version) {
		writePlainText(w, logger, http.StatusOK,
			fmt.Sprintf("Installed %s. wakeci was already stopping, so it runs %s when it starts again.", rel.Version, rel.Version))
		return
	}
	writePlainText(w, logger, http.StatusOK,
		fmt.Sprintf("Installed %s. wakeci restarts once the running builds end.", rel.Version))
}
