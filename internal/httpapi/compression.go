package httpapi

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// responseCompression compresses JSON and other textual API responses while
// leaving media, Range responses and already encoded static assets untouched.
func responseCompression(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || !requestAcceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		writer := &compressedResponseWriter{ResponseWriter: w}
		defer writer.Close()
		next.ServeHTTP(writer, r)
	})
}

type compressedResponseWriter struct {
	http.ResponseWriter
	gzipWriter  *gzip.Writer
	wroteHeader bool
	compressed  bool
}

func (w *compressedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *compressedResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	contentType := strings.ToLower(w.Header().Get("Content-Type"))
	if responseCanCompress(status, contentType, w.Header().Get("Content-Encoding")) {
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
		appendVary(w.Header(), "Accept-Encoding")
		w.gzipWriter = gzip.NewWriter(w.ResponseWriter)
		w.compressed = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *compressedResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.compressed {
		return w.gzipWriter.Write(body)
	}
	return w.ResponseWriter.Write(body)
}

func (w *compressedResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.gzipWriter != nil {
		_ = w.gzipWriter.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *compressedResponseWriter) Close() {
	if w.gzipWriter != nil {
		_ = w.gzipWriter.Close()
	}
}

func (w *compressedResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.compressed {
		return io.Copy(w.gzipWriter, reader)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(reader)
	}
	return io.Copy(struct{ io.Writer }{w.ResponseWriter}, reader)
}

func responseCanCompress(status int, contentType, encoding string) bool {
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified || encoding != "" {
		return false
	}
	return strings.HasPrefix(contentType, "application/json") || strings.HasPrefix(contentType, "application/problem+json") || strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "svg")
}

func requestAcceptsGzip(value string) bool {
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

func appendVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
