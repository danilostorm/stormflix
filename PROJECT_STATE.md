# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file and `AGENTS.md` before changing code. Update this document after meaningful changes.

Last architecture update: **2026-08-28**.

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

- Server version constant: `0.19.0-dynamic-hls`.
- Native Android application: `cloud.stormflix.app`, version **0.3.0**, versionCode 11, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- SQLite remains the supported database with WAL, synchronous=NORMAL, busy timeout, bounded connection pool and targeted indexes.

## Non-negotiable playback rules

1. **Direct Play first. Never silently transcode video.**
2. Web, Android and TV/Fire use the same native `PlaybackPlan` policy under `/api/v1`.
3. Jellyfin compatibility remains isolated from native `/api/v1`; native StormFlix clients do not depend on Jellyfin endpoints.
4. Storage origin is not playback policy. A local file, rclone/FUSE mount, Google Drive mount or NFS source is evaluated by its actual streams/capabilities, not by provider name.
5. Android/TV may keep the original multi-audio source when Media3 has a usable decoder and choose tracks locally.
6. Browser multi-audio selection may be pinned server-side because HTML audio-track APIs are inconsistent.
7. Preferred audio understands `pt-BR → pt → por`, including Português/Dublado/Brasil labels, while explicit profile languages remain authoritative.
8. If video is supported and only selected audio is incompatible, compatibility mode keeps **video stream-copy** and converts only audio to AAC-LC.
9. Exact `audio_stream` selected by PlaybackPlan is authoritative through execution.
10. Ordered progress uses playback session + sequence/event ordering; stale writes cannot overwrite newer progress and legitimate backward seeks remain valid.
11. Unsupported video codec/resolution/frame-rate/HDR/explicit bitrate limits produce an explicit unsupported result. No implicit tone-map or video-transcode fallback exists.
12. Web compatibility cache is session-scoped and disposable. Normal player close/end must delete that session's HLS cache immediately.

## Unified native playback architecture — implemented

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 ──> Playback Core ──> Media
StormFlix TV/Fire ───┘        │
                              ├── Profiles / progress
                              ├── Library access
                              ├── Metadata / subtitles
                              ├── Dynamic HLS session cache (Web)
                              └── Seekable compatibility MP4 cache (native/manual fallback)

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

## StormFlix Web Player v4 + dynamic HLS

`internal/webui/static/playback-core.js` remains the source-policy/execution owner. `player-v4.js`/`player-v4.css` own the visible player presentation.

Player v4 includes:

- cinematic full-screen overlay;
- SVG controls and responsive desktop/mobile layout;
- custom played/buffered timeline with seek preview;
- title, series/episode context and technical playback indicators;
- playback-mode chip (`Direct Play`, `Remux`, `Áudio AAC`);
- resolution/quality indicator;
- play/pause, ±10s, volume/mute;
- audio/subtitle controls;
- previous/next episode controls;
- fullscreen and native browser Picture-in-Picture;
- Media Session position integration;
- automatic control hiding while video plays.

The visual shell does **not** decide codecs or transcodes. PlaybackPlan remains authoritative.

### Direct Play path

Compatible media is served through `/api/v1/media/{id}/stream` with `os.Open` + `http.ServeContent` + HTTP Range/206 semantics.

**Direct Play creates zero StormFlix HLS/compatibility cache.** A Google Drive/rclone mount is read directly through the mount just like local storage; any VFS cache is rclone's responsibility.

### Web remux/audio compatibility path

Web no longer waits for a complete seekable compatibility MP4 to be written before playback.

For a non-Direct-Play Web plan, StormFlix creates a session-scoped dynamic fMP4 HLS timeline:

- `GET /api/v1/media/{id}/hls/{session}/index.m3u8`
- `GET /api/v1/media/{id}/hls/{session}/init/{batch}.mp4`
- `GET /api/v1/media/{id}/hls/{session}/segment/{segment}.m4s`

Default execution:

- 6-second segments;
- 4 segments per FFmpeg batch (~24 seconds);
- FFmpeg seeks to the requested batch position before input processing;
- `-c:v copy` is mandatory;
- exact selected video/audio streams are mapped;
- audio is stream-copied when compatible with the fMP4/browser path;
- DTS/TrueHD/Opus/FLAC and other incompatible selected audio is converted to AAC-LC only;
- HEVC is tagged `hvc1` where needed;
- Web uses pinned hls.js for MSE browsers, with native HLS fallback where supported.

This means a 20/40/50+ GiB remote movie is not rewritten in full before playback can start. Only the requested small batch is generated.

To prevent periodic stalls at each 24-second batch boundary on rclone/Google Drive, Web HLS segment delivery now keeps **one batch ahead warm in the background**. After a requested segment is served, the manager waits for the active batch to finish and pre-generates the next batch if no user-requested worker/seek has taken priority. This remains bounded: it never intentionally walks the whole movie ahead, speculative prefetch yields to seeks, and every batch still passes the same global cache/free-disk checks.

Web monitoring sends server-issued playback session, monotonic progress sequence and event timestamps.

## Web HLS cache / SSD safety

HLS fragments live under:

`<DataDir>/hls-cache/<playback-session>/`

They are ephemeral session data.

### Hard global defaults

- HLS maximum: **5 GiB globally for all users**, not per user (`STORMFLIX_HLS_CACHE_MAX_BYTES`).
- Segment duration: **6s** (`STORMFLIX_HLS_SEGMENT_DURATION`).
- Batch size: **4 segments / ~24s** (`STORMFLIX_HLS_BATCH_SEGMENTS`).
- Idle/crashed-session fallback TTL: **30m** (`STORMFLIX_HLS_CACHE_IDLE_TTL`).
- Minimum free disk reserve: **10 GiB OR 5%**, whichever is larger, shared with compatibility-cache safety settings.
- Old fragments behind the user's current playback position are continuously removed.
- Web streaming keeps at most the normal requested batch plus a bounded speculative next batch; the speculative batch uses the same global budget and is not allowed to override an active seek/user-requested batch.

Before starting a new FFmpeg batch, the HLS manager estimates its size from source bitrate and reserves capacity. If `current usage + estimated batch` would exceed the global maximum, it evicts oldest disposable fragments first. Sessions with a running FFmpeg worker are not pressure-evicted. If enough safe disk cannot be made available, the new batch is refused instead of allowing the SSD to fill.

### Immediate cleanup on player close/end

This is a required behavior, not an optional periodic cleanup:

1. Web sends `DELETE /api/v1/media/{id}/playback?session=<playback-session-id>` when the movie ends or the player closes.
2. The server verifies that the HLS session belongs to the authenticated user.
3. Any FFmpeg worker for that HLS session is cancelled.
4. The entire `<DataDir>/hls-cache/<session>/` directory is removed immediately.
5. `beforeunload` sends a best-effort keepalive DELETE when the tab/window disappears without a normal player close.

Source/version switches using the same logical playback session clear old HLS fragments before registering the new source. Switching from HLS compatibility to Direct Play also clears the previous HLS session immediately.

The 30-minute idle TTL exists only for crashes/network loss/clients that never deliver normal cleanup.

At StormFlix startup, the dedicated `hls-cache` directory is cleared because fragments from previous server processes are disposable and should never consume SSD indefinitely.

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

The dynamic HLS executor is currently Web-specific. Android/TV still use the existing seekable compatibility MP4 executor when server-side compatibility is required.

## Managed legacy compatibility MP4 cache

`internal/webcompat/materialize.go` + `CacheManager` remain for Android/TV/manual compatibility paths. Automatic StormFlix Web remux/audio compatibility no longer uses whole-file materialization.

The legacy compatibility cache lives at `<DataDir>/compat-cache` and remains managed:

- maximum persistent usage: **20 GiB** (`STORMFLIX_COMPAT_CACHE_MAX_BYTES`);
- TTL: **48h** (`STORMFLIX_COMPAT_CACHE_TTL`);
- cleanup interval: **15m**;
- LRU target: **85%**;
- minimum free disk: **10 GiB or 5%**;
- oversize artifacts are short-lived;
- active materialization/serving artifacts are protected;
- path confinement prevents deletion outside `compat-cache`.

The existing Admin → Configurações → **Playback · Cache de compatibilidade** controls this legacy seekable-MP4 cache. HLS-specific hard defaults are currently environment-configured; do not claim those Admin controls change the HLS budget unless that UI/settings support is explicitly added later.

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
- Playback cache management adds **no schema migration**. HLS session state is filesystem/in-memory only and disposable.

## Required validation after relevant changes

Before calling playback/cache work ready:

- `gofmt`;
- JavaScript syntax checks;
- `go test ./...`;
- `go test -race ./internal/playback ./internal/webcompat` when touching playback/cache concurrency;
- Go server build;
- Android build/tests when Android code changes.

Dynamic HLS tests must cover playlist layout, immediate session deletion, user ownership, source-switch deletion, idle cleanup, hard-budget eviction, bounded next-batch prefetch and path confinement. Legacy cache tests must continue covering LRU/TTL/active-file/temp/oversize safety.

## Known pending real-world QA

Implementation is code-complete only after CI is green. Real environments still need deployment QA for:

- Player v4 + dynamic HLS on Chrome/Chromium, Firefox and Safari-capable browsers;
- real H.264/AAC Direct Play from rclone/Google Drive mounts;
- H.264+DTS/TrueHD and multi-audio Web HLS startup/seek behavior;
- HEVC/AV1 browser capability behavior without video transcode;
- actual SSD usage under multiple simultaneous HLS sessions and immediate cleanup after close;
- existing legacy `compat-cache` startup cleanup;
- real rclone/Drive scan queue behavior;
- Jellyfin Android TV/Fire validation after catalog metadata is known-correct.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Do not store secrets or private media paths.