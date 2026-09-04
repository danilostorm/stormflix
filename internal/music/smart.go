package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var smartProviderConfig struct {
	sync.RWMutex
	lastFMAPIKey string
}

var smartSchemaMu sync.Mutex
var smartSchemaReady bool

// ConfigureProviders updates optional music providers without restarting StormFlix.
func ConfigureProviders(lastFMAPIKey string) {
	smartProviderConfig.Lock()
	smartProviderConfig.lastFMAPIKey = strings.TrimSpace(lastFMAPIKey)
	smartProviderConfig.Unlock()
}

func lastFMKey() string {
	smartProviderConfig.RLock()
	defer smartProviderConfig.RUnlock()
	return smartProviderConfig.lastFMAPIKey
}

// AgentsConfigured is the runtime-aware music agent list used by the Admin.
func (s *Service) AgentsConfigured() []AgentStatus {
	agents := s.Agents()
	keyReady := lastFMKey() != ""
	for i := range agents {
		if agents[i].Name == "Last.fm" {
			agents[i].Enabled = keyReady
			agents[i].Ready = keyReady
			agents[i].Description = "Fallback opcional para corrigir artista, faixa, álbum, tags e identidade quando os arquivos locais não possuem tags confiáveis. Requer API key."
		}
	}
	agents = append([]AgentStatus{{Name: "Parser de arquivo", Enabled: true, Ready: true, Description: "Reconhece padrões como Artista - Título diretamente no nome do arquivo antes de confiar na estrutura de pastas."}}, agents...)
	return agents
}

type SmartIndexStatus struct {
	IndexStatus
	PendingMatches   int64 `json:"pending_matches"`
	AttemptedMatches int64 `json:"attempted_matches"`
	MatchedTracks    int64 `json:"matched_tracks"`
	ParsedTracks     int64 `json:"parsed_tracks"`
	UnmatchedTracks  int64 `json:"unmatched_tracks"`
}

func (s *Service) SmartStatus(ctx context.Context) SmartIndexStatus {
	base := s.Status(ctx)
	out := SmartIndexStatus{IndexStatus: base}
	if err := s.ensureSmartSchema(ctx); err != nil {
		return out
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id JOIN music_tracks mt ON mt.media_id=m.id LEFT JOIN music_match_attempts ma ON ma.media_id=m.id WHERE m.available=1 AND l.kind='music' AND (ma.media_id IS NULL OR ma.indexed_modified_unix<>m.modified_unix)`).Scan(&out.PendingMatches)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_match_attempts`).Scan(&out.AttemptedMatches)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_match_attempts WHERE status IN ('lastfm','musicbrainz')`).Scan(&out.MatchedTracks)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_match_attempts WHERE status='parsed'`).Scan(&out.ParsedTracks)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_match_attempts WHERE status='unmatched'`).Scan(&out.UnmatchedTracks)

	if out.Indexing && out.PendingTracks == 0 && out.PendingMatches > 0 {
		out.Phase = "matching"
		out.PhaseLabel = "Identificando músicas"
		if lastFMKey() != "" {
			out.Message = "Corrigindo artista, faixa e álbum com nome do arquivo, Last.fm e MusicBrainz."
		} else {
			out.Message = "Corrigindo artista, faixa e álbum pelo nome do arquivo e MusicBrainz. Configure Last.fm para adicionar mais um fallback."
		}
		if out.TotalTracks > 0 {
			attempted := out.TotalTracks - out.PendingMatches
			if attempted < 0 {
				attempted = 0
			}
			out.Progress = float64(attempted) * 100 / float64(out.TotalTracks)
		}
	} else if !out.Indexing && out.PendingTracks == 0 && out.PendingMatches > 0 {
		out.Phase = "waiting_matches"
		out.PhaseLabel = "Identificação pendente"
		out.Message = "As tags técnicas estão prontas; ainda existem músicas aguardando identificação por nome/agentes."
		if out.TotalTracks > 0 {
			out.Progress = float64(out.TotalTracks-out.PendingMatches) * 100 / float64(out.TotalTracks)
		}
	}
	return out
}

// StartSmartIndexing runs the local tag reader, filename matcher, optional Last.fm
// fallback and MusicBrainz/Cover Art enrichment in bounded batches.
func (s *Service) StartSmartIndexing() bool {
	s.indexMu.Lock()
	if s.indexing {
		s.indexMu.Unlock()
		return false
	}
	s.indexing = true
	s.indexMu.Unlock()
	go func() {
		defer func() {
			s.indexMu.Lock()
			s.indexing = false
			s.indexMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		_ = s.ensureSmartSchema(ctx)
		_ = s.indexMissing(ctx)
		_ = s.identifySmartTracks(ctx, 80)
		_ = s.enrichMissingAlbums(ctx, 30)
	}()
	return true
}

func (s *Service) ensureSmartSchema(ctx context.Context) error {
	smartSchemaMu.Lock()
	defer smartSchemaMu.Unlock()
	if smartSchemaReady {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS music_match_attempts (
    media_id INTEGER PRIMARY KEY,
    indexed_modified_unix INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    confidence INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_music_match_status ON music_match_attempts(status,updated_at);`)
	if err == nil {
		smartSchemaReady = true
	}
	return err
}

type smartCandidate struct {
	id                  int64
	path                string
	modified            int64
	title               string
	artist              string
	albumArtist         string
	album               string
	year                int
	genre               string
	musicBrainzTrackID  string
	musicBrainzAlbumID  string
	musicBrainzArtistID string
}

type smartMatch struct {
	provider            string
	confidence          int
	title               string
	artist              string
	album               string
	year                int
	genre               string
	musicBrainzTrackID  string
	musicBrainzAlbumID  string
	musicBrainzArtistID string
	coverURL            string
}

func (s *Service) identifySmartTracks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 80
	}
	if err := s.ensureSmartSchema(ctx); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.path,m.modified_unix,COALESCE(mt.title,''),COALESCE(mt.artist,''),COALESCE(mt.album_artist,''),COALESCE(mt.album,''),COALESCE(mt.year,0),COALESCE(mt.genre,''),COALESCE(mt.musicbrainz_track_id,''),COALESCE(mt.musicbrainz_album_id,''),COALESCE(mt.musicbrainz_artist_id,'')
FROM media m JOIN libraries l ON l.id=m.library_id JOIN music_tracks mt ON mt.media_id=m.id LEFT JOIN music_match_attempts ma ON ma.media_id=m.id
WHERE m.available=1 AND l.kind='music' AND (ma.media_id IS NULL OR ma.indexed_modified_unix<>m.modified_unix)
ORDER BY m.id LIMIT ?`, limit)
	if err != nil {
		return err
	}
	items := []smartCandidate{}
	for rows.Next() {
		var c smartCandidate
		if err := rows.Scan(&c.id, &c.path, &c.modified, &c.title, &c.artist, &c.albumArtist, &c.album, &c.year, &c.genre, &c.musicBrainzTrackID, &c.musicBrainzAlbumID, &c.musicBrainzArtistID); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, c)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.gate.Wait(ctx, "music_identification", nil); err != nil {
			return err
		}
		fileArtist, fileTitle := artistTitleFromFilename(item.path)
		queryTitle := firstNonEmpty(fileTitle, item.title)
		queryArtist := firstNonEmpty(fileArtist, cleanFallbackArtist(item.artist, item.path))
		match := smartMatch{}
		var lookupErr error

		if queryArtist != "" && queryTitle != "" && lastFMKey() != "" {
			match, lookupErr = s.lookupLastFMTrack(ctx, queryArtist, queryTitle)
		}
		if match.provider == "" && queryArtist != "" && queryTitle != "" {
			match, lookupErr = s.lookupMusicBrainzRecording(ctx, queryArtist, queryTitle)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1100 * time.Millisecond):
			}
		}

		status := "unmatched"
		provider := ""
		confidence := 0
		finalTitle := item.title
		finalArtist := item.artist
		finalAlbumArtist := item.albumArtist
		finalAlbum := item.album
		finalYear := item.year
		finalGenre := item.genre
		mbTrack := item.musicBrainzTrackID
		mbAlbum := item.musicBrainzAlbumID
		mbArtist := item.musicBrainzArtistID

		if match.provider != "" {
			status = match.provider
			provider = match.provider
			confidence = match.confidence
			finalTitle = firstNonEmpty(match.title, queryTitle, finalTitle)
			finalArtist = firstNonEmpty(match.artist, queryArtist, finalArtist)
			finalAlbumArtist = finalArtist
			finalAlbum = firstNonEmpty(match.album, cleanFallbackAlbum(finalAlbum, item.path), "Singles")
			finalYear = firstPositive(match.year, finalYear)
			finalGenre = firstNonEmpty(match.genre, finalGenre)
			mbTrack = firstNonEmpty(match.musicBrainzTrackID, mbTrack)
			mbAlbum = firstNonEmpty(match.musicBrainzAlbumID, mbAlbum)
			mbArtist = firstNonEmpty(match.musicBrainzArtistID, mbArtist)
		} else if fileArtist != "" && fileTitle != "" {
			status = "parsed"
			provider = "filename"
			confidence = 55
			finalTitle = fileTitle
			finalArtist = fileArtist
			finalAlbumArtist = fileArtist
			finalAlbum = cleanFallbackAlbum(finalAlbum, item.path)
			if finalAlbum == "" {
				finalAlbum = "Singles"
			}
		}

		if _, err := s.db.ExecContext(ctx, `UPDATE music_tracks SET title=?,artist=?,album_artist=?,album=?,year=?,genre=?,musicbrainz_track_id=?,musicbrainz_album_id=?,musicbrainz_artist_id=?,updated_at=CURRENT_TIMESTAMP WHERE media_id=?`, finalTitle, finalArtist, finalAlbumArtist, finalAlbum, finalYear, finalGenre, mbTrack, mbAlbum, mbArtist, item.id); err != nil {
			return err
		}
		lastError := ""
		if status == "unmatched" && lookupErr != nil {
			lastError = lookupErr.Error()
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO music_match_attempts(media_id,indexed_modified_unix,status,provider,confidence,last_error,updated_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET indexed_modified_unix=excluded.indexed_modified_unix,status=excluded.status,provider=excluded.provider,confidence=excluded.confidence,last_error=excluded.last_error,updated_at=CURRENT_TIMESTAMP`, item.id, item.modified, status, provider, confidence, lastError)
		if err != nil {
			return err
		}
		if status == "lastfm" || status == "musicbrainz" {
			albumKey := AlbumKey(firstNonEmpty(finalAlbumArtist, finalArtist), finalAlbum)
			cover := match.coverURL
			if cover == "" && mbAlbum != "" && status == "musicbrainz" {
				cover = "https://coverartarchive.org/release/" + mbAlbum + "/front-500"
			}
			_, _ = s.db.ExecContext(ctx, `INSERT INTO music_albums(album_key,title,artist,year,musicbrainz_release_id,cover_url,last_enriched_at,updated_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
ON CONFLICT(album_key) DO UPDATE SET title=excluded.title,artist=excluded.artist,year=CASE WHEN excluded.year>0 THEN excluded.year ELSE music_albums.year END,musicbrainz_release_id=CASE WHEN excluded.musicbrainz_release_id<>'' THEN excluded.musicbrainz_release_id ELSE music_albums.musicbrainz_release_id END,cover_url=CASE WHEN excluded.cover_url<>'' THEN excluded.cover_url ELSE music_albums.cover_url END,last_enriched_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, albumKey, finalAlbum, firstNonEmpty(finalAlbumArtist, finalArtist), finalYear, mbAlbum, cover)
		}
	}
	return nil
}

func artistTitleFromFilename(path string) (string, string) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = cleanTrackTitle(name)
	for _, sep := range []string{" - ", " – ", " — ", " − "} {
		parts := strings.Split(name, sep)
		if len(parts) >= 2 {
			artist := strings.TrimSpace(parts[0])
			title := strings.TrimSpace(strings.Join(parts[1:], " - "))
			if artist != "" && title != "" && len(artist) <= 140 {
				return artist, title
			}
		}
	}
	return "", name
}

func cleanFallbackArtist(value, path string) string {
	value = strings.TrimSpace(value)
	parent2 := filepath.Base(filepath.Dir(filepath.Dir(path)))
	if value == "" || strings.EqualFold(value, parent2) || looksLikeCollectionFolder(value) {
		return ""
	}
	return value
}

func cleanFallbackAlbum(value, path string) string {
	value = strings.TrimSpace(value)
	parent := filepath.Base(filepath.Dir(path))
	if value == "" || strings.EqualFold(value, parent) || looksLikeCollectionFolder(value) {
		return ""
	}
	return value
}

func looksLikeCollectionFolder(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	bad := []string{"músicas", "musicas", "music", "songs", "coletânea", "coletanea", "collection", "academia", "playlist", "gb", "faixas", "tracks"}
	for _, token := range bad {
		if strings.Contains(v, token) {
			return true
		}
	}
	return false
}

func (s *Service) lookupLastFMTrack(ctx context.Context, artist, title string) (smartMatch, error) {
	key := lastFMKey()
	if key == "" {
		return smartMatch{}, errors.New("Last.fm API key is not configured")
	}
	params := url.Values{"method": {"track.getInfo"}, "api_key": {key}, "artist": {artist}, "track": {title}, "autocorrect": {"1"}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ws.audioscrobbler.com/2.0/?"+params.Encode(), nil)
	if err != nil {
		return smartMatch{}, err
	}
	req.Header.Set("User-Agent", "StormFlix/0.14 (+https://github.com/danilostorm/stormflix)")
	resp, err := s.client.Do(req)
	if err != nil {
		return smartMatch{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return smartMatch{}, fmt.Errorf("Last.fm HTTP %d", resp.StatusCode)
	}
	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
		Track   struct {
			Name   string `json:"name"`
			MBID   string `json:"mbid"`
			Artist struct {
				Name string `json:"name"`
				MBID string `json:"mbid"`
			} `json:"artist"`
			Album struct {
				Title string `json:"title"`
				MBID  string `json:"mbid"`
				Image []struct {
					URL  string `json:"#text"`
					Size string `json:"size"`
				} `json:"image"`
			} `json:"album"`
			TopTags struct {
				Tag []struct {
					Name string `json:"name"`
				} `json:"tag"`
			} `json:"toptags"`
		} `json:"track"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return smartMatch{}, err
	}
	if result.Error != 0 || strings.TrimSpace(result.Track.Name) == "" {
		return smartMatch{}, fmt.Errorf("Last.fm: %s", firstNonEmpty(result.Message, "no match"))
	}
	genre := ""
	if len(result.Track.TopTags.Tag) > 0 {
		genre = result.Track.TopTags.Tag[0].Name
	}
	cover := ""
	for i := len(result.Track.Album.Image) - 1; i >= 0; i-- {
		if strings.TrimSpace(result.Track.Album.Image[i].URL) != "" {
			cover = strings.TrimSpace(result.Track.Album.Image[i].URL)
			break
		}
	}
	return smartMatch{provider: "lastfm", confidence: 92, title: result.Track.Name, artist: result.Track.Artist.Name, album: result.Track.Album.Title, genre: genre, musicBrainzTrackID: result.Track.MBID, musicBrainzAlbumID: result.Track.Album.MBID, musicBrainzArtistID: result.Track.Artist.MBID, coverURL: cover}, nil
}

func (s *Service) lookupMusicBrainzRecording(ctx context.Context, artist, title string) (smartMatch, error) {
	query := fmt.Sprintf(`recording:"%s" AND artist:"%s"`, escapeMusicBrainzQuery(title), escapeMusicBrainzQuery(artist))
	params := url.Values{"query": {query}, "fmt": {"json"}, "limit": {"3"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://musicbrainz.org/ws/2/recording/?"+params.Encode(), nil)
	if err != nil {
		return smartMatch{}, err
	}
	req.Header.Set("User-Agent", "StormFlix/0.14 (+https://github.com/danilostorm/stormflix)")
	resp, err := s.client.Do(req)
	if err != nil {
		return smartMatch{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return smartMatch{}, fmt.Errorf("MusicBrainz HTTP %d", resp.StatusCode)
	}
	var result struct {
		Recordings []struct {
			ID           string `json:"id"`
			Score        int    `json:"score"`
			Title        string `json:"title"`
			ArtistCredit []struct {
				Name   string `json:"name"`
				Artist struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"artist-credit"`
			Releases []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Date  string `json:"date"`
			} `json:"releases"`
		} `json:"recordings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return smartMatch{}, err
	}
	if len(result.Recordings) == 0 || result.Recordings[0].Score < 80 {
		return smartMatch{}, sql.ErrNoRows
	}
	best := result.Recordings[0]
	out := smartMatch{provider: "musicbrainz", confidence: best.Score, title: best.Title, musicBrainzTrackID: best.ID}
	if len(best.ArtistCredit) > 0 {
		out.artist = firstNonEmpty(best.ArtistCredit[0].Name, best.ArtistCredit[0].Artist.Name)
		out.musicBrainzArtistID = best.ArtistCredit[0].Artist.ID
	}
	if len(best.Releases) > 0 {
		out.album = best.Releases[0].Title
		out.musicBrainzAlbumID = best.Releases[0].ID
		out.year = parseYear(best.Releases[0].Date)
		out.coverURL = "https://coverartarchive.org/release/" + best.Releases[0].ID + "/front-500"
	}
	return out, nil
}

func (s *Service) ResetSmartMatches(ctx context.Context) error {
	if err := s.ensureSmartSchema(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM music_match_attempts`)
	return err
}

func parseSmartInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}
