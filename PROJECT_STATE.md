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

Native modes:

- `direct_play` — original source;
- `remux` — compatible streams copied into a browser-compatible path;
- `audio_compatibility` — video copied, selected audio encoded AAC-LC;
- `unsupported` — explicit refusal when video would require forbidden transcoding.

Source probing records container, duration, bitrate, codecs, dimensions, frame-rate and HDR transfer information.

## StormFlix Web Player v4 + dynamic HLS

`playback-core.js` owns playback policy/execution. `player-v4.js`/`player-v4.css` own presentation.

Player v4 includes custom played/buffered timeline, seek preview, mode/resolution indicators, play/pause, ±10s, volume/mute, audio/subtitle/settings access, previous/next episode, fullscreen, native PiP, Media Session and responsive desktop/mobile controls.

### Direct Play

Compatible media uses `/api/v1/media/{id}/stream` with `os.Open` + `http.ServeContent` + HTTP Range/206.

**Direct Play creates zero StormFlix HLS/compatibility cache.** A Google Drive/rclone mount is read directly through the mount; rclone VFS caching is rclone's responsibility.

### Web remux/audio compatibility

Non-Direct-Play Web playback uses session-scoped dynamic fMP4 HLS instead of waiting for a complete compatibility MP4:

- `GET /api/v1/media/{id}/hls/{session}/index.m3u8`
- init fragments and media segments are served through HLS session routes;
- default 6-second segments, 4 segments/about 24 seconds per FFmpeg batch;
- `-c:v copy` is mandatory;
- exact selected audio/video streams are mapped;
- audio is copied when browser/fMP4 compatible, otherwise AAC-LC only;
- HEVC is tagged `hvc1` where needed;
- Web uses pinned hls.js with native-HLS fallback where supported.

To avoid periodic stalls on rclone/Google Drive, one next batch is warmed in the background. Prefetch stays bounded to one batch ahead and yields to seeks/user-requested batches.

## Web HLS cache / SSD safety

HLS fragments live under `<DataDir>/hls-cache/<playback-session>/`.

Defaults:

- **5 GiB global maximum for all users**, not per user;
- 6-second segments;
- 4-segment batches;
- 30-minute idle/crash fallback TTL;
- minimum free disk reserve: **10 GiB or 5%**, whichever is larger;
- old fragments behind current playback are removed continuously;
- active FFmpeg workers are protected from pressure eviction;
- new batches are refused if safe capacity cannot be reserved.

Normal close/end sends authenticated playback-session cleanup, cancels that session's FFmpeg worker and deletes the entire session directory immediately. Browser unload sends best-effort keepalive cleanup. Startup removes orphaned HLS fragments from prior server processes.

## Android / Android TV / Fire TV

Media3 consumes the same PlaybackPlan contract. Android/TV publishes MediaCodec capabilities, preserves native multi-audio Direct Play where possible, supports source/version selection and ordered resume/progress, and does not silently bypass the planner.

Dynamic HLS is currently Web-specific. Android/TV may still use the managed seekable compatibility MP4 path when server-side compatibility is required.

## Managed legacy compatibility MP4 cache

`internal/webcompat/materialize.go` + `CacheManager` remain for Android/TV/manual compatibility paths.

Defaults:

- 20 GiB maximum;
- 48h TTL;
- cleanup every 15m;
- LRU target 85%;
- 10 GiB or 5% free-space reserve;
- active materialization/serving files protected;
- oversize artifacts short-lived;
- path confinement prevents deletion outside `compat-cache`.

Admin → Configurações → **Playback · Cache de compatibilidade** controls this legacy cache. HLS budget remains environment-configured unless explicit Admin support is added later.

## Jellyfin compatibility facade

Jellyfin compatibility stays exposed separately from native `/api/v1` and supports the established official-client routes, authentication, user/home aliases, resume/latest/items, crash logging and non-null `BaseItemDto.UserData` behavior.

Do not diagnose Jellyfin playback using a catalog whose scanner metadata is known to be wrong.

## Scanner identity / metadata

Supported library kinds include `movies`, `series`, `anime`, `mixed`, `anime_series`, `animation_series`, music and existing special kinds.

For episodic libraries, scanner-owned identity is authoritative before metadata providers. `media_series_identity` stores source root, stable `series_key`, canonical scanner title, season, episode and absolute episode. Technical folders such as Remux/BluRay/1080p/Disc/Volume are not series.

Metadata enriches scanner identity and must not redefine folder hierarchy blindly.

Western series/cartoons preferred chain:

`Scanner → TMDB TV → TheTVDB v4 → Fanart.tv`

Anime with seasons combines scanner identity with TMDB TV, TheTVDB, AniList/AniDB/MAL recovery and the native HAMA-style Anime-Lists mapping bridge.

### Source-root relocation without metadata rebuild

Changing a library's physical Drive/rclone mount path must not turn unchanged media into new catalog identities. When configured source roots are replaced in an unambiguous one-for-one mapping, the Admin library update now rewrites each existing `media.path` from the old root to the new root while preserving the same `media.id`. Episodic `media_series_identity.source_root` follows the relocation while scanner series/season/episode identity remains intact.

Because metadata, artwork, subtitles, watch progress and related state are keyed by `media_id`, existing matched metadata and downloaded covers remain attached. A normal subsequent scan sees the rewritten new path and updates the existing row instead of creating a second media item. Normal non-refresh metadata jobs continue skipping already matched titles, so a Drive path move alone does not trigger cover/metadata downloads again.

Relocation is intentionally conservative: source reordering does not count as a move, ambiguous add/remove counts are not guessed, and a target-path collision is not destructively merged. Those cases fall back to normal scan/consolidation behavior.

## Manual matching — principal series only

Admin → Catálogo defaults to **Obras principais**. Episodic manual matching is stored once at principal-series level in `series_metadata_overrides`; scanner identities are rebuilt while season/episode numbers are preserved, a queued `series_refresh` updates current episodes, and future episodes inherit the same override.

Episode rows in **Arquivos / diagnóstico** do not expose normal manual matching.

## Persistent scan queue

`scan_jobs` is persistent FIFO. Library scans are serialized one at a time to avoid rclone/FUSE + SQLite contention. Duplicate active entries are avoided, queued/running work can be cancelled, restart safely requeues unfinished jobs, and an unreachable mount preserves the previous catalog instead of treating it as deletion.

Admin → **Fila & atividades** merges scan jobs, metadata jobs and `series_refresh` work.

### Stable Admin scan/metadata polling

The Admin no longer needs to rebuild the complete Bibliotecas or Metadados & Capas DOM every polling interval. While scan jobs are active, stable library structure is patched in place (counts, status, source health, queue message and button state). Full rendering is reserved for an actual structural library/source change. Metadata polling similarly patches summary counters, job rows and action states without replacing the whole page, preventing the visible flashing that previously happened about every two seconds.

Metadados & Capas keeps individual `Buscar metadados`, `Reprocessar erros` and `Atualizar tudo` controls per library and also exposes bulk `Buscar em todas` and `Atualizar todas`. Bulk work is sequential across active video libraries to avoid hammering providers. `Buscar em todas` is non-refresh and preserves already matched metadata; `Atualizar todas` deliberately reprocesses all eligible titles.

## Categories / automatic catalog organization

Category hierarchy still uses `library_categories.parent_id`, with system roots **Filmes**, **Séries** and **Animes** plus optional custom/configured children.

The Web catalog now treats metadata as the normal automatic categorization source so the user does not need to manually create genre categories:

1. Configured child categories have first priority. If the same title exists in overlapping sibling categories, the first configured child claims it.
2. Titles not already claimed are classified from metadata into **one primary genre only**. A title never appears simultaneously in two automatic genre rails.
3. Known TMDB/TV/anime genre labels are normalized to **Português do Brasil** (for example Ação, Aventura, Animação, Comédia, Documentários, Família, Ficção científica, Terror, Suspense, Sobrenatural, Esportes, Cotidiano, Notícias, Novelas and others).
4. Raw unknown/foreign genre labels are never exposed as section titles. If no recognized genre metadata exists, the title goes to **Outros** until metadata is corrected/refreshed.
5. Empty categories are not rendered.
6. `Outros` is ordered last.
7. Root pages do not add a simultaneous `Todos em ...` rail, because that would duplicate every title. When configured children exist, `Tudo em <categoria>` remains available as a separate explicit aggregate view in the secondary navigation.
8. Opening an individual child category may subdivide that child by the same exclusive pt-BR metadata classification, again with no duplicate title across those rails.

This is presentation-time classification; it introduces no schema migration and does not change profile/library access checks.

## Home / SQLite performance

SQLite remains intentional. Current optimizations include targeted indexes, 20-second access-scope-aware static Home cache, uncached profile-specific Continue Watching, concurrent independent reads under WAL and deep-copying cached feeds before request mutation.

## Schema history

- Phase 9: `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, performance indexes.
- Phase 11: ordered series-child index and cleanup of legacy episode manual flags.
- Phase 12: persistent `scan_jobs` and metadata job type/series/provider fields.
- Playback/cache, source-root relocation and automatic genre presentation changes add no schema migration.

## Required validation

Before presenting relevant work as ready:

- JavaScript syntax checks;
- `go test ./...`;
- `go test -race ./internal/playback ./internal/webcompat` when playback/cache concurrency is touched;
- Go server build;
- Android build/tests when Android code changes.

Source-root relocation tests must prove that the same `media.id`, matched metadata and selected artwork survive a physical root change and remain the same after the next scan.

## Known pending real-world QA

Real deployment should still validate:

- Player v4 + dynamic HLS on Chromium/Firefox/Safari-capable browsers;
- H.264/AAC Direct Play from rclone/Google Drive;
- H.264+DTS/TrueHD and multi-audio HLS startup/seek;
- HEVC/AV1 browser capability behavior without video transcode;
- SSD use under concurrent HLS sessions and immediate cleanup after close;
- automatic exclusive category rails against real TMDB/TV/anime metadata, including `Outros` recovery after metadata refresh;
- real Drive/rclone mount-root changes preserving media IDs/artwork and the smooth Admin polling behavior during scan-all/metadata-all;
- Jellyfin Android TV/Fire after catalog metadata is known-correct.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Never store secrets, credentials or private media paths.