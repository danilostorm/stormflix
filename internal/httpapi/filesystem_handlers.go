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
	Roots       []browseDirectory `json:"roots,omitempty"`
}

func (s *server) browseFilesystem(w http.ResponseWriter, r *http.Request) {
	roots := normalizedFilesystemRoots(s.config.MediaRoot, s.config.ManagedMoviePaths, s.config.BrowseRoots)
	if len(roots) == 0 {
		writeError(w, http.StatusInternalServerError, errors.New("no media roots are configured"))
		return
	}

	requested := strings.TrimSpace(r.URL.Query().Get("path"))
	if requested == "" {
		requested = roots[0].Path
	}
	current, err := filepath.Abs(requested)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	current = filepath.Clean(current)

	allowedRoot, ok := filesystemRootForPath(roots, current)
	if !ok {
		writeError(w, http.StatusForbidden, errors.New("path is outside the configured media roots"))
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

	resp := browseResponse{Root: allowedRoot.Path, Current: current, Directories: dirs, Roots: roots}
	if current != allowedRoot.Path {
		parent := filepath.Dir(current)
		if pathWithinRoot(allowedRoot.Path, parent) {
			resp.Parent = parent
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func normalizedFilesystemRoots(mediaRoot string, groups ...[]string) []browseDirectory {
	capacity := 1
	for _, group := range groups {
		capacity += len(group)
	}
	candidates := make([]string, 0, capacity)
	candidates = append(candidates, mediaRoot)
	for _, group := range groups {
		candidates = append(candidates, group...)
	}

	seen := make(map[string]struct{}, len(candidates))
	roots := make([]browseDirectory, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if _, exists := seen[absolute]; exists {
			continue
		}
		seen[absolute] = struct{}{}

		name := filepath.Base(absolute)
		if name == "." || name == string(os.PathSeparator) || name == "" {
			name = absolute
		}
		roots = append(roots, browseDirectory{Name: name, Path: absolute})
	}
	return roots
}

func filesystemRootForPath(roots []browseDirectory, path string) (browseDirectory, bool) {
	var winner browseDirectory
	found := false
	for _, root := range roots {
		if !pathWithinRoot(root.Path, path) {
			continue
		}
		// Prefer the most specific root if roots are nested. This prevents the
		// Back button from escaping a dedicated authorized source into a broader
		// root that may also be visible to the server.
		if !found || len(root.Path) > len(winner.Path) {
			winner = root
			found = true
		}
	}
	return winner, found
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
