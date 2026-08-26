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
import android.widget.GridLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class SeriesBrowseActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private String filter = "series";

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        api = new ApiClient(this);
        images = new ImageLoader(this);
        filter = getIntent().getStringExtra("filter_kind");
        if (filter == null || filter.isEmpty()) filter = "series";

        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(Ui.BG);
        scroll.setFillViewport(true);
        content = Ui.vertical(this, RemoteUi.isTelevision(this) ? 28 : 18);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        setContentView(scroll);
        loading();
        load();
    }

    private void loading() {
        Ui.clear(content);
        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), 0, 0, 0, 18));
        content.addView(Ui.title(this, filter.equals("anime") ? "Animes" : "Séries", 32));
        content.addView(Ui.muted(this, "Carregando temporadas e episódios…", 13));
    }

    private void load() {
        io.submit(() -> {
            try {
                JSONArray array = new JSONArray(api.get("/series"));
                List<JSONObject> list = new ArrayList<>();
                for (int i = 0; i < array.length(); i++) {
                    JSONObject item = array.optJSONObject(i);
                    if (item == null) continue;
                    String type = item.optString("media_type", "series");
                    if (filter.equals("anime") && !"anime".equalsIgnoreCase(type)) continue;
                    if (filter.equals("series") && "anime".equalsIgnoreCase(type)) continue;
                    list.add(item);
                }
                main.post(() -> render(list));
            } catch (Exception e) {
                main.post(() -> Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show());
            }
        });
    }

    private void render(List<JSONObject> series) {
        Ui.clear(content);
        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), 0, 0, 0, 16));
        content.addView(Ui.title(this, filter.equals("anime") ? "Animes" : "Séries", 32));
        content.addView(Ui.muted(this, series.size() + " títulos", 12), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 4, 0, 18));

        if (series.isEmpty()) {
            content.addView(Ui.muted(this, "Nenhum título encontrado.", 14));
            RemoteUi.focusFirst(content);
            return;
        }

        GridLayout grid = new GridLayout(this);
        int widthDp = (int) (getResources().getDisplayMetrics().widthPixels / getResources().getDisplayMetrics().density);
        int columns = RemoteUi.isTelevision(this) ? Math.max(4, Math.min(7, widthDp / 220)) : (widthDp >= 700 ? 4 : 2);
        grid.setColumnCount(columns);
        grid.setAlignmentMode(GridLayout.ALIGN_BOUNDS);
        grid.setUseDefaultMargins(false);
        for (JSONObject item : series) grid.addView(card(item));
        content.addView(grid);
        RemoteUi.focusFirst(content);
    }

    private View card(JSONObject item) {
        LinearLayout card = Ui.vertical(this, 6);
        card.setFocusable(true);
        card.setClickable(true);
        card.setGravity(Gravity.START);
        card.setBackground(Ui.round(Color.TRANSPARENT, 12));
        card.setOnFocusChangeListener((v, focused) -> RemoteUi.cardFocus(v, focused));

        int posterW = Ui.dp(this, RemoteUi.isTelevision(this) ? 190 : 150);
        int posterH = posterW * 3 / 2;
        ImageView poster = new ImageView(this);
        poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
        poster.setBackground(Ui.round(Color.rgb(24, 29, 38), 10));
        card.addView(poster, new LinearLayout.LayoutParams(posterW, posterH));
        String posterUrl = item.optString("poster_url", "");
        if (!posterUrl.isEmpty()) images.load(poster, posterUrl);

        TextView title = Ui.title(this, item.optString("title", "Série"), 13);
        title.setMaxLines(1);
        card.addView(title, new LinearLayout.LayoutParams(posterW, ViewGroup.LayoutParams.WRAP_CONTENT));
        String meta = item.optInt("season_count", 0) + " temp. · " + item.optInt("episode_count", 0) + " ep.";
        card.addView(Ui.muted(this, meta, 10), new LinearLayout.LayoutParams(posterW, ViewGroup.LayoutParams.WRAP_CONTENT));

        String id = item.optString("id", "");
        card.setOnClickListener(v -> {
            Intent intent = new Intent(this, SeriesDetailActivity.class);
            intent.putExtra("series_id", id);
            startActivity(intent);
        });

        GridLayout.LayoutParams gp = new GridLayout.LayoutParams();
        gp.width = posterW + Ui.dp(this, 18);
        gp.height = ViewGroup.LayoutParams.WRAP_CONTENT;
        gp.setMargins(Ui.dp(this, 5), Ui.dp(this, 5), Ui.dp(this, 8), Ui.dp(this, 16));
        card.setLayoutParams(gp);
        return card;
    }

    @Override protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }
}
