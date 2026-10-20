package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	// World-readable, as databases created before 0600 was enforced are
	db, err := bolt.Open(currentDBFile, 0644, nil)
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
	for _, name := range []string{"wakeci.db", "wakeci.db.backup"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if mode := info.Mode().Perm(); mode != dbFileMode {
			t.Errorf("%s mode = %o, want %o: it holds refresh tokens", name, mode, dbFileMode)
		}
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
