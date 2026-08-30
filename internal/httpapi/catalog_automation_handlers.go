package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type catalogHealthItem struct {
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	Library        string   `json:"library"`
	Available      bool     `json:"available"`
	MetadataStatus string   `json:"metadata_status"`
	Genres         []string `json:"genres"`
	PosterURL      string   `json:"poster_url"`
	TMDBID         int64    `json:"tmdb_id"`
	Year           int      `json:"year"`
	MediaType      string   `json:"media_type"`
}

type duplicateGroup struct {
	Key       string              `json:"key"`
	Title     string              `json:"title"`
	Year      int                 `json:"year"`
	MediaType string              `json:"media_type"`
	TMDBID    int64               `json:"tmdb_id"`
	Copies    []catalogHealthItem `json:"copies"`
}

func (s *server) allCatalogHealthItems(ctx context.Context) ([]catalogHealthItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.title,l.name,m.available,COALESCE(mm.status,'pending'),COALESCE(mm.genres_json,'[]'),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE(mm.tmdb_id,0),COALESCE(mm.year,0),COALESCE(mm.media_type,'')
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE l.kind<>'music' ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalogHealthItem{}
	for rows.Next() {
		var item catalogHealthItem
		var genres string
		if err := rows.Scan(&item.ID, &item.Title, &item.Library, &item.Available, &item.MetadataStatus, &genres, &item.PosterURL, &item.TMDBID, &item.Year, &item.MediaType); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *server) catalogHealth(w http.ResponseWriter, r *http.Request) {
	items, err := s.allCatalogHealthItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	metrics := map[string]int{"total": 0, "sem_metadados": 0, "sem_capa": 0, "sem_genero": 0, "outros": 0, "indisponiveis": 0, "duplicados": 0, "tecnico_pendente": 0}
	for _, item := range items {
		if item.Available {
			metrics["total"]++
		}
		if item.MetadataStatus == "" || item.MetadataStatus == "pending" || item.MetadataStatus == "error" {
			metrics["sem_metadados"]++
		}
		if item.Available && strings.TrimSpace(item.PosterURL) == "" {
			metrics["sem_capa"]++
		}
		if item.Available && len(item.Genres) == 0 {
			metrics["sem_genero"]++
		}
		if item.Available && catalogItemFallsInOutros(item.Genres) {
			metrics["outros"]++
		}
		if !item.Available {
			metrics["indisponiveis"]++
		}
	}
	groups := duplicateCatalogGroups(items)
	metrics["duplicados"] = len(groups)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_technical mt ON mt.media_id=m.id WHERE m.available=1 AND l.kind<>'music' AND (mt.media_id IS NULL OR mt.status<>'ok' OR mt.source_modified_unix<>m.modified_unix)`).Scan(&metrics["tecnico_pendente"])
	writeJSON(w, http.StatusOK, metrics)
}

func catalogItemFallsInOutros(genres []string) bool {
	if len(genres) == 0 {
		return true
	}
	known := map[string]bool{
		"action":true,"acao":true,"adventure":true,"aventura":true,"animation":true,"animacao":true,"comedy":true,"comedia":true,"crime":true,"documentary":true,"documentario":true,"documentarios":true,
		"drama":true,"family":true,"familia":true,"fantasy":true,"fantasia":true,"history":true,"historia":true,"horror":true,"terror":true,"music":true,"musica":true,"mystery":true,"misterio":true,
		"romance":true,"science-fiction":true,"sci-fi":true,"ficcao-cientifica":true,"thriller":true,"suspense":true,"war":true,"guerra":true,"western":true,"faroeste":true,"kids":true,"infantil":true,
		"reality":true,"reality-show":true,"soap":true,"novela":true,"novelas":true,"news":true,"noticias":true,"sports":true,"esportes":true,"supernatural":true,"sobrenatural":true,
	}
	for _, genre := range genres {
		if known[categoryRuleKey(genre)] {
			return false
		}
	}
	return true
}

func (s *server) catalogHealthItems(w http.ResponseWriter, r *http.Request) {
	issue := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("issue")))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.allCatalogHealthItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	filtered := []catalogHealthItem{}
	for _, item := range items {
		match := false
		switch issue {
		case "sem_metadados":
			match = item.MetadataStatus == "" || item.MetadataStatus == "pending" || item.MetadataStatus == "error"
		case "sem_capa":
			match = item.Available && item.PosterURL == ""
		case "sem_genero":
			match = item.Available && len(item.Genres) == 0
		case "outros":
			match = item.Available && catalogItemFallsInOutros(item.Genres)
		case "indisponiveis":
			match = !item.Available
		default:
			match = item.Available
		}
		if match {
			filtered = append(filtered, item)
		}
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "items": filtered[offset:end]})
}

func duplicateCatalogGroups(items []catalogHealthItem) []duplicateGroup {
	groups := map[string][]catalogHealthItem{}
	for _, item := range items {
		if !item.Available {
			continue
		}
		key := ""
		if item.TMDBID > 0 {
			key = fmt.Sprintf("tmdb:%d:%s", item.TMDBID, strings.ToLower(item.MediaType))
		} else {
			key = fmt.Sprintf("title:%s:%d:%s", categoryRuleKey(item.Title), item.Year, strings.ToLower(item.MediaType))
		}
		groups[key] = append(groups[key], item)
	}
	out := []duplicateGroup{}
	for key, copies := range groups {
		if len(copies) < 2 {
			continue
		}
		first := copies[0]
		out = append(out, duplicateGroup{Key: key, Title: first.Title, Year: first.Year, MediaType: first.MediaType, TMDBID: first.TMDBID, Copies: copies})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Copies) == len(out[j].Copies) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return len(out[i].Copies) > len(out[j].Copies)
	})
	return out
}

func (s *server) catalogDuplicates(w http.ResponseWriter, r *http.Request) {
	items, err := s.allCatalogHealthItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	groups := duplicateCatalogGroups(items)
	if len(groups) > 100 {
		groups = groups[:100]
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *server) previewLibraryScan(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	preview, err := s.libraries.PreviewMulti(ctx, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *server) scanLibraryWithBackup(w http.ResponseWriter, r *http.Request) {
	_, _ = s.ensureAutomaticBackup(r.Context(), "antes do scan da biblioteca")
	s.scanLibrary(w, r)
}

func (s *server) scanAllLibrariesWithBackup(w http.ResponseWriter, r *http.Request) {
	_, _ = s.ensureAutomaticBackup(r.Context(), "antes do scan de todas as bibliotecas")
	s.scanAllLibraries(w, r)
}

func (s *server) updateLibraryWithBackup(w http.ResponseWriter, r *http.Request) {
	_, _ = s.ensureAutomaticBackup(r.Context(), "antes de alterar biblioteca/caminho")
	s.updateLibrary(w, r)
}

func (s *server) organizeRecommendedCategoriesWithBackup(w http.ResponseWriter, r *http.Request) {
	_, _ = s.ensureAutomaticBackup(r.Context(), "antes de reorganizar os menus da Home")
	s.organizeRecommendedCategories(w, r)
}

func (s *server) recordCatalogChange(ctx context.Context, entityType, entityID, action, summary, beforeJSON, afterJSON string, userID *int64) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO catalog_changes(entity_type,entity_id,action,summary,before_json,after_json,user_id) VALUES(?,?,?,?,?,?,?)`, entityType, entityID, action, summary, beforeJSON, afterJSON, userID)
}

func idString(id int64) string { return strconv.FormatInt(id, 10) }

func (s *server) catalogHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id,c.entity_type,c.entity_id,c.action,c.summary,c.before_json,c.after_json,COALESCE(u.display_name,''),c.created_at FROM catalog_changes c LEFT JOIN users u ON u.id=c.user_id ORDER BY c.id DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var entityType, entityID, action, summary, beforeJSON, afterJSON, user, created string
		if rows.Scan(&id, &entityType, &entityID, &action, &summary, &beforeJSON, &afterJSON, &user, &created) == nil {
			out = append(out, map[string]any{"id": id, "entity_type": entityType, "entity_id": entityID, "action": action, "summary": summary, "before": beforeJSON, "after": afterJSON, "user": user, "created_at": created})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

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
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
		return nil, err
	}
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
		return map[string]any{"id": id, "path": path, "reused": true}, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.createBackup(ctx, "auto", note)
}

func (s *server) pruneAutomaticBackups(ctx context.Context, keep int) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,path FROM system_backups WHERE kind='auto' AND status='ready' ORDER BY id DESC`)
	if err != nil {
		return
	}
	type backup struct{ id int64; path string }
	items := []backup{}
	for rows.Next() {
		var b backup
		if rows.Scan(&b.id, &b.path) == nil {
			items = append(items, b)
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
		if rows.Scan(&id, &path, &kind, &size, &status, &note, &created) == nil {
			out = append(out, map[string]any{"id": id, "name": filepath.Base(path), "kind": kind, "size_bytes": size, "status": status, "note": note, "created_at": created})
		}
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
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(restorePath)
		writeError(w, http.StatusInternalServerError, errors.New("could not stage backup restore"))
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "system", "backup", "restore_scheduled", "Restauração de backup agendada para o próximo reinício", "", filepath.Base(backupPath), &uid)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "restart_required": true, "message": "backup preparado; reinicie o container para restaurar com segurança"})
}

type profileHomeMenuEntry struct {
	CategoryID int64 `json:"category_id"`
	Visible    bool  `json:"visible"`
	SortOrder  int   `json:"sort_order"`
}

func (s *server) selectedProfileHomeMenus(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if profileID <= 0 {
		writeJSON(w, http.StatusOK, []profileHomeMenuEntry{})
		return
	}
	writeJSON(w, http.StatusOK, s.profileHomeEntries(r.Context(), profileID))
}

func (s *server) profileHomeEntries(ctx context.Context, profileID int64) []profileHomeMenuEntry {
	rows, err := s.db.QueryContext(ctx, `SELECT category_id,visible,sort_order FROM profile_home_menus WHERE profile_id=? ORDER BY sort_order,category_id`, profileID)
	if err != nil {
		return []profileHomeMenuEntry{}
	}
	defer rows.Close()
	out := []profileHomeMenuEntry{}
	for rows.Next() {
		var v profileHomeMenuEntry
		if rows.Scan(&v.CategoryID, &v.Visible, &v.SortOrder) == nil {
			out = append(out, v)
		}
	}
	return out
}

func (s *server) adminProfileHomeOverview(w http.ResponseWriter, r *http.Request) {
	profileRows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.name,p.is_kids,u.display_name FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.active=1 ORDER BY u.display_name,p.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	profiles := []map[string]any{}
	for profileRows.Next() {
		var id int64
		var name, user string
		var kids bool
		if profileRows.Scan(&id, &name, &kids, &user) == nil {
			profiles = append(profiles, map[string]any{"id": id, "name": name, "user": user, "is_kids": kids, "menus": s.profileHomeEntries(r.Context(), id)})
		}
	}
	_ = profileRows.Close()
	categories, _ := s.categories(r.Context(), true)
	roots := []libraryCategory{}
	for _, c := range categories {
		if c.ParentID == nil {
			roots = append(roots, c)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles, "menus": roots})
}

func (s *server) updateProfileHomeMenus(w http.ResponseWriter, r *http.Request) {
	profileID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in struct{ Menus []profileHomeMenuEntry `json:"menus"` }
	if decodeJSON(w, r, &in) != nil {
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM profiles WHERE id=?`, profileID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, errors.New("profile not found"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM profile_home_menus WHERE profile_id=?`, profileID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	seen := map[int64]bool{}
	for i, menu := range in.Menus {
		if menu.CategoryID <= 0 || seen[menu.CategoryID] {
			continue
		}
		seen[menu.CategoryID] = true
		order := menu.SortOrder
		if order <= 0 {
			order = (i + 1) * 10
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO profile_home_menus(profile_id,category_id,visible,sort_order) SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM library_categories WHERE id=? AND parent_id IS NULL)`, profileID, menu.CategoryID, menu.Visible, order, menu.CategoryID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "profile", idString(profileID), "home", "Home personalizada do perfil atualizada", "", "", &uid)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
