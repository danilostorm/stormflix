# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file and `AGENTS.md` before changing code. Update this document after meaningful changes.

Last architecture update: **2026-08-29**.

## Deployment

Primary deployment is an Unraid checkout:

```bash
cd /mnt/user/appdata/stormflix
git pull origin main
docker compose down
docker compose up -d --build
curl -s http://127.0.0.1:8090/healthz
echo
```

Server HTTP port: **8090**, normally behind HTTPS reverse proxy.

## Current versions

- Server: `0.19.0-dynamic-hls`.
- Android package: `cloud.stormflix.app`, version **0.3.0**, versionCode 11, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- SQLite with WAL, synchronous=NORMAL, busy timeout, bounded pool and targeted indexes remains supported.

## Non-negotiable playback rules

1. **Direct Play first. Never silently transcode video.**
2. Web, Android and TV/Fire use the native `/api/v1/media/{id}/playback/plan` policy.
3. Jellyfin compatibility stays isolated from native `/api/v1`.
4. Storage provider does not determine playback mode. Local, rclone/FUSE, Google Drive mount and NFS are evaluated by actual streams/capabilities.
5. Browser multi-audio may be pinned server-side when HTML audio-track APIs are unreliable.
6. Preferred audio understands `pt-BR → pt → por`, including Português/Dublado/Brasil labels.
7. If video is supported and only selected audio is incompatible, keep **video stream-copy** and convert only audio to AAC-LC.
8. Exact selected `audio_stream` remains authoritative through execution.
9. Unsupported video codec/resolution/frame-rate/HDR/explicit bitrate limits return unsupported; no silent video transcode/tone-map fallback.
10. Ordered progress uses playback session plus sequence/event ordering.
11. Web HLS cache is disposable session data and normal close/end must delete it immediately.

## Unified native playback

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

Native modes remain `direct_play`, `remux`, `audio_compatibility` and `unsupported`. Video is never silently transcoded.

## StormFlix Web Player v4 + dynamic HLS

`playback-core.js` owns playback policy/execution. `player-v4.js`/`player-v4.css` own presentation. Player v4 includes custom played/buffered timeline, seek preview, mode/resolution indicators, play/pause, ±10s, volume/mute, audio/subtitle/settings access, previous/next episode, fullscreen, native PiP, Media Session and responsive controls.

Compatible media uses `/api/v1/media/{id}/stream` with HTTP Range. Direct Play creates zero StormFlix HLS/compatibility cache. Web remux/audio compatibility uses session-scoped dynamic fMP4 HLS, exact selected streams and mandatory video stream-copy; only incompatible audio may be converted to AAC-LC.

## Web HLS cache / SSD safety

HLS fragments live under `<DataDir>/hls-cache/<playback-session>/` with a 5 GiB global budget, 30-minute idle/crash fallback TTL and a 10 GiB or 5% minimum free-disk reserve. Old fragments are removed continuously; active FFmpeg workers are protected. Normal close/end cancels the worker and deletes the session directory immediately.

## Android / Android TV / Fire TV

Media3 consumes the same PlaybackPlan contract, publishes MediaCodec capabilities, preserves native multi-audio Direct Play where possible, supports source/version selection and ordered progress, and does not silently bypass the planner. Dynamic HLS is Web-specific; native apps may still use managed seekable compatibility MP4 when explicitly needed.

## Managed legacy compatibility MP4 cache

`internal/webcompat/materialize.go` + `CacheManager` remain for Android/TV/manual compatibility paths. Defaults: 20 GiB max, 48h TTL, 15m cleanup, LRU target 85%, free-space reserve, active-file protection and short-lived oversize artifacts. Admin → Configurações → **Playback · Cache de compatibilidade** controls this cache.

## Jellyfin compatibility facade

Jellyfin compatibility remains separate from native `/api/v1` and supports the established official-client routes, authentication, Home/resume/latest aliases, user DTO requirements and Android TV/Fire compatibility without becoming the source of truth.

## Scanner identity / metadata

Supported library kinds include `movies`, `series`, `anime`, `mixed`, `anime_series`, `animation_series`, music and existing special kinds. For episodic libraries, scanner-owned identity is authoritative before metadata providers. `media_series_identity` stores source root, stable series key/title, season, episode and absolute episode. Metadata enriches this identity and must not redefine folder hierarchy blindly.

Western series/cartoons prefer `Scanner → TMDB TV → TheTVDB v4 → Fanart.tv`. Anime with seasons combines scanner identity with TMDB TV, TheTVDB, AniList/AniDB/MAL recovery and the native HAMA-style Anime-Lists bridge.

### Source-root relocation without metadata rebuild

Changing a library's physical Drive/rclone mount path must not turn unchanged media into new catalog identities. When old/new roots form an unambiguous one-for-one replacement, Admin library update rewrites existing `media.path` and episodic `source_root` while preserving the same `media.id`.

Because metadata, artwork, subtitles, watch progress and related state are keyed by `media_id`, matched metadata and downloaded covers remain attached. A normal later scan updates the existing row instead of creating a new media item. Relocation is conservative: ambiguous source changes and collisions are not guessed.

## Manual matching — principal series only

Admin → Catálogo defaults to **Obras principais**. Episodic manual matching is stored once at principal-series level in `series_metadata_overrides`; a queued `series_refresh` updates current episodes and future episodes inherit the same override. Episode diagnostic rows do not expose normal manual matching.

## Persistent scan queue / stable Admin polling

`scan_jobs` is persistent FIFO. Library scans are serialized to avoid rclone/FUSE + SQLite contention, duplicate active entries are avoided, queued/running jobs can be cancelled, restart safely requeues unfinished jobs and unreachable mounts preserve the prior catalog.

Admin → **Fila & atividades** merges scan jobs, metadata jobs and `series_refresh`. Bibliotecas and Metadados & Capas patch progress/status in place during polling rather than rebuilding the whole DOM, preventing visible flashing. Metadados & Capas provides individual actions plus bulk **Buscar em todas** and **Atualizar todas**, processed sequentially across active video libraries.

## Home menus / gallery sections

`library_categories.parent_id` now has a clear two-level presentation meaning for the native Web interface:

- `parent_id IS NULL` = **Menu da Home** shown in the top navigation;
- direct child of a root = **Seção da galeria** rendered as its own horizontal rail inside that menu.

The system roots **Filmes**, **Séries** and **Animes** remain. Administrators may create additional root menus such as **Desenhos**. Custom active roots are inserted in the top navigation before Música. Direct child sections never appear as top-menu buttons, category explorer chips or secondary navigation.

When a Home menu has active child sections:

1. sections are rendered in configured `sort_order`;
2. each child is fetched through the existing category/library permission path;
3. selected libraries determine that section's catalog scope;
4. overlapping titles are de-duplicated, with the first configured section claiming the title;
5. only configured child sections are rendered for that menu; automatic genres do not add extra rails behind them;
6. empty sections are not rendered.

When a Home menu has **no active child sections**, the previous automatic metadata organization remains as a compatibility fallback: each title is assigned to one primary normalized pt-BR genre, unknown/missing genre becomes **Outros**, empty groups are hidden and Outros stays last.

Admin → **Menus da Home** is the presentation editor. It has root menu cards, `+ Novo menu da Home`, `+ Nova seção`, order/type/active controls and a video-library picker. Section editing allows moving a section to another Home menu. System roots cannot be deleted; custom roots with child sections must have those children removed/moved first.

The recommended organizer follows the new model:

- Filmes → 4K / UHD, Animação, Outros filmes;
- Séries → Séries de TV;
- Animes → Dublados, Séries, Filmes;
- when an `animation_series` library exists, create/reuse **Desenhos** as a Home menu and place **Todos os desenhos** (`series-desenhos`) below it;
- the reserved legacy `series-animes` section under Séries is disabled when the organizer is explicitly run.

This reformulation reuses the existing category tables and associations; **no schema migration** is required and profile/library access checks remain authoritative.

## Home / SQLite performance

SQLite remains intentional. Current optimizations include targeted indexes, a short access-scope-aware static Home cache, uncached profile-specific Continue Watching, concurrent independent reads under WAL and deep-copying cached feeds before request mutation.

## Schema history

- Phase 9: `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, performance indexes.
- Phase 11: ordered series-child index and cleanup of legacy episode manual flags.
- Phase 12: persistent `scan_jobs` and metadata job type/series/provider fields.
- Playback/cache, source-root relocation and the Home-menu/category presentation reformulation add no schema migration.

## Required validation

Before presenting relevant work as ready:

- JavaScript syntax checks, including public category navigation and Admin Home-menu editor;
- `go test ./...` including the Home-menu organizer regression test;
- `go test -race ./internal/playback ./internal/webcompat` when playback/cache concurrency is touched;
- Go server build;
- Android build/tests only when Android code changes.

Source-root relocation tests must keep proving that the same `media.id`, matched metadata and selected artwork survive a physical root change and the next scan.

## Known pending real-world QA

Real deployment should still validate:

- Player v4 + dynamic HLS on target browsers and mounted storage;
- SSD use under concurrent HLS sessions and immediate cleanup after close;
- custom Home menu creation such as **Desenhos** and its appearance before Música;
- child sections such as **Animes Dublados** appearing only as gallery rails, never as menu/subnav items;
- section ordering, overlap de-duplication and profile/library permission filtering with the real catalog;
- real Drive/rclone root changes preserving IDs/artwork and smooth scan/metadata polling;
- Jellyfin Android TV/Fire after catalog metadata is known-correct.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Never store secrets, credentials or private media paths.
