package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/transcode"
	"github.com/danilostorm/stormflix/internal/webcompat"
)

func hlsPolicy(cfg config.Config) webcompat.HLSPolicy {
	policy := webcompat.DefaultHLSPolicy()
	policy.MaxBytes = cfg.HLSCacheMaxBytes
	policy.IdleTTL = cfg.HLSCacheIdleTTL
	policy.SegmentDuration = cfg.HLSSegmentDuration
	policy.BatchSegments = cfg.HLSBatchSegments
	policy.MinFreeBytes = cfg.CompatCacheMinFreeBytes
	policy.MinFreePercent = cfg.CompatCacheMinFreePercent
	return policy
}

func newHLSCache(cfg config.Config) (*webcompat.HLSManager, error) {
	return webcompat.NewHLSManager(filepath.Join(cfg.DataDir, "hls-cache"), hlsPolicy(cfg))
}

func (s *server) authorizeHLSMedia(w http.ResponseWriter, r *http.Request, id int64) (media.StreamItem, bool) {
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
		return media.StreamItem{}, false
	}
	item, err := s.media.GetStreamItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return media.StreamItem{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return media.StreamItem{}, false
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return media.StreamItem{}, false
	}
	return item, true
}

func (s *server) hlsPlaylist(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	u := currentUser(r)
	session := r.PathValue("session")
	var playlist string
	if transcode.IsSessionID(session) {
		manager, managerErr := transcode.ForDataDir(s.config.DataDir)
		if managerErr != nil {
			writeError(w, http.StatusInternalServerError, managerErr)
			return
		}
		playlist, err = manager.Playlist(u.ID, id, session)
		w.Header().Set("X-StormFlix-Playback", "video-transcode-hls")
	} else {
		playlist, err = s.hlsCache.Playlist(u.ID, id, session)
		w.Header().Set("X-StormFlix-Playback", "dynamic-hls")
	}
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write([]byte(playlist))
}

func (s *server) hlsInitSegment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	batch, err := strconv.Atoi(r.PathValue("batch"))
	if err != nil || batch < 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid hls batch"))
		return
	}
	u := currentUser(r)
	session := r.PathValue("session")
	var path string
	if transcode.IsSessionID(session) {
		manager, managerErr := transcode.ForDataDir(s.config.DataDir)
		if managerErr != nil {
			writeError(w, http.StatusInternalServerError, managerErr)
			return
		}
		path, err = manager.InitPath(r.Context(), u.ID, id, session, batch)
	} else {
		path, err = s.hlsCache.InitPath(r.Context(), u.ID, id, session, batch)
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	serveHLSFile(w, r, path, "video/mp4")
}

func (s *server) hlsMediaSegment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	segment, err := strconv.Atoi(r.PathValue("segment"))
	if err != nil || segment < 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid hls segment"))
		return
	}
	u := currentUser(r)
	session := r.PathValue("session")
	var path string
	if transcode.IsSessionID(session) {
		manager, managerErr := transcode.ForDataDir(s.config.DataDir)
		if managerErr != nil {
			writeError(w, http.StatusInternalServerError, managerErr)
			return
		}
		path, err = manager.SegmentPath(r.Context(), u.ID, id, session, segment)
	} else {
		path, err = s.hlsCache.SegmentPathBuffered(r.Context(), u.ID, id, session, segment)
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	serveHLSFile(w, r, path, "video/mp4")
}

func serveHLSFile(w http.ResponseWriter, r *http.Request, path, contentType string) {
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("hls fragment not found"))
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}
