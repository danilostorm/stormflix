package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

type clientMediaCaps struct {
	Explicit    bool
	MaxHeight   int
	VideoCodecs map[string]bool
	HDRKnown    bool
	HDRTypes    map[string]bool
}

func clientMediaCapsFromRequest(r *http.Request) clientMediaCaps {
	q := r.URL.Query()
	caps := clientMediaCaps{VideoCodecs: map[string]bool{}, HDRTypes: map[string]bool{}}
	if raw := strings.TrimSpace(q.Get("client_max_height")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 && value <= 4320 {
			caps.MaxHeight = value
			caps.Explicit = true
		}
	}
	if raw := strings.TrimSpace(q.Get("client_video_codecs")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			value = normalizeCatalogCodec(value)
			if value != "" {
				caps.VideoCodecs[value] = true
			}
		}
		caps.Explicit = true
	}
	if raw := strings.TrimSpace(q.Get("client_hdr_known")); raw != "" {
		caps.HDRKnown = raw == "1" || strings.EqualFold(raw, "true")
		caps.Explicit = true
	}
	if raw := strings.TrimSpace(q.Get("client_hdr_types")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				caps.HDRTypes[value] = true
			}
		}
		caps.Explicit = true
	}
	return caps
}

func normalizeCatalogCodec(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "avc", "avc1", "h.264", "h264":
		return "h264"
	case "h.265", "h265", "hevc", "hev1", "hvc1":
		return "hevc"
	case "av01", "av1":
		return "av1"
	case "vp09", "vp9":
		return "vp9"
	default:
		return value
	}
}

// Device capability hints are a presentation gate for dedicated UHD shelves,
// not a catalog access rule. General genre/search rails must keep titles visible
// because PlaybackPlan can still provide a safe compatibility transcode.
func shouldGateUHDCategoryByDevice(mode string, rules categoryRules) bool {
	return mode != "libraries" && rules.MinHeight >= 2000
}

// clientAllows4KMedia gates only UHD material. 1080p and lower catalog items
// are unaffected. Older clients that do not send explicit capability hints
// retain legacy behavior.
func clientAllows4KMedia(caps clientMediaCaps, tech technicalSnapshot) bool {
	if !caps.Explicit || tech.Status != "ok" || tech.Height < 2000 {
		return true
	}
	if caps.MaxHeight > 0 && caps.MaxHeight < 2000 {
		return false
	}
	codec := normalizeCatalogCodec(tech.VideoCodec)
	if len(caps.VideoCodecs) > 0 && codec != "" && !caps.VideoCodecs[codec] {
		return false
	}
	hdr := strings.ToLower(strings.TrimSpace(tech.HDR))
	if caps.HDRKnown && hdr != "" && hdr != "sdr" && !caps.HDRTypes[hdr] {
		return false
	}
	return true
}
