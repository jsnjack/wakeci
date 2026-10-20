package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// newTestBuildEnv wires up the minimal global state CreateBuild/BroadcastUpdate
// need (Config, DB with the buckets they touch, and a running WSHub).
func newTestBuildEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	Config = &WakeConfig{
		WorkDir: dir + string(os.PathSeparator),
		JobDir:  dir + string(os.PathSeparator),
		jobsExt: ".yaml",
	}

	dbPath := filepath.Join(dir, "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(GlobalBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(HistoryBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(JobsBucket)
		return err
	})
	if err != nil {
		t.Fatalf("create buckets: %v", err)
	}

	oldDB := DB
	DB = db
	oldHub := WSHub
	WSHub = newHub()
	go WSHub.run()

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test db: %v", err)
		}
		DB = oldDB
		WSHub = oldHub
	})
}

// TestSetBuildStatusWaitsForPendingTasksBeforeRunning is a regression test:
// pendingTasksWG.Add(1) used to happen inside the goroutine spawned to run
// on_pending tasks, not before it was spawned. A sync.WaitGroup's Add must
// happen-before the matching Wait; since it didn't, SetBuildStatus(Running)'s
// Wait() could return before the on_pending task had even started, letting
// it race the build's main tasks instead of finishing first.
func TestSetBuildStatusWaitsForPendingTasksBeforeRunning(t *testing.T) {
	newTestBuildEnv(t)

	job := &Job{
		Name: "pending-race-test",
		Tasks: []*Task{
			{Kind: string(StatusPending), Name: "slow pending", Command: "sleep 0.3"},
		},
	}

	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "pending-race-test.yaml"))
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}

	start := time.Now()
	build.SetBuildStatus(StatusRunning)
	elapsed := time.Since(start)

	if elapsed < 250*time.Millisecond {
		t.Errorf("SetBuildStatus(StatusRunning) returned after %v; expected it to wait for the ~300ms on_pending task first", elapsed)
	}
}

func TestCollectArtifactsRedactsSecrets(t *testing.T) {
	originalConfig := Config
	t.Cleanup(func() { Config = originalConfig })

	dir := t.TempDir()
	Config = &WakeConfig{
		WorkDir: dir + string(os.PathSeparator),
		secrets: map[string]string{"TOKEN": "top-secret-value"},
	}
	build := &Build{
		ID:     7,
		Job:    &Job{Artifacts: []string{"result.bin"}},
		Logger: L,
	}
	if err := os.MkdirAll(build.GetWorkspaceDir(), 0755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.MkdirAll(build.GetArtifactsDir(), 0755); err != nil {
		t.Fatalf("create artifacts directory: %v", err)
	}

	prefix := bytes.Repeat([]byte("x"), 32*1024-5)
	artifact := append(prefix, []byte("top-secret-value\x00tail")...)
	source := filepath.Join(build.GetWorkspaceDir(), "result.bin")
	if err := os.WriteFile(source, artifact, 0644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	build.CollectArtifacts()

	result, err := os.ReadFile(filepath.Join(build.GetArtifactsDir(), "result.bin"))
	if err != nil {
		t.Fatalf("read collected artifact: %v", err)
	}
	expected := append(prefix, []byte(redactedSecret+"\x00tail")...)
	if !bytes.Equal(result, expected) {
		t.Errorf("collected artifact was not redacted across the copy buffer boundary")
	}
	if len(build.BuildArtifacts) != 1 {
		t.Fatalf("BuildArtifacts length = %d, want 1", len(build.BuildArtifacts))
	}
	if build.BuildArtifacts[0].Size != int64(len(expected)) {
		t.Errorf("artifact size = %d, want %d", build.BuildArtifacts[0].Size, len(expected))
	}
}

// TestProcessLogEntryCollapsesCarriageReturns is a regression test: commands
// that redraw a progress bar in place (e.g. `lxc publish`) use \r instead of
// \n between updates. go-cmd only splits stdout/stderr on \n, so every
// redraw arrives here as one line with embedded \r. Without collapsing them,
// the raw line - including every intermediate progress tick - was written to
// the log and streamed to the browser, which rendered it as a wall of
// wrapped, misaligned text instead of the single final status a terminal
// would show. A \r at the very end is a line terminator rather than a redraw
// marker, and must not empty the line.
func TestProcessLogEntryCollapsesCarriageReturns(t *testing.T) {
	newTestBuildEnv(t)

	tests := []struct {
		name    string
		line    string
		want    string
		notWant []string
	}{
		{
			name:    "keeps only the final redraw",
			line:    "Publishing instance: Image pack: 1MB (1MB/s)\rPublishing instance: Image pack: 2MB (2MB/s)\rPublishing instance: Image pack: 3MB (3MB/s)",
			want:    "3MB (3MB/s)",
			notWant: []string{"1MB (1MB/s)", "2MB (2MB/s)"},
		},
		{
			name: "keeps a line terminated by a carriage return",
			line: "Test suite 'cmproxy_lint' failed! (code: 1)\r",
			want: "Test suite 'cmproxy_lint' failed! (code: 1)",
		},
		{
			name:    "keeps the final redraw when it is also terminated",
			line:    "downloading 50%\rdownloading 100%\r",
			want:    "downloading 100%",
			notWant: []string{"downloading 50%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := &Build{ID: 1, Job: &Job{Name: "progress"}, Logger: L}
			var buf bytes.Buffer
			bw := bufio.NewWriter(&buf)

			build.ProcessLogEntry(tt.line, bw, 1, time.Now())
			if err := bw.Flush(); err != nil {
				t.Fatalf("flush: %v", err)
			}

			got := buf.String()
			if strings.Contains(got, "\r") {
				t.Errorf("logged line still contains a carriage return: %q", got)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("logged line = %q, want it to contain %q", got, tt.want)
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("logged line kept an intermediate progress update %q: %q", nw, got)
				}
			}
		})
	}
}

func TestWaitForCondition(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		timeout      time.Duration
		wantTimedOut bool
		wantError    bool
	}{
		{name: "command completes", command: "true", timeout: time.Second},
		{name: "command is false", command: "false", timeout: time.Second, wantError: true},
		{name: "command times out", command: "sleep 1", timeout: 20 * time.Millisecond, wantTimedOut: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command("bash", "-c", tt.command)
			if err := command.Start(); err != nil {
				t.Fatalf("start condition: %v", err)
			}
			build := &Build{Logger: L}
			err, timedOut := build.waitForCondition(command, 1, tt.timeout)
			if timedOut != tt.wantTimedOut {
				t.Errorf("timedOut = %t, want %t", timedOut, tt.wantTimedOut)
			}
			if (err != nil) != tt.wantError {
				t.Errorf("error = %v, wantError %t", err, tt.wantError)
			}
		})
	}
}

func TestRunTaskInjectsSecretsIntoConditions(t *testing.T) {
	newTestBuildEnv(t)
	Config.secrets = map[string]string{"FLAG": "enabled"}

	job := &Job{Name: "condition-secrets-test"}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "condition-secrets-test.yaml"))
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}

	tests := []struct {
		name string
		task *Task
	}{
		{
			name: "when",
			task: &Task{ID: 1, When: "{{ secrets.FLAG }} == enabled", Command: "true"},
		},
		{
			name: "if",
			task: &Task{ID: 2, If: `test "{{ secrets.FLAG }}" = enabled`, Command: "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.task.startedAt = time.Now()
			if status := build.runTask(tt.task); status != StatusFinished {
				t.Errorf("runTask status = %q, want %q", status, StatusFinished)
			}
		})
	}
}

func TestCreateBuildUsesPrivatePermissions(t *testing.T) {
	newTestBuildEnv(t)

	build, err := CreateBuild(&Job{Name: "private-build"}, filepath.Join(Config.JobDir, "private-build.yaml"))
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}

	for _, path := range []string{build.GetWorkspaceDir(), build.GetWakespaceDir(), build.GetArtifactsDir()} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat %s: %v", path, statErr)
		}
		if got := info.Mode().Perm(); got&0o077 != 0 {
			t.Errorf("permissions for %s = %#o, want no group or other access", path, got)
		}
	}

	info, err := os.Stat(build.GetBuildConfigFilename())
	if err != nil {
		t.Fatalf("stat build plan: %v", err)
	}
	if got := info.Mode().Perm(); got != privateBuildFileMode {
		t.Errorf("build plan permissions = %#o, want %#o", got, privateBuildFileMode)
	}

	build.Job.Artifacts = []string{"nested/result.txt"}
	sourceDir := filepath.Join(build.GetWorkspaceDir(), "nested")
	if err := os.MkdirAll(sourceDir, privateBuildDirMode); err != nil {
		t.Fatalf("create nested workspace directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "result.txt"), []byte("result"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	build.CollectArtifacts()
	info, err = os.Stat(filepath.Join(build.GetArtifactsDir(), "nested"))
	if err != nil {
		t.Fatalf("stat nested artifact directory: %v", err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Errorf("nested artifact directory permissions = %#o, want no group or other access", got)
	}
}

// TestCallerDataNeverExpandsSecrets: only text from the job file may refer
// to secrets. Params a caller passes and values tasks write to build.env are
// data, or anyone allowed to run a job could pull any secret into it.
func TestCallerDataNeverExpandsSecrets(t *testing.T) {
	newTestBuildEnv(t)
	Config.secrets = map[string]string{"TOKEN": "s3cret"}
	reference := "{{ secrets.TOKEN }}"

	job := &Job{Name: "caller-data", DefaultParams: []map[string]string{{"FROM_FILE": reference}, {"FROM_CALLER": "default"}}}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "caller-data.yaml"))
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	build.Params[1]["FROM_CALLER"] = reference
	build.callerParams = map[string]bool{"FROM_CALLER": true}
	if err := os.WriteFile(build.GetWorkspaceDir()+"build.env", []byte("FROM_ENV=\""+reference+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	task := &Task{ID: 1, Command: `[[ "$FROM_FILE" == s3cret && "$FROM_CALLER" == *secrets.TOKEN* && "$FROM_ENV" == *secrets.TOKEN* ]]`, startedAt: time.Now()}
	if status := build.runTask(task); status != StatusFinished {
		log, _ := os.ReadFile(filepath.Join(build.GetWakespaceDir(), "task_1.log"))
		t.Errorf("runTask status = %q, want %q; log:\n%s", status, StatusFinished, log)
	}
}

// TestRedactionSeesThroughColours: a colour code inside a secret must not
// hide it from redaction
func TestRedactionSeesThroughColours(t *testing.T) {
	newTestBuildEnv(t)
	Config.secrets = map[string]string{"TOKEN": "s3cret"}
	build := &Build{ID: 1, Job: &Job{Name: "colours"}, Logger: L}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	build.ProcessLogEntry("token: s3\x1b[31mcr\x1b[0met", bw, 1, time.Now())
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "s3cret") {
		t.Errorf("logged %q, want the secret redacted", buf.String())
	}
}
