package database

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

// applyPendingRestore runs before the primary SQLite connection is opened. The
// Admin never overwrites a live database: it stages <db>.restore, then the next
// container restart validates that SQLite file and atomically swaps it in.
func applyPendingRestore(path string) error {
	restorePath := path + ".restore"
	if _, err := os.Stat(restorePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	check, err := sql.Open("sqlite", restorePath)
	if err != nil {
		return fmt.Errorf("open staged restore: %w", err)
	}
	var result string
	checkErr := check.QueryRow(`PRAGMA quick_check`).Scan(&result)
	_ = check.Close()
	if checkErr != nil || result != "ok" {
		if checkErr != nil {
			return fmt.Errorf("validate staged restore: %w", checkErr)
		}
		return fmt.Errorf("validate staged restore: %s", result)
	}

	if _, err := os.Stat(path); err == nil {
		safety := fmt.Sprintf("%s.pre-restore-%s", path, time.Now().UTC().Format("20060102-150405"))
		if err := os.Rename(path, safety); err != nil {
			return fmt.Errorf("preserve database before restore: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// WAL files belong to the previous main database and must never be replayed
	// against a restored VACUUM snapshot.
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	if err := os.Rename(restorePath, path); err != nil {
		return fmt.Errorf("activate staged restore: %w", err)
	}
	return nil
}
