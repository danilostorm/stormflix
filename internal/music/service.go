package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	db      *sql.DB
	client  *http.Client
	indexMu sync.Mutex
	indexing bool
}

type Track struct {
	ID                 int64   `json:"id"`
	LibraryID          int64   `json:"library_id"`
	LibraryName        string  `json:"library_name"`
	Title              string  `json:"title"`
	Artist             string  `json:"artist"`
	AlbumArtist        string  `json:"album_artist"`
	Album              string  `json:"album"`
	TrackNumber        int     `json:"track_number"`
	DiscNumber         int     `json:"disc_number"`
	Year               int     `json:"year"`
	Genre              string  `json:"genre"`
	DurationSeconds    float64 `json:"duration_seconds"`
	Extension          string  `json:"extension"`
	SizeBytes          int64   `json:"size_bytes"`
	ModifiedUnix       int64   `json:"modified_unix"`
	Codec              string  `json:"codec"`
	Bitrate            int     `json:"bitrate"`
	SampleRate         int     `json:"sample_rate"`
	Channels           int     `json:"channels"`
	MusicBrainzTrackID string  `json:"musicbrainz_track_id,omitempty"`
	CoverURL           string  `json:"cover_url"`
	Favorite           bool    `json:"favorite,omitempty"`
}

type Album struct {
	Key                   string  `json:"key"`
	Title                 string  `json:"title"`
	Artist                string  `json:"artist"`
	Year                  int     `json:"year"`
	TrackCount            int     `json:"track_count"`
	DurationSeconds       float64 `json:"duration_seconds"`
	CoverURL              string  `json:"cover_url"`
	RepresentativeTrackID int64   `json:"representative_track_id"`
	ModifiedUnix          int64   `json:"modified_unix"`
}

type Artist struct {
	Name       string `json:"name"`
	AlbumCount int    `json:"album_count"`
	TrackCount int    `json:"track_count"`
}

type Home struct {
	RecentlyAddedAlbums []Album  `json:"recently_added_albums"`
	RecentlyAddedTracks []Track  `json:"recently_added_tracks"`
	RecentlyPlayed      []Track  `json:"recently_played"`
	MostPlayed          []Track  `json:"most_played"`
	Artists             []Artist `json:"artists"`
	Indexing            bool     `json:"indexing"`
}

type Lyrics struct {
	Provider     string `json:"provider"`
	ProviderID   string `json:"provider_id"`
	PlainLyrics  string `json:"plain_lyrics"`
	SyncedLyrics string `json:"synced_lyrics"`
	Instrumental bool   `json:"instrumental"`
}

type AgentStatus struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Ready       bool   `json:"ready"`
	Description string `json:"description"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *Service) Agents() []AgentStatus {
	_, ffprobeErr := exec.LookPath("ffprobe")
	return []AgentStatus{
		{Name: "FFprobe Tags", Enabled: true, Ready: ffprobeErr == nil, Description: "Lê tags locais, duração, codec, bitrate, sample rate e canais de MP3, FLAC, M4A/AAC, OGG/Opus, WAV, APE e outros formatos suportados pelo FFmpeg."},
		{Name: "MusicBrainz", Enabled: true, Ready: true, Description: "Identidade de artistas e álbuns, IDs estáveis e correspondência para enriquecer a biblioteca local."},
		{Name: "Cover Art Archive", Enabled: true, Ready: true, Description: "Capas de álbuns associadas aos lançamentos encontrados no MusicBrainz."},
		{Name: "LRCLIB", Enabled: true, Ready: true, Description: "Letras sob demanda, incluindo letras sincronizadas quando disponíveis; não exige chave de API."},
		{Name: "Last.fm", Enabled: false, Ready: false, Description: "Reservado para uma camada opcional de biografia e descoberta. Requer API key e não é necessário para catalogar seus próprios arquivos."},
	}
}

func (s *Service) IsIndexing() bool {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return s.indexing
}

func (s *Service) StartIndexing() bool {
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
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		defer cancel()
		_ = s.indexMissing(ctx)
		_ = s.enrichMissingAlbums(ctx, 30)
	}()
	return true
}

func (s *Service) Home(ctx context.Context, profileID int64, allowedLibraryIDs []int64) (Home, error) {
	s.StartIndexing()
	tracks, err := s.Tracks(ctx, profileID, allowedLibraryIDs, "", 1200)
	if err != nil {
		return Home{}, err
	}
	out := Home{Indexing: s.IsIndexing(), RecentlyAddedAlbums: []Album{}, RecentlyAddedTracks: []Track{}, RecentlyPlayed: []Track{}, MostPlayed: []Track{}, Artists: []Artist{}}
	if len(tracks) == 0 {
		return out, nil
	}

	recentTracks := append([]Track(nil), tracks...)
	sort.SliceStable(recentTracks, func(i, j int) bool { return recentTracks[i].ModifiedUnix > recentTracks[j].ModifiedUnix })
	if len(recentTracks) > 30 {
		recentTracks = recentTracks[:30]
	}
	out.RecentlyAddedTracks = recentTracks

	albumMap := map[string]*Album{}
	artistAlbums := map[string]map[string]bool{}
	artistTracks := map[string]int{}
	for _, track := range tracks {
		key := AlbumKey(firstNonEmpty(track.AlbumArtist, track.Artist), track.Album)
		if key == "|" {
			key = AlbumKey(track.Artist, "Singles")
		}
		album := albumMap[key]
		if album == nil {
			album = &Album{Key: key, Title: firstNonEmpty(track.Album, "Singles"), Artist: firstNonEmpty(track.AlbumArtist, track.Artist, "Artista desconhecido"), Year: track.Year, CoverURL: track.CoverURL, RepresentativeTrackID: track.ID, ModifiedUnix: track.ModifiedUnix}
			albumMap[key] = album
		}
		album.TrackCount++
		album.DurationSeconds += track.DurationSeconds
		if album.Year == 0 || (track.Year > 0 && track.Year < album.Year) {
			album.Year = track.Year
		}
		if track.ModifiedUnix > album.ModifiedUnix {
			album.ModifiedUnix = track.ModifiedUnix
			album.RepresentativeTrackID = track.ID
		}
		if album.CoverURL == "" && track.CoverURL != "" {
			album.CoverURL = track.CoverURL
		}
		artist := firstNonEmpty(track.AlbumArtist, track.Artist, "Artista desconhecido")
		artistTracks[artist]++
		if artistAlbums[artist] == nil {
			artistAlbums[artist] = map[string]bool{}
		}
		artistAlbums[artist][key] = true
	}
	for _, album := range albumMap {
		out.RecentlyAddedAlbums = append(out.RecentlyAddedAlbums, *album)
	}
	sort.SliceStable(out.RecentlyAddedAlbums, func(i, j int) bool { return out.RecentlyAddedAlbums[i].ModifiedUnix > out.RecentlyAddedAlbums[j].ModifiedUnix })
	if len(out.RecentlyAddedAlbums) > 30 {
		out.RecentlyAddedAlbums = out.RecentlyAddedAlbums[:30]
	}
	for name, count := range artistTracks {
		out.Artists = append(out.Artists, Artist{Name: name, TrackCount: count, AlbumCount: len(artistAlbums[name])})
	}
	sort.SliceStable(out.Artists, func(i, j int) bool {
		if out.Artists[i].TrackCount == out.Artists[j].TrackCount {
			return strings.ToLower(out.Artists[i].Name) < strings.ToLower(out.Artists[j].Name)
		}
		return out.Artists[i].TrackCount > out.Artists[j].TrackCount
	})
	if len(out.Artists) > 30 {
		out.Artists = out.Artists[:30]
	}

	byID := make(map[int64]Track, len(tracks))
	for _, track := range tracks {
		byID[track.ID] = track
	}
	out.RecentlyPlayed = tracksForIDs(byID, s.recentTrackIDs(ctx, profileID, 24))
	out.MostPlayed = tracksForIDs(byID, s.mostPlayedIDs(ctx, 24))
	return out, nil
}

func (s *Service) Tracks(ctx context.Context, profileID int64, allowedLibraryIDs []int64, query string, limit int) ([]Track, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Track{}, nil
	}
	args := []any{}
	q := `SELECT m.id,m.library_id,l.name,m.title,m.path,m.extension,m.size_bytes,m.modified_unix,
COALESCE(mt.title,''),COALESCE(mt.artist,''),COALESCE(mt.album_artist,''),COALESCE(mt.album,''),COALESCE(mt.track_number,0),COALESCE(mt.disc_number,0),COALESCE(mt.year,0),COALESCE(mt.genre,''),COALESCE(mt.duration_seconds,0),COALESCE(mt.codec,''),COALESCE(mt.bitrate,0),COALESCE(mt.sample_rate,0),COALESCE(mt.channels,0),COALESCE(mt.musicbrainz_track_id,'')
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN music_tracks mt ON mt.media_id=m.id
WHERE m.available=1 AND l.kind='music'`
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		q += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	query = strings.TrimSpace(query)
	if query != "" {
		like := "%" + query + "%"
		q += ` AND (m.title LIKE ? OR COALESCE(mt.title,'') LIKE ? OR COALESCE(mt.artist,'') LIKE ? OR COALESCE(mt.album,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY COALESCE(NULLIF(mt.album_artist,''),NULLIF(mt.artist,''),m.title) COLLATE NOCASE,COALESCE(NULLIF(mt.album,''),'') COLLATE NOCASE,COALESCE(mt.disc_number,0),COALESCE(mt.track_number,0),m.title COLLATE NOCASE LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tracks := []Track{}
	paths := []string{}
	for rows.Next() {
		var t Track
		var fallbackTitle, path string
		if err := rows.Scan(&t.ID, &t.LibraryID, &t.LibraryName, &fallbackTitle, &path, &t.Extension, &t.SizeBytes, &t.ModifiedUnix, &t.Title, &t.Artist, &t.AlbumArtist, &t.Album, &t.TrackNumber, &t.DiscNumber, &t.Year, &t.Genre, &t.DurationSeconds, &t.Codec, &t.Bitrate, &t.SampleRate, &t.Channels, &t.MusicBrainzTrackID); err != nil {
			return nil, err
		}
		applyFallbacks(&t, fallbackTitle, path)
		tracks = append(tracks, t)
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	covers, _ := s.albumCovers(ctx)
	favorites, _ := s.favoriteIDs(ctx, profileID)
	for i := range tracks {
		tracks[i].CoverURL = covers[AlbumKey(firstNonEmpty(tracks[i].AlbumArtist, tracks[i].Artist), tracks[i].Album)]
		tracks[i].Favorite = favorites[tracks[i].ID]
	}
	_ = paths
	return tracks, nil
}

func (s *Service) Track(ctx context.Context, profileID, mediaID int64, allowedLibraryIDs []int64) (Track, error) {
	tracks, err := s.Tracks(ctx, profileID, allowedLibraryIDs, "", 5000)
	if err != nil {
		return Track{}, err
	}
	for _, track := range tracks {
		if track.ID == mediaID {
			return track, nil
		}
	}
	return Track{}, sql.ErrNoRows
}

func (s *Service) RecordListening(ctx context.Context, profileID, mediaID int64, delta float64, started, completed bool) error {
	if profileID <= 0 || mediaID <= 0 {
		return errors.New("profile and track are required")
	}
	if delta < 0 {
		delta = 0
	}
	if delta > 90 {
		delta = 90
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.id=? AND m.available=1 AND l.kind='music'`, mediaID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	plays := 0
	if started {
		plays = 1
	}
	done := 0
	if completed {
		done = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO music_listening_daily(day,profile_id,media_id,listened_seconds,plays,completed,last_played_at)
VALUES(date('now'),?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(day,profile_id,media_id) DO UPDATE SET
 listened_seconds=MIN(86400,music_listening_daily.listened_seconds+excluded.listened_seconds),
 plays=music_listening_daily.plays+excluded.plays,
 completed=MAX(music_listening_daily.completed,excluded.completed),
 last_played_at=CURRENT_TIMESTAMP`, profileID, mediaID, delta, plays, done)
	return err
}

func (s *Service) ToggleFavorite(ctx context.Context, profileID, mediaID int64) (bool, error) {
	if profileID <= 0 {
		return false, errors.New("select a profile first")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_favorites WHERE profile_id=? AND media_id=?`, profileID, mediaID).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM music_favorites WHERE profile_id=? AND media_id=?`, profileID, mediaID)
		return false, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO music_favorites(profile_id,media_id) VALUES(?,?)`, profileID, mediaID)
	return true, err
}

func (s *Service) Lyrics(ctx context.Context, mediaID int64) (Lyrics, error) {
	var cached Lyrics
	var instrumental int
	err := s.db.QueryRowContext(ctx, `SELECT provider,provider_id,plain_lyrics,synced_lyrics,instrumental FROM music_lyrics WHERE media_id=?`, mediaID).
		Scan(&cached.Provider, &cached.ProviderID, &cached.PlainLyrics, &cached.SyncedLyrics, &instrumental)
	if err == nil {
		cached.Instrumental = instrumental != 0
		return cached, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Lyrics{}, err
	}
	track, err := s.trackWithoutPermissions(ctx, mediaID)
	if err != nil {
		return Lyrics{}, err
	}
	if strings.TrimSpace(track.Title) == "" || strings.TrimSpace(track.Artist) == "" {
		return Lyrics{}, errors.New("track needs title and artist before lyrics can be matched")
	}
	params := url.Values{}
	params.Set("track_name", track.Title)
	params.Set("artist_name", track.Artist)
	if track.Album != "" {
		params.Set("album_name", track.Album)
	}
	if track.DurationSeconds > 0 {
		params.Set("duration", strconv.Itoa(int(track.DurationSeconds+0.5)))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://lrclib.net/api/get?"+params.Encode(), nil)
	if err != nil {
		return Lyrics{}, err
	}
	req.Header.Set("User-Agent", "StormFlix/0.13 (+https://github.com/danilostorm/stormflix)")
	resp, err := s.client.Do(req)
	if err != nil {
		return Lyrics{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Lyrics{}, sql.ErrNoRows
	}
	if resp.StatusCode != http.StatusOK {
		return Lyrics{}, fmt.Errorf("LRCLIB HTTP %d", resp.StatusCode)
	}
	var result struct {
		ID           int64  `json:"id"`
		PlainLyrics  string `json:"plainLyrics"`
		SyncedLyrics string `json:"syncedLyrics"`
		Instrumental bool   `json:"instrumental"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Lyrics{}, err
	}
	out := Lyrics{Provider: "lrclib", ProviderID: strconv.FormatInt(result.ID, 10), PlainLyrics: result.PlainLyrics, SyncedLyrics: result.SyncedLyrics, Instrumental: result.Instrumental}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO music_lyrics(media_id,provider,provider_id,plain_lyrics,synced_lyrics,instrumental,updated_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET provider=excluded.provider,provider_id=excluded.provider_id,plain_lyrics=excluded.plain_lyrics,synced_lyrics=excluded.synced_lyrics,instrumental=excluded.instrumental,updated_at=CURRENT_TIMESTAMP`, mediaID, out.Provider, out.ProviderID, out.PlainLyrics, out.SyncedLyrics, out.Instrumental)
	return out, nil
}

func (s *Service) indexMissing(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.path,m.modified_unix FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN music_tracks mt ON mt.media_id=m.id WHERE m.available=1 AND l.kind='music' AND (mt.media_id IS NULL OR COALESCE(mt.indexed_modified_unix,0)<>m.modified_unix) ORDER BY m.id LIMIT 5000`)
	if err != nil {
		return err
	}
	type pending struct {
		id       int64
		path     string
		modified int64
	}
	items := []pending{}
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.path, &item.modified); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		probeCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		data, probeErr := probeTrack(probeCtx, item.path)
		cancel()
		if probeErr != nil {
			data = probeResult{}
		}
		fallback := Track{}
		applyFallbacks(&fallback, cleanTrackTitle(strings.TrimSuffix(filepath.Base(item.path), filepath.Ext(item.path))), item.path)
		data.Title = firstNonEmpty(data.Title, fallback.Title)
		data.Artist = firstNonEmpty(data.Artist, fallback.Artist)
		data.AlbumArtist = firstNonEmpty(data.AlbumArtist, fallback.AlbumArtist, data.Artist)
		data.Album = firstNonEmpty(data.Album, fallback.Album, "Singles")
		_, err := s.db.ExecContext(ctx, `INSERT INTO music_tracks(media_id,title,artist,album_artist,album,track_number,disc_number,year,genre,duration_seconds,codec,bitrate,sample_rate,channels,musicbrainz_track_id,musicbrainz_album_id,musicbrainz_artist_id,indexed_modified_unix,indexed_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET title=excluded.title,artist=excluded.artist,album_artist=excluded.album_artist,album=excluded.album,track_number=excluded.track_number,disc_number=excluded.disc_number,year=excluded.year,genre=excluded.genre,duration_seconds=excluded.duration_seconds,codec=excluded.codec,bitrate=excluded.bitrate,sample_rate=excluded.sample_rate,channels=excluded.channels,musicbrainz_track_id=excluded.musicbrainz_track_id,musicbrainz_album_id=excluded.musicbrainz_album_id,musicbrainz_artist_id=excluded.musicbrainz_artist_id,indexed_modified_unix=excluded.indexed_modified_unix,indexed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`,
			item.id, data.Title, data.Artist, data.AlbumArtist, data.Album, data.TrackNumber, data.DiscNumber, data.Year, data.Genre, data.DurationSeconds, data.Codec, data.Bitrate, data.SampleRate, data.Channels, data.MusicBrainzTrackID, data.MusicBrainzAlbumID, data.MusicBrainzArtistID, item.modified)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enrichMissingAlbums(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	tracks, err := s.Tracks(ctx, 0, nil, "", 5000)
	if err != nil {
		return err
	}
	type candidate struct{ key, title, artist string; year int }
	seen := map[string]bool{}
	candidates := []candidate{}
	for _, track := range tracks {
		artist := firstNonEmpty(track.AlbumArtist, track.Artist)
		album := strings.TrimSpace(track.Album)
		key := AlbumKey(artist, album)
		if artist == "" || album == "" || seen[key] {
			continue
		}
		seen[key] = true
		var enrichedAt sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT last_enriched_at FROM music_albums WHERE album_key=?`, key).Scan(&enrichedAt)
		if err == nil && enrichedAt.Valid && enrichedAt.String != "" {
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		candidates = append(candidates, candidate{key: key, title: album, artist: artist, year: track.Year})
		if len(candidates) >= limit {
			break
		}
	}
	for i, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		mbid, matchedTitle, matchedArtist, year := s.searchMusicBrainzRelease(ctx, candidate.title, candidate.artist)
		cover := ""
		if mbid != "" {
			cover = "https://coverartarchive.org/release/" + mbid + "/front-500"
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO music_albums(album_key,title,artist,year,musicbrainz_release_id,cover_url,last_enriched_at,updated_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
ON CONFLICT(album_key) DO UPDATE SET title=excluded.title,artist=excluded.artist,year=excluded.year,musicbrainz_release_id=excluded.musicbrainz_release_id,cover_url=excluded.cover_url,last_enriched_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, candidate.key, firstNonEmpty(matchedTitle, candidate.title), firstNonEmpty(matchedArtist, candidate.artist), firstPositive(year, candidate.year), mbid, cover)
		if err != nil {
			return err
		}
		if i < len(candidates)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1100 * time.Millisecond):
			}
		}
	}
	return nil
}

func (s *Service) searchMusicBrainzRelease(ctx context.Context, album, artist string) (string, string, string, int) {
	query := fmt.Sprintf(`release:"%s" AND artist:"%s"`, escapeMusicBrainzQuery(album), escapeMusicBrainzQuery(artist))
	params := url.Values{"query": {query}, "fmt": {"json"}, "limit": {"3"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://musicbrainz.org/ws/2/release/?"+params.Encode(), nil)
	if err != nil {
		return "", "", "", 0
	}
	req.Header.Set("User-Agent", "StormFlix/0.13 (https://github.com/danilostorm/stormflix)")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", "", 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0
	}
	var result struct {
		Releases []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Date   string `json:"date"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
		} `json:"releases"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil || len(result.Releases) == 0 {
		return "", "", "", 0
	}
	best := result.Releases[0]
	matchedArtist := ""
	if len(best.ArtistCredit) > 0 {
		matchedArtist = best.ArtistCredit[0].Name
	}
	return best.ID, best.Title, matchedArtist, parseYear(best.Date)
}

func (s *Service) albumCovers(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT album_key,cover_url FROM music_albums WHERE cover_url<>''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, cover string
		if err := rows.Scan(&key, &cover); err != nil {
			return nil, err
		}
		out[key] = cover
	}
	return out, rows.Err()
}

func (s *Service) favoriteIDs(ctx context.Context, profileID int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if profileID <= 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT media_id FROM music_favorites WHERE profile_id=?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *Service) recentTrackIDs(ctx context.Context, profileID int64, limit int) []int64 {
	if profileID <= 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT media_id,MAX(last_played_at) FROM music_listening_daily WHERE profile_id=? GROUP BY media_id ORDER BY MAX(last_played_at) DESC LIMIT ?`, profileID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		var last string
		if rows.Scan(&id, &last) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Service) mostPlayedIDs(ctx context.Context, limit int) []int64 {
	rows, err := s.db.QueryContext(ctx, `SELECT media_id,SUM(listened_seconds) total,COUNT(DISTINCT profile_id) listeners FROM music_listening_daily WHERE day>=date('now','-7 day') GROUP BY media_id ORDER BY listeners DESC,total DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		var total float64
		var listeners int
		if rows.Scan(&id, &total, &listeners) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func tracksForIDs(byID map[int64]Track, ids []int64) []Track {
	out := []Track{}
	for _, id := range ids {
		if track, ok := byID[id]; ok {
			out = append(out, track)
		}
	}
	return out
}

func (s *Service) trackWithoutPermissions(ctx context.Context, mediaID int64) (Track, error) {
	tracks, err := s.Tracks(ctx, 0, nil, "", 5000)
	if err != nil {
		return Track{}, err
	}
	for _, track := range tracks {
		if track.ID == mediaID {
			return track, nil
		}
	}
	return Track{}, sql.ErrNoRows
}

func AlbumKey(artist, album string) string {
	return normalizeKey(artist) + "|" + normalizeKey(album)
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

var leadingTrackNumber = regexp.MustCompile(`^\s*\d{1,3}(?:[-_. ]+|\s+)`)

func cleanTrackTitle(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, ".", " ")
	value = leadingTrackNumber.ReplaceAllString(value, "")
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func applyFallbacks(track *Track, fallbackTitle, path string) {
	if strings.TrimSpace(track.Title) == "" {
		track.Title = cleanTrackTitle(fallbackTitle)
	}
	albumDir := filepath.Base(filepath.Dir(path))
	artistDir := filepath.Base(filepath.Dir(filepath.Dir(path)))
	if strings.TrimSpace(track.Album) == "" && albumDir != "." && albumDir != string(filepath.Separator) {
		track.Album = strings.TrimSpace(albumDir)
	}
	if strings.TrimSpace(track.Artist) == "" && artistDir != "." && artistDir != string(filepath.Separator) {
		track.Artist = strings.TrimSpace(artistDir)
	}
	if strings.TrimSpace(track.AlbumArtist) == "" {
		track.AlbumArtist = track.Artist
	}
	if strings.TrimSpace(track.Album) == "" {
		track.Album = "Singles"
	}
	if strings.TrimSpace(track.Artist) == "" {
		track.Artist = "Artista desconhecido"
	}
	if strings.TrimSpace(track.AlbumArtist) == "" {
		track.AlbumArtist = track.Artist
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func parseYear(value string) int {
	value = strings.TrimSpace(value)
	if len(value) >= 4 {
		if year, err := strconv.Atoi(value[:4]); err == nil && year >= 1000 && year <= 3000 {
			return year
		}
	}
	return 0
}

func escapeMusicBrainzQuery(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), `"`, `\"`)
}
