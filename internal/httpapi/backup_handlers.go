package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *server) databasePath(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, path string
		if err := rows.Scan(&seq, &name, &path); err != nil {
			return "", err
		}
		if name == "main" && strings.TrimSpace(path) != "" {
			return filepath.Clean(path), nil
		}
	}
	return "", errors.New("SQLite database path not found")
}

func (s *server) createBackup(ctx context.Context, kind, note string) (map[string]any, error) {
	dbPath, err := s.databasePath(ctx)
	if err != nil {
		return nil, err
	}
	backupDir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return nil, err
	}
	name := "stormflix-" + time.Now().UTC().Format("20060102-150405.000000000") + ".db"
	path := filepath.Join(backupDir, name)
	quoted := strings.ReplaceAll(path, "'", "''")
	_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO system_backups(path,kind,size_bytes,status,note) VALUES(?,?,?,'ready',?)`, path, kind, info.Size(), note)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	id, _ := res.LastInsertId()
	if kind == "auto" {
		s.pruneAutomaticBackups(ctx, 10)
	}
	return map[string]any{"id": id, "name": name, "size_bytes": info.Size(), "kind": kind, "note": note}, nil
}

func (s *server) ensureAutomaticBackup(ctx context.Context, note string) (map[string]any, error) {
	var id int64
	var path string
	err := s.db.QueryRowContext(ctx, `SELECT id,path FROM system_backups WHERE kind='auto' AND status='ready' AND created_at>=datetime('now','-30 minutes') ORDER BY id DESC LIMIT 1`).Scan(&id, &path)
	if err == nil {
		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
			return map[string]any{"id": id, "path": path, "reused": true}, nil
		}
		// A stale database row must not satisfy the safety requirement.
		_, _ = s.db.ExecContext(ctx, `UPDATE system_backups SET status='missing' WHERE id=?`, id)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.createBackup(ctx, "auto", note)
}

func (s *server) pruneAutomaticBackups(ctx context.Context, keep int) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,path FROM system_backups WHERE kind='auto' AND status='ready' ORDER BY id DESC`)
	if err != nil {
		return
	}
	type backup struct {
		id   int64
		path string
	}
	items := []backup{}
	for rows.Next() {
		var item backup
		if rows.Scan(&item.id, &item.path) == nil {
			items = append(items, item)
		}
	}
	_ = rows.Close()
	for i := keep; i < len(items); i++ {
		_ = os.Remove(items[i].path)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM system_backups WHERE id=?`, items[i].id)
	}
}

func (s *server) listBackups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,path,kind,size_bytes,status,note,created_at FROM system_backups ORDER BY id DESC LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, size int64
		var path, kind, status, note, created string
		if err := rows.Scan(&id, &path, &kind, &size, &status, &note, &created); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, map[string]any{"id": id, "name": filepath.Base(path), "kind": kind, "size_bytes": size, "status": status, "note": note, "created_at": created})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) manualBackup(w http.ResponseWriter, r *http.Request) {
	backup, err := s.createBackup(r.Context(), "manual", "backup manual pelo Admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "system", "backup", "backup", "Backup manual criado", "", "", &uid)
	writeJSON(w, http.StatusCreated, backup)
}

func (s *server) scheduleBackupRestore(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var backupPath string
	if err := s.db.QueryRowContext(r.Context(), `SELECT path FROM system_backups WHERE id=? AND status='ready'`, id).Scan(&backupPath); err != nil {
		writeError(w, http.StatusNotFound, errors.New("backup not found"))
		return
	}
	input, err := os.Open(backupPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer input.Close()
	dbPath, err := s.databasePath(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	restorePath := dbPath + ".restore"
	output, err := os.OpenFile(restorePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(restorePath)
		writeError(w, http.StatusInternalServerError, errors.New("could not stage backup restore"))
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "system", "backup", "restore_scheduled", "Restauração de backup agendada para o próximo reinício", "", filepath.Base(backupPath), &uid)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "restart_required": true, "message": "backup preparado; reinicie o container para restaurar com segurança"})
}
