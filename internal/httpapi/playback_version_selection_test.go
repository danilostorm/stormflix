package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
)

func versionSelectionServer(t *testing.T) (*server, playback.Source, playback.Request) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := &server{db: db, media: media.NewService(db), auth: auth.NewService(db)}
	for _, query := range []string{
		`INSERT INTO libraries(id,name,kind,path,enabled) VALUES(1,'UHD','movies','/media/uhd',1),(2,'FHD','movies','/media/fhd',1)`,
		`INSERT INTO media(id,library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(10,1,'Filme','/media/uhd/filme.mkv','.mkv',1000,1,1),(20,2,'Filme','/media/fhd/filme.mp4','.mp4',500,1,1)`,
		`INSERT INTO media_metadata(media_id,media_type,tmdb_id,status,content_rating_age) VALUES(10,'movie',1234,'matched',0),(20,'movie',1234,'matched',0)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	uhd := playback.Source{Container: "mkv", DurationSeconds: 3600, Streams: []playback.Stream{{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 1600, FrameRate: 24}, {Index: 1, Type: "audio", Codec: "aac", Default: true}}}
	fhd := playback.Source{Container: "mp4", DurationSeconds: 3590, Streams: []playback.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: 24}, {Index: 1, Type: "audio", Codec: "aac", Default: true}}}
	for id, source := range map[int64]playback.Source{10: uhd, 20: fhd} {
		raw, _ := json.Marshal(source)
		v := source.Streams[0]
		if _, err := db.Exec(`INSERT INTO media_technical(media_id,source_modified_unix,status,source_json,video_codec,width,height) VALUES(?,1,'ok',?,?,?,?)`, id, string(raw), v.Codec, v.Width, v.Height); err != nil {
			t.Fatal(err)
		}
	}
	req := playback.Request{ClientKind: "web", Quality: "auto", Capabilities: playback.Capabilities{Containers: []string{"mp4"}, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}, AllowRemux: true, AllowAudioCompatibility: true, AllowVideoTranscode: true}}
	return s, uhd, req
}

func TestPlaybackPlanSelectsAllowedFHDAndPreservesRequestedProgressIdentity(t *testing.T) {
	s, _, req := versionSelectionServer(t)
	resume := 42.0
	audio := 1
	body, _ := json.Marshal(playbackPlanRequest{Request: req, StartPositionSeconds: &resume, AudioStream: &audio})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/media/10/playback/plan", bytes.NewReader(body))
	r.SetPathValue("id", "10")
	r = r.WithContext(context.WithValue(r.Context(), userKey, auth.User{ID: 1, Role: "admin"}))
	w := httptest.NewRecorder()
	s.playbackPlan(w, r)
	var plan playback.Plan
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if !plan.Available || plan.Mode != playback.ModeDirectPlay || plan.MediaID != 10 || plan.SelectedMediaID != 20 || !plan.AutoSelectedVersion || plan.SelectedVersionLabel != "1080p" || plan.ResumePositionSeconds != 42 || plan.URL != "/api/v1/media/20/stream" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestPlaybackVersionSelectionRespectsPermissionsKidsAndDirectPlay(t *testing.T) {
	s, source, request := versionSelectionServer(t)
	item, err := s.media.GetStreamItem(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	id, _, _, _ := s.selectPlaybackVersion(context.Background(), 10, item, source, request, []int64{1}, profileRestriction{})
	if id != 10 {
		t.Fatal("fallback crossed a library permission boundary")
	}
	if _, err := s.db.Exec(`UPDATE media_metadata SET content_rating_age=18 WHERE media_id=20`); err != nil {
		t.Fatal(err)
	}
	id, _, _, _ = s.selectPlaybackVersion(context.Background(), 10, item, source, request, nil, profileRestriction{Restricted: true, Limit: 6})
	if id != 10 {
		t.Fatal("fallback crossed the kids restriction")
	}
	request.Capabilities.Containers = []string{"mkv", "mp4"}
	request.Capabilities.VideoCodecs = []string{"hevc", "h264"}
	id, _, _, plan := s.selectPlaybackVersion(context.Background(), 10, item, source, request, nil, profileRestriction{})
	if id != 10 || plan.Mode != playback.ModeDirectPlay {
		t.Fatalf("compatible original UHD changed: id=%d plan=%+v", id, plan)
	}
}
