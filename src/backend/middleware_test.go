package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAFxX/httpcompression"
)

func TestCORSMiSetsHeadersBeforeResponseIsWritten(t *testing.T) {
	Config = &WakeConfig{Hostname: "ci.example.com"}
	handler := CORSMi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feed", nil))

	if got := rec.Header().Get("access-control-allow-origin"); got != "https://ci.example.com" {
		t.Errorf("access-control-allow-origin = %q, want %q", got, "https://ci.example.com")
	}
	if got := rec.Header().Get("access-control-max-age"); got != "86400" {
		t.Errorf("access-control-max-age = %q, want %q", got, "86400")
	}
}

// TestStorageSecurityMiBlocksNetworkAndFormsFromPreviewedContent is a
// regression test: /storage/build/* serves build artifacts, which can
// contain task-influenced content, with a relaxed CSP so HTML reports can
// still preview. The relaxed policy used to omit connect-src/form-action
// entirely, meaning a crafted artifact could execute a script that called
// back into wakeci's own authenticated API (change password, run/delete
// jobs) using the session cookie, which is still attached to a same-site
// request even under SameSite=Strict.
func TestStorageSecurityMiBlocksNetworkAndFormsFromPreviewedContent(t *testing.T) {
	Config = &WakeConfig{}
	handler := StorageSecurityMi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/storage/build/1/artifacts/report.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("content-security-policy")
	if csp == "" {
		t.Fatal("expected a content-security-policy header to be set")
	}
	for _, want := range []string{"connect-src 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("expected CSP to contain %q, got %q", want, csp)
		}
	}
}

// TestStorageSecurityMiStillAllowsInlineScriptForReportPreviews verifies the
// fix doesn't regress the feature it's explicitly meant to preserve:
// self-contained HTML reports with inline script/style must still render.
func TestStorageSecurityMiStillAllowsInlineScriptForReportPreviews(t *testing.T) {
	Config = &WakeConfig{}
	handler := StorageSecurityMi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/storage/build/1/artifacts/report.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("content-security-policy")
	for _, want := range []string{"script-src 'self' 'unsafe-inline'", "style-src 'self' 'unsafe-inline'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("expected CSP to still allow inline script/style for report previews, got %q", csp)
		}
	}
}

// A build artifact can't reach another page on this origin to send requests
// from: no frames or objects, no popups, no service workers. Checked in
// Chrome: an artifact under the earlier policy created API keys through an
// <object> and a popup, and registered a service worker.
func TestStorageSecurityMiIsolatesArtifacts(t *testing.T) {
	Config = &WakeConfig{}
	handler := StorageSecurityMi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, target := range []string{
		"/storage/build/1/artifacts/report.html",
		"/storage/build/1/artifacts/",
		"/storage/build/1/artifacts/report.pdf/",
		"/storage/build/1/artifacts/report.pdf/.",
		"/storage/build/1/artifacts/image.svg",
		"/storage/build/1/artifacts/noextension",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		csp := rec.Header().Get("content-security-policy")
		for _, want := range []string{
			"frame-src 'none'", "object-src 'none'", "worker-src 'none'", "frame-ancestors 'none'",
			"sandbox allow-scripts allow-same-origin",
		} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", target, csp, want)
			}
		}
		if strings.Contains(csp, "allow-popups") || strings.Contains(csp, "allow-top-navigation") {
			t.Errorf("%s: CSP %q lets the artifact open or navigate other windows", target, csp)
		}
	}

	// PDFs keep the earlier policy: they can't run scripts on this origin,
	// and Chrome won't show a sandboxed PDF
	for _, target := range []string{"/storage/build/1/artifacts/report.pdf", "/storage/build/1/artifacts/REPORT.PDF", "/storage/build/1/x/../report.pdf"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if csp := rec.Header().Get("content-security-policy"); csp != storagePDFCSP {
			t.Errorf("%s: CSP = %q, want the PDF policy", target, csp)
		}
	}
}

func TestStorageSecurityMiPinsLogContentType(t *testing.T) {
	Config = &WakeConfig{}
	handler := StorageSecurityMi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/storage/build/1/task_0.log", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ct := rec.Header().Get("content-type"); ct != "text/plain" {
		t.Errorf("expected content-type text/plain for a .log file, got %q", ct)
	}
}

// A range request gets its range even from a client that accepts gzip, as
// browsers and most HTTP libraries do; the compression adapter used to drop
// the Range header and send the whole file
func TestRangeRequestsSkipCompression(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("a line of a long task log\n", 200) + "the end\n"
	if err := os.WriteFile(filepath.Join(dir, "task_0.log"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	compress, err := httpcompression.DefaultAdapter()
	if err != nil {
		t.Fatal(err)
	}
	handler := withCompression(compress, http.FileServer(http.Dir(dir)))

	req := httptest.NewRequest("GET", "/task_0.log", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=-8")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "the end\n" {
		t.Errorf("range: got %d %q, want 206 %q", rec.Code, rec.Body.String(), "the end\n")
	}

	// Without a range the file is still compressed
	req = httptest.NewRequest("GET", "/task_0.log", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("whole file: Content-Encoding %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != content {
		t.Errorf("whole file: got %d bytes (%v), want %d", len(body), err, len(content))
	}
}
