package main

import (
	"math"
	"time"

	"github.com/sasha-s/go-deadlock"
)

// What one API key may send: a burst, refilled at a steady rate. Well above
// what clients send today (bursts of about 50 requests), so it only slows
// down a loop that went wrong, and other keys never notice.
const (
	apiKeyBurst = 120
	// apiKeyRate is requests per second
	apiKeyRate = 10
	// apiKeyBucketIdle is how long a key's bucket is kept unused; a full
	// bucket is the same as none
	apiKeyBucketIdle = 10 * time.Minute
)

type tokenBucket struct {
	tokens float64
	last   time.Time
	// limited is true while the key is refused, so it is logged once
	limited bool
}

// keyRequestLimiter limits the requests of each API key
type keyRequestLimiter struct {
	mu        deadlock.Mutex
	burst     float64
	rate      float64
	buckets   map[string]*tokenBucket
	lastSweep time.Time
}

func newKeyRequestLimiter(burst, rate float64) *keyRequestLimiter {
	return &keyRequestLimiter{burst: burst, rate: rate, buckets: make(map[string]*tokenBucket)}
}

// apiKeyRequestLimiter limits every API key's requests
var apiKeyRequestLimiter = newKeyRequestLimiter(apiKeyBurst, apiKeyRate)

// take uses one request of the key id. It returns 0 if the request may go
// on, or how long to wait; started is true for the first refusal after the
// key was let through.
func (l *keyRequestLimiter) take(id string, now time.Time) (wait time.Duration, started bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > apiKeyBucketIdle {
		for key, bucket := range l.buckets {
			if now.Sub(bucket.last) > apiKeyBucketIdle {
				delete(l.buckets, key)
			}
		}
		l.lastSweep = now
	}
	bucket, ok := l.buckets[id]
	if !ok {
		bucket = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[id] = bucket
	}
	bucket.tokens = math.Min(l.burst, bucket.tokens+now.Sub(bucket.last).Seconds()*l.rate)
	bucket.last = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		bucket.limited = false
		return 0, false
	}
	started = !bucket.limited
	bucket.limited = true
	return time.Duration((1 - bucket.tokens) / l.rate * float64(time.Second)), started
}
