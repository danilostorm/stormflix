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

- Server version constant: `0.17.0-playback-core`.
- Native Android application: `cloud.stormflix.app`, version **0.2.3**, versionCode 10, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- SQLite is the supported database. It uses WAL, synchronous=NORMAL, busy timeout, bounded connection pool and targeted indexes.

## Non-negotiable playback behavior

- **Direct Play first.** Do not silently transcode video.
- Android/Fire receives the original multi-audio source first so Media3 can apply profile language preference.
- Preferred audio order understands `pt-BR → pt → por` and labels such as Português, Dublado and Brasil.
- If the preferred audio codec is not supported, the client explicitly requests compatibility mode: video stays stream-copy and only audio is converted to AAC-LC.
- Compatibility AAC is prepared as a seekable cached MP4 and served with range support; do not regress to non-seekable fragmented pipe behavior.
- Ordered profile progress uses playback session + sequence/event ordering so stale writes cannot overwrite a newer position while legitimate backward seeks remain valid.

## Unified native playback architecture

The playback architecture is now being centralized for StormFlix Web, Android and TV/Fire. `docs/PLAYBACK_ARCHITECTURE.md` is the detailed design record.

The native architecture is:

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 ──> Playback Core ──> Media
StormFlix TV/Fire ───┘        │
                              ├── Profiles / progress
                              ├── Library access
                              └── Metadata / subtitles

Jellyfin clients ──> Jellyfin compatibility facade ──> StormFlix core
```

`internal/playback` is the first shared Playback Core foundation. It owns source probing, client capability contracts and deterministic source decisions. Current native modes are:

- `direct_play` — original source, no re-encode;
- `remux` — container compatibility while preserving streams where the existing execution path allows it;
- `audio_compatibility` — video remains stream-copy and audio becomes AAC;
- `unsupported` — explicit refusal when the advertised client capabilities cannot consume the video without video transcoding.

`POST /api/v1/media/{id}/playback/plan` is the native planning endpoint. It checks media/library/profile access, reads the selected profile audio preference and resume position, probes the source, applies client capabilities, and returns the execution URL plus a new playback session ID. Native clients must use this API rather than depending on Jellyfin playback endpoints.

### StormFlix Web migration

The browser now loads `playback-core.js` after the legacy web compatibility adapter and before player decoration/monitoring. The new controller:

- derives browser container/codec support using runtime `canPlayType` feature detection rather than relying only on User-Agent;
- requests a native Playback Plan before loading a source;
- preserves the existing seekable cached remux/AAC materialization as the execution adapter;
- refuses unsupported video instead of silently adding video transcoding;
- installs Media Session actions;
- keeps source switches under the same planner path;
- invalidates in-flight planning when the player closes so a late response cannot start hidden playback.

Web monitoring now sends the server-issued playback session, monotonic progress sequence and event timestamp. This means the browser finally exercises the ordered profile-progress protection already implemented on the server.

The old `/media/{id}/compatibility` and `/remux` behavior remains available during migration, including manual AAC controls. It is now an execution/compatibility adapter rather than the intended long-term owner of source-selection policy.

### Next playback migration work

- Move Android Media3 source selection to the same native Playback Plan while continuing to publish real MediaCodec capabilities.
- Move Android TV/Fire playback to the same plan contract while keeping TV-specific D-pad/focus/player UI.
- Continue consolidating remaining legacy browser wrappers into the unified web controller once parity is proven.
- Expand capability contracts for HDR/Dolby Vision, maximum decode resolution/frame-rate, passthrough, subtitle rendering and bandwidth policy.
- Make selected audio-stream execution explicit for every remux path so non-Portuguese profile preferences are guaranteed end-to-end instead of relying on the legacy webcompat stream picker.
- Any future video transcoding must be an explicit separate policy; it must not become a silent fallback.

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

## Manual matching — principal series only

The intended behavior is strictly Plex-style for episodic libraries.

Admin → Catálogo defaults to **Obras principais**. For `series`, `anime_series` and `animation_series`, `/api/v1/admin/catalog/works` groups every scanner-owned `series_key` into one principal card showing season/episode counts. Movies and other standalone items remain individual works.

For an episodic work:

1. the operator chooses **Corrigir obra principal** once;
2. the TMDB TV choice is stored in `series_metadata_overrides` for `(library_id, series_key)`;
3. `RebuildSeriesIdentities` runs immediately, preserving scanner-owned season/episode numbering while applying the approved canonical show title;
4. the episode refresh is inserted into `metadata_jobs` as `job_type=series_refresh` and becomes visible in **Admin → Fila & atividades**;
5. current episodes are refreshed from that one principal decision with live processed/matched/error counts;
6. future episodes discovered by later scans inherit the same series override automatically.

Individual episodes do **not** expose a normal manual-match action. Admin → Catálogo → **Arquivos / diagnóstico** can inspect files, but an episodic file only links back to its principal work.

Important semantic rule: `media_metadata.manual_match` is for standalone item-level manual matches. Series protection lives only in `series_metadata_overrides`; episodes refreshed from a protected series remain automatic children. Phase 11 clears legacy episode `manual_match=1` flags left by the earlier implementation.

## Persistent scan queue and Admin job tracking

Phase 12 introduces `scan_jobs` and extends `metadata_jobs` with job type/series/provider fields.

### Scan queue

- Clicking **Escanear agora** queues a library instead of starting an uncontrolled parallel scan.
- **Bibliotecas → Escanear todas** queues all active libraries.
- Scan jobs are processed in persistent FIFO order **one library at a time**. This is deliberate to avoid several rclone/FUSE sources and SQLite catalog writers competing at once.
- Duplicate active queue entries for the same library are avoided.
- A queued scan can be removed; a running scan can be cancelled through the same control.
- Running scan state uses one shared cancellable timeout context, so Admin cancellation always targets the actual scan.
- On server restart, unfinished scan jobs are safely returned to `queued` and resume from the queue.
- The existing mount-protection behavior remains: an unreachable source preserves its previous catalog instead of being treated as deletion.

### Fila & atividades

Admin has a dedicated **Fila & atividades** page backed by `GET /api/v1/admin/jobs`.

It merges operational visibility for:

- library scans;
- library metadata jobs;
- principal-series episode reorganization (`series_refresh`).

The UI shows running/queued/history status, progress, current message, matched/success count, errors and cancellation for scans. Bibliotecas also shows a compact queue strip with **Escanear todas** and **Ver fila completa**.

Principal-series refresh jobs persist enough information (`series_key`, `series_title`, `provider_id`) to resume after a server restart. Older full-library metadata jobs are not silently resumed after restart; they are marked failed with an instruction to restart them from the panel.

## Categories and subcategories — visible site layout

Phase 10 added `parent_id` to `library_categories`; phase 12 makes the hierarchy visibly useful in the main site.

- System categories Filmes, Séries and Animes remain top-level roots.
- Custom categories can be top-level or children of another category.
- Parent browsing aggregates libraries from active descendants.
- Child browsing stays scoped to that branch.
- The main site now has a visible **Explorar por categoria** area showing roots and child chips.
- Clicking a root with children renders **one content rail per subcategory**, instead of one large mixed rail.
- A sticky secondary navigation lets the user switch between child categories, the split subcategory view, or **Tudo em <raiz>**.
- Admin → Categorias contains **Organizar estrutura recomendada**, which creates/updates managed recommended child categories without deleting custom categories.
- Recommended automatic assignments are exclusive among sibling categories where possible, avoiding the same library appearing twice just because it matches two broad labels.

Current recommended layout:

```text
Filmes
├── 4K / UHD
├── Animação
└── Outros filmes

Séries
├── Séries de TV
├── Desenhos
└── Animes com temporadas

Animes
├── Dublados
├── Séries
└── Filmes
```

Operators can still create arbitrary custom categories/subcategories and assign libraries manually.

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

## Admin UI fixes

- Metadata & Capas music-agent decoration is guarded against concurrent duplicate rendering. The previous race could render two identical **Agentes de Música** panels.
- Catalog principal-work view is the default. File-level episodic matching is deliberately removed from the normal workflow.
- Bibliotecas exposes queue state directly on library cards and a global **Escanear todas** action.
- A dedicated **Fila & atividades** page provides live operational tracking.

## Schema history relevant to current work

- Phase 9: scanner-owned `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, category/series/artwork/home indexes.
- Phase 11: index for ordered series children and cleanup of legacy per-episode manual flags when a principal series override exists.
- Phase 12: persistent `scan_jobs`; metadata job type/series/provider fields used for observable principal-series refresh and restart recovery.

## Current admin behavior

Catalog distinguishes two views:

- **Obras principais** (default): one card per logical series plus standalone works; this is where manual matching happens.
- **Arquivos / diagnóstico**: low-level media rows; episodic rows do not offer manual matching and instead link back to the principal work.

For series, default manual search uses the scanner/canonical series title instead of the release filename. After a principal match, follow the episode reorganization in **Fila & atividades** instead of manually fixing episodes.

## Known pending / next work

- Continue the unified playback migration described above, starting with Android Media3 capability publication/Playback Plan consumption and then Android TV/Fire.
- Validate the new web Playback Plan against real H.264/AAC, H.264+DTS, HEVC/AV1 and multi-audio files on Chrome/Chromium, Firefox and Safari-capable devices; tune capability declarations only from observed/runtime support.
- Make exact selected-audio execution explicit through remux preparation instead of relying on the legacy webcompat picker for every profile language.
- Validate the new scan queue/scan-all workflow against the user's real rclone/Drive libraries and tune timeout/progress text if a specific mount behaves differently.
- Validate real-world principal-series matching against problematic cartoons and dubbed anime while watching `series_refresh` in the queue.
- Continue cleaning metadata edge cases where provider episode ordering differs (air/DVD/absolute). Consider explicit TVDB ordering/provider selection at principal-series level when needed.
- Continue Jellyfin Android TV/Fire validation after the native catalog is correct; do not diagnose Jellyfin using corrupted native metadata.
- If Home timing remains high after phase10, add server-side timing metrics per Home rail and inspect query plans before changing database engines.

## Documentation rule

After every meaningful update, append the user-visible change to `CHANGELOG.md` and refresh this file. Do not store secrets in either document.
