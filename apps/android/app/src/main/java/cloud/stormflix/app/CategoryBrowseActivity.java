package cloud.stormflix.app;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.LinearLayout;
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
    static final class CardItem {
        final JSONObject raw;
        final boolean series;
        final String key;
        CardItem(JSONObject raw, boolean series, String key) {
            this.raw = raw;
            this.series = series;
            this.key = key;
        }
    }

    static final class Section {
        final String title;
        final List<CardItem> items;
        android.os.Parcelable scrollState;
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

        content = Ui.vertical(this, RemoteUi.isTelevision(this) ? 24 : 18);
        content.setBackgroundColor(Ui.BG);
        setContentView(content);
        loading();
        load();
    }

    private void loading() {
        if (isFinishing() || isDestroyed()) return;
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
                    JSONObject data = new JSONObject(api.get("/categories/" + Uri.encode(childSlug) + "/smart"));
                    List<CardItem> items = categoryItems(data);
                    if (items.isEmpty()) continue;
                    sections.add(new Section(child.optString("name", childSlug), items));
                    for (CardItem item : items) aggregate.putIfAbsent(item.key, item);
                }

                List<CardItem> general = new ArrayList<>(aggregate.values());
                if (children.isEmpty()) sections.addAll(genreSections(general));
                main.post(() -> render(rootTitle, general, sections, !children.isEmpty()));
            } catch (Exception e) {
                main.post(() -> error(e));
            }
        });
    }

    private List<Section> genreSections(List<CardItem> items) {
        Map<String, List<CardItem>> groups = new LinkedHashMap<>();
        for (CardItem item : items) {
            String title = "Outros";
            JSONArray genres = item.raw.optJSONArray("genres");
            if (genres != null) for (int i = 0; i < genres.length(); i++) {
                String genre = CatalogGenres.title(genres.optString(i));
                if (genre != null) { title = genre; break; }
            }
            groups.computeIfAbsent(title, key -> new ArrayList<>()).add(item);
        }
        List<Section> sections = new ArrayList<>();
        for (Map.Entry<String, List<CardItem>> entry : groups.entrySet())
            sections.add(new Section(entry.getKey(), entry.getValue()));
        java.text.Collator collator = java.text.Collator.getInstance(new java.util.Locale("pt", "BR"));
        sections.sort((a, b) -> {
            if (a.title.equals("Outros")) return b.title.equals("Outros") ? 0 : 1;
            if (b.title.equals("Outros")) return -1;
            int count = Integer.compare(b.items.size(), a.items.size());
            return count != 0 ? count : collator.compare(a.title, b.title);
        });
        return sections;
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

    private void render(String title, List<CardItem> general, List<Section> sections, boolean configuredSections) {
        if (isFinishing() || isDestroyed()) return;
        Ui.clear(content);
        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), 0, 0, 0, 10));
        content.addView(Ui.title(this, title, 32));
        content.addView(Ui.muted(this, general.size() + " títulos", 12), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 4, 0, 8));

        List<Section> rows = new ArrayList<>(sections);
        if (!general.isEmpty() && (rows.isEmpty() || configuredSections))
            rows.add(new Section(rows.isEmpty() ? title : "Todos os títulos", general));
        if (rows.isEmpty()) content.addView(Ui.muted(this, "Este menu ainda não possui títulos.", 14));
        else {
            androidx.recyclerview.widget.RecyclerView list = new androidx.recyclerview.widget.RecyclerView(this);
            list.setLayoutManager(new androidx.recyclerview.widget.LinearLayoutManager(this));
            list.setHasFixedSize(true);
            list.setItemAnimator(null);
            list.setItemViewCacheSize(2);
            list.setClipToPadding(false);
            list.setAdapter(new CatalogSectionAdapter(this, rows, images, this::openCard));
            content.addView(list, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1));
        }
        RemoteUi.focusFirst(content);
    }

    private void openCard(CardItem item) {
        JSONObject raw = item.raw;
        Intent intent = new Intent(this, item.series ? SeriesDetailActivity.class : DetailActivity.class);
        if (item.series) intent.putExtra("series_id", raw.optString("series_id", raw.optString("id", "")));
        else intent.putExtra("media_id", raw.optLong("id"));
        startActivity(intent);
    }

    private void error(Exception e) {
        if (isFinishing() || isDestroyed()) return;
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
        main.removeCallbacksAndMessages(null);
        io.shutdownNow();
        super.onDestroy();
    }
}
