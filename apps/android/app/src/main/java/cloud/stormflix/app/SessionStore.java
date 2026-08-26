package cloud.stormflix.app;

import android.content.Context;
import android.content.SharedPreferences;

public final class SessionStore {
    private static final String PREFS = "stormflix_native";
    private static final String DEFAULT_SERVER = "https://stormflix.cloud";
    private final SharedPreferences prefs;

    public SessionStore(Context context) {
        prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    public String baseUrl() {
        String value = prefs.getString("base_url", DEFAULT_SERVER);
        return normalizeBase(value == null ? DEFAULT_SERVER : value);
    }

    public void setBaseUrl(String value) {
        prefs.edit().putString("base_url", normalizeBase(value)).apply();
    }

    public String sessionCookie() {
        return prefs.getString("session", "");
    }

    public void setSessionCookie(String value) {
        prefs.edit().putString("session", value == null ? "" : value).apply();
    }

    public String profileCookie() {
        return prefs.getString("profile", "");
    }

    public void setProfileCookie(String value) {
        prefs.edit().putString("profile", value == null ? "" : value).apply();
    }

    public String cookieHeader() {
        String session = sessionCookie();
        String profile = profileCookie();
        StringBuilder out = new StringBuilder();
        if (!session.isEmpty()) out.append("stormflix_session=").append(session);
        if (!profile.isEmpty()) {
            if (out.length() > 0) out.append("; ");
            out.append("stormflix_profile=").append(profile);
        }
        return out.toString();
    }

    public boolean signedIn() {
        return !sessionCookie().isEmpty();
    }

    public void clear() {
        prefs.edit().remove("session").remove("profile").apply();
    }

    public static String normalizeBase(String value) {
        String v = value == null ? "" : value.trim();
        if (v.isEmpty()) v = DEFAULT_SERVER;
        if (!v.startsWith("http://") && !v.startsWith("https://")) v = "https://" + v;
        while (v.endsWith("/")) v = v.substring(0, v.length() - 1);
        if (v.endsWith("/api/v1")) v = v.substring(0, v.length() - 7);
        return v;
    }
}
