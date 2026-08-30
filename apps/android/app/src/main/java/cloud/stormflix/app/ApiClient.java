package cloud.stormflix.app;

import android.content.Context;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;

public final class ApiClient {
    public static final class ApiException extends IOException {
        public final int status;
        ApiException(int status, String message) { super(message); this.status = status; }
    }

    private final SessionStore store;

    public ApiClient(Context context) { store = new SessionStore(context.getApplicationContext()); }
    public SessionStore store() { return store; }

    public String apiUrl(String path) {
        if (!path.startsWith("/")) path = "/" + path;
        return store.baseUrl() + "/api/v1" + path;
    }

    public String absoluteAssetUrl(String url) {
        if (url == null || url.trim().isEmpty()) return "";
        if (url.startsWith("http://") || url.startsWith("https://")) return url;
        return store.baseUrl() + (url.startsWith("/") ? url : "/" + url);
    }

    public String get(String path) throws IOException { return request("GET", path, null); }
    public String post(String path, JSONObject body) throws IOException { return request("POST", path, body == null ? "{}" : body.toString()); }
    public String delete(String path) throws IOException { return request("DELETE", path, null); }

    private String request(String method, String path, String body) throws IOException {
        HttpURLConnection c = open(apiUrl(path));
        c.setRequestMethod(method);
        c.setRequestProperty("Accept", "application/json");
        if (body != null) {
            c.setDoOutput(true);
            c.setRequestProperty("Content-Type", "application/json; charset=utf-8");
            try (OutputStream out = c.getOutputStream()) { out.write(body.getBytes(StandardCharsets.UTF_8)); }
        }
        int status = c.getResponseCode();
        captureCookies(c);
        String text = readText(status >= 400 ? c.getErrorStream() : c.getInputStream());
        c.disconnect();
        if (status >= 400) throw new ApiException(status, errorMessage(text, status));
        return text;
    }

    public byte[] getBytes(String absoluteUrl) throws IOException {
        HttpURLConnection c = open(absoluteAssetUrl(absoluteUrl));
        c.setRequestMethod("GET");
        int status = c.getResponseCode();
        captureCookies(c);
        if (status >= 400) {
            String text = readText(c.getErrorStream());
            c.disconnect();
            throw new ApiException(status, errorMessage(text, status));
        }
        byte[] data = readBytes(c.getInputStream());
        c.disconnect();
        return data;
    }

    private HttpURLConnection open(String url) throws IOException {
        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
        c.setConnectTimeout(15000);
        c.setReadTimeout(1200000);
        c.setInstanceFollowRedirects(true);
        c.setRequestProperty("User-Agent", "StormFlix-Android/0.5.2");
        String cookies = store.cookieHeader();
        if (!cookies.isEmpty()) c.setRequestProperty("Cookie", cookies);
        return c;
    }

    private void captureCookies(HttpURLConnection c) {
        Map<String, List<String>> headers = c.getHeaderFields();
        for (Map.Entry<String, List<String>> entry : headers.entrySet()) {
            if (entry.getKey() == null || !entry.getKey().equalsIgnoreCase("Set-Cookie")) continue;
            for (String raw : entry.getValue()) {
                if (raw == null) continue;
                captureCookie(raw, "stormflix_session", true);
                captureCookie(raw, "stormflix_profile", false);
            }
        }
    }

    private void captureCookie(String raw, String name, boolean session) {
        String marker = name + "="; int start = raw.indexOf(marker); if (start < 0) return; start += marker.length();
        int end = raw.indexOf(';', start); String value = (end < 0 ? raw.substring(start) : raw.substring(start, end)).trim();
        if (session) store.setSessionCookie(value); else store.setProfileCookie(value);
    }

    private static String errorMessage(String text, int status) {
        try { JSONObject obj = new JSONObject(text == null ? "{}" : text); String msg = obj.optString("error", obj.optString("message", "")); if (!msg.isEmpty()) return msg; } catch (Exception ignored) {}
        return text == null || text.trim().isEmpty() ? "HTTP " + status : text.trim();
    }
    private static String readText(InputStream in) throws IOException { if (in == null) return ""; return new String(readBytes(in), StandardCharsets.UTF_8); }
    private static byte[] readBytes(InputStream in) throws IOException { try (InputStream input = in; ByteArrayOutputStream out = new ByteArrayOutputStream()) { byte[] buffer = new byte[16384]; int n; while ((n = input.read(buffer)) >= 0) out.write(buffer, 0, n); return out.toByteArray(); } }
}
