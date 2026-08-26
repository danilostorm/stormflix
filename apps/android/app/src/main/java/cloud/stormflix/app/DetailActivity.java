package cloud.stormflix.app;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.HorizontalScrollView;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class DetailActivity extends Activity {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private ApiClient api;
    private ImageLoader images;
    private LinearLayout content;
    private long mediaId;

    @Override protected void onCreate(Bundle state) {
        super.onCreate(state);
        mediaId = getIntent().getLongExtra("media_id", 0);
        api = new ApiClient(this); images = new ImageLoader(this);
        if (mediaId <= 0) { finish(); return; }
        ScrollView scroll = new ScrollView(this); scroll.setBackgroundColor(Ui.BG);
        content = Ui.vertical(this, 18); scroll.addView(content, new ScrollView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        content.addView(Ui.muted(this, "Carregando informações…", 14)); setContentView(scroll);
        load();
    }

    private void load() {
        io.submit(() -> {
            try { JSONObject detail = new JSONObject(api.get("/media/" + mediaId)); main.post(() -> render(detail)); }
            catch (Exception e) { main.post(() -> { Toast.makeText(this, e.getMessage(), Toast.LENGTH_LONG).show(); finish(); }); }
        });
    }

    private void render(JSONObject o) {
        Ui.clear(content);
        Models.Media media = Models.Media.from(o);
        Button back = Ui.button(this, "← Voltar", false); back.setOnClickListener(v -> finish()); content.addView(back, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this,44), 0,0,0,12));

        ImageView backdrop = new ImageView(this); backdrop.setScaleType(ImageView.ScaleType.CENTER_CROP); backdrop.setBackgroundColor(Color.rgb(16,19,25));
        content.addView(backdrop, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, Ui.dp(this, 360)));
        if (!media.backdropUrl.isEmpty()) images.load(backdrop, media.backdropUrl);

        LinearLayout body = Ui.vertical(this, 4);
        body.addView(Ui.title(this, media.title, 34));
        String tagline = o.optString("tagline", ""); if (!tagline.isEmpty()) body.addView(Ui.muted(this, tagline, 15));
        body.addView(Ui.muted(this, meta(media), 13), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,6,0,0));
        LinearLayout actions = Ui.horizontal(this, 0);
        Button play = Ui.button(this, "▶ Assistir", true); play.setOnClickListener(v -> play(media));
        actions.addView(play, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, Ui.dp(this,48),0,0,10,0));
        body.addView(actions, Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,12,0,0));
        if (!media.overview.isEmpty()) { TextView overview = Ui.muted(this, media.overview, 15); overview.setLineSpacing(0,1.15f); body.addView(overview, Ui.margin(this, Ui.dp(this,900), ViewGroup.LayoutParams.WRAP_CONTENT,0,16,0,0)); }

        JSONArray directors = o.optJSONArray("directors"); if (directors != null && directors.length()>0) body.addView(labelValue("Direção", join(directors)));
        if (!media.genres.isEmpty()) body.addView(labelValue("Gêneros", String.join(", ", media.genres)));
        if (!media.libraryName.isEmpty()) body.addView(labelValue("Biblioteca", media.libraryName));

        JSONArray cast = o.optJSONArray("cast");
        if (cast != null && cast.length()>0) {
            body.addView(Ui.title(this, "Elenco", 22), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,24,0,10));
            HorizontalScrollView hs = new HorizontalScrollView(this); hs.setHorizontalScrollBarEnabled(false); LinearLayout rail = Ui.horizontal(this,0);
            for (int i=0;i<cast.length();i++) { JSONObject p=cast.optJSONObject(i); if(p==null)continue; rail.addView(personCard(p)); }
            hs.addView(rail); body.addView(hs);
        }

        JSONArray related = o.optJSONArray("related");
        if (related != null && related.length()>0) {
            body.addView(Ui.title(this, "Você também pode gostar", 22), Ui.margin(this, ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,0,26,0,10));
            HorizontalScrollView hs = new HorizontalScrollView(this); hs.setHorizontalScrollBarEnabled(false); LinearLayout rail = Ui.horizontal(this,0);
            for(int i=0;i<related.length();i++){JSONObject r=related.optJSONObject(i);if(r==null)continue;rail.addView(relatedCard(Models.Media.from(r)));}
            hs.addView(rail); body.addView(hs);
        }
        content.addView(body);
    }

    private LinearLayout personCard(JSONObject p) {
        LinearLayout card = Ui.vertical(this,0); card.setGravity(Gravity.CENTER_HORIZONTAL); card.setFocusable(true); card.setBackground(Ui.round(Color.TRANSPARENT,10));
        card.setOnFocusChangeListener((v,f)->{v.setBackground(Ui.round(f?Color.rgb(31,35,45):Color.TRANSPARENT,10));v.setScaleX(f?1.06f:1);v.setScaleY(f?1.06f:1);});
        ImageView image=new ImageView(this); image.setScaleType(ImageView.ScaleType.CENTER_CROP); image.setBackground(Ui.round(Color.rgb(35,40,50),60)); card.addView(image,new LinearLayout.LayoutParams(Ui.dp(this,92),Ui.dp(this,92)));
        String url=p.optString("profile_url",""); if(!url.isEmpty())images.load(image,url);
        TextView n=Ui.title(this,p.optString("name",""),11);n.setMaxLines(1);n.setGravity(Gravity.CENTER);card.addView(n,Ui.margin(this,Ui.dp(this,110),ViewGroup.LayoutParams.WRAP_CONTENT,0,6,0,0));
        TextView c=Ui.muted(this,p.optString("character",""),9);c.setMaxLines(1);c.setGravity(Gravity.CENTER);card.addView(c);
        LinearLayout holder=Ui.horizontal(this,0);holder.addView(card,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,0,12,0));return holder;
    }

    private LinearLayout relatedCard(Models.Media media){
        LinearLayout card=Ui.vertical(this,0);card.setFocusable(true);card.setClickable(true);card.setOnClickListener(v->{Intent i=new Intent(this,DetailActivity.class);i.putExtra("media_id",media.id);startActivity(i);});
        card.setOnFocusChangeListener((v,f)->{v.setScaleX(f?1.05f:1);v.setScaleY(f?1.05f:1);});
        ImageView poster=new ImageView(this);poster.setScaleType(ImageView.ScaleType.CENTER_CROP);poster.setBackgroundColor(Color.rgb(24,28,36));card.addView(poster,new LinearLayout.LayoutParams(Ui.dp(this,135),Ui.dp(this,203)));if(!media.posterUrl.isEmpty())images.load(poster,media.posterUrl);
        TextView t=Ui.title(this,media.title,11);t.setMaxLines(1);card.addView(t,Ui.margin(this,Ui.dp(this,135),ViewGroup.LayoutParams.WRAP_CONTENT,0,6,0,0));
        LinearLayout holder=Ui.horizontal(this,0);holder.addView(card,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,0,0,10,0));return holder;
    }

    private LinearLayout labelValue(String label,String value){LinearLayout row=Ui.horizontal(this,0);TextView l=Ui.muted(this,label+":",11);TextView v=Ui.title(this,value,11);row.addView(l);row.addView(v,Ui.margin(this,ViewGroup.LayoutParams.WRAP_CONTENT,ViewGroup.LayoutParams.WRAP_CONTENT,8,0,0,0));return row;}
    private String join(JSONArray a){List<String> out=new ArrayList<>();for(int i=0;i<a.length();i++)out.add(a.optString(i));return String.join(", ",out);}
    private String meta(Models.Media m){List<String>s=new ArrayList<>();if(m.year>0)s.add(String.valueOf(m.year));if(m.rating>0)s.add("★ "+String.format(Locale.US,"%.1f",m.rating));if(m.runtimeMinutes>0)s.add(m.runtimeMinutes+" min");if(!m.extension.isEmpty())s.add(m.extension.replace(".","").toUpperCase(Locale.ROOT)+" · DIRECT PLAY");return String.join("  ·  ",s);}
    private void play(Models.Media m){Intent i=new Intent(this,PlayerActivity.class);i.putExtra("media_id",m.id);i.putExtra("title",m.title);startActivity(i);}
}
