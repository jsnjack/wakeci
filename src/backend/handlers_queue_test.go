package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestQueueAPI(t *testing.T) {
	newSettingsTestEnv(t)
	for _, user := range []string{adminUser, "alice@example.com"} {
		t.Run("user "+user, func(t *testing.T) {
			rec := apiCall("GET", "/api/queue/", loginAs(t, user), nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}
			var status QueueStatusData
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if status.Stopping || status.AbortAt != nil || status.Version != Version {
				t.Errorf("status = %+v, want not stopping, running %s", status, Version)
			}
		})
	}
}

func TestQueueAPIWhileStopping(t *testing.T) {
	newSettingsTestEnv(t)
	build := queueTestBuild(t, &Job{Name: "later", Tasks: []*Task{{ID: 0, Command: "true", Kind: KindMain}}})
	abortAt := time.Now().Add(time.Minute)
	GlobalQueue.BeginStop(abortAt, "2.0.0")
	cookie := loginAs(t, adminUser)

	rec := apiCall("GET", "/api/queue/", cookie, nil, nil)
	var status QueueStatusData
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if !status.Stopping || status.RestartVersion != "2.0.0" || status.AbortAt == nil || status.Queued != 1 ||
		status.Version != Version {
		t.Errorf("status = %+v, want stopping into 2.0.0 with one queued build, running %s", status, Version)
	}

	rec = apiCall("POST", "/api/build/"+strconv.Itoa(build.ID)+"/start", cookie, nil, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "start after the restart") {
		t.Errorf("start now while stopping: %d %q, want 409", rec.Code, rec.Body.String())
	}
	if !GlobalQueue.Verify(build.ID) {
		t.Error("the build must stay queued")
	}
}
