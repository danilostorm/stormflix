package games

import (
	"context"
	"errors"
	"strings"
)

// RequireMetadataPrimary prevents artwork-only configuration from creating a
// queue that can never identify a game. SteamGridDB enriches an existing match;
// IGDB or MobyGames currently owns the primary title/metadata match.
func (s *Service) RequireMetadataPrimary(ctx context.Context) error {
	public, secrets, enabled, err := s.ProviderSecretsForRuntime(ctx, "igdb")
	if err != nil {
		return err
	}
	if enabled && strings.TrimSpace(public["client_id"]) != "" && strings.TrimSpace(secrets["client_secret"]) != "" {
		return nil
	}
	_, secrets, enabled, err = s.ProviderSecretsForRuntime(ctx, "mobygames")
	if err != nil {
		return err
	}
	if enabled && strings.TrimSpace(secrets["api_key"]) != "" {
		return nil
	}
	return errors.New("configure e ative IGDB ou MobyGames antes de iniciar metadados; SteamGridDB é somente enriquecimento de artwork")
}
