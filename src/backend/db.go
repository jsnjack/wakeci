package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"

	bolt "go.etcd.io/bbolt"
)

// DB schema
// key `jobs`
//   key `job_name`
// 	- count

// JobsBucket contains all registered jobs
// Schema (key is the name of the file):
// | defaultParams | null    |
// | desc          | New job |
// | interval      |         |
// | active        | true    |
var JobsBucket = []byte("jobs")

// disabledJobsBucket preserves disabled names when file discovery removes a job.
var disabledJobsBucket = []byte("disabledjobs")

// GlobalBucket contains information about global configuration
// - count: id of the build, increments
var GlobalBucket = []byte("global")

// HistoryBucket contains information about all executed builds
var HistoryBucket = []byte("history")

// APIKeysBucket contains API keys (JSON-encoded APIKey) by key ID. Only a
// hash of each key's secret is stored.
var APIKeysBucket = []byte("apikeys")

// GoogleAccountsBucket contains every Google account that may currently use
// wakeci (JSON-encoded googleAccount) by Principal.ID. An account is removed,
// along with its API keys, as soon as Google stops vouching for it.
var GoogleAccountsBucket = []byte("googleaccounts")

// ByteToInt convert byte to int via string
func ByteToInt(b []byte) (int, error) {
	bs := string(b)
	bi, err := strconv.Atoi(bs)
	if err != nil {
		return 0, err
	}
	return bi, nil
}

// IntToByte converts integer to byte via string
func IntToByte(i int) []byte {
	s := strconv.Itoa(i)
	return []byte(s)
}

// Itob converts int to sorted byte array
func Itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// dbFileMode keeps the db owner-only: it holds the password hash, API key
// hashes and Google refresh tokens
const dbFileMode = 0600

// CompactDB reclaims not used space in db file. Every copy it leaves behind
// (compacted db and backup) is owner-only, like the db itself.
func CompactDB() (err error) {
	currentDBFile := Config.WorkDir + "wakeci.db"
	newDBFile := Config.WorkDir + ".compacted.wakeci.db"
	oldDBFile := Config.WorkDir + "wakeci.db.backup"
	L.Warn("reclaiming unused space in database", "file", currentDBFile)
	// The current db becomes the backup by rename, keeping its mode
	err = os.Chmod(currentDBFile, dbFileMode)
	if err != nil {
		return fmt.Errorf("restrict current database permissions: %w", err)
	}
	// Open current database
	var oldDB *bolt.DB
	oldDB, err = bolt.Open(currentDBFile, dbFileMode, nil)
	if err != nil {
		return fmt.Errorf("open current database: %w", err)
	}
	defer func() {
		if oldDB == nil {
			return
		}
		if closeErr := oldDB.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close current database: %w", closeErr)
		}
	}()

	// Open compacted database
	err = os.Remove(newDBFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale compacted database: %w", err)
	}
	var newDB *bolt.DB
	newDB, err = bolt.Open(newDBFile, dbFileMode, nil)
	if err != nil {
		return fmt.Errorf("open compacted database: %w", err)
	}
	defer func() {
		if newDB == nil {
			return
		}
		if closeErr := newDB.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close compacted database: %w", closeErr)
		}
	}()

	// Compact
	err = bolt.Compact(newDB, oldDB, 0)
	if err != nil {
		return fmt.Errorf("compact database: %w", err)
	}

	// Report and clean up
	err = newDB.Close()
	if err != nil {
		return fmt.Errorf("close compacted database: %w", err)
	}
	newDB = nil
	err = oldDB.Close()
	if err != nil {
		return fmt.Errorf("close current database: %w", err)
	}
	oldDB = nil

	currentStat, err := os.Stat(currentDBFile)
	if err != nil {
		return fmt.Errorf("stat current database: %w", err)
	}

	newStat, err := os.Stat(newDBFile)
	if err != nil {
		return fmt.Errorf("stat compacted database: %w", err)
	}

	ratio := float64(currentStat.Size()) / float64(newStat.Size())
	L.Warn("db file size changed",
		"before", currentStat.Size(), "after", newStat.Size(), "ratio", ratio,
	)

	// Create a backup copy of the current db
	err = os.Rename(currentDBFile, oldDBFile)
	if err != nil {
		return fmt.Errorf("back up current database: %w", err)
	}

	// Replace db with the compacted version
	err = os.Rename(newDBFile, currentDBFile)
	if err != nil {
		return fmt.Errorf("install compacted database: %w", err)
	}

	return nil
}
