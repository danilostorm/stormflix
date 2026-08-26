package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/music"
	"github.com/danilostorm/stormflix/internal/webcompat"
)

// This file intentionally keeps Jellyfin compatibility isolated from /api/v1.
// It implements the TV-oriented subset needed for discovery, authentication,
// libraries, movies/series/seasons/episodes/music, images, resume state,
// PlaybackInfo, subtitles and direct streaming. It is not a fork of Jellyfin.

type jellyfinLibrary struct {
	ID   int64
	Name string
	Kind string
}

func (s *server) jellyfinServerID() string {
	sum := sha256.Sum256([]byte(s.config.DatabasePath() + "|" + s.config.ServerName))
	return hex.EncodeToString(sum[:16])
}

func (s *server) jellyfinPublicInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jellyfinInfo(false))
}

func (s *server) jellyfinSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jellyfinInfo(true))
}

func (s *server) jellyfinInfo(full bool) map[string]any {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	out := map[string]any{
		"LocalAddress":           "",
		"ServerName":             name,
		"Version":                version,
		"ProductName":            "StormFlix Jellyfin Compatibility",
		"OperatingSystem":        "StormFlix",
		"Id":                     s.jellyfinServerID(),
		"StartupWizardCompleted": true,
	}
	if full {
		out["WanAddress"] = ""
		out["WebSocketPortNumber"] = 0
		out["SupportsHttps"] = true
	}
	return out
}

func jellyfinToken(r *http.Request) string {
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	for _, key := range []string{"api_key", "ApiKey", "token"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	for _, header := range []string{"Authorization", "X-Emby-Authorization"} {
		value := r.Header.Get(header)
		lower := strings.ToLower(value)
		for _, marker := range []string{"token=\"", "token=", "api_key=\"", "api_key="} {
			idx := strings.Index(lower, marker)
			if idx < 0 {
				continue
			}
			start := idx + len(marker)
			end := len(value)
			if strings.HasSuffix(marker, "\"") {
				if p := strings.Index(value[start:], "\""); p >= 0 {
					end = start + p
				}
			} else if p := strings.IndexAny(value[start:], ", "); p >= 0 {
				end = start + p
			}
			if start < end {
				return strings.TrimSpace(value[start:end])
			}
		}
	}
	return ""
}

func (s *server) jellyfinRequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := jellyfinToken(r)
		u, err := s.auth.CurrentUser(r.Context(), token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ResponseStatus": map[string]any{"ErrorCode": "Unauthorized", "Message": "Authentication required"}})
			return
		}
		if roleLevel(u.Role) < 2 && len(u.LibraryIDs) == 0 {
			u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

func (s *server) jellyfinPublicUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *server) jellyfinAuthenticate(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid login body"))
		return
	}
	username := firstJellyfinString(raw, "Username", "username")
	password := firstJellyfinString(raw, "Pw", "Password", "pw", "password")
	u, token, err := s.auth.Login(r.Context(), username, password, shortDevice(r.UserAgent()), clientIP(r))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ResponseStatus": map[string]any{"ErrorCode": "InvalidUser", "Message": "Invalid username or password"}})
		return
	}
	if roleLevel(u.Role) < 2 && len(u.LibraryIDs) == 0 {
		u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
	}
	_ = s.jellyfinDefaultProfileID(r.Context(), u)
	writeJSON(w, http.StatusOK, map[string]any{
		"User": s.jellyfinUserObject(u),
		"SessionInfo": map[string]any{
			"UserId":     strconv.FormatInt(u.ID, 10),
			"UserName":   u.Username,
			"Client":     "StormFlix Jellyfin Compatibility",
			"DeviceName": shortDevice(r.UserAgent()),
		},
		"AccessToken": token,
		"ServerId":    s.jellyfinServerID(),
	})
}

func firstJellyfinString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(v)); text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func (s *server) jellyfinLogout(w http.ResponseWriter, r *http.Request) {
	_ = s.auth.Logout(r.Context(), jellyfinToken(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) jellyfinCurrentUser(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if requested := strings.TrimSpace(r.PathValue("id")); requested != "" && requested != strconv.FormatInt(u.ID, 10) {
		writeError(w, http.StatusForbidden, errors.New("user mismatch"))
		return
	}
	writeJSON(w, http.StatusOK, s.jellyfinUserObject(u))
}

func (s *server) jellyfinUserObject(u auth.User) map[string]any {
	return map[string]any{
		"Name":                  u.Username,
		"ServerId":              s.jellyfinServerID(),
		"Id":                    strconv.FormatInt(u.ID, 10),
		"HasPassword":           true,
		"HasConfiguredPassword": true,
		"EnableAutoLogin":       false,
		"Configuration": map[string]any{
			"AudioLanguagePreference":    "por",
			"SubtitleLanguagePreference": "por",
			"PlayDefaultAudioTrack":      true,
			"RememberAudioSelections":    true,
			"RememberSubtitleSelections": true,
		},
		"Policy": map[string]any{
			"IsAdministrator":                u.Role == "admin",
			"IsDisabled":                     !u.Active,
			"EnableAllFolders":               true,
			"EnableMediaPlayback":            true,
			"EnableAudioPlaybackTranscoding": false,
			"EnableVideoPlaybackTranscoding": false,
			"EnableContentDownloading":       false,
		},
	}
}

func (s *server) jellyfinDefaultProfileID(ctx context.Context, u auth.User) int64 {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM profiles WHERE user_id=? AND active=1 ORDER BY id LIMIT 1`, u.ID).Scan(&id); err == nil && id > 0 {
		return id
	}
	p, err := s.auth.CreateProfile(ctx, u.ID, u.DisplayName, "", "", false)
	if err == nil {
		return p.ID
	}
	return 0
}

func (s *server) jellyfinLibraries(ctx context.Context, u auth.User) ([]jellyfinLibrary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,kind FROM libraries WHERE enabled=1 ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []jellyfinLibrary{}
	for rows.Next() {
		var lib jellyfinLibrary
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Kind); err != nil {
			return nil, err
		}
		if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, lib.ID) {
			continue
		}
		out = append(out, lib)
	}
	return out, rows.Err()
}

func (s *server) jellyfinViews(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	libs, err := s.jellyfinLibraries(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := []any{}
	for _, lib := range libs {
		items = append(items, map[string]any{
			"Name":           lib.Name,
			"ServerId":       s.jellyfinServerID(),
			"Id":             jfLibraryID(lib.ID),
			"Type":           "CollectionFolder",
			"CollectionType": jfCollectionType(lib.Kind),
			"IsFolder":       true,
			"ChildCount":     0,
			"DateCreated":    time.Now().UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

func jfCollectionType(kind string) string {
	switch strings.ToLower(kind) {
	case "series", "anime":
		return "tvshows"
	case "music":
		return "music"
	default:
		return "movies"
	}
}

func jfLibraryID(id int64) string { return "lib" + strconv.FormatInt(id, 10) }
func jfMediaID(id int64) string   { return "m" + strconv.FormatInt(id, 10) }

func jfParsePrefixedID(value, prefix string) (int64, bool) {
	if !strings.HasPrefix(value, prefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	return id, err == nil && id > 0
}

func jfSeriesID(id string) string {
	return "series-" + base64.RawURLEncoding.EncodeToString([]byte(id))
}

func jfParseSeriesID(value string) (string, bool) {
	if !strings.HasPrefix(value, "series-") {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "series-"))
	return string(b), err == nil
}

func jfSeasonID(seriesID string, number int) string {
	return "season-" + base64.RawURLEncoding.EncodeToString([]byte(seriesID)) + "-" + strconv.Itoa(number)
}

func jfParseSeasonID(value string) (string, int, bool) {
	if !strings.HasPrefix(value, "season-") {
		return "", 0, false
	}
	raw := strings.TrimPrefix(value, "season-")
	cut := strings.LastIndex(raw, "-")
	if cut < 1 {
		return "", 0, false
	}
	b, err := base64.RawURLEncoding.DecodeString(raw[:cut])
	number, nerr := strconv.Atoi(raw[cut+1:])
	return string(b), number, err == nil && nerr == nil
}

func (s *server) jellyfinItems(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	parent := strings.TrimSpace(r.URL.Query().Get("ParentId"))
	search := strings.TrimSpace(r.URL.Query().Get("SearchTerm"))
	include := strings.ToLower(r.URL.Query().Get("IncludeItemTypes"))
	items := []any{}

	if libID, ok := jfParsePrefixedID(parent, "lib"); ok {
		var kind string
		if err := s.db.QueryRowContext(r.Context(), `SELECT kind FROM libraries WHERE id=? AND enabled=1`, libID).Scan(&kind); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": 0, "StartIndex": 0})
			return
		}
		if strings.EqualFold(kind, "music") {
			tracks, err := s.music.Tracks(r.Context(), profileID, []int64{libID}, search, 5000)
			if err == nil {
				for _, track := range tracks {
					items = append(items, s.jellyfinAudioItem(profileID, track))
				}
			}
		} else if strings.EqualFold(kind, "series") || strings.EqualFold(kind, "anime") {
			series, err := s.media.SeriesList(r.Context(), []int64{libID}, search)
			if err == nil {
				for _, show := range series {
					items = append(items, s.jellyfinSeriesItem(show))
				}
			}
		} else {
			mediaItems, err := s.media.List(r.Context(), libID, search, 500, 0, u.LibraryIDs)
			if err == nil {
				for _, item := range mediaItems {
					items = append(items, s.jellyfinMediaItem(profileID, item))
				}
			}
		}
	} else if seriesID, ok := jfParseSeriesID(parent); ok {
		detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs)
		if err == nil {
			for _, season := range detail.Seasons {
				items = append(items, s.jellyfinSeasonItem(detail, season))
			}
		}
	} else if seriesID, seasonNumber, ok := jfParseSeasonID(parent); ok {
		detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs)
		if err == nil {
			for _, season := range detail.Seasons {
				if season.Number == seasonNumber {
					for _, episode := range season.Episodes {
						items = append(items, s.jellyfinMediaItem(profileID, episode))
					}
				}
			}
		}
	} else if strings.Contains(include, "series") {
		series, err := s.media.SeriesList(r.Context(), u.LibraryIDs, search)
		if err == nil {
			for _, show := range series {
				items = append(items, s.jellyfinSeriesItem(show))
			}
		}
	} else if strings.Contains(include, "audio") {
		tracks, err := s.music.Tracks(r.Context(), profileID, u.LibraryIDs, search, 5000)
		if err == nil {
			for _, track := range tracks {
				items = append(items, s.jellyfinAudioItem(profileID, track))
			}
		}
	} else {
		mediaItems, err := s.media.List(r.Context(), 0, search, 500, 0, u.LibraryIDs)
		if err == nil {
			for _, item := range mediaItems {
				items = append(items, s.jellyfinMediaItem(profileID, item))
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

func (s *server) jellyfinResume(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	list, err := s.media.ContinueWatching(r.Context(), profileID, u.LibraryIDs, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := []any{}
	for _, item := range list {
		items = append(items, s.jellyfinMediaItem(profileID, item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

func (s *server) jellyfinItem(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	id := r.PathValue("id")
	if mediaID, ok := jfParsePrefixedID(id, "m"); ok {
		detail, err := s.media.Detail(r.Context(), mediaID, u.LibraryIDs)
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("item not found"))
			return
		}
		writeJSON(w, http.StatusOK, s.jellyfinMediaItem(profileID, detail.Item))
		return
	}
	if seriesID, ok := jfParseSeriesID(id); ok {
		detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs)
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("series not found"))
			return
		}
		writeJSON(w, http.StatusOK, s.jellyfinSeriesItem(detail.SeriesSummary))
		return
	}
	if seriesID, number, ok := jfParseSeasonID(id); ok {
		detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs)
		if err == nil {
			for _, season := range detail.Seasons {
				if season.Number == number {
					writeJSON(w, http.StatusOK, s.jellyfinSeasonItem(detail, season))
					return
				}
			}
		}
	}
	writeError(w, http.StatusNotFound, errors.New("item not found"))
}

func (s *server) jellyfinMediaItem(profileID int64, item media.Item) map[string]any {
	kind := "Movie"
	if strings.EqualFold(item.MediaType, "episode") || item.EpisodeNumber > 0 {
		kind = "Episode"
	}
	userData := s.jellyfinUserData(profileID, item.ID, item.PositionSeconds, item.DurationSeconds, item.ProgressPercent)
	out := map[string]any{
		"Name":              item.Title,
		"ServerId":          s.jellyfinServerID(),
		"Id":                jfMediaID(item.ID),
		"Type":              kind,
		"MediaType":         "Video",
		"IsFolder":          false,
		"CanDownload":       false,
		"Container":         strings.TrimPrefix(strings.ToLower(item.Extension), "."),
		"ProductionYear":    item.Year,
		"Overview":          item.Overview,
		"OfficialRating":    "",
		"RunTimeTicks":      int64(item.RuntimeMinutes) * 60 * 10000000,
		"UserData":          userData,
		"IndexNumber":       item.EpisodeNumber,
		"ParentIndexNumber": item.SeasonNumber,
		"CommunityRating":   item.Rating,
		"Genres":            item.Genres,
	}
	if item.PosterURL != "" {
		out["ImageTags"] = map[string]string{"Primary": "sf"}
	}
	if item.BackdropURL != "" {
		out["BackdropImageTags"] = []string{"sf"}
	}
	return out
}

func (s *server) jellyfinSeriesItem(show media.SeriesSummary) map[string]any {
	out := map[string]any{
		"Name":               show.Title,
		"ServerId":           s.jellyfinServerID(),
		"Id":                 jfSeriesID(show.ID),
		"Type":               "Series",
		"MediaType":          "Video",
		"IsFolder":           true,
		"ProductionYear":     show.Year,
		"Overview":           show.Overview,
		"CommunityRating":    show.Rating,
		"Genres":             show.Genres,
		"ChildCount":         show.SeasonCount,
		"RecursiveItemCount": show.EpisodeCount,
	}
	if show.PosterURL != "" {
		out["ImageTags"] = map[string]string{"Primary": "sf"}
	}
	if show.BackdropURL != "" {
		out["BackdropImageTags"] = []string{"sf"}
	}
	return out
}

func (s *server) jellyfinSeasonItem(show media.SeriesDetail, season media.SeriesSeason) map[string]any {
	out := map[string]any{
		"Name":       season.Title,
		"ServerId":   s.jellyfinServerID(),
		"Id":         jfSeasonID(show.ID, season.Number),
		"Type":       "Season",
		"MediaType":  "Video",
		"IsFolder":   true,
		"IndexNumber": season.Number,
		"SeriesId":   jfSeriesID(show.ID),
		"SeriesName": show.Title,
		"ChildCount": len(season.Episodes),
	}
	if show.PosterURL != "" {
		out["ImageTags"] = map[string]string{"Primary": "sf"}
	}
	return out
}

func (s *server) jellyfinAudioItem(profileID int64, track music.Track) map[string]any {
	out := map[string]any{
		"Name":              track.Title,
		"ServerId":          s.jellyfinServerID(),
		"Id":                jfMediaID(track.ID),
		"Type":              "Audio",
		"MediaType":         "Audio",
		"IsFolder":          false,
		"Container":         strings.TrimPrefix(strings.ToLower(track.Extension), "."),
		"ProductionYear":    track.Year,
		"RunTimeTicks":      int64(track.DurationSeconds * 10000000),
		"Artists":           []string{track.Artist},
		"AlbumArtist":       track.AlbumArtist,
		"Album":             track.Album,
		"IndexNumber":       track.TrackNumber,
		"ParentIndexNumber": track.DiscNumber,
		"Genres":            []string{track.Genre},
		"UserData":          s.jellyfinUserData(profileID, track.ID, 0, track.DurationSeconds, 0),
	}
	if track.CoverURL != "" {
		out["ImageTags"] = map[string]string{"Primary": "sf"}
	}
	return out
}

func (s *server) jellyfinUserData(profileID, mediaID int64, fallbackPosition, fallbackDuration, fallbackPercent float64) map[string]any {
	position, duration, completed := fallbackPosition, fallbackDuration, false
	if profileID > 0 {
		_ = s.db.QueryRowContext(context.Background(), `SELECT position_seconds,duration_seconds,completed FROM profile_progress WHERE profile_id=? AND media_id=?`, profileID, mediaID).Scan(&position, &duration, &completed)
	}
	percent := fallbackPercent
	if duration > 0 {
		percent = position / duration * 100
	}
	return map[string]any{
		"PlaybackPositionTicks": int64(position * 10000000),
		"PlayCount":             0,
		"IsFavorite":            false,
		"Played":                completed,
		"PlayedPercentage":      percent,
	}
}

func (s *server) jellyfinSeasons(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	seriesID, ok := jfParseSeriesID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid series id"))
		return
	}
	detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	items := []any{}
	for _, season := range detail.Seasons {
		items = append(items, s.jellyfinSeasonItem(detail, season))
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

func (s *server) jellyfinEpisodes(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	seriesID, ok := jfParseSeriesID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid series id"))
		return
	}
	detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	wanted := strings.TrimSpace(r.URL.Query().Get("SeasonId"))
	items := []any{}
	for _, season := range detail.Seasons {
		if wanted != "" && wanted != jfSeasonID(detail.ID, season.Number) {
			continue
		}
		for _, episode := range season.Episodes {
			items = append(items, s.jellyfinMediaItem(profileID, episode))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
}

func (s *server) jellyfinImage(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id := r.PathValue("id")
	kind := strings.ToLower(r.PathValue("kind"))
	artKind := "poster"
	if strings.Contains(kind, "backdrop") {
		artKind = "backdrop"
	}
	url := ""
	if mediaID, ok := jfParsePrefixedID(id, "m"); ok {
		item, err := s.media.GetStreamItem(r.Context(), mediaID)
		if err == nil && (roleLevel(u.Role) >= 2 || media.ContainsLibrary(u.LibraryIDs, item.LibraryID)) {
			_ = s.db.QueryRowContext(r.Context(), `SELECT public_url FROM media_artwork WHERE media_id=? AND kind=? AND selected=1 ORDER BY score DESC LIMIT 1`, mediaID, artKind).Scan(&url)
		}
	} else if seriesID, ok := jfParseSeriesID(id); ok {
		if detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs); err == nil {
			if artKind == "backdrop" {
				url = detail.BackdropURL
			} else {
				url = detail.PosterURL
			}
		}
	} else if seriesID, _, ok := jfParseSeasonID(id); ok {
		if detail, err := s.media.SeriesDetail(r.Context(), seriesID, u.LibraryIDs); err == nil {
			if artKind == "backdrop" {
				url = detail.BackdropURL
			} else {
				url = detail.PosterURL
			}
		}
	}
	if url == "" {
		writeError(w, http.StatusNotFound, errors.New("image not found"))
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *server) jellyfinPlaybackInfo(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	mediaID, ok := jfParsePrefixedID(r.PathValue("id"), "m")
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid media id"))
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), mediaID)
	if err != nil || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}

	streams := []any{}
	if probed, probeErr := webcompat.DetailedStreams(r.Context(), item.Path); probeErr == nil {
		for _, stream := range probed {
			streamType := strings.Title(strings.ToLower(stream.CodecType))
			entry := map[string]any{
				"Codec":      stream.CodecName,
				"Type":       streamType,
				"Index":      stream.Index,
				"IsDefault":  stream.Index == 0,
				"IsExternal": false,
				"Language":   stream.Tags["language"],
				"Title":      stream.Tags["title"],
			}
			if stream.Width > 0 {
				entry["Width"] = stream.Width
			}
			if stream.Height > 0 {
				entry["Height"] = stream.Height
			}
			if stream.Channels > 0 {
				entry["Channels"] = stream.Channels
			}
			if stream.SampleRate != "" {
				if rate, e := strconv.Atoi(stream.SampleRate); e == nil {
					entry["SampleRate"] = rate
				}
			}
			streams = append(streams, entry)
		}
	}

	rows, _ := s.db.QueryContext(r.Context(), `SELECT id,language,hearing_impaired FROM subtitles WHERE media_id=? ORDER BY language,id`, mediaID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var subtitleID int64
			var language string
			var hearing bool
			if rows.Scan(&subtitleID, &language, &hearing) == nil {
				streams = append(streams, map[string]any{
					"Codec":                  "vtt",
					"Type":                   "Subtitle",
					"Index":                  1000 + subtitleID,
					"IsExternal":             true,
					"IsTextSubtitleStream":   true,
					"SupportsExternalStream": true,
					"Language":               language,
					"IsHearingImpaired":      hearing,
					"DeliveryMethod":         "External",
					"DeliveryUrl":            fmt.Sprintf("/jellyfin-api/Videos/%s/%d/Subtitles/0/Stream.vtt", jfMediaID(mediaID), subtitleID),
				})
			}
		}
	}

	container := strings.TrimPrefix(strings.ToLower(item.Extension), ".")
	source := map[string]any{
		"Protocol":             "Http",
		"Id":                   jfMediaID(mediaID),
		"Path":                 fmt.Sprintf("/jellyfin-api/Videos/%s/stream", jfMediaID(mediaID)),
		"Type":                 "Default",
		"Container":            container,
		"Size":                 item.SizeBytes,
		"Name":                 "Original",
		"IsRemote":             false,
		"SupportsDirectPlay":   true,
		"SupportsDirectStream": true,
		"SupportsTranscoding":  false,
		"MediaStreams":         streams,
		"DirectStreamUrl":      fmt.Sprintf("/jellyfin-api/Videos/%s/stream?static=true", jfMediaID(mediaID)),
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"MediaSources":  []any{source},
		"PlaySessionId": fmt.Sprintf("sf-%d-%d", u.ID, time.Now().UnixNano()),
	})
}

func (s *server) jellyfinStream(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	mediaID, ok := jfParsePrefixedID(r.PathValue("id"), "m")
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid media id"))
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), mediaID)
	if err != nil || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	file, err := os.Open(item.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("media file unavailable"))
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", mediaContentType(item.Extension))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=0")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}

func (s *server) jellyfinSubtitle(w http.ResponseWriter, r *http.Request) {
	mediaID, ok := jfParsePrefixedID(r.PathValue("id"), "m")
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid media id"))
		return
	}
	r.SetPathValue("id", strconv.FormatInt(mediaID, 10))
	r.SetPathValue("subtitle_id", r.PathValue("subtitle_id"))
	s.subtitleVTT(w, r)
}

func (s *server) jellyfinProgress(w http.ResponseWriter, r *http.Request) {
	s.jellyfinSaveProgress(w, r, false)
}

func (s *server) jellyfinStopped(w http.ResponseWriter, r *http.Request) {
	s.jellyfinSaveProgress(w, r, true)
}

func (s *server) jellyfinSaveProgress(w http.ResponseWriter, r *http.Request, stopped bool) {
	u := currentUser(r)
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid progress body"))
		return
	}
	itemID := firstJellyfinString(raw, "ItemId", "itemId")
	mediaID, ok := jfParsePrefixedID(itemID, "m")
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	if profileID <= 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	position := jfMapFloat(raw, "PositionTicks", "positionTicks") / 10000000.0
	duration := jfMapFloat(raw, "RunTimeTicks", "runtimeTicks") / 10000000.0
	if duration <= 0 {
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(NULLIF(pp.duration_seconds,0),COALESCE(mm.runtime_minutes,0)*60,0) FROM media m LEFT JOIN profile_progress pp ON pp.media_id=m.id AND pp.profile_id=? LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.id=?`, profileID, mediaID).Scan(&duration)
	}
	if stopped && duration > 0 && position > duration {
		position = duration
	}
	_ = s.admin.SaveProfileProgress(r.Context(), profileID, mediaID, position, duration)
	w.WriteHeader(http.StatusNoContent)
}

func jfMapFloat(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch n := v.(type) {
			case float64:
				return n
			case json.Number:
				f, _ := n.Float64()
				return f
			case string:
				f, _ := strconv.ParseFloat(n, 64)
				return f
			}
		}
	}
	return 0
}
