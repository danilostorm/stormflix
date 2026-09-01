package games

// The provider catalogue is declared in admin.go because the Admin UI consumes
// it directly. Keep the long-tail roadmap there, while promoting providers to
// configurable only when StormFlix has a real runtime adapter for them.
func init() {
	for i := range providerDefinitions {
		d := &providerDefinitions[i]
		switch d.Key {
		case "screenscraper":
			d.Stage = "configuravel"
			d.Description = "Identificação por MD5/SHA1/CRC. Usuário/senha são da conta ScreenScraper (doações aumentam limites/threads); Developer ID e Developer Password pertencem ao aplicativo que usa a API e são exigidos pela documentação oficial."
			d.Fields = []ProviderField{
				{Key: "dev_id", Label: "Developer ID do aplicativo", Required: true, Placeholder: "credencial API do software StormFlix"},
				{Key: "dev_password", Label: "Developer Password do aplicativo", Secret: true, Required: true, Placeholder: "••••••••"},
				{Key: "username", Label: "Usuário ScreenScraper", Required: true, Placeholder: "sua conta / conta doadora"},
				{Key: "password", Label: "Senha ScreenScraper", Secret: true, Required: true, Placeholder: "••••••••"},
			}
		case "retroachievements":
			d.Stage = "configuravel"
			d.Description = "Enriquecimento quando há ID RetroAchievements verificado via Hasheous ou tag local (ra-ID)."
			d.Fields = []ProviderField{{Key: "api_key", Label: "Web API Key", Secret: true, Required: true, Placeholder: "••••••••"}}
		case "hasheous":
			d.Stage = "configuravel"
			d.Description = "Correspondência por MD5/SHA1/CRC e ponte de IDs. Use a base https://hasheous.org/api/v1; o StormFlix chama POST /Lookup/ByHash e envia a chave no cabeçalho X-API-Key."
			d.Fields = []ProviderField{
				{Key: "base_url", Label: "URL base da API", Required: false, Placeholder: "https://hasheous.org/api/v1"},
				{Key: "api_key", Label: "API Key (X-API-Key)", Secret: true, Required: true, Placeholder: "••••••••"},
			}
		case "thegamesdb":
			d.Stage = "configuravel"
			d.Description = "Fallback de metadados e artwork para títulos não resolvidos pelas fontes anteriores."
			d.Fields = []ProviderField{{Key: "api_key", Label: "API Key", Secret: true, Required: true, Placeholder: "••••••••"}}
		case "libretro":
			d.Stage = "configuravel"
			d.Description = "Artwork público de fallback do Libretro Thumbnails, alinhado ao RetroArch; não exige chave."
			d.Fields = nil
		}
	}
}

func metadataProviderRuntimeSupported(key string) bool {
	switch key {
	case "igdb", "mobygames", "steamgriddb", "screenscraper", "retroachievements", "hasheous", "thegamesdb", "libretro":
		return true
	default:
		return false
	}
}
