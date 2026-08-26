package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const maxAvatarUploadBytes int64 = 5 << 20

func (s *server) uploadOwnProfileAvatar(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.auth.Profile(r.Context(), u.ID, profileID); err != nil {
		writeError(w, http.StatusNotFound, errors.New("profile not found"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarUploadBytes+1<<20)
	if err := r.ParseMultipartForm(maxAvatarUploadBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("avatar upload is too large or invalid"))
		return
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("avatar file is required"))
		return
	}
	defer file.Close()

	limited := io.LimitReader(file, maxAvatarUploadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if int64(len(data)) > maxAvatarUploadBytes {
		writeError(w, http.StatusBadRequest, errors.New("avatar must be 5 MiB or smaller"))
		return
	}
	if len(data) < 12 {
		writeError(w, http.StatusBadRequest, errors.New("avatar image is invalid"))
		return
	}

	contentType := http.DetectContentType(data[:minInt(len(data), 512)])
	ext, ok := avatarExtension(contentType)
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("avatar must be JPEG, PNG, WebP or GIF"))
		return
	}
	_ = header

	key := filepath.ToSlash(filepath.Join("avatars", strconv.FormatInt(u.ID, 10), fmt.Sprintf("profile-%d%s", profileID, ext)))
	// Remove a previous avatar with another extension before writing the new one.
	_ = s.assets.RemoveTree(filepath.ToSlash(filepath.Join("avatars", strconv.FormatInt(u.ID, 10), fmt.Sprintf("profile-%d", profileID))))
	_, publicURL, err := s.assets.Put(key, strings.NewReader(string(data)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	profile, err := s.auth.SetProfileAvatarURL(r.Context(), u.ID, profileID, publicURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func avatarExtension(contentType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	default:
		return "", false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
