package games

import (
	"context"
	"testing"
)

func TestMetadataLockExcludesAutomaticCandidates(t *testing.T) {
	svc, _, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	if err := svc.SetMetadataLock(ctx, gameID, true); err != nil {
		t.Fatalf("lock metadata: %v", err)
	}
	items, err := svc.metadataCandidates(ctx, 0, true)
	if err != nil {
		t.Fatalf("metadata candidates: %v", err)
	}
	for _, item := range items {
		if item.ID == gameID {
			t.Fatal("locked metadata must not enter automatic refresh candidates")
		}
	}
	if err := svc.SetMetadataLock(ctx, gameID, false); err != nil {
		t.Fatalf("unlock metadata: %v", err)
	}
	items, err = svc.metadataCandidates(ctx, 0, true)
	if err != nil {
		t.Fatalf("metadata candidates after unlock: %v", err)
	}
	found := false
	for _, item := range items {
		found = found || item.ID == gameID
	}
	if !found {
		t.Fatal("unlocked game should return to automatic refresh candidates")
	}
}
