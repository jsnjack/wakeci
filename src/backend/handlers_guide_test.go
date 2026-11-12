package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAgentGuide(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"read"}, "allJobs": {"true"}})

	main := chi.NewRouter()
	main.Get("/llms.txt", HandleAgentGuide)
	rec := httptest.NewRecorder()
	main.ServeHTTP(rec, httptest.NewRequest("GET", "/llms.txt", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "POST /mcp") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/markdown") {
		t.Errorf("without a key: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := keyRequest(router, "GET", "/llms.txt", token, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/api/build/<id>/summary") {
		t.Errorf("with a key: %d", rec.Code)
	}

	tests := []struct {
		target, want string
	}{
		{"/build/661292", "this is the web page of build 661292, its API is /api/build/661292 and /api/build/661292/summary"},
		{"/job/test_init", "its API is /api/job/test_init"},
		{"/settings", "see /llms.txt"},
	}
	for _, tt := range tests {
		rec := keyRequest(router, "GET", tt.target, token, nil)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s: %d %q, want 403 with %q", tt.target, rec.Code, rec.Body.String(), tt.want)
		}
	}
}
