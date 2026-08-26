package httpapi

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/auth"
)

func (s *server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.admin.Dashboard(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	libs, _ := s.libraries.ManagedList(r.Context())
	for _, v := range libs {
		if !v.Online {
			d.OfflineLibraries++
		}
	}
	writeJSON(w, 200, d)
}
func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	v, err := s.auth.ListUsers(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if v == nil {
		v = []auth.User{}
	}
	writeJSON(w, 200, v)
}
func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string  `json:"username"`
		DisplayName string  `json:"display_name"`
		Password    string  `json:"password"`
		Role        string  `json:"role"`
		Active      bool    `json:"active"`
		LibraryIDs  []int64 `json:"library_ids"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if len(in.LibraryIDs) == 0 {
		in.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
	}
	u, err := s.auth.CreateUser(r.Context(), in.Username, in.DisplayName, in.Password, in.Role, in.Active, in.LibraryIDs)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "users", "User created", &uid, u.Username)
	writeJSON(w, 201, u)
}
func (s *server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var in struct {
		DisplayName string  `json:"display_name"`
		Password    string  `json:"password"`
		Role        string  `json:"role"`
		Active      bool    `json:"active"`
		LibraryIDs  []int64 `json:"library_ids"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if id == currentUser(r).ID && !in.Active {
		writeError(w, 400, errors.New("cannot disable your own account"))
		return
	}
	if len(in.LibraryIDs) == 0 {
		in.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
	}
	u, err := s.auth.UpdateUser(r.Context(), id, in.DisplayName, in.Password, in.Role, in.Active, in.LibraryIDs)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "users", "User updated", &uid, u.Username)
	writeJSON(w, 200, u)
}
func (s *server) allEnabledLibraryIDs(ctx context.Context) []int64 {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM libraries WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return []int64{}
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}
func (s *server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if id == currentUser(r).ID {
		writeError(w, 400, errors.New("cannot delete your own account"))
		return
	}
	if err := s.auth.DeleteUser(r.Context(), id); err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "users", "User deleted", &uid, strconv.FormatInt(id, 10))
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	v, err := s.auth.ListSessions(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if v == nil {
		v = []auth.Session{}
	}
	writeJSON(w, 200, v)
}
func (s *server) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := s.auth.RevokeSession(r.Context(), id); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *server) logs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, err := s.admin.Logs(r.Context(), limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if v == nil {
		v = []admin.LogEntry{}
	}
	writeJSON(w, 200, v)
}
func (s *server) storage(w http.ResponseWriter, r *http.Request) {
	v, err := s.libraries.ManagedList(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if v == nil {
		writeJSON(w, 200, []struct{}{})
		return
	}
	writeJSON(w, 200, v)
}
func (s *server) playbacks(w http.ResponseWriter, r *http.Request) {
	v, err := s.admin.Playbacks(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if v == nil {
		v = []admin.Playback{}
	}
	writeJSON(w, 200, v)
}
func (s *server) serverInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	writeJSON(w, 200, map[string]any{"name": "StormFlix", "version": version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "memory_alloc_bytes": m.Alloc, "memory_sys_bytes": m.Sys, "uptime_seconds": int64(time.Since(s.startedAt).Seconds()), "database": s.config.DatabasePath(), "direct_play_only": true, "transcoding_enabled": false})
}
