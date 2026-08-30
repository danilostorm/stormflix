# StormFlix Changelog

This file records user-visible and architectural changes. `PROJECT_STATE.md` is the authoritative current-state handoff.

## 2026-08-30

### Android 0.5.3 PlaybackPlan JSON compatibility retry

- Bumped the native Android/Android TV/Fire TV package to **0.5.3 / versionCode 17** after a real phone reproduced a black player at `00:00 / 00:00` with the toast `Não foi possível preparar a reprodução: invalid JSON ...`.
- Confirmed that the failure happens before Media3 receives a media URL: the native app is rejected while POSTing `/api/v1/media/{id}/playback/plan`.
- The app still sends the complete Playback Engine v5 capability document first. Only when that endpoint specifically returns HTTP 400 with an invalid-JSON message does `ApiClient` retry once with a conservative capability envelope containing the core container/video/audio support needed for authoritative planning.
- The compatibility retry remains inside PlaybackPlan and therefore preserves **Direct Play first**. It does not silently enable video transcoding or bypass the server planner, and unrelated HTTP errors are not hidden.
- Updated native API User-Agent and PlaybackPlan `client_version` to 0.5.3.
- Added real-device QA coverage requirement for this exact full-capability rejection → minimal-capability retry path.

### Playback Engine v5, Web Player v5 and Android 0.5.0

- Advanced the server to **0.21.0-playback-v5** and the native Android/Android TV/Fire TV app to **0.5.0 / versionCode 14**.
- Preserved **Direct Play first** as the primary playback rule. Video transcoding is never a hidden bypass: it is selected only by the authoritative PlaybackPlan when the client explicitly advertises `allow_video_transcode` or when the user explicitly selects a quality lower than the source.
- Added the native `video_transcode` PlaybackPlan mode after Direct Play → Remux → audio-only AAC compatibility. Unsupported video codec, decoder resolution/FPS/HDR limits, explicit Direct Play bitrate limits and user quality caps now produce a structured transcode plan when a safe target exists instead of simply failing playback.
- Added persistent playback quality options **Auto / Original / 4K / 1440p / 1080p / 720p / 480p**. Choosing a lower quality than a compatible source now intentionally produces `reason_code=quality_limit`; `Original` continues preserving compatible Direct Play and the system never upscales lower-resolution media.
- Added the bounded `internal/transcode` fMP4 HLS engine. Video is generated on demand in small batches rather than materializing an entire movie from rclone/Drive before playback begins.
- Transcode cache defaults to **5 GiB globally**, 4-second fMP4 segments, five segments per batch, 20-minute crash/idle cleanup and the existing 10 GiB/5% free-disk safety reserve. Playback close/end cancels the worker and deletes the session directory immediately.
- Added FFmpeg encoder discovery and hardware-first SDR encoding with automatic CPU fallback: NVIDIA NVENC, Intel Quick Sync, VAAPI and CPU encoders are selected according to what the installed FFmpeg actually exposes.
- Added explicit HDR→SDR tone-map planning. The reliable v5 path uses `zscale` + software `tonemap` before encoding when the client cannot accept the source HDR mode; Admin exposes whether the host FFmpeg has the required filters.
- Added per-session transcode diagnostics including source/output codec, output resolution/bitrate, chosen encoder/hardware, FPS, realtime speed, cache bytes, tone mapping and last FFmpeg error.
- Added **Web Player v5** over the existing Player v4 control base, with quality selection, PlaybackPlan diagnostics, Direct Play/Remux/Audio Transcode/Video Transcode status and source→output technical information while preserving seek, PiP, fullscreen, subtitles, audio and episode controls.
- Web quality changes re-plan at the current playback position and preserve play/pause state. Browser capabilities now explicitly advertise video-transcode support and a conservative maximum target bitrate derived from available network information when present.
- Added native Android/TV/Fire quality controls with the same Auto/Original/4K/1440p/1080p/720p/480p choices. Quality changes persist locally, request a new PlaybackPlan and restore the current playback position instead of restarting from 00:00.
- Android player information now reports the canonical playback mode, source/output resolution, target bitrate, encoder, hardware and tone mapping. The app also sends the playback session ID when closing a session so HLS/transcode cache is cleaned immediately.
- Android/TV/Fire continues preserving native multi-audio Direct Play when possible; if only the selected audio is incompatible, video remains stream-copy and only audio becomes AAC.
- Added an Admin **Playback Engine v5** panel under Saúde & Automação showing detected FFmpeg/hardware, preferred H.264 encoder, active video-transcode sessions, cache usage, tone-map readiness and live per-session encoder/FPS/speed/error metrics.
- Expanded decision regressions for explicit quality downshift and Original-quality Direct Play, and CI now syntax-checks the transcode Admin UI and race-tests `internal/transcode` alongside playback/webcompat.
- Jellyfin compatibility remains isolated and unchanged as a source-of-truth boundary. SQLite/PostgreSQL work remains a separate database migration phase rather than being mixed into this playback release.

### Android 0.4.1 autoplay, instant compatibility streaming and Releases

- Added streaming-style **next episode autoplay** to Web, Android, Android TV and Fire TV. Episodic playback uses the existing `/media/{id}/neighbors` identity and presents a 10-second “A seguir” countdown before starting the next episode.
- Autoplay is enabled by default but remains user-controllable. Web stores the preference locally and exposes it in Player v4 settings; Android/TV/Fire stores the setting in app preferences and exposes it in the native player menu.
- Bumped native Android to **0.4.1 / versionCode 13** and added the Media3 HLS module.
- Removed the long Android/TV compatibility startup caused by complete seekable-MP4 materialization. StormFlix Android, Android TV and Fire TV now use the same **dynamic fMP4 HLS** compatibility engine as Web: small on-demand batches, bounded prefetch, video stream-copy and AAC conversion only for an incompatible selected audio track.
- Direct Play remains first and unchanged: compatible mounted/rclone media still streams directly with HTTP Range and creates no HLS compatibility cache.
- Unknown/legacy native clients retain the old seekable-MP4 compatibility fallback until they explicitly use a supported streaming path.
- Android builds now publish versioned APK + SHA-256 files to **GitHub Releases** after a successful push/build on `main`; Actions artifacts remain available for CI diagnostics.
- Added server regression coverage for Web/Android/TV/Fire dynamic-HLS selection and CI syntax coverage for the Web autoplay controller.
- PostgreSQL migration was deliberately **not mixed into this playback release**. SQLite remains the production database for 0.4.1; a database migration requires a separate dual-backend/migrator phase because current migrations and queries contain SQLite-specific PRAGMAs, `INSERT OR IGNORE`, `datetime(...)`, `COLLATE NOCASE` and backup/restore semantics.

## 2026-08-29

### Android 0.4.0 and official Jellyfin client compatibility v2

- Audited the current official `jellyfin/jellyfin-android` WebView connection flow and `jellyfin/jellyfin-androidtv` native SDK usage instead of guessing client endpoint names.
- Added an official Jellyfin Android WebView bridge. When the official Android wrapper loads the StormFlix server root, its existing `main.*.bundle.js` interception injects Jellyfin NativeShell while StormFlix continues rendering the StormFlix Web layout.
- After a successful StormFlix Web login, `/api/v1/compat/jellyfin-mobile-bridge` derives the authenticated user/token from the same-origin HttpOnly StormFlix session, writes the `jellyfin_credentials` shape expected by the official wrapper and triggers `/Sessions/Capabilities/Full` so the native Android shell can finish session setup.
- The WebView bridge is limited to pages exposing Jellyfin native JS interfaces and removes only the current StormFlix origin's mirrored credentials after logout/401. Ordinary browsers are unaffected.
- Documented the unavoidable platform boundary: official **Jellyfin Android TV / Fire TV is native Leanback UI**, not a WebView, so a server cannot replace that UI with HTML. The StormFlix native app is the route for the StormFlix layout on TV/Fire; official Jellyfin TV/Fire consumes the compatibility facade.
- Expanded the isolated Jellyfin facade with current-client startup and optional surfaces including Startup Configuration, Quick Connect capability declaration, Sessions list, Genres, Persons, Suggestions, Upcoming, Similar, Theme/Special/Intro query endpoints and Playback BitrateTest while preserving existing UserViews/Items/Latest/Resume/NextUp/PlaybackInfo/seasons/episodes/images/streams/progress routes.
- Unsupported optional Jellyfin features return correctly shaped empty DTOs rather than fake catalog state. Quick Connect explicitly advertises `false` because StormFlix does not implement that protocol.
- Jellyfin tracing now marks 404/5xx client calls as `JELLYFIN_COMPAT_GAP`, allowing real Android/TV/Fire traffic to identify future compatibility gaps without logging tokens or passwords.
- Added route-registration regression coverage for the official Android/TV compatibility matrix and expanded the embedded WebView bridge test to lock `jellyfin_credentials`, bridge URL, `Sessions/Capabilities/Full` and token-header behavior.
- CI now syntax-checks `main.stormflix.bundle.js`.
- Bumped native StormFlix Android to **0.4.0 / versionCode 12** and updated its User-Agent.
- Android Home navigation is now server-driven by `/categories` plus selected-profile `/profiles/home-menus`, so custom root menus such as **Desenhos** appear automatically on phone, Android TV and Fire TV and follow profile visibility/order.
- Added `CategoryBrowseActivity`, a generic native two-level menu browser that keeps the parent aggregate and renders configured child gallery sections through the same `/categories/{slug}/smart` server rules used by Web.
- Direct Play, exact audio selection, PlaybackPlan authority and the no-silent-video-transcode invariant are unchanged for that release.

### Platform automation and catalog intelligence

- Added **smart Home sections** that can select titles by library scope and/or persistent rules: genre, media type, year range, minimum rating, recent additions, minimum/maximum resolution, HDR/SDR, Brazilian-Portuguese audio, Brazilian-Portuguese subtitles, dub/sub status and metadata readiness.
- Admin → **Menus da Home** now supports drag-and-drop ordering for root menus and child rails, live rule previews before saving and clear smart-section summaries.
- The recommended organizer now creates technically driven **Filmes 4K / UHD**, **Animes Dublados** and **Animes Legendados** rails instead of relying only on folder/library names. Technical rules are evaluated from real stream metadata.
- Added a background technical catalog index (`media_technical`). It runs **one ffprobe at a time** to protect Google Drive/rclone/FUSE mounts, caches results by source `modified_unix`, detects codec/resolution/HDR/audio/subtitle languages, retries transient failures after a cooldown and can be explicitly requeued from Admin.
- Added Admin → **Saúde & Automação** with actionable counts for missing metadata/covers/genres, `Outros`, unavailable media, technical-analysis backlog and duplicate physical versions.
- Added automatic SQLite safety backups before catalog-changing scan/path/category operations. Automatic backups are reusable for 30 minutes and retain the newest 10; missing backup files cannot satisfy the safety gate.
- Added manual backup/list/restore controls. Restore is staged as `<db>.restore`, fsynced, verified with SQLite `quick_check` at next startup and activated before opening the primary connection. The previous database plus any WAL/SHM sidecars are preserved as a pre-restore safety copy, and a failed activation rolls the original files back into place.
- Added per-profile Home menu visibility/order through `profile_home_menus`; public navigation applies the selected profile preferences without bypassing profile/kids/library access filtering.
- Added large-catalog Web rendering in bounded chunks (28 cards per rail), IntersectionObserver loading, lazy image decode/fetch priority and one-day private browser caching for authenticated local artwork. External `AssetPublicBaseURL` remains the CDN path.
- Added Web playback telemetry for current buffer depth, estimated read throughput, source bitrate, codec, playback mode and HLS cache usage. Admin “Reproduzindo agora” receives the enriched superset without breaking older consumers.
- Dynamic HLS prefetch is now adaptive: normal operation stays one batch ahead and low buffer/slow reads may raise speculative headroom to at most three small batches. The existing **single global HLS SSD budget and free-space reserve remain hard limits**, and video remains stream-copy only on that compatibility engine.
- Added schema **Phase 13** for smart rules, technical media cache, catalog audit history, profile Home preferences, backup registry and playback diagnostics.
- Added regression tests for smart rule normalization, pt-BR dub/sub classification, episode-safe duplicate detection, read-only scan preview, adaptive HLS bounds, Phase 13 migration and validated staged database restore.
- CI now syntax-checks the automation/large-catalog scripts in addition to existing Web/Admin scripts, runs the full Go suite, playback/webcompat race tests and the server build.
- Server version advanced to **0.20.0-platform-automation**.

### Home menus and gallery sections

- Reformulated the old category-tree presentation into a clearer two-level Home model: **root categories are Home menu buttons** and their direct children are **gallery sections**.
- Admin → **Menus da Home** has a dedicated visual editor for creating custom root menus such as **Desenhos**, editing their display order/type, and adding sections directly under each menu.
- Child sections such as **Animes Dublados** never appear in the top menu, category explorer or secondary navigation.
- Fixed the first real-catalog regression found after rollout: creating manual sections no longer replaces the parent catalog rail. Opening **Animes**, for example, keeps the general **Animes** rail first and stacks **Animes Dublados**, **Filmes Animes** and other configured sections below it in order.
- Each configured section is independent. A title may legitimately appear in the general parent rail and again in a relevant section, matching normal streaming-service shelf behavior.
- Manual gallery sections are now **library-scoped**: the libraries selected in Admin define which works belong to that section. The section type no longer hides a TMDB movie just because the section is labeled Anime, which fixes mixed libraries such as **Filmes Animes**.
- The parent aggregate merges titles returned by its own category scope with titles exposed by child sections, preserving anime films or other intentionally mixed works in the general rail.
- Fixed series identity merging in the Web category UI to preserve string series IDs instead of coercing them to numbers.
- A root with no manual sections keeps the previous automatic primary-genre grouping as a compatibility fallback.
- The recommended organizer understands the new model. When `animation_series` libraries exist it creates/reuses a **Desenhos** Home menu and places the reserved `series-desenhos` section below it; the legacy `series-animes` section under Séries is disabled when the organizer is explicitly run.
- Added responsive Admin styling for Home menu cards, section rows, library chips and the menu/section editor.
- Added regression coverage for the recommended **Desenhos** hierarchy and for an Anime-labeled gallery section backed by a mixed library containing movie metadata.
- CI syntax-checks `admin-categories.js` and the public category navigation.

### Drive path relocation and stable Admin operations

- Changing a library's Drive/rclone source root now preserves existing catalog media IDs when the old/new roots form an unambiguous one-for-one replacement.
- Existing media paths are rewritten to the new physical root instead of creating new catalog items; episodic `media_series_identity.source_root` follows the relocation.
- Because metadata, artwork, subtitles, watch progress and related records stay attached to the same `media_id`, a mount-path change alone no longer requires downloading covers or matching metadata again.
- A subsequent normal scan updates the already-relocated media row at the new path rather than recreating it. Non-refresh metadata jobs continue skipping already matched items.
- Relocation deliberately avoids guessing when source additions/removals are ambiguous and does not destructively merge path collisions.
- Added regression coverage proving that media ID, matched provider metadata and selected artwork survive a source-root change and the next scan.
- Fixed the Admin flashing during long scan-all operations: library polling now updates status/counts/source health/buttons in place when the library structure itself did not change.
- Fixed the same full-page redraw behavior in **Metadados & Capas**: summary counters, job progress and action state are patched without rebuilding the entire section every polling interval.
- **Metadados & Capas** now includes bulk **Buscar em todas** and **Atualizar todas** actions while retaining individual per-library controls. Bulk jobs run sequentially across active video libraries; normal bulk search preserves already matched titles, while explicit update-all refreshes them.

## 2026-08-28

### Automatic exclusive categories in pt-BR

- Filmes, Séries and Animes root pages now organize catalog content automatically from metadata instead of requiring manual genre-category maintenance.
- Existing configured child categories keep first priority; overlapping sibling categories are de-duplicated so the first configured category claims the title.
- Every remaining title is assigned to **one primary automatic genre only**. A movie/series is not repeated across several automatic genre rails just because the metadata contains multiple genres.
- Added Brazilian-Portuguese normalization for common TMDB/TV/anime genres, including Ação, Aventura, Animação, Comédia, Documentários, Família, Ficção científica, Terror, Suspense, Sobrenatural, Esportes, Cotidiano, Notícias, Novelas and related labels.
- Unknown foreign genre strings are not exposed directly in the UI. Titles with missing/unrecognized genre metadata fall into **Outros** until metadata is corrected/refreshed.
- Empty sections are never rendered and `Outros` stays last.
- Removed the simultaneous `Todos em ...` rail from automatic root views because it duplicated every title. `Tudo em <categoria>` remains an explicit separate aggregate view when configured child categories exist.
- Individual child categories may also be subdivided by the same exclusive pt-BR metadata classifier.
- No database/schema migration; profile/library permissions remain unchanged.

### Streaming-style category sections

- Category root pages were changed from a single giant catalog rail to multiple sections.
- Configured child categories can render as their own rails.
- Metadata genres are used to build additional streaming-style sections automatically.

### Dynamic Web HLS and session cache cleanup

- Replaced automatic Web whole-file compatibility MP4 materialization with **dynamic fMP4 HLS** for browser remux/audio-compatibility playback.
- Direct Play continues to serve compatible mounted media directly with HTTP Range and uses **zero StormFlix HLS cache**.
- Browser compatibility playback generates small batches (default 6-second segments, 4 segments/about 24 seconds per batch) rather than rewriting an entire remote movie before playback starts.
- Added bounded one-batch-ahead prefetch to prevent periodic stalls on rclone/Google Drive mounts.
- HLS execution always keeps video stream-copy; exact selected audio is copied when compatible or converted to AAC-LC only when required.
- Added pinned hls.js execution for MSE browsers with native-HLS fallback where available.
- Added a dedicated session-scoped `<DataDir>/hls-cache` with a **5 GiB global budget shared by all users** and the existing 10 GiB/5% free-disk reserve.
- Old HLS fragments behind current playback are removed continuously.
- **Closing or finishing a movie immediately cancels that session's FFmpeg worker and deletes its entire HLS cache directory.** Browser unload also sends best-effort cleanup.
- Added 30-minute idle cleanup for crash/disconnect recovery and startup orphan cleanup.
- Added ownership/path safety and HLS lifecycle/budget tests.
- Server version advanced to **0.19.0-dynamic-hls**.

## 2026-08-27

### Web Player v4 and managed compatibility cache

- Replaced the previous browser playback presentation with **StormFlix Web Player v4** while keeping the shared native PlaybackPlan engine authoritative.
- Added SVG controls, custom played/buffered timeline, seek preview, mode/resolution indicators, previous/next episode, audio/subtitle/settings access, fullscreen, native PiP and responsive desktop/mobile behavior.
- Replaced unbounded `compat-cache` behavior with a managed compatibility Cache Manager.
- Default persistent compatibility cache policy: **20 GiB maximum + 48h TTL**, periodic cleanup, LRU eviction, oversize cleanup, active-file protection and free-disk reserve.
- Added Admin cache status/settings/manual cleanup and regression tests.
- CI checks Player v4 syntax and playback/cache race tests.

### Unified playback completion

- Completed the native Playback Core contract for Web, Android, Android TV and Fire TV under `/api/v1/media/{id}/playback/plan`.
- Direct Play remains first choice; that release did not yet include video transcoding.
- Added exact audio-stream propagation, preferred-language selection, browser server-side audio pinning where required and ordered progress/session handling.
- Android/TV publishes MediaCodec capabilities, preserves multi-audio Direct Play where possible and evaluates alternative sources through PlaybackPlan.

### Catalog identity and metadata

- Added scanner-owned episodic identity (`media_series_identity`) so folder hierarchy determines show/season/episode before external metadata providers.
- Added `animation_series` and `anime_series` library kinds.
- Added TheTVDB v4 fallback, HAMA-style anime mapping and series-level metadata overrides.
- Principal-series manual matching is stored once and applied to current/future episodes through queued `series_refresh` work.

### Queue, categories and performance

- Added persistent serialized `scan_jobs`, queue-safe cancellation and restart recovery.
- Added Admin → Fila & atividades and scan-all controls.
- Added parent/child category hierarchy and Admin category tree tooling.
- Added SQLite read indexes, scoped Home cache and parallelized independent Home reads under WAL.

### Jellyfin compatibility

- Expanded official Jellyfin client compatibility routes, authentication, Home/resume/latest aliases, user DTO requirements and Android TV/Fire support while keeping the facade isolated from native StormFlix APIs.