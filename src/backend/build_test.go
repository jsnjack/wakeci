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
		if _, err := tx.CreateBucketIfNotExists(QueueBucket); err != nil {
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

	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "pending-race-test.yaml"), false)
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
			build := &Build{ID: 1, Logger: L}
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
	closed := make(chan struct{})
	close(closed)
	tests := []struct {
		name         string
		command      string
		timeout      time.Duration
		abort        chan struct{}
		wantTimedOut bool
		wantAborted  bool
		wantError    bool
	}{
		{name: "command completes", command: "true", timeout: time.Second},
		{name: "command is false", command: "false", timeout: time.Second, wantError: true},
		{name: "command times out", command: "sleep 1", timeout: 20 * time.Millisecond, wantTimedOut: true, wantError: true},
		{name: "build is aborted", command: "sleep 10", timeout: 5 * time.Second, abort: closed, wantAborted: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command("bash", "-c", tt.command)
			if err := command.Start(); err != nil {
				t.Fatalf("start condition: %v", err)
			}
			build := &Build{Logger: L}
			start := time.Now()
			timedOut, aborted, err := build.waitForCondition(command, 1, tt.timeout, tt.abort, func() {})
			if timedOut != tt.wantTimedOut {
				t.Errorf("timedOut = %t, want %t", timedOut, tt.wantTimedOut)
			}
			if aborted != tt.wantAborted {
				t.Errorf("aborted = %t, want %t", aborted, tt.wantAborted)
			}
			if (err != nil) != tt.wantError {
				t.Errorf("error = %v, wantError %t", err, tt.wantError)
			}
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("took %v, want the command killed promptly", took)
			}
		})
	}
}

func TestRunTaskInjectsSecretsIntoConditions(t *testing.T) {
	newTestBuildEnv(t)
	Config.secrets = map[string]string{"FLAG": "enabled"}

	job := &Job{Name: "condition-secrets-test"}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "condition-secrets-test.yaml"), false)
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

	build, err := CreateBuild(&Job{Name: "private-build"}, filepath.Join(Config.JobDir, "private-build.yaml"), false)
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

// A job may write a param into build.env, so in a build started with an API
// key build.env values aren't turned into secrets: the key could otherwise
// build a secret reference out of a param that isn't one
func TestRunTaskBuildEnvSecrets(t *testing.T) {
	newTestBuildEnv(t)
	Config.secrets = map[string]string{"TOKEN": "s3cr3t"}

	tests := []struct {
		name       string
		withAPIKey bool
		want       string
	}{
		{"without a key", false, "s3cr3t"},
		{"with a key", true, "{{ secrets.TOKEN }}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build, err := CreateBuild(&Job{Name: "build-env-secrets"}, filepath.Join(Config.JobDir, "build-env-secrets.yaml"), tt.withAPIKey)
			if err != nil {
				t.Fatalf("CreateBuild: %v", err)
			}
			tasks := []*Task{
				// printf puts the reference together, so the command holds none
				{ID: 1, Command: `printf 'X={%s secrets.TOKEN }}\n' '{' >> build.env`},
				{ID: 2, Command: `printf '%s' "$X" > out.txt`},
			}
			for _, task := range tasks {
				task.startedAt = time.Now()
				if status := build.runTask(task); status != StatusFinished {
					t.Fatalf("task %d status = %q", task.ID, status)
				}
			}
			got, err := os.ReadFile(build.GetWorkspaceDir() + "out.txt")
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("X = %q, want %q", got, tt.want)
			}
		})
	}
}

// The API key flag is part of the build before CreateBuild starts the
// pending tasks, which may read build.env too
func TestCreateBuildSetsAPIKeyFlagBeforePendingTasks(t *testing.T) {
	newTestBuildEnv(t)
	Config.secrets = map[string]string{"TOKEN": "s3cr3t"}
	job := &Job{
		Name: "pending-build-env",
		Tasks: []*Task{
			{ID: 1, Kind: StatusPending, Command: `printf 'X={%s secrets.TOKEN }}\n' '{' >> build.env`},
			{ID: 2, Kind: StatusPending, Command: `printf '%s' "$X" > out.txt`},
		},
	}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "pending-build-env.yaml"), true)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	build.pendingTasksWG.Wait()
	got, err := os.ReadFile(build.GetWorkspaceDir() + "out.txt")
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if want := "{{ secrets.TOKEN }}"; string(got) != want {
		t.Errorf("X = %q, want %q", got, want)
	}
}

func TestStartExposesFailedTaskToHooks(t *testing.T) {
	// The hook copies the log too, to check it is complete when hooks run
	dump := `env | grep -E '^WAKE_(BUILD_STATUS|FAILED_TASK_)' | sort > hook.env; ` +
		`[ -z "$WAKE_FAILED_TASK_LOG" ] || cp "$WAKE_FAILED_TASK_LOG" hook.log`
	stopped := func(status string) string {
		return "WAKE_BUILD_STATUS=" + status + "\nWAKE_FAILED_TASK_ID=1\n" +
			"WAKE_FAILED_TASK_LOG=%stask_1.log\nWAKE_FAILED_TASK_NAME=Broken step\n"
	}
	tests := []struct {
		name    string
		main    string
		when    string
		timeout string
		// abort aborts the build through the queue once the step created "started"
		abort bool
		want  string
	}{
		{name: "failed", main: "echo boom; exit 1", want: stopped("failed")},
		{name: "finished", main: "true", want: "WAKE_BUILD_STATUS=finished\n"},
		{name: "timed out", main: "echo boom; sleep 30", timeout: "1s", want: stopped("timed out")},
		{name: "aborted", main: "echo boom; touch started; sleep 30", abort: true, want: stopped("aborted")},
		{name: "skipped", main: "exit 1", when: "1 == 2", want: "WAKE_BUILD_STATUS=finished\n"},
		{
			name: "build.env can't override",
			main: "echo boom; printf 'WAKE_BUILD_STATUS=finished\\nWAKE_FAILED_TASK_NAME=x\\n' >> build.env; exit 1",
			want: stopped("failed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := abortTestBuild(t, &Job{Name: "failed-task-env", Timeout: tt.timeout, Tasks: []*Task{
				{ID: 0, Name: "Good step", Command: "true", Kind: KindMain},
				{ID: 1, Name: "Broken step", Command: tt.main, When: tt.when, Kind: KindMain},
				{ID: 2, Name: "Hook", Command: dump, Kind: StatusFailed},
				{ID: 3, Name: "Hook", Command: dump, Kind: StatusAborted},
				{ID: 4, Name: "Hook", Command: dump, Kind: StatusFinished},
			}})
			done := startBuild(build)
			if tt.abort {
				waitForFile(t, build, "started")
				if err := GlobalQueue.Abort(build.ID, StatusAborted); err != nil {
					t.Fatalf("Abort: %v", err)
				}
			}
			waitDone(t, done, 20*time.Second)

			got, err := os.ReadFile(build.GetWorkspaceDir() + "hook.env")
			if err != nil {
				t.Fatalf("read hook output: %v", err)
			}
			want := strings.Replace(tt.want, "%s", build.GetWakespaceDir(), 1)
			if string(got) != want {
				t.Errorf("hook env = %q, want %q", got, want)
			}
			if !strings.Contains(want, "WAKE_FAILED_TASK_LOG") {
				return
			}
			log, err := os.ReadFile(build.GetWorkspaceDir() + "hook.log")
			if err != nil {
				t.Fatalf("read copied log: %v", err)
			}
			if !strings.Contains(string(log), "boom") {
				t.Errorf("failed task log = %q, want the task output", log)
			}
		})
	}
}

// TestRunTaskConditionOutcomes pins what `when` and `if` do with the task and
// what they write to its log
func TestRunTaskConditionOutcomes(t *testing.T) {
	newTestBuildEnv(t)
	job := &Job{Name: "condition-outcomes"}
	build, err := CreateBuild(job, filepath.Join(Config.JobDir, "condition-outcomes.yaml"), false)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}

	tests := []struct {
		name    string
		task    *Task
		want    ItemStatus
		wantLog []string
		notLog  []string
	}{
		{
			name: "when true",
			task: &Task{When: "${WAKE_BUILD_ID} == 1", Command: "echo ran"},
			want: StatusFinished,
			wantLog: []string{
				"> Checking `when` condition: ${WAKE_BUILD_ID} == 1",
				"> Expanded condition: 1 == 1",
				"> Condition is true",
				"ran",
			},
		},
		{
			name: "when false",
			task: &Task{When: "1 == 2", Command: "echo ran"},
			want: StatusSkipped,
			wantLog: []string{
				"> Checking `when` condition: 1 == 2",
				"> Condition is false: exit status 1. Skipping the task",
			},
			notLog: []string{"> Running command"},
		},
		{
			name:    "if true",
			task:    &Task{If: "true", Command: "echo ran"},
			want:    StatusFinished,
			wantLog: []string{"> Checking `if` condition: true", "> Condition is true", "ran"},
		},
		{
			name: "if false",
			task: &Task{If: "exit 3", Command: "echo ran"},
			want: StatusSkipped,
			wantLog: []string{
				"> Checking `if` condition: exit 3",
				"> Condition is false: exit status 3. Skipping the task",
			},
			notLog: []string{"> Running command"},
		},
		{
			name:    "when false skips if",
			task:    &Task{When: "1 == 2", If: "true", Command: "echo ran"},
			want:    StatusSkipped,
			wantLog: []string{"> Checking `when` condition: 1 == 2"},
			notLog:  []string{"> Checking `if` condition", "> Running command"},
		},
		{
			name:    "when and if true",
			task:    &Task{When: "1 == 1", If: "true", Command: "echo ran"},
			want:    StatusFinished,
			wantLog: []string{"> Checking `when` condition", "> Checking `if` condition", "ran"},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.task.ID = i
			tt.task.startedAt = time.Now()
			if status := build.runTask(tt.task); status != tt.want {
				t.Errorf("runTask status = %q, want %q", status, tt.want)
			}
			log, err := os.ReadFile(build.taskLogPath(i))
			if err != nil {
				t.Fatalf("read task log: %v", err)
			}
			for _, line := range tt.wantLog {
				if !strings.Contains(string(log), line) {
					t.Errorf("log = %q, want it to contain %q", log, line)
				}
			}
			for _, line := range tt.notLog {
				if strings.Contains(string(log), line) {
					t.Errorf("log = %q, want no %q", log, line)
				}
			}
		})
	}
}

// TestWaitForConditionEndsAsTimeoutFires is a regression test: when the
// condition exited 0 just as the timer fired, waitForCondition reported a
// timeout with a nil error, and checkCondition panicked on condErr.Error()
func TestWaitForConditionEndsAsTimeoutFires(t *testing.T) {
	build := &Build{Logger: L}
	for i := 0; i < 100; i++ {
		command := exec.Command("bash", "-c", "true")
		if err := command.Start(); err != nil {
			t.Fatalf("start condition: %v", err)
		}
		// Let it exit, so both the timer and Wait are ready at once
		time.Sleep(5 * time.Millisecond)
		timedOut, _, err := build.waitForCondition(command, 1, 0, nil, func() {})
		if timedOut && err == nil {
			t.Fatal("timedOut with a nil error: the condition finished, it didn't time out")
		}
	}
}
