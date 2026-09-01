package markerdetect

import "testing"

func TestBestMatchFindsRepeatedIntroAtDifferentOffsets(t *testing.T) {
	noise := func(seed uint32, n int) []Frame {
		out := make([]Frame, n)
		for i := range out {
			out[i] = Frame{Signature: seed + uint32(i*7919), Active: true}
		}
		return out
	}
	intro := make([]Frame, 120) // 60 seconds
	for i := range intro {
		intro[i] = Frame{Signature: uint32((i*2654435761)&0x00ffffff) ^ 0x0055aa55, Active: true}
	}
	a := append(noise(100, 36), intro...)
	a = append(a, noise(9000, 80)...)
	b := append(noise(400, 74), intro...)
	b = append(b, noise(12000, 30)...)

	match, ok := BestMatch(a, b)
	if !ok {
		t.Fatal("expected repeated intro")
	}
	if match.AStart < 17 || match.AStart > 19 {
		t.Fatalf("unexpected A start %.1f", match.AStart)
	}
	if match.BStart < 36 || match.BStart > 38 {
		t.Fatalf("unexpected B start %.1f", match.BStart)
	}
	if match.AEnd-match.AStart < 58 {
		t.Fatalf("intro too short: %.1fs", match.AEnd-match.AStart)
	}
}

func TestBestMatchIgnoresShortRepeatedLogo(t *testing.T) {
	common := make([]Frame, 20) // 10 seconds, below the 20s floor
	for i := range common {
		common[i] = Frame{Signature: uint32(i*97 + 11), Active: true}
	}
	a := append([]Frame{}, common...)
	b := append([]Frame{}, common...)
	a = append(a, make([]Frame, 80)...)
	b = append(b, make([]Frame, 80)...)
	if _, ok := BestMatch(a, b); ok {
		t.Fatal("short repeated segment must not become an intro")
	}
}

func TestConsensusRequiresPeerSupport(t *testing.T) {
	candidates := []Candidate{
		{Start: 42, End: 132, Similarity: .92},
		{Start: 43, End: 133, Similarity: .89},
		{Start: 180, End: 225, Similarity: .99},
	}
	got, ok := Consensus(candidates, 2)
	if !ok {
		t.Fatal("expected consensus")
	}
	if got.Start < 42 || got.Start > 43 || got.End < 132 || got.End > 133 {
		t.Fatalf("unexpected consensus %+v", got)
	}
	if _, ok := Consensus(candidates[:1], 2); ok {
		t.Fatal("single peer must not satisfy two-peer consensus")
	}
}
