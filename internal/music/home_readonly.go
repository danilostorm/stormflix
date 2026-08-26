package music

import (
	"context"
	"sort"
	"strings"
)

// HomeReadOnly builds the music home without starting maintenance work.
// Indexing/enrichment is intentionally triggered from the Admin action instead
// of every time a phone, TV or browser opens the Music screen.
func (s *Service) HomeReadOnly(ctx context.Context, profileID int64, allowedLibraryIDs []int64) (Home, error) {
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
	if len(recentTracks) > 30 { recentTracks = recentTracks[:30] }
	out.RecentlyAddedTracks = recentTracks

	albumMap := map[string]*Album{}
	artistAlbums := map[string]map[string]bool{}
	artistTracks := map[string]int{}
	for _, track := range tracks {
		key := AlbumKey(firstNonEmpty(track.AlbumArtist, track.Artist), track.Album)
		if key == "|" { key = AlbumKey(track.Artist, "Singles") }
		album := albumMap[key]
		if album == nil {
			album = &Album{Key:key, Title:firstNonEmpty(track.Album,"Singles"), Artist:firstNonEmpty(track.AlbumArtist,track.Artist,"Artista desconhecido"), Year:track.Year, CoverURL:track.CoverURL, RepresentativeTrackID:track.ID, ModifiedUnix:track.ModifiedUnix}
			albumMap[key] = album
		}
		album.TrackCount++
		album.DurationSeconds += track.DurationSeconds
		if album.Year == 0 || (track.Year > 0 && track.Year < album.Year) { album.Year = track.Year }
		if track.ModifiedUnix > album.ModifiedUnix { album.ModifiedUnix = track.ModifiedUnix; album.RepresentativeTrackID = track.ID }
		if album.CoverURL == "" && track.CoverURL != "" { album.CoverURL = track.CoverURL }
		artist := firstNonEmpty(track.AlbumArtist, track.Artist, "Artista desconhecido")
		artistTracks[artist]++
		if artistAlbums[artist] == nil { artistAlbums[artist] = map[string]bool{} }
		artistAlbums[artist][key] = true
	}
	for _, album := range albumMap { out.RecentlyAddedAlbums = append(out.RecentlyAddedAlbums, *album) }
	sort.SliceStable(out.RecentlyAddedAlbums, func(i,j int) bool { return out.RecentlyAddedAlbums[i].ModifiedUnix > out.RecentlyAddedAlbums[j].ModifiedUnix })
	if len(out.RecentlyAddedAlbums) > 30 { out.RecentlyAddedAlbums = out.RecentlyAddedAlbums[:30] }
	for name,count := range artistTracks { out.Artists = append(out.Artists, Artist{Name:name,TrackCount:count,AlbumCount:len(artistAlbums[name])}) }
	sort.SliceStable(out.Artists, func(i,j int) bool { if out.Artists[i].TrackCount == out.Artists[j].TrackCount { return strings.ToLower(out.Artists[i].Name) < strings.ToLower(out.Artists[j].Name) }; return out.Artists[i].TrackCount > out.Artists[j].TrackCount })
	if len(out.Artists) > 30 { out.Artists = out.Artists[:30] }

	byID := make(map[int64]Track,len(tracks)); for _,track := range tracks { byID[track.ID] = track }
	out.RecentlyPlayed = tracksForIDs(byID, s.recentTrackIDs(ctx, profileID, 24))
	out.MostPlayed = tracksForIDs(byID, s.mostPlayedIDs(ctx, 24))
	return out,nil
}
