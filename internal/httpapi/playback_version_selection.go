package httpapi

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
	"github.com/danilostorm/stormflix/internal/transcode"
)

// selectPlaybackVersion avoids decoding a 4K source on the server when the
// same logical title/episode has a device-compatible HD/FHD physical version.
// Direct Play 4K is deliberately preserved for clients that advertise it.
func (s *server) selectPlaybackVersion(ctx context.Context, requestedID int64, item media.StreamItem, source playback.Source, request playback.Request, allowedLibraryIDs []int64, restriction profileRestriction) (int64, media.StreamItem, playback.Source, playback.Plan) {
	plan := playback.DecideForClient(source, request)
	if !playback.IsUHD(plan.VideoWidth, plan.VideoHeight) || plan.Available && plan.Mode != playback.ModeVideoTranscode {
		return requestedID, item, source, plan
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	versions, err := s.media.Versions(ctx, requestedID, allowedLibraryIDs)
	if err != nil {
		return requestedID, item, source, plan
	}
	sort.SliceStable(versions, func(i, j int) bool {
		fhd := func(v media.Version) bool {
			return v.Height > 0 && v.Height <= 1080 && !playback.IsUHD(v.Width, v.Height)
		}
		return fhd(versions[i]) && !fhd(versions[j])
	})
	type fallback struct {
		id     int64
		item   media.StreamItem
		source playback.Source
		plan   playback.Plan
		label  string
	}
	var transcoded *fallback
	probes := 0
	for _, version := range versions {
		if ctx.Err() != nil || probes >= 8 {
			break
		}
		if version.ID == requestedID || playback.IsUHD(version.Width, version.Height) {
			continue
		}
		candidate, getErr := s.media.GetStreamItem(ctx, version.ID)
		if getErr != nil || !candidate.Available {
			continue
		}
		if restriction.Restricted && !s.mediaAllowedForKids(ctx, version.ID, restriction.Limit) {
			continue
		}
		probes++
		candidateSource, probeErr := s.probeMediaSource(ctx, version.ID, candidate.Path, candidate.ModifiedUnix)
		if probeErr != nil {
			continue
		}
		candidatePlan := playback.DecideForClient(candidateSource, request)
		if !candidatePlan.Available || playback.IsUHD(candidatePlan.VideoWidth, candidatePlan.VideoHeight) {
			continue
		}
		picked := fallback{id: version.ID, item: candidate, source: candidateSource, plan: candidatePlan, label: version.Label}
		if candidatePlan.Mode != playback.ModeVideoTranscode {
			candidatePlan.RequestedMediaID = requestedID
			candidatePlan.AutoSelectedVersion = true
			candidatePlan.SelectedVersionLabel = version.Label
			return version.ID, candidate, candidateSource, candidatePlan
		}
		if transcoded == nil {
			transcoded = &picked
		}
	}
	if transcoded != nil {
		transcoded.plan.RequestedMediaID = requestedID
		transcoded.plan.AutoSelectedVersion = true
		transcoded.plan.SelectedVersionLabel = transcoded.label
		return transcoded.id, transcoded.item, transcoded.source, transcoded.plan
	}
	return requestedID, item, source, plan
}

func hardware4KEncoder(status transcode.EngineStatus, codec string, toneMap bool) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	wanted := []string{}
	switch codec {
	case "hevc", "h265":
		wanted = append(wanted, "hevc_nvenc")
		if !toneMap {
			wanted = append(wanted, "hevc_qsv", "hevc_vaapi")
		}
	case "av1":
		wanted = append(wanted, "av1_nvenc")
		if !toneMap {
			wanted = append(wanted, "av1_qsv", "av1_vaapi")
		}
	default:
		wanted = append(wanted, "h264_nvenc")
		if !toneMap {
			wanted = append(wanted, "h264_qsv", "h264_vaapi")
		}
	}
	available := map[string]bool{}
	for _, encoder := range status.VideoEncoders {
		available[encoder] = true
	}
	for _, encoder := range wanted {
		if strings.HasSuffix(encoder, "_nvenc") && status.NVIDIADevice == "" {
			continue
		}
		if (strings.HasSuffix(encoder, "_qsv") || strings.HasSuffix(encoder, "_vaapi")) && status.VAAPIDevice == "" {
			continue
		}
		if available[encoder] {
			return encoder
		}
	}
	return ""
}

func applyCPU4KTranscodeBlock(plan playback.Plan, status transcode.EngineStatus) playback.Plan {
	if !plan.Available || plan.Mode != playback.ModeVideoTranscode || !playback.IsUHD(plan.VideoWidth, plan.VideoHeight) {
		return plan
	}
	encoder := hardware4KEncoder(status, plan.VideoCodec, plan.ToneMap)
	if encoder == "" {
		plan.Available = false
		plan.Mode = playback.ModeUnsupported
		plan.VideoTranscode = false
		plan.ReasonCode = "cpu_4k_transcode_blocked"
		plan.Reason = "Esta versão 4K exigiria conversão de vídeo e não foi encontrada uma versão 1080p compatível e acessível. Para proteger o servidor, transcodificação 4K por CPU está desativada."
		plan.Encoder = ""
		plan.HardwareAcceleration = ""
		return plan
	}
	plan.Encoder = encoder
	plan.HardwareAcceleration = encoderHardware(encoder)
	return plan
}
