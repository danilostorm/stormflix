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

func TestSearchVerifiesAlternativeTitleAndYear(t *testing.T) {
	for _, mediaType := range []string{"movie", "tv"} {
		t.Run(mediaType, func(t *testing.T) {
			calls := 0
			p := NewTMDBProvider("token", "", "pt-BR")
			p.client = &http.Client{Transport: collectionRoundTripper(func(r *http.Request) (*http.Response, error) {
				body := `{"results":[{"id":7,"title":"Another Movie","release_date":"1981-01-01"},{"id":8,"title":"The Last Shark","original_title":"L'ultimo squalo","release_date":"1981-01-01"},{"id":9,"title":"Remake","release_date":"2020-01-01"}]}`
				if strings.HasSuffix(r.URL.Path, "/alternative_titles") {
					calls++
					body = `{"titles":[]}`
					if strings.Contains(r.URL.Path, "/8/") {
						if mediaType == "movie" {
							body = `{"titles":[{"title":"Last Jaws"}]}`
						} else {
							body = `{"results":[{"title":"Last Jaws"}]}`
						}
					}
					if strings.Contains(r.URL.Path, "/9/") {
						t.Fatal("queried wrong-year alias")
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			id, err := p.search(context.Background(), mediaType, "Last Jaws", 1981)
			if err != nil || id != 8 || calls != 2 {
				t.Fatalf("id=%d calls=%d err=%v", id, calls, err)
			}
			id, err = p.search(context.Background(), mediaType, "Unknown", 1981)
			if err != nil || id != 0 {
				t.Fatalf("unverified alias accepted: %d %v", id, err)
			}
		})
	}
}
