package httpapi

import (
	"errors"
	"net/http"
	"strings"
)

const hlsRoutePattern = "GET /api/v1/media/{id}/hls/{session}/{rest...}"

func (s *server) hlsDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimSpace(r.PathValue("rest"))
	switch {
	case rest == "index.m3u8":
		s.hlsPlaylist(w, r)
	case strings.HasPrefix(rest, "init/") && strings.HasSuffix(rest, ".mp4"):
		value := strings.TrimSuffix(strings.TrimPrefix(rest, "init/"), ".mp4")
		if value == "" || strings.Contains(value, "/") {
			writeError(w, http.StatusBadRequest, errors.New("invalid hls init path"))
			return
		}
		r.SetPathValue("batch", value)
		s.hlsInitSegment(w, r)
	case strings.HasPrefix(rest, "segment/") && strings.HasSuffix(rest, ".m4s"):
		value := strings.TrimSuffix(strings.TrimPrefix(rest, "segment/"), ".m4s")
		if value == "" || strings.Contains(value, "/") {
			writeError(w, http.StatusBadRequest, errors.New("invalid hls segment path"))
			return
		}
		r.SetPathValue("segment", value)
		s.hlsMediaSegment(w, r)
	default:
		writeError(w, http.StatusNotFound, errors.New("hls resource not found"))
	}
}
