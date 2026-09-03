package httpapi

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseCompressionCompressesJSON(t *testing.T) {
	handler := responseCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"rows":["`+strings.Repeat("StormFlix", 200)+`"]}`)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding=%q", response.Header().Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "StormFlix") {
		t.Fatalf("unexpected decompressed response: %q", body)
	}
}

func TestResponseCompressionLeavesMediaAndRangesUnchanged(t *testing.T) {
	handler := responseCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "media-bytes")
	}))
	for _, withRange := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/media/1/stream", nil)
		request.Header.Set("Accept-Encoding", "gzip")
		if withRange {
			request.Header.Set("Range", "bytes=0-4")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Header().Get("Content-Encoding") != "" || response.Body.String() != "media-bytes" {
			t.Fatalf("range=%t encoding=%q body=%q", withRange, response.Header().Get("Content-Encoding"), response.Body.String())
		}
	}
}

func TestRequestAcceptsGzipHonorsDisabledEncoding(t *testing.T) {
	if requestAcceptsGzip("gzip;q=0") {
		t.Fatal("gzip;q=0 must not enable compression")
	}
	if !requestAcceptsGzip("br, gzip;q=0.5") {
		t.Fatal("positive gzip quality should enable compression")
	}
}
