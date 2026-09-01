package games

func (s *Service) ResumeMetadataJobs() {
	_, _ = s.db.Exec(`UPDATE game_metadata_jobs SET status='queued',progress=CASE WHEN progress>=100 THEN 0 ELSE progress END,message='retomando após reinício do servidor',started_at=NULL,finished_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE status='running'`)
	go s.drainMetadata()
}
