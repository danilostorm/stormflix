package webui

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerCachesAndCompressesStaticAssets(t *testing.T) {
	handler := Handler()
	request := httptest.NewRequest(http.MethodGet, "/styles.css", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET styles.css status=%d", response.Code)
	}
	if response.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding=%q, want gzip", response.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "max-age=3600") {
		t.Fatalf("Cache-Control=%q", response.Header().Get("Cache-Control"))
	}
	etag := response.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("ETag=%q, want weak validator shared across encodings", etag)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	uncompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(uncompressed), ".topbar") {
		t.Fatal("compressed stylesheet body is not the expected asset")
	}

	conditional := httptest.NewRequest(http.MethodGet, "/styles.css", nil)
	conditional.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	handler.ServeHTTP(notModified, conditional)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional status=%d body=%d", notModified.Code, notModified.Body.Len())
	}
	if notModified.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("304 Vary=%q", notModified.Header().Get("Vary"))
	}
}

func TestHandlerDoesNotPersistHTMLAndSupportsAdminIndex(t *testing.T) {
	for _, target := range []string{"/", "/admin/"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d", target, response.Code)
		}
		if response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("GET %s Cache-Control=%q", target, response.Header().Get("Cache-Control"))
		}
		if !strings.Contains(strings.ToLower(response.Body.String()), "<!doctype html>") {
			t.Fatalf("GET %s did not serve an HTML index", target)
		}
	}
}

func TestHandlerRejectsUnsupportedMethods(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/app.js", nil)
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST app.js status=%d", response.Code)
	}
}
