package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.net.Uri;
import android.content.ActivityNotFoundException;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.Toast;

import androidx.media3.common.MediaItem;
import androidx.media3.common.MimeTypes;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.datasource.DefaultHttpDataSource;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.DefaultLoadControl;
import androidx.media3.exoplayer.DefaultRenderersFactory;
import androidx.media3.common.AudioAttributes;
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory;
import androidx.media3.ui.PlayerView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Native original-file playback shared by phone, Android TV and Fire TV. */
public final class NativePlayerActivity extends Activity {
    private static final String VERSION = "0.8.2";

    private static final class Marker {
        final String kind;
        final double start;
        final double end;
        Marker(String kind, double start, double end) {
            this.kind = kind == null ? "" : kind;
            this.start = start;
            this.end = end;
        }
        String key() { return kind + ":" + start + ":" + end; }
    }

    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final List<Marker> markers = new ArrayList<>();
    private final Set<String> skippedAutomatically = new HashSet<>();

    private ApiClient api;
    private SessionStore store;
    private FrameLayout root;
    private PlayerView playerView;
    private ProgressBar loading;
    private TextView status;
    private Button skipButton;
    private int prepareGeneration;
    private long planMs;
    private long stalledAt;
    private int stallCount;
    private long lastStallMs;
    private ExoPlayer player;
    private boolean foreground;
    private boolean initialPlayPending;

    private long mediaId;
    private JSONObject plan;
    private String playbackSessionId = "";
    private String playbackMode = "direct_play";
    private String skipMode = "manual";
    private int rewindSeconds = 10;
    private boolean stillWatching = true;
    private int stillWatchingEpisodeLimit = 3;
    private int stillWatchingHours = 3;
    private int autoplayCountdown = 10;
    private boolean autoplayNext = true;
    private int chainCount;
    private long chainStartedAt = System.currentTimeMillis();
    private long progressSequence;
    private boolean fallbackOpened;
    private boolean destroyed;
    private long startupStartedAt;
    private long firstFrameMs;

    private final Runnable heartbeatTask = new Runnable() {
        @Override public void run() {
            sendHeartbeat("periodic", false);
            if (!destroyed) main.postDelayed(this, 7000);
        }
    };

    private final Runnable markerTask = new Runnable() {
        @Override public void run() {
            updateMarkerUI();
            if (!destroyed) main.postDelayed(this, 400);
        }
    };

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        mediaId = getIntent().getLongExtra("media_id", 0L);
        if (mediaId <= 0L) { finish(); return; }
        api = new ApiClient(this);
        store = api.store();
        configureWindow();
        buildShell();
        prepareMedia(mediaId, Double.NaN);
    }

    private void configureWindow() {
        Window window = getWindow();
        window.setStatusBarColor(Color.BLACK);
        window.setNavigationBarColor(Color.BLACK);
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        enterImmersiveMode();
    }

    private void buildShell() {
        root = new FrameLayout(this);
        root.setBackgroundColor(Color.BLACK);

        playerView = new PlayerView(this);
        playerView.setBackgroundColor(Color.BLACK);
        playerView.setUseController(true);
        playerView.setControllerAutoShow(true);
        playerView.setControllerHideOnTouch(true);
        playerView.setKeepContentOnPlayerReset(true);
        playerView.setShowSubtitleButton(true);
        playerView.setControllerShowTimeoutMs(3000);
        root.addView(playerView, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        loading = new ProgressBar(this);
        FrameLayout.LayoutParams loadingLp = new FrameLayout.LayoutParams(Ui.dp(this, 54), Ui.dp(this, 54), Gravity.CENTER);
        root.addView(loading, loadingLp);

        status = Ui.muted(this, "Preparando reprodução…", 13);
        status.setGravity(Gravity.CENTER);
        FrameLayout.LayoutParams statusLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, Gravity.CENTER_HORIZONTAL | Gravity.CENTER_VERTICAL);
        statusLp.topMargin = Ui.dp(this, 84);
        root.addView(status, statusLp);

        skipButton = Ui.button(this, "Pular introdução", true);
        skipButton.setVisibility(View.GONE);
        skipButton.setOnClickListener(v -> {
            Marker marker = activeMarker();
            if (marker != null && player != null) player.seekTo(Math.max(0L, (long)((marker.end + 0.05) * 1000)));
        });
        FrameLayout.LayoutParams skipLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 48), Gravity.END | Gravity.BOTTOM);
        skipLp.setMargins(Ui.dp(this, 18), Ui.dp(this, 18), Ui.dp(this, 28), Ui.dp(this, 92));
        root.addView(skipButton, skipLp);

        setContentView(root);
    }

    private boolean preferSoftwareAudio = false;

    private void prepareMedia(long requestedMediaId, double resumeOverrideSeconds) {
        preferSoftwareAudio = false;
        final int generation = ++prepareGeneration;
        fallbackOpened = false;
        showLoading("Abrindo vídeo…");
        if (plan != null || player != null) {
            sendHeartbeat("source_change", true);
            stopServerPlayback();
        }
        releasePlayer(false);
        playbackSessionId = "";
        playbackMode = "direct_play";
        plan = null;
        skippedAutomatically.clear();
        markers.clear();
        mediaId = requestedMediaId;
        progressSequence = 0;
        startupStartedAt = System.currentTimeMillis();
        firstFrameMs = 0;
        planMs = 0; stalledAt = 0; stallCount = 0; lastStallMs = 0;

        io.submit(() -> {
            try {
                JSONObject nextPlan;
                try {
                    nextPlan = new JSONObject(api.post("/media/" + requestedMediaId + "/playback/original", new JSONObject()));
                } catch (ApiClient.ApiException error) {
                    if (error.status != 404) throw error;
                    // Older servers lack /original. Authorize/read catalog metadata,
                    // then let the embedded decoder inspect the original stream.
                    // Never ask the old planner to prepare a converted source.
                    JSONObject metadata = new JSONObject(api.get("/media/" + requestedMediaId));
                    nextPlan = new JSONObject();
                    nextPlan.put("available", true).put("mode", "direct_play")
                        .put("media_id", requestedMediaId).put("original_only", true)
                        .put("url", "/api/v1/media/" + requestedMediaId + "/stream")
                        .put("resume_position_seconds", metadata.optDouble("position_seconds", 0));
                }
                if (!nextPlan.optBoolean("available", false) ||
                        !"direct_play".equals(nextPlan.optString("mode")) || nextPlan.optString("url").isEmpty()) {
                    throw new IllegalStateException("O servidor não retornou o arquivo original. Atualize o servidor StormFlix e tente novamente.");
                }
                final JSONObject readyPlan = nextPlan;
                final long elapsed = Math.max(0, System.currentTimeMillis() - startupStartedAt);
                main.post(() -> {
                    if (destroyed || generation != prepareGeneration) return;
                    loadPlaybackState(readyPlan);
                    autoplayNext = readyPlan.optBoolean("autoplay_next", true);
                    plan = readyPlan;
                    planMs = elapsed;
                    playbackSessionId = readyPlan.optString("playback_session_id", "");
                    playbackMode = "direct_play";
                    double resume = Double.isNaN(resumeOverrideSeconds)
                        ? Math.max(0, readyPlan.optDouble("resume_position_seconds", 0) - rewindSeconds)
                        : Math.max(0, resumeOverrideSeconds);
                    startNative(readyPlan, resume);
                });
            } catch (Exception error) {
                main.post(() -> { if (!destroyed && generation == prepareGeneration) showFatal(error.getMessage()); });
            }
        });
    }

    private void loadPlaybackState(JSONObject state) {
        if (state == null) return;
        JSONObject prefs = state.optJSONObject("playback_preferences");
        if (prefs != null) {
            skipMode = prefs.optString("skip_mode", "manual");
            rewindSeconds = prefs.optInt("rewind_seconds", 10);
            stillWatching = prefs.optBoolean("still_watching", true);
            stillWatchingEpisodeLimit = Math.max(1, prefs.optInt("still_watching_episode_limit", 3));
            stillWatchingHours = Math.max(1, prefs.optInt("still_watching_hours", 3));
            autoplayCountdown = Math.max(0, prefs.optInt("autoplay_countdown", 10));
        }
        JSONArray list = state.optJSONArray("markers");
        if (list != null) {
            for (int i = 0; i < list.length(); i++) {
                JSONObject marker = list.optJSONObject(i);
                if (marker == null) continue;
                double start = marker.optDouble("start_seconds", -1);
                double end = marker.optDouble("end_seconds", -1);
                if (start >= 0 && end > start) markers.add(new Marker(marker.optString("kind", ""), start, end));
            }
        }
    }

    private void startNative(JSONObject currentPlan, double startSeconds) {
        if (destroyed) return;
        try {
            String source = absolutePlanUrl(currentPlan.optString("url", ""));
            Map<String,String> headers = new HashMap<>();
            String cookie = store.cookieHeader();
            if (!cookie.isEmpty()) headers.put("Cookie", cookie);

            DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
                .setUserAgent("StormFlix-Android-Native/" + VERSION)
                .setAllowCrossProtocolRedirects(true)
                .setConnectTimeoutMs(15000)
                .setReadTimeoutMs(60000);
            if (!headers.isEmpty()) http.setDefaultRequestProperties(headers);

            DefaultMediaSourceFactory mediaSources = new DefaultMediaSourceFactory(http);
            ExoPlayer exo = new ExoPlayer.Builder(this, new DefaultRenderersFactory(this).setEnableDecoderFallback(true)
                .setExtensionRendererMode(preferSoftwareAudio ? DefaultRenderersFactory.EXTENSION_RENDERER_MODE_PREFER
                    : DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON))
                .setMediaSourceFactory(mediaSources)
                .setLoadControl(new DefaultLoadControl.Builder().setBufferDurationsMs(15000, 50000, 750, 1500).build())
                .build();
            exo.setAudioAttributes(AudioAttributes.DEFAULT, true);
            exo.setHandleAudioBecomingNoisy(true);
            exo.setTrackSelectionParameters(
                exo.getTrackSelectionParameters().buildUpon()
                    .setPreferredAudioLanguages(store.preferredAudio(), "pt-BR", "pt")
                    .setPreferredTextLanguages(store.preferredSubtitle(), "pt-BR", "pt")
                    .build()
            );
            player = exo;
            playerView.setPlayer(exo);

            MediaItem.Builder item = new MediaItem.Builder().setUri(source);
            JSONArray subtitles = currentPlan.optJSONArray("subtitles");
            List<MediaItem.SubtitleConfiguration> subtitleItems = new ArrayList<>();
            if (subtitles != null) for (int i = 0; i < subtitles.length(); i++) {
                JSONObject subtitle = subtitles.optJSONObject(i);
                if (subtitle == null) continue;
                String format = subtitle.optString("format");
                if (!"srt".equalsIgnoreCase(format) && !"vtt".equalsIgnoreCase(format)) continue;
                subtitleItems.add(new MediaItem.SubtitleConfiguration.Builder(Uri.parse(
                    store.baseUrl() + "/api/v1/media/" + mediaId + "/subtitles/" + subtitle.optLong("id") + "/vtt"))
                    .setMimeType(MimeTypes.TEXT_VTT).setLanguage(subtitle.optString("language"))
                    .setLabel(subtitle.optString("language", "Legenda")).build());
            }
            item.setSubtitleConfigurations(subtitleItems);

            exo.addListener(new Player.Listener() {
                @Override public void onPlaybackStateChanged(int playbackState) {
                    if (playbackState == Player.STATE_READY) {
                        if (stalledAt > 0) {
                            lastStallMs = Math.max(0, System.currentTimeMillis() - stalledAt);
                            stalledAt = 0; stallCount++;
                        }
                        hideLoading();
                        sendTelemetry("");
                    } else if (playbackState == Player.STATE_BUFFERING && firstFrameMs > 0) {
                        if (stalledAt == 0) stalledAt = System.currentTimeMillis();
                    } else if (playbackState == Player.STATE_ENDED) {
                        sendHeartbeat("ended", true);
                        handleEnded();
                    }
                }

                @Override public void onRenderedFirstFrame() {
                    if (firstFrameMs == 0) firstFrameMs = Math.max(1, System.currentTimeMillis() - startupStartedAt);
                    sendTelemetry("");
                }

                @Override public void onPlayerError(PlaybackException error) {
                    if (destroyed || player != exo) return;
                    sendTelemetry(error == null ? "Media3 playback error" : String.valueOf(error.getMessage()));
                    if (!preferSoftwareAudio && error != null && error.errorCode >= 4000 && error.errorCode < 6000) {
                        // Retry a vendor decoder/audio-sink failure once with the
                        // bundled FFmpeg audio renderer, inside this same screen.
                        preferSoftwareAudio = true;
                        double resume = Math.max(startSeconds, exo.getCurrentPosition() / 1000.0);
                        releasePlayer(false);
                        showLoading("Reabrindo com o decoder integrado…");
                        startNative(currentPlan, resume);
                        return;
                    }
                    showPlaybackFailure(playbackErrorMessage(error));
                }
            });

            exo.setMediaItem(item.build(), Math.max(0L, (long)(startSeconds * 1000)));
            exo.prepare();
            initialPlayPending = !foreground;
            if (foreground) exo.play();

            main.removeCallbacks(heartbeatTask);
            main.removeCallbacks(markerTask);
            main.postDelayed(heartbeatTask, 1500);
            main.postDelayed(markerTask, 500);

        } catch (Exception error) {
            showPlaybackFailure(error.getMessage());
        }
    }

    private String absolutePlanUrl(String source) {
        String value = source == null ? "" : source.trim();
        if (value.startsWith("http://") || value.startsWith("https://")) return value;
        if (value.startsWith("/api/")) return store.baseUrl() + value;
        if (value.startsWith("/")) return store.baseUrl() + "/api/v1" + value;
        return store.baseUrl() + "/api/v1/" + value;
    }

    private Marker activeMarker() {
        if (player == null || "disabled".equalsIgnoreCase(skipMode)) return null;
        double at = player.getCurrentPosition() / 1000.0;
        for (Marker marker : markers) if (at >= marker.start && at < marker.end - 0.05) return marker;
        return null;
    }

    private void updateMarkerUI() {
        Marker marker = activeMarker();
        if (marker == null) {
            skipButton.setVisibility(View.GONE);
            return;
        }
        if ("automatic".equalsIgnoreCase(skipMode)) {
            if (skippedAutomatically.add(marker.key()) && player != null) {
                player.seekTo(Math.max(0L, (long)((marker.end + 0.05) * 1000)));
            }
            skipButton.setVisibility(View.GONE);
            return;
        }
        skipButton.setText("credits".equalsIgnoreCase(marker.kind) ? "Pular créditos" : "Pular introdução");
        skipButton.setVisibility(View.VISIBLE);
    }

    private void sendHeartbeat(String reason, boolean immediate) {
        ExoPlayer exo = player;
        if (exo == null) return;
        long positionMs = exo.getCurrentPosition();
        long durationMs = exo.getDuration();
        boolean playing = exo.isPlaying();
        JSONObject currentPlan = plan;
        long seq = ++progressSequence;
        final long targetMediaId = mediaId;
        JSONObject body = new JSONObject();
        try {
            body.put("position_seconds", Math.max(0, positionMs) / 1000.0);
            body.put("duration_seconds", durationMs > 0 ? durationMs / 1000.0 : 0);
            body.put("state", playing ? "playing" : "paused");
            body.put("mode", playbackMode);
            body.put("video_codec", currentPlan == null ? "" : currentPlan.optString("video_codec", ""));
            body.put("audio_codec", currentPlan == null ? "" : currentPlan.optString("audio_codec", ""));
            body.put("source_audio_codec", currentPlan == null ? "" : currentPlan.optString("source_audio_codec", ""));
            body.put("bitrate_kbps", currentPlan == null ? 0 : currentPlan.optLong("source_bitrate_kbps", 0));
            body.put("playback_session_id", playbackSessionId);
            body.put("progress_sequence", seq);
            body.put("progress_event_ms", System.currentTimeMillis());
            body.put("progress_reason", reason == null ? "periodic" : reason);
        } catch (Exception ignored) {}
        io.submit(() -> {
            try { api.post("/media/" + targetMediaId + "/playback", body); }
            catch (Exception ignored) {}
        });
    }

    private void sendTelemetry(String lastError) {
        JSONObject currentPlan = plan;
        if (currentPlan == null) return;
        final long targetMediaId = mediaId;
        JSONObject body = new JSONObject();
        try {
            body.put("playback_session_id", playbackSessionId);
            body.put("mode", playbackMode);
            body.put("client_kind", "android_native");
            body.put("bitrate_kbps", currentPlan.optLong("source_bitrate_kbps", 0));
            body.put("video_codec", currentPlan.optString("video_codec", ""));
            body.put("audio_codec", currentPlan.optString("audio_codec", ""));
            body.put("last_error", lastError == null ? "" : lastError);
            body.put("plan_ms", planMs);
            body.put("first_frame_ms", firstFrameMs);
            body.put("startup_ms", firstFrameMs);
            body.put("stall_count", stallCount);
            body.put("last_stall_ms", lastStallMs);
        } catch (Exception ignored) {}
        io.submit(() -> {
            try { api.post("/media/" + targetMediaId + "/playback/telemetry", body); }
            catch (Exception ignored) {}
        });
    }

    private void handleEnded() {
        if (!autoplayNext || destroyed) return;
        final long endedMediaId = mediaId;
        final int generation = prepareGeneration;
        io.submit(() -> {
            try {
                JSONObject neighbors = new JSONObject(api.get("/media/" + endedMediaId + "/neighbors"));
                JSONObject next = neighbors.optJSONObject("next");
                if (next == null || next.optLong("id", 0) <= 0) return;
                main.post(() -> {
                    if (destroyed || generation != prepareGeneration) return;
                    if (needsStillWatchingConfirmation()) showStillWatching(next);
                    else showNextCountdown(next);
                });
            } catch (Exception ignored) {}
        });
    }

    private boolean needsStillWatchingConfirmation() {
        if (!stillWatching) return false;
        long elapsed = System.currentTimeMillis() - chainStartedAt;
        return chainCount >= stillWatchingEpisodeLimit || elapsed >= stillWatchingHours * 3600000L;
    }

    private void showStillWatching(JSONObject next) {
        new AlertDialog.Builder(this)
            .setTitle("Ainda está assistindo?")
            .setMessage("O próximo episódio só começa depois da sua confirmação.")
            .setPositiveButton("Sim, continuar", (d, w) -> {
                chainCount = 0;
                chainStartedAt = System.currentTimeMillis();
                showNextCountdown(next);
            })
            .setNegativeButton("Parar", null)
            .show();
    }

    private void showNextCountdown(JSONObject next) {
        long nextId = next.optLong("id", 0);
        if (nextId <= 0) return;
        if (autoplayCountdown <= 0) {
            startNext(nextId, true);
            return;
        }
        TextView message = Ui.muted(this, "", 16);
        message.setPadding(Ui.dp(this, 22), Ui.dp(this, 8), Ui.dp(this, 22), Ui.dp(this, 8));
        final int[] remaining = {autoplayCountdown};
        AlertDialog dialog = new AlertDialog.Builder(this)
            .setTitle(next.optString("title", "Próximo episódio"))
            .setView(message)
            .setPositiveButton("Reproduzir agora", null)
            .setNegativeButton("Cancelar", null)
            .create();
        Runnable tick = new Runnable() {
            @Override public void run() {
                if (!dialog.isShowing() || destroyed) return;
                if (!foreground) { main.postDelayed(this, 1000); return; }
                message.setText("Reprodução automática em " + remaining[0] + "s.");
                if (remaining[0] <= 0) {
                    dialog.dismiss();
                    startNext(nextId, true);
                    return;
                }
                remaining[0]--;
                main.postDelayed(this, 1000);
            }
        };
        dialog.setOnShowListener(v -> {
            dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(x -> {
                main.removeCallbacks(tick);
                dialog.dismiss();
                startNext(nextId, false);
            });
            main.post(tick);
        });
        dialog.setOnDismissListener(v -> main.removeCallbacks(tick));
        dialog.show();
    }

    private void startNext(long nextId, boolean automatic) {
        if (automatic) chainCount++;
        prepareMedia(nextId, 0);
    }

    private void showPlaybackFailure(String error) {
        if (fallbackOpened || destroyed) return;
        fallbackOpened = true;
        if (player != null) player.pause();
        hideLoading();
        new AlertDialog.Builder(this)
            .setTitle("Não foi possível abrir este vídeo")
            .setMessage(error == null || error.isEmpty() ? "A reprodução interna falhou. Tente novamente." : error)
            .setPositiveButton("Tentar novamente", (d, w) -> {
                double position = player == null ? Double.NaN : player.getCurrentPosition() / 1000.0;
                prepareMedia(mediaId, position);
            })
            .setNegativeButton("Voltar", (d, w) -> finish())
            .setOnCancelListener(d -> finish())
            .show();
    }

    private String playbackErrorMessage(PlaybackException error) {
        if (error == null) return "A reprodução interna falhou. Tente novamente.";
        Throwable cause = error;
        while (cause != null) {
            if (cause instanceof androidx.media3.datasource.HttpDataSource.InvalidResponseCodeException) {
                int code = ((androidx.media3.datasource.HttpDataSource.InvalidResponseCodeException) cause).responseCode;
                if (code == 401) return "Sua sessão expirou. Entre novamente no StormFlix.";
                if (code == 403) return "Este perfil não tem acesso ao vídeo.";
                if (code == 404) return "O arquivo não está disponível no servidor. Verifique a biblioteca e o rclone.";
                return "O servidor não entregou o vídeo (HTTP " + code + "). Tente novamente.";
            }
            cause = cause.getCause();
        }
        if (error.errorCode >= 2000 && error.errorCode < 3000)
            return "A leitura do vídeo foi interrompida. Verifique a conexão com o servidor e tente novamente. (" + error.getErrorCodeName() + ")";
        return "O decoder integrado não conseguiu reproduzir este arquivo neste aparelho. (" + error.getErrorCodeName() + ")";
    }

    private void showLoading(String message) {
        loading.setVisibility(View.VISIBLE);
        status.setText(message == null ? "Preparando reprodução…" : message);
        status.setVisibility(View.VISIBLE);
    }

    private void hideLoading() {
        loading.setVisibility(View.GONE);
        status.setVisibility(View.GONE);
    }

    private void showFatal(String message) { showPlaybackFailure(message); }

    private void releasePlayer(boolean finishing) {
        main.removeCallbacks(heartbeatTask);
        main.removeCallbacks(markerTask);
        ExoPlayer exo = player;
        if (exo != null) {
            if (finishing) sendHeartbeat("stop", true);
            playerView.setPlayer(null);
            exo.release();
            player = null;
        }
    }

    private void stopServerPlayback() {
        final String session = playbackSessionId;
        final long id = mediaId;
        io.submit(() -> {
            try {
                String path = "/media/" + id + "/playback";
                if (session != null && !session.isEmpty()) {
                    path += "?session=" + URLEncoder.encode(session, StandardCharsets.UTF_8.name());
                }
                api.delete(path);
            } catch (Exception ignored) {}
        });
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

    @Override protected void onResume() {
        super.onResume();
        foreground = true;
        if (initialPlayPending && player != null) { initialPlayPending = false; player.play(); }
        enterImmersiveMode();
    }

    @Override protected void onPause() {
        foreground = false;
        if (player != null) player.pause();
        sendHeartbeat("pause", true);
        super.onPause();
    }

    @Override protected void onDestroy() {
        destroyed = true;
        prepareGeneration++;
        main.removeCallbacksAndMessages(null);
        sendHeartbeat("stop", true);
        stopServerPlayback();
        releasePlayer(false);
        io.shutdown();
        getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        super.onDestroy();
    }

    @Override public void onBackPressed() {
        sendHeartbeat("stop", true);
        finish();
    }
}
