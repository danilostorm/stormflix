# StormFlix Changelog

This file records user-visible and architectural changes. `PROJECT_STATE.md` is the authoritative current-state handoff.

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
- Direct Play remains first choice; video is never silently transcoded or tone-mapped.
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
