# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix should read this file and `AGENTS.md` before changing code. Update this document after meaningful changes.

Last architecture update: **2026-08-27**.

## Deployment

Primary deployment is an Unraid checkout, typically:

```bash
cd /mnt/user/appdata/stormflix
git pull origin main
docker compose down
docker compose up -d --build
curl -s http://127.0.0.1:8090/healthz
echo
```

Server HTTP port: **8090** behind the user's HTTPS reverse proxy. Public StormFlix/Jellyfin-compatible address is the normal HTTPS domain, without Jellyfin ports 8096/8920.

## Current versions

- Server version constant: `0.16.1-player-state-jellyfin`.
- Native Android application: `cloud.stormflix.app`, version **0.2.3**, versionCode 10, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- SQLite is the supported database. It uses WAL, synchronous=NORMAL, busy timeout, bounded connection pool and targeted indexes.

## Non-negotiable playback behavior

- **Direct Play first.** Do not silently transcode video.
- Android/Fire receives the original multi-audio source first so Media3 can apply profile language preference.
- Preferred audio order understands `pt-BR → pt → por` and labels such as Português, Dublado and Brasil.
- If the preferred audio codec is not supported, the client explicitly requests compatibility mode: video stays stream-copy and only audio is converted to AAC-LC.
- Compatibility AAC is prepared as a seekable cached MP4 and served with range support; do not regress to non-seekable fragmented pipe behavior.
- Ordered profile progress uses playback session + sequence/event ordering so stale writes cannot overwrite a newer position while legitimate backward seeks remain valid.

## Jellyfin compatibility facade

Compatibility is isolated from native `/api/v1`. It is exposed both at root Jellyfin paths and `/jellyfin-api`.

Important established compatibility behavior:

- `/System/Info/Public` advertises `ProductName: Jellyfin Server` and numeric Jellyfin-compatible version `10.11.6`.
- Official mobile WebView startup is supported through the StormFlix web bundle bridge.
- Android TV/Fire authentication flow, `/Users/Me`, display preferences, capabilities and modern Home routes are implemented.
- Modern aliases include `/UserViews`, `/UserItems/Resume`, `/Items/Latest`, `/Items` and compatibility routes retained for older clients.
- `/ClientLog/Document` is accepted so Android TV crash reports can be stored in StormFlix logs.
- Jellyfin artwork endpoints may be read without native StormFlix session where the official image loader requires it; local artwork is served directly instead of redirecting to protected `/assets`.
- `UserData` must never be null in Jellyfin `BaseItemDto`; Android TV 0.19.10 crashed when it was missing.
- Current Jellyfin Android mobile connection/login works. Android TV/Fire login and library discovery have progressed substantially; continue compatibility testing after catalog metadata is clean.

## Library and episodic scanner architecture

Library kinds include:

- `movies`
- `series`
- `anime`
- `mixed`
- `anime_series` = Séries + Anime (temporadas)
- `animation_series` = Desenhos / Séries de animação
- music and other existing special kinds

For episodic libraries, **scanner identity is authoritative before metadata providers**.

`media_series_identity` persists:

- library/source root
- stable `series_key`
- scanner/canonical `series_title`
- season number
- episode number
- absolute number

The scanner uses configured library roots and folder hierarchy. For example:

```text
Desenhos/
└── Pica-Pau e seus Amigos/
    └── Remux/
        └── 002PP-BD1080pRemux.mkv
```

becomes `Pica-Pau e seus Amigos`, season 1, episode 2. Technical folders such as Remux, BluRay, 1080p, Disc/Volume folders are not shows. Brazilian season folders such as `5ª Temporada - Stormbrasil` are recognized.

Metadata providers enrich the scanner identity; providers must not turn every release filename into an independent show.

## Metadata providers

### Western series / cartoons

Preferred chain:

`Scanner de pasta → TMDB TV → TheTVDB v4 → Fanart.tv`

`animation_series` does not automatically enter AniList/MAL merely because it is animated.

### Anime with seasons

Preferred chain combines scanner identity with TMDB TV, TheTVDB, AniList/AniDB/MAL recovery and the HAMA-style mapping bridge.

### HAMA-style bridge

StormFlix does **not** embed the Plex Hama.bundle. It implements the useful mapping concept natively using community Anime-Lists data to bridge AniDB/AniList/MAL IDs with TVDB/TMDB IDs, seasons and offsets. The mapping is cached.

### TheTVDB

TheTVDB v4 client is implemented and optional. Configuration is available in Admin → Configurações → Metadados & Capas via API Key and optional Subscriber PIN. Without a TVDB key, TMDB/Fanart continue to operate.

## Manual matching — series level

Phase 10 introduces Plex-style logical series matching through `series_metadata_overrides`.

For an episodic item with a scanner-owned `series_key`, the Admin Catalog offers **Aplicar à série inteira (recomendado)**. One TMDB TV choice:

1. stores the provider decision for `(library_id, series_key)`;
2. protects the logical show rather than requiring every episode to be matched manually;
3. refreshes current episodes in the background with the selected TMDB series and their scanner season/episode numbers;
4. keeps the canonical show title across rescans/new episodes while preserving scanner-owned numbering.

The old item-level/manual-copy flow remains available for movies and exceptional cases.

## Categories and subcategories

Phase 10 adds `parent_id` to `library_categories`.

- System categories Filmes, Séries and Animes remain top-level roots.
- Custom categories can be top-level or children of another category.
- Parent browsing aggregates libraries from active descendants.
- Child browsing stays scoped to that branch.
- The client top navigation shows roots; subcategories appear in a secondary horizontal navigation instead of crowding the main menu.
- Admin has a hierarchical category manager with parent selector and library assignment.

Recommended organization examples:

```text
Filmes
├── 4K / UHD
├── Animação
├── Clássicos
└── Outros

Séries
├── Desenhos
├── Novelas
└── TV

Animes
├── Dublados
├── Legendados
└── Filmes de Anime
```

## Home / SQLite performance

The database remains SQLite; the slowdown observed with a larger catalog was primarily repeated read work, not proof that SQLite must be replaced.

Performance work in phase 10:

- indexes for selected artwork lookup, global recent ordering, title ordering, rating/status and series identity/override lookup;
- grouped/static Home feed cache with a short **20 second TTL**;
- cache key includes the exact library access scope; `nil = all libraries` is deliberately distinct from `[] = no libraries`;
- profile-sensitive Continue Watching is never put in the static cache;
- Home static feed, Continue Watching, two trending windows and Releases are read concurrently using SQLite WAL readers;
- cached feeds are deep-copied before request-specific mutation.

If Home later becomes slow again, profile with SQL timing before replacing SQLite. The next optimization target would be materialized/denormalized Home rows or event-driven cache invalidation, not an immediate database migration.

## Schema history relevant to current work

- Phase 9: scanner-owned `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, category/series/artwork/home indexes.

## Current admin behavior

Catalog manual matching now exposes scanner series identity and distinguishes:

- automatic item;
- item-level manual match;
- **series-level manual protected match**.

For series, default manual search uses the scanner/canonical series title instead of the release filename.

## Known pending / next work

- Validate real-world series-level manual match against the user's problematic cartoons and dubbed anime after deployment.
- Continue cleaning metadata edge cases where provider episode ordering differs (air/DVD/absolute).
- Continue Jellyfin Android TV/Fire validation after the native catalog is correct; do not diagnose Jellyfin using corrupted native metadata.
- Consider explicit series-level provider selector for TVDB as a manual match source, not only TMDB, if the user needs TVDB ordering for a specific show.
- If Home timing remains high after phase10, add server-side timing metrics per Home rail and inspect query plans before changing database engines.

## Documentation rule

After every meaningful update, append the user-visible change to `CHANGELOG.md` and refresh this file. Do not store secrets in either document.
