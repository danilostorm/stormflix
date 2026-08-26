package music

import (
	"context"
	"strings"
	"time"
)

// IndexStatus is a live, queryable view of the background music organizer.
// Progress is derived from persisted catalog rows, so refreshing the Admin page
// does not lose the current counters.
type IndexStatus struct {
	Indexing          bool    `json:"indexing"`
	Phase             string  `json:"phase"`
	PhaseLabel        string  `json:"phase_label"`
	Message           string  `json:"message"`
	TotalTracks       int64   `json:"total_tracks"`
	IndexedTracks     int64   `json:"indexed_tracks"`
	PendingTracks     int64   `json:"pending_tracks"`
	FallbackTracks    int64   `json:"fallback_tracks"`
	TotalAlbums       int64   `json:"total_albums"`
	EnrichedAlbums    int64   `json:"enriched_albums"`
	AlbumsWithCover   int64   `json:"albums_with_cover"`
	PendingAlbums     int64   `json:"pending_albums"`
	Progress          float64 `json:"progress"`
	LastTrackUpdateAt string  `json:"last_track_update_at,omitempty"`
	LastAlbumUpdateAt string  `json:"last_album_update_at,omitempty"`
}

func (s *Service) Status(ctx context.Context) IndexStatus {
	out := IndexStatus{Indexing: s.IsIndexing(), Phase: "idle", PhaseLabel: "Aguardando", Message: "Nenhuma organização em andamento."}

	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.available=1 AND l.kind='music'`).Scan(&out.TotalTracks)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id JOIN music_tracks mt ON mt.media_id=m.id WHERE m.available=1 AND l.kind='music' AND COALESCE(mt.indexed_modified_unix,0)=m.modified_unix`).Scan(&out.IndexedTracks)
	if out.IndexedTracks > out.TotalTracks {
		out.IndexedTracks = out.TotalTracks
	}
	out.PendingTracks = out.TotalTracks - out.IndexedTracks

	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_tracks WHERE TRIM(COALESCE(artist,''))='' OR LOWER(TRIM(COALESCE(artist,'')))='artista desconhecido'`).Scan(&out.FallbackTracks)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(updated_at),'') FROM music_tracks`).Scan(&out.LastTrackUpdateAt)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(updated_at),'') FROM music_albums`).Scan(&out.LastAlbumUpdateAt)

	// Album counts are based on the same normalized artist+album identity used by
	// the music catalog. Singles without an artist are intentionally excluded.
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT LOWER(TRIM(COALESCE(NULLIF(album_artist,''),artist,''))) || '|' || LOWER(TRIM(COALESCE(album,'')))) FROM music_tracks WHERE TRIM(COALESCE(NULLIF(album_artist,''),artist,''))<>'' AND TRIM(COALESCE(album,''))<>''`).Scan(&out.TotalAlbums)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_albums WHERE COALESCE(last_enriched_at,'')<>''`).Scan(&out.EnrichedAlbums)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM music_albums WHERE COALESCE(cover_url,'')<>''`).Scan(&out.AlbumsWithCover)
	if out.EnrichedAlbums > out.TotalAlbums && out.TotalAlbums > 0 {
		out.EnrichedAlbums = out.TotalAlbums
	}
	out.PendingAlbums = out.TotalAlbums - out.EnrichedAlbums
	if out.PendingAlbums < 0 {
		out.PendingAlbums = 0
	}

	switch {
	case out.Indexing && out.PendingTracks > 0:
		out.Phase = "tags"
		out.PhaseLabel = "Lendo tags e informações técnicas"
		out.Message = "FFprobe está lendo título, artista, álbum, faixa, duração, codec e qualidade dos arquivos."
		if out.TotalTracks > 0 {
			out.Progress = float64(out.IndexedTracks) * 100 / float64(out.TotalTracks)
		}
	case out.Indexing:
		out.Phase = "albums"
		out.PhaseLabel = "Enriquecendo álbuns"
		out.Message = "Consultando MusicBrainz e associando capas pelo Cover Art Archive."
		if out.TotalAlbums > 0 {
			out.Progress = float64(out.EnrichedAlbums) * 100 / float64(out.TotalAlbums)
		} else {
			out.Progress = 100
		}
	case out.TotalTracks == 0:
		out.Phase = "idle"
		out.PhaseLabel = "Sem faixas"
		out.Message = "Faça primeiro o scan de uma biblioteca do tipo Música."
	case out.PendingTracks > 0:
		out.Phase = "waiting"
		out.PhaseLabel = "Organização pendente"
		out.Message = "Existem faixas novas ou alteradas aguardando organização."
		if out.TotalTracks > 0 {
			out.Progress = float64(out.IndexedTracks) * 100 / float64(out.TotalTracks)
		}
	case out.PendingAlbums > 0:
		out.Phase = "waiting_albums"
		out.PhaseLabel = "Álbuns pendentes"
		out.Message = "As tags locais estão prontas; ainda existem álbuns aguardando enriquecimento externo."
		if out.TotalAlbums > 0 {
			out.Progress = float64(out.EnrichedAlbums) * 100 / float64(out.TotalAlbums)
		}
	default:
		out.Phase = "completed"
		out.PhaseLabel = "Organização concluída"
		out.Message = "Tags e enriquecimento disponíveis foram processados."
		out.Progress = 100
	}

	// Avoid displaying SQLite's UTC-ish timestamp with excessive precision in
	// clients that only need to know whether progress is moving.
	out.LastTrackUpdateAt = cleanStatusTime(out.LastTrackUpdateAt)
	out.LastAlbumUpdateAt = cleanStatusTime(out.LastAlbumUpdateAt)
	return out
}

func cleanStatusTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}
	return value
}
