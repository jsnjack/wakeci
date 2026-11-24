package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Cancel groups (docs/wake.md): with cancel_group: tests/$BRANCH, a build
// cancels the older builds whose key is the same, of every job with that
// group, with the builds they started. The later build wins: a build queued
// after a newer one with its key cancels itself. Builds of one group never
// cancel each other, so a build's own children can share its key.
//
// concurrency_key makes concurrency count the running builds of every job
// with the same key, also those running their hooks after an abort, so a
// build waits until the build it cancelled has cleaned up.

// queueKeys are a build's keys, worked out from its params before it is
// queued, never changed after
type queueKeys struct {
	// cancelKey is the expanded cancel_group, "" if the build cancels nothing
	cancelKey string
	// concurrencyKey is the expanded concurrency_key, "" for none
	concurrencyKey string
	// root is the first build of the build's group, the build itself if it
	// is in none
	root int
}

// supersede is what Queue.Add found for a build with a cancel key: the older
// builds it cancels, or the newer build that cancels it
type supersede struct {
	older []*Build
	newer *Build
}

// setQueueKeys works out the build's keys. A key that names a param that is
// empty or missing is no key: a build without a branch cancels nothing.
func (b *Build) setQueueKeys() {
	b.setKeys(true)
}

// setHookKeys works out the keys of a build that only runs its hooks
// (FinishInterruptedBuilds): its concurrency key, but no cancel key, since
// it cancels no other, so cancel_unless doesn't run before startup ends
func (b *Build) setHookKeys() {
	b.setKeys(false)
}

func (b *Build) setKeys(cancels bool) {
	b.mutex.Lock()
	values := map[string]string{}
	for _, param := range b.Params {
		for name, value := range param {
			values[name] = value
		}
	}
	root := b.group.Root
	b.mutex.Unlock()
	if root == 0 {
		root = b.ID
	}
	keys := queueKeys{root: root}
	keys.concurrencyKey, _ = expandKey(b.Job.ConcurrencyKey, values)
	if key, ok := expandKey(b.Job.CancelGroup, values); ok && cancels {
		spared, err := b.cancelSpared(values)
		switch {
		case err != nil:
			b.Logger.Warn("cancel_unless failed, so the build cancels no other", "err", err)
		case spared:
			b.Logger.Info("cancel_unless is true, so the build cancels no other", "cancelGroup", key)
		default:
			keys.cancelKey = key
		}
	}
	b.mutex.Lock()
	b.keys, b.keysSet = keys, true
	b.mutex.Unlock()
}

// setRestoredKeys gives a queued build that a restart restored the cancel
// key it was queued with, so cancel_unless doesn't run again before startup
// ends; a record without it (an older version's) works it out again
func (b *Build) setRestoredKeys() {
	if b.savedCancelKey == nil {
		b.setQueueKeys()
		return
	}
	b.setKeys(false)
	b.mutex.Lock()
	b.keys.cancelKey = *b.savedCancelKey
	b.mutex.Unlock()
}

// expandKey fills the params into key; ok is false if key is empty or names
// a param without a value
func expandKey(key string, values map[string]string) (string, bool) {
	if key == "" {
		return "", false
	}
	ok := true
	expanded := os.Expand(key, func(name string) string {
		value := values[name]
		if value == "" {
			ok = false
		}
		return value
	})
	expanded = strings.TrimSpace(expanded)
	return expanded, ok && expanded != ""
}

// cancelSpared runs cancel_unless, with the build's params in its
// environment, and reports whether it succeeded
func (b *Build) cancelSpared(values map[string]string) (bool, error) {
	if b.Job.CancelUnless == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), WHEN_EVAL_TIMEOUT*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", b.Job.CancelUnless)
	cmd.Env = append(os.Environ(), "WAKE_BUILD_ID="+strconv.Itoa(b.ID), "WAKE_JOB_NAME="+b.Job.Name)
	for name, value := range values {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && ctx.Err() == nil {
		return false, nil
	}
	if ctx.Err() != nil {
		return false, fmt.Errorf("cancel_unless ran longer than %d s", WHEN_EVAL_TIMEOUT)
	}
	return false, err
}

// supersedeLocked finds the builds in the queue that share b's cancel key,
// outside b's group: the older ones b cancels, and the newest of the newer
// ones, which cancels b. The caller holds q.mutex.
func (q *Queue) supersedeLocked(b *Build) supersede {
	var found supersede
	if b.keys.cancelKey == "" {
		return found
	}
	for _, list := range [][]*Build{q.queued, q.running} {
		for _, other := range list {
			if other == b || other.keys.cancelKey != b.keys.cancelKey || other.keys.root == b.keys.root {
				continue
			}
			if other.ID < b.ID {
				found.older = append(found.older, other)
			} else if found.newer == nil || other.ID > found.newer.ID {
				found.newer = other
			}
		}
	}
	return found
}

// apply aborts what supersedeLocked found for b, outside q.mutex: b itself
// if a newer build has its key, or else the older builds with the builds
// they started. It records the result on b for the run's audit event.
func (q *Queue) applySupersede(b *Build, found supersede) {
	key := b.keys.cancelKey
	if found.newer != nil {
		b.Logger.Info("build superseded by a newer one", "by", found.newer.ID, "cancelGroup", key)
		if err := q.AbortBy(b.ID, StatusAborted, fmt.Sprintf("as superseded by #%d (%s)", found.newer.ID, key)); err == nil {
			b.supersededBy = found.newer.ID
		}
		return
	}
	for _, old := range found.older {
		if q.AbortBy(old.ID, StatusAborted, fmt.Sprintf("as superseded by #%d (%s)", b.ID, key)) != nil {
			continue
		}
		old.Logger.Info("build superseded by a newer one", "by", b.ID, "cancelGroup", key)
		b.cancelled = append(b.cancelled, old.ID)
		for _, id := range startedUnder(old) {
			if q.AbortBy(id, StatusAborted, fmt.Sprintf("as part of #%d, superseded by #%d (%s)", old.ID, b.ID, key)) == nil {
				b.cancelled = append(b.cancelled, id)
			}
		}
	}
}

// startedUnder returns the builds that build started, and the builds they
// started, in the order they were created. It reads the group's members
// directly, not through readGroup, which needs the root's history entry:
// cleanup may have removed the root while its chain still runs.
func startedUnder(build *Build) []int {
	under := map[int]bool{build.ID: true}
	var ids []int
	err := DB.View(func(tx *bolt.Tx) error {
		history := tx.Bucket(HistoryBucket)
		// In ID order, so a parent comes before the builds it started
		for _, id := range groupMembers(tx, build.keys.root) {
			data := history.Get(Itob(id))
			if data == nil {
				continue
			}
			var member struct {
				Parent int `json:"parent"`
			}
			if err := json.Unmarshal(data, &member); err != nil {
				return fmt.Errorf("decode build %d: %w", id, err)
			}
			if under[member.Parent] && !under[id] {
				under[id] = true
				ids = append(ids, id)
			}
		}
		return nil
	})
	if err != nil {
		build.Logger.Error("read the builds this build started", "err", err)
	}
	return ids
}

// concurrencyFull reports whether build must wait for a slot of its own:
// with a concurrency key, while as many builds with that key run, their
// hooks after an abort included, as concurrency allows (1 if unset);
// without, while as many builds of its job run. The caller holds q.mutex.
func (q *Queue) concurrencyFull(build *Build) bool {
	if build.keys.concurrencyKey == "" {
		if build.Job.Concurrency == 0 {
			return false
		}
		parallel := 0
		for _, other := range q.running {
			if other.Job.Name == build.Job.Name {
				parallel++
			}
		}
		return parallel >= build.Job.Concurrency
	}
	parallel := 0
	for _, list := range [][]*Build{q.running, q.aborting} {
		for _, other := range list {
			if other.keys.concurrencyKey == build.keys.concurrencyKey {
				parallel++
			}
		}
	}
	return parallel >= max(build.Job.Concurrency, 1)
}
