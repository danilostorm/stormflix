package cloud.stormflix.app;

import android.app.Activity;
import android.content.pm.ActivityInfo;
import android.graphics.Color;
import android.os.Bundle;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
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
 * Unified Android player.
 *
 * The native StormFlix catalog remains the shell on phone, tablet, Android TV
 * and Fire TV, but playback itself is delegated to the same Web Playback Engine
 * used by stormflix.cloud. This removes the second Media3/PlaybackPlan stack and
 * keeps Direct Play, Direct Stream, audio selection, quality, subtitles, zoom,
 * resume and HLS recovery identical to the validated Web experience.
 */
public final class PlayerActivity extends Activity {
    private static final String APP_UA = "StormFlixAndroidPlayer/0.6.1";

    private SessionStore store;
    private FrameLayout root;
    private WebView webView;
    private View customView;
    private WebChromeClient.CustomViewCallback customViewCallback;
    private boolean playerInjected;
    private long mediaId;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        mediaId = getIntent().getLongExtra("media_id", 0L);
        if (mediaId <= 0L) {
            finish();
            return;
        }

        store = new SessionStore(this);
        configureWindow();
        buildWebView();
        seedNativeSessionCookies();
        webView.loadUrl(store.baseUrl() + "/?stormflix_native_player=1&media_id=" + mediaId);
    }

    private void configureWindow() {
        Window window = getWindow();
        window.setStatusBarColor(Color.BLACK);
        window.setNavigationBarColor(Color.BLACK);
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        enterImmersiveMode();
    }

    private void buildWebView() {
        root = new FrameLayout(this);
        root.setBackgroundColor(Color.BLACK);

        webView = new WebView(this);
        webView.setBackgroundColor(Color.BLACK);
        webView.setVisibility(View.INVISIBLE);
        webView.setFocusable(true);
        webView.setFocusableInTouchMode(true);
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
        cookies.setAcceptThirdPartyCookies(webView, false);

        webView.addJavascriptInterface(new NativePlayerShell(), "StormFlixShell");
        webView.setWebViewClient(new PlayerWebViewClient());
        webView.setWebChromeClient(new PlayerWebChromeClient());
        webView.setOverScrollMode(View.OVER_SCROLL_NEVER);

        root.addView(webView);
        setContentView(root);
    }

    private void seedNativeSessionCookies() {
        String base = store.baseUrl();
        String session = store.sessionCookie();
        String profile = store.profileCookie();
        boolean secure = base.toLowerCase(Locale.ROOT).startsWith("https://");
        String suffix = "; Path=/; SameSite=Lax" + (secure ? "; Secure" : "");
        CookieManager cookies = CookieManager.getInstance();
        if (session != null && !session.isEmpty()) {
            cookies.setCookie(base, "stormflix_session=" + session + suffix);
        }
        if (profile != null && !profile.isEmpty()) {
            cookies.setCookie(base, "stormflix_profile=" + profile + suffix);
        }
        cookies.flush();
    }

    private void syncCookiesBack() {
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
            return target.getHost() != null
                && base.getHost() != null
                && target.getHost().equalsIgnoreCase(base.getHost());
        } catch (Exception ignored) {
            return false;
        }
    }

    private void injectAndStartWebPlayer() {
        if (playerInjected || webView == null) return;
        playerInjected = true;
        String script = "(function(){"
            + "var mediaId=" + mediaId + ";"
            + "var tries=0;"
            + "function fail(e){try{StormFlixShell.error(String((e&&e.message)||e||'Falha ao abrir o Web Player'));}catch(_){}}"
            + "function start(){"
            + " tries++;"
            + " if(typeof request!=='function'||typeof playMedia!=='function'){"
            + "   if(tries<160){setTimeout(start,100);return;}"
            + "   fail('Web Player não ficou pronto.');return;"
            + " }"
            + " try{"
            + "   var style=document.createElement('style');"
            + "   style.id='sf-native-player-style';"
            + "   style.textContent='#shell,#login,#profile-picker,#detail-modal,#category-explorer,#search-view,#catalog-view,#music-view{display:none!important}html,body{margin:0!important;background:#000!important;overflow:hidden!important;width:100%!important;height:100%!important}#player-modal{position:fixed!important;inset:0!important;z-index:2147483000!important;width:100vw!important;height:100vh!important;background:#000!important}';"
            + "   document.head.appendChild(style);"
            + "   if(typeof closePlayer==='function'&&!window.__stormflixNativeCloseWrapped){"
            + "     var originalClose=closePlayer;"
            + "     closePlayer=function(){var result;try{result=originalClose.apply(this,arguments);}finally{setTimeout(function(){try{StormFlixShell.close();}catch(_){}},30);}return result;};"
            + "     window.__stormflixNativeCloseWrapped=true;"
            + "   }"
            + "   request('/media/'+mediaId).then(function(item){"
            + "     return Promise.resolve(playMedia(item));"
            + "   }).then(function(){"
            + "     var close=document.querySelector('#player-close');"
            + "     if(close&&!close.dataset.nativeClose){close.dataset.nativeClose='1';close.addEventListener('click',function(){setTimeout(function(){try{StormFlixShell.close();}catch(_){}},80);});}"
            + "     try{StormFlixShell.ready();}catch(_){}"
            + "   }).catch(fail);"
            + " }catch(e){fail(e);}"
            + "}"
            + "start();"
            + "})();";
        webView.evaluateJavascript(script, null);
    }

    private final class PlayerWebViewClient extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            return !isStormFlixUrl(request.getUrl().toString());
        }

        @Override
        public boolean shouldOverrideUrlLoading(WebView view, String url) {
            return !isStormFlixUrl(url);
        }

        @Override
        public void onPageFinished(WebView view, String url) {
            super.onPageFinished(view, url);
            syncCookiesBack();
            if (isStormFlixUrl(url)) injectAndStartWebPlayer();
        }
    }

    private final class PlayerWebChromeClient extends WebChromeClient {
        @Override
        public void onShowCustomView(View view, CustomViewCallback callback) {
            if (customView != null) {
                callback.onCustomViewHidden();
                return;
            }
            customView = view;
            customViewCallback = callback;
            webView.setVisibility(View.GONE);
            root.addView(view, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            ));
            enterImmersiveMode();
        }

        @Override
        public void onHideCustomView() {
            hideCustomView();
        }
    }

    private final class NativePlayerShell {
        @JavascriptInterface
        public void ready() {
            runOnUiThread(() -> {
                if (webView == null) return;
                webView.setVisibility(View.VISIBLE);
                webView.requestFocus();
                enterImmersiveMode();
            });
        }

        @JavascriptInterface
        public void close() {
            runOnUiThread(() -> {
                syncCookiesBack();
                if (!isFinishing()) finish();
            });
        }

        @JavascriptInterface
        public void error(String message) {
            runOnUiThread(() -> {
                Toast.makeText(PlayerActivity.this,
                    message == null || message.trim().isEmpty() ? "Não foi possível iniciar o vídeo." : message,
                    Toast.LENGTH_LONG).show();
                finish();
            });
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

    private void hideCustomView() {
        if (customView == null) return;
        root.removeView(customView);
        customView = null;
        if (webView != null) webView.setVisibility(View.VISIBLE);
        if (customViewCallback != null) {
            customViewCallback.onCustomViewHidden();
            customViewCallback = null;
        }
        enterImmersiveMode();
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (webView != null) {
            webView.onResume();
            webView.resumeTimers();
        }
        enterImmersiveMode();
    }

    @Override
    protected void onPause() {
        syncCookiesBack();
        if (webView != null) webView.onPause();
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        syncCookiesBack();
        getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        if (webView != null) {
            webView.stopLoading();
            webView.loadUrl("about:blank");
            webView.removeJavascriptInterface("StormFlixShell");
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
        if (webView != null) {
            webView.evaluateJavascript("try{if(typeof closePlayer==='function'){closePlayer();}else{StormFlixShell.close();}}catch(e){StormFlixShell.close();}", null);
            return;
        }
        super.onBackPressed();
    }
}
