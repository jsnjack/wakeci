package main

import (
	"encoding/binary"
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

// GlobalBucket contains information about global configuration
// - count: id of the build, increments
var GlobalBucket = []byte("global")

// HistoryBucket contains information about all executed builds
var HistoryBucket = []byte("history")

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

// CompactDB reclaims not used space in db file
func CompactDB() error {
	currentDBFile := Config.WorkDir + "wakeci.db"
	newDBFile := Config.WorkDir + ".compacted.wakeci.db"
	oldDBFile := Config.WorkDir + "wakeci.db.backup"
	Logger.Printf("Reclaiming unused space in database %s...\n", currentDBFile)
	// Open current database
	oldDB, err := bolt.Open(currentDBFile, 0644, nil)
	if err != nil {
		return err
	}

	// Open compacted database
	err = os.Remove(newDBFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	newDB, err := bolt.Open(newDBFile, 0644, nil)
	if err != nil {
		return err
	}

	// Compact
	err = bolt.Compact(newDB, oldDB, 0)
	if err != nil {
		return err
	}

	// Report and clean up
	err = newDB.Close()
	if err != nil {
		return err
	}
	err = oldDB.Close()
	if err != nil {
		return err
	}

	currentStat, err := os.Stat(currentDBFile)
	if err != nil {
		return err
	}

	newStat, err := os.Stat(newDBFile)
	if err != nil {
		return err
	}

	ratio := float64(currentStat.Size()) / float64(newStat.Size())
	Logger.Printf(
		"DB file size changed from %d to %d (%.2fx)\n",
		currentStat.Size(), newStat.Size(), ratio,
	)

	// Create a backup copy of the current db
	err = os.Rename(currentDBFile, oldDBFile)
	if err != nil {
		return err
	}

	// Replace db with the compacted version
	err = os.Rename(newDBFile, currentDBFile)
	if err != nil {
		return err
	}

	return nil
}
