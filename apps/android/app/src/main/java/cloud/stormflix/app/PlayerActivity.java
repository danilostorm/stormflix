package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.MotionEvent;
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
    private static final long SETTINGS_VISIBLE_MS = 15000L;
    private static final long CONTROLS_VISIBLE_MS = 6000L;
    private static final long SEEK_STEP_MS = 10000L;

    private final ExecutorService io = Executors.newFixedThreadPool(2);
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ExoPlayer player;
    private PlayerView playerView;
    private Button settingsButton;
    private long mediaId;
    private long streamMediaId;
    private String title;
    private long resumePositionMs;
    private boolean resumeApplied;
    private boolean versionFallbackAttempted;
    private boolean remuxAttempted;
    private boolean audioPreferenceApplied;
    private boolean audioRecoveryAttempted;
    private boolean audioFailure;
    private boolean audioMutedForCompatibility;
    private boolean controlsVisible;
    private boolean finishingSent;
    private int audioCheckGeneration;
    private String playbackMode = "Direct Play";
    private List<MediaItem.SubtitleConfiguration> currentSubtitles = new ArrayList<>();
    private long previousEpisodeId;
    private long nextEpisodeId;
    private String previousEpisodeTitle = "Episódio anterior";
    private String nextEpisodeTitle = "Próximo episódio";

    private final Runnable heartbeat = new Runnable() {
        @Override public void run() {
            sendHeartbeat();
            main.postDelayed(this, 10000);
        }
    };

    private final Runnable hideSettings = new Runnable() {
        @Override public void run() {
            if (settingsButton != null) settingsButton.setVisibility(View.GONE);
        }
    };

    private final Runnable hideControls = new Runnable() {
        @Override public void run() {
            controlsVisible = false;
            if (playerView != null) playerView.hideController();
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
        loadPlaybackContext(true);
    }

    private void buildPlayer() {
        Map<String,String> headers = new HashMap<>();
        String cookie = api.store().cookieHeader();
        if (!cookie.isEmpty()) headers.put("Cookie", cookie);
        headers.put("User-Agent", "StormFlix-Android/0.2.0");
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("StormFlix-Android/0.2.0")
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
        playerView.setControllerShowTimeoutMs((int) CONTROLS_VISIBLE_MS);
        playerView.setControllerAutoShow(true);
        playerView.setFocusable(true);
        playerView.setOnTouchListener((v, event) -> {
            if (event.getAction() == MotionEvent.ACTION_UP) showControlsTemporarily();
            return false;
        });
        root.addView(playerView, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        settingsButton = new Button(this);
        settingsButton.setText("⚙");
        settingsButton.setTextColor(Color.WHITE);
        settingsButton.setTextSize(18);
        settingsButton.setAllCaps(false);
        settingsButton.setFocusable(true);
        settingsButton.setFocusableInTouchMode(false);
        settingsButton.setBackground(Ui.round(Color.argb(170, 10, 12, 16), 12));
        settingsButton.setOnClickListener(v -> {
            showSettingsTemporarily();
            showPlayerMenu();
        });
        FrameLayout.LayoutParams gear = new FrameLayout.LayoutParams(Ui.dp(this, 52), Ui.dp(this, 46), Gravity.TOP | Gravity.END);
        gear.setMargins(0, Ui.dp(this, 16), Ui.dp(this, 16), 0);
        root.addView(settingsButton, gear);
        setContentView(root);
        showSettingsTemporarily();

        player.addListener(new Player.Listener() {
            @Override public void onPlayerError(PlaybackException error) { handlePlaybackFailure(error); }
            @Override public void onPlaybackStateChanged(int state) {
                if (state == Player.STATE_READY) {
                    if (!resumeApplied && resumePositionMs > 0) {
                        long duration = player.getDuration();
                        if (duration <= 0 || resumePositionMs < duration) player.seekTo(resumePositionMs);
                        resumeApplied = true;
                    }
                    scheduleAudioInspection();
                    sendHeartbeat();
                } else if (state == Player.STATE_ENDED) {
                    sendHeartbeat();
                    if (nextEpisodeId > 0) main.postDelayed(PlayerActivity.this::offerNextEpisode, 350);
                }
            }
            @Override public void onTracksChanged(Tracks tracks) { scheduleAudioInspection(); sendHeartbeat(); }
            @Override public void onIsPlayingChanged(boolean isPlaying) { if (!isPlaying) sendHeartbeat(); }
            @Override public void onPositionDiscontinuity(Player.PositionInfo oldPosition, Player.PositionInfo newPosition, int reason) {
                if (reason == Player.DISCONTINUITY_REASON_SEEK || reason == Player.DISCONTINUITY_REASON_SEEK_ADJUSTMENT) sendHeartbeat();
            }
        });
        main.postDelayed(heartbeat, 10000);
    }

    private void loadPlaybackContext(boolean offerResume) {
        final long target = mediaId;
        io.submit(() -> {
            JSONObject detail = null, neighbors = null;
            try { detail = new JSONObject(api.get("/media/" + target)); } catch (Exception ignored) {}
            try { neighbors = new JSONObject(api.get("/media/" + target + "/neighbors")); } catch (Exception ignored) {}
            JSONObject mediaDetail = detail;
            JSONObject adjacent = neighbors;
            main.post(() -> {
                if (isFinishing() || mediaId != target) return;
                if (mediaDetail != null) {
                    String loadedTitle = mediaDetail.optString("title", "");
                    if (!loadedTitle.isEmpty()) title = loadedTitle;
                    double saved = mediaDetail.optDouble("position_seconds", 0);
                    double duration = mediaDetail.optDouble("duration_seconds", 0);
                    boolean completed = mediaDetail.optBoolean("completed", false);
                    if (!completed && saved >= 30 && (duration <= 0 || saved / duration < 0.92)) resumePositionMs = (long)(saved * 1000.0);
                }
                readNeighbors(adjacent);
                if (offerResume && resumePositionMs >= 30000) showResumeDialog(); else loadSource(mediaId, true);
            });
        });
    }

    private void readNeighbors(JSONObject neighbors) {
        previousEpisodeId = 0; nextEpisodeId = 0;
        if (neighbors == null) return;
        JSONObject previous = neighbors.optJSONObject("previous");
        JSONObject next = neighbors.optJSONObject("next");
        if (previous != null) { previousEpisodeId = previous.optLong("id"); previousEpisodeTitle = previous.optString("title", "Episódio anterior"); }
        if (next != null) { nextEpisodeId = next.optLong("id"); nextEpisodeTitle = next.optString("title", "Próximo episódio"); }
    }

    private void showResumeDialog() {
        String at = clock(resumePositionMs / 1000);
        new AlertDialog.Builder(this)
            .setTitle(title == null ? "Continuar assistindo" : title)
            .setMessage("Continuar de " + at + "?")
            .setPositiveButton("Continuar", (d,w) -> loadSource(mediaId, true))
            .setNegativeButton("Reiniciar", (d,w) -> { resumePositionMs = 0; resumeApplied = true; loadSource(mediaId, true); })
            .setOnCancelListener(d -> loadSource(mediaId, true))
            .show();
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
        enableAudioTracks();
        MediaItem.Builder builder = new MediaItem.Builder().setUri(Uri.parse(uri)).setMediaId(String.valueOf(streamMediaId));
        if (subtitles != null && !subtitles.isEmpty()) builder.setSubtitleConfigurations(subtitles);
        player.setMediaItem(builder.build());
        player.prepare();
        player.setPlayWhenReady(autoPlay);
    }

    private void showSettingsTemporarily() {
        if (settingsButton == null) return;
        settingsButton.setVisibility(View.VISIBLE);
        main.removeCallbacks(hideSettings);
        main.postDelayed(hideSettings, SETTINGS_VISIBLE_MS);
    }

    private void showControlsTemporarily() {
        if (playerView == null) return;
        controlsVisible = true;
        playerView.showController();
        showSettingsTemporarily();
        main.removeCallbacks(hideControls);
        main.postDelayed(hideControls, CONTROLS_VISIBLE_MS);
    }

    private void hidePlayerControls() {
        controlsVisible = false;
        main.removeCallbacks(hideControls);
        if (playerView != null) playerView.hideController();
        if (settingsButton != null) settingsButton.setVisibility(View.GONE);
    }

    private void applyProfileLanguagePreferences() {
        player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
            .setPreferredAudioLanguages(languagePriority(api.store().preferredAudio()))
            .setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle()))
            .setTrackTypeDisabled(C.TRACK_TYPE_AUDIO, false)
            .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
            .build());
    }

    private void enableAudioTracks() {
        if (player == null) return;
        player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_AUDIO, false).build());
        audioMutedForCompatibility = false;
    }

    private void muteUnsupportedAudio() {
        if (player == null || audioMutedForCompatibility) return;
        player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_AUDIO, true).build());
        audioMutedForCompatibility = true;
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
        Tracks.Group bestGroup = null; int bestIndex = -1; int bestScore = -1;
        boolean hasAudio = false, hasSupportedAudio = false;
        for (Tracks.Group group : tracks.getGroups()) {
            if (group.getType() != C.TRACK_TYPE_AUDIO) continue;
            hasAudio = true;
            for (int i=0;i<group.length;i++) {
                if (!group.isTrackSupported(i)) continue;
                hasSupportedAudio = true;
                int score = audioScore(group.getTrackFormat(i), wanted);
                if (score > bestScore) { bestScore = score; bestGroup = group; bestIndex = i; }
            }
        }
        if (bestGroup != null && bestIndex >= 0 && !audioPreferenceApplied) {
            audioPreferenceApplied = true;
            if (!bestGroup.isTrackSelected(bestIndex)) {
                player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                    .setOverrideForType(new TrackSelectionOverride(bestGroup.getMediaTrackGroup(), bestIndex)).build());
            }
            return;
        }
        if (hasAudio && !hasSupportedAudio && !audioRecoveryAttempted) {
            audioRecoveryAttempted = true; audioFailure = true; recoverUnsupportedAudio();
        }
    }

    private boolean isAudioOnlyCompatibilityProblem(Tracks tracks) {
        boolean hasAudio=false, supportedAudio=false, supportedVideo=false;
        for (Tracks.Group group:tracks.getGroups()) {
            if(group.getType()==C.TRACK_TYPE_AUDIO){hasAudio=true;for(int i=0;i<group.length;i++)if(group.isTrackSupported(i))supportedAudio=true;}
            else if(group.getType()==C.TRACK_TYPE_VIDEO){for(int i=0;i<group.length;i++)if(group.isTrackSupported(i))supportedVideo=true;}
        }
        return supportedVideo && hasAudio && !supportedAudio;
    }

    private void recoverUnsupportedAudio() {
        if (player == null || isFinishing()) return;
        muteUnsupportedAudio();
        Toast.makeText(this, "Áudio incompatível. Mantendo o vídeo e convertendo somente o áudio para AAC…", Toast.LENGTH_SHORT).show();
        tryRemux(null);
    }

    private int audioScore(Format format, String preferred) {
        int score=1;String want=normalizeLang(preferred),primary=primaryLang(want),language=normalizeLang(format.language),label=format.label==null?"":format.label.toLowerCase(Locale.ROOT);
        if(!want.isEmpty()&&language.equals(want))score+=120;else if(!primary.isEmpty()&&primaryLang(language).equals(primary))score+=100;
        if(primary.equals("pt")){if(language.equals("por")||language.equals("pt-br")||language.equals("pob"))score+=110;if(label.contains("portugu")||label.contains("pt-br")||label.contains("dublado")||label.contains("brasil"))score+=95;}
        else if(primary.equals("en")){if(language.equals("eng"))score+=95;if(label.contains("english")||label.contains("ingl"))score+=80;}
        else if(primary.equals("es")){if(language.equals("spa"))score+=95;if(label.contains("espa")||label.contains("spanish"))score+=80;}
        return score;
    }

    private String audioLabel(Format format) {
        String base;
        if(format.label!=null&&!format.label.trim().isEmpty())base=format.label.trim();
        else if(format.language!=null&&!format.language.trim().isEmpty())base=format.language.trim();
        else if(format.sampleMimeType!=null&&!format.sampleMimeType.trim().isEmpty())base=format.sampleMimeType.replace("audio/","").toUpperCase(Locale.ROOT);
        else base="Áudio";
        int channels=format.channelCount;
        if(channels>0)base+=" · "+(channels==1?"mono":channels==2?"2.0":channels+" canais");
        return base;
    }

    private String textLabel(Format format) {
        if(format.label!=null&&!format.label.trim().isEmpty())return format.label.trim();
        if(format.language!=null&&!format.language.trim().isEmpty())return format.language.trim();
        return "Legenda";
    }

    private String[] languagePriority(String preferred) {
        String normalized=normalizeLang(preferred),primary=primaryLang(normalized);List<String>out=new ArrayList<>();
        if(!normalized.isEmpty())out.add(preferred.trim());if(!primary.isEmpty()&&!containsIgnoreCase(out,primary))out.add(primary);
        if(primary.equals("pt")&&!containsIgnoreCase(out,"por"))out.add("por");else if(primary.equals("en")&&!containsIgnoreCase(out,"eng"))out.add("eng");else if(primary.equals("es")&&!containsIgnoreCase(out,"spa"))out.add("spa");
        return out.toArray(new String[0]);
    }
    private boolean containsIgnoreCase(List<String> values,String target){for(String value:values)if(value.equalsIgnoreCase(target))return true;return false;}
    private String normalizeLang(String value){return value==null?"":value.trim().toLowerCase(Locale.ROOT).replace('_','-');}
    private String primaryLang(String value){String normalized=normalizeLang(value);int dash=normalized.indexOf('-');return dash>0?normalized.substring(0,dash):normalized;}

    private void handlePlaybackFailure(PlaybackException error) {
        if(player!=null&&isAudioOnlyCompatibilityProblem(player.getCurrentTracks())&&!audioRecoveryAttempted){audioRecoveryAttempted=true;audioFailure=true;recoverUnsupportedAudio();return;}
        audioFailure=false;
        if(!versionFallbackAttempted){versionFallbackAttempted=true;tryCompatibleVersion(error);return;}
        if(!remuxAttempted){tryRemux(error);return;}
        showUnsupported(error);
    }

    private void tryCompatibleVersion(PlaybackException original) {
        io.submit(() -> {
            try {
                JSONArray versions=new JSONArray(api.get("/media/"+mediaId+"/versions"));JSONObject best=null;int bestRank=-1;
                for(int i=0;i<versions.length();i++){JSONObject v=versions.optJSONObject(i);if(v==null)continue;long id=v.optLong("id");if(id<=0||id==streamMediaId)continue;int rank=compatibilityRank(v.optString("label","Original"));if(rank>bestRank){bestRank=rank;best=v;}}
                JSONObject selected=best;if(selected==null||bestRank<0){main.post(()->tryRemux(original));return;}
                long id=selected.optLong("id");String quality=selected.optString("label","Compatível");
                main.post(()->{playbackMode="Direct Play · "+quality;loadSource(id,true);Toast.makeText(this,"Tentando "+quality+" sem transcodificação.",Toast.LENGTH_SHORT).show();});
            }catch(Exception e){main.post(()->tryRemux(original));}
        });
    }
    private int compatibilityRank(String label){String q=label==null?"":label.toLowerCase(Locale.ROOT);if(q.contains("1080"))return 3;if(q.contains("720"))return 2;if(q.contains("480"))return 1;return -1;}

    private void tryRemux(PlaybackException original) {
        if(remuxAttempted||isFinishing()){showUnsupported(original);return;}remuxAttempted=true;final long targetMediaId=streamMediaId;
        io.submit(()->{
            try{
                JSONObject plan=new JSONObject(api.get("/media/"+targetMediaId+"/compatibility?audio=aac"));if(!plan.optBoolean("available",false))throw new Exception(plan.optString("reason","Remux não disponível"));boolean audioOnly=plan.optBoolean("audio_transcode",false);
                main.post(()->{playbackMode=audioOnly?"Direct Stream · vídeo original + áudio AAC":"Remux · sem transcodificação";audioPreferenceApplied=false;audioRecoveryAttempted=false;audioFailure=false;audioCheckGeneration++;enableAudioTracks();player.stop();playUri(api.apiUrl("/media/"+targetMediaId+"/remux?audio=aac"),currentSubtitles,true);Toast.makeText(this,audioOnly?"Vídeo original mantido · áudio AAC":"Remux sem reencode",Toast.LENGTH_SHORT).show();});
            }catch(Exception e){main.post(()->showUnsupported(original));}
        });
    }

    private void showUnsupported(PlaybackException error) {
        if(isFinishing())return;if(audioFailure){muteUnsupportedAudio();playbackMode="Direct Play · vídeo sem áudio";Toast.makeText(this,"Este aparelho não decodifica o áudio desta cópia.",Toast.LENGTH_LONG).show();return;}
        String detail=error==null?"":error.getErrorCodeName();String msg="Este aparelho não conseguiu reproduzir esta cópia.";if(!detail.isEmpty())msg+=" Erro: "+detail+".";Toast.makeText(this,msg,Toast.LENGTH_LONG).show();
    }

    private void showPlayerMenu() {
        List<String> items=new ArrayList<>();items.add("Áudio");items.add("Legendas");items.add("Tela / Zoom");items.add("Reiniciar do início");
        if(previousEpisodeId>0)items.add("Episódio anterior");if(nextEpisodeId>0)items.add("Próximo episódio");items.add("Informações");
        new AlertDialog.Builder(this).setTitle(title==null?"Player StormFlix":title).setItems(items.toArray(new String[0]),(d,which)->{
            String item=items.get(which);if(item.equals("Áudio"))showAudioMenu();else if(item.equals("Legendas"))showSubtitleMenu();else if(item.equals("Tela / Zoom"))showScreenMenu();else if(item.equals("Reiniciar do início")){player.seekTo(0);sendHeartbeat();}else if(item.equals("Episódio anterior"))launchEpisode(previousEpisodeId,previousEpisodeTitle);else if(item.equals("Próximo episódio"))launchEpisode(nextEpisodeId,nextEpisodeTitle);else showPlaybackInfo();
        }).setNegativeButton("Fechar",null).show();
    }

    private void showAudioMenu() {
        Tracks tracks=player.getCurrentTracks();List<String>labels=new ArrayList<>();List<Tracks.Group>groups=new ArrayList<>();List<Integer>indexes=new ArrayList<>();
        labels.add("Automático · preferência "+api.store().preferredAudio());groups.add(null);indexes.add(-1);
        for(Tracks.Group group:tracks.getGroups()){if(group.getType()!=C.TRACK_TYPE_AUDIO)continue;for(int i=0;i<group.length;i++){String label=audioLabel(group.getTrackFormat(i));if(!group.isTrackSupported(i))label+=" · não suportado";else if(group.isTrackSelected(i))label+=" · atual";labels.add(label);groups.add(group);indexes.add(i);}}
        if(playbackMode.toLowerCase(Locale.ROOT).contains("aac"))labels.add("Compatibilidade AAC · ativa");
        new AlertDialog.Builder(this).setTitle("Áudio").setItems(labels.toArray(new String[0]),(d,which)->{
            if(which==0){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().clearOverrides().setPreferredAudioLanguages(languagePriority(api.store().preferredAudio())).setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle())).setTrackTypeDisabled(C.TRACK_TYPE_AUDIO,false).build());audioMutedForCompatibility=false;audioPreferenceApplied=false;scheduleAudioInspection();return;}
            if(which>=groups.size()){Toast.makeText(this,"Compatibilidade AAC já está ativa.",Toast.LENGTH_SHORT).show();return;}
            Tracks.Group group=groups.get(which);int index=indexes.get(which);if(group==null||!group.isTrackSupported(index)){Toast.makeText(this,"Sem decoder para esta faixa. Ativando AAC…",Toast.LENGTH_SHORT).show();audioFailure=true;recoverUnsupportedAudio();return;}enableAudioTracks();player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(),index)).build());
        }).setNegativeButton("Voltar",null).show();
    }

    private void showSubtitleMenu() {
        Tracks tracks=player.getCurrentTracks();List<String>labels=new ArrayList<>();List<Tracks.Group>groups=new ArrayList<>();List<Integer>indexes=new ArrayList<>();labels.add("Desativadas");groups.add(null);indexes.add(-1);labels.add("Automático · preferência "+api.store().preferredSubtitle());groups.add(null);indexes.add(-2);
        for(Tracks.Group group:tracks.getGroups()){if(group.getType()!=C.TRACK_TYPE_TEXT)continue;for(int i=0;i<group.length;i++){String label=textLabel(group.getTrackFormat(i));if(!group.isTrackSupported(i))label+=" · não suportada";else if(group.isTrackSelected(i))label+=" · atual";labels.add(label);groups.add(group);indexes.add(i);}}
        new AlertDialog.Builder(this).setTitle("Legendas").setItems(labels.toArray(new String[0]),(d,which)->{
            if(which==0){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_TEXT,true).build());return;}
            if(which==1){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().clearOverrides().setTrackTypeDisabled(C.TRACK_TYPE_TEXT,false).setPreferredAudioLanguages(languagePriority(api.store().preferredAudio())).setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle())).build());return;}
            Tracks.Group group=groups.get(which);int index=indexes.get(which);if(group==null||!group.isTrackSupported(index)){Toast.makeText(this,"Legenda não suportada.",Toast.LENGTH_SHORT).show();return;}player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_TEXT,false).setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(),index)).build());
        }).setNegativeButton("Voltar",null).show();
    }

    private void showScreenMenu(){String[]modes={"Ajustar / sem zoom","16:9 · sem corte","Preencher / Zoom","Esticar para tela"};new AlertDialog.Builder(this).setTitle("Tela").setItems(modes,(d,which)->{if(which==0||which==1)playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FIT);else if(which==2)playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_ZOOM);else playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FILL);Toast.makeText(this,modes[which],Toast.LENGTH_SHORT).show();}).setNegativeButton("Voltar",null).show();}

    private void showPlaybackInfo(){StringBuilder info=new StringBuilder();info.append("Modo: ").append(playbackMode);info.append("\nPosição: ").append(clock(Math.max(0,player.getCurrentPosition()/1000)));info.append("\nVídeo: ").append(selectedCodec(C.TRACK_TYPE_VIDEO));info.append("\nÁudio: ").append(selectedCodec(C.TRACK_TYPE_AUDIO));info.append("\nPreferência de áudio: ").append(api.store().preferredAudio());info.append("\nPreferência de legenda: ").append(api.store().preferredSubtitle());info.append("\nFonte: ").append(streamMediaId==mediaId?"principal":"alternativa");new AlertDialog.Builder(this).setTitle("Informações de reprodução").setMessage(info.toString()).setPositiveButton("OK",null).show();}

    private String selectedCodec(int type){if(player==null)return"—";for(Tracks.Group group:player.getCurrentTracks().getGroups()){if(group.getType()!=type)continue;for(int i=0;i<group.length;i++)if(group.isTrackSelected(i))return codecName(group.getTrackFormat(i));}return"—";}
    private String codecName(Format f){String value=f.codecs;if(value==null||value.isEmpty())value=f.sampleMimeType;if(value==null||value.isEmpty())return"—";int slash=value.indexOf('/');if(slash>=0)value=value.substring(slash+1);return value.toUpperCase(Locale.ROOT);}

    private void sendHeartbeat(){queueProgress(false);}
    private void queueProgress(boolean finishSession){
        if(player==null||mediaId<=0)return;final long target=mediaId;final long position=Math.max(0,player.getCurrentPosition());final long duration=Math.max(0,player.getDuration());final String state=player.isPlaying()?"playing":"paused";final String mode=playbackMode.toLowerCase(Locale.ROOT);final String video=selectedCodec(C.TRACK_TYPE_VIDEO);final String audio=selectedCodec(C.TRACK_TYPE_AUDIO);final int bitrate=selectedBitrateKbps();
        io.submit(()->{try{JSONObject body=new JSONObject().put("position_seconds",position/1000.0).put("duration_seconds",duration/1000.0).put("state",state).put("mode",mode).put("video_codec","—".equals(video)?"":video).put("audio_codec","—".equals(audio)?"":audio).put("resolution",playerView!=null&&player!=null&&player.getVideoSize().width>0?player.getVideoSize().width+"x"+player.getVideoSize().height:"").put("bitrate_kbps",bitrate);api.post("/media/"+target+"/playback",body);if(finishSession)api.delete("/media/"+target+"/playback");}catch(Exception ignored){}});
    }
    private int selectedBitrateKbps(){if(player==null)return 0;for(Tracks.Group group:player.getCurrentTracks().getGroups()){if(group.getType()!=C.TRACK_TYPE_VIDEO)continue;for(int i=0;i<group.length;i++)if(group.isTrackSelected(i)){Format f=group.getTrackFormat(i);int b=f.averageBitrate>0?f.averageBitrate:f.peakBitrate;return b>0?b/1000:0;}}return 0;}

    private void seekRelative(long delta){if(player==null)return;long duration=player.getDuration(),target=Math.max(0,player.getCurrentPosition()+delta);if(duration>0)target=Math.min(duration,target);player.seekTo(target);showControlsTemporarily();Toast.makeText(this,(delta>0?"+":"")+(delta/1000)+"s",Toast.LENGTH_SHORT).show();}
    private void togglePlay(){if(player==null)return;if(player.isPlaying())player.pause();else player.play();showControlsTemporarily();}

    @Override public boolean dispatchKeyEvent(KeyEvent event){
        if(event.getAction()!=KeyEvent.ACTION_DOWN)return super.dispatchKeyEvent(event);int key=event.getKeyCode();
        if(key==KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE||key==KeyEvent.KEYCODE_HEADSETHOOK){togglePlay();return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_PLAY){player.play();showControlsTemporarily();return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_PAUSE||key==KeyEvent.KEYCODE_MEDIA_STOP){player.pause();sendHeartbeat();showControlsTemporarily();return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_FAST_FORWARD){seekRelative(SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_REWIND){seekRelative(-SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_NEXT&&nextEpisodeId>0){launchEpisode(nextEpisodeId,nextEpisodeTitle);return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_PREVIOUS&&previousEpisodeId>0){launchEpisode(previousEpisodeId,previousEpisodeTitle);return true;}
        if(key==KeyEvent.KEYCODE_MENU||key==KeyEvent.KEYCODE_INFO){showSettingsTemporarily();showPlayerMenu();return true;}
        if(key==KeyEvent.KEYCODE_CAPTIONS){showSubtitleMenu();return true;}
        if(key==KeyEvent.KEYCODE_DPAD_LEFT){if(controlsVisible&&isPlayerControlFocused())return super.dispatchKeyEvent(event);seekRelative(-SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_DPAD_RIGHT){if(controlsVisible&&isPlayerControlFocused())return super.dispatchKeyEvent(event);seekRelative(SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_DPAD_CENTER||key==KeyEvent.KEYCODE_ENTER){if(!controlsVisible){showControlsTemporarily();return true;}return super.dispatchKeyEvent(event);}
        if(key==KeyEvent.KEYCODE_DPAD_UP||key==KeyEvent.KEYCODE_DPAD_DOWN){showControlsTemporarily();return super.dispatchKeyEvent(event);}
        return super.dispatchKeyEvent(event);
    }

    private boolean isPlayerControlFocused(){View focused=getCurrentFocus();if(focused==null||focused==playerView||focused==settingsButton)return false;View current=focused;while(current!=null){if(current==playerView)return true;if(!(current.getParent() instanceof View))break;current=(View)current.getParent();}return false;}

    private void offerNextEpisode(){if(nextEpisodeId<=0||isFinishing())return;new AlertDialog.Builder(this).setTitle("Próximo episódio").setMessage(nextEpisodeTitle).setPositiveButton("Reproduzir",(d,w)->launchEpisode(nextEpisodeId,nextEpisodeTitle)).setNegativeButton("Fechar",null).show();}
    private void launchEpisode(long id,String episodeTitle){if(id<=0)return;queueProgress(true);Intent intent=new Intent(this,PlayerActivity.class);intent.putExtra("media_id",id);intent.putExtra("title",episodeTitle);startActivity(intent);finish();}

    private String clock(long seconds){seconds=Math.max(0,seconds);long h=seconds/3600,m=(seconds%3600)/60,s=seconds%60;return h>0?String.format(Locale.US,"%d:%02d:%02d",h,m,s):String.format(Locale.US,"%d:%02d",m,s);}

    private void immersive(){getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN|View.SYSTEM_UI_FLAG_HIDE_NAVIGATION|View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY|View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN|View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION|View.SYSTEM_UI_FLAG_LAYOUT_STABLE);}

    @Override public void onBackPressed(){if(controlsVisible){hidePlayerControls();return;}if(!finishingSent){finishingSent=true;queueProgress(true);}finish();}
    @Override protected void onPause(){sendHeartbeat();super.onPause();}
    @Override protected void onStop(){sendHeartbeat();super.onStop();}
    @Override public void onUserLeaveHint(){sendHeartbeat();super.onUserLeaveHint();}
    @Override public void onWindowFocusChanged(boolean hasFocus){super.onWindowFocusChanged(hasFocus);if(!hasFocus)sendHeartbeat();else immersive();}
    @Override protected void onDestroy(){audioCheckGeneration++;main.removeCallbacks(heartbeat);main.removeCallbacks(hideSettings);main.removeCallbacks(hideControls);if(!finishingSent&&player!=null){finishingSent=true;queueProgress(true);}if(player!=null){player.release();player=null;}io.shutdown();super.onDestroy();}
}
