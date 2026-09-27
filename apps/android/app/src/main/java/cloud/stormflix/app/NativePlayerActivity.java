package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
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

/**
 * Native Android/Android TV/Fire TV playback route.
 *
 * PlaybackPlan remains authoritative. Media3 consumes the returned Direct Play,
 * remux/audio-compatibility or HLS transcode URL with normal HTTP Range requests.
 * The Web Playback Engine is kept only as a guarded fallback when a vendor
 * decoder/runtime rejects a route that the device reported as supported.
 */
public final class NativePlayerActivity extends Activity {
    private static final String VERSION = "0.7.0";

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
    private Button qualityButton;
    private ExoPlayer player;

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
    private long selectedProfileId;
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

        qualityButton = Ui.button(this, "Qualidade: " + qualityLabel(store.playerQuality()), false);
        qualityButton.setOnClickListener(v -> chooseQuality());
        FrameLayout.LayoutParams qualityLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), Gravity.END | Gravity.TOP);
        qualityLp.setMargins(Ui.dp(this, 18), Ui.dp(this, 18), Ui.dp(this, 24), Ui.dp(this, 18));
        root.addView(qualityButton, qualityLp);

        setContentView(root);
    }

    private void prepareMedia(long requestedMediaId, double resumeOverrideSeconds) {
        showLoading("Analisando o dispositivo e o arquivo…");
        releasePlayer(false);
        skippedAutomatically.clear();
        markers.clear();
        mediaId = requestedMediaId;
        progressSequence = 0;
        startupStartedAt = System.currentTimeMillis();
        firstFrameMs = 0;

        io.submit(() -> {
            try {
                JSONObject request = PlaybackCapabilities.buildRequest(this, store, "");
                JSONObject nextPlan = new JSONObject(api.post("/media/" + requestedMediaId + "/playback/plan", request));
                if (!nextPlan.optBoolean("available", false) || nextPlan.optString("url", "").trim().isEmpty()) {
                    throw new IllegalStateException(nextPlan.optString("reason", "Nenhuma rota compatível ficou disponível."));
                }

                JSONObject state = null;
                try {
                    state = new JSONObject(api.post("/media/" + requestedMediaId + "/playback/telemetry",
                        new JSONObject().put("operation", "playback_state_get").put("client_kind", "android_native")));
                } catch (Exception ignored) {}
                loadPlaybackState(state);
                loadSelectedProfile();

                plan = nextPlan;
                playbackSessionId = nextPlan.optString("playback_session_id", "");
                playbackMode = nextPlan.optString("mode", "direct_play");

                double resume = Double.isNaN(resumeOverrideSeconds)
                    ? nextPlan.optDouble("resume_position_seconds", 0)
                    : Math.max(0, resumeOverrideSeconds);
                if (Double.isNaN(resumeOverrideSeconds) && resume > 0 && rewindSeconds > 0) {
                    resume = Math.max(0, resume - rewindSeconds);
                }

                final double startAt = resume;
                main.post(() -> startNative(nextPlan, startAt));
            } catch (Exception error) {
                main.post(() -> showFatal(error.getMessage()));
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

    private void loadSelectedProfile() {
        try {
            JSONObject out = new JSONObject(api.get("/profiles"));
            selectedProfileId = out.optLong("selected_profile_id", 0);
            JSONArray profiles = out.optJSONArray("profiles");
            if (profiles == null) return;
            for (int i = 0; i < profiles.length(); i++) {
                JSONObject p = profiles.optJSONObject(i);
                if (p != null && p.optLong("id", 0) == selectedProfileId) {
                    autoplayNext = p.optBoolean("autoplay_next", true);
                    return;
                }
            }
        } catch (Exception ignored) {}
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
            ExoPlayer exo = new ExoPlayer.Builder(this).setMediaSourceFactory(mediaSources).build();
            player = exo;
            playerView.setPlayer(exo);

            MediaItem.Builder item = new MediaItem.Builder().setUri(source);
            if (isHls(currentPlan, source)) item.setMimeType(MimeTypes.APPLICATION_M3U8);

            exo.addListener(new Player.Listener() {
                @Override public void onPlaybackStateChanged(int playbackState) {
                    if (playbackState == Player.STATE_READY) {
                        if (firstFrameMs == 0) firstFrameMs = Math.max(0, System.currentTimeMillis() - startupStartedAt);
                        hideLoading();
                        sendTelemetry("");
                    } else if (playbackState == Player.STATE_ENDED) {
                        sendHeartbeat("ended", true);
                        handleEnded();
                    }
                }

                @Override public void onPlayerError(PlaybackException error) {
                    sendTelemetry(error == null ? "Media3 playback error" : String.valueOf(error.getMessage()));
                    fallbackToWeb(error == null ? "Media3 playback error" : error.getMessage());
                }
            });

            exo.setMediaItem(item.build(), Math.max(0L, (long)(startSeconds * 1000)));
            exo.prepare();
            exo.play();

            main.removeCallbacks(heartbeatTask);
            main.removeCallbacks(markerTask);
            main.postDelayed(heartbeatTask, 1500);
            main.postDelayed(markerTask, 500);
            qualityButton.setText("Qualidade: " + qualityLabel(store.playerQuality()));
        } catch (Exception error) {
            fallbackToWeb(error.getMessage());
        }
    }

    private String absolutePlanUrl(String source) {
        String value = source == null ? "" : source.trim();
        if (value.startsWith("http://") || value.startsWith("https://")) return value;
        if (value.startsWith("/api/")) return store.baseUrl() + value;
        if (value.startsWith("/")) return store.baseUrl() + "/api/v1" + value;
        return store.baseUrl() + "/api/v1/" + value;
    }

    private boolean isHls(JSONObject currentPlan, String url) {
        String lower = url == null ? "" : url.toLowerCase(Locale.ROOT);
        return lower.contains(".m3u8") || "hls".equalsIgnoreCase(currentPlan.optString("transport", ""))
            || "video_transcode".equalsIgnoreCase(currentPlan.optString("mode", ""));
    }

    private void chooseQuality() {
        final String[] labels = {"Automática", "Original", "4K", "1440p", "1080p", "720p", "480p"};
        final String[] values = {"auto", "original", "2160p", "1440p", "1080p", "720p", "480p"};
        new AlertDialog.Builder(this)
            .setTitle("Qualidade")
            .setItems(labels, (dialog, which) -> {
                double position = player == null ? 0 : player.getCurrentPosition() / 1000.0;
                store.setPlayerQuality(values[which]);
                qualityButton.setText("Qualidade: " + labels[which]);
                sendHeartbeat("quality_change", true);
                prepareMedia(mediaId, position);
            })
            .setNegativeButton("Cancelar", null)
            .show();
    }

    private String qualityLabel(String value) {
        String v = value == null ? "auto" : value;
        switch (v) {
            case "original": return "Original";
            case "2160p": return "4K";
            case "1440p": return "1440p";
            case "1080p": return "1080p";
            case "720p": return "720p";
            case "480p": return "480p";
            default: return "Auto";
        }
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
            try { api.post("/media/" + mediaId + "/playback", body); }
            catch (Exception ignored) {}
        });
    }

    private void sendTelemetry(String lastError) {
        JSONObject currentPlan = plan;
        if (currentPlan == null) return;
        JSONObject body = new JSONObject();
        try {
            body.put("playback_session_id", playbackSessionId);
            body.put("mode", playbackMode);
            body.put("client_kind", "android_native");
            body.put("bitrate_kbps", currentPlan.optLong("source_bitrate_kbps", 0));
            body.put("video_codec", currentPlan.optString("video_codec", ""));
            body.put("audio_codec", currentPlan.optString("audio_codec", ""));
            body.put("last_error", lastError == null ? "" : lastError);
            body.put("plan_ms", Math.max(0, System.currentTimeMillis() - startupStartedAt - firstFrameMs));
            body.put("first_frame_ms", firstFrameMs);
            body.put("startup_ms", firstFrameMs);
        } catch (Exception ignored) {}
        io.submit(() -> {
            try { api.post("/media/" + mediaId + "/playback/telemetry", body); }
            catch (Exception ignored) {}
        });
    }

    private void handleEnded() {
        if (!autoplayNext) return;
        io.submit(() -> {
            try {
                JSONObject neighbors = new JSONObject(api.get("/media/" + mediaId + "/neighbors"));
                JSONObject next = neighbors.optJSONObject("next");
                if (next == null || next.optLong("id", 0) <= 0) return;
                main.post(() -> {
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
                if (!dialog.isShowing()) return;
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

    private void fallbackToWeb(String error) {
        if (fallbackOpened || destroyed) return;
        fallbackOpened = true;
        sendHeartbeat("native_fallback", true);
        Toast.makeText(this, "Player nativo encontrou incompatibilidade; usando fallback StormFlix.", Toast.LENGTH_SHORT).show();
        Intent intent = new Intent(this, PlayerActivity.class);
        intent.putExtra("media_id", mediaId);
        if (error != null) intent.putExtra("native_error", error);
        startActivity(intent);
        finish();
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

    private void showFatal(String message) {
        hideLoading();
        new AlertDialog.Builder(this)
            .setTitle("Não foi possível reproduzir")
            .setMessage(message == null || message.trim().isEmpty() ? "Nenhuma rota compatível ficou disponível." : message)
            .setPositiveButton("Usar player de compatibilidade", (d, w) -> fallbackToWeb(message))
            .setNegativeButton("Fechar", (d, w) -> finish())
            .show();
    }

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
        enterImmersiveMode();
    }

    @Override protected void onPause() {
        sendHeartbeat("pause", true);
        super.onPause();
    }

    @Override protected void onDestroy() {
        destroyed = true;
        sendHeartbeat("stop", true);
        stopServerPlayback();
        releasePlayer(false);
        io.shutdownNow();
        getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        super.onDestroy();
    }

    @Override public void onBackPressed() {
        sendHeartbeat("stop", true);
        finish();
    }
}
