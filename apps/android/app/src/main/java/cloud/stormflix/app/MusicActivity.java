package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.KeyEvent;
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

import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
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
    private ImageView nowCover;
    private TextView nowTitle;
    private TextView nowArtist;
    private Button previousButton;
    private Button playPause;
    private Button nextButton;
    private Button queueButton;
    private SeekBar progress;
    private JSONObject currentTrack;
    private final List<JSONObject> allTracks = new ArrayList<>();
    private final List<JSONObject> queue = new ArrayList<>();
    private int queueIndex = -1;
    private long lastReportedPosition;
    private long lastMonitoringAt;

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
                long now = System.currentTimeMillis();
                if (currentTrack != null && now - lastMonitoringAt >= 10000) {
                    reportPlayback();
                    lastMonitoringAt = now;
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
            .setUserAgent("StormFlix-Android/0.2.0")
            .setAllowCrossProtocolRedirects(true)
            .setDefaultRequestProperties(headers);
        DefaultDataSource.Factory data = new DefaultDataSource.Factory(this, http);
        player = new ExoPlayer.Builder(this).setMediaSourceFactory(new DefaultMediaSourceFactory(data)).build();
        player.addListener(new Player.Listener() {
            @Override public void onPlaybackStateChanged(int state) {
                if (state == Player.STATE_READY && currentTrack != null) { reportPlayback(); lastMonitoringAt = System.currentTimeMillis(); }
                if (state == Player.STATE_ENDED && currentTrack != null) {
                    long id = currentTrack.optLong("id");
                    reportListening(0, false, true);
                    finishPlayback(id);
                    step(1);
                }
            }
            @Override public void onIsPlayingChanged(boolean isPlaying) { if (currentTrack != null) reportPlayback(); }
        });
    }

    private void buildUi() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(Ui.BG);

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        content = Ui.vertical(this, RemoteUi.isTelevision(this) ? 28 : 18);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        root.addView(scroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1));

        LinearLayout mini = Ui.horizontal(this, 11);
        mini.setGravity(Gravity.CENTER_VERTICAL);
        mini.setPadding(Ui.dp(this,14),Ui.dp(this,9),Ui.dp(this,14),Ui.dp(this,9));
        mini.setBackgroundColor(Color.rgb(11,14,20));

        FrameLayout coverBox = new FrameLayout(this);
        TextView note = Ui.title(this,"♪",20); note.setGravity(Gravity.CENTER); coverBox.addView(note,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));
        nowCover = new ImageView(this); nowCover.setScaleType(ImageView.ScaleType.CENTER_CROP); coverBox.addView(nowCover,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));
        mini.addView(coverBox,new LinearLayout.LayoutParams(Ui.dp(this,54),Ui.dp(this,54)));

        LinearLayout text = Ui.vertical(this, 0);
        nowTitle = Ui.title(this, "Nada tocando", 13); nowTitle.setMaxLines(1);
        nowArtist = Ui.muted(this, "StormFlix Música", 10); nowArtist.setMaxLines(1);
        text.addView(nowTitle); text.addView(nowArtist);
        mini.addView(text,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));

        previousButton=Ui.button(this,"|◀",false);previousButton.setOnClickListener(v->step(-1));mini.addView(previousButton,new LinearLayout.LayoutParams(Ui.dp(this,54),Ui.dp(this,46)));
        playPause=Ui.button(this,"▶",true);playPause.setOnClickListener(v->toggle());mini.addView(playPause,new LinearLayout.LayoutParams(Ui.dp(this,58),Ui.dp(this,48)));
        nextButton=Ui.button(this,"▶|",false);nextButton.setOnClickListener(v->step(1));mini.addView(nextButton,new LinearLayout.LayoutParams(Ui.dp(this,54),Ui.dp(this,46)));

        progress=new SeekBar(this);progress.setFocusable(true);
        progress.setOnSeekBarChangeListener(new SeekBar.OnSeekBarChangeListener(){public void onProgressChanged(SeekBar s,int p,boolean fromUser){}public void onStartTrackingTouch(SeekBar s){}public void onStopTrackingTouch(SeekBar s){player.seekTo(s.getProgress());lastReportedPosition=s.getProgress();reportPlayback();}});
        mini.addView(progress,new LinearLayout.LayoutParams(RemoteUi.isTelevision(this)?Ui.dp(this,300):Ui.dp(this,150),ViewGroup.LayoutParams.WRAP_CONTENT));
        queueButton=Ui.button(this,"Fila",false);queueButton.setOnClickListener(v->showQueue());mini.addView(queueButton,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT,Ui.dp(this,44)));

        root.addView(mini,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,RemoteUi.isTelevision(this)?Ui.dp(this,82):Ui.dp(this,76)));
        setContentView(root);
        main.post(progressTick);
    }

    private void loadHome() {
        Ui.clear(content);
        header(true);
        content.addView(Ui.muted(this,"Carregando sua biblioteca de música…",13),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,16,0,0));
        io.submit(() -> {
            try {
                JSONObject home = new JSONObject(api.get("/music/home"));
                JSONArray tracks = new JSONArray(api.get("/music/tracks?limit=5000"));
                List<JSONObject> parsed = jsonList(tracks);
                main.post(() -> { allTracks.clear(); allTracks.addAll(parsed); render(home); });
            } catch (Exception e) { main.post(() -> Toast.makeText(this,e.getMessage(),Toast.LENGTH_LONG).show()); }
        });
    }

    private void header(boolean loading) {
        LinearLayout head=Ui.horizontal(this,12);head.setGravity(Gravity.CENTER_VERTICAL);
        Button back=Ui.button(this,"←",false);back.setOnClickListener(v->finish());head.addView(back,new LinearLayout.LayoutParams(Ui.dp(this,54),Ui.dp(this,46)));
        LinearLayout copy=Ui.vertical(this,0);copy.addView(Ui.title(this,"Música",RemoteUi.isTelevision(this)?35:31));copy.addView(Ui.muted(this,"Álbuns, artistas e faixas do seu StormFlix",12));
        head.addView(copy,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));
        if(!loading){Button fila=Ui.button(this,"Minha fila",false);fila.setOnClickListener(v->showQueue());head.addView(fila);}
        content.addView(head);
    }

    private void render(JSONObject home) {
        Ui.clear(content);header(false);
        JSONArray recent=home.optJSONArray("recently_played"),albums=home.optJSONArray("recently_added_albums"),most=home.optJSONArray("most_played"),added=home.optJSONArray("recently_added_tracks");
        addTrackRail("Tocadas recentemente",recent,12);
        addAlbumRail("Álbuns",albums);
        addArtistRail("Artistas");
        addTrackList("Mais ouvidas",most,10);
        addTrackList("Faixas recentes",added,16);
        if((recent==null||recent.length()==0)&&(albums==null||albums.length()==0)&&allTracks.isEmpty()){
            content.addView(Ui.title(this,"Sua biblioteca está vazia",22),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,28,0,8));
            content.addView(Ui.muted(this,"Adicione uma biblioteca do tipo Música no painel administrativo e execute o scan.",13));
        }
        RemoteUi.focusFirst(content);
    }

    private void addTrackRail(String title, JSONArray tracks, int max) {
        List<JSONObject> list=jsonList(tracks);if(list.isEmpty())return;
        content.addView(Ui.title(this,title,21),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,25,0,10));
        HorizontalScrollView hs=new HorizontalScrollView(this);hs.setHorizontalScrollBarEnabled(false);hs.setFocusable(false);LinearLayout rail=Ui.horizontal(this,0);
        for(int i=0;i<Math.min(max,list.size());i++){JSONObject track=list.get(i);LinearLayout card=Ui.vertical(this,7);card.setFocusable(true);card.setClickable(true);card.setBackground(Ui.round(Color.rgb(18,23,31),12));card.setPadding(Ui.dp(this,12),Ui.dp(this,12),Ui.dp(this,12),Ui.dp(this,12));card.setOnFocusChangeListener((v,f)->RemoteUi.cardFocus(v,f));card.addView(squareCover(track.optString("cover_url",""),Ui.dp(this,150)));TextView t=Ui.title(this,track.optString("title","Faixa"),12);t.setMaxLines(1);card.addView(t,new LinearLayout.LayoutParams(Ui.dp(this,150),ViewGroup.LayoutParams.WRAP_CONTENT));TextView a=Ui.muted(this,track.optString("artist","Artista desconhecido"),10);a.setMaxLines(1);card.addView(a,new LinearLayout.LayoutParams(Ui.dp(this,150),ViewGroup.LayoutParams.WRAP_CONTENT));card.setOnClickListener(v->playTrack(track,list));rail.addView(card,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,0,10,0));}
        hs.addView(rail);content.addView(hs);
    }

    private void addAlbumRail(String title, JSONArray albums) {
        if(albums==null||albums.length()==0)return;content.addView(Ui.title(this,title,21),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,25,0,10));
        HorizontalScrollView hs=new HorizontalScrollView(this);hs.setHorizontalScrollBarEnabled(false);hs.setFocusable(false);LinearLayout rail=Ui.horizontal(this,0);
        for(int i=0;i<albums.length();i++){JSONObject album=albums.optJSONObject(i);if(album==null)continue;LinearLayout card=Ui.vertical(this,7);card.setFocusable(true);card.setClickable(true);card.setBackground(Ui.round(Color.TRANSPARENT,12));card.setOnFocusChangeListener((v,f)->RemoteUi.cardFocus(v,f));card.addView(squareCover(album.optString("cover_url",""),Ui.dp(this,170)));TextView name=Ui.title(this,album.optString("title","Álbum"),12);name.setMaxLines(1);card.addView(name,new LinearLayout.LayoutParams(Ui.dp(this,170),ViewGroup.LayoutParams.WRAP_CONTENT));TextView artist=Ui.muted(this,album.optString("artist",""),10);artist.setMaxLines(1);card.addView(artist,new LinearLayout.LayoutParams(Ui.dp(this,170),ViewGroup.LayoutParams.WRAP_CONTENT));long id=album.optLong("representative_track_id");String albumName=album.optString("title","");String albumArtist=album.optString("artist","");card.setOnClickListener(v->{List<JSONObject>albumQueue=tracksForAlbum(albumName,albumArtist);JSONObject first=findTrack(id);if(first==null&&!albumQueue.isEmpty())first=albumQueue.get(0);if(first!=null)playTrack(first,albumQueue);});rail.addView(card,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,0,12,0));}
        hs.addView(rail);content.addView(hs);
    }

    private void addArtistRail(String title) {
        if(allTracks.isEmpty())return;LinkedHashMap<String,Integer>artists=new LinkedHashMap<>();for(JSONObject track:allTracks){String artist=track.optString("album_artist",track.optString("artist","Artista desconhecido")).trim();if(!artist.isEmpty())artists.put(artist,artists.getOrDefault(artist,0)+1);if(artists.size()>=30)break;}if(artists.isEmpty())return;
        content.addView(Ui.title(this,title,21),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,25,0,10));HorizontalScrollView hs=new HorizontalScrollView(this);hs.setHorizontalScrollBarEnabled(false);hs.setFocusable(false);LinearLayout rail=Ui.horizontal(this,0);
        for(String artist:artists.keySet()){Button button=Ui.button(this,artist,false);button.setOnClickListener(v->{List<JSONObject>list=tracksForArtist(artist);if(!list.isEmpty())playTrack(list.get(0),list);});rail.addView(button,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,Ui.dp(this,46),0,0,8,0));}hs.addView(rail);content.addView(hs);
    }

    private void addTrackList(String title, JSONArray tracks, int max) {
        List<JSONObject> list=jsonList(tracks);if(list.isEmpty())return;content.addView(Ui.title(this,title,21),Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,25,0,10));LinearLayout host=Ui.vertical(this,0);
        for(int i=0;i<Math.min(max,list.size());i++)host.addView(trackRow(list.get(i),list,i));content.addView(host);
    }

    private View trackRow(JSONObject track,List<JSONObject>source,int index){LinearLayout row=Ui.horizontal(this,10);row.setGravity(Gravity.CENTER_VERTICAL);row.setFocusable(true);row.setClickable(true);row.setPadding(Ui.dp(this,12),Ui.dp(this,8),Ui.dp(this,12),Ui.dp(this,8));row.setBackground(Ui.round(Color.rgb(18,23,31),10));row.setOnFocusChangeListener((v,f)->RemoteUi.cardFocus(v,f));TextView n=Ui.muted(this,String.valueOf(track.optInt("track_number",index+1)),11);n.setGravity(Gravity.CENTER);row.addView(n,new LinearLayout.LayoutParams(Ui.dp(this,30),Ui.dp(this,48)));row.addView(squareCover(track.optString("cover_url",""),Ui.dp(this,48)));LinearLayout copy=Ui.vertical(this,0);TextView t=Ui.title(this,track.optString("title","Faixa"),12);t.setMaxLines(1);copy.addView(t);TextView a=Ui.muted(this,track.optString("artist","Artista desconhecido")+" · "+track.optString("album","Singles"),10);a.setMaxLines(1);copy.addView(a);row.addView(copy,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));row.addView(Ui.muted(this,clock((long)track.optDouble("duration_seconds",0)),10));TextView play=Ui.title(this,"▶",14);play.setGravity(Gravity.CENTER);row.addView(play,new LinearLayout.LayoutParams(Ui.dp(this,42),Ui.dp(this,42)));row.setOnClickListener(v->playTrack(track,source));LinearLayout holder=new LinearLayout(this);holder.addView(row,Ui.margin(this,ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,0,0,6));return holder;}

    private View squareCover(String url,int size){FrameLayout box=new FrameLayout(this);box.setBackground(Ui.round(Color.rgb(27,32,42),10));TextView note=Ui.title(this,"♪",size>=Ui.dp(this,100)?28:16);note.setGravity(Gravity.CENTER);box.addView(note,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));ImageView image=new ImageView(this);image.setScaleType(ImageView.ScaleType.CENTER_CROP);box.addView(image,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));if(url!=null&&!url.isEmpty())images.load(image,url);box.setLayoutParams(new LinearLayout.LayoutParams(size,size));return box;}

    private List<JSONObject> jsonList(JSONArray array){List<JSONObject>out=new ArrayList<>();if(array==null)return out;for(int i=0;i<array.length();i++){JSONObject o=array.optJSONObject(i);if(o!=null)out.add(o);}return out;}
    private JSONObject findTrack(long id){for(JSONObject t:allTracks)if(t.optLong("id")==id)return t;return null;}
    private List<JSONObject> tracksForAlbum(String album,String artist){List<JSONObject>out=new ArrayList<>();for(JSONObject t:allTracks){String ta=t.optString("album","");String ar=t.optString("album_artist",t.optString("artist",""));if(ta.equalsIgnoreCase(album)&&(artist.isEmpty()||ar.equalsIgnoreCase(artist)))out.add(t);}return out;}
    private List<JSONObject> tracksForArtist(String artist){List<JSONObject>out=new ArrayList<>();for(JSONObject t:allTracks){String ar=t.optString("album_artist",t.optString("artist",""));if(ar.equalsIgnoreCase(artist)||t.optString("artist","").equalsIgnoreCase(artist))out.add(t);}return out;}

    private void playTrack(JSONObject track,List<JSONObject>source){long id=track.optLong("id");if(id<=0)return;long previous=currentTrack==null?0:currentTrack.optLong("id");if(previous>0&&previous!=id){reportPlayback();finishPlayback(previous);}currentTrack=track;queue.clear();if(source!=null&&!source.isEmpty())queue.addAll(source);else queue.addAll(allTracks);queueIndex=-1;for(int i=0;i<queue.size();i++)if(queue.get(i).optLong("id")==id){queueIndex=i;break;}if(queueIndex<0){queue.clear();queue.add(track);queueIndex=0;}syncNowPlaying();player.setMediaItem(new MediaItem.Builder().setUri(Uri.parse(api.apiUrl("/music/tracks/"+id+"/stream"))).setMediaId(String.valueOf(id)).build());player.prepare();player.play();lastReportedPosition=0;lastMonitoringAt=0;reportListening(0,true,false);reportPlayback();}
    private void toggle(){if(currentTrack==null)return;if(player.isPlaying())player.pause();else player.play();}
    private void step(int direction){if(queue.isEmpty())return;int next=queueIndex+direction;if(next<0)next=queue.size()-1;if(next>=queue.size())next=0;playTrack(queue.get(next),new ArrayList<>(queue));}

    private void syncNowPlaying(){if(currentTrack==null)return;nowTitle.setText(currentTrack.optString("title","Faixa"));nowArtist.setText(currentTrack.optString("artist",currentTrack.optString("album_artist","")));nowCover.setImageDrawable(null);String url=currentTrack.optString("cover_url","");if(!url.isEmpty())images.load(nowCover,url);}

    private void showQueue(){if(queue.isEmpty()){Toast.makeText(this,"A fila está vazia.",Toast.LENGTH_SHORT).show();return;}String[]labels=new String[queue.size()];for(int i=0;i<queue.size();i++){JSONObject t=queue.get(i);labels[i]=(i==queueIndex?"▶  ":"")+t.optString("title","Faixa")+" · "+t.optString("artist","");}new AlertDialog.Builder(this).setTitle("Fila de reprodução").setItems(labels,(d,which)->playTrack(queue.get(which),new ArrayList<>(queue))).setNegativeButton("Fechar",null).show();}

    private void reportListening(double delta,boolean started,boolean completed){JSONObject track=currentTrack;if(track==null)return;long id=track.optLong("id");if(id<=0)return;io.submit(()->{try{api.post("/music/tracks/"+id+"/listening",new JSONObject().put("delta_seconds",delta).put("started",started).put("completed",completed));}catch(Exception ignored){}});}
    private void reportPlayback(){JSONObject track=currentTrack;if(track==null||player==null)return;long id=track.optLong("id");if(id<=0)return;long position=Math.max(0,player.getCurrentPosition()),duration=Math.max(0,player.getDuration());String state=player.isPlaying()?"playing":"paused",codec=track.optString("codec","");int bitrate=Math.max(0,track.optInt("bitrate",0)/1000);io.submit(()->{try{JSONObject body=new JSONObject().put("position_seconds",position/1000.0).put("duration_seconds",duration>0?duration/1000.0:track.optDouble("duration_seconds",0)).put("state",state).put("mode","music").put("audio_codec",codec).put("source_audio_codec",codec).put("bitrate_kbps",bitrate);api.post("/media/"+id+"/playback",body);}catch(Exception ignored){}});}
    private void finishPlayback(long id){if(id<=0)return;io.submit(()->{try{api.delete("/media/"+id+"/playback");}catch(Exception ignored){}});}

    private String clock(long seconds){seconds=Math.max(0,seconds);long m=seconds/60,s=seconds%60;return m+":"+String.format(java.util.Locale.US,"%02d",s);}

    @Override public boolean dispatchKeyEvent(KeyEvent event){if(event.getAction()!=KeyEvent.ACTION_DOWN)return super.dispatchKeyEvent(event);int key=event.getKeyCode();if(key==KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE||key==KeyEvent.KEYCODE_HEADSETHOOK){toggle();return true;}if(key==KeyEvent.KEYCODE_MEDIA_PLAY){if(currentTrack!=null)player.play();return true;}if(key==KeyEvent.KEYCODE_MEDIA_PAUSE||key==KeyEvent.KEYCODE_MEDIA_STOP){player.pause();reportPlayback();return true;}if(key==KeyEvent.KEYCODE_MEDIA_NEXT){step(1);return true;}if(key==KeyEvent.KEYCODE_MEDIA_PREVIOUS){step(-1);return true;}return super.dispatchKeyEvent(event);}
    @Override protected void onPause(){reportPlayback();super.onPause();}
    @Override protected void onStop(){reportPlayback();super.onStop();}
    @Override protected void onDestroy(){main.removeCallbacks(progressTick);long id=currentTrack==null?0:currentTrack.optLong("id");if(id>0){reportPlayback();finishPlayback(id);}if(player!=null){player.release();player=null;}io.shutdown();super.onDestroy();}
}
