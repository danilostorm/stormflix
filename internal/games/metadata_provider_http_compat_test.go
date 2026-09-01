package games

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn providerRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func providerOKResponse(req *http.Request) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}
}

func TestHasheousRequestUsesOfficialHeaderAndHashNames(t *testing.T) {
	var captured *http.Request
	transport := &metadataProviderTransport{base: providerRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		return providerOKResponse(req), nil
	})}
	req, err := http.NewRequest(http.MethodPost, "https://hasheous.org/api/v1/Lookup/ByHash?returnAllSources=true", strings.NewReader(`[{"mD5":"aa","shA1":"bb","crc":"CC"}]`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json-patch+json")
	req.Header.Set("X-Client-API-Key", "secret")
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if captured == nil {
		t.Fatal("request not captured")
	}
	if got := captured.Header.Get("X-API-Key"); got != "secret" {
		t.Fatalf("X-API-Key=%q", got)
	}
	if captured.Header.Get("X-Client-API-Key") != "" {
		t.Fatal("legacy Hasheous header must be removed")
	}
	if got := captured.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	body, _ := io.ReadAll(captured.Body)
	var payload []map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) != 1 || payload[0]["MD5"] != "aa" || payload[0]["SHA1"] != "bb" || payload[0]["CRC"] != "CC" {
		t.Fatalf("unexpected Hasheous payload: %#v", payload)
	}
}

func TestScreenScraperRequestUsesOfficialGameLookupContract(t *testing.T) {
	const md5Value = "0123456789abcdef"
	screenScraperROMDescriptors.Store(md5Value, screenScraperROMDescriptor{Name: "Sonic 2.bin", Size: 749652})
	defer screenScraperROMDescriptors.Delete(md5Value)

	var captured *http.Request
	transport := &metadataProviderTransport{base: providerRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		return providerOKResponse(req), nil
	})}
	req, err := http.NewRequest(http.MethodGet, "https://www.screenscraper.fr/api2/jeuInfos.php?devid=dev&devpassword=pass&softname=StormFlix&rommd5="+md5Value+"&romsha1=sha&romcrc=ABCDEF12&romnom=Sonic", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if captured == nil {
		t.Fatal("request not captured")
	}
	if captured.URL.Host != "api.screenscraper.fr" {
		t.Fatalf("host=%q", captured.URL.Host)
	}
	q := captured.URL.Query()
	if q.Get("md5") != md5Value || q.Get("sha1") != "sha" || q.Get("crc") != "ABCDEF12" {
		t.Fatalf("official hashes missing: %s", captured.URL.RawQuery)
	}
	if q.Get("romtype") != "rom" || q.Get("romnom") != "Sonic 2.bin" || q.Get("romtaille") != "749652" {
		t.Fatalf("ROM descriptor missing: %s", captured.URL.RawQuery)
	}
	for _, legacy := range []string{"rommd5", "romsha1", "romcrc"} {
		if q.Get(legacy) != "" {
			t.Fatalf("legacy query key %s survived", legacy)
		}
	}
}
