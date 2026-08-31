package cloud.stormflix.app;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
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

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Generic two-level Home menu browser. Root categories are menu buttons and
 * direct children are gallery rails, matching the StormFlix Web category model.
 * The native app consumes only /api/v1 and never reimplements catalog rules:
 * smart child membership is resolved by the server.
 */
public class CategoryBrowseActivity extends Activity {
    private static final class CardItem {
        final JSONObject raw;
        final boolean series;
        final String key;
        CardItem(JSONObject raw, boolean series, String key) {
            this.raw = raw;
            this.series = series;
            this.key = key;
        }
    }

    private static final class Section {
        final String title;
        final List<CardItem> items;
        Section(String title, List<CardItem> items) {
            this.title = title;
            this.items = items;
        }
    }

    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private String slug = "";
    private String requestedTitle = "";

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        api = new ApiClient(this);
        images = new ImageLoader(this);
        slug = getIntent().getStringExtra("category_slug");
        requestedTitle = getIntent().getStringExtra("category_title");
        if (slug == null) slug = "";
        if (requestedTitle == null) requestedTitle = "";

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(Ui.BG);
        content = Ui.vertical(this, RemoteUi.isTelevision(this) ? 24 : 18);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        setContentView(scroll);
        loading();
        load();
    }

    private void loading() {
        Ui.clear(content);
        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), 0, 0, 0, 16));
        content.addView(Ui.title(this, requestedTitle.isEmpty() ? "Catálogo" : requestedTitle, 32));
        content.addView(Ui.muted(this, "Carregando seções…", 13));
    }

    private void load() {
        if (slug.trim().isEmpty()) {
            Toast.makeText(this, "Categoria inválida", Toast.LENGTH_LONG).show();
            finish();
            return;
        }
        io.submit(() -> {
            try {
                JSONObject rootData = new JSONObject(api.get("/categories/" + Uri.encode(slug)));
                JSONObject root = rootData.optJSONObject("category");
                long rootId = root == null ? 0 : root.optLong("id");
                String rootTitle = root == null ? requestedTitle : root.optString("name", requestedTitle);
                String catalogCaps = PlaybackCapabilities.catalogQuery(this);

                JSONArray categories = new JSONArray(api.get("/categories"));
                List<JSONObject> children = new ArrayList<>();
                for (int i = 0; i < categories.length(); i++) {
                    JSONObject category = categories.optJSONObject(i);
                    if (category == null || category.isNull("parent_id")) continue;
                    if (category.optLong("parent_id") == rootId) children.add(category);
                }
                children.sort((a, b) -> {
                    int order = Integer.compare(a.optInt("sort_order"), b.optInt("sort_order"));
                    return order != 0 ? order : Long.compare(a.optLong("id"), b.optLong("id"));
                });

                List<Section> sections = new ArrayList<>();
                LinkedHashMap<String, CardItem> aggregate = new LinkedHashMap<>();
                for (CardItem item : categoryItems(rootData)) aggregate.put(item.key, item);

                for (JSONObject child : children) {
                    String childSlug = child.optString("slug", "");
                    if (childSlug.isEmpty()) continue;
                    JSONObject data = new JSONObject(api.get("/categories/" + Uri.encode(childSlug) + "/smart" + catalogCaps));
                    List<CardItem> items = categoryItems(data);
                    if (items.isEmpty()) continue;
                    sections.add(new Section(child.optString("name", childSlug), items));
                    for (CardItem item : items) aggregate.putIfAbsent(item.key, item);
                }

                List<CardItem> general = new ArrayList<>(aggregate.values());
                main.post(() -> render(rootTitle, general, sections));
            } catch (Exception e) {
                main.post(() -> error(e));
            }
        });
    }

    private List<CardItem> categoryItems(JSONObject data) {
        List<CardItem> out = new ArrayList<>();
        Map<String, Boolean> seen = new LinkedHashMap<>();
        JSONArray series = data.optJSONArray("series");
        if (series != null) {
            for (int i = 0; i < series.length(); i++) {
                JSONObject item = series.optJSONObject(i);
                if (item == null) continue;
                String id = item.optString("id", "");
                if (id.isEmpty()) continue;
                String key = "s:" + id;
                if (seen.put(key, true) == null) out.add(new CardItem(item, true, key));
            }
        }
        JSONArray media = data.optJSONArray("media");
        if (media != null) {
            for (int i = 0; i < media.length(); i++) {
                JSONObject item = media.optJSONObject(i);
                if (item == null) continue;
                String seriesId = item.optString("series_id", "");
                boolean logicalSeries = "series".equalsIgnoreCase(item.optString("entity_type", "")) && !seriesId.isEmpty();
                String key = logicalSeries ? "s:" + seriesId : "m:" + item.optLong("id");
                if (seen.put(key, true) == null) out.add(new CardItem(item, logicalSeries, key));
            }
        }
        return out;
    }

    private void render(String title, List<CardItem> general, List<Section> sections) {
        Ui.clear(content);
        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), 0, 0, 0, 10));
        content.addView(Ui.title(this, title, 32));
        content.addView(Ui.muted(this, general.size() + " títulos", 12), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 4, 0, 8));

        if (!general.isEmpty()) content.addView(row(title, general));
        for (Section section : sections) content.addView(row(section.title, section.items));
        if (general.isEmpty() && sections.isEmpty()) content.addView(Ui.muted(this, "Este menu ainda não possui títulos.", 14));
        RemoteUi.focusFirst(content);
    }

    private View row(String title, List<CardItem> items) {
        LinearLayout section = Ui.vertical(this, 0);
        section.addView(Ui.title(this, title, 21), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 20, 0, 10));
        HorizontalScrollView scroll = new HorizontalScrollView(this);
        scroll.setHorizontalScrollBarEnabled(false);
        scroll.setFocusable(false);
        LinearLayout rail = Ui.horizontal(this, 0);
        for (CardItem item : items) rail.addView(card(item));
        scroll.addView(rail);
        section.addView(scroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 292)));
        return section;
    }

    private View card(CardItem item) {
        JSONObject raw = item.raw;
        LinearLayout card = Ui.vertical(this, 0);
        card.setFocusable(true);
        card.setClickable(true);
        card.setBackground(Ui.round(Color.TRANSPARENT, 10));
        card.setOnFocusChangeListener((v, focused) -> RemoteUi.cardFocus(v, focused));

        ImageView poster = new ImageView(this);
        poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
        poster.setBackground(Ui.round(Color.rgb(25, 29, 38), 8));
        card.addView(poster, new LinearLayout.LayoutParams(Ui.dp(this, 150), Ui.dp(this, 225)));
        String posterUrl = raw.optString("poster_url", "");
        if (!posterUrl.isEmpty()) images.load(poster, posterUrl);

        TextView name = Ui.title(this, raw.optString("title", item.series ? "Série" : "Título"), 12);
        name.setMaxLines(1);
        card.addView(name, Ui.margin(this, Ui.dp(this, 150), ViewGroup.LayoutParams.WRAP_CONTENT, 0, 7, 0, 0));
        String meta = item.series
            ? raw.optInt("season_count", 0) + " temp. · " + raw.optInt("episode_count", 0) + " ep."
            : (raw.optInt("year", 0) > 0 ? String.valueOf(raw.optInt("year")) : raw.optString("media_type", ""));
        TextView detail = Ui.muted(this, meta, 10);
        detail.setMaxLines(1);
        card.addView(detail);

        card.setOnClickListener(v -> {
            if (item.series) {
                String id = raw.optString("series_id", raw.optString("id", ""));
                Intent intent = new Intent(this, SeriesDetailActivity.class);
                intent.putExtra("series_id", id);
                startActivity(intent);
            } else {
                long id = raw.optLong("id");
                Intent intent = new Intent(this, DetailActivity.class);
                intent.putExtra("media_id", id);
                startActivity(intent);
            }
        });

        LinearLayout holder = new LinearLayout(this);
        holder.addView(card, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 0, 10, 0));
        return holder;
    }

    private void error(Exception e) {
        if (e instanceof ApiClient.ApiException && ((ApiClient.ApiException) e).status == 401) {
            api.store().clear();
            startActivity(new Intent(this, LoginActivity.class));
            finish();
            return;
        }
        Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show();
        Ui.clear(content);
        content.addView(Ui.title(this, "Não foi possível carregar", 26));
        content.addView(Ui.muted(this, e.getMessage(), 13));
    }

    @Override protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }
}