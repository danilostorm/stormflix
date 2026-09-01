package httpapi

import (
	"net/http"
	"testing"
)

func TestGamesAdminMutationRoutesDoNotConflict(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Games Admin routes conflict in http.ServeMux: %v", recovered)
		}
	}()

	mux := http.NewServeMux()
	noop := func(http.ResponseWriter, *http.Request) {}

	for _, pattern := range []string{
		"PUT /api/v1/admin/games/providers/{provider}",
		"POST /api/v1/admin/games/metadata",
		"POST /api/v1/admin/games/libraries/{id}/metadata",
		"PUT /api/v1/admin/games/catalog/{id}/metadata-lock",
	} {
		mux.HandleFunc(pattern, noop)
	}
}
