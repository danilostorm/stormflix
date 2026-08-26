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
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.SeekBar;
import android.widget.TextView;
import android.widget.Toast;

import androidx.media3.common.MediaItem;
import androidx.media3.common.Player;
import androidx.media3.datasource.DefaultDataSource;
import androidx.media3.datasource.DefaultHttpDataSource;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MusicActivity extends Activity {
    private final ExecutorService io = Executors.newFixedThreadPool(2);
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private ExoPlayer player;
    private LinearLayout content;
    private TextView nowTitle;
    private TextView nowArtist;
    private Button playPause;
    private SeekBar progress;
    private JSONObject currentTrack;
    private long lastReportedPosition;

    private final Runnable progressTick = new Runnable() {
        @Override public void run() {
            if (player != null) {
                long duration = Math.max(0, player.getDuration());
                long pos = Math.max(0, player.getCurrentPosition());
                progress.setMax((int)Math.min(Integer.MAX_VALUE, duration));
                progress.setProgress((int)Math.min(Integer.MAX_VALUE, pos));
                playPause.setText(player.isPlaying() ? "❚❚" : "▶");
                if (currentTrack != null && player.isPlaying() && pos - lastReportedPosition >= 15000) {
                    reportListening((pos - lastReportedPosition) / 1000.0, false, false);
                    lastReportedPosition = pos;
                }
            }
            main.postDelayed(this, 1000);
        }
    };

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        api = new ApiClient(this);
        images = new ImageLoader(this);
        buildPlayer();
        buildUi();
        loadHome();
    }

    private void buildPlayer() {
        Map<String,String> headers = new HashMap<>();
        String cookie = api.store().cookieHeader();
        if (!cookie.isEmpty()) headers.put("Cookie", cookie);
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("StormFlix-Android/0.1.4")
            .setAllowCrossProtocolRedirects(true)
            .setDefaultRequestProperties(headers);
        DefaultDataSource.Factory data = new DefaultDataSource.Factory(this, http);
        player = new ExoPlayer.Builder(this).setMediaSourceFactory(new DefaultMediaSourceFactory(data)).build();
        player.addListener(new Player.Listener() {
            @Override public void onPlaybackStateChanged(int state) {
                if (state == Player.STATE_ENDED && currentTrack != null) reportListening(0, false, true);
            }
        });
    }

    private void buildUi() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(Ui.BG);

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        content = Ui.vertical(this, 18);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        root.addView(scroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1));

        LinearLayout mini = Ui.horizontal(this, 12);
        mini.setBackgroundColor(Color.rgb(14,17,23));
        playPause = Ui.button(this, "▶", true);
        playPause.setOnClickListener(v -> { if (player.isPlaying()) player.pause(); else player.play(); });
        mini.addView(playPause, new LinearLayout.LayoutParams(Ui.dp(this,58), Ui.dp(this,48)));
        LinearLayout text = Ui.vertical(this, 0);
        nowTitle = Ui.title(this, "Nada tocando", 13);
        nowArtist = Ui.muted(this, "StormFlix Música", 10);
        text.addView(nowTitle); text.addView(nowArtist);
        mini.addView(text, Ui.margin(this, 0, ViewGroup.LayoutParams.WRAP_CONTENT, 12,0,12,0));
        LinearLayout.LayoutParams textLp = (LinearLayout.LayoutParams) text.getLayoutParams(); textLp.width = 0; textLp.weight = 1; text.setLayoutParams(textLp);
        progress = new SeekBar(this);
        progress.setOnSeekBarChangeListener(new SeekBar.OnSeekBarChangeListener() {
            public void onProgressChanged(SeekBar s, int p, boolean fromUser) {}
            public void onStartTrackingTouch(SeekBar s) {}
            public void onStopTrackingTouch(SeekBar s) { player.seekTo(s.getProgress()); lastReportedPosition = s.getProgress(); }
        });
        mini.addView(progress, new LinearLayout.LayoutParams(Ui.dp(this,220), ViewGroup.LayoutParams.WRAP_CONTENT));
        root.addView(mini, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this,72)));
        setContentView(root);
        main.post(progressTick);
    }

    private void loadHome() {
        Ui.clear(content);
        LinearLayout head = Ui.horizontal(this, 0);
        Button back = Ui.button(this, "←", false); back.setOnClickListener(v -> finish());
        head.addView(back, new LinearLayout.LayoutParams(Ui.dp(this,52), Ui.dp(this,44)));
        LinearLayout copy = Ui.vertical(this, 0);
        copy.addView(Ui.title(this, "Música", 31));
        copy.addView(Ui.muted(this, "Sua biblioteca sonora do StormFlix", 12));
        head.addView(copy, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 14,0,0,0));
        content.addView(head);
        content.addView(Ui.muted(this, "Carregando álbuns, artistas e faixas…", 13), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,18,0,0));
        io.submit(() -> {
            try {
                JSONObject home = new JSONObject(api.get("/music/home"));
                main.post(() -> render(home));
            } catch (Exception e) {
                main.post(() -> Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show());
            }
        });
    }

    private void render(JSONObject home) {
        Ui.clear(content);
        LinearLayout head = Ui.horizontal(this, 0);
        Button back = Ui.button(this, "←", false); back.setOnClickListener(v -> finish());
        head.addView(back, new LinearLayout.LayoutParams(Ui.dp(this,52), Ui.dp(this,44)));
        LinearLayout copy = Ui.vertical(this, 0);
        copy.addView(Ui.title(this, "Música", 31));
        copy.addView(Ui.muted(this, home.optBoolean("indexing") ? "Organizando metadados em segundo plano" : "Álbuns, artistas e faixas do seu Drive", 12));
        head.addView(copy, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 14,0,0,0));
        content.addView(head);

        addAlbumRail("Álbuns adicionados recentemente", home.optJSONArray("recently_added_albums"));
        addTrackList("Tocadas recentemente", home.optJSONArray("recently_played"), 10);
        addTrackList("Mais ouvidas", home.optJSONArray("most_played"), 10);
        addTrackList("Faixas recentes", home.optJSONArray("recently_added_tracks"), 18);
    }

    private void addAlbumRail(String title, JSONArray albums) {
        if (albums == null || albums.length() == 0) return;
        content.addView(Ui.title(this, title, 21), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,24,0,10));
        HorizontalScrollView hs = new HorizontalScrollView(this); hs.setHorizontalScrollBarEnabled(false);
        LinearLayout rail = Ui.horizontal(this,0);
        for (int i=0;i<albums.length();i++) {
            JSONObject album = albums.optJSONObject(i); if (album == null) continue;
            LinearLayout card = Ui.vertical(this,0); card.setFocusable(true); card.setClickable(true);
            ImageView cover = new ImageView(this); cover.setScaleType(ImageView.ScaleType.CENTER_CROP); cover.setBackground(Ui.round(Color.rgb(25,29,38),10));
            card.addView(cover,new LinearLayout.LayoutParams(Ui.dp(this,160),Ui.dp(this,160)));
            String url=album.optString("cover_url",""); if(!url.isEmpty())images.load(cover,url);
            TextView t=Ui.title(this,album.optString("title","Álbum"),12);t.setMaxLines(1);card.addView(t,Ui.margin(this,Ui.dp(this,160),ViewGroup.LayoutParams.WRAP_CONTENT,0,7,0,0));
            TextView a=Ui.muted(this,album.optString("artist",""),10);a.setMaxLines(1);card.addView(a);
            long trackId=album.optLong("representative_track_id");
            card.setOnClickListener(v -> loadAndPlay(trackId));
            card.setOnFocusChangeListener((v,f)->{v.setScaleX(f?1.05f:1);v.setScaleY(f?1.05f:1);});
            rail.addView(card,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,0,12,0));
        }
        hs.addView(rail); content.addView(hs);
    }

    private void addTrackList(String title, JSONArray tracks, int max) {
        if (tracks == null || tracks.length() == 0) return;
        content.addView(Ui.title(this,title,21),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,24,0,10));
        LinearLayout list=Ui.vertical(this,0);
        for(int i=0;i<Math.min(max,tracks.length());i++){
            JSONObject track=tracks.optJSONObject(i); if(track==null)continue;
            Button row=Ui.button(this,"▶  "+track.optString("title","Faixa")+"   ·   "+track.optString("artist",""),false);
            row.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);
            row.setOnClickListener(v->playTrack(track));
            list.addView(row,Ui.margin(this,ViewGroup.LayoutParams.MATCH_PARENT,Ui.dp(this,48),0,0,0,5));
        }
        content.addView(list);
    }

    private void loadAndPlay(long id) {
        if (id <= 0) return;
        io.submit(() -> {
            try {
                JSONObject track = new JSONObject(api.get("/music/tracks/" + id));
                main.post(() -> playTrack(track));
            } catch (Exception e) { main.post(() -> Toast.makeText(this,e.getMessage(),Toast.LENGTH_LONG).show()); }
        });
    }

    private void playTrack(JSONObject track) {
        long id = track.optLong("id"); if (id <= 0) return;
        currentTrack = track;
        nowTitle.setText(track.optString("title","Faixa"));
        nowArtist.setText(track.optString("artist", track.optString("album_artist","")));
        player.setMediaItem(new MediaItem.Builder().setUri(Uri.parse(api.apiUrl("/music/tracks/"+id+"/stream"))).setMediaId(String.valueOf(id)).build());
        player.prepare(); player.play();
        lastReportedPosition = 0;
        reportListening(0, true, false);
    }

    private void reportListening(double delta, boolean started, boolean completed) {
        JSONObject track=currentTrack; if(track==null)return; long id=track.optLong("id"); if(id<=0)return;
        io.submit(() -> {
            try { api.post("/music/tracks/"+id+"/listening",new JSONObject().put("delta_seconds",delta).put("started",started).put("completed",completed)); }
            catch(Exception ignored){}
        });
    }

    @Override protected void onDestroy() {
        main.removeCallbacks(progressTick);
        if (player != null) { player.release(); player = null; }
        io.shutdownNow();
        super.onDestroy();
    }
}
