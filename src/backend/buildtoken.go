package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sasha-s/go-deadlock"
)

// Build tokens (docs/wake.md): every build's tasks get WAKE_TOKEN, which the
// wake command sends to call the API as that build. A token may read and run
// every job, since the job's own commands can already reach anything on the
// server; it can't change jobs, so the builds it starts follow the rules for
// untrusted parameters. The builds it starts are its build's children. It
// stops working when its build leaves the queue, and lives in memory only,
// so a restart gives the builds it restores new tokens.

const (
	// buildTokenPrefix starts every build token: wb_<build ID>_<secret>
	buildTokenPrefix      = "wb_"
	buildTokenSecretBytes = 32
)

// buildTokens are the tokens of the builds that haven't left the queue, by
// build ID, with a hash of each secret
var buildTokens = struct {
	mu   deadlock.Mutex
	byID map[int]buildTokenEntry
}{byID: map[int]buildTokenEntry{}}

type buildTokenEntry struct {
	token string
	hash  [sha256.Size]byte
	build *Build
}

var errBuildToken = errors.New("invalid build token, or its build has ended")

// buildToken returns the build's token, created on first use, or "" once
// the build left the queue
func (b *Build) buildToken() (string, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.tokenRevoked {
		return "", nil
	}
	if b.token != "" {
		return b.token, nil
	}
	secret := make([]byte, buildTokenSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("create build token: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	b.token = buildTokenPrefix + strconv.Itoa(b.ID) + "_" + encoded
	buildTokens.mu.Lock()
	buildTokens.byID[b.ID] = buildTokenEntry{token: b.token, hash: sha256.Sum256([]byte(encoded)), build: b}
	buildTokens.mu.Unlock()
	return b.token, nil
}

// revokeBuildToken ends the build's token, as the build leaves the queue
func (b *Build) revokeBuildToken() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.tokenRevoked = true
	buildTokens.mu.Lock()
	delete(buildTokens.byID, b.ID)
	buildTokens.mu.Unlock()
}

// verifyBuildToken returns the build whose token is token
func verifyBuildToken(token string) (*Build, error) {
	rest, ok := strings.CutPrefix(token, buildTokenPrefix)
	if !ok {
		return nil, errBuildToken
	}
	idText, secret, ok := strings.Cut(rest, "_")
	id, err := strconv.Atoi(idText)
	if !ok || err != nil || id < 1 || strconv.Itoa(id) != idText {
		return nil, errBuildToken
	}
	buildTokens.mu.Lock()
	entry, found := buildTokens.byID[id]
	buildTokens.mu.Unlock()
	hash := sha256.Sum256([]byte(secret))
	if !found || subtle.ConstantTimeCompare(hash[:], entry.hash[:]) != 1 {
		return nil, errBuildToken
	}
	return entry.build, nil
}

// buildTokenValues are the tokens of the builds in the queue, which build
// logs redact like secrets: a reader of a log mustn't be able to act as the
// build while it runs
func buildTokenValues() []string {
	buildTokens.mu.Lock()
	defer buildTokens.mu.Unlock()
	values := make([]string, 0, len(buildTokens.byID))
	for _, entry := range buildTokens.byID {
		values = append(values, entry.token)
	}
	return values
}

// buildTokenAccess is what a build token may do: read and run every job
func buildTokenAccess() *Access {
	return &Access{
		Grants:  []Grant{{Scopes: []string{ScopeRead, ScopeRun}, AllJobs: true}},
		subject: "A build's token",
	}
}

type tokenBuildContextKey struct{}

// tokenBuildFromContext returns the build whose token authorized the
// request, or nil
func tokenBuildFromContext(ctx context.Context) *Build {
	build, _ := ctx.Value(tokenBuildContextKey{}).(*Build)
	return build
}
