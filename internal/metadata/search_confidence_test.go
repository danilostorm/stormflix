package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSearchIgnoresEmptyNamesAndHandlesAccents(t *testing.T) {
	p := NewTMDBProvider("token", "", "pt-BR")
	p.client = &http.Client{Transport: collectionRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{"id":1,"title":"Unrelated blockbuster","popularity":999999,"release_date":"2003-01-01"},{"id":2,"title":"Looney Tunes: De Volta à Ação","release_date":"2003-01-01"}]}`))}, nil
	})}
	id, err := p.search(context.Background(), "movie", "Looney Tunes De Volta a Acao", 2003)
	if err != nil || id != 2 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	id, err = p.search(context.Background(), "movie", "Completely different title", 2003)
	if err != nil || id != 0 {
		t.Fatalf("must not guess popular result: id=%d err=%v", id, err)
	}
}
func TestSearchMinorTypo(t *testing.T) {
	if searchTitleSimilarity(normalizeSearchTitle("Rambo Last Blod"), normalizeSearchTitle("Rambo: Last Blood")) < .82 {
		t.Fatal("minor typo rejected")
	}
	if searchTitleSimilarity("batman", "") != 0 {
		t.Fatal("empty candidate accepted")
	}
}

func TestThemeSearchUsesShowName(t *testing.T) {
	p := NewThemeProvider("BR")
	p.client = &http.Client{Transport: collectionRoundTripper(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("term"); got != "Resident Evil soundtrack 2022" {
			t.Fatalf("query=%q", got)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[]}`))}, nil
	})}
	if _, _, err := p.Lookup(context.Background(), "Resident Evil S01E01 · Piloto", 2022); err != nil {
		t.Fatal(err)
	}
}
