package music

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type probeResult struct {
	Title               string
	Artist              string
	AlbumArtist         string
	Album               string
	TrackNumber         int
	DiscNumber          int
	Year                int
	Genre               string
	DurationSeconds     float64
	Codec               string
	Bitrate             int
	SampleRate          int
	Channels            int
	MusicBrainzTrackID  string
	MusicBrainzAlbumID  string
	MusicBrainzArtistID string
}

type ffprobeResponse struct {
	Streams []struct {
		CodecType  string            `json:"codec_type"`
		CodecName  string            `json:"codec_name"`
		SampleRate string            `json:"sample_rate"`
		Channels   int               `json:"channels"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func probeTrack(ctx context.Context, path string) (probeResult, error) {
	binary, err := exec.LookPath("ffprobe")
	if err != nil {
		return probeResult{}, err
	}
	cmd := exec.CommandContext(ctx, binary,
		"-v", "error",
		"-show_entries", "format=duration,bit_rate:format_tags:stream=codec_type,codec_name,sample_rate,channels,bit_rate:stream_tags",
		"-of", "json",
		path,
	)
	data, err := cmd.Output()
	if err != nil {
		return probeResult{}, fmt.Errorf("ffprobe audio metadata: %w", err)
	}
	var parsed ffprobeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return probeResult{}, err
	}
	tags := lowerTags(parsed.Format.Tags)
	for _, stream := range parsed.Streams {
		if stream.CodecType == "audio" {
			for key, value := range lowerTags(stream.Tags) {
				if _, exists := tags[key]; !exists || strings.TrimSpace(tags[key]) == "" {
					tags[key] = value
				}
			}
		}
	}

	out := probeResult{
		Title:               tag(tags, "title"),
		Artist:              firstNonEmpty(tag(tags, "artist"), tag(tags, "performer")),
		AlbumArtist:         firstNonEmpty(tag(tags, "album_artist"), tag(tags, "albumartist"), tag(tags, "album artist")),
		Album:               tag(tags, "album"),
		TrackNumber:         tagNumber(firstNonEmpty(tag(tags, "track"), tag(tags, "tracknumber"), tag(tags, "track_number"))),
		DiscNumber:          tagNumber(firstNonEmpty(tag(tags, "disc"), tag(tags, "discnumber"), tag(tags, "disc_number"))),
		Year:                parseYear(firstNonEmpty(tag(tags, "date"), tag(tags, "year"), tag(tags, "originaldate"))),
		Genre:               tag(tags, "genre"),
		MusicBrainzTrackID:  firstNonEmpty(tag(tags, "musicbrainz_trackid"), tag(tags, "musicbrainz_track_id"), tag(tags, "musicbrainz recording id")),
		MusicBrainzAlbumID:  firstNonEmpty(tag(tags, "musicbrainz_albumid"), tag(tags, "musicbrainz_album_id"), tag(tags, "musicbrainz release id")),
		MusicBrainzArtistID: firstNonEmpty(tag(tags, "musicbrainz_artistid"), tag(tags, "musicbrainz_artist_id"), tag(tags, "musicbrainz artist id")),
	}
	out.DurationSeconds, _ = strconv.ParseFloat(parsed.Format.Duration, 64)
	out.Bitrate = atoi(parsed.Format.BitRate)
	for _, stream := range parsed.Streams {
		if stream.CodecType != "audio" {
			continue
		}
		out.Codec = stream.CodecName
		out.SampleRate = atoi(stream.SampleRate)
		out.Channels = stream.Channels
		if bitrate := atoi(stream.BitRate); bitrate > 0 {
			out.Bitrate = bitrate
		}
		break
	}
	return out, nil
}

func lowerTags(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return out
}

func tag(tags map[string]string, key string) string {
	return strings.TrimSpace(tags[strings.ToLower(strings.TrimSpace(key))])
}

func tagNumber(value string) int {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, '/'); i >= 0 {
		value = value[:i]
	}
	return atoi(value)
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}
