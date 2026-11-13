package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestByteToInt(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		expected  int
		expectErr bool
	}{
		{name: "Zero", input: []byte("0"), expected: 0},
		{name: "Positive", input: []byte("42"), expected: 42},
		{name: "NotANumber", input: []byte("abc"), expectErr: true},
		{name: "Empty", input: []byte(""), expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ByteToInt(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestIntToByte(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected string
	}{
		{name: "Zero", input: 0, expected: "0"},
		{name: "Positive", input: 42, expected: "42"},
		{name: "Negative", input: -7, expected: "-7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(IntToByte(tt.input))
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestItob(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected []byte
	}{
		{name: "Zero", input: 0, expected: []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{name: "One", input: 1, expected: []byte{0, 0, 0, 0, 0, 0, 0, 1}},
		{name: "256", input: 256, expected: []byte{0, 0, 0, 0, 0, 0, 1, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Itob(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestItobOrdering(t *testing.T) {
	// Itob must produce a byte-sorted representation, since it is used as a
	// bbolt key so that iteration order matches numeric order.
	small := Itob(1)
	large := Itob(2)
	if bytes.Compare(small, large) >= 0 {
		t.Errorf("expected Itob(1) to sort before Itob(2), got %v >= %v", small, large)
	}
}

func TestCompactDBClosesCurrentDatabaseOnEarlyError(t *testing.T) {
	dir := t.TempDir()
	currentDBFile := filepath.Join(dir, "wakeci.db")
	db, err := bolt.Open(currentDBFile, 0600, nil)
	if err != nil {
		t.Fatalf("create current database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close current database: %v", err)
	}

	blockedCompactedPath := filepath.Join(dir, ".compacted.wakeci.db")
	if err := os.Mkdir(blockedCompactedPath, 0700); err != nil {
		t.Fatalf("create blocking directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedCompactedPath, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatalf("make blocking directory non-empty: %v", err)
	}

	oldConfig := Config
	Config = &WakeConfig{WorkDir: dir + string(os.PathSeparator)}
	t.Cleanup(func() { Config = oldConfig })

	if err := CompactDB(); err == nil {
		t.Fatal("CompactDB() error = nil, want stale compacted path error")
	}

	reopened, err := bolt.Open(currentDBFile, 0600, &bolt.Options{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("current database remained locked after CompactDB error: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened current database: %v", err)
	}
}

func TestCompactDBReplacesDatabaseAndClosesFiles(t *testing.T) {
	dir := t.TempDir()
	currentDBFile := filepath.Join(dir, "wakeci.db")
	db, err := bolt.Open(currentDBFile, 0600, nil)
	if err != nil {
		t.Fatalf("create current database: %v", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		bucket, createErr := tx.CreateBucket([]byte("data"))
		if createErr != nil {
			return createErr
		}
		return bucket.Put([]byte("key"), []byte("value"))
	}); err != nil {
		t.Fatalf("populate current database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close current database: %v", err)
	}

	oldConfig := Config
	Config = &WakeConfig{WorkDir: dir + string(os.PathSeparator)}
	t.Cleanup(func() { Config = oldConfig })

	if err := CompactDB(); err != nil {
		t.Fatalf("CompactDB: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wakeci.db.backup")); err != nil {
		t.Fatalf("stat database backup: %v", err)
	}

	compacted, err := bolt.Open(currentDBFile, 0600, &bolt.Options{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("open compacted database: %v", err)
	}
	if err := compacted.View(func(tx *bolt.Tx) error {
		if got := tx.Bucket([]byte("data")).Get([]byte("key")); !bytes.Equal(got, []byte("value")) {
			t.Errorf("compacted value = %q, want %q", got, "value")
		}
		return nil
	}); err != nil {
		t.Fatalf("read compacted database: %v", err)
	}
	if err := compacted.Close(); err != nil {
		t.Fatalf("close compacted database: %v", err)
	}
}

// openBackupTestDB opens a database in a new directory, made by an older
// wakeci with a build in its history if old is true
func openBackupTestDB(t *testing.T, old bool) *bolt.DB {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "wakeci.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if old {
		if err := db.Update(func(tx *bolt.Tx) error {
			if err := bootstrapDB(tx); err != nil {
				return err
			}
			return tx.Bucket(HistoryBucket).Put(Itob(1), []byte(`{"id":1}`))
		}); err != nil {
			t.Fatalf("fill db: %v", err)
		}
	}
	return db
}

func storedDBVersion(t *testing.T, db *bolt.DB) string {
	t.Helper()
	var version string
	if err := db.View(func(tx *bolt.Tx) error {
		version = string(tx.Bucket(GlobalBucket).Get(dbVersionKey))
		return nil
	}); err != nil {
		t.Fatalf("read version: %v", err)
	}
	return version
}

// backupNames lists the files next to the db but the db itself
func backupNames(t *testing.T, db *bolt.DB) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(db.Path()))
	if err != nil {
		t.Fatalf("list dir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != "wakeci.db" {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestBackupForNewVersionCopiesTheDBTheOldVersionLeft(t *testing.T) {
	db := openBackupTestDB(t, true)
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GlobalBucket).Put(dbVersionKey, []byte("1.0.0"))
	}); err != nil {
		t.Fatalf("set version: %v", err)
	}
	now := time.Date(2026, 11, 13, 13, 4, 5, 0, time.FixedZone("CET", 3600))

	path, err := backupForNewVersion(db, "1.1.0", now)
	if err != nil {
		t.Fatalf("backupForNewVersion: %v", err)
	}
	if want := db.Path() + ".before-1.1.0-20261113T120405Z"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("backup stat = %v, %v; want mode 0600", info, err)
	}
	backup, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = backup.Close() }()
	if err := backup.View(func(tx *bolt.Tx) error {
		if got := tx.Bucket(HistoryBucket).Get(Itob(1)); string(got) != `{"id":1}` {
			t.Errorf("backup history = %q, want the build", got)
		}
		if got := tx.Bucket(GlobalBucket).Get(dbVersionKey); string(got) != "1.0.0" {
			t.Errorf("backup version = %q, want 1.0.0, as the old version left it", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if got := storedDBVersion(t, db); got != "1.1.0" {
		t.Errorf("version = %q, want 1.1.0", got)
	}

	// The same version again copies nothing
	again, err := backupForNewVersion(db, "1.1.0", now.Add(time.Hour))
	if err != nil || again != "" {
		t.Errorf("second start = %q, %v; want no backup", again, err)
	}
	if names := backupNames(t, db); len(names) != 1 {
		t.Errorf("files = %v, want only the first backup", names)
	}
}

func TestBackupForNewVersionCopiesADBOfAVersionBeforeIt(t *testing.T) {
	// Versions before this one record none
	db := openBackupTestDB(t, true)
	path, err := backupForNewVersion(db, "2.68.0", time.Now())
	if err != nil || path == "" {
		t.Fatalf("backupForNewVersion = %q, %v; want a backup", path, err)
	}
	if got := storedDBVersion(t, db); got != "2.68.0" {
		t.Errorf("version = %q, want 2.68.0", got)
	}
}

func TestBackupForNewVersionSkipsANewDB(t *testing.T) {
	db := openBackupTestDB(t, false)
	path, err := backupForNewVersion(db, "2.68.0", time.Now())
	if err != nil || path != "" {
		t.Fatalf("backupForNewVersion = %q, %v; want no backup", path, err)
	}
	if names := backupNames(t, db); len(names) != 0 {
		t.Errorf("files = %v, want none", names)
	}
	if got := storedDBVersion(t, db); got != "2.68.0" {
		t.Errorf("version = %q, want 2.68.0", got)
	}
}

func TestBackupForNewVersionKeepsTheNewest(t *testing.T) {
	db := openBackupTestDB(t, true)
	dir := filepath.Dir(db.Path())
	for _, name := range []string{"wakeci.db.before-0.9-20200101T000000Z.tmp", "wakeci.db.backup", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Times sort the backups, not versions: 10 is newer than 9
	for i, version := range []string{"7", "8", "9", "10", "dev/x"} {
		if _, err := backupForNewVersion(db, version, start.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("backupForNewVersion(%s): %v", version, err)
		}
	}
	want := []string{
		"notes.txt",
		"wakeci.db.backup",
		"wakeci.db.before-10-20260101T030000Z",
		"wakeci.db.before-9-20260101T020000Z",
		"wakeci.db.before-dev_x-20260101T040000Z",
	}
	if got := backupNames(t, db); !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestBackupForNewVersionRecordsTheVersionWhenTheCopyFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	db := openBackupTestDB(t, true)
	dir := filepath.Dir(db.Path())
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	path, err := backupForNewVersion(db, "2.68.0", time.Now())
	if err == nil || path != "" {
		t.Fatalf("backupForNewVersion = %q, %v; want an error and no backup", path, err)
	}
	// So the next start doesn't save the db this version changed as its backup
	if got := storedDBVersion(t, db); got != "2.68.0" {
		t.Errorf("version = %q, want 2.68.0", got)
	}
}
