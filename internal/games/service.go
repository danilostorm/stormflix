package games

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/danilostorm/stormflix/internal/workload"
)

const maxROMBytes int64 = 512 << 20 // G1 is deliberately cartridge-focused.

type Service struct {
	db      *sql.DB
	mu      sync.Mutex
	worker  bool
	cancels map[int64]context.CancelFunc
	gate    *workload.Gate
}

type Job struct {
	ID         int64   `json:"id"`
	LibraryID  int64   `json:"library_id"`
	Library    string  `json:"library"`
	Status     string  `json:"status"`
	Progress   int     `json:"progress"`
	Total      int     `json:"total"`
	Processed  int     `json:"processed"`
	Matched    int     `json:"matched"`
	Failed     int     `json:"failed"`
	Message    string  `json:"message"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type Game struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"library_id"`
	Library     string `json:"library"`
	Platform    string `json:"platform"`
	Title       string `json:"title"`
	Overview    string `json:"overview"`
	ReleaseYear int    `json:"release_year"`
	CoverURL    string `json:"cover_url"`
	Favorite    bool   `json:"favorite"`
	PlaySeconds int64  `json:"play_seconds"`
	LastPlayed  string `json:"last_played_at"`
	CreatedAt   string `json:"created_at"`
}

type Home struct {
	Continue      []Game         `json:"continue_playing"`
	Favorites     []Game         `json:"favorites"`
	RecentlyAdded []Game         `json:"recently_added"`
	Platforms     []PlatformStat `json:"platforms"`
}

type PlatformStat struct {
	Platform string `json:"platform"`
	Label    string `json:"label"`
	Count    int    `json:"count"`
}

type discoveredROM struct {
	Path         string
	Platform     string
	Title        string
	Extension    string
	SizeBytes    int64
	ModifiedUnix int64
	Hash         string
	CoverPath    string
}

type sourceScan struct {
	Root  string
	ROMs  []discoveredROM
	Error error
}

func NewService(db *sql.DB) *Service {
	s := &Service{db: db, cancels: map[int64]context.CancelFunc{}, gate: workload.For(db)}
	_, _ = db.Exec(`UPDATE game_scan_jobs SET status='error',progress=100,message='scan interrompido por reinício do servidor; execute novamente',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE status IN ('running','cancelling')`)
	go s.drain()
	return s
}

var platformByExtension = map[string]string{
	".nes": "nes",
	".sfc": "snes", ".smc": "snes",
	".md": "genesis", ".gen": "genesis", ".smd": "genesis",
	".gb": "gb", ".gbc": "gbc", ".gba": "gba",
}

var platformLabels = map[string]string{
	"nes":     "Nintendo Entertainment System",
	"snes":    "Super Nintendo",
	"genesis": "Mega Drive / Genesis",
	"gb":      "Game Boy",
	"gbc":     "Game Boy Color",
	"gba":     "Game Boy Advance",
}

func SupportedExtensions() []string {
	out := make([]string, 0, len(platformByExtension)+1)
	for ext := range platformByExtension {
		out = append(out, ext)
	}
	out = append(out, ".zip")
	sort.Strings(out)
	return out
}

func PlatformLabel(platform string) string {
	if label := platformLabels[platform]; label != "" {
		return label
	}
	return strings.ToUpper(platform)
}

func (s *Service) IsGamesLibrary(ctx context.Context, libraryID int64) (bool, error) {
	var kind string
	err := s.db.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, libraryID).Scan(&kind)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(kind), "games"), nil
}

func (s *Service) EnqueueScan(ctx context.Context, libraryID int64) (Job, error) {
	var name, kind string
	var enabled bool
	if err := s.db.QueryRowContext(ctx, `SELECT name,kind,enabled FROM libraries WHERE id=?`, libraryID).Scan(&name, &kind, &enabled); err != nil {
		return Job{}, err
	}
	if !enabled {
		return Job{}, errors.New("library is disabled")
	}
	if !strings.EqualFold(strings.TrimSpace(kind), "games") {
		return Job{}, errors.New("library is not a games library")
	}
	var sourceCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_sources WHERE library_id=? AND enabled=1`, libraryID).Scan(&sourceCount); err != nil {
		return Job{}, err
	}
	if sourceCount == 0 {
		return Job{}, errors.New("game library has no enabled sources")
	}
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM game_scan_jobs WHERE library_id=? AND status IN ('queued','running','cancelling') ORDER BY id DESC LIMIT 1`, libraryID).Scan(&existingID)
	if err == nil {
		return s.Job(ctx, existingID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Job{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO game_scan_jobs(library_id,status,message) VALUES(?,'queued','aguardando na fila de jogos')`, libraryID)
	if err != nil {
		return Job{}, err
	}
	id, _ := res.LastInsertId()
	_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='queued',last_error='scan de jogos aguardando na fila',updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryID)
	go s.drain()
	return s.Job(ctx, id)
}

func (s *Service) EnqueueAll(ctx context.Context) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM libraries WHERE enabled=1 AND lower(trim(kind))='games' ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	jobs := []Job{}
	for _, id := range ids {
		job, err := s.EnqueueScan(ctx, id)
		if err == nil {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func (s *Service) Job(ctx context.Context, id int64) (Job, error) {
	var job Job
	err := s.db.QueryRowContext(ctx, `SELECT j.id,j.library_id,l.name,j.status,j.progress,j.total,j.processed,j.matched,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM game_scan_jobs j JOIN libraries l ON l.id=j.library_id WHERE j.id=?`, id).
		Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Progress, &job.Total, &job.Processed, &job.Matched, &job.Failed, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	return job, err
}

func (s *Service) Jobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	rows, err := s.db.QueryContext(ctx, `SELECT j.id,j.library_id,l.name,j.status,j.progress,j.total,j.processed,j.matched,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM game_scan_jobs j JOIN libraries l ON l.id=j.library_id ORDER BY CASE j.status WHEN 'running' THEN 0 WHEN 'cancelling' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END,j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Progress, &job.Total, &job.Processed, &job.Matched, &job.Failed, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	go s.drain()
	return out, rows.Err()
}

func (s *Service) CancelScan(ctx context.Context, libraryID int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE game_scan_jobs SET status='cancelled',progress=100,message='removido da fila pelo administrador',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status='queued'`, libraryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='cancelled',last_error='scan de jogos removido da fila; catálogo preservado',updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryID)
		return nil
	}
	s.mu.Lock()
	cancel := s.cancels[libraryID]
	s.mu.Unlock()
	if cancel == nil {
		return errors.New("game scan is not running")
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE game_scan_jobs SET status='cancelling',message='cancelamento solicitado; parando com segurança',updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status='running'`, libraryID)
	cancel()
	return nil
}

func (s *Service) drain() {
	s.mu.Lock()
	if s.worker {
		s.mu.Unlock()
		return
	}
	s.worker = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.worker = false
		s.mu.Unlock()
	}()

	for {
		var jobID, libraryID int64
		err := s.db.QueryRow(`SELECT id,library_id FROM game_scan_jobs WHERE status='queued' ORDER BY id LIMIT 1`).Scan(&jobID, &libraryID)
		if errors.Is(err, sql.ErrNoRows) || err != nil {
			return
		}
		res, err := s.db.Exec(`UPDATE game_scan_jobs SET status='running',progress=2,started_at=CURRENT_TIMESTAMP,message='descobrindo ROMs',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, jobID)
		if err != nil {
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.cancels[libraryID] = cancel
		s.mu.Unlock()
		_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status='running',last_error='descobrindo ROMs',updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryID)
		s.run(jobID, libraryID, ctx)
		cancel()
		s.mu.Lock()
		delete(s.cancels, libraryID)
		s.mu.Unlock()
	}
}

func (s *Service) run(jobID, libraryID int64, ctx context.Context) {
	sources, err := s.sourceRoots(ctx, libraryID)
	if err != nil {
		s.finishError(jobID, libraryID, err)
		return
	}
	scans := make([]sourceScan, 0, len(sources))
	total := 0
	for _, root := range sources {
		if err := s.gate.Wait(ctx, "game_scan", nil); err != nil {
			s.finishCancelled(jobID, libraryID)
			return
		}
		roms, discoverErr := discoverROMs(ctx, root)
		scans = append(scans, sourceScan{Root: root, ROMs: roms, Error: discoverErr})
		if discoverErr == nil {
			total += len(roms)
		}
		if ctx.Err() != nil {
			s.finishCancelled(jobID, libraryID)
			return
		}
	}
	_, _ = s.db.Exec(`UPDATE game_scan_jobs SET total=?,progress=8,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, total, fmt.Sprintf("%d ROM(s) encontrada(s) · calculando identidade por hash", total), jobID)

	processed, matched, failed := 0, 0, 0
	for i := range scans {
		if scans[i].Error != nil {
			failed++
			continue
		}
		for j := range scans[i].ROMs {
			if ctx.Err() != nil {
				s.finishCancelled(jobID, libraryID)
				return
			}
			if !s.waitForPlayback(ctx, jobID, processed, total, matched, failed) {
				s.finishCancelled(jobID, libraryID)
				return
			}
			rom := &scans[i].ROMs[j]
			hash, hashErr := hashROM(ctx, rom.Path, rom.SizeBytes)
			processed++
			if hashErr != nil {
				failed++
			} else {
				rom.Hash = hash
				matched++
			}
			progress := 8
			if total > 0 {
				progress = 8 + processed*78/total
			}
			if progress > 86 {
				progress = 86
			}
			_, _ = s.db.Exec(`UPDATE game_scan_jobs SET progress=?,processed=?,matched=?,failed=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, progress, processed, matched, failed, fmt.Sprintf("Identificando %d/%d · %s", processed, total, rom.Title), jobID)
		}
	}

	if ctx.Err() != nil {
		s.finishCancelled(jobID, libraryID)
		return
	}
	_, _ = s.db.Exec(`UPDATE game_scan_jobs SET progress=90,message='salvando catálogo de jogos',updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
	if err := s.commitScans(ctx, libraryID, scans); err != nil {
		s.finishError(jobID, libraryID, err)
		return
	}
	status := "completed"
	message := fmt.Sprintf("%d ROM(s) catalogada(s)", matched)
	if failed > 0 {
		status = "completed_with_errors"
		message += fmt.Sprintf(" · %d erro(s)/origem(ns) indisponível(is) preservado(s)", failed)
	}
	_, _ = s.db.Exec(`UPDATE game_scan_jobs SET status=?,progress=100,processed=?,matched=?,failed=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, processed, matched, failed, message, jobID)
	libraryStatus := "ok"
	if failed > 0 {
		libraryStatus = "partial"
	}
	_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status=?,last_scan_at=CURRENT_TIMESTAMP,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryStatus, message, libraryID)
}

func (s *Service) waitForPlayback(ctx context.Context, jobID int64, processed, total, matched, failed int) bool {
	err := s.gate.Wait(ctx, "game_scan", func(paused bool) {
		if paused {
			_, _ = s.db.Exec(`UPDATE game_scan_jobs SET message='Pausado para priorizar uma reprodução ou jogo ativo',processed=?,matched=?,failed=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, processed, matched, failed, jobID)
		}
	})
	return err == nil
}

func (s *Service) sourceRoots(ctx context.Context, libraryID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM library_sources WHERE library_id=? AND enabled=1 ORDER BY sort_order,id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return nil, err
		}
		root = filepath.Clean(strings.TrimSpace(root))
		if root != "" {
			out = append(out, root)
		}
	}
	return out, rows.Err()
}

func singleZIPROM(path string) (*zip.ReadCloser, *zip.File, string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, "", err
	}
	var selected *zip.File
	platform := ""
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name))
		candidatePlatform := platformByExtension[ext]
		if candidatePlatform == "" {
			continue
		}
		if file.UncompressedSize64 == 0 || file.UncompressedSize64 > uint64(maxROMBytes) {
			_ = archive.Close()
			return nil, nil, "", errors.New("ZIP ROM size is outside the safe cartridge limit")
		}
		if selected != nil {
			_ = archive.Close()
			return nil, nil, "", errors.New("ZIP contains more than one supported ROM")
		}
		selected = file
		platform = candidatePlatform
	}
	if selected == nil {
		_ = archive.Close()
		return nil, nil, "", errors.New("ZIP contains no supported cartridge ROM")
	}
	return archive, selected, platform, nil
}

func discoverROMs(ctx context.Context, root string) ([]discoveredROM, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("source is not a directory")
		}
		return nil, fmt.Errorf("game source %s: %w", root, err)
	}
	out := []discoveredROM{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		platform := platformByExtension[ext]
		fileInfo, err := entry.Info()
		if err != nil {
			return nil
		}
		if fileInfo.Size() <= 0 || fileInfo.Size() > maxROMBytes {
			return nil
		}
		romSize := fileInfo.Size()
		if ext == ".zip" {
			archive, romFile, zipPlatform, zipErr := singleZIPROM(path)
			if zipErr != nil {
				return nil
			}
			romSize = int64(romFile.UncompressedSize64)
			platform = zipPlatform
			_ = archive.Close()
		}
		if platform == "" {
			return nil
		}
		out = append(out, discoveredROM{Path: filepath.Clean(path), Platform: platform, Title: cleanROMTitle(path), Extension: ext, SizeBytes: romSize, ModifiedUnix: fileInfo.ModTime().Unix(), CoverPath: sidecarCover(path)})
		return nil
	})
	return out, err
}

func hashReader(ctx context.Context, reader io.Reader) (string, error) {
	h := sha256.New()
	buf := make([]byte, 1<<20)
	var read int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := reader.Read(buf)
		if n > 0 {
			read += int64(n)
			if read > maxROMBytes {
				return "", errors.New("ROM exceeded safe cartridge size limit while hashing")
			}
			_, _ = h.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if read <= 0 {
		return "", errors.New("ROM is empty")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashROM(ctx context.Context, path string, size int64) (string, error) {
	if size <= 0 || size > maxROMBytes {
		return "", errors.New("ROM size is outside the safe G1 limit")
	}
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		archive, romFile, _, err := singleZIPROM(path)
		if err != nil {
			return "", err
		}
		defer archive.Close()
		reader, err := romFile.Open()
		if err != nil {
			return "", err
		}
		defer reader.Close()
		return hashReader(ctx, reader)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return hashReader(ctx, file)
}

var trailingTagRE = regexp.MustCompile(`(?i)\s*(\([^)]*\)|\[[^]]*\])\s*$`)

func cleanROMTitle(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for {
		next := strings.TrimSpace(trailingTagRE.ReplaceAllString(name, ""))
		if next == name {
			break
		}
		name = next
	}
	name = strings.NewReplacer("_", " ", ".", " ").Replace(name)
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "Jogo sem título"
	}
	return name
}

func sidecarCover(romPath string) string {
	base := strings.TrimSuffix(romPath, filepath.Ext(romPath))
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		path := base + ext
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Size() > 0 && info.Size() <= 20<<20 {
			return path
		}
	}
	return ""
}

func (s *Service) commitScans(ctx context.Context, libraryID int64, scans []sourceScan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, scan := range scans {
		if scan.Error != nil {
			continue
		}
		prefix := filepath.Clean(scan.Root) + string(os.PathSeparator)
		if _, err := tx.ExecContext(ctx, `UPDATE game_files SET available=0,updated_at=CURRENT_TIMESTAMP WHERE game_id IN (SELECT id FROM games WHERE library_id=?) AND (path=? OR substr(path,1,?)=?)`, libraryID, filepath.Clean(scan.Root), len(prefix), prefix); err != nil {
			return err
		}
		for _, rom := range scan.ROMs {
			if rom.Hash == "" {
				continue
			}
			_, err := tx.ExecContext(ctx, `
INSERT INTO games(library_id,platform,title,sort_title,content_hash,cover_path)
VALUES(?,?,?,?,?,?)
ON CONFLICT(library_id,platform,content_hash) DO UPDATE SET
 title=excluded.title,sort_title=excluded.sort_title,
 cover_path=CASE WHEN excluded.cover_path<>'' THEN excluded.cover_path ELSE games.cover_path END,
 updated_at=CURRENT_TIMESTAMP`, libraryID, rom.Platform, rom.Title, strings.ToLower(rom.Title), rom.Hash, rom.CoverPath)
			if err != nil {
				return err
			}
			var gameID int64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM games WHERE library_id=? AND platform=? AND content_hash=?`, libraryID, rom.Platform, rom.Hash).Scan(&gameID); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
INSERT INTO game_files(game_id,path,extension,size_bytes,modified_unix,available)
VALUES(?,?,?,?,?,1)
ON CONFLICT(game_id,path) DO UPDATE SET extension=excluded.extension,size_bytes=excluded.size_bytes,modified_unix=excluded.modified_unix,available=1,updated_at=CURRENT_TIMESTAMP`, gameID, rom.Path, rom.Extension, rom.SizeBytes, rom.ModifiedUnix)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Service) finishError(jobID, libraryID int64, err error) {
	message := err.Error()
	if len(message) > 400 {
		message = message[:400]
	}
	_, _ = s.db.Exec(`UPDATE game_scan_jobs SET status='error',progress=100,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, message, jobID)
	_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status='error',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, message, libraryID)
}

func (s *Service) finishCancelled(jobID, libraryID int64) {
	_, _ = s.db.Exec(`UPDATE game_scan_jobs SET status='cancelled',progress=100,message='scan cancelado; catálogo anterior preservado',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
	_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status='cancelled',last_error='scan de jogos cancelado; catálogo anterior preservado',updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryID)
}

func allowedClause(column string, allowed []int64) (string, []any) {
	if allowed == nil {
		return "", nil
	}
	if len(allowed) == 0 {
		return " AND 1=0", nil
	}
	marks := make([]string, len(allowed))
	args := make([]any, len(allowed))
	for i, id := range allowed {
		marks[i] = "?"
		args[i] = id
	}
	return " AND " + column + " IN (" + strings.Join(marks, ",") + ")", args
}

func (s *Service) List(ctx context.Context, profileID int64, allowed []int64, query, platform string, favorites bool, limit int) ([]Game, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	where := ` WHERE EXISTS(SELECT 1 FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1)`
	args := []any{profileID}
	clause, accessArgs := allowedClause("g.library_id", allowed)
	where += clause
	args = append(args, accessArgs...)
	if q := strings.TrimSpace(query); q != "" {
		where += ` AND lower(g.title) LIKE ?`
		args = append(args, "%"+strings.ToLower(q)+"%")
	}
	if p := strings.ToLower(strings.TrimSpace(platform)); p != "" {
		where += ` AND g.platform=?`
		args = append(args, p)
	}
	if favorites {
		where += ` AND COALESCE(ps.favorite,0)=1`
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT g.id,g.library_id,l.name,g.platform,g.title,g.overview,g.release_year,g.cover_path,COALESCE(ps.favorite,0),COALESCE(ps.play_seconds,0),COALESCE(ps.last_played_at,''),g.created_at FROM games g JOIN libraries l ON l.id=g.library_id LEFT JOIN game_profile_state ps ON ps.game_id=g.id AND ps.profile_id=?`+where+` ORDER BY CASE WHEN COALESCE(ps.last_played_at,'')<>'' THEN 0 ELSE 1 END,COALESCE(ps.last_played_at,g.created_at) DESC,g.sort_title,g.id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Game{}
	for rows.Next() {
		var game Game
		var cover string
		if err := rows.Scan(&game.ID, &game.LibraryID, &game.Library, &game.Platform, &game.Title, &game.Overview, &game.ReleaseYear, &cover, &game.Favorite, &game.PlaySeconds, &game.LastPlayed, &game.CreatedAt); err != nil {
			return nil, err
		}
		if cover != "" {
			game.CoverURL = fmt.Sprintf("/api/v1/games/%d/cover", game.ID)
		}
		out = append(out, game)
	}
	return out, rows.Err()
}

func (s *Service) Home(ctx context.Context, profileID int64, allowed []int64) (Home, error) {
	all, err := s.List(ctx, profileID, allowed, "", "", false, 300)
	if err != nil {
		return Home{}, err
	}
	home := Home{Continue: []Game{}, Favorites: []Game{}, RecentlyAdded: []Game{}, Platforms: []PlatformStat{}}
	counts := map[string]int{}
	for _, game := range all {
		counts[game.Platform]++
		if game.LastPlayed != "" && len(home.Continue) < 16 {
			home.Continue = append(home.Continue, game)
		}
		if game.Favorite && len(home.Favorites) < 24 {
			home.Favorites = append(home.Favorites, game)
		}
		if len(home.RecentlyAdded) < 30 {
			home.RecentlyAdded = append(home.RecentlyAdded, game)
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return PlatformLabel(keys[i]) < PlatformLabel(keys[j]) })
	for _, key := range keys {
		home.Platforms = append(home.Platforms, PlatformStat{Platform: key, Label: PlatformLabel(key), Count: counts[key]})
	}
	return home, nil
}

func (s *Service) Detail(ctx context.Context, id, profileID int64, allowed []int64) (Game, error) {
	games, err := s.List(ctx, profileID, allowed, "", "", false, 500)
	if err != nil {
		return Game{}, err
	}
	for _, game := range games {
		if game.ID == id {
			return game, nil
		}
	}
	return Game{}, sql.ErrNoRows
}

func (s *Service) SetFavorite(ctx context.Context, gameID, profileID int64, favorite bool) error {
	if profileID <= 0 {
		return errors.New("profile is required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO game_profile_state(profile_id,game_id,favorite) VALUES(?,?,?) ON CONFLICT(profile_id,game_id) DO UPDATE SET favorite=excluded.favorite,updated_at=CURRENT_TIMESTAMP`, profileID, gameID, favorite)
	return err
}

func (s *Service) CoverPath(ctx context.Context, gameID int64) (string, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `SELECT cover_path FROM games WHERE id=?`, gameID).Scan(&path)
	return path, err
}
