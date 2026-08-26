package cloud.stormflix.app;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;

public final class Models {
    private Models() {}

    public static final class Media {
        public long id;
        public String title = "";
        public String libraryName = "";
        public String mediaType = "";
        public String overview = "";
        public String extension = "";
        public String posterUrl = "";
        public String backdropUrl = "";
        public String logoUrl = "";
        public int year;
        public int runtimeMinutes;
        public double rating;
        public final List<String> genres = new ArrayList<>();

        public static Media from(JSONObject o) {
            Media m = new Media();
            m.id = o.optLong("id");
            m.title = o.optString("title", "");
            m.libraryName = o.optString("library_name", "");
            m.mediaType = o.optString("media_type", "");
            m.overview = o.optString("overview", "");
            m.extension = o.optString("extension", "");
            m.posterUrl = o.optString("poster_url", "");
            m.backdropUrl = o.optString("backdrop_url", "");
            m.logoUrl = o.optString("logo_url", "");
            m.year = o.optInt("year");
            m.runtimeMinutes = o.optInt("runtime_minutes");
            m.rating = o.optDouble("rating");
            JSONArray genres = o.optJSONArray("genres");
            if (genres != null) for (int i = 0; i < genres.length(); i++) m.genres.add(genres.optString(i));
            return m;
        }
    }

    public static final class Row {
        public String title = "";
        public final List<Media> items = new ArrayList<>();
    }

    public static final class Home {
        public Media hero;
        public final List<Row> rows = new ArrayList<>();
        public static Home from(String json) throws Exception {
            JSONObject o = new JSONObject(json);
            Home h = new Home();
            JSONObject hero = o.optJSONObject("hero");
            if (hero != null) h.hero = Media.from(hero);
            JSONArray rows = o.optJSONArray("rows");
            if (rows != null) {
                for (int i = 0; i < rows.length(); i++) {
                    JSONObject ro = rows.optJSONObject(i); if (ro == null) continue;
                    Row r = new Row(); r.title = ro.optString("title", "");
                    JSONArray items = ro.optJSONArray("items");
                    if (items != null) for (int j = 0; j < items.length(); j++) {
                        JSONObject item = items.optJSONObject(j); if (item != null) r.items.add(Media.from(item));
                    }
                    h.rows.add(r);
                }
            }
            return h;
        }
    }

    public static final class Profile {
        public long id;
        public String name = "";
        public String avatarUrl = "";
        public boolean kids;
        public boolean pinEnabled;
        public static Profile from(JSONObject o) {
            Profile p = new Profile();
            p.id = o.optLong("id");
            p.name = o.optString("name", "Perfil");
            p.avatarUrl = o.optString("avatar_url", "");
            p.kids = o.optBoolean("is_kids", false);
            p.pinEnabled = o.optBoolean("pin_enabled", false);
            return p;
        }
    }
}
