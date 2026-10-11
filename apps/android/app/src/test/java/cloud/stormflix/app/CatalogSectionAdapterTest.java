package cloud.stormflix.app;

import android.app.Activity;
import android.app.Application;
import android.view.View;
import android.view.ViewGroup;
import android.widget.ImageView;
import androidx.recyclerview.widget.LinearLayoutManager;
import androidx.recyclerview.widget.RecyclerView;
import java.util.ArrayList;
import java.util.List;
import org.json.JSONObject;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.robolectric.Robolectric;
import org.robolectric.RobolectricTestRunner;
import org.robolectric.annotation.Config;
import static org.junit.Assert.*;

@RunWith(RobolectricTestRunner.class)
@Config(sdk = 28, application = Application.class)
public class CatalogSectionAdapterTest {
    private List<CategoryBrowseActivity.Section> catalog(int rows, int cards) throws Exception {
        List<CategoryBrowseActivity.Section> result = new ArrayList<>();
        for (int r = 0; r < rows; r++) {
            List<CategoryBrowseActivity.CardItem> items = new ArrayList<>();
            for (int c = 0; c < cards; c++) {
                String key = r + ":" + c;
                items.add(new CategoryBrowseActivity.CardItem(new JSONObject().put("title", key), false, key));
            }
            result.add(new CategoryBrowseActivity.Section("Genre " + r, items));
        }
        return result;
    }

    private void layout(View view) {
        view.measure(View.MeasureSpec.makeMeasureSpec(1080, View.MeasureSpec.EXACTLY),
                     View.MeasureSpec.makeMeasureSpec(720, View.MeasureSpec.EXACTLY));
        view.layout(0, 0, 1080, 720);
    }

    private int posters(View view) {
        int count = view instanceof ImageView ? 1 : 0;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = 0; i < group.getChildCount(); i++) count += posters(group.getChildAt(i));
        }
        return count;
    }

    @Test public void largeCatalogOnlyAttachesNearbyPosters() throws Exception {
        try (var controller = Robolectric.buildActivity(Activity.class).setup()) {
            Activity activity = controller.get();
            RecyclerView list = new RecyclerView(activity);
            LinearLayoutManager manager = new LinearLayoutManager(activity);
            list.setLayoutManager(manager);
            list.setItemAnimator(null);
            list.setAdapter(new CatalogSectionAdapter(activity, catalog(50, 100), new ImageLoader(activity), item -> {}));
            activity.setContentView(list);
            layout(list);
            assertTrue(posters(list) > 0);
            assertTrue("5000 cards must not create 5000 poster views", posters(list) < 100);
            manager.scrollToPositionWithOffset(40, 0);
            layout(list);
            assertEquals(40, manager.findFirstVisibleItemPosition());
            assertTrue(posters(list) > 0 && posters(list) < 100);
        }
    }

    @Test public void recycledRowRestoresPositionAndUsesCurrentClickTarget() throws Exception {
        try (var controller = Robolectric.buildActivity(Activity.class).setup()) {
            Activity activity = controller.get();
            List<CategoryBrowseActivity.Section> sections = catalog(2, 100);
            String[] selected = { null };
            CatalogSectionAdapter adapter = new CatalogSectionAdapter(activity, sections, new ImageLoader(activity), item -> selected[0] = item.key);
            RecyclerView parent = new RecyclerView(activity);
            CatalogSectionAdapter.SectionHolder holder = adapter.createViewHolder(parent, 0);
            activity.setContentView(holder.itemView);
            adapter.bindViewHolder(holder, 0);
            layout(holder.itemView);
            holder.layout.scrollToPositionWithOffset(30, 0);
            layout(holder.itemView);
            assertEquals(30, holder.layout.findFirstVisibleItemPosition());
            adapter.onViewRecycled(holder);
            adapter.onBindViewHolder(holder, 1);
            layout(holder.itemView);
            assertEquals(0, holder.layout.findFirstVisibleItemPosition());
            holder.layout.findViewByPosition(0).performClick();
            assertEquals("1:0", selected[0]);
            adapter.onViewRecycled(holder);
            adapter.onBindViewHolder(holder, 0);
            layout(holder.itemView);
            assertEquals(30, holder.layout.findFirstVisibleItemPosition());
            holder.layout.findViewByPosition(30).performClick();
            assertEquals("0:30", selected[0]);
        }
    }
}
