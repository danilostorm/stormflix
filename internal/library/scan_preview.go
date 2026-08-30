package library

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ScanPreviewItem struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Change string `json:"change"`
}

type ScanPreview struct {
	LibraryID      int64             `json:"library_id"`
	Existing       int               `json:"existing"`
	Discovered     int               `json:"discovered"`
	New            int               `json:"new"`
	Changed        int               `json:"changed"`
	Missing        int               `json:"missing"`
	Unchanged      int               `json:"unchanged"`
	SourcesScanned int               `json:"sources_scanned"`
	SourcesOffline int               `json:"sources_offline"`
	DurationMS     int64             `json:"duration_ms"`
	Samples        []ScanPreviewItem `json:"samples"`
}

// PreviewMulti traverses the same enabled sources as ScanMulti but never
// changes media availability or inserts/deletes catalog rows. Offline roots are
// treated conservatively, exactly like the real scanner, so a disconnected
// Drive never appears as thousands of deletions in the preview.
func (s *Service) PreviewMulti(ctx context.Context, libraryID int64) (ScanPreview, error) {
	started := time.Now()
	lib, err := s.Get(ctx, libraryID)
	if err != nil {
		return ScanPreview{}, err
	}
	sources, err := s.scanSources(ctx, libraryID)
	if err != nil {
		return ScanPreview{}, err
	}
	if len(sources) == 0 && strings.TrimSpace(lib.Path) != "" {
		sources = []LibrarySource{{LibraryID: libraryID, Path: lib.Path, Label: "Origem 1", Enabled: true}}
	}

	filesByPath := map[string]discoveredFile{}
	scannedRoots := []string{}
	preview := ScanPreview{LibraryID: libraryID, Samples: []ScanPreviewItem{}}
	enabled := 0
	for _, source := range sources {
		if source.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return preview, errors.New("library has no enabled sources")
	}

	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		root := filepath.Clean(source.Path)
		sourceLibrary := lib
		sourceLibrary.Path = root
		discovered, discoverErr := s.discover(ctx, sourceLibrary, libraryID)
		if discoverErr != nil {
			if ctx.Err() != nil {
				return preview, ctx.Err()
			}
			preview.SourcesOffline++
			continue
		}
		if len(discovered) == 0 {
			previous, countErr := s.existingAvailableUnderRoot(ctx, libraryID, root)
			if countErr == nil && previous > 0 {
				preview.SourcesOffline++
				continue
			}
		}
		preview.SourcesScanned++
		scannedRoots = append(scannedRoots, root)
		for _, file := range discovered {
			filesByPath[filepath.Clean(file.path)] = file
		}
	}
	if len(scannedRoots) == 0 {
		return preview, errors.New("nenhuma origem respondeu à simulação; catálogo anterior preservado")
	}

	type existingItem struct {
		path         string
		title        string
		sizeBytes    int64
		modifiedUnix int64
	}
	existing := map[string]existingItem{}
	rows, err := s.db.QueryContext(ctx, `SELECT path,title,size_bytes,modified_unix FROM media WHERE library_id=? AND available=1`, libraryID)
	if err != nil {
		return preview, err
	}
	for rows.Next() {
		var item existingItem
		if err := rows.Scan(&item.path, &item.title, &item.sizeBytes, &item.modifiedUnix); err != nil {
			_ = rows.Close()
			return preview, err
		}
		existing[filepath.Clean(item.path)] = item
	}
	if err := rows.Close(); err != nil {
		return preview, err
	}
	preview.Existing = len(existing)
	preview.Discovered = len(filesByPath)

	for path, file := range filesByPath {
		old, ok := existing[path]
		if !ok {
			preview.New++
			preview.addSample(ScanPreviewItem{Path: path, Title: file.title, Change: "novo"})
			continue
		}
		changed := (file.sizeBytes > 0 && old.sizeBytes > 0 && file.sizeBytes != old.sizeBytes) || (file.modifiedUnix > 0 && old.modifiedUnix > 0 && file.modifiedUnix != old.modifiedUnix)
		if changed {
			preview.Changed++
			preview.addSample(ScanPreviewItem{Path: path, Title: old.title, Change: "alterado"})
		} else {
			preview.Unchanged++
		}
	}
	for path, old := range existing {
		if _, ok := filesByPath[path]; ok {
			continue
		}
		if underAnyRoot(path, scannedRoots) {
			preview.Missing++
			preview.addSample(ScanPreviewItem{Path: path, Title: old.title, Change: "ausente"})
		}
	}
	sort.SliceStable(preview.Samples, func(i, j int) bool {
		if preview.Samples[i].Change == preview.Samples[j].Change {
			return preview.Samples[i].Title < preview.Samples[j].Title
		}
		return preview.Samples[i].Change < preview.Samples[j].Change
	})
	preview.DurationMS = time.Since(started).Milliseconds()
	return preview, nil
}

func (p *ScanPreview) addSample(item ScanPreviewItem) {
	if len(p.Samples) < 40 {
		p.Samples = append(p.Samples, item)
	}
}
