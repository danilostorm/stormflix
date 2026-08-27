package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestJellyfinQueryValueIsCaseInsensitive(t *testing.T) {
	req := httptest.NewRequest("GET", "/Items?parentId=lib8&includeItemTypes=Movie&limit=25", nil)
	cases := map[string]string{
		"ParentId":         "lib8",
		"parentid":         "lib8",
		"IncludeItemTypes": "Movie",
		"LIMIT":            "25",
	}
	for key, want := range cases {
		if got := jellyfinQueryValue(req, key); got != want {
			t.Fatalf("jellyfinQueryValue(%q)=%q want %q", key, got, want)
		}
	}
}

func TestJellyfinCatalogItemsNormalizesAndroidTVCamelCase(t *testing.T) {
	req := httptest.NewRequest("GET", "/Items?parentId=lib8&includeItemTypes=Movie&searchTerm=dragon", nil)
	query := req.URL.Query()
	for _, key := range []string{"ParentId", "SearchTerm", "IncludeItemTypes"} {
		if query.Get(key) == "" {
			if value := jellyfinQueryValue(req, key); value != "" {
				query.Set(key, value)
			}
		}
	}
	req.URL.RawQuery = query.Encode()

	if got := req.URL.Query().Get("ParentId"); got != "lib8" {
		t.Fatalf("ParentId=%q", got)
	}
	if got := req.URL.Query().Get("IncludeItemTypes"); got != "Movie" {
		t.Fatalf("IncludeItemTypes=%q", got)
	}
	if got := req.URL.Query().Get("SearchTerm"); got != "dragon" {
		t.Fatalf("SearchTerm=%q", got)
	}
}
