package playback

import (
	"reflect"
	"testing"
)

func TestAvailableQualitiesRespectSourceHeight(t *testing.T) {
	tests := []struct {
		name   string
		height int
		want   []string
	}{
		{"1080p source", 1080, []string{"auto", "original", "1080p", "720p", "480p"}},
		{"720p source", 720, []string{"auto", "original", "720p", "480p"}},
		{"480p source", 480, []string{"auto", "original", "480p"}},
		{"sub-480 source", 360, []string{"auto", "original"}},
		{"4K source", 2160, []string{"auto", "original", "2160p", "1440p", "1080p", "720p", "480p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := availableQualities(tt.height); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("availableQualities(%d)=%v want %v", tt.height, got, tt.want)
			}
		})
	}
}

func TestPlanNeverAdvertisesQualityAboveSource(t *testing.T) {
	req := baseRequest()
	req.Quality = "2160p"
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	want := []string{"auto", "original", "1080p", "720p", "480p"}
	if !reflect.DeepEqual(plan.AvailableQualities, want) {
		t.Fatalf("available qualities=%v want %v", plan.AvailableQualities, want)
	}
	if plan.Mode != ModeDirectPlay || !plan.Available || plan.VideoTranscode {
		t.Fatalf("a higher quality ceiling must not force upscaling/transcoding: %+v", plan)
	}
}
