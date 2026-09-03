package playback

import "strings"

// ClientCapabilities describes optional client-side decode engines that can
// consume a server-remuxed stream without video transcoding on the server.
// PlaybackPlan remains authoritative and only selects this path when native
// Direct Play is unavailable and the client explicitly advertises it.
type ClientCapabilities struct {
	Kind                string   `json:"kind,omitempty"`
	Enabled             bool     `json:"enabled"`
	WASM                bool     `json:"wasm"`
	Worker              bool     `json:"worker"`
	WebGL               bool     `json:"webgl"`
	WebGPU              bool     `json:"webgpu"`
	WebCodecs           bool     `json:"webcodecs"`
	SecureContext       bool     `json:"secure_context"`
	HEVCWASM            bool     `json:"hevc_wasm"`
	AV1WASM             bool     `json:"av1_wasm"`
	HDR                 bool     `json:"hdr"`
	MaxWidth            int      `json:"max_width,omitempty"`
	MaxHeight           int      `json:"max_height,omitempty"`
	HardwareConcurrency int      `json:"hardware_concurrency,omitempty"`
	DeviceMemoryGB      float64  `json:"device_memory_gb,omitempty"`
	Codecs              []string `json:"codecs,omitempty"`
}

// SupportsLocalDecode reports whether this client can decode the requested
// video locally through an explicitly advertised v6 engine. It intentionally
// rejects unknown clients/codecs and respects conservative device limits.
func (c ClientCapabilities) SupportsLocalDecode(codec string, width, height int, hdr string) bool {
	if !c.Enabled || !c.WASM || !c.Worker || !c.WebCodecs || !c.SecureContext {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(c.Kind))
	if kind != "web" && kind != "desktop" {
		return false
	}
	codec = normalizeCodec(codec)
	switch codec {
	case "hevc":
		if !c.HEVCWASM {
			return false
		}
	case "av1":
		if !c.AV1WASM {
			return false
		}
	default:
		return false
	}
	if c.MaxWidth > 0 && width > c.MaxWidth {
		return false
	}
	if c.MaxHeight > 0 && height > c.MaxHeight {
		return false
	}
	hdr = strings.ToLower(strings.TrimSpace(hdr))
	if hdr != "" && hdr != "sdr" && !c.HDR {
		return false
	}
	return true
}
