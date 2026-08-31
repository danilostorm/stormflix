package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DeduplicateReport describes a lossless asset optimization pass. Duplicate
// files keep their existing paths, but identical content is backed by one
// inode through hard links. Database/public URLs therefore remain unchanged.
type DeduplicateReport struct {
	ScannedFiles int   `json:"scanned_files"`
	HashedFiles  int   `json:"hashed_files"`
	LinkedFiles  int   `json:"linked_files"`
	SavedBytes   int64 `json:"saved_bytes"`
}

type dedupeFile struct {
	path string
	size int64
}

// Deduplicate consolidates byte-identical regular files inside the configured
// asset root. It is intentionally lossless: no image is resized or recompressed.
// Files on filesystems that do not support hard links are simply left untouched.
func (s *Store) Deduplicate() (DeduplicateReport, error) {
	root, _ := s.Snapshot()
	report := DeduplicateReport{}
	bySize := map[int64][]dedupeFile{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".dedupe-tmp") {
			return nil
		}
		report.ScannedFiles++
		if info.Size() > 0 {
			bySize[info.Size()] = append(bySize[info.Size()], dedupeFile{path: path, size: info.Size()})
		}
		return nil
	})
	if err != nil {
		return report, err
	}

	sizes := make([]int64, 0, len(bySize))
	for size, files := range bySize {
		if len(files) > 1 {
			sizes = append(sizes, size)
		}
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] > sizes[j] })

	for _, size := range sizes {
		canonical := map[string]string{}
		for _, file := range bySize[size] {
			hash, err := fileSHA256(file.path)
			if err != nil {
				continue
			}
			report.HashedFiles++
			first := canonical[hash]
			if first == "" {
				canonical[hash] = file.path
				continue
			}
			firstInfo, firstErr := os.Stat(first)
			fileInfo, fileErr := os.Stat(file.path)
			if firstErr != nil || fileErr != nil || os.SameFile(firstInfo, fileInfo) {
				continue
			}

			tmp := file.path + ".dedupe-tmp"
			_ = os.Remove(tmp)
			if err := os.Link(first, tmp); err != nil {
				continue
			}
			if err := os.Rename(tmp, file.path); err != nil {
				_ = os.Remove(tmp)
				continue
			}
			report.LinkedFiles++
			report.SavedBytes += file.size
		}
	}
	return report, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
