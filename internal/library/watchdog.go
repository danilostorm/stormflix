package library

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) watchScanProgress(libraryID int64, ctx context.Context, cancel context.CancelFunc, maxIdle time.Duration) {
	if maxIdle <= 0 {
		maxIdle = 6 * time.Minute
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var idleSeconds int64
			var status string
			err := s.db.QueryRow(`SELECT COALESCE(strftime('%s','now')-strftime('%s',updated_at),0),last_scan_status FROM libraries WHERE id=?`, libraryID).Scan(&idleSeconds, &status)
			if err != nil || (status != "running" && status != "cancelling") {
				return
			}
			if time.Duration(idleSeconds)*time.Second <= maxIdle {
				continue
			}
			minutes := int(maxIdle.Round(time.Minute) / time.Minute)
			message := fmt.Sprintf("scan ficou sem progresso por mais de %d minutos; catálogo anterior preservado", minutes)
			_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status='timeout',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND last_scan_status IN ('running','cancelling')`, message, libraryID)
			cancel()
			return
		}
	}
}
