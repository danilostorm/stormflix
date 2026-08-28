package playback

const (
	ModeDirectPlay         = "direct_play"
	ModeRemux              = "remux"
	ModeAudioCompatibility = "audio_compatibility"
	ModeUnsupported        = "unsupported"
)

type Capabilities struct {
	Containers              []string `json:"containers"`
	VideoCodecs             []string `json:"video_codecs"`
	AudioCodecs             []string `json:"audio_codecs"`
	AllowRemux              bool     `json:"allow_remux"`
	AllowAudioCompatibility bool     `json:"allow_audio_compatibility"`
}

type Request struct {
	ClientKind             string       `json:"client_kind"`
	ClientName             string       `json:"client_name"`
	ClientVersion          string       `json:"client_version"`
	Capabilities           Capabilities `json:"capabilities"`
	PreferredAudioLanguage string       `json:"preferred_audio_language"`
}

type Stream struct {
	Index    int    `json:"index"`
	Type     string `json:"type"`
	Codec    string `json:"codec"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default,omitempty"`
}

type Source struct {
	Container       string   `json:"container"`
	DurationSeconds float64  `json:"duration_seconds,omitempty"`
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
	AudioCodec            string  `json:"audio_codec,omitempty"`
	SourceAudioCodec      string  `json:"source_audio_codec,omitempty"`
	AudioLanguage         string  `json:"audio_language,omitempty"`
	AudioTitle            string  `json:"audio_title,omitempty"`
	AudioTrackCount       int     `json:"audio_track_count"`
	VideoStream           int     `json:"video_stream"`
	AudioStream           int     `json:"audio_stream"`
	AudioTranscode        bool    `json:"audio_transcode"`
	VideoTranscode        bool    `json:"video_transcode"`
	URL                   string  `json:"url,omitempty"`
	PrepareURL            string  `json:"prepare_url,omitempty"`
	ResumePositionSeconds float64 `json:"resume_position_seconds,omitempty"`
	PlaybackSessionID     string  `json:"playback_session_id,omitempty"`
}
