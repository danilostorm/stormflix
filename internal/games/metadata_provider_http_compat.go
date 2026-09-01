package games

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// metadataProviderTransport keeps the provider clients aligned with the current
// public API contracts while preserving the existing metadata pipeline. Only
// exact Hasheous and ScreenScraper lookup requests are touched; every other HTTP
// request is delegated unchanged.
type metadataProviderTransport struct {
	base http.RoundTripper
}

func init() {
	base := http.DefaultTransport
	if _, ok := base.(*metadataProviderTransport); ok {
		return
	}
	http.DefaultTransport = &metadataProviderTransport{base: base}
}

func (t *metadataProviderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	host := strings.ToLower(req.URL.Hostname())
	if host == "hasheous.org" && strings.HasSuffix(strings.ToLower(req.URL.Path), "/lookup/byhash") {
		return base.RoundTrip(normalizeHasheousRequest(req))
	}
	if (host == "www.screenscraper.fr" || host == "api.screenscraper.fr") && strings.EqualFold(req.URL.Path, "/api2/jeuInfos.php") {
		return base.RoundTrip(normalizeScreenScraperRequest(req))
	}
	return base.RoundTrip(req)
}

func normalizeHasheousRequest(req *http.Request) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.URL = cloneURL(req.URL)
	if !strings.Contains(strings.ToLower(clone.URL.Path), "/api/v1/") {
		clone.URL.Path = "/api/v1/Lookup/ByHash"
	}
	clone.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(clone.Header.Get("X-Client-API-Key")); key != "" {
		clone.Header.Del("X-Client-API-Key")
		clone.Header.Set("X-API-Key", key)
	}
	if req.Body == nil {
		return clone
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return clone
	}
	_ = req.Body.Close()
	var old []map[string]string
	if json.Unmarshal(raw, &old) == nil {
		fixed := make([]map[string]string, 0, len(old))
		for _, item := range old {
			out := map[string]string{}
			for key, value := range item {
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "md5":
					out["MD5"] = value
				case "sha1":
					out["SHA1"] = value
				case "sha256":
					out["SHA256"] = value
				case "crc":
					out["CRC"] = value
				}
			}
			fixed = append(fixed, out)
		}
		if encoded, marshalErr := json.Marshal(fixed); marshalErr == nil {
			raw = encoded
		}
	}
	clone.Body = io.NopCloser(bytes.NewReader(raw))
	clone.ContentLength = int64(len(raw))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	return clone
}

func normalizeScreenScraperRequest(req *http.Request) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.URL = cloneURL(req.URL)
	clone.URL.Scheme = "https"
	clone.URL.Host = "api.screenscraper.fr"
	q := clone.URL.Query()
	moveQueryValue(q, "rommd5", "md5")
	moveQueryValue(q, "romsha1", "sha1")
	moveQueryValue(q, "romcrc", "crc")
	q.Set("romtype", "rom")
	if descriptor, ok := screenScraperROMDescriptors.Load(strings.ToLower(strings.TrimSpace(q.Get("md5")))); ok {
		if value, valid := descriptor.(screenScraperROMDescriptor); valid {
			if strings.TrimSpace(value.Name) != "" {
				q.Set("romnom", value.Name)
			}
			if value.Size > 0 {
				q.Set("romtaille", strconv.FormatInt(value.Size, 10))
			}
		}
	}
	clone.URL.RawQuery = q.Encode()
	return clone
}

func moveQueryValue(q url.Values, oldKey, newKey string) {
	if value := strings.TrimSpace(q.Get(oldKey)); value != "" && strings.TrimSpace(q.Get(newKey)) == "" {
		q.Set(newKey, value)
	}
	q.Del(oldKey)
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return &url.URL{}
	}
	copy := *value
	return &copy
}
