package markerdetect

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
)

const (
	maximumCreditWindowSecs = 900.0
	minimumCreditMediaSecs  = 120.0
)

// CreditFingerprint is a tail fingerprint plus its absolute media offset.
// Keeping the offset explicit lets the comparison algorithm work on small tail
// windows while the player still receives absolute timestamps.
type CreditFingerprint struct {
	Frames []Frame
	Offset float64
}

// ExtractCreditsFingerprint decodes only the tail of the first audio stream.
// Fifteen minutes is enough for long TV/anime endings while bounding remote I/O.
func ExtractCreditsFingerprint(ctx context.Context, path string, durationSeconds float64) (CreditFingerprint, error) {
	if durationSeconds < minimumCreditMediaSecs {
		return CreditFingerprint{}, errors.New("episode is too short for credit detection")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return CreditFingerprint{}, errors.New("ffmpeg is not installed")
	}
	window := math.Min(maximumCreditWindowSecs, durationSeconds/2)
	if window < 20 {
		return CreditFingerprint{}, errors.New("episode has no analyzable credit window")
	}
	offset := math.Max(0, durationSeconds-window)
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-ss", strconv.FormatFloat(offset, 'f', 3, 64),
		"-i", path,
		"-t", strconv.FormatFloat(window, 'f', 3, 64),
		"-map", "0:a:0", "-vn", "-sn", "-dn",
		"-ac", "1", "-ar", strconv.Itoa(SampleRate),
		"-f", "s16le", "pipe:1",
	)
	pcm, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return CreditFingerprint{}, ctx.Err()
		}
		return CreditFingerprint{}, fmt.Errorf("extract credit audio: %w", err)
	}
	frames := FingerprintPCM(pcm)
	if len(frames) < int(20/FrameSeconds) {
		return CreditFingerprint{}, errors.New("not enough decoded audio for credit detection")
	}
	return CreditFingerprint{Frames: frames, Offset: offset}, nil
}

// FindMatches discovers several separated repeated intervals. After each strong
// match, that interval is masked in both copies before looking again. This is the
// key post-credit safety property: credits on both sides of a unique scene become
// two markers instead of one marker that swallows the scene between them.
func FindMatches(a, b []Frame, maximum int) []Segment {
	if maximum <= 0 {
		maximum = 1
	}
	left := append([]Frame(nil), a...)
	right := append([]Frame(nil), b...)
	out := make([]Segment, 0, maximum)
	for len(out) < maximum {
		match, ok := BestMatch(left, right)
		if !ok {
			break
		}
		out = append(out, match)
		maskFrames(left, match.AStart, match.AEnd)
		maskFrames(right, match.BStart, match.BEnd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AStart < out[j].AStart })
	return out
}

func maskFrames(frames []Frame, startSeconds, endSeconds float64) {
	start := int(math.Floor(startSeconds / FrameSeconds))
	end := int(math.Ceil(endSeconds / FrameSeconds))
	padding := int(2 / FrameSeconds)
	start -= padding
	end += padding
	if start < 0 {
		start = 0
	}
	if end > len(frames) {
		end = len(frames)
	}
	for i := start; i < end; i++ {
		frames[i].Active = false
	}
}

// ConsensusAll extracts several independent consensus clusters for one episode.
// Candidate intervals must be supported by the requested number of peer episodes.
func ConsensusAll(candidates []Candidate, minimumSupport, maximum int) []Candidate {
	if maximum <= 0 {
		maximum = 1
	}
	pool := append([]Candidate(nil), candidates...)
	out := make([]Candidate, 0, maximum)
	for len(pool) > 0 && len(out) < maximum {
		candidate, ok := Consensus(pool, minimumSupport)
		if !ok {
			break
		}
		out = append(out, candidate)
		kept := pool[:0]
		for _, item := range pool {
			if math.Abs(item.Start-candidate.Start) <= 12 && math.Abs(item.End-candidate.End) <= 12 {
				continue
			}
			kept = append(kept, item)
		}
		pool = kept
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
