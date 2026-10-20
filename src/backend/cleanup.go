package main

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// BuildCleanupPeriod is a period to clean up old builds
const BuildCleanupPeriod = 15 * time.Minute

// Cleaner respresents a struct to schdeule old build cleanups
type Cleaner struct {
	Logger *slog.Logger
}

// Clean removes old builds from filesystem and database
func (cl *Cleaner) Clean() {
	cl.Logger.Debug("looking for builds to clean up")
	started := time.Now()
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(GlobalBucket))

		preserve, err := ByteToInt(b.Get([]byte("buildHistorySize")))
		if err != nil {
			return err
		}

		hb := tx.Bucket([]byte(HistoryBucket))
		c := hb.Cursor()
		// Check what is the last one
		lastK, _ := c.Last()
		if lastK == nil {
			return nil
		}
		// Find starting point for removing
		fromB := make([]byte, 8)
		binary.BigEndian.PutUint64(fromB, binary.BigEndian.Uint64(lastK)-uint64(preserve))
		for key, _ := c.Seek(fromB); key != nil; key, _ = c.Prev() {
			var id = binary.BigEndian.Uint64(key)
			if id > binary.BigEndian.Uint64(fromB) {
				continue
			}
			if GlobalQueue.Verify(int(id)) {
				cl.Logger.Debug("preserving active build", "build", id)
				continue
			}
			cl.Logger.Info("cleaning up build", "build", id)
			err = os.RemoveAll(filepath.Join(Config.WorkDir, "workspace/", fmt.Sprintf("%d", id)))
			if err != nil {
				cl.Logger.Error("remove workspace", "build", id, "err", err)
			}
			err = os.RemoveAll(filepath.Join(Config.WorkDir, "wakespace/", fmt.Sprintf("%d", id)))
			if err != nil {
				cl.Logger.Error("remove wakespace", "build", id, "err", err)
			}
			err = c.Delete()
			if err != nil {
				cl.Logger.Error("delete build history", "build", id, "err", err)
			}
		}
		return nil
	})
	cl.Logger.Debug("cleanup finished", "took", time.Since(started))
	if err != nil {
		cl.Logger.Error("clean up old builds", "err", err)
		return
	}
}

// CleanupOldBuilds periodically clean ups old builds
func CleanupOldBuilds(d time.Duration) {
	ticker := time.NewTicker(d)
	c := Cleaner{
		Logger: L.With("component", "cleaner"),
	}
	go func() {
		for range ticker.C {
			c.Clean()
		}
	}()
}

// CleanupJobsBucket removes registrations whose files no longer qualify as jobs.
func CleanupJobsBucket() {
	jobFilesMu.Lock()
	defer jobFilesMu.Unlock()
	var removed []string
	err := DB.Update(func(tx *bolt.Tx) error {
		jobs := tx.Bucket(JobsBucket)
		cursor := jobs.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			name := string(key)
			file := Config.JobDir + name + Config.jobsExt
			remove := !isValidJobName(name) || Config.isProtectedFile(file)
			if !remove {
				hasTasks, err := definesTasks(file)
				switch {
				case os.IsNotExist(err):
					remove = true
				case err != nil:
					// A partially written document must not erase its registration.
					L.Warn("check registered job file", "job", name, "err", err)
				case !hasTasks:
					remove = true
				}
			}
			if remove {
				removed = append(removed, name)
			}
		}
		for _, name := range removed {
			if err := deleteJobRegistration(tx, jobs, name); err != nil {
				return fmt.Errorf("delete stale job %q: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		L.Error("clean up jobs bucket", "err", err)
		return
	}
	for _, name := range removed {
		RemoveJobFromCron(name)
		L.Info("removed stale job registration", "job", name)
	}
}
