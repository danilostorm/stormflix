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

import androidx.media3.common.C;
import androidx.media3.common.Format;
import androidx.media3.common.MediaItem;
import androidx.media3.common.MimeTypes;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.common.TrackSelectionOverride;
import androidx.media3.common.Tracks;
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
    private boolean audioPreferenceApplied;
    private boolean audioRecoveryAttempted;
    private boolean audioFailure;
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
        headers.put("User-Agent", "StormFlix-Android/0.1.3");
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("StormFlix-Android/0.1.3")
            .setAllowCrossProtocolRedirects(true)
            .setDefaultRequestProperties(headers);
        DefaultDataSource.Factory data = new DefaultDataSource.Factory(this, http);
        player = new ExoPlayer.Builder(this)
            .setMediaSourceFactory(new DefaultMediaSourceFactory(data))
            .build();

        player.setTrackSelectionParameters(
            player.getTrackSelectionParameters()
                .buildUpon()
                .setPreferredAudioLanguages(languagePriority(api.store().preferredAudio()))
                .setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle()))
                .build());

        FrameLayout root = new FrameLayout(this); root.setBackgroundColor(Color.BLACK);
        playerView = new PlayerView(this); playerView.setPlayer(player); playerView.setUseController(true); playerView.setKeepScreenOn(true);
        root.addView(playerView, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        badge = new TextView(this); badge.setText("DIRECT PLAY"); badge.setTextColor(Color.rgb(117,255,170)); badge.setTextSize(11); badge.setPadding(Ui.dp(this,10),Ui.dp(this,6),Ui.dp(this,10),Ui.dp(this,6)); badge.setBackground(Ui.round(Color.argb(210,15,35,25),8));
        FrameLayout.LayoutParams bp = new FrameLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, Gravity.TOP|Gravity.END); bp.setMargins(0,Ui.dp(this,18),Ui.dp(this,18),0); root.addView(badge,bp);
        setContentView(root);

        player.addListener(new Player.Listener() {
            @Override public void onPlayerError(PlaybackException error) { handlePlaybackFailure(error); }
            @Override public void onPlaybackStateChanged(int state) { if (state == Player.STATE_ENDED) sendHeartbeat(); }
            @Override public void onTracksChanged(Tracks tracks) { inspectAndSelectAudio(tracks); }
        });
        main.postDelayed(heartbeat, 10000);
    }

    private void loadSource(long sourceId, String label, boolean autoPlay) {
        streamMediaId = sourceId;
        audioPreferenceApplied = false;
        audioRecoveryAttempted = false;
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

    private void inspectAndSelectAudio(Tracks tracks) {
        if (player == null || isFinishing()) return;
        String wanted = api.store().preferredAudio();
        Tracks.Group bestGroup = null;
        int bestIndex = -1;
        int bestScore = -1;
        boolean hasAudio = false;
        boolean hasSupportedAudio = false;
        boolean selectedSupportedAudio = false;

        for (Tracks.Group group : tracks.getGroups()) {
            if (group.getType() != C.TRACK_TYPE_AUDIO) continue;
            hasAudio = true;
            for (int i = 0; i < group.length; i++) {
                boolean supported = group.isTrackSupported(i);
                if (supported) hasSupportedAudio = true;
                if (supported && group.isTrackSelected(i)) selectedSupportedAudio = true;
                if (!supported) continue;
                int score = audioScore(group.getTrackFormat(i), wanted);
                if (score > bestScore) {
                    bestScore = score;
                    bestGroup = group;
                    bestIndex = i;
                }
            }
        }

        if (bestGroup != null && bestIndex >= 0 && !audioPreferenceApplied) {
            audioPreferenceApplied = true;
            if (!bestGroup.isTrackSelected(bestIndex)) {
                player.setTrackSelectionParameters(
                    player.getTrackSelectionParameters()
                        .buildUpon()
                        .setOverrideForType(new TrackSelectionOverride(bestGroup.getMediaTrackGroup(), bestIndex))
                        .build());
                Format selected = bestGroup.getTrackFormat(bestIndex);
                String label = audioLabel(selected);
                if (!label.isEmpty()) Toast.makeText(this, "Áudio selecionado: " + label, Toast.LENGTH_SHORT).show();
            }
        }

        if (hasAudio && !hasSupportedAudio && !audioRecoveryAttempted) {
            audioRecoveryAttempted = true;
            audioFailure = true;
            badge.setText("ÁUDIO INCOMPATÍVEL · PROCURANDO ALTERNATIVA…");
            badge.setTextColor(Color.rgb(255,196,94));
            main.postDelayed(this::recoverUnsupportedAudio, 350);
        } else if (hasSupportedAudio && !selectedSupportedAudio && bestGroup == null && !audioRecoveryAttempted) {
            audioRecoveryAttempted = true;
            audioFailure = true;
            main.postDelayed(this::recoverUnsupportedAudio, 350);
        }
    }

    private void recoverUnsupportedAudio() {
        if (player == null || isFinishing()) return;
        if (!versionFallbackAttempted) {
            versionFallbackAttempted = true;
            tryCompatibleVersion(null);
            return;
        }
        if (!remuxAttempted) {
            tryRemux(null);
            return;
        }
        showUnsupported(null);
    }

    private int audioScore(Format format, String preferred) {
        int score = 1;
        String want = normalizeLang(preferred);
        String primary = primaryLang(want);
        String language = normalizeLang(format.language);
        String label = format.label == null ? "" : format.label.toLowerCase(Locale.ROOT);

        if (!want.isEmpty() && language.equals(want)) score += 120;
        else if (!primary.isEmpty() && primaryLang(language).equals(primary)) score += 100;

        if (primary.equals("pt")) {
            if (language.equals("por") || language.equals("pt-br") || language.equals("pob")) score += 110;
            if (label.contains("portugu") || label.contains("pt-br") || label.contains("dublado") || label.contains("brasil")) score += 95;
        } else if (primary.equals("en")) {
            if (language.equals("eng")) score += 95;
            if (label.contains("english") || label.contains("ingl")) score += 80;
        } else if (primary.equals("es")) {
            if (language.equals("spa")) score += 95;
            if (label.contains("espa") || label.contains("spanish")) score += 80;
        }
        return score;
    }

    private String audioLabel(Format format) {
        if (format.label != null && !format.label.trim().isEmpty()) return format.label.trim();
        if (format.language != null && !format.language.trim().isEmpty()) return format.language.trim();
        if (format.sampleMimeType != null && !format.sampleMimeType.trim().isEmpty()) return format.sampleMimeType.replace("audio/", "").toUpperCase(Locale.ROOT);
        return "";
    }

    private String[] languagePriority(String preferred) {
        String normalized = normalizeLang(preferred);
        String primary = primaryLang(normalized);
        List<String> out = new ArrayList<>();
        if (!normalized.isEmpty()) out.add(preferred.trim());
        if (!primary.isEmpty() && !containsIgnoreCase(out, primary)) out.add(primary);
        if (primary.equals("pt")) {
            if (!containsIgnoreCase(out, "por")) out.add("por");
        } else if (primary.equals("en")) {
            if (!containsIgnoreCase(out, "eng")) out.add("eng");
        } else if (primary.equals("es")) {
            if (!containsIgnoreCase(out, "spa")) out.add("spa");
        }
        return out.toArray(new String[0]);
    }

    private boolean containsIgnoreCase(List<String> values, String target) {
        for (String value : values) if (value.equalsIgnoreCase(target)) return true;
        return false;
    }

    private String normalizeLang(String value) {
        return value == null ? "" : value.trim().toLowerCase(Locale.ROOT).replace('_', '-');
    }

    private String primaryLang(String value) {
        String normalized = normalizeLang(value);
        int dash = normalized.indexOf('-');
        return dash > 0 ? normalized.substring(0, dash) : normalized;
    }

    private void handlePlaybackFailure(PlaybackException error) {
        audioFailure = false;
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
        badge.setText(audioFailure ? "PROCURANDO OUTRA VERSÃO COM ÁUDIO…" : "PROCURANDO VERSÃO COMPATÍVEL…"); badge.setTextColor(Color.WHITE);
        io.submit(() -> {
            try {
                JSONArray versions = new JSONArray(api.get("/media/" + mediaId + "/versions"));
                JSONObject best = null;
                int bestRank = -1;
                for (int i=0; i<versions.length(); i++) {
                    JSONObject v = versions.optJSONObject(i); if (v == null) continue;
                    long id = v.optLong("id"); if (id <= 0 || id == streamMediaId) continue;
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
                    Toast.makeText(this, audioFailure ? "Tentando outra cópia com áudio compatível." : "O arquivo original não abriu. Tentando " + quality + " sem transcodificação.", Toast.LENGTH_SHORT).show();
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
        badge.setText(audioFailure ? "VERIFICANDO REMUX DE ÁUDIO…" : "VERIFICANDO REMUX…"); badge.setTextColor(Color.WHITE);
        final long targetMediaId = streamMediaId;
        io.submit(() -> {
            try {
                JSONObject plan = new JSONObject(api.get("/media/" + targetMediaId + "/compatibility"));
                if (!plan.optBoolean("available", false)) throw new Exception(plan.optString("reason", "Remux não disponível"));
                main.post(() -> {
                    playbackMode = audioFailure ? "remux-audio-compatible" : "remux";
                    streamMediaId = targetMediaId;
                    audioPreferenceApplied = false;
                    audioRecoveryAttempted = false;
                    badge.setText(audioFailure ? "REMUX · ÁUDIO COMPATÍVEL" : "REMUX · SEM TRANSCODIFICAÇÃO");
                    badge.setTextColor(Color.rgb(255,196,94));
                    player.stop();
                    playUri(api.apiUrl("/media/" + targetMediaId + "/remux"), null, true);
                    Toast.makeText(this, audioFailure ? "Tentando uma faixa de áudio compatível no Remux." : "Tentando Remux. O vídeo continua com a resolução e codec originais.", Toast.LENGTH_SHORT).show();
                });
            } catch (Exception e) {
                main.post(() -> showUnsupported(original));
            }
        });
    }

    private void showUnsupported(PlaybackException error) {
        if (isFinishing()) return;
        if (audioFailure) {
            badge.setText("ÁUDIO NÃO SUPORTADO"); badge.setTextColor(Color.rgb(255,100,110));
            Toast.makeText(this, "O vídeo abriu, mas este aparelho não possui decoder para nenhuma faixa de áudio compatível deste arquivo. Se houver outra cópia com AAC/AC3/EAC3 o StormFlix tentará automaticamente; DTS/TrueHD podem exigir uma versão de compatibilidade de áudio.", Toast.LENGTH_LONG).show();
            return;
        }
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
