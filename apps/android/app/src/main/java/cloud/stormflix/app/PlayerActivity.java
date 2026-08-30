package cloud.stormflix.app;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.view.KeyEvent;
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
 * used by stormflix.cloud.
 *
 * TV remote input is deliberately intercepted before Android System WebView can
 * deliver D-pad keys to the HTML <video>. Browsers commonly interpret Up/Down
 * on a focused media element as volume changes. We instead translate physical
 * keys to semantic StormFlix TV commands, mirroring the architecture used by
 * mature TV clients such as Jellyfin.
 */
public final class PlayerActivity extends Activity {
    private static final String APP_UA = "StormFlixAndroidPlayer/0.6.2";

    private SessionStore store;
    private FrameLayout root;
    private WebView webView;
    private View customView;
    private WebChromeClient.CustomViewCallback customViewCallback;
    private boolean playerInjected;
    private boolean tvDevice;
    private long mediaId;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        mediaId = getIntent().getLongExtra("media_id", 0L);
        if (mediaId <= 0L) {
            finish();
            return;
        }

        tvDevice = RemoteUi.isTelevision(this);
        store = new SessionStore(this);
        configureWindow();
        buildWebView();
        seedNativeSessionCookies();
        String tv = tvDevice ? "&stormflix_tv=1" : "";
        webView.loadUrl(store.baseUrl() + "/?stormflix_native_player=1&media_id=" + mediaId + tv);
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

    /**
     * Translate Android/Fire TV key codes to the same semantic command names
     * consumed by tv-remote.js. Volume keys are intentionally NOT mapped so the
     * operating system keeps ownership of hardware volume.
     */
    private String remoteCommandForKey(int keyCode) {
        switch (keyCode) {
            case KeyEvent.KEYCODE_DPAD_UP:
                return "up";
            case KeyEvent.KEYCODE_DPAD_DOWN:
                return "down";
            case KeyEvent.KEYCODE_DPAD_LEFT:
                return "left";
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                return "right";
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
            case KeyEvent.KEYCODE_NUMPAD_ENTER:
            case KeyEvent.KEYCODE_BUTTON_A:
            case KeyEvent.KEYCODE_BUTTON_SELECT:
                return "select";
            case KeyEvent.KEYCODE_BACK:
            case KeyEvent.KEYCODE_ESCAPE:
            case KeyEvent.KEYCODE_BUTTON_B:
                return "back";
            case KeyEvent.KEYCODE_MENU:
            case KeyEvent.KEYCODE_SETTINGS:
            case KeyEvent.KEYCODE_INFO:
                return "menu";
            case KeyEvent.KEYCODE_MEDIA_PLAY:
                return "play";
            case KeyEvent.KEYCODE_MEDIA_PAUSE:
                return "pause";
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
            case KeyEvent.KEYCODE_HEADSETHOOK:
                return "playpause";
            case KeyEvent.KEYCODE_MEDIA_REWIND:
                return "rewind";
            case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD:
                return "fastforward";
            case KeyEvent.KEYCODE_MEDIA_STOP:
                return "stop";
            case KeyEvent.KEYCODE_MEDIA_PREVIOUS:
            case KeyEvent.KEYCODE_CHANNEL_DOWN:
                return "previoustrack";
            case KeyEvent.KEYCODE_MEDIA_NEXT:
            case KeyEvent.KEYCODE_CHANNEL_UP:
                return "nexttrack";
            case KeyEvent.KEYCODE_CAPTIONS:
                return "subtitles";
            case KeyEvent.KEYCODE_MEDIA_AUDIO_TRACK:
                return "audio";
            default:
                return null;
        }
    }

    private boolean repeatableRemoteCommand(String command) {
        return "left".equals(command)
            || "right".equals(command)
            || "up".equals(command)
            || "down".equals(command)
            || "rewind".equals(command)
            || "fastforward".equals(command);
    }

    private void sendRemoteCommand(String command, boolean repeated) {
        if (webView == null || command == null) return;
        String script = "try{if(window.sfTvRemote){window.sfTvRemote.handleNativeKey('"
            + command + "'," + (repeated ? "true" : "false") + ");}}catch(e){}";
        webView.evaluateJavascript(script, null);
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        if (tvDevice && event != null) {
            int keyCode = event.getKeyCode();

            // HTML fullscreen custom views hide the WebView. Back must first
            // leave that custom view before normal player navigation resumes.
            if (customView != null && (keyCode == KeyEvent.KEYCODE_BACK
                || keyCode == KeyEvent.KEYCODE_ESCAPE
                || keyCode == KeyEvent.KEYCODE_BUTTON_B)) {
                if (event.getAction() == KeyEvent.ACTION_DOWN && event.getRepeatCount() == 0) {
                    hideCustomView();
                }
                return true;
            }

            String command = remoteCommandForKey(keyCode);
            if (command != null) {
                if (event.getAction() == KeyEvent.ACTION_DOWN) {
                    boolean repeated = event.getRepeatCount() > 0;
                    if (!repeated || repeatableRemoteCommand(command)) {
                        sendRemoteCommand(command, repeated);
                    }
                }
                // Consume BOTH down and up. Letting ACTION_UP fall through can
                // still trigger WebView/HTMLMediaElement default key behavior.
                return true;
            }
        }
        return super.dispatchKeyEvent(event);
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
            if (tvDevice) {
                sendRemoteCommand("back", false);
            } else {
                webView.evaluateJavascript("try{if(typeof closePlayer==='function'){closePlayer();}else{StormFlixShell.close();}}catch(e){StormFlixShell.close();}", null);
            }
            return;
        }
        super.onBackPressed();
    }
}
