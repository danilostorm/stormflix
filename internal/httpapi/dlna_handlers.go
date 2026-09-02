package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/dlna"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
)

var stormflixDLNA = dlna.NewManager()

type dlnaPlayInput struct {
	MediaURL string `json:"media_url"`
	Title string `json:"title"`
	MIME string `json:"mime"`
	PositionSeconds float64 `json:"position_seconds"`
}
type dlnaControlInput struct { Command string `json:"command"`; PositionSeconds float64 `json:"position_seconds"` }

func (s *server) dlnaDevices(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "1"
	ctx, cancel := contextWithTimeout(r, 3*time.Second); defer cancel()
	devices, err := stormflixDLNA.Discover(ctx, force)
	if err != nil { writeError(w, http.StatusServiceUnavailable, fmt.Errorf("DLNA discovery failed: %w", err)); return }
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices, "network": "server"})
}

func (s *server) dlnaPlay(w http.ResponseWriter, r *http.Request) {
	deviceID := dlna.ParseDeviceID(r.PathValue("device_id")); var in dlnaPlayInput
	if decodeJSON(w, r, &in) != nil { return }
	if err := s.validateDLNAMediaURL(r, in.MediaURL); err != nil { writeError(w, http.StatusBadRequest, err); return }
	ctx, cancel := contextWithTimeout(r, 8*time.Second); defer cancel()
	device, err := stormflixDLNA.Find(ctx, deviceID); if err != nil { writeError(w, http.StatusNotFound, err); return }
	if err := stormflixDLNA.Play(ctx, device, in.MediaURL, in.Title, in.MIME, in.PositionSeconds); err != nil { writeError(w, http.StatusBadGateway, err); return }
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "device": device.Name})
}

func (s *server) dlnaControl(w http.ResponseWriter, r *http.Request) {
	deviceID := dlna.ParseDeviceID(r.PathValue("device_id")); var in dlnaControlInput
	if decodeJSON(w, r, &in) != nil { return }
	ctx, cancel := contextWithTimeout(r, 6*time.Second); defer cancel()
	device, err := stormflixDLNA.Find(ctx, deviceID); if err != nil { writeError(w, http.StatusNotFound, err); return }
	if err := stormflixDLNA.Control(ctx, device, in.Command, in.PositionSeconds); err != nil { writeError(w, http.StatusBadGateway, err); return }
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func contextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) { return context.WithTimeout(r.Context(), timeout) }

func (s *server) validateDLNAMediaURL(r *http.Request, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw)); if err != nil || !parsed.IsAbs() || parsed.Path == "" { return errors.New("invalid DLNA media URL") }
	allowedHosts := map[string]bool{strings.ToLower(strings.TrimSpace(r.Host)): true}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwarded != "" { allowedHosts[strings.ToLower(forwarded)] = true }
	if !allowedHosts[strings.ToLower(parsed.Host)] { return errors.New("DLNA media URL must belong to this StormFlix server") }
	mediaID := playbackGrantMediaID(parsed.Path)
	if mediaID <= 0 || !playbackGrantAllowedPath(mediaID, parsed.Path) { return errors.New("DLNA media URL is outside the playback grant scope") }
	claims, err := s.parsePlaybackGrant(strings.TrimSpace(parsed.Query().Get("st")))
	if err != nil || claims.MediaID != mediaID { return errors.New("DLNA media URL does not contain a valid playback grant") }
	u := currentUser(r); if u.ID <= 0 || claims.UserID != u.ID { return errors.New("DLNA playback grant belongs to another user") }
	return nil
}

func dlnaSessionID(device dlna.Device) string { sum := sha256.Sum256([]byte("stormflix-dlna|" + device.ID)); return hex.EncodeToString(sum[:16]) }

func (s *server) jellyfinSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2600*time.Millisecond); defer cancel()
	devices, err := stormflixDLNA.Discover(ctx, false); if err != nil { writeJSON(w, http.StatusOK, []any{}); return }
	now := time.Now().UTC().Format(time.RFC3339Nano); out := make([]any, 0, len(devices))
	for _, device := range devices {
		commands := []string{"Play", "Playstate", "Seek", "Stop", "Pause"}
		out = append(out, map[string]any{
			"Id": dlnaSessionID(device), "Client": "DLNA", "DeviceName": device.Name, "DeviceId": device.ID,
			"ApplicationVersion": "UPnP AVTransport", "IsActive": true, "SupportsMediaControl": true, "SupportsRemoteControl": true,
			"PlayableMediaTypes": []string{"Video", "Audio"}, "SupportedCommands": commands, "LastActivityDate": now, "LastPlaybackCheckIn": now,
			"ServerId": s.jellyfinServerID(), "Capabilities": map[string]any{"PlayableMediaTypes": []string{"Video", "Audio"}, "SupportedCommands": commands, "SupportsMediaControl": true},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) jellyfinDLNAPlay(w http.ResponseWriter, r *http.Request) {
	device, err := s.dlnaDeviceForSession(r, r.PathValue("session_id")); if err != nil { writeError(w, http.StatusNotFound, err); return }
	mediaID, ok := s.jellyfinPlayMediaID(r); if !ok { writeError(w, http.StatusBadRequest, errors.New("Jellyfin Play To request did not contain a media item")); return }
	start := dlna.SecondsFromTicks(jellyfinQueryValue(r, "StartPositionTicks"))
	audioStream := -1
	if raw := strings.TrimSpace(jellyfinQueryValue(r, "AudioStreamIndex")); raw != "" { audioStream, _ = strconv.Atoi(raw) }
	resource, title, mimeType, err := s.jellyfinDLNAResource(r, mediaID, audioStream); if err != nil { writeError(w, http.StatusBadRequest, err); return }
	ctx, cancel := contextWithTimeout(r, 8*time.Second); defer cancel()
	if err := stormflixDLNA.Play(ctx, device, resource, title, mimeType, start); err != nil { writeError(w, http.StatusBadGateway, err); return }
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) jellyfinDLNAControl(w http.ResponseWriter, r *http.Request) {
	device, err := s.dlnaDeviceForSession(r, r.PathValue("session_id")); if err != nil { writeError(w, http.StatusNotFound, err); return }
	command := strings.ToLower(strings.TrimSpace(r.PathValue("command")))
	position := dlna.SecondsFromTicks(jellyfinQueryValue(r, "SeekPositionTicks"))
	switch command { case "playpause": command="play"; case "unpause": command="play"; case "pause", "stop", "seek", "play": default: writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported Jellyfin DLNA command %q", command)); return }
	ctx, cancel := contextWithTimeout(r, 6*time.Second); defer cancel()
	if err := stormflixDLNA.Control(ctx, device, command, position); err != nil { writeError(w, http.StatusBadGateway, err); return }
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) dlnaDeviceForSession(r *http.Request, sessionID string) (dlna.Device, error) {
	ctx, cancel := contextWithTimeout(r, 2600*time.Millisecond); defer cancel()
	devices, err := stormflixDLNA.Discover(ctx, false); if err != nil { return dlna.Device{}, err }
	for _, device := range devices { if dlnaSessionID(device) == strings.TrimSpace(sessionID) { return device, nil } }
	return dlna.Device{}, errors.New("DLNA session not found")
}

func (s *server) jellyfinPlayMediaID(r *http.Request) (int64, bool) {
	for key, values := range r.URL.Query() {
		if !strings.EqualFold(key, "ItemIds") { continue }
		for _, value := range values {
			for _, part := range strings.Split(value, ",") {
				candidate := strings.TrimSpace(part)
				if original, ok := s.jellyfinInternalID(r.Context(), candidate); ok { candidate = original }
				if id, ok := jfParsePrefixedID(candidate, "m"); ok { return id, true }
			}
		}
	}
	return 0, false
}

func (s *server) jellyfinDLNAResource(r *http.Request, mediaID int64, audioStream int) (string, string, string, error) {
	u := currentUser(r)
	item, err := s.media.GetStreamItem(r.Context(), mediaID)
	if errors.Is(err, sql.ErrNoRows) || !item.Available { return "", "", "", errors.New("media not found") }
	if err != nil { return "", "", "", err }
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) { return "", "", "", errors.New("library access denied") }
	source, err := s.probeMediaSource(r.Context(), mediaID, item.Path, item.ModifiedUnix); if err != nil { return "", "", "", err }
	request := playback.Request{ClientKind:"dlna", ClientName:"StormFlix DLNA", ClientVersion:"1.0", Capabilities:playback.Capabilities{
		Containers:[]string{"mp4"}, VideoCodecs:[]string{"h264"}, AudioCodecs:[]string{"aac","mp3","ac3"}, AllowRemux:true, AllowAudioCompatibility:true, AllowVideoTranscode:false, ServerSelectsAudio:true,
	}}
	plan := playback.DecideForClient(source, request); if audioStream >= 0 { plan = playback.ApplyAudioStream(source, request, plan, audioStream) }
	if !plan.Available || plan.Mode == playback.ModeVideoTranscode { return "", "", "", errors.New("este renderer DLNA precisa de um formato de vídeo que o modo Play To progressivo ainda não pode gerar") }
	path, _ := playbackExecutionURLs(mediaID, plan)
	profileID := s.selectedProfileID(r, u.ID); if profileID <= 0 { profileID = s.jellyfinDefaultProfileID(r.Context(), u) }
	token, _, err := s.issuePlaybackGrant(u.ID, profileID, mediaID); if err != nil { return "", "", "", err }
	parsed, _ := url.Parse(path); query := parsed.Query(); query.Set("st", token); parsed.RawQuery = query.Encode()
	base := strings.TrimSuffix(jellyfinRequestBaseURL(r), "/"); if base == "" { return "", "", "", errors.New("could not determine StormFlix public playback address") }
	mimeType := "video/mp4"; if strings.HasPrefix(strings.ToLower(plan.AudioCodec), "mp3") && plan.VideoCodec == "" { mimeType = "audio/mpeg" }
	return base + parsed.String(), item.Title, mimeType, nil
}
