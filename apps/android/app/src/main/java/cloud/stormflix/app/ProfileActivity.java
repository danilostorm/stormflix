package cloud.stormflix.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.EditText;
import android.widget.GridLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class ProfileActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        api = new ApiClient(this);
        images = new ImageLoader(this);
        if (!api.store().signedIn()) { startActivity(new Intent(this, LoginActivity.class)); finish(); return; }
        buildLoading();
        loadProfiles();
    }

    private void buildLoading() {
        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(Ui.BG);
        content = Ui.vertical(this, 28);
        content.setGravity(Gravity.CENTER_HORIZONTAL);
        scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        content.addView(Ui.title(this, "STORMFLIX", 25));
        content.addView(Ui.title(this, "Quem está assistindo?", 34), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 40, 0, 20));
        content.addView(Ui.muted(this, "Carregando perfis…", 14));
        setContentView(scroll);
    }

    private void loadProfiles() {
        io.submit(() -> {
            try {
                JSONObject out = new JSONObject(api.get("/profiles"));
                JSONArray arr = out.optJSONArray("profiles");
                List<Models.Profile> profiles = new ArrayList<>();
                if (arr != null) for (int i = 0; i < arr.length(); i++) {
                    JSONObject p = arr.optJSONObject(i); if (p != null) profiles.add(Models.Profile.from(p));
                }
                main.post(() -> render(profiles));
            } catch (Exception e) {
                main.post(() -> renderError(e.getMessage()));
            }
        });
    }

    private void render(List<Models.Profile> profiles) {
        Ui.clear(content);
        content.addView(Ui.title(this, "STORMFLIX", 25));
        content.addView(Ui.title(this, "Quem está assistindo?", 34), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 38, 0, 8));
        content.addView(Ui.muted(this, "Escolha um perfil para abrir sua experiência.", 13), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 0, 0, 24));
        if (profiles.isEmpty()) {
            content.addView(Ui.muted(this, "Nenhum perfil disponível. Crie um perfil pelo StormFlix Web/Admin.", 15));
            return;
        }
        GridLayout grid = new GridLayout(this);
        int screenDp = (int)(getResources().getDisplayMetrics().widthPixels / getResources().getDisplayMetrics().density);
        grid.setColumnCount(screenDp >= 900 ? 5 : screenDp >= 600 ? 4 : 2);
        grid.setUseDefaultMargins(false);
        for (Models.Profile profile : profiles) grid.addView(profileCard(profile));
        content.addView(grid);
    }

    private View profileCard(Models.Profile profile) {
        LinearLayout card = Ui.vertical(this, 10);
        card.setGravity(Gravity.CENTER_HORIZONTAL);
        card.setFocusable(true);
        card.setClickable(true);
        card.setBackground(Ui.round(Color.TRANSPARENT, 14));
        card.setOnFocusChangeListener((v, focused) -> {
            v.setBackground(Ui.round(focused ? Color.rgb(31, 35, 45) : Color.TRANSPARENT, 14));
            v.setScaleX(focused ? 1.07f : 1f); v.setScaleY(focused ? 1.07f : 1f);
        });
        int size = Ui.dp(this, 128);
        ImageView avatar = new ImageView(this);
        avatar.setScaleType(ImageView.ScaleType.CENTER_CROP);
        avatar.setBackground(Ui.round(Color.rgb(40, 46, 59), 18));
        card.addView(avatar, new LinearLayout.LayoutParams(size, size));
        if (!profile.avatarUrl.isEmpty()) images.load(avatar, profile.avatarUrl);
        else {
            TextView initial = Ui.title(this, profile.name.isEmpty() ? "?" : profile.name.substring(0, 1).toUpperCase(), 44);
            initial.setGravity(Gravity.CENTER);
            card.removeView(avatar);
            LinearLayout holder = Ui.vertical(this, 0); holder.setGravity(Gravity.CENTER); holder.setBackground(Ui.round(Color.rgb(40,46,59),18));
            holder.addView(initial, new LinearLayout.LayoutParams(size, size));
            card.addView(holder, new LinearLayout.LayoutParams(size, size));
        }
        TextView name = Ui.title(this, profile.name, 14); name.setGravity(Gravity.CENTER);
        card.addView(name, Ui.margin(this, Ui.dp(this, 150), ViewGroup.LayoutParams.WRAP_CONTENT, 0, 8, 0, 0));
        if (profile.kids) { TextView kids = Ui.muted(this, "INFANTIL", 10); kids.setTextColor(Ui.RED); card.addView(kids); }
        if (profile.pinEnabled) card.addView(Ui.muted(this, "PIN", 10));
        card.setOnClickListener(v -> choose(profile));
        GridLayout.LayoutParams gp = new GridLayout.LayoutParams();
        gp.width = Ui.dp(this, 176); gp.height = ViewGroup.LayoutParams.WRAP_CONTENT;
        gp.setMargins(Ui.dp(this, 8), Ui.dp(this, 8), Ui.dp(this, 8), Ui.dp(this, 18));
        card.setLayoutParams(gp);
        return card;
    }

    private void choose(Models.Profile profile) {
        if (!profile.pinEnabled) { select(profile, ""); return; }
        EditText pin = new EditText(this);
        pin.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_VARIATION_PASSWORD);
        pin.setHint("PIN");
        pin.setTextColor(Color.WHITE); pin.setHintTextColor(Ui.MUTED);
        pin.setBackground(Ui.round(Color.rgb(30,34,43),9)); pin.setPadding(Ui.dp(this,14),0,Ui.dp(this,14),0);
        new AlertDialog.Builder(this).setTitle(profile.name).setMessage("Digite o PIN deste perfil.")
            .setView(pin).setNegativeButton("Cancelar", null)
            .setPositiveButton("Entrar", (d,w) -> select(profile, pin.getText().toString())).show();
    }

    private void select(Models.Profile profile, String pin) {
        io.submit(() -> {
            try {
                api.post("/profiles/" + profile.id + "/select", new JSONObject().put("pin", pin));
                api.store().setProfilePreferences(profile.preferredAudio, profile.preferredSubtitle);
                main.post(() -> { startActivity(new Intent(this, MainActivity.class)); finish(); });
            } catch (Exception e) {
                main.post(() -> new AlertDialog.Builder(this).setTitle("Não foi possível abrir o perfil").setMessage(e.getMessage()).setPositiveButton("OK", null).show());
            }
        });
    }

    private void renderError(String value) {
        Ui.clear(content);
        content.addView(Ui.title(this, "Perfis indisponíveis", 28));
        content.addView(Ui.muted(this, value == null ? "Erro desconhecido" : value, 14));
    }
}
