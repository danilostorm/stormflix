package cloud.stormflix.app;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;

import androidx.mediarouter.app.MediaRouteChooserDialog;

import com.google.android.gms.cast.MediaInfo;
import com.google.android.gms.cast.MediaLoadRequestData;
import com.google.android.gms.cast.MediaMetadata;
import com.google.android.gms.cast.framework.CastContext;
import com.google.android.gms.cast.framework.CastSession;
import com.google.android.gms.cast.framework.SessionManagerListener;
import com.google.android.gms.cast.framework.media.RemoteMediaClient;

import org.json.JSONObject;

import java.util.Locale;

/**
 * Native Android bridge for Playback Anywhere.
 *
 * The Web UI remains responsible for asking StormFlix for the authoritative
 * PlaybackPlan + short-lived playback grant. This class only hands that already
 * authorized URL to Android routing/player APIs, so no cookie/session secret is
 * exposed to a receiver application.
 */
public final class PlaybackAnywhereNative {
    public static final String WEB_VIDEO_CAST_PACKAGE = "com.instantbits.cast.webvideo";

    private final Activity activity;
    private final WebView webView;
    private final Handler main = new Handler(Looper.getMainLooper());
    private SessionManagerListener<CastSession> pendingListener;
    private PendingCast pendingCast;

    PlaybackAnywhereNative(Activity activity, WebView webView) {
        this.activity = activity;
        this.webView = webView;
    }

    @JavascriptInterface
    public boolean isAvailable() {
        return true;
    }

    @JavascriptInterface
    public boolean isWebVideoCastInstalled() {
        try {
            activity.getPackageManager().getPackageInfo(WEB_VIDEO_CAST_PACKAGE, 0);
            return true;
        } catch (PackageManager.NameNotFoundException ignored) {
            return false;
        }
    }

    @JavascriptInterface
    public void openExternalPlayer(String url, String title, String mime) {
        main.post(() -> {
            try {
                Intent target = mediaIntent(url, mime);
                Intent chooser = Intent.createChooser(target, "Abrir vídeo com…");
                activity.startActivity(chooser);
                emit("external_opened", "Escolha VLC, MX Player, Web Video Cast ou outro player instalado.");
            } catch (ActivityNotFoundException error) {
                emit("error", "Nenhum player externo compatível foi encontrado neste aparelho.");
            } catch (Exception error) {
                emit("error", safeMessage(error, "Não foi possível abrir o seletor de players."));
            }
        });
    }

    @JavascriptInterface
    public void openWebVideoCast(String url, String title, String mime) {
        main.post(() -> {
            if (!isWebVideoCastInstalled()) {
                emit("wvc_missing", "Web Video Cast não está instalado neste aparelho.");
                return;
            }
            try {
                Intent intent = mediaIntent(url, mime);
                intent.setPackage(WEB_VIDEO_CAST_PACKAGE);
                activity.startActivity(intent);
                emit("wvc_opened", "Stream enviado ao Web Video Cast. Escolha Roku, DLNA, Fire TV ou outra TV dentro dele.");
            } catch (ActivityNotFoundException error) {
                emit("wvc_missing", "Web Video Cast está instalado, mas não aceitou este tipo de stream.");
            } catch (Exception error) {
                emit("error", safeMessage(error, "Não foi possível abrir o Web Video Cast."));
            }
        });
    }

    @JavascriptInterface
    public void openNativeCast(String url, String title, String mime, double startSeconds) {
        main.post(() -> startNativeCast(url, title, mime, startSeconds));
    }

    private Intent mediaIntent(String rawUrl, String rawMime) {
        Uri uri = Uri.parse(rawUrl);
        String mime = normalizeMime(rawMime, rawUrl);
        Intent intent = new Intent(Intent.ACTION_VIEW);
        intent.setDataAndType(uri, mime);
        intent.addCategory(Intent.CATEGORY_BROWSABLE);
        intent.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return intent;
    }

    private void startNativeCast(String url, String title, String mime, double startSeconds) {
        if (url == null || url.trim().isEmpty()) {
            emit("error", "StormFlix não recebeu uma URL válida para transmitir.");
            return;
        }
        try {
            CastContext context = CastContext.getSharedInstance(activity);
            CastSession current = context.getSessionManager().getCurrentCastSession();
            pendingCast = new PendingCast(url, title, normalizeMime(mime, url), Math.max(0d, startSeconds));
            if (current != null && current.isConnected()) {
                loadPendingCast(current);
                return;
            }

            removePendingListener(context);
            pendingListener = new SessionManagerListener<CastSession>() {
                @Override public void onSessionStarting(CastSession session) {}

                @Override public void onSessionStarted(CastSession session, String sessionId) {
                    loadPendingCast(session);
                    removePendingListener(context);
                }

                @Override public void onSessionStartFailed(CastSession session, int error) {
                    emit("error", "Não foi possível iniciar a sessão Chromecast (" + error + ").");
                    removePendingListener(context);
                }

                @Override public void onSessionEnding(CastSession session) {}
                @Override public void onSessionEnded(CastSession session, int error) {}
                @Override public void onSessionResuming(CastSession session, String sessionId) {}

                @Override public void onSessionResumed(CastSession session, boolean wasSuspended) {
                    loadPendingCast(session);
                    removePendingListener(context);
                }

                @Override public void onSessionResumeFailed(CastSession session, int error) {
                    emit("error", "Não foi possível retomar a sessão Chromecast (" + error + ").");
                    removePendingListener(context);
                }

                @Override public void onSessionSuspended(CastSession session, int reason) {}
            };
            context.getSessionManager().addSessionManagerListener(pendingListener, CastSession.class);

            emit("cast_searching", "Abrindo o seletor nativo do Google Cast…");
            MediaRouteChooserDialog dialog = new MediaRouteChooserDialog(activity);
            dialog.setRouteSelector(context.getMergedSelector());
            dialog.show();

            // Avoid retaining the Activity indefinitely if the user simply leaves
            // the chooser open or dismisses it without creating a session.
            main.postDelayed(() -> {
                if (pendingListener != null) {
                    removePendingListener(context);
                    if (pendingCast != null) emit("cast_cancelled", "Seleção de Chromecast encerrada.");
                }
            }, 60_000L);
        } catch (Exception error) {
            emit("error", safeMessage(error, "Google Cast não está disponível neste aparelho."));
        }
    }

    private void loadPendingCast(CastSession session) {
        PendingCast item = pendingCast;
        if (item == null || session == null || !session.isConnected()) return;
        try {
            RemoteMediaClient remote = session.getRemoteMediaClient();
            if (remote == null) {
                emit("error", "O Chromecast conectou, mas o player remoto não ficou disponível.");
                return;
            }
            MediaMetadata metadata = new MediaMetadata(MediaMetadata.MEDIA_TYPE_MOVIE);
            metadata.putString(MediaMetadata.KEY_TITLE, item.title.isEmpty() ? "StormFlix" : item.title);
            metadata.putString(MediaMetadata.KEY_SUBTITLE, "StormFlix");
            MediaInfo info = new MediaInfo.Builder(item.url)
                .setStreamType(MediaInfo.STREAM_TYPE_BUFFERED)
                .setContentType(item.mime)
                .setMetadata(metadata)
                .build();
            MediaLoadRequestData request = new MediaLoadRequestData.Builder()
                .setMediaInfo(info)
                .setAutoplay(true)
                .setCurrentTime((long) (item.startSeconds * 1000d))
                .build();
            remote.load(request);
            pendingCast = null;
            emit("cast_connected", "Transmitindo pelo Google Cast nativo.");
        } catch (Exception error) {
            emit("error", safeMessage(error, "Não foi possível carregar a mídia no Chromecast."));
        }
    }

    private void removePendingListener(CastContext context) {
        if (pendingListener == null) return;
        try {
            context.getSessionManager().removeSessionManagerListener(pendingListener, CastSession.class);
        } catch (Exception ignored) {}
        pendingListener = null;
    }

    private String normalizeMime(String rawMime, String url) {
        String mime = rawMime == null ? "" : rawMime.trim();
        String lower = url == null ? "" : url.toLowerCase(Locale.ROOT);
        if (mime.isEmpty()) {
            mime = lower.contains(".m3u8") ? "application/x-mpegURL" : "video/mp4";
        }
        return mime;
    }

    private String safeMessage(Exception error, String fallback) {
        String value = error == null ? "" : String.valueOf(error.getMessage()).trim();
        return value.isEmpty() ? fallback : value;
    }

    private void emit(String state, String message) {
        if (webView == null) return;
        String script = "try{window.sfPlaybackAnywhereNativeResult&&window.sfPlaybackAnywhereNativeResult("
            + JSONObject.quote(state == null ? "" : state) + ","
            + JSONObject.quote(message == null ? "" : message) + ");}catch(e){}";
        webView.evaluateJavascript(script, null);
    }

    private static final class PendingCast {
        final String url;
        final String title;
        final String mime;
        final double startSeconds;

        PendingCast(String url, String title, String mime, double startSeconds) {
            this.url = url;
            this.title = title == null ? "" : title.trim();
            this.mime = mime;
            this.startSeconds = startSeconds;
        }
    }
}
