package dlna

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestParseSSDPAndRendererDescription(t *testing.T) {
	headers := parseSSDP([]byte("HTTP/1.1 200 OK\r\nLOCATION: http://192.168.1.20:1400/xml/device.xml\r\nST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n"))
	if got := headers["location"]; got != "http://192.168.1.20:1400/xml/device.xml" { t.Fatalf("location=%q",got) }

	xmlDoc := `<?xml version="1.0"?><root><device><deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType><friendlyName>Sala TV</friendlyName><manufacturer>Storm Test</manufacturer><modelName>Renderer</modelName><UDN>uuid:abc-123</UDN><serviceList><service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>/upnp/control/avtransport</controlURL></service></serviceList></device></root>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){ _,_=io.WriteString(w,xmlDoc) }))
	defer server.Close()
	device, err := fetchDevice(context.Background(),server.URL,net.ParseIP("127.0.0.1"))
	if err != nil { t.Fatal(err) }
	if device.ID!="abc-123" || device.Name!="Sala TV" { t.Fatalf("device=%+v",device) }
	if !strings.HasSuffix(device.AVTransportControlURL,"/upnp/control/avtransport") { t.Fatalf("control=%q",device.AVTransportControlURL) }
}

func TestAVTransportPlayBuildsValidSOAP(t *testing.T) {
	var mu sync.Mutex
	actions:=[]string{}
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		body,_:=io.ReadAll(r.Body)
		if strings.Contains(string(body),`\"`) { t.Errorf("SOAP contains escaped quote bytes: %s",body) }
		if !strings.HasPrefix(string(body),`<?xml version="1.0"`) { t.Errorf("invalid XML prolog: %s",body) }
		mu.Lock(); actions=append(actions,r.Header.Get("SOAPAction")); mu.Unlock()
		w.Header().Set("Content-Type","text/xml"); _,_=io.WriteString(w,`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body/></s:Envelope>`)
	}))
	defer server.Close()
	device:=Device{ID:"x",Name:"TV",AVTransportControlURL:server.URL}
	if err:=NewManager().Play(context.Background(),device,"http://127.0.0.1/media.mp4","Filme & Teste","video/mp4",0); err!=nil { t.Fatal(err) }
	mu.Lock(); defer mu.Unlock()
	if len(actions)!=2 || !strings.Contains(actions[0],"#SetAVTransportURI") || !strings.Contains(actions[1],"#Play") { t.Fatalf("actions=%v",actions) }
}

func TestDurationAndTicks(t *testing.T) {
	if got:=formatDuration(3661); got!="01:01:01" { t.Fatalf("duration=%s",got) }
	if got:=SecondsFromTicks("150000000"); got!=15 { t.Fatalf("seconds=%f",got) }
}
