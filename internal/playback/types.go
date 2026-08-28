package playback

const (
	ModeDirectPlay         = "direct_play"
	ModeRemux              = "remux"
	ModeAudioCompatibility = "audio_compatibility"
	ModeUnsupported        = "unsupported"
)

type VideoProfile struct {
	Codec        string   `json:"codec"`
	MaxWidth     int      `json:"max_width,omitempty"`
	MaxHeight    int      `json:"max_height,omitempty"`
	MaxFrameRate float64  `json:"max_frame_rate,omitempty"`
	HDRKnown     bool     `json:"hdr_known,omitempty"`
	HDRTypes     []string `json:"hdr_types,omitempty"`
}

type Capabilities struct {
	Containers                []string       `json:"containers"`
	VideoCodecs               []string       `json:"video_codecs"`
	AudioCodecs               []string       `json:"audio_codecs"`
	VideoProfiles             []VideoProfile `json:"video_profiles,omitempty"`
	SubtitleFormats           []string       `json:"subtitle_formats,omitempty"`
	AudioPassthrough          []string       `json:"audio_passthrough,omitempty"`
	AllowRemux                bool           `json:"allow_remux"`
	AllowAudioCompatibility   bool           `json:"allow_audio_compatibility"`
	NativeAudioTrackSelection bool           `json:"native_audio_track_selection"`
	DirectPlayMaxBitrateKbps  int64          `json:"direct_play_max_bitrate_kbps,omitempty"`
	PictureInPicture          bool           `json:"picture_in_picture,omitempty"`
	MediaSession              bool           `json:"media_session,omitempty"`
}

type Request struct {
	ClientKind             string       `json:"client_kind"`
	ClientName             string       `json:"client_name"`
	ClientVersion          string       `json:"client_version"`
	PlaybackSessionID      string       `json:"playback_session_id,omitempty"`
	Capabilities           Capabilities `json:"capabilities"`
	PreferredAudioLanguage string       `json:"preferred_audio_language"`
}

type Stream struct {
	Index       int     `json:"index"`
	Type        string  `json:"type"`
	Codec       string  `json:"codec"`
	Language    string  `json:"language,omitempty"`
	Title       string  `json:"title,omitempty"`
	Default     bool    `json:"default,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	FrameRate   float64 `json:"frame_rate,omitempty"`
	HDR         string  `json:"hdr,omitempty"`
	BitrateKbps int64   `json:"bitrate_kbps,omitempty"`
}

type Source struct {
	Container       string   `json:"container"`
	DurationSeconds float64  `json:"duration_seconds,omitempty"`
	BitrateKbps     int64    `json:"bitrate_kbps,omitempty"`
	Streams         []Stream `json:"streams"`
}

type Plan struct {
	Available             bool    `json:"available"`
	Mode                  string  `json:"mode"`
	ReasonCode            string  `json:"reason_code"`
	Reason                string  `json:"reason"`
	MediaID               int64   `json:"media_id,omitempty"`
	ClientKind            string  `json:"client_kind,omitempty"`
	SourceContainer       string  `json:"source_container"`
	Container             string  `json:"container"`
	VideoCodec            string  `json:"video_codec"`
	VideoWidth            int     `json:"video_width,omitempty"`
	VideoHeight           int     `json:"video_height,omitempty"`
	VideoFrameRate        float64 `json:"video_frame_rate,omitempty"`
	VideoHDR              string  `json:"video_hdr,omitempty"`
	SourceBitrateKbps     int64   `json:"source_bitrate_kbps,omitempty"`
	AudioCodec            string  `json:"audio_codec,omitempty"`
	SourceAudioCodec      string  `json:"source_audio_codec,omitempty"`
	AudioLanguage         string  `json:"audio_language,omitempty"`
	AudioTitle            string  `json:"audio_title,omitempty"`
	AudioTrackCount       int     `json:"audio_track_count"`
	VideoStream           int     `json:"video_stream"`
	AudioStream           int     `json:"audio_stream"`
	AudioTranscode        bool    `json:"audio_transcode"`
	VideoTranscode        bool    `json:"video_transcode"`
	ClientSelectsAudio    bool    `json:"client_selects_audio"`
	URL                   string  `json:"url,omitempty"`
	PrepareURL            string  `json:"prepare_url,omitempty"`
	ResumePositionSeconds float64 `json:"resume_position_seconds,omitempty"`
	PlaybackSessionID     string  `json:"playback_session_id,omitempty"`
}
