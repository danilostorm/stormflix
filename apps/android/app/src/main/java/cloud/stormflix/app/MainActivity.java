package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {
    private final ExecutorService io = Executors.newFixedThreadPool(3);
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout page;
    private LinearLayout content;
    private boolean supports4k;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        api = new ApiClient(this);
        images = new ImageLoader(this);
        supports4k = DeviceCapabilities.supports4kVideo();
        if (!api.store().signedIn()) { startActivity(new Intent(this, LoginActivity.class)); finish(); return; }
        if (api.store().profileCookie().isEmpty()) { startActivity(new Intent(this, ProfileActivity.class)); finish(); return; }
        buildShell();
        loadHome();
    }

    @Override protected void onResume() {
        super.onResume();
        if (content != null && api != null && api.store().signedIn()) loadHome();
    }

    private void buildShell() {
        page = new LinearLayout(this);
        page.setOrientation(LinearLayout.VERTICAL);
        page.setBackgroundColor(Ui.BG);

        HorizontalScrollView topScroll = new HorizontalScrollView(this);
        topScroll.setHorizontalScrollBarEnabled(false);
        topScroll.setFocusable(false);
        LinearLayout top = Ui.horizontal(this, 12);
        top.setBackgroundColor(Color.rgb(7, 8, 12));
        TextView brand = Ui.title(this, "STORMFLIX", 22);
        top.addView(brand, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 8, 0, 26, 0));
        addNav(top, "Início", this::loadHome);
        addNav(top, "Filmes", () -> loadCategory("Filmes", "movie"));
        addNav(top, "Séries", () -> openSeries("series"));
        addNav(top, "Animes", () -> openSeries("anime"));
        addNav(top, "Música", () -> startActivity(new Intent(this, MusicActivity.class)));
        addNav(top, "Buscar", this::searchDialog);
        addNav(top, "Perfis", () -> startActivity(new Intent(this, ProfileActivity.class)));
        addNav(top, "Sair", this::logout);
        topScroll.addView(top);
        page.addView(topScroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 66)));

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        content = Ui.vertical(this, RemoteUi.isTelevision(this) ? 24 : 18);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        page.addView(scroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1));
        setContentView(page);
        RemoteUi.focusFirst(top);
    }

    private void addNav(LinearLayout top, String title, Runnable action) {
        Button b = Ui.button(this, title, false);
        b.setOnClickListener(v -> action.run());
        top.addView(b, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 42), 0, 0, 8, 0));
    }

    private void openSeries(String kind) {
        Intent intent = new Intent(this, SeriesBrowseActivity.class);
        intent.putExtra("filter_kind", kind);
        startActivity(intent);
    }

    private void loading(String label) {
        Ui.clear(content);
        content.addView(Ui.title(this, label, 28));
        content.addView(Ui.muted(this, "Carregando catálogo…", 13), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 8, 0, 0));
    }

    private void loadHome() {
        loading("Início");
        io.submit(() -> {
            try {
                Models.Home home = Models.Home.from(api.get("/home"));
                List<Models.Media> continuing = parseMediaArray(api.get("/profiles/continue"));
                main.post(() -> renderHome(home, continuing));
            } catch (Exception e) { main.post(() -> error(e)); }
        });
    }

    private void renderHome(Models.Home home, List<Models.Media> continuing) {
        Ui.clear(content);
        Models.Media hero = visibleOnDevice(home.hero) ? home.hero : firstVisible(home.rows);
        if (hero != null) content.addView(hero(hero));

        Set<String> renderedRows = new HashSet<>();
        List<Models.Media> continueVisible = dedupeMedia(filterForDevice(continuing));
        if (!continueVisible.isEmpty()) {
            content.addView(row("Continuar assistindo", continueVisible));
            renderedRows.add(normalizeRowTitle("Continuar assistindo"));
        }

        for (Models.Row r : home.rows) {
            String rowKey = normalizeRowTitle(r.title);
            if (renderedRows.contains(rowKey)) continue;
            if (!continueVisible.isEmpty() && isContinueWatchingTitle(r.title)) continue;
            if (!supports4k && looks4k(r.title)) continue;
            List<Models.Media> visible = dedupeMedia(filterForDevice(r.items));
            if (!visible.isEmpty()) {
                content.addView(row(r.title, visible));
                renderedRows.add(rowKey);
            }
        }
    }

    private boolean isContinueWatchingTitle(String value) {
        String normalized = normalizeRowTitle(value);
        return normalized.contains("continuar assistindo") || normalized.contains("continue watching");
    }

    private String normalizeRowTitle(String value) {
        return value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
    }

    private List<Models.Media> dedupeMedia(List<Models.Media> items) {
        List<Models.Media> out = new ArrayList<>();
        Set<Long> ids = new HashSet<>();
        for (Models.Media item : items) {
            if (item == null || item.id <= 0 || !ids.add(item.id)) continue;
            out.add(item);
        }
        return out;
    }

    private Models.Media firstVisible(List<Models.Row> rows) {
        for (Models.Row row : rows) {
            if (!supports4k && looks4k(row.title)) continue;
            for (Models.Media media : row.items) if (visibleOnDevice(media)) return media;
        }
        return null;
    }

    private List<Models.Media> filterForDevice(List<Models.Media> items) {
        if (supports4k) return items;
        List<Models.Media> out = new ArrayList<>();
        for (Models.Media media : items) if (visibleOnDevice(media)) out.add(media);
        return out;
    }

    private boolean visibleOnDevice(Models.Media media) {
        if (media == null) return false;
        return supports4k || !looks4k(media.libraryName);
    }

    private boolean looks4k(String value) {
        String text = value == null ? "" : value.toLowerCase(Locale.ROOT);
        return text.contains("4k") || text.contains("uhd") || text.contains("2160p");
    }

    private View hero(Models.Media media) {
        LinearLayout wrap = Ui.vertical(this, 0);
        FrameLayout art = new FrameLayout(this);
        art.setBackgroundColor(Color.rgb(12, 15, 21));
        ImageView backdrop = new ImageView(this);
        backdrop.setScaleType(ImageView.ScaleType.CENTER_CROP);
        art.addView(backdrop, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 330)));
        if (!media.backdropUrl.isEmpty()) images.load(backdrop, media.backdropUrl);
        LinearLayout copy = Ui.vertical(this, 18);
        copy.setGravity(Gravity.BOTTOM);
        FrameLayout.LayoutParams cp = new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT, Gravity.BOTTOM);
        copy.addView(Ui.title(this, media.title, 34));
        copy.addView(Ui.muted(this, meta(media), 13), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 5, 0, 0));
        if (!media.overview.isEmpty()) {
            TextView overview = Ui.muted(this, media.overview, 14); overview.setMaxLines(3);
            copy.addView(overview, Ui.margin(this, Ui.dp(this, 720), ViewGroup.LayoutParams.WRAP_CONTENT, 0, 10, 0, 0));
        }
        LinearLayout actions = Ui.horizontal(this, 0);
        Button play = Ui.button(this, "▶ Assistir", true); play.setOnClickListener(v -> play(media));
        Button info = Ui.button(this, "Mais informações", false); info.setOnClickListener(v -> detail(media));
        actions.addView(play, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 48), 0, 0, 10, 0));
        actions.addView(info, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 48), 0, 0, 10, 0));
        copy.addView(actions, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 14, 0, 0));
        art.addView(copy, cp);
        wrap.addView(art, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 330)));
        return wrap;
    }

    private View row(String title, List<Models.Media> items) {
        LinearLayout section = Ui.vertical(this, 0);
        if (title != null && !title.trim().isEmpty()) {
            section.addView(Ui.title(this, title, 21), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 22, 0, 10));
        }
        HorizontalScrollView scroll = new HorizontalScrollView(this);
        scroll.setHorizontalScrollBarEnabled(false);
        scroll.setFocusable(false);
        LinearLayout rail = Ui.horizontal(this, 0);
        for (Models.Media media : items) rail.addView(card(media));
        scroll.addView(rail);
        section.addView(scroll, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 292)));
        return section;
    }

    private View card(Models.Media media) {
        LinearLayout card = Ui.vertical(this, 0);
        card.setFocusable(true); card.setClickable(true);
        card.setBackground(Ui.round(Color.TRANSPARENT, 10));
        card.setOnFocusChangeListener((v, focused) -> RemoteUi.cardFocus(v, focused));
        ImageView poster = new ImageView(this);
        poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
        poster.setBackground(Ui.round(Color.rgb(25,29,38), 8));
        card.addView(poster, new LinearLayout.LayoutParams(Ui.dp(this, 150), Ui.dp(this, 225)));
        if (!media.posterUrl.isEmpty()) images.load(poster, media.posterUrl);
        TextView title = Ui.title(this, media.title, 12); title.setMaxLines(1);
        card.addView(title, Ui.margin(this, Ui.dp(this, 150), ViewGroup.LayoutParams.WRAP_CONTENT, 0, 7, 0, 0));
        TextView meta = Ui.muted(this, meta(media), 10); meta.setMaxLines(1); card.addView(meta);
        card.setOnClickListener(v -> detail(media));
        return withMargin(card, 0, 0, 10, 0);
    }

    private View withMargin(View v, int l, int t, int r, int b) {
        LinearLayout holder = new LinearLayout(this);
        holder.addView(v, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, l,t,r,b));
        return holder;
    }

    private void loadCategory(String title, String kind) {
        loading(title);
        io.submit(() -> {
            try {
                List<Models.Media> all = parseMediaArray(api.get("/media?limit=500"));
                List<Models.Media> filtered = new ArrayList<>();
                for (Models.Media m : all) {
                    String type = m.mediaType.toLowerCase(Locale.ROOT);
                    boolean match = kind.equals("movie") ? type.equals("movie") || type.equals("film") : type.equals(kind);
                    if (match && visibleOnDevice(m)) filtered.add(m);
                }
                main.post(() -> renderGridLike(title, dedupeMedia(filtered)));
            } catch (Exception e) { main.post(() -> error(e)); }
        });
    }

    private void renderGridLike(String title, List<Models.Media> list) {
        Ui.clear(content);
        content.addView(Ui.title(this, title, 30));
        content.addView(Ui.muted(this, list.size() + " títulos", 12));
        int chunk = 18;
        for (int i = 0; i < list.size(); i += chunk) content.addView(row(i == 0 ? title : "", list.subList(i, Math.min(list.size(), i + chunk))));
    }

    private void searchDialog() {
        EditText q = new EditText(this); q.setHint("Filme, série ou anime"); q.setSingleLine(true);
        q.setTextColor(Color.WHITE); q.setHintTextColor(Ui.MUTED); q.setBackground(Ui.round(Color.rgb(30,34,43),9)); q.setPadding(Ui.dp(this,14),0,Ui.dp(this,14),0);
        new AlertDialog.Builder(this).setTitle("Buscar no StormFlix").setView(q).setNegativeButton("Cancelar", null).setPositiveButton("Buscar", (d,w) -> search(q.getText().toString())).show();
    }

    private void search(String query) {
        String value = query == null ? "" : query.trim(); if (value.isEmpty()) return;
        loading("Resultados");
        io.submit(() -> {
            try {
                String encoded = URLEncoder.encode(value, StandardCharsets.UTF_8.toString());
                List<Models.Media> list = dedupeMedia(filterForDevice(parseMediaArray(api.get("/media?q=" + encoded + "&limit=200"))));
                main.post(() -> renderGridLike("Resultados para “" + value + "”", list));
            } catch (Exception e) { main.post(() -> error(e)); }
        });
    }

    private List<Models.Media> parseMediaArray(String json) throws Exception {
        JSONArray arr = new JSONArray(json); List<Models.Media> list = new ArrayList<>();
        for (int i=0;i<arr.length();i++){ JSONObject o=arr.optJSONObject(i); if(o!=null) list.add(Models.Media.from(o)); }
        return list;
    }

    private String meta(Models.Media m) {
        StringBuilder s = new StringBuilder();
        if (m.year > 0) s.append(m.year);
        if (m.rating > 0) { if (s.length()>0) s.append(" · "); s.append("★ ").append(String.format(Locale.US,"%.1f",m.rating)); }
        if (!m.extension.isEmpty()) { if (s.length()>0) s.append(" · "); s.append(m.extension.replace(".","").toUpperCase(Locale.ROOT)); }
        return s.toString();
    }

    private void detail(Models.Media media) {
        Intent i = new Intent(this, DetailActivity.class); i.putExtra("media_id", media.id); startActivity(i);
    }

    private void play(Models.Media media) {
        Intent i = new Intent(this, PlayerActivity.class); i.putExtra("media_id", media.id); i.putExtra("title", media.title); startActivity(i);
    }

    private void logout() {
        io.submit(() -> { try { api.post("/auth/logout", new JSONObject()); } catch (Exception ignored) {} api.store().clear(); main.post(() -> { startActivity(new Intent(this, LoginActivity.class)); finish(); }); });
    }

    private void error(Exception e) {
        if (e instanceof ApiClient.ApiException && ((ApiClient.ApiException)e).status == 401) {
            api.store().clear(); startActivity(new Intent(this, LoginActivity.class)); finish(); return;
        }
        Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show();
        Ui.clear(content); content.addView(Ui.title(this, "Não foi possível carregar", 26)); content.addView(Ui.muted(this, e.getMessage(), 13));
    }
}
