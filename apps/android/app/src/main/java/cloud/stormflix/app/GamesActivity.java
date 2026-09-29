package cloud.stormflix.app;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.WindowManager;
import android.webkit.CookieManager;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import java.net.URI;

/** Authenticated games runtime inside the APK. ROMs and saves keep server profile authorization. */
public final class GamesActivity extends Activity {
    private WebView web;
    private SessionStore store;
    private boolean ready;
    private boolean foreground;
    private int axisX, axisY;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        store = new SessionStore(this);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN |
            View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
        web = new WebView(this);
        web.setBackgroundColor(Color.BLACK);
        WebSettings settings = web.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setMediaPlaybackRequiresUserGesture(false);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setSupportMultipleWindows(false);
        settings.setUserAgentString(settings.getUserAgentString() + " StormFlixGames/0.9.0");
        CookieManager cookies = CookieManager.getInstance();
        cookies.setAcceptCookie(true);
        cookies.setAcceptThirdPartyCookies(web, false);
        String base = store.baseUrl();
        String suffix = "; Path=/; SameSite=Lax" + (base.startsWith("https://") ? "; Secure" : "");
        cookies.setCookie(base, "stormflix_session=" + store.sessionCookie() + suffix);
        cookies.setCookie(base, "stormflix_profile=" + store.profileCookie() + suffix);
        cookies.flush();
        web.setWebChromeClient(new WebChromeClient());
        web.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return !sameOrigin(request.getUrl().toString());
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, String url) { return !sameOrigin(url); }
            @Override public void onPageFinished(WebView view, String url) {
                ready = sameOrigin(url);
                if (ready && !foreground) call("background()");
            }
        });
        setContentView(web);
        web.loadUrl(base + "/?stormflix_native_games=1" + (RemoteUi.isTelevision(this) ? "&stormflix_tv=1" : ""));
    }

    private boolean sameOrigin(String url) {
        try {
            URI a = URI.create(store.baseUrl()), b = URI.create(url);
            int ap = a.getPort() < 0 ? ("https".equals(a.getScheme()) ? 443 : 80) : a.getPort();
            int bp = b.getPort() < 0 ? ("https".equals(b.getScheme()) ? 443 : 80) : b.getPort();
            return a.getScheme().equalsIgnoreCase(b.getScheme()) && a.getHost().equalsIgnoreCase(b.getHost()) && ap == bp;
        } catch (Exception e) { return false; }
    }
    private void call(String expression) {
        if (ready && web != null) web.evaluateJavascript("window.sfAndroidGames?." + expression, null);
    }
    @Override public void onBackPressed() {
        if (!ready) { finish(); return; }
        web.evaluateJavascript("window.sfAndroidGames?.back() || false", result -> {
            if (!"true".equals(result)) finish();
        });
    }
    @Override protected void onResume() { super.onResume(); foreground = true; if (web != null) web.onResume(); call("foreground()"); }
    @Override protected void onPause() {
        foreground = false;
        // Keep the JS runtime alive long enough to pause and upload saves. Do not
        // pause global WebView timers, which would interrupt save serialization.
        call("background()");
        axisX = axisY = 0;
        super.onPause();
    }
    @Override protected void onDestroy() {
        ready = false;
        if (web != null) { web.stopLoading(); web.loadUrl("about:blank"); web.destroy(); web = null; }
        super.onDestroy();
    }
    private String button(int code) {
        switch (code) {
            case KeyEvent.KEYCODE_DPAD_UP: return "up";
            case KeyEvent.KEYCODE_DPAD_DOWN: return "down";
            case KeyEvent.KEYCODE_DPAD_LEFT: return "left";
            case KeyEvent.KEYCODE_DPAD_RIGHT: return "right";
            case KeyEvent.KEYCODE_BUTTON_A: return "b";
            case KeyEvent.KEYCODE_BUTTON_B: return "a";
            case KeyEvent.KEYCODE_BUTTON_X: return "y";
            case KeyEvent.KEYCODE_BUTTON_Y: return "x";
            case KeyEvent.KEYCODE_BUTTON_L1: return "l";
            case KeyEvent.KEYCODE_BUTTON_R1: return "r";
            case KeyEvent.KEYCODE_BUTTON_L2: return "l2";
            case KeyEvent.KEYCODE_BUTTON_R2: return "r2";
            case KeyEvent.KEYCODE_BUTTON_START: return "start";
            case KeyEvent.KEYCODE_BUTTON_SELECT: return "select";
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER: return "confirm";
            case KeyEvent.KEYCODE_MENU: return "menu";
            default: return null;
        }
    }
    @Override public boolean dispatchKeyEvent(KeyEvent event) {
        String input = button(event.getKeyCode());
        if (ready && input != null && (event.getAction() == KeyEvent.ACTION_DOWN || event.getAction() == KeyEvent.ACTION_UP)) {
            if (event.getRepeatCount() == 0 || input.equals("up") || input.equals("down") || input.equals("left") || input.equals("right"))
                call("key('" + input + "'," + (event.getAction() == KeyEvent.ACTION_DOWN) + ")");
            return true;
        }
        return super.dispatchKeyEvent(event);
    }
    private int direction(float value) { return value > .45f ? 1 : value < -.45f ? -1 : 0; }
    @Override public boolean onGenericMotionEvent(MotionEvent event) {
        if (ready && (event.getSource() & android.view.InputDevice.SOURCE_JOYSTICK) == android.view.InputDevice.SOURCE_JOYSTICK) {
            float x = event.getAxisValue(MotionEvent.AXIS_HAT_X), y = event.getAxisValue(MotionEvent.AXIS_HAT_Y);
            if (Math.abs(x) < .1f) x = event.getAxisValue(MotionEvent.AXIS_X);
            if (Math.abs(y) < .1f) y = event.getAxisValue(MotionEvent.AXIS_Y);
            int nx = direction(x), ny = direction(y);
            if (nx != axisX) { if (axisX != 0) call("key('" + (axisX < 0 ? "left" : "right") + "',false)"); if (nx != 0) call("key('" + (nx < 0 ? "left" : "right") + "',true)"); axisX = nx; }
            if (ny != axisY) { if (axisY != 0) call("key('" + (axisY < 0 ? "up" : "down") + "',false)"); if (ny != 0) call("key('" + (ny < 0 ? "up" : "down") + "',true)"); axisY = ny; }
            return true;
        }
        return super.onGenericMotionEvent(event);
    }
}
