package httpapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var scanSourceProgressRE = regexp.MustCompile(`(?i)origem\s+(\d+)\s*/\s*(\d+)`)

type adminJobView struct {
	Key        string  `json:"key"`
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Label      string  `json:"label"`
	LibraryID  int64   `json:"library_id"`
	Library    string  `json:"library"`
	Status     string  `json:"status"`
	Progress   int     `json:"progress"`
	Current    int     `json:"current"`
	Total      int     `json:"total"`
	Success    int     `json:"success"`
	Failed     int     `json:"failed"`
	Message    string  `json:"message"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	UpdatedAt  string  `json:"updated_at"`
}

func (s *server) adminJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	out := []adminJobView{}

	scanJobs, err := s.libraries.ScanJobs(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, job := range scanJobs {
		progress := job.Progress
		if job.Status == "running" || job.Status == "cancelling" {
			if match := scanSourceProgressRE.FindStringSubmatch(job.Message); len(match) == 3 {
				current, _ := strconv.Atoi(match[1])
				total, _ := strconv.Atoi(match[2])
				if total > 0 {
					derived := 5 + ((current - 1) * 85 / total)
					if derived > progress {
						progress = derived
					}
				}
			}
		}
		out = append(out, adminJobView{
			Key: "scan:" + strconv.FormatInt(job.ID, 10), ID: job.ID, Kind: "scan", Label: "Scan de biblioteca",
			LibraryID: job.LibraryID, Library: job.Library, Status: job.Status, Progress: progress,
			Current: job.Files, Total: 0, Success: job.Files, Failed: job.SourcesOffline, Message: job.Message,
			CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, FinishedAt: job.FinishedAt, UpdatedAt: job.UpdatedAt,
		})
	}

	rows, err := s.db.QueryContext(r.Context(), `SELECT j.id,j.library_id,l.name,j.job_type,j.status,j.total,j.processed,j.matched,j.failed,j.message,j.series_title,j.created_at,j.started_at,j.finished_at,j.updated_at FROM metadata_jobs j JOIN libraries l ON l.id=j.library_id ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for rows.Next() {
		var id, libraryID int64
		var libraryName, jobType, status, message, seriesTitle, createdAt, updatedAt string
		var total, processed, matched, failed int
		var startedAt, finishedAt *string
		if err := rows.Scan(&id, &libraryID, &libraryName, &jobType, &status, &total, &processed, &matched, &failed, &message, &seriesTitle, &createdAt, &startedAt, &finishedAt, &updatedAt); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		label := "Metadados da biblioteca"
		kind := "metadata"
		if jobType == "series_refresh" {
			kind = "series_refresh"
			label = "Reorganizar obra principal"
			if seriesTitle != "" {
				label += " · " + seriesTitle
			}
		} else if jobType == "library_refresh" {
			label = "Atualização completa de metadados"
		}
		progress := 0
		if total > 0 {
			progress = processed * 100 / total
			if progress > 100 {
				progress = 100
			}
		}
		out = append(out, adminJobView{Key: kind + ":" + strconv.FormatInt(id, 10), ID: id, Kind: kind, Label: label, LibraryID: libraryID, Library: libraryName, Status: status, Progress: progress, Current: processed, Total: total, Success: matched, Failed: failed, Message: message, CreatedAt: createdAt, StartedAt: startedAt, FinishedAt: finishedAt, UpdatedAt: updatedAt})
	}
	_ = rows.Close()

	markerRows, err := s.db.QueryContext(r.Context(), `
SELECT j.id,j.library_id,l.name,j.series_title,j.season_number,j.status,j.progress,j.total,j.processed,j.detected,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at
FROM marker_analysis_jobs j JOIN libraries l ON l.id=j.library_id
ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for markerRows.Next() {
		var id, libraryID int64
		var libraryName, seriesTitle, status, message, createdAt, updatedAt string
		var season, progress, total, processed, detected, failed int
		var startedAt, finishedAt *string
		if err := markerRows.Scan(&id, &libraryID, &libraryName, &seriesTitle, &season, &status, &progress, &total, &processed, &detected, &failed, &message, &createdAt, &startedAt, &finishedAt, &updatedAt); err != nil {
			_ = markerRows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		label := "Detecção automática de introduções"
		if seriesTitle != "" {
			label += " · " + seriesTitle
		}
		if season > 0 {
			label += " · T" + strconv.Itoa(season)
		}
		out = append(out, adminJobView{
			Key: "intro_detection:" + strconv.FormatInt(id, 10), ID: id, Kind: "intro_detection", Label: label,
			LibraryID: libraryID, Library: libraryName, Status: status, Progress: progress,
			Current: processed, Total: total, Success: detected, Failed: failed, Message: message,
			CreatedAt: createdAt, StartedAt: startedAt, FinishedAt: finishedAt, UpdatedAt: updatedAt,
		})
	}
	_ = markerRows.Close()

	// Most useful operational order: running, queued, then recent history.
	statusRank := func(status string) int {
		switch status {
		case "running", "cancelling":
			return 0
		case "queued":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := statusRank(out[i].Status), statusRank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		ti, _ := time.Parse("2006-01-02 15:04:05", out[i].UpdatedAt)
		tj, _ := time.Parse("2006-01-02 15:04:05", out[j].UpdatedAt)
		return ti.After(tj)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) scanAllLibraries(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.libraries.EnqueueAllAdminScans(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "scanner", "Todas as bibliotecas foram colocadas na fila de scan", &uid, strconv.Itoa(len(jobs))+" job(s)")
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "queued": len(jobs), "jobs": jobs})
}
