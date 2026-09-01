package markerdetect

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"os/exec"
	"sort"
	"strconv"
)

const (
	SampleRate       = 4000
	FrameSeconds     = 0.5
	frameSamples     = int(SampleRate * FrameSeconds)
	minimumIntroSecs = 20.0
	maximumIntroSecs = 480.0
)

// Frame is a small, volume-invariant spectral fingerprint of one audio window.
// The signature is intentionally cheap to compute so large remote libraries can
// be analyzed in the background without introducing another native dependency.
type Frame struct {
	Signature uint32
	Active    bool
}

// Segment describes the same repeated audio interval found in two episodes.
type Segment struct {
	AStart     float64
	AEnd       float64
	BStart     float64
	BEnd       float64
	Similarity float64
}

// Candidate is one episode-local intro interval proposed by a comparison with
// another episode from the same season.
type Candidate struct {
	Start      float64
	End        float64
	Similarity float64
}

// ExtractIntroFingerprint decodes only the beginning of the first audio stream.
// It never copies/materializes the whole media file. Analysis is capped at eight
// minutes and never goes beyond half of the episode, mirroring the safety rule
// used by mature intro detectors.
func ExtractIntroFingerprint(ctx context.Context, path string, durationSeconds float64) ([]Frame, error) {
	if durationSeconds < minimumIntroSecs*2 {
		return nil, errors.New("episode is too short for intro detection")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg is not installed")
	}
	limit := math.Min(maximumIntroSecs, durationSeconds/2)
	if limit < minimumIntroSecs {
		return nil, errors.New("episode has no analyzable intro window")
	}
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-i", path,
		"-t", strconv.FormatFloat(limit, 'f', 3, 64),
		"-map", "0:a:0", "-vn", "-sn", "-dn",
		"-ac", "1", "-ar", strconv.Itoa(SampleRate),
		"-f", "s16le", "pipe:1",
	)
	pcm, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("extract intro audio: %w", err)
	}
	frames := FingerprintPCM(pcm)
	if len(frames) < int(minimumIntroSecs/FrameSeconds) {
		return nil, errors.New("not enough decoded audio for intro detection")
	}
	return frames, nil
}

// FingerprintPCM converts mono signed-16-bit little-endian PCM at SampleRate
// into compact spectral signatures. Pairwise band comparisons make the result
// largely insensitive to codec and volume differences.
func FingerprintPCM(pcm []byte) []Frame {
	if len(pcm) < frameSamples*2 {
		return nil
	}
	samples := make([]float64, len(pcm)/2)
	for i := range samples {
		samples[i] = float64(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768.0
	}
	out := make([]Frame, 0, len(samples)/frameSamples)
	for start := 0; start+frameSamples <= len(samples); start += frameSamples {
		out = append(out, fingerprintWindow(samples[start:start+frameSamples]))
	}
	return out
}

var fingerprintFrequencies = [...]float64{120, 180, 260, 360, 480, 620, 800, 1000, 1240, 1500, 1760}

var fingerprintPairs = [...][2]int{
	{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {6, 7}, {7, 8}, {8, 9}, {9, 10},
	{0, 4}, {1, 5}, {2, 6}, {3, 7}, {4, 8}, {5, 9}, {6, 10},
	{0, 7}, {1, 8}, {2, 9}, {3, 10}, {0, 10}, {2, 8}, {4, 10},
}

func fingerprintWindow(samples []float64) Frame {
	mean := 0.0
	for _, v := range samples {
		mean += v
	}
	mean /= float64(len(samples))
	rms := 0.0
	for _, v := range samples {
		d := v - mean
		rms += d * d
	}
	rms = math.Sqrt(rms / float64(len(samples)))
	if rms < 0.0045 {
		return Frame{}
	}
	energies := make([]float64, len(fingerprintFrequencies))
	for i, frequency := range fingerprintFrequencies {
		energies[i] = math.Log1p(goertzelPower(samples, mean, frequency))
	}
	var signature uint32
	for bit, pair := range fingerprintPairs {
		if energies[pair[0]] > energies[pair[1]] {
			signature |= 1 << bit
		}
	}
	return Frame{Signature: signature, Active: true}
}

func goertzelPower(samples []float64, mean, frequency float64) float64 {
	omega := 2 * math.Pi * frequency / SampleRate
	coeff := 2 * math.Cos(omega)
	q0, q1, q2 := 0.0, 0.0, 0.0
	for _, sample := range samples {
		q0 = (sample-mean) + coeff*q1 - q2
		q2, q1 = q1, q0
	}
	return q1*q1 + q2*q2 - coeff*q1*q2
}

func similar(a, b Frame) bool {
	if !a.Active || !b.Active {
		return false
	}
	return bits.OnesCount32(a.Signature^b.Signature) <= 5
}

// BestMatch searches every temporal offset for a repeated segment. Short
// coincidences and silence are ignored. Small discontinuities are tolerated to
// survive dialogue overlays, edits and lossy re-encoding.
func BestMatch(a, b []Frame) (Segment, bool) {
	minimumFrames := int(minimumIntroSecs / FrameSeconds)
	if len(a) < minimumFrames || len(b) < minimumFrames {
		return Segment{}, false
	}
	best := Segment{}
	bestFrames := 0
	for offset := -len(b) + minimumFrames; offset <= len(a)-minimumFrames; offset++ {
		startA, startB := 0, 0
		if offset >= 0 {
			startA = offset
		} else {
			startB = -offset
		}
		overlap := minInt(len(a)-startA, len(b)-startB)
		runStart, lastGood, good, gap := -1, -1, 0, 0
		evaluate := func() {
			if runStart < 0 || lastGood < runStart {
				return
			}
			span := lastGood - runStart + 1
			if span < minimumFrames {
				return
			}
			ratio := float64(good) / float64(span)
			if ratio < 0.78 {
				return
			}
			if span > bestFrames || (span == bestFrames && ratio > best.Similarity) {
				bestFrames = span
				best = Segment{
					AStart: float64(startA+runStart) * FrameSeconds,
					AEnd:   float64(startA+lastGood+1) * FrameSeconds,
					BStart: float64(startB+runStart) * FrameSeconds,
					BEnd:   float64(startB+lastGood+1) * FrameSeconds,
					Similarity: ratio,
				}
			}
		}
		for k := 0; k < overlap; k++ {
			if similar(a[startA+k], b[startB+k]) {
				if runStart < 0 {
					runStart = k
					good = 0
				}
				good++
				lastGood = k
				gap = 0
				continue
			}
			if runStart < 0 {
				continue
			}
			gap++
			if gap > 4 {
				evaluate()
				runStart, lastGood, good, gap = -1, -1, 0, 0
			}
		}
		evaluate()
	}
	if bestFrames == 0 {
		return Segment{}, false
	}
	return best, true
}

// Consensus groups pairwise matches for one episode and returns the strongest
// repeated interval. A season with >=3 usable episodes should require support
// from at least two peers; two-episode seasons can use one peer.
func Consensus(candidates []Candidate, minimumSupport int) (Candidate, bool) {
	if minimumSupport < 1 {
		minimumSupport = 1
	}
	bestCluster := []Candidate{}
	for _, seed := range candidates {
		cluster := []Candidate{}
		for _, candidate := range candidates {
			if math.Abs(seed.Start-candidate.Start) <= 12 && math.Abs(seed.End-candidate.End) <= 12 {
				cluster = append(cluster, candidate)
			}
		}
		if len(cluster) > len(bestCluster) {
			bestCluster = cluster
		} else if len(cluster) == len(bestCluster) && meanSimilarity(cluster) > meanSimilarity(bestCluster) {
			bestCluster = cluster
		}
	}
	if len(bestCluster) < minimumSupport {
		return Candidate{}, false
	}
	starts := make([]float64, len(bestCluster))
	ends := make([]float64, len(bestCluster))
	for i, candidate := range bestCluster {
		starts[i], ends[i] = candidate.Start, candidate.End
	}
	sort.Float64s(starts)
	sort.Float64s(ends)
	result := Candidate{Start: median(starts), End: median(ends), Similarity: meanSimilarity(bestCluster)}
	if result.End-result.Start < minimumIntroSecs {
		return Candidate{}, false
	}
	return result, true
}

func meanSimilarity(candidates []Candidate) float64 {
	if len(candidates) == 0 {
		return 0
	}
	total := 0.0
	for _, candidate := range candidates {
		total += candidate.Similarity
	}
	return total / float64(len(candidates))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
