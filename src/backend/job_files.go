package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	bolt "go.etcd.io/bbolt"
)

var jobFilesMu sync.Mutex

// writeJobFile publishes complete files, never a truncated intermediate version.
func writeJobFile(path string, data []byte, create bool) error {
	jobFilesMu.Lock()
	defer jobFilesMu.Unlock()
	mode := os.FileMode(0644)
	if !create {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat job before save: %w", err)
		}
		mode = info.Mode().Perm()
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".wakeci-job-*")
	if err != nil {
		return fmt.Errorf("create temporary job file: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil {
				L.Log(context.Background(), LevelTrace, "close temporary job file", "err", err)
			}
		}
		if err := os.Remove(file.Name()); err != nil && !os.IsNotExist(err) {
			L.Log(context.Background(), LevelTrace, "remove temporary job file", "err", err)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set job file mode: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write temporary job file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync temporary job file: %w", err)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("close completed job file: %w", err)
	}
	if create {
		// Link fails if another creator already published this name.
		err = os.Link(file.Name(), path)
	} else {
		err = os.Rename(file.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("publish job file: %w", err)
	}
	return nil
}

func deleteJobFile(path string) error {
	jobFilesMu.Lock()
	defer jobFilesMu.Unlock()
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove job file: %w", err)
	}
	name := GetJobNameFromPath(path)
	err := DB.Update(func(tx *bolt.Tx) error {
		if err := deleteJobRegistration(tx, tx.Bucket(JobsBucket), name); err != nil {
			return err
		}
		return setJobDisabled(tx, name, false)
	})
	if err != nil {
		return fmt.Errorf("delete job state %q: %w", name, err)
	}
	RemoveJobFromCron(name)
	return nil
}

func setJobDisabled(tx *bolt.Tx, name string, disabled bool) error {
	bucket := tx.Bucket(disabledJobsBucket)
	if !disabled {
		if bucket == nil {
			return nil
		}
		if err := bucket.Delete([]byte(name)); err != nil {
			return fmt.Errorf("clear disabled job state: %w", err)
		}
		return nil
	}
	var err error
	bucket, err = tx.CreateBucketIfNotExists(disabledJobsBucket)
	if err != nil {
		return fmt.Errorf("create disabled job state: %w", err)
	}
	if err := bucket.Put([]byte(name), []byte{1}); err != nil {
		return fmt.Errorf("store disabled job state: %w", err)
	}
	return nil
}

func deleteJobRegistration(tx *bolt.Tx, jobs *bolt.Bucket, name string) error {
	job := jobs.Bucket([]byte(name))
	if job == nil {
		return nil
	}
	if string(job.Get([]byte("active"))) == "false" {
		if err := setJobDisabled(tx, name, true); err != nil {
			return err
		}
	}
	if err := jobs.DeleteBucket([]byte(name)); err != nil {
		return fmt.Errorf("delete job registration: %w", err)
	}
	return nil
}
