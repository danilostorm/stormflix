package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.FrameLayout;
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
import androidx.media3.ui.AspectRatioFrameLayout;
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
    private Button settingsButton;
    private long mediaId;
    private long streamMediaId;
    private String title;
    private boolean versionFallbackAttempted;
    private boolean remuxAttempted;
    private boolean audioPreferenceApplied;
    private boolean audioRecoveryAttempted;
    private boolean audioFailure;
    private int audioCheckGeneration;
    private String playbackMode = "Direct Play";
    private List<MediaItem.SubtitleConfiguration> currentSubtitles = new ArrayList<>();

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
        loadSource(mediaId, true);
    }

    private void buildPlayer() {
        Map<String,String> headers = new HashMap<>();
        String cookie = api.store().cookieHeader();
        if (!cookie.isEmpty()) headers.put("Cookie", cookie);
        headers.put("User-Agent", "StormFlix-Android/0.1.4");
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("StormFlix-Android/0.1.4")
            .setAllowCrossProtocolRedirects(true)
            .setDefaultRequestProperties(headers);
        DefaultDataSource.Factory data = new DefaultDataSource.Factory(this, http);
        player = new ExoPlayer.Builder(this)
            .setMediaSourceFactory(new DefaultMediaSourceFactory(data))
            .build();

        applyProfileLanguagePreferences();

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Color.BLACK);
        playerView = new PlayerView(this);
        playerView.setPlayer(player);
        playerView.setUseController(true);
        playerView.setKeepScreenOn(true);
        playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FIT);
        root.addView(playerView, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        settingsButton = new Button(this);
        settingsButton.setText("⚙");
        settingsButton.setTextColor(Color.WHITE);
        settingsButton.setTextSize(18);
        settingsButton.setAllCaps(false);
        settingsButton.setFocusable(true);
        settingsButton.setBackground(Ui.round(Color.argb(150, 10, 12, 16), 12));
        settingsButton.setOnClickListener(v -> showPlayerMenu());
        FrameLayout.LayoutParams gear = new FrameLayout.LayoutParams(Ui.dp(this, 52), Ui.dp(this, 46), Gravity.TOP | Gravity.END);
        gear.setMargins(0, Ui.dp(this, 16), Ui.dp(this, 16), 0);
        root.addView(settingsButton, gear);
        setContentView(root);

        player.addListener(new Player.Listener() {
            @Override public void onPlayerError(PlaybackException error) { handlePlaybackFailure(error); }
            @Override public void onPlaybackStateChanged(int state) {
                if (state == Player.STATE_ENDED) sendHeartbeat();
                if (state == Player.STATE_READY) scheduleAudioInspection();
            }
            @Override public void onTracksChanged(Tracks tracks) { scheduleAudioInspection(); }
        });
        main.postDelayed(heartbeat, 10000);
    }

    private void applyProfileLanguagePreferences() {
        player.setTrackSelectionParameters(
            player.getTrackSelectionParameters()
                .buildUpon()
                .setPreferredAudioLanguages(languagePriority(api.store().preferredAudio()))
                .setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle()))
                .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
                .build());
    }

    private void loadSource(long sourceId, boolean autoPlay) {
        streamMediaId = sourceId;
        audioPreferenceApplied = false;
        audioRecoveryAttempted = false;
        audioFailure = false;
        audioCheckGeneration++;
        io.submit(() -> {
            List<MediaItem.SubtitleConfiguration> subtitles = new ArrayList<>();
            try {
                JSONArray arr = new JSONArray(api.get("/media/" + sourceId + "/subtitles"));
                for (int i=0;i<arr.length();i++) {
                    JSONObject s = arr.optJSONObject(i); if (s==null) continue;
                    long id=s.optLong("id"); String lang=s.optString("language","pt");
                    Uri uri=Uri.parse(api.apiUrl("/media/"+sourceId+"/subtitles/"+id+"/vtt"));
                    subtitles.add(new MediaItem.SubtitleConfiguration.Builder(uri)
                        .setMimeType(MimeTypes.TEXT_VTT).setLanguage(lang).setLabel(lang).build());
                }
            } catch (Exception ignored) {}
            currentSubtitles = subtitles;
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
        player.setMediaItem(builder.build());
        player.prepare();
        player.setPlayWhenReady(autoPlay);
    }

    private void scheduleAudioInspection() {
        final int generation = ++audioCheckGeneration;
        main.postDelayed(() -> {
            if (generation != audioCheckGeneration || player == null || isFinishing()) return;
            if (player.getPlaybackState() == Player.STATE_IDLE) return;
            inspectAndSelectAudio(player.getCurrentTracks());
        }, 1800);
    }

    private void inspectAndSelectAudio(Tracks tracks) {
        if (player == null || isFinishing()) return;
        String wanted = api.store().preferredAudio();
        Tracks.Group bestGroup = null;
        int bestIndex = -1;
        int bestScore = -1;
        boolean hasAudio = false;
        boolean hasSupportedAudio = false;

        for (Tracks.Group group : tracks.getGroups()) {
            if (group.getType() != C.TRACK_TYPE_AUDIO) continue;
            hasAudio = true;
            for (int i = 0; i < group.length; i++) {
                if (!group.isTrackSupported(i)) continue;
                hasSupportedAudio = true;
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
                    player.getTrackSelectionParameters().buildUpon()
                        .setOverrideForType(new TrackSelectionOverride(bestGroup.getMediaTrackGroup(), bestIndex))
                        .build());
                String label = audioLabel(bestGroup.getTrackFormat(bestIndex));
                if (!label.isEmpty()) Toast.makeText(this, "Áudio: " + label, Toast.LENGTH_SHORT).show();
            }
            return;
        }

        if (hasAudio && !hasSupportedAudio && !audioRecoveryAttempted) {
            audioRecoveryAttempted = true;
            audioFailure = true;
            recoverUnsupportedAudio();
        }
    }

    private void recoverUnsupportedAudio() {
        if (player == null || isFinishing()) return;
        Toast.makeText(this, "Faixa de áudio incompatível. Procurando alternativa…", Toast.LENGTH_SHORT).show();
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
        return "Áudio";
    }

    private String textLabel(Format format) {
        if (format.label != null && !format.label.trim().isEmpty()) return format.label.trim();
        if (format.language != null && !format.language.trim().isEmpty()) return format.language.trim();
        return "Legenda";
    }

    private String[] languagePriority(String preferred) {
        String normalized = normalizeLang(preferred);
        String primary = primaryLang(normalized);
        List<String> out = new ArrayList<>();
        if (!normalized.isEmpty()) out.add(preferred.trim());
        if (!primary.isEmpty() && !containsIgnoreCase(out, primary)) out.add(primary);
        if (primary.equals("pt") && !containsIgnoreCase(out, "por")) out.add("por");
        else if (primary.equals("en") && !containsIgnoreCase(out, "eng")) out.add("eng");
        else if (primary.equals("es") && !containsIgnoreCase(out, "spa")) out.add("spa");
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
        io.submit(() -> {
            try {
                JSONArray versions = new JSONArray(api.get("/media/" + mediaId + "/versions"));
                JSONObject best = null;
                int bestRank = -1;
                for (int i=0; i<versions.length(); i++) {
                    JSONObject v = versions.optJSONObject(i); if (v == null) continue;
                    long id = v.optLong("id"); if (id <= 0 || id == streamMediaId) continue;
                    int rank = compatibilityRank(v.optString("label", "Original"));
                    if (rank > bestRank) { bestRank = rank; best = v; }
                }
                JSONObject selected = best;
                if (selected == null || bestRank < 0) {
                    main.post(() -> tryRemux(original));
                    return;
                }
                long id = selected.optLong("id");
                String quality = selected.optString("label", "Compatível");
                main.post(() -> {
                    playbackMode = "Direct Play · " + quality;
                    Toast.makeText(this, audioFailure ? "Tentando outra cópia com áudio compatível." : "Tentando " + quality + " sem transcodificação.", Toast.LENGTH_SHORT).show();
                    loadSource(id, true);
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
        final long targetMediaId = streamMediaId;
        io.submit(() -> {
            try {
                JSONObject plan = new JSONObject(api.get("/media/" + targetMediaId + "/compatibility"));
                if (!plan.optBoolean("available", false)) throw new Exception(plan.optString("reason", "Remux não disponível"));
                main.post(() -> {
                    playbackMode = audioFailure ? "Remux · áudio compatível" : "Remux · sem transcodificação";
                    streamMediaId = targetMediaId;
                    audioPreferenceApplied = false;
                    audioRecoveryAttempted = false;
                    audioCheckGeneration++;
                    player.stop();
                    playUri(api.apiUrl("/media/" + targetMediaId + "/remux"), currentSubtitles, true);
                    Toast.makeText(this, audioFailure ? "Tentando Remux com uma faixa de áudio compatível." : "Tentando Remux sem reencode.", Toast.LENGTH_SHORT).show();
                });
            } catch (Exception e) {
                main.post(() -> showUnsupported(original));
            }
        });
    }

    private void showUnsupported(PlaybackException error) {
        if (isFinishing()) return;
        if (audioFailure) {
            Toast.makeText(this, "Áudio não suportado neste aparelho. Abra ⚙ → Áudio para conferir as faixas disponíveis.", Toast.LENGTH_LONG).show();
            return;
        }
        String detail = error == null ? "" : error.getErrorCodeName();
        String msg = "Este aparelho não conseguiu reproduzir o arquivo por Direct Play, versão alternativa ou Remux.";
        if (!detail.isEmpty()) msg += " Erro: " + detail + ".";
        Toast.makeText(this, msg, Toast.LENGTH_LONG).show();
    }

    private void showPlayerMenu() {
        String label = title == null || title.trim().isEmpty() ? "Player StormFlix" : title;
        String[] items = {"Áudio", "Legendas", "Tela / Zoom", "Informações"};
        new AlertDialog.Builder(this)
            .setTitle(label)
            .setItems(items, (d, which) -> {
                if (which == 0) showAudioMenu();
                else if (which == 1) showSubtitleMenu();
                else if (which == 2) showScreenMenu();
                else showPlaybackInfo();
            })
            .setNegativeButton("Fechar", null)
            .show();
    }

    private void showAudioMenu() {
        Tracks tracks = player.getCurrentTracks();
        List<String> labels = new ArrayList<>();
        List<Tracks.Group> groups = new ArrayList<>();
        List<Integer> indexes = new ArrayList<>();
        labels.add("Automático · preferência " + api.store().preferredAudio());
        groups.add(null); indexes.add(-1);
        for (Tracks.Group group : tracks.getGroups()) {
            if (group.getType() != C.TRACK_TYPE_AUDIO) continue;
            for (int i=0;i<group.length;i++) {
                Format f = group.getTrackFormat(i);
                String label = audioLabel(f);
                if (!group.isTrackSupported(i)) label += " · não suportado";
                else if (group.isTrackSelected(i)) label += " · atual";
                labels.add(label);
                groups.add(group); indexes.add(i);
            }
        }
        new AlertDialog.Builder(this).setTitle("Áudio")
            .setItems(labels.toArray(new String[0]), (d, which) -> {
                if (which == 0) {
                    player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                        .clearOverrides()
                        .setPreferredAudioLanguages(languagePriority(api.store().preferredAudio()))
                        .setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle()))
                        .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
                        .build());
                    audioPreferenceApplied = false;
                    scheduleAudioInspection();
                    return;
                }
                Tracks.Group group = groups.get(which); int index = indexes.get(which);
                if (group == null || !group.isTrackSupported(index)) {
                    Toast.makeText(this, "Este aparelho não possui decoder para esta faixa.", Toast.LENGTH_LONG).show(); return;
                }
                player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                    .setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(), index)).build());
                Toast.makeText(this, "Áudio: " + audioLabel(group.getTrackFormat(index)), Toast.LENGTH_SHORT).show();
            }).setNegativeButton("Voltar", null).show();
    }

    private void showSubtitleMenu() {
        Tracks tracks = player.getCurrentTracks();
        List<String> labels = new ArrayList<>();
        List<Tracks.Group> groups = new ArrayList<>();
        List<Integer> indexes = new ArrayList<>();
        labels.add("Desativadas"); groups.add(null); indexes.add(-1);
        labels.add("Automático · preferência " + api.store().preferredSubtitle()); groups.add(null); indexes.add(-2);
        for (Tracks.Group group : tracks.getGroups()) {
            if (group.getType() != C.TRACK_TYPE_TEXT) continue;
            for (int i=0;i<group.length;i++) {
                String label = textLabel(group.getTrackFormat(i));
                if (!group.isTrackSupported(i)) label += " · não suportada";
                else if (group.isTrackSelected(i)) label += " · atual";
                labels.add(label); groups.add(group); indexes.add(i);
            }
        }
        new AlertDialog.Builder(this).setTitle("Legendas")
            .setItems(labels.toArray(new String[0]), (d, which) -> {
                if (which == 0) {
                    player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true).build());
                    return;
                }
                if (which == 1) {
                    player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                        .clearOverrides()
                        .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
                        .setPreferredAudioLanguages(languagePriority(api.store().preferredAudio()))
                        .setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle()))
                        .build());
                    return;
                }
                Tracks.Group group = groups.get(which); int index = indexes.get(which);
                if (group == null || !group.isTrackSupported(index)) {
                    Toast.makeText(this, "Legenda não suportada.", Toast.LENGTH_SHORT).show(); return;
                }
                player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                    .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
                    .setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(), index)).build());
            }).setNegativeButton("Voltar", null).show();
    }

    private void showScreenMenu() {
        String[] modes = {"Ajustar / sem zoom", "16:9 · sem corte", "Preencher / Zoom", "Esticar para tela"};
        new AlertDialog.Builder(this).setTitle("Tela")
            .setItems(modes, (d, which) -> {
                if (which == 0 || which == 1) playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FIT);
                else if (which == 2) playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_ZOOM);
                else playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FILL);
                Toast.makeText(this, modes[which], Toast.LENGTH_SHORT).show();
            }).setNegativeButton("Voltar", null).show();
    }

    private void showPlaybackInfo() {
        StringBuilder info = new StringBuilder();
        info.append("Modo: ").append(playbackMode);
        info.append("\nPreferência de áudio: ").append(api.store().preferredAudio());
        info.append("\nPreferência de legenda: ").append(api.store().preferredSubtitle());
        info.append("\nFonte: ").append(streamMediaId == mediaId ? "principal" : "alternativa");
        new AlertDialog.Builder(this).setTitle("Informações de reprodução").setMessage(info.toString()).setPositiveButton("OK", null).show();
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
                    .put("mode", playbackMode.toLowerCase(Locale.ROOT));
                api.post("/media/" + mediaId + "/playback", body);
            } catch (Exception ignored) {}
        });
    }

    private void immersive() {
        getWindow().getDecorView().setSystemUiVisibility(
            View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY |
            View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
    }

    @Override public void onBackPressed() { finish(); }

    @Override protected void onStop() {
        sendHeartbeat();
        super.onStop();
    }

    @Override protected void onDestroy() {
        audioCheckGeneration++;
        main.removeCallbacks(heartbeat);
        if (player != null) { player.release(); player = null; }
        io.shutdownNow();
        super.onDestroy();
    }
}
