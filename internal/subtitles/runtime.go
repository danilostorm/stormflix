package subtitles

import "github.com/danilostorm/stormflix/internal/config"

func (s *Service) Configure(cfg config.Config) {
	s.mu.Lock()
	s.providers = []Provider{
		NewOpenSubtitlesProvider(cfg.OpenSubtitlesAPIKey, cfg.OpenSubtitlesUsername, cfg.OpenSubtitlesPassword, cfg.OpenSubtitlesUserAgent),
		NewSubDLProvider(cfg.SubDLAPIKey),
	}
	s.mu.Unlock()
}
