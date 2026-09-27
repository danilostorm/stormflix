package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
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

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Cinematic native details page shared by phone/tablet/Android TV/Fire TV.
 *
 * The layout intentionally keeps the same interaction ideas that worked well in
 * the DriveTV reference: full-bleed backdrop, dark readability gradients, large
 * title/meta, short synopsis and a small row of obvious primary actions.
 */
public class DetailActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private PlaybackAnywhereNative anywhereBridge;
    private long mediaId;
    private boolean television;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        mediaId = getIntent().getLongExtra("media_id", 0);
        api = new ApiClient(this);
        images = new ImageLoader(this);
        anywhereBridge = new PlaybackAnywhereNative(this, null);
        television = RemoteUi.isTelevision(this);
        if (mediaId <= 0) { finish(); return; }

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(Ui.BG);
        scroll.setClipToPadding(false);
        content = Ui.vertical(this, 0);
        scroll.addView(content, new ScrollView.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        TextView loading = Ui.muted(this, "Carregando informações…", 14);
        loading.setGravity(Gravity.CENTER);
        content.addView(loading, new LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 240)));
        setContentView(scroll);
        load();
    }

    private void load() {
        io.submit(() -> {
            try {
                JSONObject detail = new JSONObject(api.get("/media/" + mediaId));
                main.post(() -> render(detail));
            } catch (Exception e) {
                main.post(() -> {
                    Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show();
                    finish();
                });
            }
        });
    }

    private void render(JSONObject o) {
        Ui.clear(content);
        Models.Media media = Models.Media.from(o);

        content.addView(hero(media, o));
        LinearLayout body = Ui.vertical(this, television ? 42 : 24);

        if (!media.overview.isEmpty()) {
            TextView heading = Ui.title(this, "Sinopse", television ? 22 : 20);
            body.addView(heading);
            TextView overview = Ui.muted(this, media.overview, television ? 16 : 15);
            overview.setTextColor(Color.rgb(205, 211, 222));
            overview.setLineSpacing(0, 1.2f);
            overview.setMaxWidth(Ui.dp(this, television ? 980 : 760));
            body.addView(overview, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT, 0, 10, 0, 0));
        }

        JSONArray directors = o.optJSONArray("directors");
        LinearLayout facts = Ui.vertical(this, 0);
        boolean hasFacts = false;
        if (directors != null && directors.length() > 0) {
            facts.addView(labelValue("Direção", join(directors)));
            hasFacts = true;
        }
        if (!media.genres.isEmpty()) {
            facts.addView(labelValue("Gêneros", String.join(", ", media.genres)));
            hasFacts = true;
        }
        if (!media.libraryName.isEmpty()) {
            facts.addView(labelValue("Biblioteca", media.libraryName));
            hasFacts = true;
        }
        if (hasFacts) body.addView(facts, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 22, 0, 0));

        JSONArray cast = o.optJSONArray("cast");
        if (cast != null && cast.length() > 0) {
            body.addView(Ui.title(this, "Elenco", television ? 24 : 21),
                Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
                    ViewGroup.LayoutParams.WRAP_CONTENT, 0, 30, 0, 8));
            HorizontalScrollView hs = new HorizontalScrollView(this);
            hs.setHorizontalScrollBarEnabled(false);
            hs.setClipToPadding(false);
            LinearLayout rail = Ui.horizontal(this, 0);
            for (int i = 0; i < cast.length(); i++) {
                JSONObject p = cast.optJSONObject(i);
                if (p != null) rail.addView(personCard(p));
            }
            hs.addView(rail);
            body.addView(hs);
        }

        JSONArray related = o.optJSONArray("related");
        if (related != null && related.length() > 0) {
            body.addView(Ui.title(this, "Você também pode gostar", television ? 24 : 21),
                Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
                    ViewGroup.LayoutParams.WRAP_CONTENT, 0, 32, 0, 10));
            HorizontalScrollView hs = new HorizontalScrollView(this);
            hs.setHorizontalScrollBarEnabled(false);
            hs.setClipToPadding(false);
            LinearLayout rail = Ui.horizontal(this, 0);
            for (int i = 0; i < related.length(); i++) {
                JSONObject r = related.optJSONObject(i);
                if (r != null) rail.addView(relatedCard(Models.Media.from(r)));
            }
            hs.addView(rail);
            body.addView(hs);
        }

        body.setPadding(body.getPaddingLeft(), body.getPaddingTop(),
            body.getPaddingRight(), Ui.dp(this, 60));
        content.addView(body);
    }

    private View hero(Models.Media media, JSONObject detail) {
        int heroHeight = Ui.dp(this, television ? 510 : 440);
        FrameLayout hero = new FrameLayout(this);
        hero.setBackgroundColor(Color.rgb(12, 14, 19));

        ImageView backdrop = new ImageView(this);
        backdrop.setScaleType(ImageView.ScaleType.CENTER_CROP);
        backdrop.setBackgroundColor(Color.rgb(20, 23, 30));
        hero.addView(backdrop, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, heroHeight));
        if (!media.backdropUrl.isEmpty()) images.load(backdrop, media.backdropUrl);

        View horizontalShade = new View(this);
        GradientDrawable horizontal = new GradientDrawable(
            GradientDrawable.Orientation.LEFT_RIGHT,
            new int[]{Color.argb(245, 5, 6, 9), Color.argb(205, 5, 6, 9),
                Color.argb(70, 5, 6, 9), Color.TRANSPARENT});
        horizontalShade.setBackground(horizontal);
        hero.addView(horizontalShade, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, heroHeight));

        View verticalShade = new View(this);
        GradientDrawable vertical = new GradientDrawable(
            GradientDrawable.Orientation.TOP_BOTTOM,
            new int[]{Color.argb(25, 5, 6, 9), Color.argb(35, 5, 6, 9),
                Color.argb(235, 5, 6, 9)});
        verticalShade.setBackground(vertical);
        hero.addView(verticalShade, new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, heroHeight));

        Button back = Ui.button(this, "← Voltar", false);
        back.setOnClickListener(v -> finish());
        FrameLayout.LayoutParams backLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this, 44), Gravity.START | Gravity.TOP);
        backLp.setMargins(Ui.dp(this, television ? 40 : 20), Ui.dp(this, 20), 0, 0);
        hero.addView(back, backLp);

        LinearLayout info = Ui.vertical(this, 0);
        int side = television ? 48 : 24;
        info.setPadding(Ui.dp(this, side), 0, Ui.dp(this, side), Ui.dp(this, television ? 36 : 28));

        if (!media.logoUrl.isEmpty()) {
            ImageView logo = new ImageView(this);
            logo.setAdjustViewBounds(true);
            logo.setScaleType(ImageView.ScaleType.CENTER_INSIDE);
            info.addView(logo, new LinearLayout.LayoutParams(
                (television ? Ui.dp(this, 420) : ViewGroup.LayoutParams.MATCH_PARENT), Ui.dp(this, television ? 120 : 92)));
            images.load(logo, media.logoUrl);
        } else {
            TextView title = Ui.title(this, media.title, television ? 43 : 34);
            title.setMaxLines(2);
            title.setShadowLayer(8, 0, 3, Color.BLACK);
            info.addView(title);
        }

        LinearLayout badges = Ui.horizontal(this, 0);
        if (detail.optBoolean("is_4k", false) || media.title.toLowerCase(Locale.ROOT).contains("4k")) {
            badges.addView(badge("4K UHD", Ui.RED));
        }
        if (media.year > 0) badges.addView(badge(String.valueOf(media.year), Color.rgb(45, 50, 62)));
        if (media.rating > 0) badges.addView(badge("★ " + String.format(Locale.US, "%.1f", media.rating),
            Color.rgb(45, 50, 62)));
        if (media.runtimeMinutes > 0) badges.addView(badge(media.runtimeMinutes + " min",
            Color.rgb(45, 50, 62)));
        if (!media.extension.isEmpty()) badges.addView(badge(
            media.extension.replace(".", "").toUpperCase(Locale.ROOT), Color.rgb(45, 50, 62)));
        info.addView(badges, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 14, 0, 0));

        String tagline = detail.optString("tagline", "");
        if (!tagline.isEmpty()) {
            TextView tag = Ui.muted(this, tagline, television ? 16 : 14);
            tag.setTextColor(Color.rgb(220, 224, 232));
            tag.setMaxLines(2);
            info.addView(tag, Ui.margin(this, (television ? Ui.dp(this, 820) : ViewGroup.LayoutParams.MATCH_PARENT),
                ViewGroup.LayoutParams.WRAP_CONTENT, 0, 12, 0, 0));
        }

        String shortOverview = media.overview;
        if (!shortOverview.isEmpty()) {
            TextView overview = Ui.muted(this, shortOverview, television ? 15 : 14);
            overview.setTextColor(Color.rgb(210, 215, 225));
            overview.setMaxLines(television ? 4 : 3);
            overview.setLineSpacing(0, 1.16f);
            info.addView(overview, Ui.margin(this, (television ? Ui.dp(this, 820) : ViewGroup.LayoutParams.MATCH_PARENT),
                ViewGroup.LayoutParams.WRAP_CONTENT, 0, 12, 0, 0));
        }

        LinearLayout actions = television ? Ui.horizontal(this, 0) : Ui.vertical(this, 0);
        Button play = Ui.button(this, "▶  Assistir agora", true);
        play.setTextSize(television ? 16 : 14);
        play.setOnClickListener(v -> play(media));
        actions.addView(play, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            Ui.dp(this, television ? 54 : 50), 0, 0, 12, 0));

        Button anywhere = Ui.button(this, "▣  Reproduzir em…", false);
        anywhere.setTextSize(television ? 15 : 13);
        anywhere.setOnClickListener(v -> showPlaybackAnywhere(media));
        actions.addView(anywhere, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            Ui.dp(this, television ? 54 : 50), 0, 0, 12, 0));

        info.addView(actions, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 18, 0, 0));

        FrameLayout.LayoutParams infoLp = new FrameLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT,
            Gravity.START | Gravity.BOTTOM);
        hero.addView(info, infoLp);
        hero.setLayoutParams(new LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, heroHeight));
        return hero;
    }

    private TextView badge(String value, int color) {
        TextView badge = Ui.title(this, value, 11);
        badge.setTextColor(Color.WHITE);
        badge.setGravity(Gravity.CENTER);
        badge.setPadding(Ui.dp(this, 9), Ui.dp(this, 5), Ui.dp(this, 9), Ui.dp(this, 5));
        badge.setBackground(Ui.round(color, 7));
        badge.setLayoutParams(Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 0, 8, 0));
        return badge;
    }

    private void showPlaybackAnywhere(Models.Media media) {
        anywhereBridge.chooseOriginal(api, media.id, media.title, 0);
    }

    private String remoteMime(JSONObject plan, String url) {
        String lower = url == null ? "" : url.toLowerCase(Locale.ROOT);
        String mode = plan.optString("mode", "");
        String transport = plan.optString("transport", "");
        if (lower.contains(".m3u8") || "hls".equalsIgnoreCase(transport)
            || "video_transcode".equals(mode)) return "application/x-mpegURL";
        return "video/mp4";
    }

    private LinearLayout personCard(JSONObject p) {
        LinearLayout card = Ui.vertical(this, 0);
        card.setGravity(Gravity.CENTER_HORIZONTAL);
        card.setFocusable(true);
        card.setClickable(true);
        card.setBackground(Ui.round(Color.TRANSPARENT, 10));
        card.setOnFocusChangeListener((v, f) -> {
            v.setBackground(Ui.round(f ? Color.rgb(31, 35, 45) : Color.TRANSPARENT, 10));
            v.setScaleX(f ? 1.06f : 1f);
            v.setScaleY(f ? 1.06f : 1f);
            if (f) RemoteUi.keepVisible(v);
        });
        ImageView image = new ImageView(this);
        image.setScaleType(ImageView.ScaleType.CENTER_CROP);
        image.setBackground(Ui.round(Color.rgb(35, 40, 50), 60));
        card.addView(image, new LinearLayout.LayoutParams(Ui.dp(this, 92), Ui.dp(this, 92)));
        String url = p.optString("profile_url", "");
        if (!url.isEmpty()) images.load(image, url);
        String name = p.optString("name", "");
        TextView n = Ui.title(this, name, 11);
        n.setMaxLines(1);
        n.setGravity(Gravity.CENTER);
        card.addView(n, Ui.margin(this, Ui.dp(this, 110),
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 6, 0, 0));
        TextView c = Ui.muted(this, p.optString("character", ""), 9);
        c.setMaxLines(1);
        c.setGravity(Gravity.CENTER);
        card.addView(c);
        card.setOnClickListener(v -> {
            if (name.trim().isEmpty()) return;
            Intent intent = new Intent(this, PersonActivity.class);
            intent.putExtra("person_name", name);
            startActivity(intent);
        });
        LinearLayout holder = Ui.horizontal(this, 0);
        holder.addView(card, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 0, 14, 0));
        return holder;
    }

    private LinearLayout relatedCard(Models.Media media) {
        LinearLayout card = Ui.vertical(this, 0);
        card.setFocusable(true);
        card.setClickable(true);
        card.setOnClickListener(v -> {
            Intent i = new Intent(this, DetailActivity.class);
            i.putExtra("media_id", media.id);
            startActivity(i);
        });
        card.setOnFocusChangeListener((v, f) -> {
            v.setScaleX(f ? 1.05f : 1);
            v.setScaleY(f ? 1.05f : 1);
            if (f) RemoteUi.keepVisible(v);
        });
        ImageView poster = new ImageView(this);
        poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
        poster.setBackgroundColor(Color.rgb(24, 28, 36));
        card.addView(poster, new LinearLayout.LayoutParams(
            Ui.dp(this, television ? 150 : 132), Ui.dp(this, television ? 225 : 198)));
        if (!media.posterUrl.isEmpty()) images.load(poster, media.posterUrl);
        TextView t = Ui.title(this, media.title, 11);
        t.setMaxLines(1);
        card.addView(t, Ui.margin(this, Ui.dp(this, television ? 150 : 132),
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 6, 0, 0));
        LinearLayout holder = Ui.horizontal(this, 0);
        holder.addView(card, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 0, 0, 12, 0));
        return holder;
    }

    private LinearLayout labelValue(String label, String value) {
        LinearLayout row = Ui.horizontal(this, 0);
        TextView l = Ui.muted(this, label + ":", 12);
        TextView v = Ui.title(this, value, 12);
        row.addView(l);
        row.addView(v, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT,
            ViewGroup.LayoutParams.WRAP_CONTENT, 8, 0, 0, 0));
        return row;
    }

    private String join(JSONArray a) {
        List<String> out = new ArrayList<>();
        for (int i = 0; i < a.length(); i++) out.add(a.optString(i));
        return String.join(", ", out);
    }

    private void play(Models.Media media) {
        Intent i = new Intent(this, NativePlayerActivity.class);
        i.putExtra("media_id", media.id);
        i.putExtra("title", media.title);
        startActivity(i);
    }
}
