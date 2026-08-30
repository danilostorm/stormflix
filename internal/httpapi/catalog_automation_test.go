package httpapi

import (
	"testing"

	"github.com/danilostorm/stormflix/internal/playback"
)

func TestTechnicalFromSourceDetectsBrazilianDubAndSubtitle(t *testing.T) {
	dubbed := technicalFromSource(10, playback.Source{BitrateKbps: 18000, DurationSeconds: 7200, Streams: []playback.Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, HDR: "hdr10"},
		{Index: 1, Type: "audio", Codec: "eac3", Language: "eng"},
		{Index: 2, Type: "audio", Codec: "aac", Language: "por", Title: "Português Brasil"},
	}})
	if !dubbed.AudioPTBR || dubbed.DubStatus != "dublado" {
		t.Fatalf("expected dubbed pt-BR detection, got audio_pt_br=%v status=%q", dubbed.AudioPTBR, dubbed.DubStatus)
	}
	if dubbed.Height != 2160 || dubbed.HDR != "hdr10" || dubbed.VideoCodec != "hevc" {
		t.Fatalf("unexpected video technical snapshot: %#v", dubbed)
	}

	subbed := technicalFromSource(11, playback.Source{Streams: []playback.Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac", Language: "jpn"},
		{Index: 2, Type: "subtitle", Codec: "subrip", Language: "pt-BR", Title: "Português"},
	}})
	if subbed.AudioPTBR || !subbed.SubtitlePTBR || subbed.DubStatus != "legendado" {
		t.Fatalf("expected subtitled pt-BR detection, got audio=%v subtitle=%v status=%q", subbed.AudioPTBR, subbed.SubtitlePTBR, subbed.DubStatus)
	}
}

func TestDuplicateCatalogGroupsKeepsEpisodesDistinct(t *testing.T) {
	items := []catalogHealthItem{
		{ID: 1, Title: "Série", Available: true, TMDBID: 99, MediaType: "series", SeasonNumber: 1, EpisodeNumber: 1},
		{ID: 2, Title: "Série", Available: true, TMDBID: 99, MediaType: "series", SeasonNumber: 1, EpisodeNumber: 1},
		{ID: 3, Title: "Série", Available: true, TMDBID: 99, MediaType: "series", SeasonNumber: 1, EpisodeNumber: 2},
	}
	groups := duplicateCatalogGroups(items)
	if len(groups) != 1 {
		t.Fatalf("expected only the duplicated physical copy group, got %d groups: %#v", len(groups), groups)
	}
	if got := len(groups[0].Copies); got != 2 {
		t.Fatalf("expected 2 copies of S01E01, got %d", got)
	}
	if groups[0].Copies[0].EpisodeNumber != 1 || groups[0].Copies[1].EpisodeNumber != 1 {
		t.Fatalf("different episodes were collapsed as duplicates: %#v", groups[0].Copies)
	}
}

func TestCategoryRuleNormalization(t *testing.T) {
	rules := normalizeRules(categoryRules{
		Genres:     []string{"Ação", "acao", "Ficção Científica"},
		MediaTypes: []string{"Movie", "ANIME"},
		MinRating:  12,
		RecentDays: -2,
	})
	if rules.MinRating != 10 || rules.RecentDays != 0 {
		t.Fatalf("unexpected numeric normalization: %#v", rules)
	}
	if len(rules.Genres) != 2 || rules.Genres[0] != "acao" || rules.Genres[1] != "ficcao-cientifica" {
		t.Fatalf("unexpected genre normalization: %#v", rules.Genres)
	}
	if len(rules.MediaTypes) != 2 || rules.MediaTypes[0] != "movie" || rules.MediaTypes[1] != "anime" {
		t.Fatalf("unexpected media type normalization: %#v", rules.MediaTypes)
	}
}
