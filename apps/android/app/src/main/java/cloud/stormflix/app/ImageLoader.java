package cloud.stormflix.app;

import android.content.Context;
import android.graphics.Color;
import android.graphics.drawable.ColorDrawable;
import android.graphics.drawable.Drawable;
import android.net.Uri;
import android.widget.ImageView;
import com.bumptech.glide.Glide;
import com.bumptech.glide.RequestBuilder;
import com.bumptech.glide.load.engine.DiskCacheStrategy;
import com.bumptech.glide.load.model.GlideUrl;
import com.bumptech.glide.load.model.LazyHeaders;
import com.bumptech.glide.signature.ObjectKey;

/** Shared artwork cache, with an original-image retry when a thumbnail fails. */
public final class ImageLoader {
    private final ApiClient api;
    public ImageLoader(Context context) { api = new ApiClient(context); }

    public void load(ImageView view, String url) {
        String original = api.absoluteAssetUrl(url == null ? "" : url.trim());
        if (original.isEmpty()) { clear(view); return; }
        String resolved = original;
        Uri uri = Uri.parse(original);
        if (api.sameOrigin(original) && uri.getPath() != null && uri.getPath().startsWith("/assets/")) {
            int width = view.getLayoutParams() == null ? 0 : view.getLayoutParams().width;
            if (width <= 0) width = view.getResources().getDisplayMetrics().widthPixels;
            int variant = width <= 240 ? 240 : width <= 360 ? 360 : width <= 500 ? 500 : width <= 780 ? 780 : 1280;
            Uri.Builder base = uri.buildUpon().clearQuery();
            for (String key : uri.getQueryParameterNames()) {
                if (!key.equals("w") && !key.equals("format"))
                    for (String value : uri.getQueryParameters(key)) base.appendQueryParameter(key, value);
            }
            original = base.build().toString();
            resolved = base.appendQueryParameter("w", String.valueOf(variant))
                .appendQueryParameter("format", "webp").build().toString();
        }
        RequestBuilder<Drawable> request = request(view, resolved);
        if (!resolved.equals(original)) request = request.error(request(view, original));
        request.placeholder(new ColorDrawable(Color.rgb(24, 29, 39))).into(view);
    }

    private RequestBuilder<Drawable> request(ImageView view, String url) {
        LazyHeaders.Builder headers = new LazyHeaders.Builder()
            .addHeader("Accept", "image/webp,image/jpeg,image/png,image/*;q=0.8");
        if (api.sameOrigin(url)) headers.addHeader("Cookie", api.store().cookieHeader());
        return Glide.with(view).load(new GlideUrl(url, headers.build()))
            .signature(new ObjectKey(api.store().cacheScope()))
            .diskCacheStrategy(DiskCacheStrategy.DATA).timeout(30000).dontAnimate();
    }

    public void clear(ImageView view) {
        Glide.with(view).clear(view);
        view.setImageDrawable(null);
    }
}
