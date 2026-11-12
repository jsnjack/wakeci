package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Build stats are one small record per build in BuildStatsBucket, which the
// analytics page reads. Cleanup deletes a build's history entry once it falls
// out of the last buildHistorySize builds, but keeps its record until it is
// statsRetention old, so the page can show months. A build saves its record
// with each history entry, in the same transaction (BroadcastUpdate).

// statsRetention is how long a build's record is kept
const statsRetention = 400 * 24 * time.Hour

// Who or what started a build
const (
	// TriggerInterval is the job's interval (cron)
	TriggerInterval = "interval"
	// TriggerPassword is the admin password, in a session or Basic auth
	TriggerPassword = "password"
	// TriggerSSO is a single sign-on user
	TriggerSSO = "sso"
	// TriggerAPIKey is an API key, also through MCP
	TriggerAPIKey = "apikey"
	// TriggerUnknown names the builds from before wakeci recorded who
	// started them, which are stored with an empty kind
	TriggerUnknown = "unknown"
)

// triggerKinds are the kinds a query may ask for
var triggerKinds = []string{TriggerInterval, TriggerPassword, TriggerSSO, TriggerAPIKey, TriggerUnknown}

// buildTrigger is who or what started a build
type buildTrigger struct {
	// Kind is one of the Trigger constants, empty if unknown
	Kind string `json:"kind,omitempty"`
	// User is the single sign-on email, or who created the API key
	User string `json:"user,omitempty"`
	// APIKey is the key's ID
	APIKey string `json:"apiKey,omitempty"`
}

// triggerOf returns who made the request, as its auth middleware recorded it
func triggerOf(r *http.Request) buildTrigger {
	info := requestInfoOf(r)
	switch info.auth {
	case "password", "basic":
		return buildTrigger{Kind: TriggerPassword}
	case "sso":
		return buildTrigger{Kind: TriggerSSO, User: info.user}
	case "apikey":
		return buildTrigger{Kind: TriggerAPIKey, User: info.user, APIKey: info.apiKey}
	}
	return buildTrigger{}
}

// buildStats is a build's record in BuildStatsBucket
type buildStats struct {
	ID     int        `json:"id"`
	Job    string     `json:"job"`
	Status ItemStatus `json:"status"`
	// QueuedAt is when the build was created; zero for builds from before
	// wakeci recorded it
	QueuedAt  time.Time     `json:"queuedAt,omitzero"`
	StartedAt time.Time     `json:"startedAt,omitzero"`
	Duration  time.Duration `json:"duration,omitempty"`
	// Ended is set once the build's on_* and finally tasks ran, when
	// Duration is final
	Ended   bool         `json:"ended,omitempty"`
	Trigger buildTrigger `json:"trigger,omitzero"`
	// FailedTask is the name of the main task that stopped the build
	FailedTask string `json:"failedTask,omitempty"`
}

// at is when the build was queued, or started for builds from before wakeci
// recorded the first; zero if neither is known
func (s *buildStats) at() time.Time {
	if !s.QueuedAt.IsZero() {
		return s.QueuedAt
	}
	return s.StartedAt
}

// final reports whether the build has its last status
func (s *buildStats) final() bool {
	switch s.Status {
	case StatusFinished, StatusFailed, StatusAborted, StatusTimedOut:
		return true
	}
	return false
}

// wait returns how long the build waited in the queue, if known
func (s *buildStats) wait() (time.Duration, bool) {
	if s.QueuedAt.IsZero() || s.StartedAt.IsZero() {
		return 0, false
	}
	return max(0, s.StartedAt.Sub(s.QueuedAt)), true
}

// triggerKind is the record's trigger kind, TriggerUnknown if it has none
func (s *buildStats) triggerKind() string {
	if s.Trigger.Kind == "" {
		return TriggerUnknown
	}
	return s.Trigger.Kind
}

// putBuildStats saves a build's record. Databases made by tests may lack the
// bucket; wakeci always creates it (bootstrapDB)
func putBuildStats(tx *bolt.Tx, data []byte, id int) error {
	b := tx.Bucket(BuildStatsBucket)
	if b == nil {
		return nil
	}
	return b.Put(Itob(id), data)
}

// getBuildStats returns a build's record, nil if it has none
func getBuildStats(tx *bolt.Tx, id int) *buildStats {
	b := tx.Bucket(BuildStatsBucket)
	if b == nil {
		return nil
	}
	data := b.Get(Itob(id))
	if data == nil {
		return nil
	}
	var record buildStats
	if err := json.Unmarshal(data, &record); err != nil {
		return nil
	}
	return &record
}

// statsFromHistory makes a build's record from its history entry, for builds
// that ended before wakeci recorded stats. failedTask names the main task
// that stopped it, if known
func statsFromHistory(msg *BuildUpdateData, failedTask string) *buildStats {
	return &buildStats{
		ID:         msg.ID,
		Job:        msg.Name,
		Status:     msg.Status,
		StartedAt:  msg.StartedAt,
		Duration:   msg.Duration,
		Ended:      true,
		FailedTask: failedTask,
	}
}

// stoppedTask returns the ID of the main task that failed, timed out or was
// aborted, and false if none did
func stoppedTask(tasks []*TaskStatus) (int, bool) {
	for _, task := range tasks {
		if task.Kind != KindMain {
			continue
		}
		switch task.Status {
		case StatusFailed, StatusTimedOut, StatusAborted:
			return task.ID, true
		}
	}
	return 0, false
}

// taskName returns the name of the task with the ID in the build's plan, ""
// if the plan is gone
func taskName(buildID, taskID int) string {
	job, err := getBuildConfig(buildID)
	if err != nil {
		return ""
	}
	for _, task := range job.Tasks {
		if task.ID == taskID {
			return displayTaskName(task)
		}
	}
	return ""
}

// displayTaskName is a task's name, or its position for a task without one
func displayTaskName(task *Task) string {
	if task.Name != "" {
		return task.Name
	}
	return fmt.Sprintf("task %d", task.ID)
}

// BackfillBuildStats records the builds in the history that have no record
// yet: those that ended before wakeci recorded stats, or while an older
// version ran after a downgrade. Call it after MarkInterruptedBuilds, so
// that interrupted builds are aborted. It reads the history and the build
// plans first and writes once, so it holds the write lock only to save
func BackfillBuildStats() error {
	var missing []*BuildUpdateData
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(HistoryBucket).ForEach(func(k, v []byte) error {
			if getBuildStats(tx, int(binary.BigEndian.Uint64(k))) != nil {
				return nil
			}
			var msg BuildUpdateData
			if err := json.Unmarshal(v, &msg); err != nil {
				L.Error("decode build history entry", "build", binary.BigEndian.Uint64(k), "err", err)
				return nil
			}
			missing = append(missing, &msg)
			return nil
		})
	})
	if err != nil {
		return err
	}
	records := make([]*buildStats, 0, len(missing))
	for _, msg := range missing {
		// Builds aborted while queued have no time to count them on
		if msg.StartedAt.IsZero() {
			continue
		}
		failedTask := ""
		if id, ok := stoppedTask(msg.Tasks); ok {
			failedTask = taskName(msg.ID, id)
		}
		records = append(records, statsFromHistory(msg, failedTask))
	}
	if len(records) == 0 {
		return nil
	}
	err = DB.Update(func(tx *bolt.Tx) error {
		for _, record := range records {
			// A build that is queued again or running writes its own
			if getBuildStats(tx, record.ID) != nil || !record.final() {
				continue
			}
			data, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if err := putBuildStats(tx, data, record.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	L.Info("recorded stats of earlier builds", "builds", len(records))
	return nil
}

// markStatsAborted saves the record of a build that the history now has as
// aborted, keeping who started it and when it was queued
func markStatsAborted(tx *bolt.Tx, msg *BuildUpdateData) error {
	record := getBuildStats(tx, msg.ID)
	if record == nil {
		if msg.StartedAt.IsZero() {
			return nil
		}
		record = statsFromHistory(msg, "")
	}
	record.Status = StatusAborted
	record.Ended = true
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return putBuildStats(tx, data, msg.ID)
}

// pruneBuildStats deletes the records that are older than statsRetention.
// Records are in the order builds were created, so it stops at the first
// newer one. Records without a time are kept, as no age can be told
func pruneBuildStats(tx *bolt.Tx, now time.Time, logger *slog.Logger) error {
	b := tx.Bucket(BuildStatsBucket)
	if b == nil {
		return nil
	}
	cutoff := now.Add(-statsRetention)
	c := b.Cursor()
	deleted := 0
	for k, v := c.First(); k != nil; {
		var record buildStats
		if err := json.Unmarshal(v, &record); err != nil {
			logger.Error("decode build stats", "build", binary.BigEndian.Uint64(k), "err", err)
			k, v = c.Next()
			continue
		}
		at := record.at()
		if at.IsZero() {
			k, v = c.Next()
			continue
		}
		if !at.Before(cutoff) {
			break
		}
		key := append([]byte(nil), k...)
		if err := c.Delete(); err != nil {
			return err
		}
		deleted++
		// After Delete, Next would skip a key
		k, v = c.Seek(key)
	}
	if deleted > 0 {
		logger.Info("deleted old build stats", "builds", deleted)
	}
	return nil
}
