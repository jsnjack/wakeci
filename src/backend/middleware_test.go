package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
