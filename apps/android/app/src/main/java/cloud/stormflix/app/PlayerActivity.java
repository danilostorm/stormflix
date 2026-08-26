package cloud.stormflix.app;

import android.app.Activity;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.Toast;

import androidx.media3.common.MediaItem;
import androidx.media3.common.MimeTypes;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.datasource.DefaultDataSource;
import androidx.media3.datasource.DefaultHttpDataSource;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory;
import androidx.media3.ui.PlayerView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class PlayerActivity extends Activity {
    private final ExecutorService io = Executors.newFixedThreadPool(2);
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ExoPlayer player;
    private PlayerView playerView;
    private TextView badge;
    private long mediaId;
    private long streamMediaId;
    private String title;
    private boolean versionFallbackAttempted;
    private boolean remuxAttempted;
    private String playbackMode = "direct";
    private final Runnable heartbeat = new Runnable() {
        @Override public void run() {
            sendHeartbeat();
            main.postDelayed(this, 10000);
        }
    };

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        mediaId = getIntent().getLongExtra("media_id", 0);
        streamMediaId = mediaId;
        title = getIntent().getStringExtra("title");
        if (mediaId <= 0) { finish(); return; }
        api = new ApiClient(this);
        immersive();
        buildPlayer();
        loadSource(mediaId, "DIRECT PLAY", true);
    }

    private void buildPlayer() {
        Map<String,String> headers = new HashMap<>();
        String cookie = api.store().cookieHeader();
        if (!cookie.isEmpty()) headers.put("Cookie", cookie);
        headers.put("User-Agent", "StormFlix-Android/0.1.2");
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("StormFlix-Android/0.1.2")
            .setAllowCrossProtocolRedirects(true)
            .setDefaultRequestProperties(headers);
        DefaultDataSource.Factory data = new DefaultDataSource.Factory(this, http);
        player = new ExoPlayer.Builder(this)
            .setMediaSourceFactory(new DefaultMediaSourceFactory(data))
            .build();

        FrameLayout root = new FrameLayout(this); root.setBackgroundColor(Color.BLACK);
        playerView = new PlayerView(this); playerView.setPlayer(player); playerView.setUseController(true); playerView.setKeepScreenOn(true);
        root.addView(playerView, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        badge = new TextView(this); badge.setText("DIRECT PLAY"); badge.setTextColor(Color.rgb(117,255,170)); badge.setTextSize(11); badge.setPadding(Ui.dp(this,10),Ui.dp(this,6),Ui.dp(this,10),Ui.dp(this,6)); badge.setBackground(Ui.round(Color.argb(210,15,35,25),8));
        FrameLayout.LayoutParams bp = new FrameLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, Gravity.TOP|Gravity.END); bp.setMargins(0,Ui.dp(this,18),Ui.dp(this,18),0); root.addView(badge,bp);
        setContentView(root);

        player.addListener(new Player.Listener() {
            @Override public void onPlayerError(PlaybackException error) { handlePlaybackFailure(error); }
            @Override public void onPlaybackStateChanged(int state) { if (state == Player.STATE_ENDED) sendHeartbeat(); }
        });
        main.postDelayed(heartbeat, 10000);
    }

    private void loadSource(long sourceId, String label, boolean autoPlay) {
        streamMediaId = sourceId;
        badge.setText(label);
        badge.setTextColor(label.contains("1080") || label.contains("720") ? Color.rgb(126,200,255) : Color.rgb(117,255,170));
        io.submit(() -> {
            List<MediaItem.SubtitleConfiguration> subtitles = new ArrayList<>();
            try {
                JSONArray arr = new JSONArray(api.get("/media/" + sourceId + "/subtitles"));
                for (int i=0;i<arr.length();i++) {
                    JSONObject s = arr.optJSONObject(i); if (s==null) continue;
                    long id=s.optLong("id"); String lang=s.optString("language","pt");
                    Uri uri=Uri.parse(api.apiUrl("/media/"+sourceId+"/subtitles/"+id+"/vtt"));
                    subtitles.add(new MediaItem.SubtitleConfiguration.Builder(uri).setMimeType(MimeTypes.TEXT_VTT).setLanguage(lang).setLabel(lang).build());
                }
            } catch (Exception ignored) {}
            main.post(() -> {
                if (player == null || isFinishing()) return;
                player.stop();
                playUri(api.apiUrl("/media/" + sourceId + "/stream"), subtitles, autoPlay);
            });
        });
    }

    private void playUri(String uri, List<MediaItem.SubtitleConfiguration> subtitles, boolean autoPlay) {
        MediaItem.Builder builder = new MediaItem.Builder().setUri(Uri.parse(uri)).setMediaId(String.valueOf(streamMediaId));
        if (subtitles != null && !subtitles.isEmpty()) builder.setSubtitleConfigurations(subtitles);
        player.setMediaItem(builder.build()); player.prepare(); player.setPlayWhenReady(autoPlay);
    }

    private void handlePlaybackFailure(PlaybackException error) {
        if (!versionFallbackAttempted) {
            versionFallbackAttempted = true;
            tryCompatibleVersion(error);
            return;
        }
        if (!remuxAttempted) {
            tryRemux(error);
            return;
        }
        showUnsupported(error);
    }

    private void tryCompatibleVersion(PlaybackException original) {
        badge.setText("PROCURANDO VERSÃO COMPATÍVEL…"); badge.setTextColor(Color.WHITE);
        io.submit(() -> {
            try {
                JSONArray versions = new JSONArray(api.get("/media/" + mediaId + "/versions"));
                JSONObject best = null;
                int bestRank = -1;
                for (int i=0; i<versions.length(); i++) {
                    JSONObject v = versions.optJSONObject(i); if (v == null) continue;
                    long id = v.optLong("id"); if (id <= 0 || id == mediaId) continue;
                    String label = v.optString("label", "Original");
                    int rank = compatibilityRank(label);
                    if (rank > bestRank) { bestRank = rank; best = v; }
                }
                JSONObject selected = best;
                if (selected == null || bestRank < 0) {
                    main.post(() -> tryRemux(original));
                    return;
                }
                long id = selected.optLong("id");
                String quality = selected.optString("label", "Compatível");
                String server = selected.optString("server_label", "");
                main.post(() -> {
                    playbackMode = "direct-compatible";
                    String label = "DIRECT PLAY · " + quality;
                    if (!server.isEmpty()) label += " · " + server;
                    Toast.makeText(this, "O arquivo original não abriu. Tentando " + quality + " sem transcodificação.", Toast.LENGTH_SHORT).show();
                    loadSource(id, label, true);
                });
            } catch (Exception e) {
                main.post(() -> tryRemux(original));
            }
        });
    }

    private int compatibilityRank(String label) {
        String q = label == null ? "" : label.toLowerCase(Locale.ROOT);
        if (q.contains("1080")) return 3;
        if (q.contains("720")) return 2;
        if (q.contains("480")) return 1;
        return -1;
    }

    private void tryRemux(PlaybackException original) {
        if (remuxAttempted || isFinishing()) { showUnsupported(original); return; }
        remuxAttempted = true;
        badge.setText("VERIFICANDO REMUX…"); badge.setTextColor(Color.WHITE);
        io.submit(() -> {
            try {
                JSONObject plan = new JSONObject(api.get("/media/" + mediaId + "/compatibility"));
                if (!plan.optBoolean("available", false)) throw new Exception(plan.optString("reason", "Remux não disponível"));
                main.post(() -> {
                    playbackMode = "remux";
                    streamMediaId = mediaId;
                    badge.setText("REMUX · SEM TRANSCODIFICAÇÃO"); badge.setTextColor(Color.rgb(255,196,94));
                    player.stop();
                    playUri(api.apiUrl("/media/" + mediaId + "/remux"), null, true);
                    Toast.makeText(this, "Tentando Remux. O vídeo continua com a resolução e codec originais.", Toast.LENGTH_SHORT).show();
                });
            } catch (Exception e) {
                main.post(() -> showUnsupported(original));
            }
        });
    }

    private void showUnsupported(PlaybackException error) {
        if (isFinishing()) return;
        badge.setText("APARELHO NÃO SUPORTA ESTE ARQUIVO"); badge.setTextColor(Color.rgb(255,100,110));
        String detail = error == null ? "" : error.getErrorCodeName();
        String msg = "Direct Play e Remux falharam. Remux não reduz 4K nem troca o codec. ";
        if (!detail.isEmpty()) msg += "Erro: " + detail + ". ";
        msg += "Se não existir uma versão 1080p/720p deste título, será necessária uma versão de compatibilidade para este aparelho.";
        Toast.makeText(this, msg, Toast.LENGTH_LONG).show();
    }

    private void sendHeartbeat() {
        if (player == null) return;
        long position = Math.max(0, player.getCurrentPosition());
        long duration = Math.max(0, player.getDuration());
        String state = player.isPlaying() ? "playing" : "paused";
        io.submit(() -> {
            try {
                JSONObject body = new JSONObject()
                    .put("position_seconds", position / 1000.0)
                    .put("duration_seconds", duration / 1000.0)
                    .put("state", state)
                    .put("mode", playbackMode);
                api.post("/media/" + mediaId + "/playback", body);
            } catch (Exception ignored) {}
        });
    }

    private void immersive() {
        getWindow().getDecorView().setSystemUiVisibility(
            View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
    }

    @Override public void onBackPressed() { finish(); }

    @Override protected void onStop() {
        sendHeartbeat();
        super.onStop();
    }

    @Override protected void onDestroy() {
        main.removeCallbacks(heartbeat);
        if (player != null) { player.release(); player = null; }
        io.shutdownNow();
        super.onDestroy();
    }
}
