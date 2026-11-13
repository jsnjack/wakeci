package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
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

// APIKeysBucket contains API keys by ID, see storedAPIKey
var APIKeysBucket = []byte("apikeys")

// QueueBucket contains the queued builds by ID, see queuedBuild, so they are
// queued again after a restart
var QueueBucket = []byte("queue")

// SessionsBucket contains the active sessions by sessionKey, see
// storedSession, so a restart doesn't sign everyone out
var SessionsBucket = []byte("sessions")

// BuildStatsBucket holds a small record of each build by ID, kept after
// cleanup deletes its history entry, see buildStats
var BuildStatsBucket = []byte("buildstats")

// SSOGroupsBucket holds the single sign-on groups that may log in and what
// their members may do, by the lowercased group name
var SSOGroupsBucket = []byte("ssogroups")

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
func CompactDB() (err error) {
	currentDBFile := Config.WorkDir + "wakeci.db"
	newDBFile := Config.WorkDir + ".compacted.wakeci.db"
	oldDBFile := Config.WorkDir + "wakeci.db.backup"
	L.Warn("reclaiming unused space in database", "file", currentDBFile)
	// Open current database
	var oldDB *bolt.DB
	oldDB, err = bolt.Open(currentDBFile, 0644, nil)
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
	newDB, err = bolt.Open(newDBFile, 0644, nil)
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

// bootstrapDB creates the buckets and the default settings that are
// missing: the password admin, 2 concurrent builds and 200 builds kept. Each
// default is written only if its setting is missing, so settings imported
// into a new database (--import-settings) keep their values.
func bootstrapDB(tx *bolt.Tx) error {
	gb, err := tx.CreateBucketIfNotExists(GlobalBucket)
	if err != nil {
		return err
	}
	if gb.Get([]byte("password")) == nil {
		L.Info("creating default password")
		passwordH, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := gb.Put([]byte("password"), passwordH); err != nil {
			return err
		}
	}
	for name, value := range map[string]int{"concurrentBuilds": 2, "buildHistorySize": 200} {
		if gb.Get([]byte(name)) == nil {
			if err := gb.Put([]byte(name), IntToByte(value)); err != nil {
				return err
			}
		}
	}
	for _, bucket := range [][]byte{JobsBucket, HistoryBucket, APIKeysBucket, QueueBucket, SessionsBucket, SSOGroupsBucket, BuildStatsBucket} {
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return err
		}
	}
	return nil
}

// dbVersionKey in GlobalBucket names the wakeci version that last opened the
// database, so a new version backs it up first (backupForNewVersion)
var dbVersionKey = []byte("dbVersion")

// dbBackupsKept is how many backups backupForNewVersion keeps
const dbBackupsKept = 3

// dbBackupTime ends a backup's name; it sorts by time
const dbBackupTime = "20060102T150405Z"

// unsafeInFileName is what a version may not put in a backup's name
var unsafeInFileName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// backUpForThisVersion backs up the database if this version of wakeci opens
// it for the first time, and logs the outcome. A failed backup doesn't stop
// wakeci from starting
func backUpForThisVersion(db *bolt.DB) {
	path, err := backupForNewVersion(db, Version, time.Now())
	if path != "" {
		L.Info("backed up the database before this version first opened it", "version", Version, "path", path)
	}
	if err != nil {
		L.Error("back up the database for a new version", "version", Version, "err", err)
	}
}

// backupForNewVersion copies the database to <db>.before-<version>-<UTC time>
// when version opens it for the first time, before anything writes to it, so
// an update can be undone with the data the old version left. It keeps the
// newest dbBackupsKept backups and returns the new one's path. A new database
// isn't copied. The version is recorded even when the copy fails, so a later
// start doesn't save the database it changed under that name
func backupForNewVersion(db *bolt.DB, version string, now time.Time) (string, error) {
	var stored []byte
	isNew := false
	if err := db.View(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		if gb == nil {
			isNew = true
			return nil
		}
		stored = slices.Clone(gb.Get(dbVersionKey))
		return nil
	}); err != nil {
		return "", fmt.Errorf("read the version that last opened the db: %w", err)
	}
	if stored != nil && string(stored) == version {
		return "", nil
	}
	var path string
	var backupErr error
	if !isNew {
		path, backupErr = writeDBBackup(db, version, now)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		gb, err := tx.CreateBucketIfNotExists(GlobalBucket)
		if err != nil {
			return err
		}
		return gb.Put(dbVersionKey, []byte(version))
	}); err != nil {
		return path, errors.Join(backupErr, fmt.Errorf("record the version that opened the db: %w", err))
	}
	return path, backupErr
}

// writeDBBackup writes a consistent copy of db next to it, under a temporary
// name until it is complete, then deletes all but the newest dbBackupsKept
func writeDBBackup(db *bolt.DB, version string, now time.Time) (string, error) {
	name := unsafeInFileName.ReplaceAllString(version, "_")
	path := db.Path() + ".before-" + name + "-" + now.UTC().Format(dbBackupTime)
	tmp := path + ".tmp"
	// Only its owner may read it, like the db it copies should be
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}
	err = db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(f)
		return err
	})
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write backup: %w", err)
	}
	if err := pruneDBBackups(db.Path()); err != nil {
		return path, fmt.Errorf("delete old backups: %w", err)
	}
	return path, nil
}

// pruneDBBackups deletes all but the newest dbBackupsKept backups of the db
// at dbPath, and copies a crash left unfinished
func pruneDBBackups(dbPath string) error {
	dir, base := filepath.Split(dbPath)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	prefix := base + ".before-"
	var backups []string
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			errs = append(errs, os.Remove(filepath.Join(dir, name)))
			continue
		}
		if len(name) < len(prefix)+len(dbBackupTime) {
			continue
		}
		if _, err := time.Parse(dbBackupTime, backupTime(name)); err == nil {
			backups = append(backups, name)
		}
	}
	// Newest first
	slices.SortFunc(backups, func(a, b string) int { return strings.Compare(backupTime(b), backupTime(a)) })
	for _, name := range backups[min(len(backups), dbBackupsKept):] {
		errs = append(errs, os.Remove(filepath.Join(dir, name)))
	}
	return errors.Join(errs...)
}

// backupTime is the time at the end of a backup's name
func backupTime(name string) string {
	return name[len(name)-len(dbBackupTime):]
}
