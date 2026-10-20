package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// secureWorkDir protects existing credential-bearing database copies too.
func secureWorkDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create private workdir: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("restrict workdir: %w", err)
	}
	for _, name := range []string{"wakeci.db", "wakeci.db.backup", ".compacted.wakeci.db"} {
		file := filepath.Join(dir, name)
		info, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat database copy: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("database copy %s is not a regular file", name)
		}
		if err := os.Chmod(file, dbFileMode); err != nil {
			return fmt.Errorf("restrict database copy: %w", err)
		}
	}
	return nil
}
