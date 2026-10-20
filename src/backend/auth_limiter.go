package main

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	authMaxFailures   = 100
	authFailureWindow = 5 * time.Minute
	authBlockDuration = 5 * time.Minute
	// authPruneAt is how many tracked clients trigger dropping stale ones
	authPruneAt = 10000
)

type authAttempt struct {
	failures     int
	lastFailure  time.Time
	blockedUntil time.Time
}

type authAttemptLimiter struct {
	mu            sync.Mutex
	attempts      map[string]authAttempt
	maxFailures   int
	failureWindow time.Duration
	blockDuration time.Duration
}

var globalAuthAttemptLimiter = newAuthAttemptLimiter(
	authMaxFailures,
	authFailureWindow,
	authBlockDuration,
)

func newAuthAttemptLimiter(maxFailures int, failureWindow time.Duration, blockDuration time.Duration) *authAttemptLimiter {
	return &authAttemptLimiter{
		attempts:      make(map[string]authAttempt),
		maxFailures:   maxFailures,
		failureWindow: failureWindow,
		blockDuration: blockDuration,
	}
}

func (l *authAttemptLimiter) retryAfter(client string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, ok := l.attempts[client]
	if !ok || attempt.blockedUntil.IsZero() {
		return 0, false
	}
	retryAfter := attempt.blockedUntil.Sub(now)
	if retryAfter <= 0 {
		delete(l.attempts, client)
		return 0, false
	}
	return retryAfter, true
}

func (l *authAttemptLimiter) failure(client string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt := l.attempts[client]
	if retryAfter := attempt.blockedUntil.Sub(now); retryAfter > 0 {
		return retryAfter, true
	}
	if attempt.lastFailure.IsZero() || now.Sub(attempt.lastFailure) >= l.failureWindow {
		attempt.failures = 0
	}
	attempt.failures++
	attempt.lastFailure = now
	if attempt.failures >= l.maxFailures {
		attempt.blockedUntil = now.Add(l.blockDuration)
	}
	l.attempts[client] = attempt
	if len(l.attempts) >= authPruneAt {
		l.prune(now)
	}

	if attempt.blockedUntil.IsZero() {
		return 0, false
	}
	return l.blockDuration, true
}

// prune forgets clients whose failures have expired and who aren't
// blocked. Successes don't reset a client's failures: behind a shared proxy
// or NAT, one user's success would wipe out another's failed guesses.
func (l *authAttemptLimiter) prune(now time.Time) {
	for client, attempt := range l.attempts {
		if now.Sub(attempt.lastFailure) >= l.failureWindow && !now.Before(attempt.blockedUntil) {
			delete(l.attempts, client)
		}
	}
}

// authClient is who failed attempts are counted against: the client's
// address, or its /64 for IPv6, which one host usually holds whole
func authClient(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return host
}

func rejectRateLimited(w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	now := time.Now()
	retryAfter, limited := globalAuthAttemptLimiter.retryAfter(authClient(r), now)
	if !limited {
		return false
	}
	writeRateLimitResponse(w, logger, retryAfter, now.Add(retryAfter))
	return true
}

func recordAuthFailure(w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	now := time.Now()
	retryAfter, limited := globalAuthAttemptLimiter.failure(authClient(r), now)
	if !limited {
		return false
	}
	writeRateLimitResponse(w, logger, retryAfter, now.Add(retryAfter))
	return true
}

func writeRateLimitResponse(
	w http.ResponseWriter,
	logger *slog.Logger,
	retryAfter time.Duration,
	resetAt time.Time,
) {
	seconds := max(1, int((retryAfter+time.Second-1)/time.Second))
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	w.Header().Set("RateLimit-Reset", strconv.Itoa(seconds))
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(globalAuthAttemptLimiter.maxFailures))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
	w.WriteHeader(http.StatusTooManyRequests)
	writeBody(logger, w, []byte("Too many authentication attempts"))
}
