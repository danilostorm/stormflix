package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var scannerSeriesIdentity sync.Map

var (
	scannerSeasonDirRE   = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:season|temporada)[ ._-]*(\d{1,3})(?:$|[ ._-])`)
	scannerLeadingSeasonRE = regexp.MustCompile(`(?i)^(\d{1,3})[ºª°]?[ ._-]*(?:season|temporada)(?:$|[ ._-])`)
)

type scannerIdentity struct {
	SourceRoot  string
	SeriesKey   string
	SeriesTitle string
	Season      int
	Episode     int
	Absolute    int
}

type scannerIdentityRow struct {
	MediaID int64
	Path    string
	scannerIdentity
}

// RebuildSeriesIdentities is the scanner stage for episodic libraries. It uses
// the configured library roots and directory hierarchy as authoritative input;
// external providers are deliberately not involved here. A previously chosen
// manual series match may replace only the display/search title, never the
// scanner-owned series key, season or episode identity.
func (s *Service) RebuildSeriesIdentities(ctx context.Context, libraryID int64) error {
	var kind, fallbackRoot string
	if err := s.db.QueryRowContext(ctx, `SELECT kind,path FROM libraries WHERE id=?`, libraryID).Scan(&kind, &fallbackRoot); err != nil {
		return err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if !isSeriesLibraryKind(kind) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM media_series_identity WHERE library_id=?`, libraryID)
		return nil
	}

	roots := []string{}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM library_sources WHERE library_id=? AND enabled=1 ORDER BY sort_order,id`, libraryID)
	if err == nil {
		for rows.Next() {
			var root string
			if rows.Scan(&root) == nil && strings.TrimSpace(root) != "" {
				roots = append(roots, filepath.Clean(root))
			}
		}
		_ = rows.Close()
	}
	if len(roots) == 0 && strings.TrimSpace(fallbackRoot) != "" {
		roots = append(roots, filepath.Clean(fallbackRoot))
	}
	// Prefer the longest root if multiple mounts could match a path.
	sort.SliceStable(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })

	mediaRows, err := s.db.QueryContext(ctx, `SELECT id,path FROM media WHERE library_id=? AND available=1 ORDER BY path`, libraryID)
	if err != nil {
		return err
	}
	items := []scannerIdentityRow{}
	for mediaRows.Next() {
		var id int64
		var path string
		if err := mediaRows.Scan(&id, &path); err != nil {
			_ = mediaRows.Close()
			return err
		}
		root := matchingSourceRoot(path, roots)
		identity := deriveScannerIdentity(path, root, kind)
		items = append(items, scannerIdentityRow{MediaID: id, Path: path, scannerIdentity: identity})
	}
	if err := mediaRows.Close(); err != nil {
		return err
	}

	// Plex-like series matching: a manual choice belongs to the logical show,
	// not one episode. Reuse its canonical provider title after every rescan so
	// newly discovered episodes search for the already-approved show.
	manualTitles := map[string]string{}
	overrideRows, overrideErr := s.db.QueryContext(ctx, `SELECT series_key,title FROM series_metadata_overrides WHERE library_id=? AND manual=1`, libraryID)
	if overrideErr == nil {
		for overrideRows.Next() {
			var key, title string
			if overrideRows.Scan(&key, &title) == nil && strings.TrimSpace(key) != "" && strings.TrimSpace(title) != "" {
				manualTitles[key] = strings.TrimSpace(title)
			}
		}
		_ = overrideRows.Close()
	}
	for i := range items {
		if title := manualTitles[items[i].SeriesKey]; title != "" {
			items[i].SeriesTitle = title
		}
	}

	// Files without explicit numbering still need deterministic episode order.
	bySeriesSeason := map[string][]int{}
	for i := range items {
		if items[i].Episode > 0 {
			continue
		}
		key := items[i].SeriesKey + ":" + strconv.Itoa(maxScannerInt(items[i].Season, 1))
		bySeriesSeason[key] = append(bySeriesSeason[key], i)
	}
	for _, indexes := range bySeriesSeason {
		for order, index := range indexes {
			items[index].Season = maxScannerInt(items[index].Season, 1)
			items[index].Episode = order + 1
			if items[index].Absolute == 0 {
				items[index].Absolute = order + 1
			}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_series_identity WHERE library_id=?`, libraryID); err != nil {
		return err
	}
	for _, item := range items {
		if item.SeriesTitle == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_series_identity(media_id,library_id,source_root,series_key,series_title,season_number,episode_number,absolute_number,updated_at) VALUES(?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
			item.MediaID, libraryID, item.SourceRoot, item.SeriesKey, item.SeriesTitle, item.Season, item.Episode, item.Absolute); err != nil {
			return err
		}
		scannerSeriesIdentity.Store(filepath.Clean(item.Path), item.scannerIdentity)
	}
	return tx.Commit()
}

func matchingSourceRoot(path string, roots []string) string {
	clean := filepath.Clean(path)
	for _, root := range roots {
		rel, err := filepath.Rel(root, clean)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return root
	}
	return ""
}

func deriveScannerIdentity(path, root, kind string) scannerIdentity {
	cleanPath := filepath.Clean(path)
	seriesTitle := ""
	seriesRoot := ""
	if root != "" {
		rel, err := filepath.Rel(root, cleanPath)
		if err == nil {
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) > 1 {
				first := cleanSeriesFolderName(parts[0])
				if first != "" && !isGenericDirectory(first) {
					seriesTitle = first
					seriesRoot = filepath.Join(root, parts[0])
				}
			}
		}
		if seriesTitle == "" {
			base := cleanSeriesFolderName(filepath.Base(root))
			if base != "" && !isGenericDirectory(base) {
				seriesTitle = base
				seriesRoot = root
			}
		}
	}

	parsed := ParseFilename(cleanPath, kind)
	if seriesTitle == "" {
		seriesTitle = parsed.Title
		seriesRoot = filepath.Dir(cleanPath)
	}
	season := scannerSeasonFromPath(cleanPath)
	if season == 0 {
		season = parsed.Season
	}
	episode := parsed.Episode
	if episode > 0 && season == 0 {
		season = 1
	}
	absolute := episode
	key := normalizeTitle(seriesTitle)
	if key == "" {
		key = normalizeTitle(seriesRoot)
	}
	return scannerIdentity{SourceRoot: root, SeriesKey: key, SeriesTitle: seriesTitle, Season: season, Episode: episode, Absolute: absolute}
}

func scannerSeasonFromPath(path string) int {
	dir := filepath.Dir(path)
	for depth := 0; depth < 8; depth++ {
		name := cleanMetadataText(filepath.Base(dir))
		if match := scannerSeasonDirRE.FindStringSubmatch(name); len(match) == 2 {
			n, _ := strconv.Atoi(match[1])
			return n
		}
		if match := scannerLeadingSeasonRE.FindStringSubmatch(name); len(match) == 2 {
			n, _ := strconv.Atoi(match[1])
			return n
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return 0
}

func cleanSeriesFolderName(value string) string {
	value = cleanMetadataText(value)
	value = yearRE.ReplaceAllString(value, " ")
	return compactTitle(value)
}

func applyScannerIdentity(path string, parsed ParsedName) ParsedName {
	value, ok := scannerSeriesIdentity.Load(filepath.Clean(path))
	if !ok {
		return parsed
	}
	identity, ok := value.(scannerIdentity)
	if !ok {
		return parsed
	}
	if identity.SeriesTitle != "" {
		if parsed.Title != "" && normalizeTitle(parsed.Title) != normalizeTitle(identity.SeriesTitle) {
			parsed.Alternates = append([]string{parsed.Title}, parsed.Alternates...)
		}
		parsed.Title = identity.SeriesTitle
	}
	if identity.Season > 0 {
		parsed.Season = identity.Season
	}
	if identity.Episode > 0 {
		parsed.Episode = identity.Episode
	}
	parsed.Alternates = uniqueTitles(parsed.Title, parsed.Alternates)
	return parsed
}

func (s *Service) scannerIdentityForMedia(ctx context.Context, mediaID int64) (scannerIdentity, error) {
	var out scannerIdentity
	err := s.db.QueryRowContext(ctx, `SELECT source_root,series_key,series_title,season_number,episode_number,absolute_number FROM media_series_identity WHERE media_id=?`, mediaID).
		Scan(&out.SourceRoot, &out.SeriesKey, &out.SeriesTitle, &out.Season, &out.Episode, &out.Absolute)
	return out, err
}

func loadSourceItemIdentity(ctx context.Context, db *sql.DB, item *SourceItem) {
	if item == nil || item.ID == 0 {
		return
	}
	_ = db.QueryRowContext(ctx, `SELECT source_root,series_key,series_title,season_number,episode_number,absolute_number FROM media_series_identity WHERE media_id=?`, item.ID).
		Scan(&item.SourceRoot, &item.SeriesKey, &item.SeriesTitle, &item.Season, &item.Episode, &item.Absolute)
}

func maxScannerInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func scannerIdentityDebug(identity scannerIdentity) string {
	return fmt.Sprintf("%s S%02dE%02d", identity.SeriesTitle, identity.Season, identity.Episode)
}
