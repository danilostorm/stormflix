package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.PictureInPictureParams;
import android.content.Intent;
import android.content.res.Configuration;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.util.Log;
import android.util.Rational;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.Toast;

import androidx.media3.common.C;
import androidx.media3.common.Format;
import androidx.media3.common.MediaItem;
import androidx.media3.common.MediaMetadata;
import androidx.media3.common.MimeTypes;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.common.Timeline;
import androidx.media3.common.TrackSelectionOverride;
import androidx.media3.common.Tracks;
import androidx.media3.common.VideoSize;
import androidx.media3.datasource.DefaultDataSource;
import androidx.media3.datasource.DefaultHttpDataSource;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory;
import androidx.media3.session.MediaSession;
import androidx.media3.ui.AspectRatioFrameLayout;
import androidx.media3.ui.PlayerView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.atomic.AtomicLong;

/** Native Media3 player shared by Android phone/tablet, Android TV and Fire TV. */
public class PlayerActivity extends Activity {
    private static final String TAG = "StormFlixPlayer";
    private static final long SETTINGS_VISIBLE_MS = 15000L;
    private static final long CONTROLS_VISIBLE_MS = 6000L;
    private static final long SEEK_STEP_MS = 10000L;
    private static final long SEEK_FEEDBACK_MS = 850L;
    private static final long SEEK_REPEAT_MIN_MS = 110L;
    private static final int AUTO_NEXT_SECONDS = 10;
    private static final String PLAYER_PREFS = "stormflix_player";
    private static final String PREF_AUTO_NEXT = "auto_next_episode";

    private final ExecutorService io = Executors.newFixedThreadPool(3);
    private final ExecutorService progressIo = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final AtomicLong progressSequence = new AtomicLong();
    private final AtomicLong planGeneration = new AtomicLong();

    private ApiClient api;
    private ExoPlayer player;
    private PlayerView playerView;
    private MediaSession mediaSession;
    private Button settingsButton;
    private TextView seekFeedback;

    private long mediaId;
    private long streamMediaId;
    private String title;
    private volatile String playbackSessionId = UUID.randomUUID().toString();
    private JSONObject currentPlan;
    private long resumePositionMs;
    private long lastKnownPositionMs;
    private long lastKnownDurationMs;
    private long pendingRestorePositionMs = C.TIME_UNSET;
    private boolean pendingRestoreAutoPlay;
    private boolean restoringPosition;
    private boolean fallbackInProgress;
    private boolean versionFallbackAttempted;
    private boolean compatibilityRecoveryAttempted;
    private boolean audioPreferenceApplied;
    private boolean audioRecoveryAttempted;
    private boolean audioFailure;
    private boolean audioMutedForCompatibility;
    private boolean controlsVisible;
    private boolean finishingSent;
    private int audioCheckGeneration;
    private int sourceGeneration;
    private String playbackMode = "Direct Play";
    private List<MediaItem.SubtitleConfiguration> currentSubtitles = new ArrayList<>();
    private long previousEpisodeId;
    private long nextEpisodeId;
    private String previousEpisodeTitle = "Episódio anterior";
    private String nextEpisodeTitle = "Próximo episódio";
    private AlertDialog autoNextDialog;
    private int autoNextRemaining;

    private final Runnable autoNextTick = new Runnable() {
        @Override public void run() {
            if (autoNextDialog == null || !autoNextDialog.isShowing() || nextEpisodeId <= 0) return;
            autoNextRemaining--;
            if (autoNextRemaining <= 0) {
                long id = nextEpisodeId;
                String episodeTitle = nextEpisodeTitle;
                cancelAutoNextCountdown();
                launchEpisode(id, episodeTitle);
                return;
            }
            updateAutoNextMessage();
            main.postDelayed(this, 1000);
        }
    };

    private long seekFeedbackAccumulatedMs;
    private long lastSeekFeedbackAt;
    private long lastSeekDispatchAt;

    private final Runnable heartbeat = new Runnable() {
        @Override public void run() {
            queueProgress("periodic", false);
            main.postDelayed(this, 10000);
        }
    };

    private final Runnable hideSettings = () -> {
        if (settingsButton != null) settingsButton.setVisibility(View.GONE);
    };

    private final Runnable hideControls = () -> {
        controlsVisible = false;
        if (playerView != null) playerView.hideController();
    };

    private final Runnable hideSeekFeedback = () -> {
        if (seekFeedback != null) seekFeedback.setVisibility(View.GONE);
        seekFeedbackAccumulatedMs = 0;
        lastSeekFeedbackAt = 0;
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
        reportEvent("PLAYBACK_SESSION_CREATED", "media=" + mediaId + " client=" + (RemoteUi.isTelevision(this) ? "tv" : "android"));
        loadPlaybackContext(true);
    }

    private void buildPlayer() {
        Map<String,String> headers = new HashMap<>();
        String cookie = api.store().cookieHeader();
        if (!cookie.isEmpty()) headers.put("Cookie", cookie);
        headers.put("User-Agent", "StormFlix-Android/" + BuildConfig.VERSION_NAME);
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("StormFlix-Android/" + BuildConfig.VERSION_NAME)
            .setAllowCrossProtocolRedirects(true)
            .setDefaultRequestProperties(headers);
        DefaultDataSource.Factory data = new DefaultDataSource.Factory(this, http);
        player = new ExoPlayer.Builder(this).setMediaSourceFactory(new DefaultMediaSourceFactory(data)).build();
        applyProfileLanguagePreferences();
        try { mediaSession = new MediaSession.Builder(this, player).build(); }
        catch (Exception e) { Log.w(TAG, "MediaSession unavailable", e); }

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

        seekFeedback = new TextView(this);
        seekFeedback.setTextColor(Color.WHITE);
        seekFeedback.setTextSize(30);
        seekFeedback.setGravity(Gravity.CENTER);
        seekFeedback.setFocusable(false);
        seekFeedback.setClickable(false);
        seekFeedback.setVisibility(View.GONE);
        seekFeedback.setBackground(Ui.round(Color.argb(210, 10, 12, 16), 12));
        root.addView(seekFeedback, new FrameLayout.LayoutParams(Ui.dp(this,170), Ui.dp(this,92), Gravity.CENTER));

        settingsButton = new Button(this);
        settingsButton.setText("⚙");
        settingsButton.setTextColor(Color.WHITE);
        settingsButton.setTextSize(18);
        settingsButton.setAllCaps(false);
        settingsButton.setFocusable(true);
        settingsButton.setFocusableInTouchMode(false);
        settingsButton.setBackground(Ui.round(Color.argb(170, 10, 12, 16), 12));
        settingsButton.setOnClickListener(v -> { showSettingsTemporarily(); showPlayerMenu(); });
        FrameLayout.LayoutParams gear = new FrameLayout.LayoutParams(Ui.dp(this,52), Ui.dp(this,46), Gravity.TOP | Gravity.END);
        gear.setMargins(0, Ui.dp(this,16), Ui.dp(this,16), 0);
        root.addView(settingsButton, gear);
        setContentView(root);
        showSettingsTemporarily();

        player.addListener(new Player.Listener() {
            @Override public void onPlayerError(PlaybackException error) {
                reportEvent("PLAYER_ERROR", error.getErrorCodeName());
                handlePlaybackFailure(error);
            }

            @Override public void onPlaybackStateChanged(int state) {
                updateKnownPlaybackValues();
                reportEvent("PLAYER_STATE", playbackStateName(state));
                if (state == Player.STATE_READY) {
                    reportTimeline("TIMELINE_READY");
                    maybeRestorePendingPosition();
                    scheduleAudioInspection();
                    queueProgress("ready", false);
                } else if (state == Player.STATE_ENDED) {
                    queueProgress("ended", false);
                    if (nextEpisodeId > 0) main.postDelayed(PlayerActivity.this::handleEpisodeEnded, 350);
                }
            }

            @Override public void onTimelineChanged(Timeline timeline, int reason) {
                updateKnownPlaybackValues();
                reportTimeline("TIMELINE_CHANGED");
                if (player != null && player.getPlaybackState() == Player.STATE_READY) maybeRestorePendingPosition();
            }

            @Override public void onTracksChanged(Tracks tracks) { scheduleAudioInspection(); }

            @Override public void onIsPlayingChanged(boolean isPlaying) {
                updateKnownPlaybackValues();
                if (!isPlaying && !finishingSent) queueProgress("pause", false);
            }

            @Override public void onPositionDiscontinuity(Player.PositionInfo oldPosition, Player.PositionInfo newPosition, int reason) {
                updateKnownPlaybackValues();
                if (reason == Player.DISCONTINUITY_REASON_SEEK || reason == Player.DISCONTINUITY_REASON_SEEK_ADJUSTMENT) {
                    reportEvent("POSITION_DISCONTINUITY", "old=" + oldPosition.positionMs + " new=" + newPosition.positionMs + " reason=" + reason);
                    queueProgress(restoringPosition ? "restore_seek_confirmed" : "seek_confirmed", false);
                    if (restoringPosition) {
                        restoringPosition = false;
                        player.setPlayWhenReady(pendingRestoreAutoPlay);
                        if (fallbackInProgress) {
                            fallbackInProgress = false;
                            reportEvent("FALLBACK_COMPLETE", "restored=" + newPosition.positionMs);
                        }
                    }
                }
            }
        });
        main.postDelayed(heartbeat, 10000);
    }

    private void loadPlaybackContext(boolean offerResume) {
        final long target = mediaId;
        safeSubmit(io, () -> {
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
                    if (duration > 0) lastKnownDurationMs = (long)(duration * 1000.0);
                    if (!completed && saved >= 30 && (duration <= 0 || saved / duration < 0.92)) {
                        resumePositionMs = (long)(saved * 1000.0);
                        lastKnownPositionMs = resumePositionMs;
                        reportEvent("RESUME_REQUESTED", "position=" + resumePositionMs + " duration=" + lastKnownDurationMs);
                    }
                }
                readNeighbors(adjacent);
                if (offerResume && resumePositionMs >= 30000) showResumeDialog();
                else loadSource(mediaId, true, C.TIME_UNSET, "SOURCE_INITIAL");
            });
        });
    }

    private void readNeighbors(JSONObject neighbors) {
        previousEpisodeId=0; nextEpisodeId=0;
        if(neighbors==null)return;
        JSONObject previous=neighbors.optJSONObject("previous"),next=neighbors.optJSONObject("next");
        if(previous!=null){previousEpisodeId=previous.optLong("id");previousEpisodeTitle=previous.optString("title","Episódio anterior");}
        if(next!=null){nextEpisodeId=next.optLong("id");nextEpisodeTitle=next.optString("title","Próximo episódio");}
    }

    private void showResumeDialog() {
        String at=clock(resumePositionMs/1000);
        new AlertDialog.Builder(this)
            .setTitle(title==null?"Continuar assistindo":title)
            .setMessage("Continuar de "+at+"?")
            .setPositiveButton("Continuar",(d,w)->loadSource(mediaId,true,resumePositionMs,"SOURCE_INITIAL_RESUME"))
            .setNegativeButton("Reiniciar",(d,w)->{resumePositionMs=0;lastKnownPositionMs=0;loadSource(mediaId,true,C.TIME_UNSET,"SOURCE_INITIAL_RESTART");})
            .setOnCancelListener(d->loadSource(mediaId,true,resumePositionMs,"SOURCE_INITIAL_RESUME"))
            .show();
    }

    private void loadSource(long sourceId, boolean autoPlay, long restorePosition, String sourceEvent) {
        streamMediaId = sourceId;
        audioPreferenceApplied = false;
        audioRecoveryAttempted = false;
        audioFailure = false;
        audioCheckGeneration++;
        requestPlannedSource(sourceId, autoPlay, restorePosition, sourceEvent, true, false, null);
    }

    private void requestPlannedSource(long sourceId, boolean autoPlay, long restorePosition, String sourceEvent, boolean nativeAudioSelection, boolean forceMP4, String preferredLanguage) {
        final long requestedId = sourceId;
        final long generation = planGeneration.incrementAndGet();
        safeSubmit(io, () -> {
            try {
                List<MediaItem.SubtitleConfiguration> subtitles = loadSubtitles(requestedId);
                JSONObject plan = requestPlan(requestedId, nativeAudioSelection, forceMP4, preferredLanguage);
                if (generation != planGeneration.get() || isFinishing()) return;
                String serverSession = plan.optString("playback_session_id", "").trim();
                if (!serverSession.isEmpty()) playbackSessionId = serverSession;
                if (!plan.optBoolean("available", false)) {
                    String reason = plan.optString("reason", "Este aparelho não suporta esta fonte sem transcodificar vídeo.");
                    String code = plan.optString("reason_code", "unsupported");
                    main.post(() -> onPlanUnavailable(requestedId, reason, code));
                    return;
                }

                JSONObject prepared = null;
                String prepareURL = plan.optString("prepare_url", "");
                if (!prepareURL.isEmpty()) {
                    reportEvent("PLAYBACK_PLAN_PREPARE", "mode=" + plan.optString("mode") + " url=" + redactUri(prepareURL));
                    prepared = new JSONObject(api.post(apiPath(prepareURL), new JSONObject()));
                    if (!prepared.optBoolean("ready", false)) throw new Exception("A fonte de compatibilidade não ficou pronta");
                    if (!prepared.optBoolean("seekable", true)) throw new Exception("A fonte de compatibilidade não ficou seekable");
                }
                if (generation != planGeneration.get() || isFinishing()) return;
                JSONObject readyPlan = plan;
                JSONObject readyPrepared = prepared;
                main.post(() -> {
                    if (generation != planGeneration.get() || player == null || isFinishing() || streamMediaId != requestedId) return;
                    currentPlan = readyPlan;
                    currentSubtitles = subtitles;
                    playbackMode = playbackModeLabel(readyPlan.optString("mode", "direct_play"));
                    String sourceURL = readyPrepared == null ? readyPlan.optString("url", "") : readyPrepared.optString("url", readyPlan.optString("url", ""));
                    if (sourceURL.isEmpty()) sourceURL = "/api/v1/media/" + requestedId + "/stream";
                    fallbackInProgress = forceMP4 || readyPlan.optString("mode", "").contains("compatibility") || "remux".equals(readyPlan.optString("mode", ""));
                    reportEvent("PLAYBACK_PLAN_SELECTED", "mode=" + readyPlan.optString("mode") + " reason=" + readyPlan.optString("reason_code") + " source=" + requestedId + " audio=" + readyPlan.optInt("audio_stream", -1));
                    replaceSource(absolutePlanURL(sourceURL), subtitles, autoPlay, restorePosition, sourceEvent);
                    if (forceMP4) Toast.makeText(this, playbackMode, Toast.LENGTH_SHORT).show();
                });
            } catch (Exception e) {
                if (generation != planGeneration.get() || isFinishing()) return;
                main.post(() -> onPlanFailure(requestedId, e));
            }
        });
    }

    private JSONObject requestPlan(long targetMediaId, boolean nativeAudioSelection, boolean forceMP4, String preferredLanguage) throws Exception {
        JSONObject body = PlaybackCapabilities.buildRequest(this, api.store(), playbackSessionId);
        JSONObject caps = body.getJSONObject("capabilities");
        caps.put("native_audio_track_selection", nativeAudioSelection);
        if (forceMP4) caps.put("containers", new JSONArray().put("mp4"));
        if (preferredLanguage != null && !preferredLanguage.trim().isEmpty()) body.put("preferred_audio_language", preferredLanguage.trim());
        return new JSONObject(api.post("/media/" + targetMediaId + "/playback/plan", body));
    }

    private List<MediaItem.SubtitleConfiguration> loadSubtitles(long targetMediaId) {
        List<MediaItem.SubtitleConfiguration> subtitles = new ArrayList<>();
        try {
            JSONArray arr = new JSONArray(api.get("/media/" + targetMediaId + "/subtitles"));
            for (int i=0;i<arr.length();i++) {
                JSONObject s=arr.optJSONObject(i); if(s==null)continue;
                long id=s.optLong("id"); String lang=s.optString("language","pt");
                Uri uri=Uri.parse(api.apiUrl("/media/"+targetMediaId+"/subtitles/"+id+"/vtt"));
                subtitles.add(new MediaItem.SubtitleConfiguration.Builder(uri).setMimeType(MimeTypes.TEXT_VTT).setLanguage(lang).setLabel(lang).build());
            }
        } catch(Exception ignored) {}
        return subtitles;
    }

    private String apiPath(String planURL) {
        String value = planURL == null ? "" : planURL.trim();
        if (value.startsWith("/api/v1")) return value.substring("/api/v1".length());
        return value;
    }

    private String absolutePlanURL(String planURL) {
        String value = planURL == null ? "" : planURL.trim();
        if (value.startsWith("http://") || value.startsWith("https://")) return value;
        if (value.startsWith("/api/v1")) return api.store().baseUrl() + value;
        if (value.startsWith("/")) return api.store().baseUrl() + value;
        return api.apiUrl(value);
    }

    private String playbackModeLabel(String mode) {
        if ("audio_compatibility".equals(mode)) return "Direct Stream · vídeo original + áudio AAC";
        if ("remux".equals(mode)) return "Remux · sem reencode";
        if ("unsupported".equals(mode)) return "Não suportado";
        return "Direct Play";
    }

    private void onPlanUnavailable(long sourceId, String reason, String code) {
        reportEvent("PLAYBACK_PLAN_UNSUPPORTED", "source=" + sourceId + " code=" + code + " reason=" + reason);
        if (!versionFallbackAttempted && sourceId == mediaId) {
            tryCompatibleVersion(null);
            return;
        }
        if (!compatibilityRecoveryAttempted) {
            tryCompatibilityRecovery(null, null);
            return;
        }
        Toast.makeText(this, reason, Toast.LENGTH_LONG).show();
    }

    private void onPlanFailure(long sourceId, Exception error) {
        reportEvent("PLAYBACK_PLAN_FAILED", "source=" + sourceId + " error=" + String.valueOf(error.getMessage()));
        // The server planner is authoritative for native clients. We do not
        // bypass it with a silent raw /stream fallback because doing so would
        // re-introduce duplicated compatibility policy.
        if (!versionFallbackAttempted && sourceId == mediaId) {
            tryCompatibleVersion(null);
            return;
        }
        Toast.makeText(this, "Não foi possível preparar a reprodução: " + String.valueOf(error.getMessage()), Toast.LENGTH_LONG).show();
    }

    private void replaceSource(String uri,List<MediaItem.SubtitleConfiguration> subtitles,boolean autoPlay,long restorePosition,String sourceEvent){
        if(player==null)return;
        updateKnownPlaybackValues(); clearSeekFeedback(); enableAudioTracks(); sourceGeneration++;
        pendingRestorePositionMs=restorePosition; pendingRestoreAutoPlay=autoPlay; restoringPosition=restorePosition!=C.TIME_UNSET&&restorePosition>0;
        MediaMetadata metadata = new MediaMetadata.Builder().setTitle(title == null ? "StormFlix" : title).build();
        MediaItem.Builder builder=new MediaItem.Builder().setUri(Uri.parse(uri)).setMediaId(String.valueOf(streamMediaId)).setMediaMetadata(metadata);
        if(subtitles!=null&&!subtitles.isEmpty())builder.setSubtitleConfigurations(subtitles);
        player.stop(); player.setMediaItem(builder.build()); player.setPlayWhenReady(false); player.prepare(); if(!restoringPosition)player.setPlayWhenReady(autoPlay);
        reportEvent(sourceEvent,"uri="+redactUri(uri)+" restore="+restorePosition+" autoplay="+autoPlay+" mode="+playbackMode);
    }

    private void maybeRestorePendingPosition(){
        if(player==null||pendingRestorePositionMs==C.TIME_UNSET||pendingRestorePositionMs<=0)return;
        long duration=safeDurationMs(); boolean seekable=player.isCurrentMediaItemSeekable();
        if(!seekable||duration<=0||player.getCurrentTimeline().isEmpty()){
            reportEvent(fallbackInProgress?"TIMELINE_AFTER_FALLBACK":"TIMELINE_NOT_READY","duration="+duration+" seekable="+seekable+" empty="+player.getCurrentTimeline().isEmpty());
            return;
        }
        long target=Math.min(pendingRestorePositionMs,Math.max(0,duration-1000)); pendingRestorePositionMs=C.TIME_UNSET; restoringPosition=target>0;
        if(target<=0){restoringPosition=false;player.setPlayWhenReady(pendingRestoreAutoPlay);if(fallbackInProgress){fallbackInProgress=false;reportEvent("FALLBACK_COMPLETE","restored=0");}return;}
        reportEvent("RESTORE_POSITION","target="+target+" duration="+duration+" seekable="+seekable); player.seekTo(target);
    }

    private void showSettingsTemporarily(){if(settingsButton==null)return;settingsButton.setVisibility(View.VISIBLE);main.removeCallbacks(hideSettings);main.postDelayed(hideSettings,SETTINGS_VISIBLE_MS);}
    private void showControlsTemporarily(){if(playerView==null)return;controlsVisible=true;playerView.showController();showSettingsTemporarily();main.removeCallbacks(hideControls);main.postDelayed(hideControls,CONTROLS_VISIBLE_MS);}
    private void hidePlayerControls(){controlsVisible=false;main.removeCallbacks(hideControls);if(playerView!=null)playerView.hideController();if(settingsButton!=null)settingsButton.setVisibility(View.GONE);}

    private void showSeekFeedback(long delta){
        if(seekFeedback==null)return;long now=SystemClock.elapsedRealtime();
        if(lastSeekFeedbackAt==0||now-lastSeekFeedbackAt>SEEK_FEEDBACK_MS+250)seekFeedbackAccumulatedMs=0;
        seekFeedbackAccumulatedMs+=delta;lastSeekFeedbackAt=now;long seconds=Math.abs(seekFeedbackAccumulatedMs)/1000;
        seekFeedback.setText((seekFeedbackAccumulatedMs>=0?"+":"−")+seconds+"s");seekFeedback.setVisibility(View.VISIBLE);
        main.removeCallbacks(hideSeekFeedback);main.postDelayed(hideSeekFeedback,SEEK_FEEDBACK_MS);
    }
    private void clearSeekFeedback(){main.removeCallbacks(hideSeekFeedback);if(seekFeedback!=null)seekFeedback.setVisibility(View.GONE);seekFeedbackAccumulatedMs=0;lastSeekFeedbackAt=0;}

    private void applyProfileLanguagePreferences(){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setPreferredAudioLanguages(languagePriority(api.store().preferredAudio())).setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle())).setTrackTypeDisabled(C.TRACK_TYPE_AUDIO,false).setTrackTypeDisabled(C.TRACK_TYPE_TEXT,false).build());}
    private void enableAudioTracks(){if(player==null)return;player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_AUDIO,false).build());audioMutedForCompatibility=false;}
    private void muteUnsupportedAudio(){if(player==null||audioMutedForCompatibility)return;player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_AUDIO,true).build());audioMutedForCompatibility=true;}

    private void scheduleAudioInspection(){final int generation=++audioCheckGeneration;main.postDelayed(()->{if(generation!=audioCheckGeneration||player==null||isFinishing()||fallbackInProgress)return;if(player.getPlaybackState()==Player.STATE_IDLE)return;inspectAndSelectAudio(player.getCurrentTracks());},1200);}
    private void inspectAndSelectAudio(Tracks tracks){
        if(player==null||isFinishing())return;String wanted=api.store().preferredAudio();Tracks.Group bestGroup=null;int bestIndex=-1,bestScore=-1;boolean hasAudio=false,hasSupportedAudio=false;
        for(Tracks.Group group:tracks.getGroups()){if(group.getType()!=C.TRACK_TYPE_AUDIO)continue;hasAudio=true;for(int i=0;i<group.length;i++){if(!group.isTrackSupported(i))continue;hasSupportedAudio=true;int score=audioScore(group.getTrackFormat(i),wanted);if(score>bestScore){bestScore=score;bestGroup=group;bestIndex=i;}}}
        if(bestGroup!=null&&bestIndex>=0&&!audioPreferenceApplied){audioPreferenceApplied=true;if(!bestGroup.isTrackSelected(bestIndex))player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setOverrideForType(new TrackSelectionOverride(bestGroup.getMediaTrackGroup(),bestIndex)).build());return;}
        if(hasAudio&&!hasSupportedAudio&&!audioRecoveryAttempted){audioRecoveryAttempted=true;audioFailure=true;reportEvent("AUDIO_UNSUPPORTED","tracks detected but no decoder-supported audio");recoverUnsupportedAudio(null);}
    }
    private boolean isAudioOnlyCompatibilityProblem(Tracks tracks){boolean hasAudio=false,supportedAudio=false,supportedVideo=false;for(Tracks.Group group:tracks.getGroups()){if(group.getType()==C.TRACK_TYPE_AUDIO){hasAudio=true;for(int i=0;i<group.length;i++)if(group.isTrackSupported(i))supportedAudio=true;}else if(group.getType()==C.TRACK_TYPE_VIDEO){for(int i=0;i<group.length;i++)if(group.isTrackSupported(i))supportedVideo=true;}}return supportedVideo&&hasAudio&&!supportedAudio;}
    private void recoverUnsupportedAudio(String preferredLanguage){if(player==null||isFinishing())return;muteUnsupportedAudio();Toast.makeText(this,"Áudio incompatível. Mantendo o vídeo e preparando AAC…",Toast.LENGTH_SHORT).show();tryCompatibilityRecovery(null,preferredLanguage);}

    private int audioScore(Format format,String preferred){int score=1;String want=normalizeLang(preferred),primary=primaryLang(want),language=normalizeLang(format.language),label=format.label==null?"":format.label.toLowerCase(Locale.ROOT);if(!want.isEmpty()&&language.equals(want))score+=120;else if(!primary.isEmpty()&&primaryLang(language).equals(primary))score+=100;if(primary.equals("pt")){if(language.equals("por")||language.equals("pt-br")||language.equals("pob"))score+=110;if(label.contains("portugu")||label.contains("pt-br")||label.contains("dublado")||label.contains("brasil"))score+=95;}else if(primary.equals("en")){if(language.equals("eng"))score+=95;if(label.contains("english")||label.contains("ingl"))score+=80;}else if(primary.equals("es")){if(language.equals("spa"))score+=95;if(label.contains("espa")||label.contains("spanish"))score+=80;}return score;}
    private String audioLabel(Format format){String base;if(format.label!=null&&!format.label.trim().isEmpty())base=format.label.trim();else if(format.language!=null&&!format.language.trim().isEmpty())base=format.language.trim();else if(format.sampleMimeType!=null&&!format.sampleMimeType.trim().isEmpty())base=format.sampleMimeType.replace("audio/","").toUpperCase(Locale.ROOT);else base="Áudio";int channels=format.channelCount;if(channels>0)base+=" · "+(channels==1?"mono":channels==2?"2.0":channels+" canais");return base;}
    private String textLabel(Format format){if(format.label!=null&&!format.label.trim().isEmpty())return format.label.trim();if(format.language!=null&&!format.language.trim().isEmpty())return format.language.trim();return"Legenda";}
    private String[] languagePriority(String preferred){String normalized=normalizeLang(preferred),primary=primaryLang(normalized);List<String>out=new ArrayList<>();if(!normalized.isEmpty())out.add(preferred.trim());if(!primary.isEmpty()&&!containsIgnoreCase(out,primary))out.add(primary);if(primary.equals("pt")&&!containsIgnoreCase(out,"por"))out.add("por");else if(primary.equals("en")&&!containsIgnoreCase(out,"eng"))out.add("eng");else if(primary.equals("es")&&!containsIgnoreCase(out,"spa"))out.add("spa");return out.toArray(new String[0]);}
    private boolean containsIgnoreCase(List<String>values,String target){for(String value:values)if(value.equalsIgnoreCase(target))return true;return false;}
    private String normalizeLang(String value){return value==null?"":value.trim().toLowerCase(Locale.ROOT).replace('_','-');}
    private String primaryLang(String value){String normalized=normalizeLang(value);int dash=normalized.indexOf('-');return dash>0?normalized.substring(0,dash):normalized;}

    private void handlePlaybackFailure(PlaybackException error){
        if(player!=null&&isAudioOnlyCompatibilityProblem(player.getCurrentTracks())&&!audioRecoveryAttempted){audioRecoveryAttempted=true;audioFailure=true;recoverUnsupportedAudio(null);return;}
        audioFailure=false;
        if(!versionFallbackAttempted){tryCompatibleVersion(error);return;}
        if(!compatibilityRecoveryAttempted){tryCompatibilityRecovery(error,null);return;}
        showUnsupported(error);
    }

    private void tryCompatibleVersion(PlaybackException original){
        if(versionFallbackAttempted){if(!compatibilityRecoveryAttempted)tryCompatibilityRecovery(original,null);else showUnsupported(original);return;}
        versionFallbackAttempted=true;
        final long restore=snapshotPositionMs(); final boolean wasPlaying=player==null||player.getPlayWhenReady(); final long current=streamMediaId;
        safeSubmit(io,()->{
            try{
                JSONArray versions=new JSONArray(api.get("/media/"+mediaId+"/versions"));
                List<JSONObject> candidates=new ArrayList<>();
                for(int i=0;i<versions.length();i++){JSONObject v=versions.optJSONObject(i);if(v!=null&&v.optLong("id")>0&&v.optLong("id")!=current)candidates.add(v);}
                candidates.sort(Comparator.comparingInt((JSONObject v)->compatibilityRank(v.optString("label","Original"))).reversed());
                JSONObject selected=null;
                for(JSONObject candidate:candidates){
                    try{JSONObject plan=requestPlan(candidate.optLong("id"),true,false,null);if(plan.optBoolean("available",false)){selected=candidate;break;}}catch(Exception ignored){}
                }
                JSONObject chosen=selected;
                main.post(()->{
                    if(isFinishing())return;
                    if(chosen==null){tryCompatibilityRecovery(original,null);return;}
                    long id=chosen.optLong("id");String quality=chosen.optString("label","Compatível");
                    queueProgress("source_change",false);streamMediaId=id;Toast.makeText(this,"Tentando "+quality+" sem transcodificação.",Toast.LENGTH_SHORT).show();
                    loadSource(id,wasPlaying,restore,"SOURCE_ALTERNATIVE");
                });
            }catch(Exception e){main.post(()->tryCompatibilityRecovery(original,null));}
        });
    }
    private int compatibilityRank(String label){String q=label==null?"":label.toLowerCase(Locale.ROOT);if(q.contains("4k")||q.contains("2160"))return 4;if(q.contains("1080"))return 3;if(q.contains("720"))return 2;if(q.contains("480"))return 1;return 0;}

    private void tryCompatibilityRecovery(PlaybackException original,String preferredLanguage){
        if(compatibilityRecoveryAttempted||isFinishing()){showUnsupported(original);return;}
        compatibilityRecoveryAttempted=true;fallbackInProgress=true;
        final long targetMediaId=streamMediaId,beforePosition=snapshotPositionMs();final boolean wasPlaying=player==null||player.getPlayWhenReady();
        queueProgress("fallback_start",false);reportEvent("FALLBACK_START","position="+beforePosition+" force_mp4=true preferred="+String.valueOf(preferredLanguage));
        requestPlannedSource(targetMediaId,wasPlaying,beforePosition,"SOURCE_COMPATIBILITY",false,true,preferredLanguage);
    }

    private void showUnsupported(PlaybackException error){
        if(isFinishing())return;
        String detail=error==null?"":error.getErrorCodeName();
        String reason=currentPlan==null?"":currentPlan.optString("reason","");
        String msg=reason.isEmpty()?"Este aparelho não conseguiu reproduzir esta cópia sem transcodificar vídeo.":reason;
        if(!detail.isEmpty())msg+=" Erro: "+detail+".";
        if(audioFailure){muteUnsupportedAudio();playbackMode="Direct Play · vídeo sem áudio";}
        Toast.makeText(this,msg,Toast.LENGTH_LONG).show();
    }

    private void showPlayerMenu(){
        List<String>items=new ArrayList<>();items.add("Áudio");items.add("Legendas");items.add("Tela / Zoom");
        if(canPictureInPicture())items.add("Picture-in-Picture");
        items.add("Reiniciar do início");items.add("Próximo episódio automático: "+(autoNextEnabled()?"Ligado":"Desligado"));if(previousEpisodeId>0)items.add("Episódio anterior");if(nextEpisodeId>0)items.add("Próximo episódio");items.add("Informações");
        new AlertDialog.Builder(this).setTitle(title==null?"Player StormFlix":title).setItems(items.toArray(new String[0]),(d,which)->{
            String item=items.get(which);if(item.equals("Áudio"))showAudioMenu();else if(item.equals("Legendas"))showSubtitleMenu();else if(item.equals("Tela / Zoom"))showScreenMenu();else if(item.equals("Picture-in-Picture"))enterPictureInPicture();else if(item.equals("Reiniciar do início")){reportEvent("SEEK_REQUESTED","target=0 restart");player.seekTo(0);}else if(item.startsWith("Próximo episódio automático:"))toggleAutoNext();else if(item.equals("Episódio anterior"))launchEpisode(previousEpisodeId,previousEpisodeTitle);else if(item.equals("Próximo episódio"))launchEpisode(nextEpisodeId,nextEpisodeTitle);else showPlaybackInfo();
        }).setNegativeButton("Fechar",null).show();
    }

    private void showAudioMenu(){
        Tracks tracks=player.getCurrentTracks();List<String>labels=new ArrayList<>();List<Tracks.Group>groups=new ArrayList<>();List<Integer>indexes=new ArrayList<>();
        labels.add("Automático · preferência "+api.store().preferredAudio());groups.add(null);indexes.add(-1);
        for(Tracks.Group group:tracks.getGroups()){if(group.getType()!=C.TRACK_TYPE_AUDIO)continue;for(int i=0;i<group.length;i++){String label=audioLabel(group.getTrackFormat(i));if(!group.isTrackSupported(i))label+=" · não suportado";else if(group.isTrackSelected(i))label+=" · atual";labels.add(label);groups.add(group);indexes.add(i);}}
        if(playbackMode.toLowerCase(Locale.ROOT).contains("aac"))labels.add("Compatibilidade AAC · ativa");
        new AlertDialog.Builder(this).setTitle("Áudio").setItems(labels.toArray(new String[0]),(d,which)->{
            if(which==0){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().clearOverrides().setPreferredAudioLanguages(languagePriority(api.store().preferredAudio())).setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle())).setTrackTypeDisabled(C.TRACK_TYPE_AUDIO,false).build());audioMutedForCompatibility=false;audioPreferenceApplied=false;scheduleAudioInspection();return;}
            if(which>=groups.size()){Toast.makeText(this,"Compatibilidade AAC já está ativa.",Toast.LENGTH_SHORT).show();return;}
            Tracks.Group group=groups.get(which);int index=indexes.get(which);Format format=group==null?null:group.getTrackFormat(index);
            if(group==null||!group.isTrackSupported(index)){Toast.makeText(this,"Sem decoder para esta faixa. Preparando AAC…",Toast.LENGTH_SHORT).show();audioFailure=true;recoverUnsupportedAudio(format==null?null:format.language);return;}
            enableAudioTracks();player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(),index)).build());
        }).setNegativeButton("Voltar",null).show();
    }

    private void showSubtitleMenu(){Tracks tracks=player.getCurrentTracks();List<String>labels=new ArrayList<>();List<Tracks.Group>groups=new ArrayList<>();List<Integer>indexes=new ArrayList<>();labels.add("Desativadas");groups.add(null);indexes.add(-1);labels.add("Automático · preferência "+api.store().preferredSubtitle());groups.add(null);indexes.add(-2);for(Tracks.Group group:tracks.getGroups()){if(group.getType()!=C.TRACK_TYPE_TEXT)continue;for(int i=0;i<group.length;i++){String label=textLabel(group.getTrackFormat(i));if(!group.isTrackSupported(i))label+=" · não suportada";else if(group.isTrackSelected(i))label+=" · atual";labels.add(label);groups.add(group);indexes.add(i);}}new AlertDialog.Builder(this).setTitle("Legendas").setItems(labels.toArray(new String[0]),(d,which)->{if(which==0){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_TEXT,true).build());return;}if(which==1){player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().clearOverrides().setTrackTypeDisabled(C.TRACK_TYPE_TEXT,false).setPreferredAudioLanguages(languagePriority(api.store().preferredAudio())).setPreferredTextLanguages(languagePriority(api.store().preferredSubtitle())).build());return;}Tracks.Group group=groups.get(which);int index=indexes.get(which);if(group==null||!group.isTrackSupported(index)){Toast.makeText(this,"Legenda não suportada.",Toast.LENGTH_SHORT).show();return;}player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon().setTrackTypeDisabled(C.TRACK_TYPE_TEXT,false).setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(),index)).build());}).setNegativeButton("Voltar",null).show();}
    private void showScreenMenu(){String[]modes={"Ajustar / sem zoom","16:9 · sem corte","Preencher / Zoom","Esticar para tela"};new AlertDialog.Builder(this).setTitle("Tela").setItems(modes,(d,which)->{if(which==0||which==1)playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FIT);else if(which==2)playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_ZOOM);else playerView.setResizeMode(AspectRatioFrameLayout.RESIZE_MODE_FILL);Toast.makeText(this,modes[which],Toast.LENGTH_SHORT).show();}).setNegativeButton("Voltar",null).show();}
    private void showPlaybackInfo(){StringBuilder info=new StringBuilder();info.append("Modo: ").append(playbackMode);info.append("\nPosição: ").append(clock(snapshotPositionMs()/1000));info.append("\nDuração: ").append(clock(safeDurationMs()/1000));info.append("\nSeekable: ").append(player!=null&&player.isCurrentMediaItemSeekable()?"sim":"não");info.append("\nVídeo: ").append(selectedCodec(C.TRACK_TYPE_VIDEO));info.append("\nÁudio: ").append(selectedCodec(C.TRACK_TYPE_AUDIO));info.append("\nPreferência de áudio: ").append(api.store().preferredAudio());info.append("\nPreferência de legenda: ").append(api.store().preferredSubtitle());info.append("\nFonte: ").append(streamMediaId==mediaId?"principal":"alternativa");if(currentPlan!=null){info.append("\nPlano: ").append(currentPlan.optString("mode","—"));info.append("\nMotivo: ").append(currentPlan.optString("reason_code","—"));info.append("\nContainer: ").append(currentPlan.optString("source_container","—"));String hdr=currentPlan.optString("video_hdr","");if(!hdr.isEmpty())info.append("\nHDR: ").append(hdr);int w=currentPlan.optInt("video_width",0),h=currentPlan.optInt("video_height",0);if(w>0&&h>0)info.append("\nFonte de vídeo: ").append(w).append('x').append(h);info.append("\nÁudio planejado: ").append(currentPlan.optInt("audio_stream",-1));}info.append("\nSessão: ").append(playbackSessionId);info.append("\nGeração: ").append(sourceGeneration);new AlertDialog.Builder(this).setTitle("Informações de reprodução").setMessage(info.toString()).setPositiveButton("OK",null).show();}

    private String selectedCodec(int type){if(player==null)return"—";for(Tracks.Group group:player.getCurrentTracks().getGroups()){if(group.getType()!=type)continue;for(int i=0;i<group.length;i++)if(group.isTrackSelected(i))return codecName(group.getTrackFormat(i));}return"—";}
    private String codecName(Format f){String value=f.codecs;if(value==null||value.isEmpty())value=f.sampleMimeType;if(value==null||value.isEmpty())return"—";int slash=value.indexOf('/');if(slash>=0)value=value.substring(slash+1);return value.toUpperCase(Locale.ROOT);}
    private long snapshotPositionMs(){if(player!=null){long p=player.getCurrentPosition();if(p>=0)lastKnownPositionMs=p;}return Math.max(0,lastKnownPositionMs);}
    private long safeDurationMs(){if(player!=null){long d=player.getDuration();if(d!=C.TIME_UNSET&&d>0)lastKnownDurationMs=d;}return Math.max(0,lastKnownDurationMs);}
    private void updateKnownPlaybackValues(){snapshotPositionMs();safeDurationMs();}

    private void queueProgress(String reason,boolean finishSession){
        if(player==null||mediaId<=0)return;updateKnownPlaybackValues();final long target=mediaId,position=snapshotPositionMs(),duration=safeDurationMs();final String state=player.isPlaying()?"playing":"paused",mode=playbackMode.toLowerCase(Locale.ROOT),video=selectedCodec(C.TRACK_TYPE_VIDEO),audio=selectedCodec(C.TRACK_TYPE_AUDIO),resolution=player.getVideoSize().width>0?player.getVideoSize().width+"x"+player.getVideoSize().height:"";final int bitrate=selectedBitrateKbps();final long sequence=progressSequence.incrementAndGet(),eventMs=System.currentTimeMillis();final String session=playbackSessionId;
        Log.i(TAG,"PROGRESS_SAVE_REQUEST session="+session+" seq="+sequence+" reason="+reason+" position="+position+" duration="+duration);
        safeSubmit(progressIo,()->{try{JSONObject body=new JSONObject().put("position_seconds",position/1000.0).put("duration_seconds",duration/1000.0).put("state",state).put("mode",mode).put("video_codec","—".equals(video)?"":video).put("audio_codec","—".equals(audio)?"":audio).put("resolution",resolution).put("bitrate_kbps",bitrate).put("playback_session_id",session).put("progress_sequence",sequence).put("progress_event_ms",eventMs).put("progress_reason",reason);String response=api.post("/media/"+target+"/playback",body);Log.i(TAG,"PROGRESS_SAVE_SUCCESS session="+session+" seq="+sequence+" response="+response.trim());if(finishSession)api.delete("/media/"+target+"/playback");}catch(Exception e){Log.w(TAG,"PROGRESS_SAVE_FAILED session="+session+" seq="+sequence,e);}});
    }
    private int selectedBitrateKbps(){if(player==null)return 0;for(Tracks.Group group:player.getCurrentTracks().getGroups()){if(group.getType()!=C.TRACK_TYPE_VIDEO)continue;for(int i=0;i<group.length;i++)if(group.isTrackSelected(i)){Format f=group.getTrackFormat(i);int b=f.averageBitrate>0?f.averageBitrate:f.peakBitrate;return b>0?b/1000:0;}}return 0;}

    private void seekRelative(long delta){if(player==null)return;long duration=safeDurationMs(),current=snapshotPositionMs(),target=Math.max(0,current+delta);if(duration>0)target=Math.min(duration,target);reportEvent("SEEK_REQUESTED","from="+current+" target="+target+" delta="+delta);player.seekTo(target);showSeekFeedback(delta);showControlsTemporarily();}
    private void togglePlay(){if(player==null)return;if(player.isPlaying()){player.pause();clearSeekFeedback();if(!finishingSent)queueProgress("pause_key",false);}else player.play();showControlsTemporarily();}
    private boolean shouldHandleSeekRepeat(KeyEvent event){long now=SystemClock.elapsedRealtime();if(event.getRepeatCount()==0){lastSeekDispatchAt=now;return true;}if(now-lastSeekDispatchAt<SEEK_REPEAT_MIN_MS)return false;lastSeekDispatchAt=now;return true;}

    @Override public boolean dispatchKeyEvent(KeyEvent event){
        int key=event.getKeyCode();boolean seekKey=key==KeyEvent.KEYCODE_DPAD_LEFT||key==KeyEvent.KEYCODE_DPAD_RIGHT||key==KeyEvent.KEYCODE_MEDIA_FAST_FORWARD||key==KeyEvent.KEYCODE_MEDIA_REWIND;
        if(event.getAction()==KeyEvent.ACTION_UP&&seekKey)return true;if(event.getAction()!=KeyEvent.ACTION_DOWN)return super.dispatchKeyEvent(event);
        if(key==KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE||key==KeyEvent.KEYCODE_HEADSETHOOK){togglePlay();return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_PLAY){if(player!=null)player.play();showControlsTemporarily();return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_PAUSE||key==KeyEvent.KEYCODE_MEDIA_STOP){if(player!=null)player.pause();clearSeekFeedback();if(!finishingSent)queueProgress("pause_key",false);showControlsTemporarily();return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_FAST_FORWARD){if(shouldHandleSeekRepeat(event))seekRelative(SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_REWIND){if(shouldHandleSeekRepeat(event))seekRelative(-SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_NEXT&&nextEpisodeId>0){launchEpisode(nextEpisodeId,nextEpisodeTitle);return true;}
        if(key==KeyEvent.KEYCODE_MEDIA_PREVIOUS&&previousEpisodeId>0){launchEpisode(previousEpisodeId,previousEpisodeTitle);return true;}
        if(key==KeyEvent.KEYCODE_MENU||key==KeyEvent.KEYCODE_INFO){showSettingsTemporarily();showPlayerMenu();return true;}
        if(key==KeyEvent.KEYCODE_CAPTIONS){showSubtitleMenu();return true;}
        if(key==KeyEvent.KEYCODE_DPAD_LEFT){if(controlsVisible&&isPlayerControlFocused())return super.dispatchKeyEvent(event);if(shouldHandleSeekRepeat(event))seekRelative(-SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_DPAD_RIGHT){if(controlsVisible&&isPlayerControlFocused())return super.dispatchKeyEvent(event);if(shouldHandleSeekRepeat(event))seekRelative(SEEK_STEP_MS);return true;}
        if(key==KeyEvent.KEYCODE_DPAD_CENTER||key==KeyEvent.KEYCODE_ENTER){if(!controlsVisible){showControlsTemporarily();return true;}return super.dispatchKeyEvent(event);}
        if(key==KeyEvent.KEYCODE_DPAD_UP||key==KeyEvent.KEYCODE_DPAD_DOWN){showControlsTemporarily();return super.dispatchKeyEvent(event);}
        return super.dispatchKeyEvent(event);
    }

    private boolean isPlayerControlFocused(){View focused=getCurrentFocus();if(focused==null||focused==playerView||focused==settingsButton)return false;View current=focused;while(current!=null){if(current==playerView)return true;if(!(current.getParent() instanceof View))break;current=(View)current.getParent();}return false;}
    private boolean autoNextEnabled(){return getSharedPreferences(PLAYER_PREFS,MODE_PRIVATE).getBoolean(PREF_AUTO_NEXT,true);}
    private void setAutoNextEnabled(boolean enabled){getSharedPreferences(PLAYER_PREFS,MODE_PRIVATE).edit().putBoolean(PREF_AUTO_NEXT,enabled).apply();}
    private void toggleAutoNext(){boolean enabled=!autoNextEnabled();setAutoNextEnabled(enabled);Toast.makeText(this,"Próximo episódio automático "+(enabled?"ligado":"desligado"),Toast.LENGTH_SHORT).show();}
    private void handleEpisodeEnded(){if(nextEpisodeId<=0||isFinishing())return;if(autoNextEnabled())startAutoNextCountdown();else offerNextEpisode();}
    private void startAutoNextCountdown(){
        if(nextEpisodeId<=0||isFinishing())return;
        cancelAutoNextCountdown();
        autoNextRemaining=AUTO_NEXT_SECONDS;
        autoNextDialog=new AlertDialog.Builder(this)
            .setTitle("A seguir")
            .setMessage(nextEpisodeTitle+"\nReprodução automática em "+autoNextRemaining+"s.")
            .setPositiveButton("Reproduzir agora",(d,w)->launchEpisode(nextEpisodeId,nextEpisodeTitle))
            .setNegativeButton("Cancelar",(d,w)->cancelAutoNextCountdown())
            .setNeutralButton("Desativar automático",(d,w)->{setAutoNextEnabled(false);cancelAutoNextCountdown();})
            .create();
        autoNextDialog.setOnDismissListener(d->main.removeCallbacks(autoNextTick));
        autoNextDialog.show();
        main.postDelayed(autoNextTick,1000);
    }
    private void updateAutoNextMessage(){if(autoNextDialog!=null&&autoNextDialog.isShowing())autoNextDialog.setMessage(nextEpisodeTitle+"\nReprodução automática em "+autoNextRemaining+"s.");}
    private void cancelAutoNextCountdown(){main.removeCallbacks(autoNextTick);AlertDialog dialog=autoNextDialog;autoNextDialog=null;if(dialog!=null&&dialog.isShowing())dialog.dismiss();}
    private void offerNextEpisode(){if(nextEpisodeId<=0||isFinishing())return;cancelAutoNextCountdown();new AlertDialog.Builder(this).setTitle("Próximo episódio").setMessage(nextEpisodeTitle).setPositiveButton("Reproduzir",(d,w)->launchEpisode(nextEpisodeId,nextEpisodeTitle)).setNegativeButton("Fechar",null).show();}
    private void launchEpisode(long id,String episodeTitle){if(id<=0)return;cancelAutoNextCountdown();clearSeekFeedback();if(!finishingSent){finishingSent=true;queueProgress("episode_change",true);}Intent intent=new Intent(this,PlayerActivity.class);intent.putExtra("media_id",id);intent.putExtra("title",episodeTitle);startActivity(intent);finish();}

    private boolean canPictureInPicture(){return Build.VERSION.SDK_INT>=26&&!RemoteUi.isTelevision(this);}
    private void enterPictureInPicture(){
        if(!canPictureInPicture()||player==null)return;
        try{
            VideoSize size=player.getVideoSize();int w=size.width>0?size.width:16,h=size.height>0?size.height:9;
            PictureInPictureParams params=new PictureInPictureParams.Builder().setAspectRatio(new Rational(w,h)).build();
            enterPictureInPictureMode(params);
        }catch(Exception e){Log.w(TAG,"PiP unavailable",e);}
    }

    private void reportTimeline(String event){if(player==null)return;long raw=player.getDuration();String details="rawDuration="+raw+" safeDuration="+safeDurationMs()+" position="+snapshotPositionMs()+" seekable="+player.isCurrentMediaItemSeekable()+" empty="+player.getCurrentTimeline().isEmpty()+" mediaItem="+(player.getCurrentMediaItem()==null?"null":player.getCurrentMediaItem().mediaId);reportEvent(event,details);}
    private void reportEvent(String event,String details){
        if(Looper.myLooper()!=Looper.getMainLooper()){main.post(()->reportEvent(event,details));return;}
        long position=snapshotPositionMs(),duration=safeDurationMs();boolean seekable=player!=null&&player.isCurrentMediaItemSeekable();int state=player==null?Player.STATE_IDLE:player.getPlaybackState();int generation=sourceGeneration;String session=playbackSessionId;
        String line=event+" session="+session+" generation="+generation+" position="+position+" duration="+duration+" seekable="+seekable+" state="+state+" "+(details==null?"":details);Log.i(TAG,line);
        if(api==null||mediaId<=0)return;safeSubmit(io,()->{try{JSONObject body=new JSONObject().put("event",event).put("playback_session_id",session).put("source_generation",generation).put("position_seconds",position/1000.0).put("duration_seconds",duration/1000.0).put("seekable",seekable).put("playback_state",state).put("details",details==null?"":details);api.post("/media/"+mediaId+"/playback/event",body);}catch(Exception ignored){}});
    }

    private void safeSubmit(ExecutorService executor,Runnable task){try{executor.submit(task);}catch(RejectedExecutionException ignored){}}
    private String playbackStateName(int state){if(state==Player.STATE_IDLE)return"IDLE";if(state==Player.STATE_BUFFERING)return"BUFFERING";if(state==Player.STATE_READY)return"READY";if(state==Player.STATE_ENDED)return"ENDED";return String.valueOf(state);}
    private String redactUri(String uri){if(uri==null)return"";int q=uri.indexOf('?');return q>=0?uri.substring(0,q)+"?[redacted-query]":uri;}
    private String clock(long seconds){seconds=Math.max(0,seconds);long h=seconds/3600,m=(seconds%3600)/60,s=seconds%60;return h>0?String.format(Locale.US,"%d:%02d:%02d",h,m,s):String.format(Locale.US,"%d:%02d",m,s);}
    private void immersive(){getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN|View.SYSTEM_UI_FLAG_HIDE_NAVIGATION|View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY|View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN|View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION|View.SYSTEM_UI_FLAG_LAYOUT_STABLE);}

    @Override public void onBackPressed(){if(controlsVisible){hidePlayerControls();return;}clearSeekFeedback();if(!finishingSent){finishingSent=true;queueProgress("back",true);}finish();}
    @Override protected void onPause(){clearSeekFeedback();if(!finishingSent)queueProgress("onPause",false);super.onPause();}
    @Override protected void onStop(){clearSeekFeedback();if(!finishingSent)queueProgress("onStop",false);super.onStop();}
    @Override public void onUserLeaveHint(){clearSeekFeedback();if(canPictureInPicture()&&player!=null&&player.isPlaying())enterPictureInPicture();if(!finishingSent)queueProgress("background",false);super.onUserLeaveHint();}
    @Override public void onPictureInPictureModeChanged(boolean isInPictureInPictureMode, Configuration newConfig){super.onPictureInPictureModeChanged(isInPictureInPictureMode,newConfig);if(isInPictureInPictureMode)hidePlayerControls();else immersive();}
    @Override public void onWindowFocusChanged(boolean hasFocus){super.onWindowFocusChanged(hasFocus);if(!hasFocus){clearSeekFeedback();if(!finishingSent)queueProgress("focus_lost",false);}else immersive();}
    @Override protected void onDestroy(){planGeneration.incrementAndGet();audioCheckGeneration++;main.removeCallbacks(heartbeat);main.removeCallbacks(hideSettings);main.removeCallbacks(hideControls);cancelAutoNextCountdown();clearSeekFeedback();if(!finishingSent&&player!=null){finishingSent=true;queueProgress("destroy",true);}if(mediaSession!=null){mediaSession.release();mediaSession=null;}if(player!=null){player.release();player=null;}io.shutdown();progressIo.shutdown();super.onDestroy();}
}
