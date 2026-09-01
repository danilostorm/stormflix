package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/danilostorm/stormflix/internal/games"
)

func (s *server) gameSavesGallery(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.games.SaveGallery(r.Context(), profileID, s.gameAllowedLibraries(r), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) adminGamesOverview(w http.ResponseWriter, r *http.Request) {
	out, err := s.games.AdminOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) adminGamesCatalog(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.games.AdminCatalogExact(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("platform"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) adminGamesProviders(w http.ResponseWriter, r *http.Request) {
	items, err := s.games.ProviderSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": items,
		"note":      "Credenciais ficam criptografadas no DataDir. Nenhum segredo é devolvido ao navegador; somente o estado configurado/não configurado.",
	})
}

func (s *server) adminUpdateGameProvider(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	var in struct {
		Enabled bool              `json:"enabled"`
		Values  map[string]string `json:"values"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.Values == nil {
		in.Values = map[string]string{}
	}
	if err := s.games.UpdateProviderSettings(r.Context(), games.ProviderUpdate{Provider: provider, Enabled: in.Enabled, Values: in.Values}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "games", "Games metadata provider settings updated", &uid, provider)
	s.adminGamesProviders(w, r)
}

func (s *server) adminGamesMetadataJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.games.MetadataJobs(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) adminStartGamesMetadata(w http.ResponseWriter, r *http.Request) {
	libraryID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1" || strings.EqualFold(r.URL.Query().Get("refresh"), "true")
	job, err := s.games.EnqueueMetadata(r.Context(), libraryID, refresh)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "games", "Games metadata job queued", &uid, job.Library)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *server) adminStartAllGamesMetadata(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "1" || strings.EqualFold(r.URL.Query().Get("refresh"), "true")
	job, err := s.games.EnqueueMetadata(r.Context(), 0, refresh)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "games", "All Games metadata job queued", &uid, job.Provider)
	writeJSON(w, http.StatusAccepted, job)
}
