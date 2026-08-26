package cloud.stormflix.app;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.Gravity;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.json.JSONObject;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class LoginActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private EditText server, username, password;
    private TextView message;
    private Button submit;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(Ui.BG);
        getWindow().setNavigationBarColor(Ui.BG);
        api = new ApiClient(this);
        build();
    }

    private void build() {
        LinearLayout page = Ui.vertical(this, 24);
        page.setBackgroundColor(Ui.BG);
        page.setGravity(Gravity.CENTER);

        LinearLayout card = Ui.vertical(this, 22);
        card.setBackground(Ui.round(Ui.PANEL, 18));
        int width = Ui.dp(this, 430);
        page.addView(card, new LinearLayout.LayoutParams(width, ViewGroup.LayoutParams.WRAP_CONTENT));

        TextView brand = Ui.title(this, "STORMFLIX", 29);
        brand.setTextColor(Color.WHITE);
        card.addView(brand);
        TextView kicker = Ui.muted(this, "ANDROID · TV · FIRE TV · DIRECT PLAY", 11);
        card.addView(kicker, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 4, 0, 20));
        card.addView(Ui.title(this, "Entrar", 26));

        server = field("Servidor", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        server.setText(api.store().baseUrl());
        username = field("Usuário", InputType.TYPE_CLASS_TEXT);
        password = field("Senha", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        card.addView(server, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 52), 0, 18, 0, 0));
        card.addView(username, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 52), 0, 10, 0, 0));
        card.addView(password, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 52), 0, 10, 0, 0));

        submit = Ui.button(this, "Entrar", true);
        card.addView(submit, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 52), 0, 14, 0, 0));
        submit.setOnClickListener(v -> login());

        message = Ui.muted(this, "", 12);
        card.addView(message, Ui.margin(this, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 12, 0, 0));
        username.requestFocus();
    }

    private EditText field(String hint, int type) {
        EditText e = new EditText(this);
        e.setHint(hint);
        e.setHintTextColor(Color.rgb(110, 119, 133));
        e.setTextColor(Color.WHITE);
        e.setSingleLine(true);
        e.setInputType(type);
        e.setPadding(Ui.dp(this, 14), 0, Ui.dp(this, 14), 0);
        e.setBackground(Ui.round(Color.rgb(28, 32, 41), 9));
        return e;
    }

    private void login() {
        String base = SessionStore.normalizeBase(server.getText().toString());
        String user = username.getText().toString().trim();
        String pass = password.getText().toString();
        if (user.isEmpty() || pass.isEmpty()) { message.setText("Informe usuário e senha."); return; }
        submit.setEnabled(false); submit.setText("Entrando…"); message.setText("");
        api.store().setBaseUrl(base);
        io.submit(() -> {
            try {
                JSONObject body = new JSONObject().put("username", user).put("password", pass);
                api.post("/auth/login", body);
                main.post(() -> {
                    startActivity(new Intent(this, ProfileActivity.class));
                    finish();
                });
            } catch (Exception e) {
                main.post(() -> {
                    submit.setEnabled(true); submit.setText("Entrar");
                    message.setText("Não foi possível entrar: " + e.getMessage());
                });
            }
        });
    }
}
