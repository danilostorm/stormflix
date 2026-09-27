package cloud.stormflix.app;

import android.app.Application;
import com.bumptech.glide.Glide;
import com.bumptech.glide.GlideBuilder;
import com.bumptech.glide.load.engine.cache.InternalCacheDiskCacheFactory;
import com.bumptech.glide.load.engine.cache.LruResourceCache;
import com.bumptech.glide.load.engine.bitmap_recycle.LruBitmapPool;

public final class StormFlixApplication extends Application {
    @Override public void onCreate() {
        super.onCreate();
        long memory = Math.min(24L * 1024 * 1024, Runtime.getRuntime().maxMemory() / 8);
        Glide.init(this, new GlideBuilder()
            .setMemoryCache(new LruResourceCache(memory))
            .setBitmapPool(new LruBitmapPool(memory / 2))
            .setDiskCache(new InternalCacheDiskCacheFactory(this, "artwork-v1", 64L * 1024 * 1024)));
    }
}
