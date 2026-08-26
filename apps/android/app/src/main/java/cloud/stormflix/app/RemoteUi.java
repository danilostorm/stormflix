package cloud.stormflix.app;

import android.app.UiModeManager;
import android.content.Context;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Color;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewTreeObserver;
import android.widget.HorizontalScrollView;
import android.widget.ScrollView;

/** Shared navigation helpers for Android phone/tablet, Android TV and Fire TV. */
public final class RemoteUi {
    private RemoteUi() {}

    public static boolean isTelevision(Context context) {
        UiModeManager ui = (UiModeManager) context.getSystemService(Context.UI_MODE_SERVICE);
        if (ui != null && ui.getCurrentModeType() == Configuration.UI_MODE_TYPE_TELEVISION) return true;
        PackageManager pm = context.getPackageManager();
        return pm.hasSystemFeature(PackageManager.FEATURE_LEANBACK)
            || pm.hasSystemFeature("amazon.hardware.fire_tv");
    }

    public static void focusFirst(View root) {
        if (root == null) return;
        root.getViewTreeObserver().addOnGlobalLayoutListener(new ViewTreeObserver.OnGlobalLayoutListener() {
            @Override public void onGlobalLayout() {
                root.getViewTreeObserver().removeOnGlobalLayoutListener(this);
                View first = findFocusable(root);
                if (first != null) first.requestFocus();
            }
        });
    }

    private static View findFocusable(View view) {
        if (view.getVisibility() != View.VISIBLE || !view.isEnabled()) return null;
        if (view.isFocusable() && view.isClickable()) return view;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = 0; i < group.getChildCount(); i++) {
                View found = findFocusable(group.getChildAt(i));
                if (found != null) return found;
            }
        }
        return null;
    }

    public static void cardFocus(View view, boolean focused) {
        view.setScaleX(focused ? 1.055f : 1f);
        view.setScaleY(focused ? 1.055f : 1f);
        view.setElevation(focused ? Ui.dp(view.getContext(), 12) : 0);
        if (focused) view.setBackground(Ui.round(Color.rgb(33, 38, 49), 12));
        else view.setBackground(Ui.round(Color.TRANSPARENT, 12));
        if (focused) keepVisible(view);
    }

    public static void keepVisible(View view) {
        View parent = (View) view.getParent();
        while (parent != null) {
            if (parent instanceof HorizontalScrollView) {
                HorizontalScrollView horizontal = (HorizontalScrollView) parent;
                horizontal.smoothScrollTo(Math.max(0, view.getLeft() - Ui.dp(view.getContext(), 40)), 0);
            } else if (parent instanceof ScrollView) {
                ScrollView vertical = (ScrollView) parent;
                int top = view.getTop();
                View p = (View) view.getParent();
                while (p != null && p != vertical) { top += p.getTop(); p = (View) p.getParent(); }
                vertical.smoothScrollTo(0, Math.max(0, top - Ui.dp(view.getContext(), 60)));
            }
            if (!(parent.getParent() instanceof View)) break;
            parent = (View) parent.getParent();
        }
    }
}
