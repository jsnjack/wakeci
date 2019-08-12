package main

import (
	"time"

	bolt "github.com/etcd-io/bbolt"
)

// CleanupOldBuilds periodically clean ups old builds
func CleanupOldBuilds(d time.Duration) {
	ticker := time.NewTicker(d)
	go func() {
		for range ticker.C {
			RemoveOlderBuilds()
		}
	}()
}

// RemoveOlderBuilds removes old builds from filesystem and database
func RemoveOlderBuilds() {
	// Get last build id
	var lastBuild int
	var preserve int
	err := DB.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte(GlobalBucket))

		lastBuild, err = ByteToInt(b.Get([]byte("count")))
		if err != nil {
			return err
		}

		preserve, err := ByteToInt(b.Get([]byte("buildHistorySize")))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		Logger.Println(err)
		return
	}
	Logger.Printf("Last build ID: %d\n", lastBuild)
	cleanUntil := lastBuild - preserve
	if cleanUntil > 1 {

	}
}
