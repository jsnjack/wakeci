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
		if err := pruneBuildStats(tx, time.Now(), cl.Logger); err != nil {
			return err
		}
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

// CleanupJobsBucket verifies that items in jobs bucket have job files in
// config dir
func CleanupJobsBucket() {
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(JobsBucket)
		c := b.Cursor()
		var toRemove [][]byte
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			name := string(key)
			path := Config.JobDir + name + Config.jobsExt
			_, err := os.Stat(path)
			if err != nil {
				L.Info("removing job from database", "job", name, "err", err)
				toRemove = append(toRemove, key)
			}
		}
		for _, rk := range toRemove {
			err := b.DeleteBucket(rk)
			if err != nil {
				L.Error("delete job bucket", "job", string(rk), "err", err)
			}
		}
		return nil
	})
	if err != nil {
		L.Error("clean up jobs bucket", "err", err)
	}
}
