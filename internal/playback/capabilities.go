package playback

// ClientCapabilities describes what a playback client can decode locally.
// It is intentionally independent from codec policy: PlaybackPlan remains the
// single place that decides whether to direct play, use local decode, or
// transcode.
type ClientCapabilities struct {
	Kind       string   `json:"kind,omitempty"`
	WASM       bool     `json:"wasm"`
	WebGL      bool     `json:"webgl"`
	WebGPU     bool     `json:"webgpu"`
	HEVC       bool     `json:"hevc"`
	AV1        bool     `json:"av1"`
	HDR        bool     `json:"hdr"`
	MaxWidth   int      `json:"max_width,omitempty"`
	MaxHeight  int      `json:"max_height,omitempty"`
	Codecs     []string `json:"codecs,omitempty"`
}

// SupportsLocalDecode reports whether the client can be considered a
// candidate for a future local decoder path (for example a WASM decoder).
func (c ClientCapabilities) SupportsLocalDecode() bool {
	return c.WASM && (c.WebGL || c.WebGPU)
}
