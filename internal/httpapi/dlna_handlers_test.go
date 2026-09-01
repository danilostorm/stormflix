package httpapi

import (
	"net/url"
	"testing"
)

func TestJellyfinDLNAQueryNamesAreCaseInsensitive(t *testing.T) {
	values := url.Values{}
	values.Set("itemIds", "abc")
	values.Set("startPositionTicks", "150000000")
	values.Set("audioStreamIndex", "2")
	if got := jellyfinQueryValue(values, "StartPositionTicks"); got != "150000000" { t.Fatalf("startPositionTicks=%q", got) }
	if got := jellyfinQueryValue(values, "AudioStreamIndex"); got != "2" { t.Fatalf("audioStreamIndex=%q", got) }
}
