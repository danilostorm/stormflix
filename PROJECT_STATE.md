# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file and `AGENTS.md` before changing code. Update this document after meaningful changes.

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

Server HTTP port: **8090**, normally behind the user's HTTPS reverse proxy.

## Current versions

- Server version constant: `0.18.0-player-cache`.
- Native Android application: `cloud.stormflix.app`, version **0.3.0**, versionCode 11, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- SQLite remains the supported database with WAL, synchronous=NORMAL, busy timeout, bounded connection pool and targeted indexes.

## Non-negotiable playback rules

1. **Direct Play first. Never silently transcode video.**
2. Web, Android and TV/Fire use the same native `PlaybackPlan` policy under `/api/v1`.
3. Jellyfin compatibility remains isolated from native `/api/v1`; native StormFlix clients do not depend on Jellyfin endpoints.
4. Android/TV may keep the original multi-audio source when Media3 has a usable decoder and choose tracks locally.
5. Browser multi-audio selection may be pinned server-side because HTML audio-track APIs are inconsistent.
6. Preferred audio understands `pt-BR → pt → por`, including Português/Dublado/Brasil labels, while explicit profile languages remain authoritative.
7. If video is supported and only selected audio is incompatible, compatibility mode keeps **video stream-copy** and converts only audio to AAC-LC.
8. Compatibility MP4 output must remain seekable and HTTP Range/206 capable.
9. Exact `audio_stream` selected by PlaybackPlan is authoritative through remux/materialization.
10. Ordered progress uses playback session + sequence/event ordering; stale writes cannot overwrite newer progress and legitimate backward seeks remain valid.
11. Unsupported video codec/resolution/frame-rate/HDR/explicit bitrate limits produce an explicit unsupported result. No implicit tone-map or video-transcode fallback exists.

## Unified native playback architecture — implemented

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 ──> Playback Core ──> Media
StormFlix TV/Fire ───┘        │
                              ├── Profiles / progress
                              ├── Library access
                              └── Metadata / subtitles

Jellyfin clients ──> Jellyfin compatibility facade ──> StormFlix core
```

`internal/playback` owns source probing, capability contracts and deterministic source decisions.

Native modes:

- `direct_play` — original source, no re-encode;
- `remux` — container/audio-track compatibility while streams are copied where supported;
- `audio_compatibility` — video copied, selected audio encoded AAC-LC;
- `unsupported` — explicit refusal when client capabilities cannot consume video without forbidden video transcoding.

`POST /api/v1/media/{id}/playback/plan` is the authoritative native planning endpoint. It performs media/library/profile checks, preferred-audio selection, resume lookup, source probe and client-capability evaluation, then returns the execution URL, exact stream information and playback-session ID.

Source probing includes container, duration, bitrate, codecs, video dimensions, frame rate and HDR transfer classification. Capabilities can carry codec decode profiles, resolution/frame-rate limits, known HDR types, subtitle formats, audio telemetry, PiP/Media Session support and explicit Direct Play bitrate limits.

## StormFlix Web Player v4

The earlier 0.17 playback work changed the **playback engine**, but still rendered the old v2/v3 player presentation. Version 0.18 fixes that product gap: the browser now visibly uses **StormFlix Web Player v4**.

`internal/webui/static/playback-core.js` remains the source-policy owner. `player-v4.js`/`player-v4.css` own the new presentation.

Player v4 includes:

- cinematic full-screen overlay instead of the old player bar;
- SVG controls and responsive desktop/mobile layout;
- custom played/buffered timeline with seek preview;
- title, series/episode context and technical playback indicators;
- playback-mode chip (`Direct Play`, `Remux`, `Áudio AAC`);
- resolution/quality indicator;
- play/pause, ±10s, volume/mute;
- audio and subtitle controls through the existing track/settings system;
- previous/next episode controls from `/api/v1/media/{id}/neighbors`;
- fullscreen and native browser Picture-in-Picture;
- Media Session position integration through Playback Core;
- automatic control hiding while video plays.

The visual shell does **not** decide codecs or transcodes. PlaybackPlan remains authoritative.

Web monitoring sends server-issued playback session, monotonic progress sequence and event timestamps.

## Android / Android TV / Fire TV

Media3 executes the same PlaybackPlan contract as Web.

`PlaybackCapabilities.java` publishes MediaCodec decoder capabilities, container/audio/video support, resolution/frame-rate profiles and client features. TV/Fire stays remote/D-pad-first; phone/tablet stays touch-first.

`PlayerActivity`:

- requests PlaybackPlan before every source;
- preserves playback session across source/version switches;
- keeps native multi-audio Direct Play when possible;
- requests AAC compatibility when no usable local audio path exists;
- evaluates alternative physical versions through PlaybackPlan;
- preserves resume/seek position through source replacement;
- supports native audio/subtitle selection, previous/next episode, MediaSession and phone/tablet PiP;
- never silently bypasses the planner with raw `/stream` after a planning failure.

## Managed compatibility cache

`internal/webcompat` remains the seekable FFmpeg execution adapter. It is now fronted by a real `CacheManager`; `data/compat-cache` must never be treated as an unbounded permanent media store.

### Why the cache exists

Browsers/devices sometimes need a normal seekable MP4 when the original container/audio combination is not directly usable. StormFlix may therefore materialize an MP4 with:

- video stream-copy;
- exact selected audio stream;
- audio stream-copy when MP4-compatible, or AAC-LC audio conversion when required;
- `faststart` and normal HTTP Range/206 serving.

A source can be tens of gigabytes, so these artifacts require explicit lifecycle management.

### Default policy

- Maximum persistent usage: **20 GiB** (`STORMFLIX_COMPAT_CACHE_MAX_BYTES`).
- TTL: **48h** (`STORMFLIX_COMPAT_CACHE_TTL`).
- Automatic cleanup: enabled.
- Cleanup interval: **15 minutes**.
- LRU eviction target after exceeding the limit: **85%** of configured maximum.
- Minimum free disk reserve: **10 GiB OR 5% of the filesystem**, whichever is larger.
- Abandoned `.tmp` cleanup age: 1h.
- An artifact larger than the configured cache maximum is marked **oversize** and treated as short-lived; default idle expiration is 15 minutes.

`0` maximum means intentionally unlimited; this is not the default.

### Cache safety

- Last use is tracked in the cache's own persisted `.stormflix-cache.json` manifest; do not depend only on filesystem atime.
- A cached MP4 being generated or served is marked active and cannot be evicted.
- A temporary FFmpeg file belonging to an active materialization is also protected, even when materialization takes longer than the abandoned-temp threshold.
- Old abandoned `.tmp` files are removed.
- Eviction is LRU: least recently used inactive artifacts go first.
- Startup runs cleanup asynchronously so startup is not blocked by a large legacy cache.
- Before materializing a large file, disk pressure is checked and inactive cache entries are evicted first.
- If the free-space reserve still cannot be maintained, playback preparation fails cleanly instead of allowing FFmpeg to fill the disk.
- Cleanup is path-confined to `<DataDir>/compat-cache`; it must never delete `stormflix.db`, WAL files, `assets/`, artwork, original libraries or any other DataDir path.

A legacy cache such as the previously observed ~94 GiB directory is adopted on startup and reduced by TTL/LRU/oversize policy automatically when files are inactive.

### Admin controls

Admin → Configurações now contains **Playback · Cache de compatibilidade**.

It shows:

- current usage;
- configured limit;
- file count;
- active file count;
- oldest last-used entry;
- free filesystem space;
- last cleanup result.

It allows configuring:

- maximum size (5/10/20/50/100 GiB or unlimited);
- TTL;
- automatic cleanup;
- minimum free bytes;
- minimum free percentage.

`GET /api/v1/admin/playback/cache` returns cache status.

`POST /api/v1/admin/playback/cache/cleanup` performs manual cleanup while preserving active artifacts.

Settings are persisted through the existing generic `settings` table and applied without server restart.

## Jellyfin compatibility facade

Compatibility remains exposed at root Jellyfin paths and `/jellyfin-api`, isolated from native `/api/v1`.

Established behavior that must remain:

- `/System/Info/Public` advertises `ProductName: Jellyfin Server` and compatible numeric version `10.11.6`;
- official mobile WebView startup bridge;
- Android TV/Fire authentication flow;
- `/Users/Me`, display preferences, capabilities and modern Home aliases;
- `/UserViews`, `/UserItems/Resume`, `/Items/Latest`, `/Items`;
- `/ClientLog/Document` accepted for Android TV crash reports;
- artwork loader compatibility where official clients do not send native session state;
- Jellyfin `BaseItemDto.UserData` must never be null.

Do not diagnose Jellyfin client behavior using a catalog whose native scanner metadata is already known to be wrong.

## Library / episodic scanner identity

Supported kinds include:

- `movies`
- `series`
- `anime`
- `mixed`
- `anime_series` = Anime organized as television seasons
- `animation_series` = western cartoons / animated series
- music and existing special kinds.

For episodic libraries, **scanner-owned identity is authoritative before metadata providers**.

`media_series_identity` persists:

- library/source root;
- stable `series_key`;
- scanner/canonical `series_title`;
- season;
- episode;
- absolute episode.

Folder hierarchy defines the show. Technical folders such as Remux, BluRay, 1080p, Disc/Volume are not series. Brazilian season folders such as `5ª Temporada - Stormbrasil` are recognized.

Metadata enriches scanner identity and must not turn release filenames into separate shows.

## Metadata providers

Western series/cartoons preferred chain:

`Scanner → TMDB TV → TheTVDB v4 → Fanart.tv`

`animation_series` does not automatically enter anime providers merely because it is animated.

Anime with seasons combines scanner identity with TMDB TV, TheTVDB, AniList/AniDB/MAL recovery and the native HAMA-style Anime-Lists mapping bridge.

TheTVDB v4 is optional and configurable with API key + optional PIN. Without it, TMDB/Fanart continue.

## Manual matching — principal series only

Episodic manual matching is Plex-style and performed once at principal-series level.

Admin → Catálogo defaults to **Obras principais**. `/api/v1/admin/catalog/works` groups scanner `series_key` values into one principal work.

When a principal series is corrected:

1. decision is stored in `series_metadata_overrides` for `(library_id, series_key)`;
2. scanner identities are rebuilt while preserving scanner season/episode numbers;
3. episode refresh is queued as `metadata_jobs.job_type=series_refresh`;
4. current episodes refresh from the one principal decision;
5. future discovered episodes inherit the same override.

Episode rows in **Arquivos / diagnóstico** do not expose normal manual matching; they link back to the principal work.

`media_metadata.manual_match` remains standalone item-level. Series protection lives in `series_metadata_overrides`.

## Persistent scan queue / operational jobs

`scan_jobs` is persistent and FIFO.

- `Escanear agora` queues a library.
- `Escanear todas` queues all active libraries.
- Scans run **one library at a time by design** to avoid rclone/FUSE + SQLite contention.
- Duplicate active queue entries are avoided.
- Queued/running jobs can be removed/cancelled.
- Running scan uses one shared cancellable timeout context.
- Restart returns unfinished scan jobs safely to queued state.
- An unreachable mount preserves the previous catalog rather than treating it as deletion.

Admin → **Fila & atividades** merges scan jobs, library metadata jobs and `series_refresh`, including progress, current message, matched/success/error counts and cancellation.

## Categories / site hierarchy

Category hierarchy uses `library_categories.parent_id`.

System roots:

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

Custom categories may also be top-level or nested. Parent browsing aggregates active descendants; child browsing stays scoped. The site exposes category chips and one rail per child in split-root view. Recommended organization never deletes custom categories.

## Home / SQLite performance

SQLite remains intentional.

Current optimizations:

- indexes for artwork, recent/title ordering, ratings/status and series identity/overrides;
- 20-second static grouped Home cache keyed by exact library-access scope;
- Continue Watching is never static-cached;
- independent Home reads execute concurrently under WAL;
- cached feeds are deep-copied before request mutation.

If Home becomes slow again, add SQL/query timing first. Do not jump directly to a database migration.

## Schema history

- Phase 9: `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, performance indexes.
- Phase 11: ordered series-child index and cleanup of legacy per-episode manual flags.
- Phase 12: persistent `scan_jobs` and metadata job type/series/provider fields.
- Playback cache management adds **no schema migration**; runtime cache settings use the existing key/value `settings` table and cache LRU metadata uses the manifest inside `compat-cache`.

## Required validation after relevant changes

Before calling playback/cache work ready:

- `gofmt`;
- JavaScript syntax checks;
- `go test ./...`;
- `go test -race ./internal/playback ./internal/webcompat` when touching playback/cache concurrency;
- Go server build;
- Android build/tests when Android code changes.

Cache tests must demonstrate that an artificially oversized cache is reduced below the configured target, active files are preserved, abandoned temporary files are removed, active temp files are preserved, oversize entries expire and cleanup cannot escape the cache directory.

## Known pending real-world QA

Implementation is complete, but these behaviors still depend on real environments and should be verified during deployment/QA rather than guessed from unit tests:

- Player v4 visual/interaction QA on Chrome/Chromium, Firefox and Safari-capable browsers;
- playback of real H.264/AAC, H.264+DTS, HEVC/AV1 and multi-audio media across browsers/devices;
- first startup cleanup against the server's existing legacy `compat-cache` size;
- real rclone/Drive scan queue behavior;
- problematic cartoon/dubbed-anime principal matching while watching `series_refresh`;
- Jellyfin Android TV/Fire validation after catalog metadata is known-correct.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Do not store secrets or private media paths.
