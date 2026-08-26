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
import android.widget.GridLayout;
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
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class PersonActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private String personName;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        personName = getIntent().getStringExtra("person_name");
        if (personName == null || personName.trim().isEmpty()) { finish(); return; }
        api = new ApiClient(this);
        images = new ImageLoader(this);
        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(Ui.BG);
        content = Ui.vertical(this, 20);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        content.addView(Ui.title(this, personName, 30));
        content.addView(Ui.muted(this, "Buscando títulos disponíveis no seu StormFlix…", 13));
        setContentView(scroll);
        load();
    }

    private void load() {
        io.submit(() -> {
            try {
                String encoded = URLEncoder.encode(personName, StandardCharsets.UTF_8.toString());
                JSONObject result = new JSONObject(api.get("/people?name=" + encoded));
                main.post(() -> render(result));
            } catch (Exception e) {
                main.post(() -> {
                    Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show();
                    finish();
                });
            }
        });
    }

    private void render(JSONObject result) {
        Ui.clear(content);
        TextView back = Ui.title(this, "←  " + personName, 28);
        back.setFocusable(true); back.setClickable(true); back.setOnClickListener(v -> finish());
        content.addView(back);

        JSONObject person = result.optJSONObject("person");
        if (person != null) {
            LinearLayout personRow = Ui.horizontal(this, 0);
            ImageView photo = new ImageView(this);
            photo.setScaleType(ImageView.ScaleType.CENTER_CROP);
            photo.setBackground(Ui.round(Color.rgb(31,35,45), 60));
            personRow.addView(photo, new LinearLayout.LayoutParams(Ui.dp(this, 110), Ui.dp(this, 110)));
            String url = person.optString("profile_url", "");
            if (!url.isEmpty()) images.load(photo, url);
            LinearLayout copy = Ui.vertical(this, 0);
            copy.addView(Ui.title(this, person.optString("name", personName), 26));
            String character = person.optString("character", "");
            if (!character.isEmpty()) copy.addView(Ui.muted(this, "No título atual: " + character, 12));
            personRow.addView(copy, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 18,0,0,0));
            content.addView(personRow, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,20,0,20));
        }

        JSONArray items = result.optJSONArray("items");
        int count = items == null ? 0 : items.length();
        content.addView(Ui.title(this, "No StormFlix", 22));
        content.addView(Ui.muted(this, count + (count == 1 ? " título disponível" : " títulos disponíveis"), 12), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,2,0,14));
        if (count == 0) {
            content.addView(Ui.muted(this, "Nenhum outro título deste artista está catalogado com elenco identificado.", 14));
            return;
        }

        GridLayout grid = new GridLayout(this);
        int widthDp = Math.round(getResources().getDisplayMetrics().widthPixels / getResources().getDisplayMetrics().density);
        int columns = widthDp >= 1000 ? 6 : widthDp >= 700 ? 4 : widthDp >= 450 ? 3 : 2;
        grid.setColumnCount(columns);
        for (int i = 0; i < count; i++) {
            JSONObject obj = items.optJSONObject(i);
            if (obj == null) continue;
            grid.addView(card(Models.Media.from(obj)));
        }
        content.addView(grid);
    }

    private View card(Models.Media media) {
        LinearLayout card = Ui.vertical(this, 0);
        card.setFocusable(true); card.setClickable(true);
        card.setOnFocusChangeListener((v,f)->{
            v.setBackground(Ui.round(f ? Color.rgb(31,35,45) : Color.TRANSPARENT, 10));
            v.setScaleX(f ? 1.05f : 1f); v.setScaleY(f ? 1.05f : 1f);
        });
        ImageView poster = new ImageView(this);
        poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
        poster.setBackground(Ui.round(Color.rgb(24,28,36), 8));
        card.addView(poster, new LinearLayout.LayoutParams(Ui.dp(this, 145), Ui.dp(this, 218)));
        if (!media.posterUrl.isEmpty()) images.load(poster, media.posterUrl);
        TextView title = Ui.title(this, media.title, 12); title.setMaxLines(2);
        card.addView(title, Ui.margin(this, Ui.dp(this,145), ViewGroup.LayoutParams.WRAP_CONTENT,0,7,0,0));
        List<String> meta = new ArrayList<>();
        if (media.year > 0) meta.add(String.valueOf(media.year));
        if (media.rating > 0) meta.add("★ " + String.format(Locale.US, "%.1f", media.rating));
        card.addView(Ui.muted(this, String.join(" · ", meta), 10));
        card.setOnClickListener(v -> {
            Intent intent = new Intent(this, DetailActivity.class);
            intent.putExtra("media_id", media.id);
            startActivity(intent);
        });
        GridLayout.LayoutParams gp = new GridLayout.LayoutParams();
        gp.width = Ui.dp(this, 165); gp.height = ViewGroup.LayoutParams.WRAP_CONTENT;
        gp.setMargins(Ui.dp(this,5), Ui.dp(this,5), Ui.dp(this,8), Ui.dp(this,18));
        card.setLayoutParams(gp);
        return card;
    }
}
