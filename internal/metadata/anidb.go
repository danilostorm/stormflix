package metadata

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const aniDBTitlesURL = "https://anidb.net/api/anime-titles.xml.gz"

type AniDBMatch struct {
	ID    int64
	Title string
}

type AniDBResolver struct {
	client    *http.Client
	mu        sync.RWMutex
	titles    map[string]AniDBMatch
	loadedAt  time.Time
	lastError error
	retryAt   time.Time
}

func NewAniDBResolver() *AniDBResolver {
	return &AniDBResolver{client: &http.Client{Timeout: 45 * time.Second}}
}

func (r *AniDBResolver) Ready() bool { return true }

func (r *AniDBResolver) Resolve(ctx context.Context, titles []string) (AniDBMatch, error) {
	if err := r.ensureLoaded(ctx); err != nil { return AniDBMatch{}, err }
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, title := range titles {
		if match, ok := r.titles[normalizeTitle(title)]; ok { return match, nil }
	}
	return AniDBMatch{}, errors.New("AniDB: no match")
}

func (r *AniDBResolver) ensureLoaded(ctx context.Context) error {
	r.mu.RLock()
	fresh := len(r.titles) > 0 && time.Since(r.loadedAt) < 24*time.Hour
	lastErr, retryAt := r.lastError, r.retryAt
	r.mu.RUnlock()
	if fresh { return nil }
	if lastErr != nil && time.Now().Before(retryAt) { return lastErr }

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.titles) > 0 && time.Since(r.loadedAt) < 24*time.Hour { return nil }
	if r.lastError != nil && time.Now().Before(r.retryAt) { return r.lastError }

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aniDBTitlesURL, nil)
	if err != nil { return err }
	req.Header.Set("User-Agent", "StormFlix/0.6 (+https://github.com/danilostorm/stormflix)")
	resp, err := r.client.Do(req)
	if err != nil { r.rememberError(err); return err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("AniDB titles: HTTP %d", resp.StatusCode); r.rememberError(err); return err
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil { r.rememberError(err); return err }
	defer gz.Close()

	type titleXML struct {
		Type string `xml:"type,attr"`
		Lang string `xml:"lang,attr"`
		Text string `xml:",chardata"`
	}
	type animeXML struct {
		AID    string     `xml:"aid,attr"`
		Titles []titleXML `xml:"title"`
	}
	decoder := xml.NewDecoder(gz)
	index := make(map[string]AniDBMatch, 160000)
	for {
		token, decodeErr := decoder.Token()
		if decodeErr != nil {
			if strings.Contains(strings.ToLower(decodeErr.Error()), "eof") { break }
			r.rememberError(decodeErr); return decodeErr
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "anime" { continue }
		var anime animeXML
		if err := decoder.DecodeElement(&anime, &start); err != nil { r.rememberError(err); return err }
		aid, _ := strconv.ParseInt(anime.AID, 10, 64)
		if aid == 0 { continue }
		canonical := ""
		for _, title := range anime.Titles {
			text := strings.TrimSpace(title.Text); if text == "" { continue }
			lang := strings.ToLower(title.Lang)
			if canonical == "" || lang == "en" || lang == "x-jat" { canonical = text }
		}
		if canonical == "" { continue }
		for _, title := range anime.Titles {
			text := strings.TrimSpace(title.Text); if text == "" { continue }
			key := normalizeTitle(text); if key == "" { continue }
			if _, exists := index[key]; !exists { index[key] = AniDBMatch{ID: aid, Title: canonical} }
		}
	}
	if len(index) == 0 { err = errors.New("AniDB titles: empty index"); r.rememberError(err); return err }
	r.titles = index
	r.loadedAt = time.Now()
	r.lastError = nil
	r.retryAt = time.Time{}
	return nil
}

func (r *AniDBResolver) rememberError(err error) {
	r.lastError = err
	r.retryAt = time.Now().Add(15 * time.Minute)
}
