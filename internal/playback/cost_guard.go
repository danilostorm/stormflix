package playback

import "strings"

const automatic4KFallbackHeight = 1080

// applyAutomatic4KCostGuard keeps compatible UHD on Direct Play, but prevents
// an automatic compatibility transcode from spending server resources on a
// 4K->4K encode. Auto and Original both mean "do not intentionally downshift";
// if the source is incompatible and must be re-encoded anyway, the safe live
// compatibility ceiling is 1080p. An explicit 2160p choice remains authoritative.
func applyAutomatic4KCostGuard(plan Plan) Plan {
	if !plan.Available || plan.Mode != ModeVideoTranscode || !plan.VideoTranscode {
		return plan
	}
	quality := normalizeQuality(plan.Quality)
	if (quality != "auto" && quality != "original") || plan.VideoHeight < 2000 {
		return plan
	}
	targetHeight := plan.TargetVideoHeight
	if targetHeight <= 0 {
		targetHeight = plan.VideoHeight
	}
	if targetHeight <= automatic4KFallbackHeight {
		return plan
	}
	targetWidth := plan.TargetVideoWidth
	if targetWidth <= 0 {
		targetWidth = plan.VideoWidth
	}
	if targetWidth > 0 && targetHeight > 0 {
		targetWidth = even(int(float64(targetWidth) * float64(automatic4KFallbackHeight) / float64(targetHeight)))
	}
	plan.TargetVideoWidth = targetWidth
	plan.TargetVideoHeight = automatic4KFallbackHeight
	if plan.TargetBitrateKbps <= 0 || plan.TargetBitrateKbps > 8000 {
		plan.TargetBitrateKbps = 8000
	}
	if !containsNormalized(plan.TranscodeReasons, "auto_4k_cost_guard") {
		plan.TranscodeReasons = append(plan.TranscodeReasons, "auto_4k_cost_guard")
	}
	if !strings.Contains(plan.Reason, "automatic 4K cost guard") {
		plan.Reason += "; automatic 4K cost guard limits compatibility transcoding to 1080p while Direct Play remains full resolution"
	}
	return plan
}
