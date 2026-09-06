package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
)

var errNoCompatibleMediaVersion = errors.New("nenhuma versão compatível com este dispositivo está disponível")

// adaptItemsForClient replaces an incompatible UHD physical representative
// with the best available HD/FHD version of the same title. If every known
// version is incompatible UHD, the logical card is hidden. Unknown technical
// rows remain visible until the bounded technical indexer probes them; the
// PlaybackPlan CPU guard is still authoritative at playback time.
func (s *server) adaptItemsForClient(r *http.Request, allowedLibraryIDs []int64, items []media.Item) ([]media.Item, error) {
	caps := clientMediaCapsFromRequest(r)
	if !caps.Explicit || len(items) == 0 {
		return items, nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	candidates, err := s.media.VersionCandidates(r.Context(), ids, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}
	out, technicalPending := adaptItemsWithCandidates(caps, items, candidates)
	if technicalPending {
		s.kickTechnicalIndexer()
	}
	return out, nil
}

func adaptItemsWithCandidates(caps clientMediaCaps, items []media.Item, candidates map[int64][]media.Version) ([]media.Item, bool) {
	out := make([]media.Item, 0, len(items))
	seen := map[int64]bool{}
	technicalPending := false
	for _, item := range items {
		selected, ok, pending := selectCatalogVersion(caps, item.ID, candidates[item.ID])
		technicalPending = technicalPending || pending
		if !ok {
			continue
		}
		item.ID = selected.ID
		item.LibraryID = selected.LibraryID
		item.LibraryName = selected.LibraryName
		item.Extension = selected.Extension
		item.SizeBytes = selected.SizeBytes
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, item)
	}
	return out, technicalPending
}

func (s *server) adaptSeriesForClient(r *http.Request, allowedLibraryIDs []int64, items []media.SeriesSummary) ([]media.SeriesSummary, error) {
	caps := clientMediaCapsFromRequest(r)
	if !caps.Explicit || len(items) == 0 {
		return items, nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RepresentativeMediaID)
	}
	candidates, err := s.media.VersionCandidates(r.Context(), ids, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}
	out := make([]media.SeriesSummary, 0, len(items))
	technicalPending := false
	for _, item := range items {
		selected, ok, pending := selectCatalogVersion(caps, item.RepresentativeMediaID, candidates[item.RepresentativeMediaID])
		technicalPending = technicalPending || pending
		if !ok {
			continue
		}
		item.RepresentativeMediaID = selected.ID
		item.LibraryID = selected.LibraryID
		item.LibraryName = selected.LibraryName
		out = append(out, item)
	}
	if technicalPending {
		s.kickTechnicalIndexer()
	}
	return out, nil
}

func (s *server) adaptSeriesDetailForClient(r *http.Request, allowedLibraryIDs []int64, detail *media.SeriesDetail) error {
	if detail == nil || !clientMediaCapsFromRequest(r).Explicit {
		return nil
	}
	seasons := make([]media.SeriesSeason, 0, len(detail.Seasons))
	for _, season := range detail.Seasons {
		episodes, err := s.adaptItemsForClient(r, allowedLibraryIDs, season.Episodes)
		if err != nil {
			return err
		}
		season.Episodes = episodes
		if len(season.Episodes) > 0 {
			seasons = append(seasons, season)
		}
	}
	detail.Seasons = seasons
	detail.SeasonCount = len(seasons)
	detail.EpisodeCount = 0
	for _, season := range seasons {
		detail.EpisodeCount += len(season.Episodes)
	}
	if len(seasons) > 0 {
		detail.RepresentativeMediaID = seasons[0].Episodes[0].ID
	}
	return nil
}

func selectCatalogVersion(caps clientMediaCaps, currentID int64, candidates []media.Version) (media.Version, bool, bool) {
	if len(candidates) == 0 {
		return media.Version{}, false, true
	}
	var current *media.Version
	pending := false
	for i := range candidates {
		candidate := &candidates[i]
		known := candidate.TechnicalStatus == "ok" && candidate.Height > 0
		pending = pending || !known
		if candidate.ID == currentID {
			current = candidate
		}
		if known && catalogVersionAllowed(caps, *candidate) {
			if candidate.ID == currentID {
				return *candidate, true, pending
			}
		}
	}
	// Prefer a known FHD/HD alternate before trying another UHD encode or 1440p.
	for _, candidate := range candidates {
		if candidate.TechnicalStatus == "ok" && candidate.Height > 0 && candidate.Height <= 1080 && !playback.IsUHD(candidate.Width, candidate.Height) {
			return candidate, true, pending
		}
	}
	for _, candidate := range candidates {
		if candidate.TechnicalStatus == "ok" && candidate.Height > 0 && catalogVersionAllowed(caps, candidate) {
			return candidate, true, pending
		}
	}
	if current != nil && (current.TechnicalStatus != "ok" || current.Height <= 0) {
		if caps.Explicit && caps.MaxHeight < 2000 && versionLooksUHD(*current) {
			return media.Version{}, false, true
		}
		return *current, true, true
	}
	return media.Version{}, false, pending
}

func versionLooksUHD(version media.Version) bool {
	return playback.IsUHD(version.Width, version.Height) || version.Label == "4K" || version.Label == "8K"
}

func catalogVersionAllowed(caps clientMediaCaps, version media.Version) bool {
	if !playback.IsUHD(version.Width, version.Height) {
		return true
	}
	return clientAllows4KMedia(caps, technicalSnapshot{
		Status:     version.TechnicalStatus,
		Width:      version.Width,
		Height:     version.Height,
		VideoCodec: version.VideoCodec,
		HDR:        version.HDR,
	})
}

func (s *server) adaptHomeForClient(r *http.Request, allowedLibraryIDs []int64, feed *media.HomeFeed) error {
	caps := clientMediaCapsFromRequest(r)
	if feed == nil || !caps.Explicit {
		return nil
	}
	rows := make([]media.HomeRow, 0, len(feed.Rows))
	ids := make([]int64, 0, 1+len(feed.Rows)*12)
	if feed.Hero != nil {
		ids = append(ids, feed.Hero.ID)
	}
	for _, row := range feed.Rows {
		for _, item := range row.Items {
			ids = append(ids, item.ID)
		}
	}
	candidates, err := s.media.VersionCandidates(r.Context(), ids, allowedLibraryIDs)
	if err != nil {
		return err
	}
	technicalPending := false
	for _, row := range feed.Rows {
		if caps.MaxHeight > 0 && caps.MaxHeight < 2000 && looksLikeUHDLabel(row.Title) {
			continue
		}
		items, pending := adaptItemsWithCandidates(caps, row.Items, candidates)
		technicalPending = technicalPending || pending
		if looksLikeUHDLabel(row.Title) {
			items = keepUHDItems(items, candidates)
		}
		row.Items = items
		if len(row.Items) > 0 {
			rows = append(rows, row)
		}
	}
	feed.Rows = rows
	if feed.Hero != nil {
		items, pending := adaptItemsWithCandidates(caps, []media.Item{*feed.Hero}, candidates)
		technicalPending = technicalPending || pending
		if len(items) > 0 {
			candidate := items[0]
			feed.Hero = &candidate
		} else {
			feed.Hero = firstHomeItem(feed.Rows)
		}
	}
	if technicalPending {
		s.kickTechnicalIndexer()
	}
	return nil
}

func keepUHDItems(items []media.Item, candidates map[int64][]media.Version) []media.Item {
	uhd := map[int64]bool{}
	for _, versions := range candidates {
		for _, version := range versions {
			if versionLooksUHD(version) {
				uhd[version.ID] = true
			}
		}
	}
	out := make([]media.Item, 0, len(items))
	for _, item := range items {
		if uhd[item.ID] {
			out = append(out, item)
		}
	}
	return out
}

func firstHomeItem(rows []media.HomeRow) *media.Item {
	for _, row := range rows {
		if len(row.Items) == 0 {
			continue
		}
		item := row.Items[0]
		return &item
	}
	return nil
}

func filterVersionsForClient(caps clientMediaCaps, versions []media.Version) []media.Version {
	if !caps.Explicit {
		return versions
	}
	out := make([]media.Version, 0, len(versions))
	for _, version := range versions {
		if caps.MaxHeight < 2000 && versionLooksUHD(version) {
			continue
		}
		if version.TechnicalStatus != "ok" || version.Height <= 0 || catalogVersionAllowed(caps, version) {
			out = append(out, version)
		}
	}
	return out
}

func looksLikeUHDLabel(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "4k") || strings.Contains(value, "uhd") || strings.Contains(value, "2160")
}
