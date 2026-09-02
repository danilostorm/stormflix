package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.net.wifi.WifiManager;
import android.os.Handler;
import android.os.Looper;

import org.w3c.dom.Document;
import org.w3c.dom.Element;
import org.w3c.dom.Node;
import org.w3c.dom.NodeList;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.HttpURLConnection;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import javax.xml.parsers.DocumentBuilderFactory;

/** Native UPnP/DLNA controller used directly by the StormFlix Android APK. */
final class DlnaNative {
    interface Callback { void emit(String state, String message); }

    private static final String SSDP_HOST = "239.255.255.250";
    private static final int SSDP_PORT = 1900;
    private static final String RENDERER_ST = "urn:schemas-upnp-org:device:MediaRenderer:1";
    private static final String AV_TRANSPORT = "urn:schemas-upnp-org:service:AVTransport:1";

    private final Activity activity;
    private final Callback callback;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final ExecutorService io = Executors.newSingleThreadExecutor();

    DlnaNative(Activity activity, Callback callback) {
        this.activity = activity;
        this.callback = callback;
    }

    void discoverAndPlay(String mediaUrl, String title, String mime, double startSeconds) {
        if (mediaUrl == null || mediaUrl.trim().isEmpty()) {
            callback.emit("error", "StormFlix não recebeu uma URL válida para DLNA.");
            return;
        }
        callback.emit("dlna_searching", "Procurando TVs e MediaRenderers DLNA/UPnP na rede local…");
        io.submit(() -> {
            try {
                List<Device> devices = discover();
                main.post(() -> chooseDevice(devices, mediaUrl, title, mime, startSeconds));
            } catch (Exception error) {
                callback.emit("error", safeMessage(error, "Não foi possível procurar dispositivos DLNA nesta rede."));
            }
        });
    }

    private List<Device> discover() throws Exception {
        WifiManager.MulticastLock multicastLock = null;
        DatagramSocket socket = null;
        try {
            WifiManager wifi = (WifiManager) activity.getApplicationContext().getSystemService(Context.WIFI_SERVICE);
            if (wifi != null) {
                multicastLock = wifi.createMulticastLock("StormFlix-DLNA");
                multicastLock.setReferenceCounted(false);
                multicastLock.acquire();
            }

            socket = new DatagramSocket(null);
            socket.setReuseAddress(true);
            socket.bind(new InetSocketAddress(0));
            socket.setSoTimeout(260);
            InetAddress multicast = InetAddress.getByName(SSDP_HOST);
            for (String st : new String[]{RENDERER_ST, "ssdp:all"}) {
                String request = "M-SEARCH * HTTP/1.1\r\n"
                    + "HOST: " + SSDP_HOST + ":" + SSDP_PORT + "\r\n"
                    + "MAN: \"ssdp:discover\"\r\n"
                    + "MX: 1\r\n"
                    + "ST: " + st + "\r\n\r\n";
                byte[] data = request.getBytes(StandardCharsets.UTF_8);
                socket.send(new DatagramPacket(data, data.length, multicast, SSDP_PORT));
            }

            long end = System.currentTimeMillis() + 1700L;
            Map<String, InetAddress> locations = new LinkedHashMap<>();
            byte[] buffer = new byte[65507];
            while (System.currentTimeMillis() < end) {
                DatagramPacket packet = new DatagramPacket(buffer, buffer.length);
                try {
                    socket.receive(packet);
                } catch (java.net.SocketTimeoutException timeout) {
                    continue;
                }
                InetAddress source = packet.getAddress();
                if (!isLocalAddress(source)) continue;
                String response = new String(packet.getData(), packet.getOffset(), packet.getLength(), StandardCharsets.UTF_8);
                String location = header(response, "location");
                if (location != null && !location.trim().isEmpty()) locations.put(location.trim(), source);
            }

            Map<String, Device> unique = new LinkedHashMap<>();
            for (Map.Entry<String, InetAddress> entry : locations.entrySet()) {
                try {
                    Device device = fetchDevice(entry.getKey(), entry.getValue());
                    if (device != null && device.controlUrl != null && !device.controlUrl.isEmpty()) unique.put(device.id, device);
                } catch (Exception ignored) {}
            }
            List<Device> out = new ArrayList<>(unique.values());
            Collections.sort(out, Comparator.comparing(d -> d.name.toLowerCase(Locale.ROOT)));
            return out;
        } finally {
            if (socket != null) socket.close();
            if (multicastLock != null && multicastLock.isHeld()) multicastLock.release();
        }
    }

    private void chooseDevice(List<Device> devices, String mediaUrl, String title, String mime, double startSeconds) {
        if (activity.isFinishing() || activity.isDestroyed()) return;
        if (devices == null || devices.isEmpty()) {
            callback.emit("error", "Nenhuma TV ou MediaRenderer DLNA/UPnP foi encontrada nesta rede local.");
            return;
        }
        String[] labels = new String[devices.size()];
        for (int i = 0; i < devices.size(); i++) {
            Device d = devices.get(i);
            String extra = joinNonEmpty(d.manufacturer, d.model);
            labels[i] = extra.isEmpty() ? d.name : d.name + "\n" + extra;
        }
        new AlertDialog.Builder(activity)
            .setTitle("DLNA / UPnP")
            .setItems(labels, (dialog, which) -> {
                Device selected = devices.get(which);
                callback.emit("dlna_searching", "Conectando a " + selected.name + "…");
                io.submit(() -> {
                    try {
                        play(selected, mediaUrl, title, mime, startSeconds);
                        callback.emit("dlna_connected", "Transmitindo para " + selected.name + " por DLNA / UPnP nativo.");
                    } catch (Exception error) {
                        callback.emit("error", safeMessage(error, "O dispositivo DLNA recusou a reprodução."));
                    }
                });
            })
            .setNegativeButton("Cancelar", null)
            .show();
    }

    private Device fetchDevice(String rawLocation, InetAddress source) throws Exception {
        URI location = new URI(rawLocation.trim());
        if (!"http".equalsIgnoreCase(location.getScheme()) || !safeLocalHost(location.getHost())) throw new IllegalArgumentException("Descrição UPnP fora da rede local");
        HttpURLConnection c = (HttpURLConnection) location.toURL().openConnection();
        c.setConnectTimeout(1800); c.setReadTimeout(2200); c.setRequestProperty("Accept", "text/xml, application/xml");
        int status = c.getResponseCode();
        if (status < 200 || status >= 300) { c.disconnect(); throw new IllegalStateException("UPnP HTTP " + status); }
        byte[] xml = readLimited(c.getInputStream(), 1024 * 1024); c.disconnect();

        DocumentBuilderFactory factory = secureFactory();
        Document document = factory.newDocumentBuilder().parse(new ByteArrayInputStream(xml));
        Element renderer = findRenderer(document.getDocumentElement());
        if (renderer == null) return null;
        String avControl = findServiceControl(renderer, "AVTransport");
        if (avControl.isEmpty()) return null;
        URI control = location.resolve(avControl);
        if (!"http".equalsIgnoreCase(control.getScheme()) || !safeLocalHost(control.getHost())) throw new IllegalArgumentException("Controle UPnP fora da rede local");
        String udn = childText(renderer, "UDN").replaceFirst("(?i)^uuid:", "").trim();
        if (udn.isEmpty()) udn = Integer.toHexString(rawLocation.hashCode());
        String name = childText(renderer, "friendlyName").trim();
        if (name.isEmpty()) name = childText(renderer, "modelName").trim();
        if (name.isEmpty()) name = source == null ? "Dispositivo DLNA" : source.getHostAddress();
        return new Device(udn, name, childText(renderer, "manufacturer").trim(), childText(renderer, "modelName").trim(), control.toString());
    }

    private void play(Device device, String mediaUrl, String title, String mime, double startSeconds) throws Exception {
        String metadata = didl(mediaUrl, title, mime);
        soap(device.controlUrl, "SetAVTransportURI", mapOf("InstanceID", "0", "CurrentURI", mediaUrl, "CurrentURIMetaData", metadata));
        soap(device.controlUrl, "Play", mapOf("InstanceID", "0", "Speed", "1"));
        if (startSeconds > 1d) {
            try { Thread.sleep(220L); soap(device.controlUrl, "Seek", mapOf("InstanceID", "0", "Unit", "REL_TIME", "Target", duration(startSeconds))); }
            catch (Exception ignored) {}
        }
    }

    private void soap(String rawUrl, String action, Map<String,String> args) throws Exception {
        URI uri = new URI(rawUrl);
        if (!"http".equalsIgnoreCase(uri.getScheme()) || !safeLocalHost(uri.getHost())) throw new IllegalArgumentException("Destino DLNA inválido");
        StringBuilder xml = new StringBuilder("<?xml version=\"1.0\" encoding=\"utf-8\"?>");
        xml.append("<s:Envelope xmlns:s=\"http://schemas.xmlsoap.org/soap/envelope/\" s:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\"><s:Body>");
        xml.append("<u:").append(action).append(" xmlns:u=\"").append(AV_TRANSPORT).append("\">");
        for (Map.Entry<String,String> entry : args.entrySet()) xml.append('<').append(entry.getKey()).append('>').append(escape(entry.getValue())).append("</").append(entry.getKey()).append('>');
        xml.append("</u:").append(action).append("></s:Body></s:Envelope>");
        byte[] body = xml.toString().getBytes(StandardCharsets.UTF_8);
        HttpURLConnection c = (HttpURLConnection) uri.toURL().openConnection();
        c.setConnectTimeout(2500); c.setReadTimeout(4500); c.setRequestMethod("POST"); c.setDoOutput(true);
        c.setRequestProperty("Content-Type", "text/xml; charset=\"utf-8\"");
        c.setRequestProperty("SOAPAction", "\"" + AV_TRANSPORT + "#" + action + "\"");
        c.setFixedLengthStreamingMode(body.length);
        try (OutputStream out = c.getOutputStream()) { out.write(body); }
        int status = c.getResponseCode();
        if (status < 200 || status >= 300) {
            String detail = "";
            try { detail = new String(readLimited(c.getErrorStream(), 32 * 1024), StandardCharsets.UTF_8).trim(); } catch (Exception ignored) {}
            c.disconnect(); throw new IllegalStateException("DLNA " + action + " retornou HTTP " + status + (detail.isEmpty() ? "" : ": " + detail));
        }
        try { if (c.getInputStream() != null) c.getInputStream().close(); } catch (Exception ignored) {}
        c.disconnect();
    }

    private static DocumentBuilderFactory secureFactory() throws Exception {
        DocumentBuilderFactory f = DocumentBuilderFactory.newInstance();
        f.setNamespaceAware(false); f.setXIncludeAware(false); f.setExpandEntityReferences(false);
        try { f.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true); } catch (Exception ignored) {}
        try { f.setFeature("http://xml.org/sax/features/external-general-entities", false); } catch (Exception ignored) {}
        try { f.setFeature("http://xml.org/sax/features/external-parameter-entities", false); } catch (Exception ignored) {}
        return f;
    }

    private static Element findRenderer(Element root) {
        NodeList devices = root.getElementsByTagName("device");
        for (int i = 0; i < devices.getLength(); i++) {
            if (!(devices.item(i) instanceof Element)) continue;
            Element e = (Element) devices.item(i);
            if (childText(e, "deviceType").contains(":device:MediaRenderer:")) return e;
        }
        if ("device".equalsIgnoreCase(root.getTagName()) && childText(root, "deviceType").contains(":device:MediaRenderer:")) return root;
        return null;
    }

    private static String findServiceControl(Element renderer, String name) {
        NodeList services = renderer.getElementsByTagName("service");
        for (int i = 0; i < services.getLength(); i++) {
            if (!(services.item(i) instanceof Element)) continue;
            Element service = (Element) services.item(i);
            if (childText(service, "serviceType").contains(":service:" + name + ":")) return childText(service, "controlURL").trim();
        }
        return "";
    }

    private static String childText(Element parent, String tag) {
        NodeList nodes = parent.getElementsByTagName(tag);
        if (nodes.getLength() == 0) return "";
        Node node = nodes.item(0); return node == null || node.getTextContent() == null ? "" : node.getTextContent();
    }

    private static String header(String response, String wanted) {
        String[] lines = response.split("\\r?\\n");
        for (String line : lines) {
            int cut = line.indexOf(':'); if (cut <= 0) continue;
            if (line.substring(0, cut).trim().equalsIgnoreCase(wanted)) return line.substring(cut + 1).trim();
        }
        return null;
    }

    private static boolean safeLocalHost(String host) {
        if (host == null || host.trim().isEmpty()) return false;
        try { for (InetAddress address : InetAddress.getAllByName(host)) if (!isLocalAddress(address)) return false; return true; }
        catch (Exception ignored) { return false; }
    }

    private static boolean isLocalAddress(InetAddress address) {
        return address != null && (address.isSiteLocalAddress() || address.isLinkLocalAddress() || address.isLoopbackAddress());
    }

    private static byte[] readLimited(InputStream input, int limit) throws Exception {
        if (input == null) return new byte[0];
        try (InputStream in = input; ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[8192]; int total = 0, n;
            while ((n = in.read(buffer, 0, Math.min(buffer.length, limit - total))) > 0) { out.write(buffer, 0, n); total += n; if (total >= limit) break; }
            return out.toByteArray();
        }
    }

    private static Map<String,String> mapOf(String... values) {
        Map<String,String> map = new LinkedHashMap<>(); for (int i = 0; i + 1 < values.length; i += 2) map.put(values[i], values[i + 1] == null ? "" : values[i + 1]); return map;
    }

    private static String didl(String mediaUrl, String title, String mime) {
        String type = mime == null || mime.trim().isEmpty() ? "video/mp4" : mime.trim();
        String mediaClass = type.toLowerCase(Locale.ROOT).startsWith("audio/") ? "object.item.audioItem.musicTrack" : "object.item.videoItem";
        String protocol = "http-get:*:" + type + ":DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000";
        return "<?xml version=\"1.0\" encoding=\"UTF-8\"?><DIDL-Lite xmlns=\"urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/\" xmlns:dc=\"http://purl.org/dc/elements/1.1/\" xmlns:upnp=\"urn:schemas-upnp-org:metadata-1-0/upnp/\"><item id=\"0\" parentID=\"0\" restricted=\"1\"><dc:title>" + escape(title) + "</dc:title><upnp:class>" + mediaClass + "</upnp:class><res protocolInfo=\"" + escape(protocol) + "\">" + escape(mediaUrl) + "</res></item></DIDL-Lite>";
    }

    private static String duration(double seconds) {
        long total = Math.max(0L, (long) seconds); return String.format(Locale.US, "%02d:%02d:%02d", total / 3600, (total % 3600) / 60, total % 60);
    }

    private static String escape(String value) {
        String text = value == null ? "" : value; return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("\"", "&quot;").replace("'", "&apos;");
    }

    private static String joinNonEmpty(String a, String b) {
        String left = a == null ? "" : a.trim(), right = b == null ? "" : b.trim();
        if (left.isEmpty()) return right; if (right.isEmpty()) return left; return left + " · " + right;
    }

    private static String safeMessage(Exception error, String fallback) {
        String text = error == null || error.getMessage() == null ? "" : error.getMessage().trim(); return text.isEmpty() ? fallback : text;
    }

    private static final class Device {
        final String id, name, manufacturer, model, controlUrl;
        Device(String id, String name, String manufacturer, String model, String controlUrl) { this.id=id; this.name=name; this.manufacturer=manufacturer; this.model=model; this.controlUrl=controlUrl; }
    }
}
