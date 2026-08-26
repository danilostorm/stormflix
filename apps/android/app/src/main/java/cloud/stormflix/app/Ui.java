package cloud.stormflix.app;

import android.content.Context;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class Ui {
    public static final int BG = Color.rgb(5, 6, 9);
    public static final int PANEL = Color.rgb(18, 21, 28);
    public static final int TEXT = Color.rgb(244, 246, 250);
    public static final int MUTED = Color.rgb(144, 153, 169);
    public static final int RED = Color.rgb(255, 82, 106);

    private Ui() {}

    public static int dp(Context c, int value) {
        return Math.round(value * c.getResources().getDisplayMetrics().density);
    }

    public static LinearLayout vertical(Context c, int padding) {
        LinearLayout l = new LinearLayout(c);
        l.setOrientation(LinearLayout.VERTICAL);
        l.setPadding(dp(c, padding), dp(c, padding), dp(c, padding), dp(c, padding));
        return l;
    }

    public static LinearLayout horizontal(Context c, int padding) {
        LinearLayout l = new LinearLayout(c);
        l.setOrientation(LinearLayout.HORIZONTAL);
        l.setGravity(Gravity.CENTER_VERTICAL);
        l.setPadding(dp(c, padding), dp(c, padding), dp(c, padding), dp(c, padding));
        return l;
    }

    public static TextView title(Context c, String value, float size) {
        TextView t = new TextView(c);
        t.setText(value);
        t.setTextColor(TEXT);
        t.setTextSize(size);
        t.setTypeface(Typeface.DEFAULT_BOLD);
        return t;
    }

    public static TextView muted(Context c, String value, float size) {
        TextView t = new TextView(c);
        t.setText(value);
        t.setTextColor(MUTED);
        t.setTextSize(size);
        return t;
    }

    public static Button button(Context c, String value, boolean primary) {
        Button b = new Button(c);
        b.setText(value);
        b.setTextColor(Color.WHITE);
        b.setTextSize(14);
        b.setAllCaps(false);
        b.setFocusable(true);
        b.setFocusableInTouchMode(false);
        b.setPadding(dp(c, 16), dp(c, 10), dp(c, 16), dp(c, 10));
        GradientDrawable normal = round(primary ? RED : Color.rgb(37, 42, 52), 10);
        b.setBackground(normal);
        b.setOnFocusChangeListener((v, focused) -> {
            GradientDrawable d = round(focused ? Color.rgb(255, 105, 123) : (primary ? RED : Color.rgb(37, 42, 52)), 10);
            if (focused) d.setStroke(dp(c, 2), Color.WHITE);
            v.setBackground(d);
            v.setScaleX(focused ? 1.035f : 1f);
            v.setScaleY(focused ? 1.035f : 1f);
            if (focused) RemoteUi.keepVisible(v);
        });
        return b;
    }

    public static GradientDrawable round(int color, int radiusDp) {
        GradientDrawable d = new GradientDrawable();
        d.setColor(color);
        d.setCornerRadius(radiusDp);
        return d;
    }

    public static LinearLayout.LayoutParams lp(int w, int h) {
        return new LinearLayout.LayoutParams(w, h);
    }

    public static LinearLayout.LayoutParams margin(Context c, int w, int h, int left, int top, int right, int bottom) {
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(w, h);
        p.setMargins(dp(c, left), dp(c, top), dp(c, right), dp(c, bottom));
        return p;
    }

    public static void clear(ViewGroup group) {
        group.removeAllViews();
    }
}
