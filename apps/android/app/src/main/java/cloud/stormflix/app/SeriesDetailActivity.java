package cloud.stormflix.app;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.HorizontalScrollView;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class SeriesDetailActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private LinearLayout episodesHost;
    private JSONObject detail;
    private String seriesId;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        seriesId = getIntent().getStringExtra("series_id");
        if (seriesId == null || seriesId.trim().isEmpty()) { finish(); return; }
        api = new ApiClient(this);
        images = new ImageLoader(this);
        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(Ui.BG);
        scroll.setFillViewport(true);
        content = Ui.vertical(this, RemoteUi.isTelevision(this) ? 28 : 18);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        setContentView(scroll);
        content.addView(Ui.muted(this, "Carregando série…", 14));
        load();
    }

    private void load() {
        io.submit(() -> {
            try {
                JSONObject data = new JSONObject(api.get("/series/" + seriesId));
                main.post(() -> render(data));
            } catch (Exception e) {
                main.post(() -> { Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show(); finish(); });
            }
        });
    }

    private void render(JSONObject data) {
        detail = data;
        Ui.clear(content);
        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this,44), 0,0,0,14));

        LinearLayout hero = Ui.horizontal(this, 0);
        ImageView poster = new ImageView(this);
        int posterW = Ui.dp(this, RemoteUi.isTelevision(this) ? 220 : 145);
        poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
        poster.setBackground(Ui.round(Color.rgb(23,28,37),12));
        hero.addView(poster, new LinearLayout.LayoutParams(posterW, posterW * 3 / 2));
        String posterUrl = data.optString("poster_url", ""); if (!posterUrl.isEmpty()) images.load(poster, posterUrl);

        LinearLayout copy = Ui.vertical(this, 18);
        copy.setGravity(Gravity.CENTER_VERTICAL);
        copy.addView(Ui.title(this, data.optString("title", "Série"), RemoteUi.isTelevision(this) ? 35 : 28));
        String facts = data.optInt("season_count",0) + " temporadas · " + data.optInt("episode_count",0) + " episódios";
        copy.addView(Ui.muted(this, facts, 13), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,7,0,0));
        String overview = data.optString("overview", "");
        if (!overview.isEmpty()) {
            TextView overviewView = Ui.muted(this, overview, 14); overviewView.setMaxLines(5); overviewView.setLineSpacing(0,1.15f);
            copy.addView(overviewView, Ui.margin(this, RemoteUi.isTelevision(this)?Ui.dp(this,750):ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,12,0,0));
        }
        JSONArray seasons = data.optJSONArray("seasons");
        JSONObject firstEpisode = firstEpisode(seasons);
        if (firstEpisode != null) {
            Button play = Ui.button(this, "▶ Reproduzir primeiro episódio", true);
            play.setOnClickListener(v -> playEpisode(firstEpisode));
            copy.addView(play, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this,48),0,16,0,0));
        }
        hero.addView(copy, new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1));
        content.addView(hero);

        content.addView(Ui.title(this, "Temporadas", 22), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,26,0,10));
        HorizontalScrollView seasonScroll = new HorizontalScrollView(this); seasonScroll.setHorizontalScrollBarEnabled(false); seasonScroll.setFocusable(false);
        LinearLayout seasonRail = Ui.horizontal(this,0);
        if (seasons != null) for (int i=0;i<seasons.length();i++) {
            JSONObject season = seasons.optJSONObject(i); if (season == null) continue;
            Button button = Ui.button(this, season.optString("title", "Temporada " + season.optInt("number", i+1)), i==0);
            final int index=i;
            button.setOnClickListener(v -> renderEpisodes(index));
            seasonRail.addView(button, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this,44),0,0,8,0));
        }
        seasonScroll.addView(seasonRail); content.addView(seasonScroll);

        episodesHost = Ui.vertical(this,0);
        content.addView(episodesHost, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,14,0,30));
        renderEpisodes(0);
        RemoteUi.focusFirst(content);
    }

    private JSONObject firstEpisode(JSONArray seasons) {
        if (seasons == null) return null;
        for (int i=0;i<seasons.length();i++) {
            JSONArray episodes = seasons.optJSONObject(i) == null ? null : seasons.optJSONObject(i).optJSONArray("episodes");
            if (episodes != null && episodes.length() > 0) return episodes.optJSONObject(0);
        }
        return null;
    }

    private void renderEpisodes(int seasonIndex) {
        if (episodesHost == null || detail == null) return;
        Ui.clear(episodesHost);
        JSONArray seasons = detail.optJSONArray("seasons");
        if (seasons == null || seasonIndex < 0 || seasonIndex >= seasons.length()) return;
        JSONObject season = seasons.optJSONObject(seasonIndex); if (season == null) return;
        episodesHost.addView(Ui.title(this, season.optString("title", "Episódios"), 20), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,0,0,9));
        JSONArray episodes = season.optJSONArray("episodes");
        if (episodes == null || episodes.length() == 0) { episodesHost.addView(Ui.muted(this,"Nenhum episódio.",13)); return; }
        for (int i=0;i<episodes.length();i++) {
            JSONObject episode=episodes.optJSONObject(i); if(episode==null)continue;
            episodesHost.addView(episodeRow(episode));
        }
    }

    private View episodeRow(JSONObject episode) {
        LinearLayout row = Ui.horizontal(this, 12);
        row.setFocusable(true); row.setClickable(true); row.setBackground(Ui.round(Color.rgb(17,21,29),11));
        row.setOnFocusChangeListener((v,f)->RemoteUi.cardFocus(v,f));

        TextView number = Ui.title(this, String.format("S%02d\nE%02d", episode.optInt("season_number",1), episode.optInt("episode_number",1)), 12);
        number.setGravity(Gravity.CENTER); row.addView(number, new LinearLayout.LayoutParams(Ui.dp(this,58),Ui.dp(this,62)));
        LinearLayout copy=Ui.vertical(this,0);
        TextView title=Ui.title(this,episode.optString("title","Episódio"),13);title.setMaxLines(1);copy.addView(title);
        String meta=episode.optInt("runtime_minutes",0)>0?episode.optInt("runtime_minutes")+" min":"Toque para reproduzir";
        copy.addView(Ui.muted(this,meta,10));
        row.addView(copy,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));
        TextView play=Ui.title(this,"▶",16);play.setGravity(Gravity.CENTER);row.addView(play,new LinearLayout.LayoutParams(Ui.dp(this,46),Ui.dp(this,46)));
        row.setOnClickListener(v->playEpisode(episode));
        return withMargin(row,0,0,0,7);
    }

    private View withMargin(View v,int l,int t,int r,int b){LinearLayout holder=new LinearLayout(this);holder.addView(v,Ui.margin(this,ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.WRAP_CONTENT,l,t,r,b));return holder;}

    private void playEpisode(JSONObject episode) {
        long id=episode.optLong("id");if(id<=0)return;
        Intent intent=new Intent(this,PlayerActivity.class);
        intent.putExtra("media_id",id);
        intent.putExtra("title",episode.optString("title","Episódio"));
        startActivity(intent);
    }

    @Override protected void onDestroy(){io.shutdownNow();super.onDestroy();}
}
