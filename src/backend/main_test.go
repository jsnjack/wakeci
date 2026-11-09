package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestServeUntilShutdownStopsServerAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	serveStarted := make(chan struct{})
	shutdownCalled := make(chan struct{})

	serve := func() error {
		close(serveStarted)
		<-shutdownCalled
		return http.ErrServerClosed
	}
	shutdown := func(context.Context) error {
		close(shutdownCalled)
		return nil
	}

	result := make(chan error, 1)
	go func() {
		result <- serveUntilShutdown(ctx, serve, shutdown)
	}()

	<-serveStarted
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serveUntilShutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveUntilShutdown did not stop after cancellation")
	}
}

func TestServeUntilShutdownReturnsListenerErrors(t *testing.T) {
	want := errors.New("listen failed")
	err := serveUntilShutdown(context.Background(), func() error { return want }, func(context.Context) error {
		t.Fatal("shutdown called after listener failure")
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("serveUntilShutdown error = %v, want wrapped %v", err, want)
	}
}

// versionFlagEnv makes the test binary re-enter TestVersionFlagPrintsVersion
// as a child process, running initApp with the flag spelling it names.
const versionFlagEnv = "WAKECI_TEST_VERSION_FLAG"

// TestVersionFlagPrintsVersion runs the real initApp in a child process to
// cover the -version exit path: initApp calls os.Exit, so it cannot be
// exercised in-process. It asserts the exact output and exit status of each
// spelling, and that the request is honoured before the logger is configured —
// the child also passes -trace, so a version request that reached initLogger
// would leave TraceLogPath behind.
func TestVersionFlagPrintsVersion(t *testing.T) {
	if spelling := os.Getenv(versionFlagEnv); spelling != "" {
		flag.CommandLine = flag.NewFlagSet("wakeci", flag.ExitOnError)
		os.Args = []string{"wakeci", "-trace", spelling}
		initApp()
		// initApp is expected to exit before returning.
		fmt.Fprintln(os.Stderr, "initApp returned instead of exiting")
		os.Exit(2)
	}

	want := fmt.Sprintf("wakeci %s\n", Version)
	for _, spelling := range []string{"-version", "--version", "-V"} {
		t.Run(spelling, func(t *testing.T) {
			before := traceLogState(t)

			cmd := exec.Command(os.Args[0], "-test.run=^TestVersionFlagPrintsVersion$")
			cmd.Env = append(os.Environ(), versionFlagEnv+"="+spelling)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				t.Fatalf("%s: %v (stdout %q, stderr %q)", spelling, err, stdout.String(), stderr.String())
			}
			if got := stdout.String(); got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
			if after := traceLogState(t); after != before {
				t.Errorf("%s touched %s: %q -> %q", spelling, TraceLogPath, before, after)
			}
		})
	}
}

// TestVersionDefaultsToDev guards the fallback used when the binary is built
// without -ldflags="-X main.Version=...", so -version never prints an empty
// version.
func TestVersionDefaultsToDev(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty; it must default to a placeholder such as \"dev\"")
	}
}

// traceLogState describes TraceLogPath well enough to tell whether a run
// created or truncated it, without requiring the file to be absent — the path
// is fixed, so a real wakeci instance may own it.
func traceLogState(t *testing.T) string {
	t.Helper()
	info, err := os.Stat(TraceLogPath)
	switch {
	case os.IsNotExist(err):
		return "absent"
	case err != nil:
		t.Fatalf("stat %s: %v", TraceLogPath, err)
		return ""
	default:
		return fmt.Sprintf("size=%d mtime=%s", info.Size(), info.ModTime())
	}
}

// TestExitPlan is a regression test: wakeci re-executed into an update even
// when tasks it couldn't stop still ran, so they ran beside the builds the
// new process restored
func TestExitPlan(t *testing.T) {
	tests := []struct {
		name           string
		restartVersion string
		tasksLeft      bool
		wantRestart    bool
		wantErr        bool
	}{
		{"update", "2.0.0", false, true, false},
		{"update with tasks left", "2.0.0", true, false, true},
		{"stop", "", false, false, false},
		{"stop with tasks left", "", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restart, err := exitPlan(tt.restartVersion, tt.tasksLeft)
			if restart != tt.wantRestart || (err != nil) != tt.wantErr {
				t.Errorf("exitPlan() = %v, %v; want restart %v, error %v", restart, err, tt.wantRestart, tt.wantErr)
			}
		})
	}
}
