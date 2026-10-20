package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestAuthAttemptLimiter(t *testing.T) {
	start := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)

	t.Run("blocks at failure limit and expires", func(t *testing.T) {
		limiter := newAuthAttemptLimiter(2, time.Minute, 5*time.Minute)
		if _, limited := limiter.failure("client", start); limited {
			t.Fatal("first failure unexpectedly rate limited")
		}
		if retryAfter, limited := limiter.failure("client", start.Add(time.Second)); !limited || retryAfter != 5*time.Minute {
			t.Fatalf("second failure limited = %t, retryAfter = %v; want true, 5m", limited, retryAfter)
		}
		if retryAfter, limited := limiter.retryAfter("client", start.Add(2*time.Minute)); !limited || retryAfter != 3*time.Minute+time.Second {
			t.Fatalf("active block limited = %t, retryAfter = %v; want true, 3m1s", limited, retryAfter)
		}
		if _, limited := limiter.retryAfter("client", start.Add(6*time.Minute)); limited {
			t.Fatal("expired block remained rate limited")
		}
	})

	t.Run("stale clients are pruned", func(t *testing.T) {
		limiter := newAuthAttemptLimiter(2, time.Minute, time.Minute)
		for i := range authPruneAt - 1 {
			limiter.failure(strconv.Itoa(i), start)
		}
		limiter.failure("blocked", start.Add(time.Minute))
		limiter.failure("blocked", start.Add(time.Minute))
		// The next failure finds the map full; only the blocked client and
		// the newcomer are still current
		limiter.failure("new", start.Add(90*time.Second))
		if n := len(limiter.attempts); n != 2 {
			t.Errorf("tracking %d clients after pruning, want 2", n)
		}
		if _, limited := limiter.retryAfter("blocked", start.Add(90*time.Second)); !limited {
			t.Error("pruning dropped a blocked client")
		}
	})

	t.Run("failure window resets", func(t *testing.T) {
		limiter := newAuthAttemptLimiter(2, time.Minute, time.Minute)
		limiter.failure("client", start)
		if _, limited := limiter.failure("client", start.Add(2*time.Minute)); limited {
			t.Fatal("failure outside the window unexpectedly reached the limit")
		}
	})

	t.Run("clients are isolated", func(t *testing.T) {
		limiter := newAuthAttemptLimiter(1, time.Minute, time.Minute)
		limiter.failure("blocked", start)
		if _, limited := limiter.retryAfter("other", start); limited {
			t.Fatal("one client's failures blocked another client")
		}
	})
}

func TestAuthClientGroupsIPv6By64(t *testing.T) {
	client := func(addr string) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = addr
		return authClient(req)
	}
	if a, b := client("[2001:db8:1:2::1]:1000"), client("[2001:db8:1:2:ffff::9]:2000"); a != b {
		t.Errorf("one /64 counted as %q and %q", a, b)
	}
	if a, b := client("[2001:db8:1:2::1]:1000"), client("[2001:db8:1:3::1]:1000"); a == b {
		t.Errorf("different /64s both counted as %q", a)
	}
	if got := client("192.0.2.7:1234"); got != "192.0.2.7" {
		t.Errorf("IPv4 client = %q", got)
	}
}
