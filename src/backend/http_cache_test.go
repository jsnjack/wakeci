package main

import (
	"net/http"
	"testing"
)

func TestETagMatches(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{`"abc"`, true},
		{`W/"abc"`, true},
		{`"x", "abc"`, true},
		{`*`, true},
		{`"x"`, false},
		{``, false},
	}
	for _, tt := range tests {
		if got := etagMatches(tt.header, `"abc"`); got != tt.want {
			t.Errorf("etagMatches(%q) = %t, want %t", tt.header, got, tt.want)
		}
	}
}

// A client that has a build or its summary asks whether it changed instead
// of downloading it again
func TestBuildRoutesAnswerIfNoneMatch(t *testing.T) {
	newAPIKeyTestEnv(t)
	basicAuth := func(r *http.Request) { r.SetBasicAuth("", "admin") }
	for _, target := range []string{"/api/build/1", "/api/build/1/summary"} {
		t.Run(target, func(t *testing.T) {
			first := apiCall("GET", target, nil, nil, basicAuth)
			etag := first.Header().Get("ETag")
			if first.Code != http.StatusOK || etag == "" || first.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("first: %d, ETag %q, Cache-Control %q", first.Code, etag, first.Header().Get("Cache-Control"))
			}
			again := apiCall("GET", target, nil, nil, func(r *http.Request) {
				basicAuth(r)
				r.Header.Set("If-None-Match", etag)
			})
			if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
				t.Errorf("unchanged: %d with %d bytes, want 304 without a body", again.Code, again.Body.Len())
			}
			// Any change, such as a new status, makes a new tag
			saveTestBuildStatus(t, 1, StatusFailed)
			changed := apiCall("GET", target, nil, nil, func(r *http.Request) {
				basicAuth(r)
				r.Header.Set("If-None-Match", etag)
			})
			if changed.Code != http.StatusOK || changed.Header().Get("ETag") == etag {
				t.Errorf("changed: %d, ETag %q; want 200 and a new tag", changed.Code, changed.Header().Get("ETag"))
			}
			saveTestBuildStatus(t, 1, StatusFinished)
		})
	}
}
