package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) agentStatus(w http.ResponseWriter, r *http.Request) {
	mode := "mounted-storage"
	dataRoot, _ := filepath.Abs(s.config.DataDir)
	assetRoot, _ := filepath.Abs(s.assets.Root)
	if s.config.AssetPublicBaseURL != "" {
		mode = "external-cdn"
	} else if rel, err := filepath.Rel(dataRoot, assetRoot); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		mode = "local"
	}
	writeJSON(w, 200, map[string]any{
		"metadata":  s.metadata.Agents(),
		"subtitles": s.subtitles.Agents(),
		"assets": map[string]any{
			"mode":            mode,
			"directory":       s.assets.Root,
			"public_base_url": s.config.AssetPublicBaseURL,
			"note":            "AssetDir can be local or any mounted rclone backend such as Google Drive, FTP, S3 or WebDAV.",
		},
	})
}

func (s *server) metadataStatus(w http.ResponseWriter, r *http.Request) {
	_, _ = s.metadata.RecoverStaleJobs(30 * time.Minute)
	counts, err := s.metadata.StatusCounts(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, counts)
}

func (s *server) metadataJobs(w http.ResponseWriter, r *http.Request) {
	_, _ = s.metadata.RecoverStaleJobs(30 * time.Minute)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := s.metadata.Jobs(r.Context(), limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, jobs)
}

func (s *server) startMetadataJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := s.metadata.ValidateLibraryJob(r.Context(), id); err != nil {
		writeError(w, 409, err)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1" || strings.EqualFold(r.URL.Query().Get("refresh"), "true")
	job, err := s.metadata.StartLibraryJob(r.Context(), id, refresh)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	go func(jobID, libraryID int64) {
		s.metadata.WatchJob(jobID, 35*time.Minute)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		s.metadata.RetryLibraryErrorsWithMyAnimeList(ctx, libraryID)
	}(job.ID, id)
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "metadata", "Metadata scan started", &uid, job.Library)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *server) refreshMediaMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := s.metadata.RefreshMediaSmart(r.Context(), id); err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "metadata", "Media metadata refreshed", &uid, strconv.FormatInt(id, 10))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) mediaArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	items, err := s.metadata.Artwork(r.Context(), id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, items)
}

func (s *server) selectMediaArtwork(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	artworkID, err := parseID(r.PathValue("artwork_id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := s.metadata.SelectArtwork(r.Context(), mediaID, artworkID); err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) subtitleJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := s.subtitles.Jobs(r.Context(), limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, jobs)
}

func (s *server) startSubtitleJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var in struct {
		Language string `json:"language"`
	}
	if r.ContentLength > 0 {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		if err := dec.Decode(&in); err != nil {
			writeError(w, 400, errors.New("invalid JSON body"))
			return
		}
	}
	if strings.TrimSpace(in.Language) == "" {
		in.Language = strings.TrimSpace(strings.Split(s.config.SubtitleLanguages, ",")[0])
	}
	job, err := s.subtitles.StartLibraryJob(r.Context(), id, in.Language)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "subtitles", "Automatic subtitle job started", &uid, job.Library+" · "+job.Language)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *server) mediaSubtitles(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, 404, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, 500, err)
		return
	}
	u := currentUser(r)
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, 403, errors.New("library access denied"))
		return
	}
	items, err := s.subtitles.ListForMedia(r.Context(), id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, items)
}
