package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const jellyfinClientLogMaxBytes = 1_000_000

// jellyfinUserViewGroupingOptions matches the modern Jellyfin SDK endpoint.
// StormFlix does not currently expose special/preset view grouping, so the
// correct compatible response is an empty list.
func (s *server) jellyfinUserViewGroupingOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// jellyfinClientLogDocument accepts the crash/diagnostic document uploaded by
// official Jellyfin clients. Android TV shows a "report sent to server" dialog
// after an uncaught exception; keeping that report in StormFlix activity logs
// makes client-side crashes diagnosable without adb access to the Fire device.
func (s *server) jellyfinClientLogDocument(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, jellyfinClientLogMaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ResponseStatus": map[string]any{
				"ErrorCode": "BadRequest",
				"Message":   "Unable to read client log",
			},
		})
		return
	}
	if len(body) > jellyfinClientLogMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"ResponseStatus": map[string]any{
				"ErrorCode": "RequestTooLarge",
				"Message":   "Client log exceeds 1000000 bytes",
			},
		})
		return
	}

	u := currentUser(r)
	report := strings.TrimSpace(string(body))
	if report == "" {
		report = "[empty client log document]"
	}
	uid := u.ID
	s.admin.Log(
		r.Context(),
		"error",
		"jellyfin",
		"JELLYFIN_CLIENT_CRASH",
		&uid,
		fmt.Sprintf("client=%q\n%s", shortDevice(r.UserAgent()), report),
	)

	filename := fmt.Sprintf("stormflix-jellyfin-client-%d.log", time.Now().UTC().UnixMilli())
	writeJSON(w, http.StatusOK, map[string]any{"FileName": filename})
}
