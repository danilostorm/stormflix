# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file and `AGENTS.md` before changing code. Update this document after meaningful changes.

Last architecture update: **2026-08-30**.

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

- Server: **`0.21.0-playback-v5`**.
- Android package: `cloud.stormflix.app`, version **0.5.0**, versionCode **14**, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- Jellyfin compatibility facade advertises numeric compatibility version `10.11.6` to satisfy current official-client version parsing; this does **not** turn StormFlix into Jellyfin or change the native server version.
- SQLite remains the production catalog database with WAL, `synchronous=NORMAL`, busy timeout, bounded connection pool and targeted indexes.
- PostgreSQL remains a separate planned scale-out phase. It is deliberately not mixed into Playback Engine v5 because existing migrations, backup/restore semantics and queries contain SQLite-specific behavior.

## Non-negotiable invariants

1. Native `/api/v1` is the StormFlix source of truth.
2. **Direct Play is always evaluated first.** Video transcoding must never be a silent bypass around PlaybackPlan.
3. Web, Android, Android TV and Fire TV use `/api/v1/media/{id}/playback/plan` as the authoritative playback decision.
4. Jellyfin compatibility is an isolated facade and must not redefine native catalog/playback state.
5. Scanner-owned episodic identity is authoritative before metadata providers.
6. Episodic manual matching is principal-series level only.
7. Profile/kids/library access controls must survive every browse, smart-section and playback path.
8. Exact selected audio stream remains authoritative. If only audio is incompatible, video stays stream-copy and only audio may be converted to AAC-LC.
9. **Explicit video transcoding is allowed only when the client advertises `allow_video_transcode` or the user explicitly requests a lower playback quality.** The returned plan must expose the incompatibility/quality reason, source/target codec, target resolution/bitrate and tone-map state. Unknown legacy clients do not receive the new video-transcode transport.
10. Ordered progress remains session/sequence aware; changing source/version/quality must preserve progress and seek state.

## Native Playback Engine v5

Playback policy is now deterministic and shared by Web + native apps:

```text
1. Direct Play
   ↓ only if required
2. Remux / server-side track selection (video + compatible audio copied)
   ↓ only if audio alone is incompatible
3. Audio compatibility (video copied, selected audio → AAC-LC)
   ↓ only if video/device/quality requires it and client opted in
4. Video transcode (explicit PlaybackPlan v5 HLS session)
   ↓ if no safe route exists
5. Unsupported
```

Native modes are:

- `direct_play`
- `remux`
- `audio_compatibility`
- `video_transcode`
- `unsupported`

`video_transcode` is selected for explicit incompatibilities such as unsupported video codec, advertised decoder resolution/FPS/HDR limits, explicit Direct Play bitrate limits, or a user-selected quality lower than the source. It is not selected merely because FFmpeg exists.

### User quality policy

Web and Android/TV/Fire expose:

- `Auto`
- `Original`
- `4K / 2160p`
- `1440p`
- `1080p`
- `720p`
- `480p`

`Auto` chooses the best compatibility route from device/network capabilities. `Original` never forces a resolution downshift. Selecting a lower explicit quality than the source produces `reason_code=quality_limit` and an explicit video-transcode plan; StormFlix never upscales a lower-resolution source merely to match a higher setting.

The selected quality is persistent per client. Quality changes during playback rebuild the plan and source while preserving current time and play/pause state.

## Streaming architecture

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 ──> PlaybackPlan v5 ──> Media/rclone
StormFlix TV/Fire ───┘        │
                              ├── HTTP Range Direct Play
                              ├── Dynamic fMP4 HLS remux/audio compatibility
                              ├── Dynamic fMP4 HLS video transcode
                              ├── Profiles / ordered progress
                              ├── Metadata / subtitles
                              └── bounded session caches

Official Jellyfin clients ──> isolated compatibility facade ──> StormFlix core
```

### Direct Play

Compatible media streams directly from the mounted/rclone source using HTTP Range. It creates **zero StormFlix HLS/transcode cache**. A source/session switch from a previous HLS/transcode route closes the old temporary session before Direct Play begins.

### Dynamic HLS remux / audio compatibility

`internal/webcompat/hls_manager.go` remains the lightweight compatibility engine for cases where video does not need recoding.

Defaults:

- fMP4 HLS
- 6-second segments
- 4 segments/batch by default
- 5 GiB global HLS cache budget
- 30-minute idle/crash cleanup
- 10 GiB or 5% minimum free-space reserve
- bounded adaptive look-ahead

Video is always stream-copy on this path. Only the exact selected incompatible audio may become AAC-LC.

### Playback Engine v5 video transcode

`internal/transcode/manager.go` is a separate bounded engine for explicit `video_transcode` sessions. Session IDs are prefixed `v5t-`, allowing the existing authenticated HLS route family to dispatch either stream-copy HLS or video-transcode HLS without exposing a second public transport contract.

Default policy:

- cache: `<DataDir>/transcode-cache/<playback-session>/`
- **5 GiB global transcode cache budget**
- 4-second fMP4 segments
- 5 segments/batch (about 20 seconds of work at a time)
- 20-minute idle/crash fallback cleanup
- 10 GiB or 5% minimum free-space reserve
- segments behind playback are trimmed
- normal playback stop/close immediately cancels the FFmpeg worker and removes the entire session directory

Transcoding is on-demand in small batches rather than materializing an entire movie before playback. This is specifically important for Google Drive/rclone mounts.

### Hardware acceleration and CPU fallback

Engine discovery inspects the installed FFmpeg build and available device nodes. Supported encoder candidates include, when present:

- NVIDIA NVENC: `h264_nvenc`, `hevc_nvenc`, `av1_nvenc`
- Intel Quick Sync: `h264_qsv`, `hevc_qsv`, `av1_qsv`
- VAAPI: `h264_vaapi`, `hevc_vaapi`, `av1_vaapi` using `/dev/dri/renderD128` when available
- CPU fallback: `libx264`, `libx265`, and available AV1 software encoders (`libsvtav1`, `librav1e`, `libaom-av1`)

For normal SDR conversion the engine tries hardware first and falls back to CPU automatically when the candidate fails. The planned encoder/hardware is returned in PlaybackPlan and the actual active encoder/speed/FPS/error is visible in Admin session diagnostics.

### HDR and tone mapping

When the source HDR mode is known to be unsupported by the client and the safe output is SDR/H.264, PlaybackPlan marks `tone_map=true` and adds `tone_map_sdr` to the transcode reasons.

The current reliable tone-map path uses FFmpeg `zscale` + software `tonemap` before encoding. Hardware-specific tone-map surfaces differ substantially by driver, so Playback Engine v5 intentionally prioritizes correctness over pretending that every NVENC/QSV/VAAPI environment can tone-map identically. Normal SDR transcodes still use hardware-first encoding.

If the installed FFmpeg build lacks the required tone-map filters, Admin reports tone-map readiness as limited and real-device QA is required before relying on HDR→SDR playback for that host.

## Web Player v5

`playback-core.js` owns planning/execution and `player-v5.js` / `player-v5.css` layer the v5 experience over the established Player v4 controls.

Player v5 adds:

- persistent Auto/Original/4K/1440p/1080p/720p/480p quality selection;
- seamless quality replanning at the current playback position;
- explicit Direct Play / Remux / Audio Transcode / Video Transcode state;
- source codec/resolution versus output codec/resolution;
- source/target bitrate;
- planned encoder and hardware acceleration;
- HDR tone-map indication;
- PlaybackPlan reasons/session diagnostics;
- desktop keyboard shortcuts while preserving touch/fullscreen/PiP/media-session behavior.

Web HLS continues using hls.js 1.7.1 with native-HLS fallback where available. This CDN dependency remains a future self-hosting consideration, not a server playback-policy dependency.

## StormFlix Android / Android TV / Fire TV 0.5.0

The native app remains a single Media3 package with touch and remote behavior. It publishes real MediaCodec video/audio capabilities and opts into PlaybackPlan v5 video transcoding.

Version **0.5.0 / versionCode 14** adds:

- PlaybackPlan v5 capability advertisement (`allow_video_transcode=true`);
- max transcode bitrate guidance: 25 Mbps for TV/Fire, 16 Mbps for phone/tablet;
- persistent playback quality selection in the native player menu;
- Auto/Original/4K/1440p/1080p/720p/480p choices;
- quality/source replanning without restarting from 00:00;
- playback information with source/output resolution, target bitrate, encoder, hardware and tone mapping;
- canonical playback-mode telemetry rather than human-readable mode strings;
- proper HLS/transcode session ID on playback DELETE so temporary cache is removed when leaving/changing episode;
- Media3 HLS support from the prior 0.4.1 work remains active.

Native multi-audio Direct Play is still preserved when any decoder-supported track can be selected locally. If only the desired audio is incompatible, the server keeps video stream-copy and produces AAC. If video itself is incompatible, PlaybackPlan v5 may instead produce explicit video transcode.

### Episodic autoplay

Web and native Android/TV/Fire use `/api/v1/media/{id}/neighbors` for previous/next identity. At natural episode completion, a 10-second **A seguir** countdown can start the next episode automatically. The preference is enabled by default and can be disabled. Manual previous/next controls remain available.

### Android release channel

`.github/workflows/android.yml` builds the installable APK and publishes APK + SHA-256 as Actions artifacts. On successful `main` pushes that include Android changes it creates a versioned GitHub Release `android-v<version>`. The automated APK is suitable for direct testing; store production signing remains a separate distribution concern.

## Admin playback/transcode diagnostics

Admin **Reproduzindo agora** retains the enriched playback diagnostics endpoint.

Admin **Saúde & Automação** now also contains a Playback Engine v5 transcode panel showing:

- FFmpeg version;
- acceleration detected (NVENC / Quick Sync / VAAPI / CPU);
- preferred H.264 encoder;
- active video-transcode session count;
- transcode cache usage/max and free-space reserve;
- tone-map readiness;
- per-session title/user/device mapping when available;
- source/output codec;
- target quality/bitrate;
- actual encoder/hardware;
- FFmpeg FPS and speed relative to realtime;
- session cache bytes;
- tone mapping and last error.

Direct Play, Remux and audio-only AAC sessions are intentionally not counted as video-transcode sessions in this panel.

## Managed legacy compatibility MP4 cache

`internal/webcompat/materialize.go` + `CacheManager` remain for legacy/unknown native and manual compatibility paths. Defaults remain 20 GiB max, 48h TTL, periodic cleanup, LRU target, free-space reserve and active-file protection.

Playback Engine v5 does not remove this legacy fallback, but Web/Android/TV/Fire use dynamic HLS paths instead of waiting for a whole compatibility MP4.

## Official Jellyfin Android / Android TV / Fire TV compatibility

The native StormFlix playback rewrite does **not** move the source of truth into Jellyfin. The compatibility facade remains isolated.

### Official Jellyfin Android phone/tablet

The official Android app is a WebView wrapper. StormFlix emits `/main.stormflix.bundle.js` only when a Jellyfin native JS interface is present. The deferred bridge keeps the StormFlix Web layout, mirrors the already-authenticated StormFlix session into the credential shape expected by the official wrapper and triggers `/Sessions/Capabilities/Full`. Tokens are derived server-side from the authenticated session; browser-supplied user IDs are not trusted.

### Official Jellyfin Android TV / Fire TV

The official TV application is native Android/Leanback, not a WebView. A server cannot replace its native UI with StormFlix HTML. Therefore:

- official Jellyfin TV/Fire keeps Jellyfin's native UI and consumes the compatibility facade;
- the StormFlix native app is the route for the StormFlix UI on TV/Fire;
- current compatibility surfaces include UserViews, Items, Latest, Resume, NextUp, PlaybackInfo, sessions/progress, images, seasons/episodes, streams and shaped empty optional DTOs where appropriate.

`QuickConnect/Enabled` remains false because StormFlix does not advertise an unsupported Jellyfin Quick Connect implementation. Compatibility 404/5xx calls remain traceable as `JELLYFIN_COMPAT_GAP` without logging secrets.

## Scanner identity, metadata and sources

Supported video library kinds include `movies`, `series`, `anime`, `mixed`, `anime_series`, `animation_series` and existing special kinds. `media_series_identity` owns source root, stable series key/title, season, episode and absolute episode. External metadata enriches but does not redefine folder identity blindly.

Western series/cartoons prefer `Scanner → TMDB TV → TheTVDB v4 → Fanart.tv`. Anime with seasons combines scanner identity with TMDB TV, TheTVDB, AniList/AniDB/MAL recovery and the HAMA-style Anime-Lists bridge.

Changing a Drive/rclone source root preserves media IDs when the replacement is an unambiguous one-for-one mapping. Metadata, artwork, subtitles and progress stay attached to the same `media_id`. Ambiguous relocations/collisions are not guessed.

## Principal-series manual matching

Admin → Catálogo defaults to **Obras principais**. Episodic manual matching lives in `series_metadata_overrides`; queued `series_refresh` updates current episodes and future episodes inherit the override. Normal episode rows are diagnostic children, not independent manual matches.

## Scan queue and safe simulation

`scan_jobs` is persistent FIFO and scans are serialized to protect rclone/FUSE + SQLite. Duplicate active jobs are avoided; queued/running work can be cancelled; restart requeues unfinished jobs; unreachable mounts preserve prior catalog.

`PreviewMulti` is the dry-run path used by Admin **Simular scan**. It traverses the same enabled roots as `ScanMulti`, reports existing/discovered/new/changed/missing/unchanged and never mutates catalog rows.

Protected scan, scan-all, library-path and recommended-category operations require a successful automatic database backup before execution.

## Smart Home menus and sections

`library_categories.parent_id` keeps the two-level presentation contract:

- `parent_id IS NULL` = **Menu da Home** in top navigation;
- direct child = **Seção da galeria** / rail inside that menu.

The parent/general rail is never replaced by child sections. Child sections stack by `sort_order`; empty sections are omitted. A title may appear in the general rail and again in a relevant child rail.

### Persistent smart rules

Category rules support library scope and filters for genres, media type, year, rating, recent additions, resolution, HDR/SDR, Brazilian-Portuguese audio/subtitles, technical dub/sub classification and metadata readiness.

Admin → **Menus da Home** supports custom root menus such as Desenhos, drag/drop order and live preview. Profile Home preferences continue to control root visibility/order without bypassing kids/library access.

## Technical catalog index

`media_technical` caches stream inspection keyed by `media_id` + `modified_unix`, including video codec/resolution/HDR/bitrate/duration, audio/subtitle languages and technical dub/sub classification.

The background indexer runs one ffprobe at a time to protect remote mounts. PlaybackPlan reuses a matching serialized technical `playback.Source` snapshot and falls back to live probe when necessary. A cache write failure after a successful probe must not turn a valid playback into failure.

Physical versions keep independent technical snapshots while logical cards remain deduplicated.

## Catalog health, backups and restore

Admin health tracks missing metadata/covers/genres, `Outros`, unavailable titles, technical backlog and duplicate physical versions. Duplicate grouping includes season/episode identity.

`system_backups` registers SQLite safety snapshots. Scans/path/category operations create or reuse backups according to their safety policy. Restore is staged to `<database>.restore`, verified with SQLite `quick_check` before activation, preserves a pre-restore copy and rolls back if activation fails. Media/assets are not deleted by DB restore.

## Large-catalog Web performance / artwork caching

Catalog rails render in bounded chunks with lazy image decode/fetch priority. Authenticated local assets use private browser caching; configured external `AssetPublicBaseURL` remains the CDN layer.

## Schema history

- Phase 9: `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, performance indexes.
- Phase 11: ordered series-child index and legacy episode-manual cleanup.
- Phase 12: persistent `scan_jobs` and metadata job type/series/provider fields.
- Phase 13: category smart rules, `media_technical`, `catalog_changes`, `profile_home_menus`, `system_backups` and playback diagnostic columns.
- Playback Engine v5 introduces **no destructive database migration**; transcode state is process/session scoped.

## Required validation before merge/release

For Playback Engine v5, all of the following are mandatory on the exact PR head:

- JavaScript syntax checks for public Web + Admin, including `player-v5.js`, `admin-transcode.js` and Jellyfin bridge code;
- `go test ./...`;
- `go test -race ./internal/playback ./internal/webcompat ./internal/transcode`;
- `go build -trimpath ./cmd/stormflix`;
- Android Gradle APK build because Android source/version changed;
- exact PR-head CI **and** Android workflow green before merge;
- post-merge `main` CI **and** Android workflow green before deployment is presented as ready.

## Real-world QA after deployment

Architecture/tests do not replace real host/device/storage validation. Check:

- compatible H.264/AAC media stays Direct Play with zero HLS/transcode cache;
- container-only mismatch uses Remux;
- incompatible selected audio uses AAC while video remains copy;
- unsupported HEVC/AV1/etc. on a client that advertises transcode produces a `video_transcode` H.264 route;
- explicit 1080p/720p/480p quality downshift preserves current position and produces the expected output resolution;
- `Original` preserves compatible Direct Play;
- 4K→1080p/720p on representative rclone media;
- HDR source on an SDR-only declared client uses tone mapping and colors remain correct;
- hardware SDR encode on each actually deployed GPU path (NVENC/QSV/VAAPI) where available;
- CPU fallback works when hardware candidate is unavailable/fails;
- sustained transcode speed remains above realtime for the expected concurrent user count;
- transcode cache never exceeds budget/free-space reserve and disappears promptly after playback closes;
- Android phone, Android TV and Fire TV quality changes do not reset progress;
- audio/subtitle selection, previous/next and 10-second autoplay continue working after transcode/quality changes;
- Admin Playback Engine v5 panel reports encoder, hardware, FPS, speed, cache and errors correctly;
- official Jellyfin compatibility routes remain unaffected.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Never store secrets, credentials or private media paths.