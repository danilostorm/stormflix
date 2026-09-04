package webui

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// Static contains the lightweight StormFlix web client.
//
//go:embed static/*
var Static embed.FS

type cachedAsset struct {
	body        []byte
	gzipBody    []byte
	contentType string
	etag        string
	cache       string
}

type staticHandler struct {
	root  fs.FS
	cache sync.Map
}

// Handler serves the embedded web applications with validation caching and
// on-demand gzip compression. Assets are compressed only once per process;
// subsequent requests reuse the immutable byte slice.
func Handler() http.Handler {
	root, err := fs.Sub(Static, "static")
	if err != nil {
		panic(err)
	}
	return &staticHandler{root: root}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	} else if info, err := fs.Stat(h.root, name); err == nil && info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		name = path.Join(name, "index.html")
	}
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	asset, err := h.asset(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", asset.cache)
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("ETag", asset.etag)
	body := asset.body
	if len(asset.gzipBody) > 0 {
		w.Header().Set("Vary", "Accept-Encoding")
	}
	if len(asset.gzipBody) > 0 && acceptsGzip(r.Header.Get("Accept-Encoding")) && r.Header.Get("Range") == "" {
		body = asset.gzipBody
		w.Header().Set("Content-Encoding", "gzip")
	}
	if matchesETag(r.Header.Get("If-None-Match"), asset.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(body))
}

func (h *staticHandler) asset(name string) (cachedAsset, error) {
	if value, ok := h.cache.Load(name); ok {
		return value.(cachedAsset), nil
	}
	body, err := fs.ReadFile(h.root, name)
	if err != nil {
		return cachedAsset{}, err
	}
	sum := sha256.Sum256(body)
	asset := cachedAsset{
		body: body, contentType: staticContentType(name, body),
		// A weak validator intentionally covers both identity and gzip transfer
		// representations of the same embedded resource.
		etag:  fmt.Sprintf(`W/"%x"`, sum[:12]),
		cache: "public, max-age=3600, stale-while-revalidate=86400",
	}
	if strings.HasSuffix(strings.ToLower(name), ".html") {
		asset.cache = "no-cache"
	} else if name == "bundles/manifest.json" {
		asset.cache = "no-cache"
	} else if immutableAssetName(name) {
		asset.cache = "public, max-age=31536000, immutable"
	}
	if len(body) >= 1024 && compressibleContentType(asset.contentType) {
		var compressed bytes.Buffer
		writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
		_, _ = writer.Write(body)
		_ = writer.Close()
		if compressed.Len() < len(body) {
			asset.gzipBody = compressed.Bytes()
		}
	}
	actual, _ := h.cache.LoadOrStore(name, asset)
	return actual.(cachedAsset), nil
}

func immutableAssetName(name string) bool {
	if strings.HasPrefix(name, "vendor-libmedia/") {
		return true
	}
	base := path.Base(name)
	if strings.HasPrefix(base, "vendor-") {
		return true
	}
	parts := strings.Split(base, ".")
	if len(parts) < 3 {
		return false
	}
	hash := parts[len(parts)-2]
	if len(hash) < 12 {
		return false
	}
	for _, value := range hash {
		if value < '0' || value > '9' && value < 'a' || value > 'f' {
			return false
		}
	}
	return true
}

func staticContentType(name string, body []byte) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".wasm":
		return "application/wasm"
	case ".json", ".map":
		return "application/json; charset=utf-8"
	}
	if value := mime.TypeByExtension(path.Ext(name)); value != "" {
		return value
	}
	return http.DetectContentType(body)
}

func compressibleContentType(value string) bool {
	return strings.HasPrefix(value, "text/") || strings.Contains(value, "javascript") || strings.Contains(value, "json") || strings.Contains(value, "svg") || strings.Contains(value, "wasm")
}

func acceptsGzip(value string) bool {
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ";")
		if len(parts) == 0 || parts[0] != "gzip" && parts[0] != "*" {
			continue
		}
		disabled := false
		for _, parameter := range parts[1:] {
			parameter = strings.ReplaceAll(strings.TrimSpace(parameter), " ", "")
			if strings.HasPrefix(parameter, "q=") && strings.Trim(strings.TrimPrefix(parameter, "q="), "0.") == "" {
				disabled = true
			}
		}
		if !disabled {
			return true
		}
	}
	return false
}

func matchesETag(header, etag string) bool {
	wanted := strings.TrimPrefix(strings.TrimSpace(etag), "W/")
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == wanted {
			return true
		}
	}
	return false
}
