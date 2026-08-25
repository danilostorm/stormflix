package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type browseDirectory struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type browseResponse struct {
	Root        string            `json:"root"`
	Current     string            `json:"current"`
	Parent      string            `json:"parent,omitempty"`
	Directories []browseDirectory `json:"directories"`
}

func (s *server) browseFilesystem(w http.ResponseWriter, r *http.Request) {
	root, err := filepath.Abs(s.config.MediaRoot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	root = filepath.Clean(root)

	requested := strings.TrimSpace(r.URL.Query().Get("path"))
	if requested == "" {
		requested = root
	}
	current, err := filepath.Abs(requested)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	current = filepath.Clean(current)

	if !pathWithinRoot(root, current) {
		writeError(w, http.StatusForbidden, errors.New("path is outside the configured media root"))
		return
	}

	info, err := os.Stat(current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, errors.New("path is not a directory"))
		return
	}

	entries, err := os.ReadDir(current)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	dirs := make([]browseDirectory, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dirs = append(dirs, browseDirectory{Name: name, Path: filepath.Join(current, name)})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	resp := browseResponse{Root: root, Current: current, Directories: dirs}
	if current != root {
		parent := filepath.Dir(current)
		if pathWithinRoot(root, parent) {
			resp.Parent = parent
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
