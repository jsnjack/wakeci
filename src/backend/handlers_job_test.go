package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A missing job and a bad flag are the client's mistakes: 404 and 400, as
// documented, not 500, which says the server failed
func TestJobRoutesAnswerMissingJobsAndBadInput(t *testing.T) {
	newAPIKeyTestEnv(t)
	for _, c := range []struct {
		method, target string
		form           url.Values
		want           int
	}{
		{"GET", "/api/job/nosuch", nil, http.StatusNotFound},
		{"GET", "/api/job/deploy", nil, http.StatusOK},
		{"DELETE", "/api/job/nosuch", nil, http.StatusNotFound},
		{"POST", "/api/job/nosuch/set_active", url.Values{"active": {"true"}}, http.StatusNotFound},
		{"POST", "/api/job/deploy/set_active", url.Values{"active": {"maybe"}}, http.StatusBadRequest},
		{"POST", "/api/job/deploy/set_active", url.Values{"active": {"true"}}, http.StatusOK},
		{"POST", "/api/jobs/create", url.Values{"name": {"a/b"}}, http.StatusBadRequest},
	} {
		rec := adminRequest(c.method, c.target, c.form)
		if rec.Code != c.want {
			t.Errorf("%s %s: got %d %q, want %d", c.method, c.target, rec.Code, rec.Body.String(), c.want)
		}
		if rec.Code >= 400 && !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
			t.Errorf("%s %s: Content-Type %q, want text/plain", c.method, c.target, rec.Header().Get("Content-Type"))
		}
	}
}
