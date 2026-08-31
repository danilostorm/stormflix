package settings

import (
	"context"
	"strings"

	"github.com/danilostorm/stormflix/internal/config"
)

type TraktApplicationPublic struct {
	Configured             bool   `json:"configured"`
	ClientIDConfigured     bool   `json:"client_id_configured"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	RedirectURI            string `json:"redirect_uri"`
}

type TraktApplicationUpdate struct {
	ClientID     *string `json:"client_id"`
	ClientSecret *string `json:"client_secret"`
	RedirectURI  *string `json:"redirect_uri"`
}

// TraktApplication resolves persisted Admin settings over environment defaults.
// Presence of an empty persisted credential is intentional: it means the Admin
// explicitly cleared that value and therefore also overrides an environment
// default. Non-empty Client credentials are encrypted with settings.key at rest.
func (s *Service) TraktApplication(ctx context.Context, base config.Config) (string, string, string, TraktApplicationPublic, error) {
	values, err := s.all(ctx)
	if err != nil {
		return "", "", "", TraktApplicationPublic{}, err
	}
	clientID := strings.TrimSpace(base.TraktClientID)
	clientSecret := strings.TrimSpace(base.TraktClientSecret)
	redirectURI := strings.TrimSpace(base.TraktRedirectURI)
	if redirectURI == "" {
		redirectURI = "urn:ietf:wg:oauth:2.0:oob"
	}
	if value, ok := values["trakt_client_id"]; ok {
		value = strings.TrimSpace(value)
		if value == "" {
			clientID = ""
		} else {
			clientID, err = s.decrypt(value)
			if err != nil {
				return "", "", "", TraktApplicationPublic{}, err
			}
		}
	}
	if value, ok := values["trakt_client_secret"]; ok {
		value = strings.TrimSpace(value)
		if value == "" {
			clientSecret = ""
		} else {
			clientSecret, err = s.decrypt(value)
			if err != nil {
				return "", "", "", TraktApplicationPublic{}, err
			}
		}
	}
	if value, ok := values["trakt_redirect_uri"]; ok && strings.TrimSpace(value) != "" {
		redirectURI = strings.TrimSpace(value)
	}
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	return clientID, clientSecret, redirectURI, TraktApplicationPublic{
		Configured:             clientID != "" && clientSecret != "",
		ClientIDConfigured:     clientID != "",
		ClientSecretConfigured: clientSecret != "",
		RedirectURI:            redirectURI,
	}, nil
}

func (s *Service) UpdateTraktApplication(ctx context.Context, in TraktApplicationUpdate) error {
	for key, value := range map[string]*string{"trakt_client_id": in.ClientID, "trakt_client_secret": in.ClientSecret} {
		if value == nil {
			continue
		}
		plain := strings.TrimSpace(*value)
		if plain == "" {
			continue
		}
		if plain == "__clear__" {
			if err := s.put(ctx, key, ""); err != nil {
				return err
			}
			continue
		}
		sealed, err := s.encrypt(plain)
		if err != nil {
			return err
		}
		if err := s.put(ctx, key, sealed); err != nil {
			return err
		}
	}
	if in.RedirectURI != nil {
		value := strings.TrimSpace(*in.RedirectURI)
		if value == "" {
			value = "urn:ietf:wg:oauth:2.0:oob"
		}
		if err := s.put(ctx, "trakt_redirect_uri", value); err != nil {
			return err
		}
	}
	return nil
}
