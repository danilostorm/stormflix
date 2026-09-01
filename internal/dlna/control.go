package dlna

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (m *Manager) Play(ctx context.Context, device Device, mediaURL, title, mime string, startSeconds float64) error {
	if strings.TrimSpace(mediaURL) == "" { return errors.New("media URL is required") }
	if strings.TrimSpace(device.AVTransportControlURL) == "" { return errors.New("renderer does not expose AVTransport") }
	metadata := didlMetadata(mediaURL, title, mime)
	if err := soap(ctx, device.AVTransportControlURL, avTransportType, "SetAVTransportURI", map[string]string{
		"InstanceID":"0", "CurrentURI":mediaURL, "CurrentURIMetaData":metadata,
	}); err != nil { return err }
	if err := soap(ctx, device.AVTransportControlURL, avTransportType, "Play", map[string]string{"InstanceID":"0", "Speed":"1"}); err != nil { return err }
	if startSeconds > 1 {
		time.Sleep(220*time.Millisecond)
		_ = soap(ctx, device.AVTransportControlURL, avTransportType, "Seek", map[string]string{"InstanceID":"0", "Unit":"REL_TIME", "Target":formatDuration(startSeconds)})
	}
	return nil
}

func (m *Manager) Control(ctx context.Context, device Device, command string, positionSeconds float64) error {
	command = strings.ToLower(strings.TrimSpace(command))
	switch command {
	case "play", "resume", "unpause":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Play", map[string]string{"InstanceID":"0", "Speed":"1"})
	case "pause":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Pause", map[string]string{"InstanceID":"0"})
	case "stop":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Stop", map[string]string{"InstanceID":"0"})
	case "seek":
		return soap(ctx, device.AVTransportControlURL, avTransportType, "Seek", map[string]string{"InstanceID":"0", "Unit":"REL_TIME", "Target":formatDuration(positionSeconds)})
	default:
		return fmt.Errorf("unsupported DLNA command %q", command)
	}
}

func soap(ctx context.Context, controlURL, serviceType, action string, args map[string]string) error {
	parsed, err := url.Parse(strings.TrimSpace(controlURL))
	if err != nil || parsed.Scheme != "http" || !urlIsPrivate(parsed) { return errors.New("unsafe DLNA control URL") }
	var body strings.Builder
	body.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>")
	body.WriteString("<s:Envelope xmlns:s=\"http://schemas.xmlsoap.org/soap/envelope/\" s:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\"><s:Body>")
	body.WriteString("<u:"+action+" xmlns:u=\""+html.EscapeString(serviceType)+"\">")
	keys := make([]string,0,len(args)); for key := range args { keys=append(keys,key) }; sort.Strings(keys)
	for _, key := range keys { body.WriteString("<"+key+">"+html.EscapeString(args[key])+"</"+key+">") }
	body.WriteString("</u:"+action+"></s:Body></s:Envelope>")
	req, _ := http.NewRequestWithContext(ctx,http.MethodPost,parsed.String(),strings.NewReader(body.String()))
	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", "\""+serviceType+"#"+action+"\"")
	resp, err := (&http.Client{Timeout:4*time.Second}).Do(req)
	if err != nil { return fmt.Errorf("DLNA %s failed: %w",action,err) }
	defer resp.Body.Close()
	payload,_ := io.ReadAll(io.LimitReader(resp.Body,32*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("DLNA %s returned HTTP %d: %s",action,resp.StatusCode,strings.TrimSpace(string(payload))) }
	return nil
}

func didlMetadata(mediaURL,title,mime string) string {
	mime = strings.TrimSpace(mime); if mime=="" { mime="video/mp4" }
	class := "object.item.videoItem"; if strings.HasPrefix(strings.ToLower(mime),"audio/") { class="object.item.audioItem.musicTrack" }
	protocol := "http-get:*:"+mime+":DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?><DIDL-Lite xmlns=\"urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/\" xmlns:dc=\"http://purl.org/dc/elements/1.1/\" xmlns:upnp=\"urn:schemas-upnp-org:metadata-1-0/upnp/\"><item id=\"0\" parentID=\"0\" restricted=\"1\"><dc:title>"+html.EscapeString(strings.TrimSpace(title))+"</dc:title><upnp:class>"+class+"</upnp:class><res protocolInfo=\""+html.EscapeString(protocol)+"\">"+html.EscapeString(mediaURL)+"</res></item></DIDL-Lite>"
}

func formatDuration(seconds float64) string {
	if seconds < 0 { seconds=0 }; total:=int64(seconds); return fmt.Sprintf("%02d:%02d:%02d",total/3600,(total%3600)/60,total%60)
}

func ParseDeviceID(raw string) string { value,_:=url.PathUnescape(strings.TrimSpace(raw)); return value }
func SecondsFromTicks(raw string) float64 { ticks,_:=strconv.ParseInt(strings.TrimSpace(raw),10,64); if ticks<=0{return 0}; return float64(ticks)/10_000_000 }
