package games

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type gameLookupHashSet struct {
	MD5  string
	SHA1 string
	CRC  string
}

type screenScraperROMDescriptor struct {
	Name string
	Size int64
}

// The ScreenScraper adapter was originally written with legacy query names and
// did not retain the byte size/name that its official jeuInfos endpoint expects.
// Hash calculation already knows the exact payload being hashed, including the
// inner ROM of a ZIP, so keep that descriptor keyed by MD5 for the HTTP adapter.
var screenScraperROMDescriptors sync.Map // md5 -> screenScraperROMDescriptor

func (s *Service) gameLookupHashes(ctx context.Context, gameID int64) (gameLookupHashSet, error) {
	var path, extension string
	if err := s.db.QueryRowContext(ctx, `SELECT path,extension FROM game_files WHERE game_id=? AND available=1 ORDER BY id LIMIT 1`, gameID).Scan(&path, &extension); err != nil {
		return gameLookupHashSet{}, err
	}
	var reader io.ReadCloser
	var expected int64
	romName := filepath.Base(path)
	if strings.EqualFold(extension, ".zip") {
		archive, inner, _, err := singleZIPROM(path)
		if err != nil {
			return gameLookupHashSet{}, err
		}
		stream, err := inner.Open()
		if err != nil {
			_ = archive.Close()
			return gameLookupHashSet{}, err
		}
		reader = &zipHashReader{ReadCloser: stream, archive: archive}
		expected = int64(inner.UncompressedSize64)
		if name := strings.TrimSpace(filepath.Base(inner.Name)); name != "" {
			romName = name
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			return gameLookupHashSet{}, err
		}
		reader = file
		if info, statErr := file.Stat(); statErr == nil {
			expected = info.Size()
		}
	}
	defer reader.Close()
	if expected <= 0 || expected > maxROMBytes {
		return gameLookupHashSet{}, errors.New("ROM size is outside metadata hash limits")
	}
	md5Hash, sha1Hash, crcHash := md5.New(), sha1.New(), crc32.NewIEEE()
	n, err := io.Copy(io.MultiWriter(md5Hash, sha1Hash, crcHash), io.LimitReader(reader, maxROMBytes+1))
	if err != nil {
		return gameLookupHashSet{}, err
	}
	if n != expected || n > maxROMBytes {
		return gameLookupHashSet{}, fmt.Errorf("ROM changed while computing provider hashes: got %d bytes, expected %d", n, expected)
	}
	md5Value := hex.EncodeToString(md5Hash.Sum(nil))
	screenScraperROMDescriptors.Store(strings.ToLower(md5Value), screenScraperROMDescriptor{Name: romName, Size: expected})
	return gameLookupHashSet{
		MD5:  md5Value,
		SHA1: hex.EncodeToString(sha1Hash.Sum(nil)),
		CRC:  fmt.Sprintf("%08X", crcHash.Sum32()),
	}, nil
}

type zipHashReader struct {
	io.ReadCloser
	archive io.Closer
}

func (r *zipHashReader) Close() error {
	streamErr := r.ReadCloser.Close()
	archiveErr := r.archive.Close()
	if streamErr != nil {
		return streamErr
	}
	return archiveErr
}
