package cloud.stormflix.app;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
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
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Cinematic series details optimized for touch and ten-foot/D-pad use. */
public class SeriesDetailActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private LinearLayout episodesHost;
    private JSONObject detail;
    private String seriesId;
    private boolean television;
    private int selectedSeason;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        seriesId = getIntent().getStringExtra("series_id");
        if (seriesId == null || seriesId.trim().isEmpty()) { finish(); return; }
        api = new ApiClient(this);
        images = new ImageLoader(this);
        television = RemoteUi.isTelevision(this);

        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(Ui.BG);
        scroll.setFillViewport(true);
        scroll.setClipToPadding(false);
        content = Ui.vertical(this, 0);
        scroll.addView(content, new ScrollView.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        setContentView(scroll);

        TextView loading = Ui.muted(this, "Carregando série…", 14);
        loading.setGravity(Gravity.CENTER);
        content.addView(loading, new LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 220)));
        load();
    }

    private void load() {
        io.submit(() -> {
            try {
                JSONObject data = new JSONObject(api.get("/series/" + seriesId));
                main.post(() -> render(data));
            } catch (Exception e) {
                main.post(() -> {
                    Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show();
                    finish();
                });
            }
        });
    }

    private void render(JSONObject data) {
        detail = data;
        Ui.clear(content);
        content.addView(hero(data));

        LinearLayout body = Ui.vertical(this, television ? 42 : 22);
        body.addView(Ui.title(this, "Temporadas", television ? 24 : 21));

        HorizontalScrollView seasonScroll = new HorizontalScrollView(this);
        seasonScroll.setHorizontalScrollBarEnabled(false);
        seasonScroll.setFocusable(false);
        LinearLayout seasonRail = Ui.horizontal(this, 0);
        JSONArray seasons = data.optJSONArray("seasons");
        if (seasons != null) {
            for (int i = 0; i < seasons.length(); i++) {
                JSONObject season = seasons.optJSONObject(i);
                if (season == null) continue;
                final int index = i;
                Button button = Ui.button(this,
                    season.optString("title", "Temporada " + season.optInt("number", i + 1)),
                    i == selectedSeason);
                button.setOnClickListener(v -> {
                    selectedSeason = index;
                    renderEpisodes(index);
                    refreshSeasonButtons(seasonRail, index);
                });
                seasonRail.addView(button, Ui.margin(this,
                    ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), 0, 8, 8, 0));
            }
        }
        seasonScroll.addView(seasonRail);
        body.addView(seasonScroll);

        episodesHost = Ui.vertical(this, 0);
        body.addView(episodesHost, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 16, 0, 48));
        content.addView(body);
        renderEpisodes(Math.min(selectedSeason, Math.max(0, (seasons == null ? 1 : seasons.length()) - 1)));
        RemoteUi.focusFirst(content);
    }

    private View hero(JSONObject data) {
        int height = Ui.dp(this, television ? 470 : 400);
        FrameLayout hero = new FrameLayout(this);
        hero.setBackgroundColor(Color.rgb(12, 14, 19));

        ImageView backdrop = new ImageView(this);
        backdrop.setScaleType(ImageView.ScaleType.CENTER_CROP);
        backdrop.setBackgroundColor(Color.rgb(20, 23, 30));
        hero.addView(backdrop, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, height));
        String backdropUrl = data.optString("backdrop_url", "");
        if (!backdropUrl.isEmpty()) images.load(backdrop, backdropUrl);

        View leftShade = new View(this);
        leftShade.setBackground(new GradientDrawable(
            GradientDrawable.Orientation.LEFT_RIGHT,
            new int[]{Color.argb(245, 5, 6, 9), Color.argb(205, 5, 6, 9),
                Color.argb(65, 5, 6, 9), Color.TRANSPARENT}));
        hero.addView(leftShade, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, height));

        View bottomShade = new View(this);
        bottomShade.setBackground(new GradientDrawable(
            GradientDrawable.Orientation.TOP_BOTTOM,
            new int[]{Color.TRANSPARENT, Color.argb(60, 5, 6, 9), Color.argb(245, 5, 6, 9)}));
        hero.addView(bottomShade, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, height));

        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        FrameLayout.LayoutParams backLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), Gravity.START | Gravity.TOP);
        backLp.setMargins(Ui.dp(this, television ? 40 : 20), Ui.dp(this, 20), 0, 0);
        hero.addView(back, backLp);

        LinearLayout info = Ui.vertical(this, 0);
        int side = television ? 48 : 24;
        info.setPadding(Ui.dp(this, side), 0, Ui.dp(this, side), Ui.dp(this, television ? 34 : 26));

        String logoUrl = data.optString("logo_url", "");
        if (!logoUrl.isEmpty()) {
            ImageView logo = new ImageView(this);
            logo.setAdjustViewBounds(true);
            logo.setScaleType(ImageView.ScaleType.CENTER_INSIDE);
            info.addView(logo, new LinearLayout.LayoutParams(
                (television ? Ui.dp(this, 430) : ViewGroup.LayoutParams.MATCH_PARENT), Ui.dp(this, television ? 120 : 90)));
            images.load(logo, logoUrl);
        } else {
            TextView title = Ui.title(this, data.optString("title", "Série"), television ? 42 : 33);
            title.setMaxLines(2);
            title.setShadowLayer(8, 0, 3, Color.BLACK);
            info.addView(title);
        }

        String facts = data.optInt("season_count", 0) + " temporadas  ·  "
            + data.optInt("episode_count", 0) + " episódios";
        if (data.optInt("year", 0) > 0) facts = data.optInt("year") + "  ·  " + facts;
        if (data.optDouble("rating", 0) > 0) {
            facts += "  ·  ★ " + String.format(Locale.US, "%.1f", data.optDouble("rating"));
        }
        TextView meta = Ui.title(this, facts, 13);
        meta.setTextColor(Color.rgb(224, 229, 237));
        info.addView(meta, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 12, 0, 0));

        String overview = data.optString("overview", "");
        if (!overview.isEmpty()) {
            TextView overviewView = Ui.muted(this, overview, television ? 15 : 14);
            overviewView.setTextColor(Color.rgb(208, 214, 224));
            overviewView.setMaxLines(television ? 4 : 3);
            overviewView.setLineSpacing(0, 1.15f);
            info.addView(overviewView, Ui.margin(this, (television ? Ui.dp(this, 820) : ViewGroup.LayoutParams.MATCH_PARENT),
                ViewGroup.LayoutParams.WRAP_CONTENT, 0, 12, 0, 0));
        }

        JSONObject first = firstUnwatchedOrFirst(data.optJSONArray("seasons"));
        if (first != null) {
            Button play = Ui.button(this, "▶  Reproduzir", true);
            play.setTextSize(television ? 16 : 14);
            play.setOnClickListener(v -> playEpisode(first));
            info.addView(play, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
                Ui.dp(this, television ? 54 : 50), 0, 18, 0, 0));
        }

        FrameLayout.LayoutParams infoLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT,
            Gravity.START | Gravity.BOTTOM);
        hero.addView(info, infoLp);
        hero.setLayoutParams(new LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, height));
        return hero;
    }

    private JSONObject firstUnwatchedOrFirst(JSONArray seasons) {
        if (seasons == null) return null;
        JSONObject first = null;
        for (int i = 0; i < seasons.length(); i++) {
            JSONObject season = seasons.optJSONObject(i);
            JSONArray episodes = season == null ? null : season.optJSONArray("episodes");
            if (episodes == null) continue;
            for (int j = 0; j < episodes.length(); j++) {
                JSONObject episode = episodes.optJSONObject(j);
                if (episode == null) continue;
                if (first == null) first = episode;
                double duration = episode.optDouble("duration_seconds", 0);
                double position = episode.optDouble("position_seconds", 0);
                if (position > 0 && duration > 0 && position / duration < .92) return episode;
            }
        }
        return first;
    }

    private void refreshSeasonButtons(LinearLayout rail, int selected) {
        for (int i = 0; i < rail.getChildCount(); i++) {
            View child = rail.getChildAt(i);
            child.setAlpha(i == selected ? 1f : .72f);
        }
    }

    private void renderEpisodes(int seasonIndex) {
        if (episodesHost == null || detail == null) return;
        Ui.clear(episodesHost);
        JSONArray seasons = detail.optJSONArray("seasons");
        if (seasons == null || seasonIndex < 0 || seasonIndex >= seasons.length()) return;
        JSONObject season = seasons.optJSONObject(seasonIndex);
        if (season == null) return;

        JSONArray episodes = season.optJSONArray("episodes");
        if (episodes == null || episodes.length() == 0) {
            episodesHost.addView(Ui.muted(this, "Nenhum episódio.", 13));
            return;
        }
        for (int i = 0; i < episodes.length(); i++) {
            JSONObject episode = episodes.optJSONObject(i);
            if (episode != null) episodesHost.addView(episodeRow(episode));
        }
    }

    private View episodeRow(JSONObject episode) {
        LinearLayout row = Ui.horizontal(this, television ? 14 : 10);
        row.setFocusable(true);
        row.setClickable(true);
        row.setBackground(Ui.round(Color.rgb(16, 20, 28), 12));
        row.setOnFocusChangeListener((v, focused) -> RemoteUi.cardFocus(v, focused));

        String still = episode.optString("backdrop_url", "");
        if (still.isEmpty()) still = episode.optString("poster_url", "");
        if (!still.isEmpty()) {
            ImageView image = new ImageView(this);
            image.setScaleType(ImageView.ScaleType.CENTER_CROP);
            image.setBackgroundColor(Color.rgb(27, 31, 40));
            int w = Ui.dp(this, television ? 180 : 118);
            int h = Ui.dp(this, television ? 102 : 68);
            row.addView(image, new LinearLayout.LayoutParams(w, h));
            images.load(image, still);
        }

        TextView number = Ui.title(this,
            String.format(Locale.US, "E%02d", episode.optInt("episode_number", 1)), 12);
        number.setGravity(Gravity.CENTER);
        row.addView(number, new LinearLayout.LayoutParams(Ui.dp(this, 52), Ui.dp(this, 54)));

        LinearLayout copy = Ui.vertical(this, 0);
        TextView title = Ui.title(this, episode.optString("title", "Episódio"), television ? 14 : 13);
        title.setMaxLines(1);
        copy.addView(title);
        String runtime = episode.optInt("runtime_minutes", 0) > 0
            ? episode.optInt("runtime_minutes") + " min" : "Reproduzir";
        copy.addView(Ui.muted(this, runtime, 10));
        row.addView(copy, new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1));

        TextView play = Ui.title(this, "▶", 16);
        play.setGravity(Gravity.CENTER);
        row.addView(play, new LinearLayout.LayoutParams(Ui.dp(this, 46), Ui.dp(this, 46)));
        row.setOnClickListener(v -> playEpisode(episode));

        LinearLayout holder = new LinearLayout(this);
        holder.addView(row, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 0, 0, 8));
        return holder;
    }

    private void playEpisode(JSONObject episode) {
        long id = episode.optLong("id");
        if (id <= 0) return;
        Intent intent = new Intent(this, NativePlayerActivity.class);
        intent.putExtra("media_id", id);
        intent.putExtra("title", episode.optString("title", "Episódio"));
        startActivity(intent);
    }

    @Override protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }
}
