package webui

import (
	"bytes"
	"testing"
)

func TestGamesMetadataDoesNotRefreshFromMutationObserver(t *testing.T) {
	script, err := Static.ReadFile("static/admin/admin-games-metadata.js")
	if err != nil {
		t.Fatalf("read Games metadata script: %v", err)
	}
	if bytes.Contains(script, []byte("MutationObserver")) {
		t.Fatal("Games metadata refresh must not be driven by DOM mutations; it can recurse on its own render")
	}
	if !bytes.Contains(script, []byte("setTimeout(enhance,2200)")) {
		t.Fatal("expected bounded polling only while a metadata job is active")
	}
}
