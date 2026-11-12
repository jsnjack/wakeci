package main

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// A job summary is shown to whoever may read the build, so secret values in
// it are redacted like those of task logs, also one that crosses the 64 KB
// cut; storage doesn't serve the raw file
func TestJobSummaryHidesSecrets(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	Config.secrets = map[string]string{"TOKEN": "s3cret-summary-value"}
	write := func(text string) {
		t.Helper()
		if err := os.MkdirAll(wakespaceDir(1), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(wakespaceDir(1)+jobSummaryFile, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("deployed with s3cret-summary-value\n")
	text, err := readJobSummary(1)
	if err != nil || strings.Contains(text, "s3cret") || !strings.Contains(text, redactedSecret) {
		t.Errorf("summary = %q, %v; want the secret redacted", text, err)
	}
	write(strings.Repeat("a", maxJobSummary-3) + "s3cret-summary-value and more")
	text, err = readJobSummary(1)
	if err != nil || strings.Contains(text, "s3c") || !strings.HasSuffix(text, "(cut at 64 KB)") {
		t.Errorf("summary cut at 64 KB ends %q, %v; want no part of the secret", text[max(0, len(text)-60):], err)
	}

	token := createTestKey(t, url.Values{"name": {"reader"}, "scope": {"read"}, "allJobs": {"true"}})
	for _, target := range []string{"/storage/build/1/summary.md", "/storage/build/1/./summary.md", "/storage/build/1//summary.md"} {
		if rec := keyRequest(router, "GET", target, token, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", target, rec.Code)
		}
	}
	if rec := keyRequest(router, "GET", "/storage/build/1/task_1.log", token, nil); rec.Code != http.StatusOK {
		t.Errorf("a task log through storage: %d, want 200", rec.Code)
	}
}
