package cloud.stormflix.app;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;
import android.webkit.CookieManager;
import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.Toast;

import java.net.URI;
import java.util.Locale;

/**
 * Mobile/tablet shell that deliberately reuses the StormFlix Web application
 * and Web Playback Engine instead of maintaining a second playback stack.
 *
 * Android TV / Fire TV continue through the native UI because D-pad focus,
 * passthrough and ten-foot controls have different requirements.
 */
public final class WebAppActivity extends Activity {
    private static final String APP_UA = "StormFlixAndroid/0.6.0";

    private SessionStore store;
    private FrameLayout root;
    private WebView webView;
    private View customView;
    private WebChromeClient.CustomViewCallback customViewCallback;
    private int previousOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);

        // Keep the native ten-foot interface for Android TV / Fire TV.
        if (RemoteUi.isTelevision(this)) {
            startActivity(new Intent(this, MainActivity.class));
            finish();
            return;
        }

        store = new SessionStore(this);
        configureWindow();
        buildWebView();
        seedNativeSessionCookies();

        if (state != null) {
            webView.restoreState(state);
        } else {
            webView.loadUrl(store.baseUrl() + "/");
        }
    }

    private void configureWindow() {
        Window window = getWindow();
        window.setStatusBarColor(Color.rgb(5, 6, 9));
        window.setNavigationBarColor(Color.rgb(5, 6, 9));
    }

    private void buildWebView() {
        root = new FrameLayout(this);
        root.setBackgroundColor(Color.BLACK);

        webView = new WebView(this);
        webView.setBackgroundColor(Color.BLACK);
        webView.setLayoutParams(new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.MATCH_PARENT
        ));

        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setMediaPlaybackRequiresUserGesture(false);
        settings.setBuiltInZoomControls(false);
        settings.setDisplayZoomControls(false);
        settings.setSupportZoom(false);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setLoadWithOverviewMode(false);
        settings.setUseWideViewPort(true);
        settings.setJavaScriptCanOpenWindowsAutomatically(false);
        settings.setSupportMultipleWindows(false);
        settings.setUserAgentString(settings.getUserAgentString() + " " + APP_UA);

        CookieManager cookies = CookieManager.getInstance();
        cookies.setAcceptCookie(true);
        CookieManager.setAcceptFileSchemeCookies(false);
        cookies.setAcceptThirdPartyCookies(webView, false);

        webView.setWebViewClient(new StormFlixWebViewClient());
        webView.setWebChromeClient(new StormFlixWebChromeClient());
        webView.setOverScrollMode(View.OVER_SCROLL_NEVER);

        root.addView(webView);
        setContentView(root);
    }

    private void seedNativeSessionCookies() {
        CookieManager cookies = CookieManager.getInstance();
        String base = store.baseUrl();
        String session = store.sessionCookie();
        String profile = store.profileCookie();
        boolean secure = base.toLowerCase(Locale.ROOT).startsWith("https://");
        String suffix = "; Path=/; SameSite=Lax" + (secure ? "; Secure" : "");

        if (session != null && !session.isEmpty()) {
            cookies.setCookie(base, "stormflix_session=" + session + suffix);
        }
        if (profile != null && !profile.isEmpty()) {
            cookies.setCookie(base, "stormflix_profile=" + profile + suffix);
        }
        cookies.flush();
    }

    private void syncWebCookiesBackToNativeStore() {
        if (store == null) return;
        String header = CookieManager.getInstance().getCookie(store.baseUrl());
        if (header == null || header.isEmpty()) return;

        String session = cookieValue(header, "stormflix_session");
        String profile = cookieValue(header, "stormflix_profile");
        if (session != null) store.setSessionCookie(session);
        if (profile != null) store.setProfileCookie(profile);
    }

    private static String cookieValue(String header, String name) {
        String prefix = name + "=";
        for (String raw : header.split(";")) {
            String part = raw.trim();
            if (part.startsWith(prefix)) return part.substring(prefix.length());
        }
        return null;
    }

    private boolean isStormFlixUrl(String url) {
        try {
            URI target = URI.create(url);
            URI base = URI.create(store.baseUrl());
            String targetHost = target.getHost();
            String baseHost = base.getHost();
            return targetHost != null && baseHost != null && targetHost.equalsIgnoreCase(baseHost);
        } catch (Exception ignored) {
            return false;
        }
    }

    private void openExternal(String url) {
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url)));
        } catch (ActivityNotFoundException e) {
            Toast.makeText(this, "Não foi possível abrir este link.", Toast.LENGTH_SHORT).show();
        }
    }

    private final class StormFlixWebViewClient extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            String url = request.getUrl().toString();
            if (isStormFlixUrl(url)) return false;
            openExternal(url);
            return true;
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, String url) {
            if (isStormFlixUrl(url)) return false;
            openExternal(url);
            return true;
        }

        @Override
        public void onPageFinished(WebView view, String url) {
            super.onPageFinished(view, url);
            syncWebCookiesBackToNativeStore();
        }
    }

    private final class StormFlixWebChromeClient extends WebChromeClient {
        @Override
        public void onShowCustomView(View view, CustomViewCallback callback) {
            if (customView != null) {
                callback.onCustomViewHidden();
                return;
            }

            customView = view;
            customViewCallback = callback;
            previousOrientation = getRequestedOrientation();
            webView.setVisibility(View.GONE);
            root.addView(view, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            ));
            getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            enterImmersiveMode();
        }

        @Override
        public void onHideCustomView() {
            hideCustomView();
        }

        @Override
        public void onPermissionRequest(PermissionRequest request) {
            // Playback itself needs no microphone/camera permissions. Do not
            // grant arbitrary web permissions silently.
            request.deny();
        }
    }

    private void enterImmersiveMode() {
        getWindow().getDecorView().setSystemUiVisibility(
            View.SYSTEM_UI_FLAG_FULLSCREEN
                | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
                | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                | View.SYSTEM_UI_FLAG_LAYOUT_STABLE
        );
    }

    private void leaveImmersiveMode() {
        getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
    }

    private void hideCustomView() {
        if (customView == null) return;
        root.removeView(customView);
        customView = null;
        webView.setVisibility(View.VISIBLE);
        getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        leaveImmersiveMode();
        if (previousOrientation != ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED) {
            setRequestedOrientation(previousOrientation);
        }
        if (customViewCallback != null) {
            customViewCallback.onCustomViewHidden();
            customViewCallback = null;
        }
    }

    @Override
    protected void onSaveInstanceState(Bundle outState) {
        webView.saveState(outState);
        super.onSaveInstanceState(outState);
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (webView != null) {
            webView.onResume();
            webView.resumeTimers();
        }
    }

    @Override
    protected void onPause() {
        syncWebCookiesBackToNativeStore();
        if (webView != null) webView.onPause();
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        syncWebCookiesBackToNativeStore();
        if (webView != null) {
            webView.stopLoading();
            webView.loadUrl("about:blank");
            webView.clearHistory();
            webView.removeAllViews();
            webView.destroy();
            webView = null;
        }
        super.onDestroy();
    }

    @Override
    public void onBackPressed() {
        if (customView != null) {
            hideCustomView();
            return;
        }
        if (webView != null && webView.canGoBack()) {
            webView.goBack();
            return;
        }
        super.onBackPressed();
    }
}
