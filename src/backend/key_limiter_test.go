package main

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestKeyRequestLimiter(t *testing.T) {
	limiter := newKeyRequestLimiter(3, 2)
	now := time.Now()
	for i := range 3 {
		if wait, _ := limiter.take("a", now); wait != 0 {
			t.Fatalf("request %d of the burst refused, wait %v", i+1, wait)
		}
	}
	wait, started := limiter.take("a", now)
	if wait != 500*time.Millisecond || !started {
		t.Errorf("after the burst: wait %v, started %t; want 500ms and the first refusal", wait, started)
	}
	if _, started := limiter.take("a", now); started {
		t.Error("a second refusal counted as the first")
	}
	if wait, _ := limiter.take("b", now); wait != 0 {
		t.Error("another key was limited too")
	}
	// Two requests a second come back
	if wait, _ := limiter.take("a", now.Add(time.Second)); wait != 0 {
		t.Errorf("a second later: wait %v, want none", wait)
	}
	// Buckets unused for a while are dropped, and a new one is full
	later := now.Add(apiKeyBucketIdle + time.Minute)
	limiter.take("c", later)
	if _, ok := limiter.buckets["a"]; ok {
		t.Error("an idle bucket was kept")
	}
}

// TestKeyRequestLimiterKeepsEmptyBuckets is a regression test: buckets
// unused for apiKeyBucketIdle were dropped, so with a burst larger than that
// time refills, an exhausted key got the whole burst back
func TestKeyRequestLimiterKeepsEmptyBuckets(t *testing.T) {
	limiter := newKeyRequestLimiter(1000, 1)
	now := time.Now()
	for range 1000 {
		limiter.take("a", now)
	}
	limiter.take("b", now)
	later := now.Add(apiKeyBucketIdle + time.Minute)
	allowed := 0
	for range 1000 {
		if wait, _ := limiter.take("a", later); wait == 0 {
			allowed++
		}
	}
	if refilled := int((apiKeyBucketIdle + time.Minute).Seconds()); allowed != refilled {
		t.Errorf("after %v idle an exhausted key sent %d requests, want the %d refilled", apiKeyBucketIdle+time.Minute, allowed, refilled)
	}
	// A bucket that would be full again is dropped
	if _, kept := limiter.buckets["b"]; kept {
		t.Error("a bucket that refilled was kept")
	}
}

func TestAPIKeyTooManyRequests(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	old := apiKeyRequestLimiter
	apiKeyRequestLimiter = newKeyRequestLimiter(2, 1)
	t.Cleanup(func() { apiKeyRequestLimiter = old })
	token := createTestKey(t, url.Values{"name": {"busy"}, "scope": {"read"}, "allJobs": {"true"}})
	other := createTestKey(t, url.Values{"name": {"calm"}, "scope": {"read"}, "allJobs": {"true"}})

	for i := range 2 {
		if rec := keyRequest(router, "GET", "/api/jobs/", token, nil); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	rec := keyRequest(router, "GET", "/api/jobs/", token, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
		t.Errorf("third request: %d, Retry-After %q; want 429 and 1", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := keyRequest(router, "GET", "/api/jobs/", other, nil); rec.Code != http.StatusOK {
		t.Errorf("another key: %d, want 200", rec.Code)
	}
}
