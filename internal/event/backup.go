package event

import (
	"fmt"
	"os"
	"path/filepath"
)

// Backup writes a consistent copy of the log to path, safely while writes continue.
//
// A plain file copy is not sufficient. WAL mode keeps recent commits in a sidecar
// (`.db-wal`) that has not yet been folded into the main file, so copying the main
// file alone silently loses everything since the last checkpoint — in practice most
// of a young database. Copying all three files without pausing writes can capture
// them at different instants and produce a torn backup.
//
// VACUUM INTO takes a read transaction and writes a single defragmented file that is
// internally consistent, without blocking writers. That is what makes "copy this one
// file" a true statement.
func (s *Store) Backup(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving backup path %s: %w", path, err)
	}
	// VACUUM INTO refuses to overwrite, which is the safer default: a backup that
	// silently replaces an earlier one is how people lose the good copy.
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("%s already exists; backups are never overwritten", abs)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking backup path %s: %w", abs, err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("creating backup directory: %w", err)
	}

	if _, err := s.db.Exec("VACUUM INTO ?", abs); err != nil {
		return fmt.Errorf("writing backup to %s: %w", abs, err)
	}
	return nil
}

// Checkpoint folds the write-ahead log into the main database file.
//
// Useful before archiving a stopped instance, so the sidecar files can be discarded.
func (s *Store) Checkpoint() error {
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpointing: %w", err)
	}
	return nil
}
