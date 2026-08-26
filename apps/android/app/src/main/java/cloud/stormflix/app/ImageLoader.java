package cloud.stormflix.app;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.os.Handler;
import android.os.Looper;
import android.widget.ImageView;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class ImageLoader {
    private final ApiClient api;
    private final ExecutorService pool = Executors.newFixedThreadPool(4);
    private final Handler main = new Handler(Looper.getMainLooper());
    private final Map<String, Bitmap> cache = new LinkedHashMap<String, Bitmap>(80, 0.75f, true) {
        @Override protected boolean removeEldestEntry(Map.Entry<String, Bitmap> eldest) { return size() > 80; }
    };

    public ImageLoader(Context context) { api = new ApiClient(context); }

    public void load(ImageView view, String url) {
        String resolved = api.absoluteAssetUrl(url);
        if (resolved.isEmpty()) return;
        view.setTag(resolved);
        Bitmap cached;
        synchronized (cache) { cached = cache.get(resolved); }
        if (cached != null) { view.setImageBitmap(cached); return; }
        pool.submit(() -> {
            try {
                byte[] bytes = api.getBytes(resolved);
                Bitmap bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.length);
                if (bitmap == null) return;
                synchronized (cache) { cache.put(resolved, bitmap); }
                main.post(() -> {
                    if (resolved.equals(view.getTag())) view.setImageBitmap(bitmap);
                });
            } catch (Exception ignored) {}
        });
    }
}
