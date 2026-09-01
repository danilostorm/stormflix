package games

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type coreBundleFiles struct {
	JS   []byte
	WASM []byte
}

func coreBundleURL(core string) string {
	return fmt.Sprintf("https://cdn.jsdelivr.net/gh/arianrhodsandlot/retroarch-emscripten-build@%s/retroarch/%s_libretro.zip", RetroArchBuild, core)
}

func extractCoreBundle(payload []byte) (coreBundleFiles, error) {
	if len(payload) == 0 || int64(len(payload)) > maxRuntimeBytes {
		return coreBundleFiles{}, errors.New("pinned core bundle size is outside the safety limit")
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return coreBundleFiles{}, fmt.Errorf("open pinned core bundle: %w", err)
	}
	var out coreBundleFiles
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(filepath.Base(entry.Name))
		if !strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".wasm") {
			continue
		}
		if entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > uint64(maxRuntimeBytes) {
			return coreBundleFiles{}, errors.New("pinned core bundle contains an invalid runtime asset")
		}
		handle, err := entry.Open()
		if err != nil {
			return coreBundleFiles{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(handle, maxRuntimeBytes+1))
		closeErr := handle.Close()
		if readErr != nil {
			return coreBundleFiles{}, readErr
		}
		if closeErr != nil {
			return coreBundleFiles{}, closeErr
		}
		if int64(len(body)) > maxRuntimeBytes || uint64(len(body)) != entry.UncompressedSize64 {
			return coreBundleFiles{}, errors.New("pinned core runtime asset exceeded the safety limit")
		}
		if strings.HasSuffix(name, ".wasm") {
			out.WASM = body
		} else if strings.HasSuffix(name, ".js") {
			out.JS = body
		}
	}
	if len(out.JS) == 0 || len(out.WASM) == 0 {
		return coreBundleFiles{}, errors.New("pinned core bundle does not contain both JS and WASM runtime files")
	}
	return out, nil
}

func writeRuntimeCache(path string, payload []byte) error {
	if len(payload) == 0 || int64(len(payload)) > maxRuntimeBytes {
		return errors.New("runtime cache payload is outside the safety limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stormflix-runtime-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *Service) runtimeCoreAsset(ctx context.Context, asset string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(asset))
	core := strings.TrimSuffix(asset, filepath.Ext(asset))
	if !allowedRuntimeCores[core] || (ext != ".js" && ext != ".wasm") {
		return "", "", errors.New("unsupported game runtime asset")
	}
	contentType := "text/javascript; charset=utf-8"
	if ext == ".wasm" {
		contentType = "application/wasm"
	}
	dataDir, err := s.dataDir(ctx)
	if err != nil {
		return "", "", err
	}
	cacheDir := filepath.Join(dataDir, "game-runtime", "retroarch-"+RetroArchBuild)
	jsPath := filepath.Join(cacheDir, core+"_libretro.js")
	wasmPath := filepath.Join(cacheDir, core+"_libretro.wasm")
	destination := jsPath
	if ext == ".wasm" {
		destination = wasmPath
	}

	gameRuntimeMu.Lock()
	defer gameRuntimeMu.Unlock()
	if info, statErr := os.Stat(destination); statErr == nil && !info.IsDir() && info.Size() > 0 {
		return destination, contentType, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, coreBundleURL(core), nil)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("User-Agent", "StormFlix-Games/1.0")
	response, err := (&http.Client{Timeout: 2 * time.Minute}).Do(request)
	if err != nil {
		return "", "", fmt.Errorf("download pinned core bundle %s: %w", core, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("download pinned core bundle %s: HTTP %d", core, response.StatusCode)
	}
	if response.ContentLength > maxRuntimeBytes {
		return "", "", errors.New("pinned core bundle exceeds safety limit")
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxRuntimeBytes+1))
	if err != nil {
		return "", "", fmt.Errorf("read pinned core bundle %s: %w", core, err)
	}
	if int64(len(payload)) > maxRuntimeBytes {
		return "", "", errors.New("pinned core bundle exceeds safety limit")
	}
	files, err := extractCoreBundle(payload)
	if err != nil {
		return "", "", fmt.Errorf("extract pinned core bundle %s: %w", core, err)
	}
	if err := writeRuntimeCache(jsPath, files.JS); err != nil {
		return "", "", fmt.Errorf("cache core JS %s: %w", core, err)
	}
	if err := writeRuntimeCache(wasmPath, files.WASM); err != nil {
		_ = os.Remove(jsPath)
		return "", "", fmt.Errorf("cache core WASM %s: %w", core, err)
	}
	return destination, contentType, nil
}
