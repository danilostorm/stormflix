package dlna

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ssdpAddress = "239.255.255.250:1900"
	mediaRendererST = "urn:schemas-upnp-org:device:MediaRenderer:1"
	avTransportType = "urn:schemas-upnp-org:service:AVTransport:1"
)

type Device struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Manufacturer          string `json:"manufacturer,omitempty"`
	Model                 string `json:"model,omitempty"`
	Location              string `json:"location,omitempty"`
	AVTransportControlURL string `json:"-"`
	RenderingControlURL   string `json:"-"`
}

type Manager struct {
	mu       sync.RWMutex
	devices  map[string]Device
	lastScan time.Time
}

func NewManager() *Manager {
	return &Manager{devices: map[string]Device{}}
}

func (m *Manager) Discover(ctx context.Context, force bool) ([]Device, error) {
	m.mu.RLock()
	if !force && time.Since(m.lastScan) < 8*time.Second && len(m.devices) > 0 {
		out := cloneDevices(m.devices)
		m.mu.RUnlock()
		return out, nil
	}
	m.mu.RUnlock()

	devices, err := discover(ctx, 1500*time.Millisecond)
	if err != nil {
		m.mu.RLock()
		out := cloneDevices(m.devices)
		m.mu.RUnlock()
		if len(out) > 0 {
			return out, nil
		}
		return nil, err
	}
	m.mu.Lock()
	m.devices = make(map[string]Device, len(devices))
	for _, device := range devices {
		m.devices[device.ID] = device
	}
	m.lastScan = time.Now()
	m.mu.Unlock()
	return devices, nil
}

func (m *Manager) Find(ctx context.Context, id string) (Device, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Device{}, errors.New("DLNA device id is required")
	}
	m.mu.RLock()
	device, ok := m.devices[id]
	fresh := time.Since(m.lastScan) < 30*time.Second
	m.mu.RUnlock()
	if ok && fresh {
		return device, nil
	}
	devices, err := m.Discover(ctx, true)
	if err != nil {
		return Device{}, err
	}
	for _, candidate := range devices {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return Device{}, errors.New("DLNA renderer is no longer available")
}

func cloneDevices(in map[string]Device) []Device {
	out := make([]Device, 0, len(in))
	for _, device := range in {
		out = append(out, device)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func discover(ctx context.Context, wait time.Duration) ([]Device, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("open SSDP socket: %w", err)
	}
	defer conn.Close()
	remote, err := net.ResolveUDPAddr("udp4", ssdpAddress)
	if err != nil {
		return nil, err
	}
	for _, st := range []string{mediaRendererST, "ssdp:all"} {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: " + ssdpAddress + "\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\n\r\n"
		_, _ = conn.WriteToUDP([]byte(msg), remote)
	}

	deadline := time.Now().Add(wait)
	locations := map[string]net.IP{}
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			break
		}
		_ = conn.SetReadDeadline(minTime(deadline, time.Now().Add(250*time.Millisecond)))
		n, addr, readErr := conn.ReadFromUDP(buffer)
		if ne, ok := readErr.(net.Error); ok && ne.Timeout() {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("read SSDP response: %w", readErr)
		}
		if addr == nil || !isPrivateIP(addr.IP) {
			continue
		}
		headers := parseSSDP(buffer[:n])
		location := strings.TrimSpace(headers["location"])
		if location == "" {
			continue
		}
		if _, ok := locations[location]; !ok {
			locations[location] = append(net.IP(nil), addr.IP...)
		}
	}

	devices := make([]Device, 0, len(locations))
	seen := map[string]bool{}
	for location, sourceIP := range locations {
		device, err := fetchDevice(ctx, location, sourceIP)
		if err != nil || device.AVTransportControlURL == "" || seen[device.ID] {
			continue
		}
		seen[device.ID] = true
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool { return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name) })
	return devices, nil
}

func parseSSDP(raw []byte) map[string]string {
	headers := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return headers
}

type rootDescription struct {
	Device upnpDevice `xml:"device"`
}

type upnpDevice struct {
	DeviceType   string        `xml:"deviceType"`
	FriendlyName string        `xml:"friendlyName"`
	Manufacturer string        `xml:"manufacturer"`
	ModelName    string        `xml:"modelName"`
	UDN          string        `xml:"UDN"`
	Services     []upnpService `xml:"serviceList>service"`
	Devices      []upnpDevice  `xml:"deviceList>device"`
}

type upnpService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

func fetchDevice(ctx context.Context, location string, sourceIP net.IP) (Device, error) {
	parsed, err := url.Parse(strings.TrimSpace(location))
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
		return Device{}, errors.New("invalid UPnP description URL")
	}
	if !urlIsPrivate(parsed) {
		return Device{}, errors.New("UPnP description is outside the local network")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	resp, err := client.Do(req)
	if err != nil {
		return Device{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Device{}, fmt.Errorf("UPnP description returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return Device{}, err
	}
	var root rootDescription
	if err := xml.Unmarshal(data, &root); err != nil {
		return Device{}, err
	}
	device, ok := findRenderer(root.Device)
	if !ok {
		return Device{}, errors.New("UPnP device is not a MediaRenderer")
	}
	av := serviceControlURL(device.Services, "AVTransport")
	if av == "" {
		return Device{}, errors.New("MediaRenderer does not expose AVTransport")
	}
	avURL, err := resolveControlURL(parsed, av)
	if err != nil {
		return Device{}, err
	}
	rendering := serviceControlURL(device.Services, "RenderingControl")
	renderingURL := ""
	if rendering != "" {
		if resolved, resolveErr := resolveControlURL(parsed, rendering); resolveErr == nil {
			renderingURL = resolved
		}
	}
	id := strings.TrimPrefix(strings.TrimSpace(device.UDN), "uuid:")
	if id == "" {
		sum := sha256.Sum256([]byte(parsed.String()))
		id = hex.EncodeToString(sum[:12])
	}
	name := strings.TrimSpace(device.FriendlyName)
	if name == "" {
		name = strings.TrimSpace(device.ModelName)
	}
	if name == "" {
		name = sourceIP.String()
	}
	return Device{
		ID: id, Name: name, Manufacturer: strings.TrimSpace(device.Manufacturer), Model: strings.TrimSpace(device.ModelName),
		Location: parsed.String(), AVTransportControlURL: avURL, RenderingControlURL: renderingURL,
	}, nil
}

func findRenderer(device upnpDevice) (upnpDevice, bool) {
	if strings.Contains(device.DeviceType, ":device:MediaRenderer:") {
		return device, true
	}
	for _, child := range device.Devices {
		if found, ok := findRenderer(child); ok {
			return found, true
		}
	}
	return upnpDevice{}, false
}

func serviceControlURL(services []upnpService, name string) string {
	needle := ":service:" + name + ":"
	for _, service := range services {
		if strings.Contains(service.ServiceType, needle) {
			return strings.TrimSpace(service.ControlURL)
		}
	}
	return ""
}

func resolveControlURL(base *url.URL, raw string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" || !urlIsPrivate(resolved) {
		return "", errors.New("UPnP control URL is outside the local network")
	}
	return resolved.String(), nil
}

func urlIsPrivate(value *url.URL) bool {
	if value == nil || value.Hostname() == "" {
		return false
	}
	if ip := net.ParseIP(value.Hostname()); ip != nil {
		return isPrivateIP(ip)
	}
	ips, err := net.LookupIP(value.Hostname())
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !isPrivateIP(ip) {
			return false
		}
	}
	return true
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLoopback()
}

func (m *Manager) Play(ctx context.Context, device Device, mediaURL, title, mime string, startSeconds float64) error {
	if strings.TrimSpace(mediaURL) == "" {
		return errors.New("media URL is required")
	}
	if strings.TrimSpace(device.AVTransportControlURL) == "" {
		return errors.New("renderer does not expose AVTransport")
	}
	metadata := didlMetadata(mediaURL, title, mime)
	if err := soap(ctx, device.AVTransportControlURL, avTransportType, "SetAVTransportURI", map[string]string{
		"InstanceID": "0", "CurrentURI": mediaURL, "CurrentURIMetaData": metadata,
	}); err != nil {
		return err
	}
	if err := soap(ctx, device.AVTransportControlURL, avTransportType, "Play", map[string]string{"InstanceID": "0", "Speed": "1"}); err != nil {
		return err
	}
	if startSeconds > 1 {
		time.Sleep(220 * time.Millisecond)
		_ = soap(ctx, device.AVTransportControlURL, avTransportType, "Seek", map[string]string{
			"InstanceID": "0", "Unit": "REL_TIME", "Target": formatDuration(startSeconds),
		})
	}
	return nil
}

func (m *Manager) Control(ctx context.Context, device Device, command string, positionSeconds float64) error {
	command = strings.ToLower(strings.TrimSpace(command))
	switch command {
	case "play", "resume", "unpause":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Play", map[string]string{"InstanceID": "0", "Speed": "1"})
	case "pause":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Pause", map[string]string{"InstanceID": "0"})
	case "stop":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Stop", map[string]string{"InstanceID": "0"})
	case "seek":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Seek", map[string]string{"InstanceID": "0", "Unit": "REL_TIME", "Target": formatDuration(positionSeconds)})
	default:
		return fmt.Errorf("unsupported DLNA command %q", command)
	}
}

func soap(ctx context.Context, controlURL, serviceType, action string, args map[string]string) error {
	parsed, err := url.Parse(strings.TrimSpace(controlURL))
	if err != nil || parsed.Scheme != "http" || !urlIsPrivate(parsed) {
		return errors.New("unsafe DLNA control URL")
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	body.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	body.WriteString(`<u:` + action + ` xmlns:u="` + html.EscapeString(serviceType) + `">`)
	keys := make([]string, 0, len(args))
	for key := range args { keys = append(keys, key) }
	sort.Strings(keys)
	for _, key := range keys {
		body.WriteString("<" + key + ">" + html.EscapeString(args[key]) + "</" + key + ">")
	}
	body.WriteString(`</u:` + action + `></s:Body></s:Envelope>`)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(body.String()))
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+serviceType+`#`+action+`"`)
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DLNA %s failed: %w", action, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DLNA %s returned HTTP %d: %s", action, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func didlMetadata(mediaURL, title, mime string) string {
	mime = strings.TrimSpace(mime)
	if mime == "" { mime = "video/mp4" }
	class := "object.item.videoItem"
	if strings.HasPrefix(strings.ToLower(mime), "audio/") { class = "object.item.audioItem.musicTrack" }
	protocol := "http-get:*:" + mime + ":DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"
	return `<?xml version="1.0" encoding="UTF-8"?><DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"><item id="0" parentID="0" restricted="1"><dc:title>` + html.EscapeString(strings.TrimSpace(title)) + `</dc:title><upnp:class>` + class + `</upnp:class><res protocolInfo="` + html.EscapeString(protocol) + `">` + html.EscapeString(mediaURL) + `</res></item></DIDL-Lite>`
}

func formatDuration(seconds float64) string {
	if seconds < 0 { seconds = 0 }
	total := int64(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) { return a }
	return b
}

func ParseDeviceID(raw string) string {
	value, _ := url.PathUnescape(strings.TrimSpace(raw))
	return value
}

func SecondsFromTicks(raw string) float64 {
	ticks, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if ticks <= 0 { return 0 }
	return float64(ticks) / 10_000_000
}
