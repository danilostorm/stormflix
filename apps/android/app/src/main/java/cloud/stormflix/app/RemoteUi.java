package cloud.stormflix.app;

import android.app.UiModeManager;
import android.content.Context;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Color;
import android.graphics.Rect;
import android.text.TextUtils;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewTreeObserver;
import android.widget.HorizontalScrollView;
import android.widget.ScrollView;
import android.widget.TextView;

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
        // Large scale transforms were being clipped by the row holder on Fire
        // TV. Keep the visual focus subtle and use border/background/elevation.
        view.setScaleX(focused ? 1.018f : 1f);
        view.setScaleY(focused ? 1.018f : 1f);
        view.setElevation(focused ? Ui.dp(view.getContext(), 12) : 0);
        if (focused) view.setBackground(Ui.round(Color.rgb(33, 38, 49), 12));
        else view.setBackground(Ui.round(Color.TRANSPARENT, 12));

        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            boolean titleAdjusted = false;
            for (int i = 0; i < group.getChildCount(); i++) {
                View child = group.getChildAt(i);
                if (child instanceof TextView) {
                    TextView text = (TextView) child;
                    if (!titleAdjusted) {
                        text.setMaxLines(focused ? 2 : 1);
                        text.setEllipsize(TextUtils.TruncateAt.END);
                        titleAdjusted = true;
                    }
                }
            }
        }
        if (focused) keepVisible(view);
    }

    public static void keepVisible(View view) {
        if (view == null) return;
        View parent = view;
        while (parent != null) {
            if (parent instanceof HorizontalScrollView) {
                HorizontalScrollView horizontal = (HorizontalScrollView) parent;
                Rect rect = new Rect(0, 0, view.getWidth(), view.getHeight());
                horizontal.offsetDescendantRectToMyCoords(view, rect);
                int viewportLeft = horizontal.getScrollX() + horizontal.getPaddingLeft();
                int viewportRight = horizontal.getScrollX() + horizontal.getWidth() - horizontal.getPaddingRight();
                int safety = Ui.dp(view.getContext(), 42);
                int target = horizontal.getScrollX();
                if (rect.right + safety > viewportRight) {
                    target += rect.right + safety - viewportRight;
                } else if (rect.left - safety < viewportLeft) {
                    target -= viewportLeft - (rect.left - safety);
                }
                int max = Math.max(0, horizontal.getChildAt(0) == null ? 0 : horizontal.getChildAt(0).getWidth() - horizontal.getWidth());
                horizontal.smoothScrollTo(Math.max(0, Math.min(target, max)), 0);
            } else if (parent instanceof ScrollView) {
                ScrollView vertical = (ScrollView) parent;
                Rect rect = new Rect(0, 0, view.getWidth(), view.getHeight());
                vertical.offsetDescendantRectToMyCoords(view, rect);
                int top = vertical.getScrollY() + vertical.getPaddingTop();
                int bottom = vertical.getScrollY() + vertical.getHeight() - vertical.getPaddingBottom();
                int safety = Ui.dp(view.getContext(), 56);
                int target = vertical.getScrollY();
                if (rect.bottom + safety > bottom) target += rect.bottom + safety - bottom;
                else if (rect.top - safety < top) target -= top - (rect.top - safety);
                vertical.smoothScrollTo(0, Math.max(0, target));
            }
            if (!(parent.getParent() instanceof View)) break;
            parent = (View) parent.getParent();
        }
    }
}
