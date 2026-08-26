package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxAssetBytes int64 = 25 << 20

type Store struct {
	mu            sync.RWMutex
	Root          string
	PublicBaseURL string
	client        *http.Client
}

func New(root, publicBaseURL string) (*Store, error) {
	s := &Store{client: &http.Client{Timeout: 30 * time.Second}}
	if err := s.Configure(root, publicBaseURL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Configure(root, publicBaseURL string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("asset directory is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("create asset directory: %w", err)
	}
	s.mu.Lock()
	s.Root = abs
	s.PublicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	s.mu.Unlock()
	return nil
}

func (s *Store) Snapshot() (root, publicBaseURL string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Root, s.PublicBaseURL
}

func (s *Store) PutURL(ctx context.Context, key, sourceURL string) (assetPath, publicURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "StormFlix/0.4")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("download artwork: HTTP %d", resp.StatusCode)
	}
	if ext := filepath.Ext(key); ext == "" {
		key += extensionFromURL(sourceURL, resp.Header.Get("Content-Type"))
	}
	return s.Put(key, resp.Body)
}

func (s *Store) Put(key string, r io.Reader) (assetPath, publicURL string, err error) {
	root, _ := s.Snapshot()
	key = filepath.ToSlash(strings.TrimSpace(key))
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "../") || key == ".." {
		return "", "", errors.New("invalid asset key")
	}
	dest := filepath.Join(root, filepath.FromSlash(key))
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", errors.New("asset path escapes root")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", "", err
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", "", err
	}
	_, copyErr := io.Copy(f, io.LimitReader(r, maxAssetBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", "", closeErr
	}
	info, err := os.Stat(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	if info.Size() > maxAssetBytes {
		_ = os.Remove(tmp)
		return "", "", errors.New("asset exceeds 25 MiB limit")
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	return key, s.URL(key), nil
}

// RemoveTree removes one asset subtree without allowing callers to escape the
// configured asset root. It is used when metadata is refreshed so superseded
// posters, backdrops and logos do not accumulate on disk.
func (s *Store) RemoveTree(key string) error {
	root, _ := s.Snapshot()
	key = filepath.ToSlash(strings.TrimSpace(key))
	key = strings.TrimPrefix(key, "/")
	if key == "" || key == "." || strings.Contains(key, "../") || key == ".." {
		return errors.New("invalid asset key")
	}
	dest := filepath.Join(root, filepath.FromSlash(key))
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("asset path escapes root")
	}
	return os.RemoveAll(dest)
}

func (s *Store) URL(key string) string {
	_, base := s.Snapshot()
	key = strings.TrimPrefix(filepath.ToSlash(key), "/")
	if base != "" {
		return base + "/" + key
	}
	return "/assets/" + key
}

func (s *Store) Resolve(key string) (string, error) {
	root, _ := s.Snapshot()
	key = filepath.ToSlash(strings.TrimSpace(key))
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "../") || key == ".." {
		return "", errors.New("invalid asset key")
	}
	path := filepath.Join(root, filepath.FromSlash(key))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("asset path escapes root")
	}
	return path, nil
}

func extensionFromURL(rawURL, contentType string) string {
	if u, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(filepath.Ext(u.Path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".svg" {
			return ext
		}
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".jpg"
	}
}
