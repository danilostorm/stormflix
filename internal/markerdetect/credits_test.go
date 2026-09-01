package markerdetect

import "testing"

func creditFrames(seed uint32, n int) []Frame {
	out := make([]Frame, n)
	for i := range out {
		out[i] = Frame{Signature: seed ^ uint32((i+1)*2654435761), Active: true}
	}
	return out
}

func TestFindMatchesKeepsPostCreditGapSeparate(t *testing.T) {
	creditsA := creditFrames(0x00112233, 100) // 50 seconds
	creditsB := creditFrames(0x00445566, 80)  // 40 seconds
	sceneA := creditFrames(0x0000aa11, 70)    // unique 35 second scene
	sceneB := creditFrames(0x0000bb22, 70)

	a := append(creditFrames(0x100, 40), creditsA...)
	a = append(a, sceneA...)
	a = append(a, creditsB...)
	b := append(creditFrames(0x200, 55), creditsA...)
	b = append(b, sceneB...)
	b = append(b, creditsB...)

	matches := FindMatches(a, b, 4)
	if len(matches) != 2 {
		t.Fatalf("expected two separated credit matches, got %d: %+v", len(matches), matches)
	}
	if matches[0].AEnd >= matches[1].AStart {
		t.Fatalf("credit markers overlap the post-credit scene: %+v", matches)
	}
	gap := matches[1].AStart - matches[0].AEnd
	if gap < 30 {
		t.Fatalf("post-credit scene gap unexpectedly short: %.1fs", gap)
	}
}

func TestConsensusAllReturnsSeparatedCreditBlocks(t *testing.T) {
	candidates := []Candidate{
		{Start: 2200, End: 2260, Similarity: .91},
		{Start: 2202, End: 2261, Similarity: .89},
		{Start: 2320, End: 2390, Similarity: .94},
		{Start: 2321, End: 2391, Similarity: .92},
		{Start: 2100, End: 2130, Similarity: .99},
	}
	got := ConsensusAll(candidates, 2, 4)
	if len(got) != 2 {
		t.Fatalf("expected two supported blocks, got %d: %+v", len(got), got)
	}
	if got[0].Start < 2199 || got[0].Start > 2203 || got[1].Start < 2319 || got[1].Start > 2322 {
		t.Fatalf("unexpected credit consensus: %+v", got)
	}
}
