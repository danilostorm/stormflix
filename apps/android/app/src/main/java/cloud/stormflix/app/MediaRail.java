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

/** Recycles off-screen artwork and preserves D-pad focus within each rail. */
final class MediaRail extends RecyclerView {
    interface OnSelect { void open(Models.Media media); }

    MediaRail(Context context, List<Models.Media> items, ImageLoader images, OnSelect select) {
        super(context);
        int width = Ui.dp(context, RemoteUi.isTelevision(context) ? 150 : 128);
        LinearLayoutManager layout = new LinearLayoutManager(context, HORIZONTAL, false);
        layout.setInitialPrefetchItemCount(2);
        setLayoutManager(layout);
        setItemViewCacheSize(3);
        setHasFixedSize(true);
        setNestedScrollingEnabled(false);
        setClipToPadding(false);
        setClipChildren(false);
        setPadding(0, Ui.dp(context, 8), 0, Ui.dp(context, 10));
        setAdapter(new Adapter<PosterHolder>() {
            @Override public PosterHolder onCreateViewHolder(ViewGroup parent, int type) {
                LinearLayout card = Ui.vertical(context, 0);
                card.setFocusable(true);
                card.setClickable(true);
                card.setOnFocusChangeListener((v, focused) -> RemoteUi.cardFocus(v, focused));
                LayoutParams lp = new LayoutParams(width, ViewGroup.LayoutParams.WRAP_CONTENT);
                lp.rightMargin = Ui.dp(context, 12);
                card.setLayoutParams(lp);
                ImageView poster = new ImageView(context);
                poster.setScaleType(ImageView.ScaleType.CENTER_CROP);
                poster.setBackground(Ui.round(Color.rgb(25, 29, 38), 8));
                card.addView(poster, new LinearLayout.LayoutParams(width, width * 3 / 2));
                TextView title = Ui.title(context, "", 12);
                title.setMaxLines(1);
                title.setEllipsize(TextUtils.TruncateAt.END);
                card.addView(title, Ui.margin(context, width, ViewGroup.LayoutParams.WRAP_CONTENT, 0, 7, 0, 0));
                return new PosterHolder(card, poster, title);
            }
            @Override public void onBindViewHolder(PosterHolder holder, int position) {
                Models.Media item = items.get(position);
                holder.title.setText(item.title);
                holder.itemView.setContentDescription(item.title);
                holder.itemView.setOnClickListener(v -> select.open(item));
                images.load(holder.poster, item.posterUrl);
            }
            @Override public void onViewRecycled(PosterHolder holder) {
                images.clear(holder.poster);
                RemoteUi.cardFocus(holder.itemView, false);
            }
            @Override public int getItemCount() { return items.size(); }
        });
    }

    private static final class PosterHolder extends ViewHolder {
        final ImageView poster;
        final TextView title;
        PosterHolder(LinearLayout card, ImageView poster, TextView title) {
            super(card);
            this.poster = poster;
            this.title = title;
        }
    }
}
