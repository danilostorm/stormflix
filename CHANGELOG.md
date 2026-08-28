# StormFlix Changelog

This file records user-visible and architectural changes. `PROJECT_STATE.md` is the authoritative current-state handoff.

## 2026-08-28

### Dynamic Web HLS and session cache cleanup

- Replaced automatic Web whole-file compatibility MP4 materialization with **dynamic fMP4 HLS** for browser remux/audio-compatibility playback.
- Direct Play remains unchanged: compatible mounted media is served directly with HTTP Range and uses **zero StormFlix HLS cache**.
- Browser compatibility playback now generates only small on-demand batches (default 6-second segments, 4 segments/about 24 seconds per batch) instead of rewriting a complete 20–50+ GiB movie before playback can begin.
- Fixed periodic Web buffering on rclone/Google Drive sources: after serving a segment, StormFlix now warms the **next 24-second HLS batch in the background** instead of waiting until the browser reaches the batch boundary. Prefetch remains bounded to one batch ahead, yields to seeks/user-requested batches and still obeys the 5 GiB global HLS cache plus free-disk reserve.
- HLS execution always keeps video stream-copy; exact selected audio is copied when compatible or converted to AAC-LC only when required.
- Added pinned hls.js execution for MSE browsers with native-HLS fallback where available.
- Added a dedicated session-scoped `<DataDir>/hls-cache` with a **5 GiB hard global budget shared by all users**, plus the existing 10 GiB/5% free-disk reserve.
- The HLS manager reserves capacity before launching a batch, evicts oldest disposable fragments when needed, protects sessions with active FFmpeg workers and refuses new generation rather than filling the SSD.
- Old HLS fragments behind current playback are removed continuously.
- **Closing or finishing a movie immediately cancels that session's FFmpeg worker and deletes its entire HLS cache directory.** Browser unload also sends best-effort keepalive cleanup.
- Source/version switches remove the previous HLS fragments before reusing a logical playback session; switching to Direct Play clears any prior HLS session cache as well.
- Added 30-minute idle-session cleanup as a crash/disconnect fallback, and startup removes all orphaned HLS fragments from previous server processes.
- Added ownership/path-safety checks so one authenticated user cannot delete another user's HLS session and session IDs cannot escape the dedicated cache directory.
- Expanded Web audio capability probing with DTS and FLAC checks.
- Added HLS lifecycle/budget/session-isolation regression tests; existing playback/cache race CI covers the new manager.
- Server version advanced to **0.19.0-dynamic-hls**.

## 2026-08-27

### Web Player v4 and managed compatibility cache

- Replaced the old-looking browser playback shell with **StormFlix Web Player v4**, a visibly new cinematic player presentation layered over the native PlaybackPlan engine.
- Player v4 adds SVG controls, custom played/buffered timeline, seek preview, playback-mode/resolution/quality indicators, previous/next episode controls, audio/subtitle/settings access, fullscreen, native Picture-in-Picture and responsive desktop/mobile layouts.
- The visual redesign does not fork playback policy: Direct Play/remux/AAC decisions still come from the shared native Playback Core.
- Replaced the unbounded `data/compat-cache` behavior with a managed compatibility Cache Manager.
- Default cache policy is **20 GiB maximum + 48h TTL**, periodic cleanup every 15 minutes and LRU eviction down to an 85% target.
- Compatibility cache tracks its own persisted `last_used_at` metadata instead of depending on filesystem atime.
- Files being materialized or served are protected from eviction; abandoned temporary files are cleaned without deleting active FFmpeg output.
- Oversize compatibility artifacts larger than the configured cache limit are treated as short-lived entries and expire after they become idle rather than permanently pushing the cache above its budget.
- Before creating a large compatibility MP4, StormFlix applies disk-pressure eviction and preserves at least **10 GiB or 5% free filesystem space**, whichever is larger by default.
- Startup cleanup safely adopts/evicts legacy compatibility MP4 files, so servers that accumulated a large pre-manager cache are brought back under policy without touching the database, artwork, source media or other DataDir content.
- Added Admin → Configurações → **Playback · Cache de compatibilidade** with current usage, limit, file count, active files, oldest entry, free space, maximum size, TTL, auto-cleanup, disk reserve and **Limpar cache agora**.
- Added authenticated admin status/cleanup APIs for the playback cache and reload cache policy immediately when settings are changed.
- Added compatibility-cache regression tests for LRU, TTL, active-file safety, active/abandoned temporary files, oversize entries, manual cleanup isolation, path safety and unlimited mode.
- CI now checks Player v4 JavaScript syntax and runs `go test -race` for playback/cache packages.
- Server version advanced to **0.18.0-player-cache**.

### Unified playback completion

- Completed the native StormFlix Playback Core for **Web, Android, Android TV and Fire TV** behind the same `/api/v1/media/{id}/playback/plan` contract.
- Playback planning now carries real client capabilities for containers, video/audio codecs, maximum decode resolution/frame-rate, known HDR support, optional bitrate limits, subtitle formats, PiP and Media Session support.
- Source probing now records video dimensions, frame rate, HDR transfer mode and bitrate in addition to codecs/container.
- Direct Play remains the first choice and **video is never silently transcoded or tone-mapped**. Unsupported video capability is returned explicitly.
- Fixed MP4 compatibility planning so DTS/other non-MP4-copy-compatible audio cannot be mislabeled as a pure remux; video remains stream-copy while only audio falls back to AAC-LC.
- Added exact `audio_stream` propagation from PlaybackPlan through compatibility preparation and range-served remux output. The execution adapter can no longer silently choose a different language track from the one selected by the planner.
- Browsers now declare server-side audio selection because HTML multi-audio selection is not reliably exposed across engines. If the preferred track is not the default track, StormFlix performs a stream-copy remux that pins the preferred audio without re-encoding video.
- Web source switches preserve the same playback session, keep ordered progress monotonic, expose Picture-in-Picture when supported and maintain Media Session position state.
- Removed the web planner-outage raw-stream bypass: native StormFlix playback policy now remains authoritative rather than silently falling back to a second source-selection path.
- Android/TV Media3 now publishes actual MediaCodec decoder capabilities and consumes PlaybackPlan before loading every source.
- Android/TV preserves native multi-audio Direct Play when at least one local audio decoder can handle a track, while server-side AAC compatibility remains available when no usable track exists.
- Android alternative-source fallback now tests each physical version through PlaybackPlan rather than guessing compatibility only from labels alone.
- Added native Media3 `MediaSession`; phone/tablet playback adds Picture-in-Picture while TV/Fire keeps remote/D-pad-first behavior.
- Android app advanced to **0.3.0 / versionCode 11**.
- Added regression tests for native multi-audio selection, exact remux audio execution, HDR/resolution/frame-rate policy, bitrate policy, server-selected browser audio and playback-session URL behavior.

### Catalog identity and metadata

- Added scanner-owned episodic identity (`media_series_identity`) so folder hierarchy determines show/season/episode before external metadata providers.
- Added `animation_series` for western cartoons and `anime_series` for anime organized as television seasons.
- Improved parsing for technical subfolders, compact episode names and Brazilian season folders.
- Added TheTVDB v4 as optional series/cartoon fallback with Admin settings for API Key and optional PIN.
- Added native HAMA-style Anime-Lists bridge between AniDB/AniList/MAL and TVDB/TMDB identifiers.
- Added series-level manual metadata overrides.
- **Corrected the manual-match workflow to be principal-series only:** Admin → Catálogo now defaults to `Obras principais`, grouping an episodic library into one card per `series_key` instead of one card per episode.
- Matching a principal series stores one provider decision and rebuilds scanner identities; the episode reorganization is now an observable queued `series_refresh` job instead of an invisible background goroutine.
- Future episodes inherit the same series override automatically.
- `Arquivos / diagnóstico` remains available for low-level inspection, but episodic rows no longer expose manual matching and instead point back to the principal work.
- Added phase11 cleanup so legacy builds that marked every episode `manual_match=1` flags are normalized; series protection now lives only in `series_metadata_overrides`.
- Added a regression test requiring two episodes of the same scanner series to appear as one principal work in the Admin catalog.

### Queue, scans and background activity

- Added phase12 persistent `scan_jobs` queue.
- Individual library scans now enter a serialized FIFO queue instead of creating uncontrolled parallel scans.
- Added **Bibliotecas → Escanear todas**, which queues every active library and processes them one at a time.
- Added queue-safe cancellation for both queued and running scans using the same cancellable context as the actual scan worker.
- Added restart recovery for unfinished scan jobs.
- Added Admin → **Fila & atividades**, merging scan jobs, normal metadata jobs and principal-series episode refreshes into one operational view.
- Added live job status, progress, current scanner message, matched/success/error counts and recent history.
- Principal-series refresh jobs persist series/provider information so they can resume after a server restart.
- Added a regression test that queues two libraries through **scan all** and requires both jobs and media discovery to complete.

### Admin UI

- Fixed a race in `Metadados & Capas` that could render the **Agentes de Música** panel twice.
- Library cards now reflect queued/running scan state and expose queue cancellation/removal.
- Added a compact queue strip to Bibliotecas with **Escanear todas** and **Ver fila completa**.

### Categories and site layout

- Added parent/child category hierarchy.
- Parent categories aggregate descendant libraries while child categories remain individually browsable.
- Added Admin category tree editor.
- Added **Organizar estrutura recomendada** in Admin → Categorias. It creates/updates managed children under Filmes/Séries/Animes while preserving custom categories.
- Recommended automatic sibling assignments are exclusive where possible to avoid duplicate rails.
- The main site now visibly shows **Explorar por categoria** with root groups and subcategory chips.
- Opening a root category with children renders **one rail per subcategory** instead of mixing everything into one large rail.
- Added sticky secondary subcategory navigation with split view, direct child access and `Tudo em <categoria>`.

### Home performance

- Added phase10 SQLite read indexes for artwork, recent/title ordering, ratings/status and series identity.
- Added 20-second access-scope-aware cache for the static grouped Home feed.
- Continue Watching remains uncached and profile-specific.
- Parallelized independent Home reads (static grouped catalog, Continue Watching, two trending windows and Releases).

### Jellyfin compatibility

- Jellyfin discovery now advertises a numeric compatible server version and supports official-client root paths.
- Added mobile WebView startup bridge.
- Added Android TV/Fire post-login endpoints, modern `/UserViews`, `/UserItems/Resume`, `/Items/Latest` aliases and client crash logging.
- Fixed required `UserDto` fields and non-null Jellyfin `UserData` objects.
- Fixed Jellyfin artwork delivery for clients whose image loader does not carry the native StormFlix session.

### Playback foundation

- Added `docs/PLAYBACK_ARCHITECTURE.md` defining the native StormFlix playback architecture for Web, Android and TV/Fire while keeping the Jellyfin facade isolated.
- Added `internal/playback` with a capability-driven source probe and deterministic Playback Decision Engine for `direct_play`, `remux`, `audio_compatibility` and explicit `unsupported` results.
- Added native `POST /api/v1/media/{id}/playback/plan`; it applies library/profile access, profile audio preference, resume state and creates a playback session without routing native clients through Jellyfin compatibility.
- StormFlix Web now detects browser codec/container support at runtime and requests a Playback Plan before loading the media source.
- Browser playback preserves Direct Play first, reuses the existing seekable remux/AAC execution path when required and does not silently introduce video transcoding.
- Web playback heartbeats now send the server-issued playback session, monotonic sequence and event timestamp so the existing ordered-progress protection is actually used by the browser client.
- Added protection against late planner responses starting hidden playback after the player is closed.
- Bumped the native server version to `0.17.0-playback-core`.
- Restored preferred Portuguese audio selection before codec fallback.
- AAC compatibility fallback keeps video stream-copy and prepares seekable cached MP4 output with HTTP range support.
- Ordered playback progress prevents stale requests from overwriting newer resume positions while allowing intentional backward seeks.
- Replaced repeated Fire TV seek Toasts with bounded in-player seek feedback.

## Documentation maintenance

From this point forward, meaningful architecture/compatibility/schema/playback updates should update this file and `PROJECT_STATE.md` in the same development round.