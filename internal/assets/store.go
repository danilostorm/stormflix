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
	"time"
)

const maxAssetBytes int64 = 25 << 20

type Store struct {
	Root          string
	PublicBaseURL string
	client        *http.Client
}

func New(root, publicBaseURL string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("asset directory is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create asset directory: %w", err)
	}
	return &Store{
		Root:          abs,
		PublicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		client:        &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (s *Store) PutURL(ctx context.Context, key, sourceURL string) (assetPath, publicURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "StormFlix/0.3")
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
	key = filepath.ToSlash(strings.TrimSpace(key))
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "../") || key == ".." {
		return "", "", errors.New("invalid asset key")
	}
	dest := filepath.Join(s.Root, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.Root, dest)
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

func (s *Store) URL(key string) string {
	key = strings.TrimPrefix(filepath.ToSlash(key), "/")
	if s.PublicBaseURL != "" {
		return s.PublicBaseURL + "/" + key
	}
	return "/assets/" + key
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
