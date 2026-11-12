package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// onlyKey returns the one key the admin password lists
func onlyKey(t *testing.T) (*APIKey, APIKeysData) {
	t.Helper()
	data := listKeysAs(t, adminUser)
	if len(data.Keys) != 1 {
		t.Fatalf("keys = %d, want 1", len(data.Keys))
	}
	return data.Keys[0], data
}

// Every request that gets past a key's checks counts once, and the list
// shows it at once, though only the minute's first request writes to the
// database; a stop saves the rest without counting it twice
func TestAPIKeyUses(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"busy"}, "scope": {"read"}, "allJobs": {"true"}})
	if key, _ := onlyKey(t); key.Uses != 0 || key.LastUsedAt != nil || key.UsesByDay == nil || len(key.UsesByDay) != 0 {
		t.Errorf("a new key = uses %d, last %v, days %v; want 0, never and []", key.Uses, key.LastUsedAt, key.UsesByDay)
	}
	for i := range 3 {
		if rec := keyRequest(router, "GET", "/api/jobs/", token, nil); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	today := usageDay(time.Now())
	key, data := onlyKey(t)
	if key.Uses != 3 || key.LastUsedAt == nil || !slices.Equal(key.UsesByDay, []DayUses{{Day: today, Uses: 3}}) || data.Today != today {
		t.Errorf("after 3 requests: uses %d, last %v, days %v, today %q", key.Uses, key.LastUsedAt, key.UsesByDay, data.Today)
	}

	if err := flushAPIKeyUses(); err != nil {
		t.Fatal(err)
	}
	var stored storedAPIKey
	if err := DB.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(APIKeysBucket).Get([]byte(key.ID)), &stored)
	}); err != nil {
		t.Fatal(err)
	}
	if stored.Uses != 3 || !slices.Equal(stored.UsesByDay, []DayUses{{Day: today, Uses: 3}}) {
		t.Errorf("saved: uses %d, days %v; want 3 today", stored.Uses, stored.UsesByDay)
	}
	if key, _ := onlyKey(t); key.Uses != 3 {
		t.Errorf("listed after saving: %d, want 3", key.Uses)
	}

	// A request refused for the rate doesn't count
	old := apiKeyRequestLimiter
	apiKeyRequestLimiter = newKeyRequestLimiter(1, 1)
	t.Cleanup(func() { apiKeyRequestLimiter = old })
	keyRequest(router, "GET", "/api/jobs/", token, nil)
	if rec := keyRequest(router, "GET", "/api/jobs/", token, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: %d, want 429", rec.Code)
	}
	if key, _ := onlyKey(t); key.Uses != 4 {
		t.Errorf("with a refused request: %d, want 4", key.Uses)
	}

	// Uses of a key revoked meanwhile don't bring it back
	if rec := manageKeys(t, "DELETE", "/api/keys/"+key.ID, adminUser, nil); rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if err := flushAPIKeyUses(); err != nil {
		t.Fatal(err)
	}
	if data := listKeysAs(t, adminUser); len(data.Keys) != 0 {
		t.Errorf("keys after revoking and saving = %d, want 0", len(data.Keys))
	}
}

// A key keeps the last 30 days; older days drop out, the total stays
func TestAPIKeyUsesKeepThirtyDays(t *testing.T) {
	now := time.Now()
	day := func(ago int) string { return usageDay(now.AddDate(0, 0, -ago)) }
	key := &APIKey{Uses: 10, UsesByDay: []DayUses{{Day: day(40), Uses: 5}, {Day: day(29), Uses: 3}, {Day: day(1), Uses: 2}}}
	keyUse{count: 2, last: now, days: map[string]int64{day(0): 1, day(1): 1}}.addTo(key)
	want := []DayUses{{Day: day(29), Uses: 3}, {Day: day(1), Uses: 3}, {Day: day(0), Uses: 1}}
	if key.Uses != 12 || !slices.Equal(key.UsesByDay, want) {
		t.Errorf("uses %d, days %v; want 12 and %v", key.Uses, key.UsesByDay, want)
	}
}

// An MCP tool call is one request of its key: the route the tool calls is
// neither counted nor limited again
func TestMCPToolCallCountsOnce(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"read"}, "allJobs": {"true"}})
	session := mcpSession(t, router, token)
	before, _ := onlyKey(t)
	if _, failed := callTool(t, session, "list_jobs", map[string]any{}); failed {
		t.Fatal("list_jobs failed")
	}
	if after, _ := onlyKey(t); after.Uses != before.Uses+1 {
		t.Errorf("a tool call counted %d requests, want 1", after.Uses-before.Uses)
	}
}
