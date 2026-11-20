package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestColorLine(t *testing.T) {
	old := Config
	Config = &WakeConfig{secrets: map[string]string{"TOKEN": "hunter2"}}
	t.Cleanup(func() { Config = old })
	link := "\x1b]8;;https://example.com/report\x1b\\report\x1b]8;;\x1b\\"
	tests := []struct {
		name, line, plain, ansi string
		colored                 bool
	}{
		{"plain text", "ok", "ok", "ok", false},
		{"colours kept in the colour log only", "\x1b[31mFAIL\x1b[0m x", "FAIL x", "\x1b[31mFAIL\x1b[0m x", true},
		{"other sequences dropped from both", "\x1b[2Kdone\x1b[1A", "done", "done", false},
		{"hyperlinks kept, their text in the plain log", "see " + link, "see report", "see " + link, true},
		{"a secret redacted", "token hunter2", "token " + redactedSecret, "token " + redactedSecret, false},
		// Colour codes inside a secret hid it from the redaction, and
		// stripping them joined it again in the plain log
		{"a secret split by colours redacted, the codes before it", "x hun\x1b[1mter2\x1b[0m y", "x " + redactedSecret + " y", "x \x1b[1m" + redactedSecret + "\x1b[0m y", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain, ansi, colored := colorLine(tt.line)
			if plain != tt.plain || ansi != tt.ansi || colored != tt.colored {
				t.Errorf("colorLine(%q) = %q, %q, %v; want %q, %q, %v", tt.line, plain, ansi, colored, tt.plain, tt.ansi, tt.colored)
			}
			// The colour log is the plain log, line for line, with codes
			if visible := anyEscapeRE.ReplaceAllString(ansi, ""); visible != plain {
				t.Errorf("colour log's text %q differs from the plain log's %q", visible, plain)
			}
		})
	}
}

// The colour log starts at the first line with colours, with the lines the
// log had before it, so the two stay line for line
func TestColourLogStartsAtTheFirstColouredLine(t *testing.T) {
	dir := t.TempDir()
	path, ansiPath := filepath.Join(dir, "task_0.log"), filepath.Join(dir, "task_0.ansi.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	log := &taskLogWriter{file: file, w: bufio.NewWriter(file), path: path, ansiPath: ansiPath}
	if _, err := log.writeLines("one\n", "one\n", false); err != nil {
		t.Fatal(err)
	}
	if err := log.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ansiPath); !os.IsNotExist(err) {
		t.Fatalf("colour log before any colour: %v, want none", err)
	}
	for _, line := range [][2]string{{"two\n", "\x1b[32mtwo\x1b[0m\n"}, {"three\n", "three\n"}} {
		first, err := log.writeLines(line[0], line[1], line[0] != line[1])
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.Count("one\ntwo\n", "\n"); line[0] == "two\n" && first != want {
			t.Errorf("two is line %d, want %d", first, want)
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	ansi, err := os.ReadFile(ansiPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(ansi) != "one\n\x1b[32mtwo\x1b[0m\nthree\n" {
		t.Errorf("colour log = %q", ansi)
	}
}

// color: true asks the task's tools for colours; without it, nothing changes
func TestColorAsksToolsForColours(t *testing.T) {
	for _, tt := range []struct {
		name     string
		jobColor bool
		task     *Task
		want     string
	}{
		{"job", true, &Task{ID: 0, Command: "echo force=$FORCE_COLOR", Kind: KindMain}, "force=1"},
		{"task", false, &Task{ID: 0, Command: "echo force=$FORCE_COLOR", Kind: KindMain, Color: true}, "force=1"},
		{"neither", false, &Task{ID: 0, Command: "echo force=$FORCE_COLOR", Kind: KindMain}, "force=\n"},
		{"the task's env wins", true, &Task{ID: 0, Command: "echo force=$FORCE_COLOR", Kind: KindMain, Env: map[string]string{"FORCE_COLOR": "0"}}, "force=0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			build := abortTestBuild(t, &Job{Name: "color-" + strings.ReplaceAll(tt.name, " ", "-"), Color: tt.jobColor, Tasks: []*Task{tt.task}})
			waitDone(t, startBuild(build), 10_000_000_000)
			if log := taskLog(t, build, 0); !strings.Contains(log, tt.want) {
				t.Errorf("log = %q, want %q", log, tt.want)
			}
		})
	}
}
