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

	safety := ""
	movedWAL := false
	movedSHM := false
	if _, err := os.Stat(path); err == nil {
		safety = fmt.Sprintf("%s.pre-restore-%s", path, time.Now().UTC().Format("20060102-150405.000000000"))
		if err := os.Rename(path, safety); err != nil {
			return fmt.Errorf("preserve database before restore: %w", err)
		}
		// A clean shutdown usually checkpoints WAL, but preserving the sidecars
		// makes the safety copy complete even after a crash/restart edge case.
		if _, err := os.Stat(path + "-wal"); err == nil {
			if err := os.Rename(path+"-wal", safety+"-wal"); err != nil {
				_ = os.Rename(safety, path)
				return fmt.Errorf("preserve database WAL before restore: %w", err)
			}
			movedWAL = true
		} else if !os.IsNotExist(err) {
			_ = os.Rename(safety, path)
			return err
		}
		if _, err := os.Stat(path + "-shm"); err == nil {
			if err := os.Rename(path+"-shm", safety+"-shm"); err != nil {
				if movedWAL {
					_ = os.Rename(safety+"-wal", path+"-wal")
				}
				_ = os.Rename(safety, path)
				return fmt.Errorf("preserve database SHM before restore: %w", err)
			}
			movedSHM = true
		} else if !os.IsNotExist(err) {
			if movedWAL {
				_ = os.Rename(safety+"-wal", path+"-wal")
			}
			_ = os.Rename(safety, path)
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(restorePath, path); err != nil {
		// Never strand the installation without its previous database if the
		// final activation rename fails for any filesystem reason.
		if safety != "" {
			_ = os.Rename(safety, path)
			if movedWAL {
				_ = os.Rename(safety+"-wal", path+"-wal")
			}
			if movedSHM {
				_ = os.Rename(safety+"-shm", path+"-shm")
			}
		}
		return fmt.Errorf("activate staged restore: %w", err)
	}
	return nil
}
