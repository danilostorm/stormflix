package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type collectionRoundTripper func(*http.Request) (*http.Response, error)

func (f collectionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTMDBMovieCollection(t *testing.T) {
	p := NewTMDBProvider("token", "", "pt-BR")
	p.client = &http.Client{Transport: collectionRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/3/movie/123" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("language") != "pt-BR" {
			t.Fatalf("expected pt-BR language, got %q", r.URL.Query().Get("language"))
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"belongs_to_collection":{"id":456,"name":"Resident Evil Collection"}}`))}, nil
	})}

	collection, err := p.MovieCollection(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if collection.ID != 456 || collection.Name != "Resident Evil Collection" {
		t.Fatalf("unexpected collection: %+v", collection)
	}
}

func TestTMDBMovieWithoutCollection(t *testing.T) {
	p := NewTMDBProvider("token", "", "pt-BR")
	p.client = &http.Client{Transport: collectionRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"belongs_to_collection":null}`))}, nil
	})}
	collection, err := p.MovieCollection(context.Background(), 321)
	if err != nil {
		t.Fatal(err)
	}
	if collection.ID != 0 || collection.Name != "" {
		t.Fatalf("expected no collection, got %+v", collection)
	}
}
