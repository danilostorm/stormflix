package cloud.stormflix.app;

import java.text.Normalizer;
import java.util.HashMap;
import java.util.Locale;
import java.util.Map;

/** Genre vocabulary shared with category-nav.js. No catalog access decisions here. */
final class CatalogGenres {
    private static final Map<String, String> TITLES = new HashMap<>();
    static {
        TITLES.put("action", "Ação");
        TITLES.put("acao", "Ação");
        TITLES.put("adventure", "Aventura");
        TITLES.put("aventura", "Aventura");
        TITLES.put("animation", "Animação");
        TITLES.put("animacao", "Animação");
        TITLES.put("comedy", "Comédia");
        TITLES.put("comedia", "Comédia");
        TITLES.put("crime", "Crime");
        TITLES.put("documentary", "Documentários");
        TITLES.put("documentario", "Documentários");
        TITLES.put("documentarios", "Documentários");
        TITLES.put("drama", "Drama");
        TITLES.put("family", "Família");
        TITLES.put("familia", "Família");
        TITLES.put("fantasy", "Fantasia");
        TITLES.put("fantasia", "Fantasia");
        TITLES.put("history", "História");
        TITLES.put("historia", "História");
        TITLES.put("horror", "Terror");
        TITLES.put("terror", "Terror");
        TITLES.put("music", "Música");
        TITLES.put("musica", "Música");
        TITLES.put("mystery", "Mistério");
        TITLES.put("misterio", "Mistério");
        TITLES.put("romance", "Romance");
        TITLES.put("science-fiction", "Ficção científica");
        TITLES.put("sci-fi", "Ficção científica");
        TITLES.put("ficcao-cientifica", "Ficção científica");
        TITLES.put("thriller", "Suspense");
        TITLES.put("suspense", "Suspense");
        TITLES.put("war", "Guerra");
        TITLES.put("guerra", "Guerra");
        TITLES.put("western", "Faroeste");
        TITLES.put("faroeste", "Faroeste");
        TITLES.put("action-adventure", "Ação e aventura");
        TITLES.put("acao-e-aventura", "Ação e aventura");
        TITLES.put("kids", "Infantil");
        TITLES.put("infantil", "Infantil");
        TITLES.put("reality", "Reality show");
        TITLES.put("reality-show", "Reality show");
        TITLES.put("sci-fi-fantasy", "Ficção científica e fantasia");
        TITLES.put("science-fiction-fantasy", "Ficção científica e fantasia");
        TITLES.put("ficcao-cientifica-e-fantasia", "Ficção científica e fantasia");
        TITLES.put("ficcao-cientifica-fantasia", "Ficção científica e fantasia");
        TITLES.put("soap", "Novelas");
        TITLES.put("novela", "Novelas");
        TITLES.put("novelas", "Novelas");
        TITLES.put("talk", "Programas de entrevista");
        TITLES.put("talk-show", "Programas de entrevista");
        TITLES.put("talk-shows", "Programas de entrevista");
        TITLES.put("programa-de-entrevista", "Programas de entrevista");
        TITLES.put("programas-de-entrevista", "Programas de entrevista");
        TITLES.put("war-politics", "Guerra e política");
        TITLES.put("guerra-politica", "Guerra e política");
        TITLES.put("guerra-e-politica", "Guerra e política");
        TITLES.put("tv-movie", "Filmes para TV");
        TITLES.put("television-movie", "Filmes para TV");
        TITLES.put("filme-para-tv", "Filmes para TV");
        TITLES.put("filmes-para-tv", "Filmes para TV");
        TITLES.put("news", "Notícias");
        TITLES.put("noticia", "Notícias");
        TITLES.put("noticias", "Notícias");
        TITLES.put("psychological", "Psicológico");
        TITLES.put("psicologico", "Psicológico");
        TITLES.put("slice-of-life", "Cotidiano");
        TITLES.put("cotidiano", "Cotidiano");
        TITLES.put("sports", "Esportes");
        TITLES.put("sport", "Esportes");
        TITLES.put("esporte", "Esportes");
        TITLES.put("esportes", "Esportes");
        TITLES.put("supernatural", "Sobrenatural");
        TITLES.put("sobrenatural", "Sobrenatural");
        TITLES.put("mahou-shoujo", "Garotas mágicas");
        TITLES.put("magical-girl", "Garotas mágicas");
        TITLES.put("magical-girls", "Garotas mágicas");
        TITLES.put("garotas-magicas", "Garotas mágicas");
        TITLES.put("mecha", "Mecha");
        TITLES.put("martial-arts", "Artes marciais");
        TITLES.put("artes-marciais", "Artes marciais");
    }
    static String title(String value) {
        String key = Normalizer.normalize(value, Normalizer.Form.NFD)
            .replaceAll("\\p{M}+", "").toLowerCase(Locale.ROOT)
            .replaceAll("[^a-z0-9]+", "-").replaceAll("^-+|-+$", "");
        return TITLES.get(key);
    }
}
