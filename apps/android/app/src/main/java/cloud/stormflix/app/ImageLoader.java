package cloud.stormflix.app;

import android.content.Context;
import android.net.Uri;
import android.widget.ImageView;
import com.bumptech.glide.Glide;
import com.bumptech.glide.load.engine.DiskCacheStrategy;
import com.bumptech.glide.load.model.GlideUrl;
import com.bumptech.glide.load.model.LazyHeaders;
import com.bumptech.glide.signature.ObjectKey;

/** Shared, bounded, lifecycle-aware cache. Decode to the view's measured size. */
public final class ImageLoader {
    private final ApiClient api;
    public ImageLoader(Context context) { api = new ApiClient(context); }

    public void load(ImageView view, String url) {
        String resolved = api.absoluteAssetUrl(url);
        if (resolved.isEmpty()) { clear(view); return; }
        if (api.sameOrigin(resolved) && Uri.parse(resolved).getPath().startsWith("/assets/")) {
            int width = view.getLayoutParams() == null ? 0 : view.getLayoutParams().width;
            if (width <= 0) width = view.getResources().getDisplayMetrics().widthPixels;
            int variant = width <= 240 ? 240 : width <= 360 ? 360 : width <= 500 ? 500 : width <= 780 ? 780 : 1280;
            resolved = Uri.parse(resolved).buildUpon().appendQueryParameter("w", String.valueOf(variant))
                .appendQueryParameter("format", "auto").build().toString();
        }
        LazyHeaders.Builder headers = new LazyHeaders.Builder();
        if (api.sameOrigin(resolved)) headers.addHeader("Cookie", api.store().cookieHeader());
        Glide.with(view).load(new GlideUrl(resolved, headers.build()))
            .signature(new ObjectKey(api.store().cacheScope()))
            .diskCacheStrategy(DiskCacheStrategy.DATA).timeout(15000)
            .dontAnimate().into(view);
    }

    public void clear(ImageView view) {
        Glide.with(view).clear(view);
        view.setImageDrawable(null);
    }
}
