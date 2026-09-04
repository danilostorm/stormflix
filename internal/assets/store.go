package assets

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/transcode"
)

const maxAssetBytes int64 = 25 << 20

type Store struct {
	mu            sync.RWMutex
	Root          string
	PublicBaseURL string
	client        *http.Client
}

// Variant returns a cached, source-versioned responsive image. Generation is
// globally bounded so opening Home cannot create an FFmpeg process storm.
func (s *Store) Variant(ctx context.Context, key string, width int, accept string) (string, string, bool) {
	width = responsiveWidth(width)
	format := responsiveFormat(accept)
	if width == 0 || format == "" {
		return "", "", false
	}
	source, err := s.Resolve(key)
	if err != nil {
		return "", "", false
	}
	info, err := os.Stat(source)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	ext := strings.ToLower(filepath.Ext(source))
	if ext == ".svg" || ext == ".gif" {
		return "", "", false
	}
	root, _ := s.Snapshot()
	fingerprint := fmt.Sprintf("%s|%d|%d|%d|%s", filepath.ToSlash(key), info.Size(), info.ModTime().UnixNano(), width, format)
	name := fmt.Sprintf("%x.%s", sha256.Sum256([]byte(fingerprint)), format)
	target := filepath.Join(root, "_variants", name)
	if _, err := os.Stat(target); err == nil {
		return target, "image/" + format, true
	}
	release, _, err := transcode.AcquireProcess(ctx, false)
	if err != nil {
		return "", "", false
	}
	defer release()
	if _, err := os.Stat(target); err == nil {
		return target, "image/" + format, true
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", "", false
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", "", false
	}
	temporary := target + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 10) + "." + format
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-i", source, "-vf", fmt.Sprintf("scale='min(%d,iw)':-2", width), "-frames:v", "1"}
	if format == "avif" {
		args = append(args, "-c:v", "libaom-av1", "-still-picture", "1", "-crf", "34", "-cpu-used", "6")
	} else {
		args = append(args, "-c:v", "libwebp", "-quality", "78")
	}
	args = append(args, "-y", temporary)
	if _, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		_ = os.Remove(temporary)
		return "", "", false
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return "", "", false
	}
	return target, "image/" + format, true
}

func responsiveWidth(value int) int {
	for _, width := range []int{240, 360, 500, 780, 1280} {
		if value <= width {
			if value > 0 {
				return width
			}
			return 0
		}
	}
	return 1280
}

func responsiveFormat(accept string) string {
	accept = strings.ToLower(accept)
	if strings.Contains(accept, "image/avif") {
		return "avif"
	}
	if strings.Contains(accept, "image/webp") {
		return "webp"
	}
	return ""
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
