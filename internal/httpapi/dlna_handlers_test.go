package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestJellyfinDLNAQueryNamesAreCaseInsensitive(t *testing.T) {
	r := httptest.NewRequest("GET", "/Sessions/test/Playing?itemIds=abc&startPositionTicks=150000000&audioStreamIndex=2", nil)
	if got := jellyfinQueryValue(r, "StartPositionTicks"); got != "150000000" {
		t.Fatalf("startPositionTicks=%q", got)
	}
	if got := jellyfinQueryValue(r, "AudioStreamIndex"); got != "2" {
		t.Fatalf("audioStreamIndex=%q", got)
	}
}
