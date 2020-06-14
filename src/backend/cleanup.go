package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// BuildCleanupPeriod is a period to clean up old builds
const BuildCleanupPeriod = 15 * time.Minute

// Cleaner respresents a struct to schdeule old build cleanups
type Cleaner struct {
	Logger *log.Logger
}

// Clean removes old builds from filesystem and database
func (cl *Cleaner) Clean() {
	cl.Logger.Println("Looking for builds to clean up...")
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
			cl.Logger.Printf("Cleaning up build %d...\n", id)
			err = os.RemoveAll(filepath.Join(Config.WorkDir, "workspace/", fmt.Sprintf("%d", id)))
			if err != nil {
				cl.Logger.Println(err)
			}
			err = os.RemoveAll(filepath.Join(Config.WorkDir, "wakespace/", fmt.Sprintf("%d", id)))
			if err != nil {
				cl.Logger.Println(err)
			}
			err = hb.Delete(key)
			if err != nil {
				cl.Logger.Println(err)
			}
		}
		return nil
	})
	cl.Logger.Printf("Took %s\n", time.Since(started))
	if err != nil {
		cl.Logger.Println(err)
		return
	}
}

// CleanupOldBuilds periodically clean ups old builds
func CleanupOldBuilds(d time.Duration) {
	ticker := time.NewTicker(d)
	c := Cleaner{
		Logger: log.New(os.Stdout, "[cleaner] ", log.Lmicroseconds|log.Lshortfile),
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
	DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(JobsBucket)
		c := b.Cursor()
		var toRemove [][]byte
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			name := string(key)
			path := Config.JobDir + name + Config.jobsExt
			_, err := os.Stat(path)
			if err != nil {
				Logger.Printf("Removing %s: %s\n", name, err.Error())
				toRemove = append(toRemove, key)
			}
		}
		for _, rk := range toRemove {
			err := b.DeleteBucket(rk)
			if err != nil {
				Logger.Println(err)
			}
		}
		return nil
	})
}
