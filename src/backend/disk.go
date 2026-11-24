package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
)

// Disk use per job (job insights): a build's size on disk is measured right
// after it left the queue and kept in its stats record; the insights add up the job's builds
// still in the history, whose files cleanup hasn't removed. Builds from
// before wakeci measured them are measured once after startup.

// buildDisk is what a build keeps on disk, in bytes: its logs and other
// files in the wakespace, its artifacts, and its workspace
type buildDisk struct {
	Logs      int64 `json:"logs"`
	Artifacts int64 `json:"artifacts"`
	Workspace int64 `json:"workspace"`
}

// measureBuildDisk adds up the sizes of build id's files
func measureBuildDisk(id int) *buildDisk {
	return measureDirs(wakespaceDir(id), Config.WorkDir+"workspace/"+strconv.Itoa(id)+"/")
}

// measureDirs adds up the sizes of the files of a build's wakespace and
// workspace
func measureDirs(wakespace, workspace string) *buildDisk {
	disk := &buildDisk{}
	artifacts := wakespace + "artifacts" + string(filepath.Separator)
	for size, path := range walkFiles(wakespace) {
		if strings.HasPrefix(path, artifacts) {
			disk.Artifacts += size
		} else {
			disk.Logs += size
		}
	}
	for size := range walkFiles(workspace) {
		disk.Workspace += size
	}
	return disk
}

// walkFiles yields the size and path of each regular file under dir; what
// can't be read counts as nothing
func walkFiles(dir string) func(yield func(int64, string) bool) {
	return func(yield func(int64, string) bool) {
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || !entry.Type().IsRegular() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			if !yield(info.Size(), path) {
				return fs.SkipAll
			}
			return nil
		})
	}
}

// diskMeasures lets one build be measured at a time, so builds that end
// together don't walk their files at once
var diskMeasures sync.Mutex

// saveDiskLater measures build b, which has left the queue, in the
// background and saves its size in its stats record. A walk of a large
// workspace takes long, so the build doesn't hold its place in the queue
// for it. A stop may close the database first; BackfillBuildDisk then
// measures the build at the next start. What it needs is read now, so the
// walk doesn't read globals a restart (or a test) changes.
func saveDiskLater(b *Build) {
	db, id, logger := DB, b.ID, b.Logger
	wakespace, workspace := b.GetWakespaceDir(), b.GetWorkspaceDir()
	if db == nil {
		return
	}
	go func() {
		diskMeasures.Lock()
		defer diskMeasures.Unlock()
		disk := measureDirs(wakespace, workspace)
		err := db.Update(func(tx *bolt.Tx) error {
			record := getBuildStats(tx, id)
			if record == nil {
				return nil
			}
			record.Disk = disk
			data, err := json.Marshal(record)
			if err != nil {
				return err
			}
			return putBuildStats(tx, data, id)
		})
		switch {
		case errors.Is(err, berrors.ErrDatabaseNotOpen):
			logger.Debug("save build size: wakeci stopped first")
		case err != nil:
			logger.Error("save build size", "err", err)
		}
	}()
}

// diskBackfillBatch is how many builds one transaction of the backfill
// saves, so a long history doesn't hold the write lock for long
const diskBackfillBatch = 100

// BackfillBuildDisk measures the ended builds in the history whose stats
// record has no size yet. It runs once, in the background, after startup,
// and stops between batches once ctx is done (wakeci stops).
func BackfillBuildDisk(ctx context.Context) {
	var ids []int
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(HistoryBucket).ForEach(func(k, _ []byte) error {
			id := int(binary.BigEndian.Uint64(k))
			if record := getBuildStats(tx, id); record != nil && record.Disk == nil && record.final() {
				ids = append(ids, id)
			}
			return nil
		})
	})
	if err != nil {
		L.Error("find builds to measure", "err", err)
		return
	}
	started := time.Now()
	for len(ids) > 0 && ctx.Err() == nil {
		batch := ids[:min(diskBackfillBatch, len(ids))]
		ids = ids[len(batch):]
		sizes := make(map[int]*buildDisk, len(batch))
		for _, id := range batch {
			// Cleanup may have removed it meanwhile; then there's nothing
			if !GlobalQueue.Verify(id) {
				sizes[id] = measureBuildDisk(id)
			}
		}
		err := DB.Update(func(tx *bolt.Tx) error {
			for id, disk := range sizes {
				record := getBuildStats(tx, id)
				if record == nil || record.Disk != nil || tx.Bucket(HistoryBucket).Get(Itob(id)) == nil {
					continue
				}
				record.Disk = disk
				data, err := json.Marshal(record)
				if err != nil {
					return err
				}
				if err := putBuildStats(tx, data, id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			L.Error("save build sizes", "err", err)
			return
		}
	}
	L.Debug("measured the builds from before sizes were recorded", "took", time.Since(started))
}

// JobDisk is what a job's builds in the history keep on disk, as measured
// when each ended
type JobDisk struct {
	// Builds counts the builds measured; Unmeasured those not measured yet
	// (just after an update) or that ended before their stats were kept
	Builds     int   `json:"builds"`
	Unmeasured int   `json:"unmeasured"`
	Logs       int64 `json:"logs"`
	Artifacts  int64 `json:"artifacts"`
	Workspace  int64 `json:"workspace"`
}

// jobDisk adds up the sizes of builds ids
func jobDisk(tx *bolt.Tx, ids []int) JobDisk {
	var total JobDisk
	for _, id := range ids {
		record := getBuildStats(tx, id)
		if record == nil || record.Disk == nil {
			total.Unmeasured++
			continue
		}
		total.Builds++
		total.Logs += record.Disk.Logs
		total.Artifacts += record.Disk.Artifacts
		total.Workspace += record.Disk.Workspace
	}
	return total
}
