package cloud.stormflix.app;

import android.content.Context;
import android.graphics.Color;
import android.text.TextUtils;
import android.view.ViewGroup;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;
import androidx.recyclerview.widget.LinearLayoutManager;
import androidx.recyclerview.widget.RecyclerView;
import java.util.List;
import org.json.JSONObject;

/** Both axes recycle views: catalog size does not determine live poster count. */
final class CatalogSectionAdapter extends RecyclerView.Adapter<CatalogSectionAdapter.SectionHolder> {
    interface OnSelect { void open(CategoryBrowseActivity.CardItem item); }
    private final Context context;
    private final List<CategoryBrowseActivity.Section> sections;
    private final ImageLoader images;
    private final OnSelect select;
    private final RecyclerView.RecycledViewPool posters = new RecyclerView.RecycledViewPool();

    CatalogSectionAdapter(Context context, List<CategoryBrowseActivity.Section> sections,
                          ImageLoader images, OnSelect select) {
        this.context = context;
        this.sections = sections;
        this.images = images;
        this.select = select;
        posters.setMaxRecycledViews(0, 16);
    }

    @Override public SectionHolder onCreateViewHolder(ViewGroup parent, int type) {
        LinearLayout row = Ui.vertical(context, 0);
        row.setLayoutParams(new RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(context, 344)));
        TextView title = Ui.title(context, "", 21);
        title.setMaxLines(1);
        title.setEllipsize(TextUtils.TruncateAt.END);
        row.addView(title, Ui.margin(context, ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(context, 32), 0, 12, 0, 4));
        RecyclerView rail = new RecyclerView(context);
        LinearLayoutManager layout = new LinearLayoutManager(context, RecyclerView.HORIZONTAL, false);
        layout.setInitialPrefetchItemCount(2);
        rail.setLayoutManager(layout);
        rail.setHasFixedSize(true);
        rail.setItemAnimator(null);
        rail.setItemViewCacheSize(2);
        rail.setRecycledViewPool(posters);
        rail.setNestedScrollingEnabled(false);
        rail.setClipToPadding(false);
        rail.setClipChildren(false);
        rail.setPadding(0, Ui.dp(context, 8), 0, Ui.dp(context, 8));
        row.addView(rail, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(context, 292)));
        return new SectionHolder(row, title, rail, layout);
    }

    @Override public void onBindViewHolder(SectionHolder holder, int position) {
        holder.savePosition();
        holder.section = sections.get(position);
        holder.title.setText(holder.section.title);
        // Detach the previous adapter so its image requests are cleared before
        // this row is reused for another genre. All rows share compatible holders.
        holder.rail.setAdapter(new Cards(holder.section.items));
        if (holder.section.scrollState != null) holder.layout.onRestoreInstanceState(holder.section.scrollState);
        else holder.layout.scrollToPositionWithOffset(0, 0);
    }

    @Override public void onViewRecycled(SectionHolder holder) {
        holder.savePosition();
        holder.rail.setAdapter(null);
        holder.section = null;
    }

    @Override public int getItemCount() { return sections.size(); }

    static final class SectionHolder extends RecyclerView.ViewHolder {
        final TextView title;
        final RecyclerView rail;
        final LinearLayoutManager layout;
        CategoryBrowseActivity.Section section;
        SectionHolder(LinearLayout row, TextView title, RecyclerView rail, LinearLayoutManager layout) {
            super(row); this.title = title; this.rail = rail; this.layout = layout;
        }
        void savePosition() {
            if (section != null) section.scrollState = layout.onSaveInstanceState();
        }
    }

    private final class Cards extends RecyclerView.Adapter<PosterHolder> {
        private final List<CategoryBrowseActivity.CardItem> items;
        Cards(List<CategoryBrowseActivity.CardItem> items) { this.items = items; }
        @Override public PosterHolder onCreateViewHolder(ViewGroup parent, int type) {
            LinearLayout card = Ui.vertical(context, 0);
            card.setFocusable(true);
            card.setClickable(true);
            card.setOnFocusChangeListener((v, focused) -> RemoteUi.cardFocus(v, focused));
            RecyclerView.LayoutParams lp = new RecyclerView.LayoutParams(Ui.dp(context, 150), Ui.dp(context, 276));
            lp.rightMargin = Ui.dp(context, 10);
            card.setLayoutParams(lp);
            ImageView poster = new ImageView(context);
            poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
            poster.setBackground(Ui.round(Color.rgb(25, 29, 38), 8));
            card.addView(poster, new LinearLayout.LayoutParams(Ui.dp(context, 150), Ui.dp(context, 225)));
            TextView title = Ui.title(context, "", 12);
            title.setMaxLines(1);
            title.setEllipsize(TextUtils.TruncateAt.END);
            // Fixed text bounds prevent image/focus updates from resizing rails.
            card.addView(title, Ui.margin(context, Ui.dp(context, 150), Ui.dp(context, 30), 0, 4, 0, 0));
            TextView detail = Ui.muted(context, "", 10);
            detail.setMaxLines(1);
            card.addView(detail, new LinearLayout.LayoutParams(Ui.dp(context, 150), Ui.dp(context, 16)));
            return new PosterHolder(card, poster, title, detail);
        }
        @Override public void onBindViewHolder(PosterHolder holder, int position) {
            CategoryBrowseActivity.CardItem item = items.get(position);
            JSONObject raw = item.raw;
            holder.title.setText(raw.optString("title", item.series ? "Série" : "Título"));
            holder.detail.setText(item.series
                ? raw.optInt("season_count", 0) + " temp. · " + raw.optInt("episode_count", 0) + " ep."
                : (raw.optInt("year", 0) > 0 ? String.valueOf(raw.optInt("year")) : raw.optString("media_type", "")));
            holder.itemView.setContentDescription(holder.title.getText());
            holder.itemView.setOnClickListener(v -> select.open(item));
            RemoteUi.cardFocus(holder.itemView, holder.itemView.hasFocus());
            images.load(holder.poster, raw.optString("poster_url", ""));
        }
        @Override public void onViewRecycled(PosterHolder holder) {
            images.clear(holder.poster);
            holder.itemView.setOnClickListener(null);
            RemoteUi.cardFocus(holder.itemView, false);
        }
        @Override public int getItemCount() { return items.size(); }
    }

    private static final class PosterHolder extends RecyclerView.ViewHolder {
        final ImageView poster;
        final TextView title, detail;
        PosterHolder(LinearLayout card, ImageView poster, TextView title, TextView detail) {
            super(card); this.poster = poster; this.title = title; this.detail = detail;
        }
    }
}
