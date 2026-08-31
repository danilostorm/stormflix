# StormFlix Changelog

This file records user-visible and architectural changes. `PROJECT_STATE.md` is the authoritative current-state handoff and `ENTERTAINMENT_ROADMAP.md` tracks planned work.

## 2026-08-31

### Profile avatar integrity, faster Home and Trakt per profile

- Fixed the profile-avatar regression caused by safe cleanup: active local `profiles.avatar_url` files now count as referenced assets and cannot be deleted as orphans. Historical missing files fall back to the profile color/initial instead of showing a broken image.
- Reordered Web bootstrap so profile selection happens before the expensive Home request. Single-profile accounts no longer load Home twice and multi-profile accounts no longer load a hidden Home behind “Quem está assistindo?”.
- Grouped Home now uses **2 minutes fresh + 10 minutes stale-while-revalidate**. A valid stale snapshot returns immediately while one background refresh rebuilds the static catalog.
- Delayed/rate-limited automatic collection backfill away from first-Home startup and added Phase 15 SQLite indexes for selected artwork, available/recent media and collection queries.
- Added Phase 15 **Trakt per profile** integration. The administrator configures one Trakt application while every profile authorizes its own Trakt account with Device OAuth.
- Trakt Client ID/Secret can be configured in Admin or through environment defaults. Persisted application credentials and profile access/refresh tokens are encrypted with the existing AES-GCM settings key.
- OAuth refresh stores the newly returned access + refresh token together; profile ownership is checked on every account endpoint and application credentials hot-reload without a server restart.
- Local StormFlix progress remains authoritative. Trakt scrobble is asynchronous, timeout-bounded and throttled, so a Trakt outage never blocks playback.
- Added profile connect/code/poll/disconnect UI, Admin Trakt configuration UI and regression tests for Phase 15 plus encrypted Trakt credentials.
- Added `ENTERTAINMENT_ROADMAP.md` covering Home latency, Skip Intro/Créditos, Smart Downloads, Watch Party, smart playlists, rewind-on-resume, still-watching protection, editions/extras, authentication improvements, reusable media analysis and the native Jogos module.

### Automatic movie collections and stable Admin cleanup

- Added automatic movie-franchise grouping using TMDB `belongs_to_collection` instead of title heuristics.
- Phase 14 persists collection identity and invalidates it when the movie TMDB match changes.
- Existing matched movies are backfilled by a single low-rate background worker.
- Added permission-aware `/api/v1/media?group=collections&minimum_size=2` and automatic Web **Coleções** navigation.
- Fixed Admin → Limpeza so only one renderer owns the page; stale async responses no longer replace the optimization view.

### 4K device policy, cheaper live transcode and lossless assets

- Dedicated 4K/UHD shelves use client display/decoder hints without hiding UHD titles from ordinary catalog/search surfaces.
- Android 0.6.3 derives UHD capability from real `MediaCodecList` decoders.
- Compatible UHD remains original Direct Play; audio-only incompatibility keeps source video copied.
- Automatic incompatible UHD video transcode under Auto/Original is capped at **1080p / 8 Mbps** instead of 4K→4K.
- Hardware encoders remain preferred; CPU H.264 fallback uses a cheaper preset and UHD→1080 uses a low-cost scaler.
- Added lossless SHA-256/hardlink asset deduplication plus logical-vs-physical asset storage reporting.

### Safe nested library source ownership

- Parent/child source roots may belong to different logical libraries.
- The most-specific configured source owns a subtree; parent scans prune delegated children to avoid duplicate catalog items.
- Exact duplicate roots across libraries and redundant parent/child roots in one library remain blocked.
- Parent-before-child migration, offline-source behavior and scan preview use the same ownership rules.

## 2026-08-30

### Samsung Tizen / LG webOS and managed movie roots

- Added StormFlix Tizen 0.1.0 and LG webOS 0.1.0 thin shells using the hosted StormFlix Web UI/Web Player.
- Added Smart TV CI, Tizen certificate-ready packaging and webOS IPK packaging/release.
- Added additive/idempotent deployment-managed movie roots with safe read-only mounts and regression tests.

### Playback Engine v5 and Android playback evolution

- Established Direct Play → Remux → audio AAC compatibility → video transcode as the authoritative PlaybackPlan sequence.
- Added Auto/Original/4K/1440p/1080p/720p/480p quality planning, on-demand fMP4 HLS video transcode, NVENC/QSV/VAAPI preference, HDR→SDR planning and Admin live transcode diagnostics.
- Web Player v5 and native Android/TV quality controls preserve progress across real source/quality changes.
- Android compatibility retries malformed/full capability plans conservatively without bypassing PlaybackPlan.

## 2026-08-29

### Platform automation, profiles and compatibility

- Added smart Home rules for genre/media/year/rating/recency/resolution/HDR/language/dub/sub/metadata state.
- Added serialized `media_technical` ffprobe indexing, catalog health/automation, scan simulation, duplicate-version handling, change history and backup/restore safety.
- Added per-profile Home menu visibility/order and large-catalog Web rendering/caching improvements.
- Expanded Jellyfin Android/TV compatibility while keeping the facade isolated from native StormFlix APIs.
- Added scanner-owned episodic identity, TVDB/AniList/HAMA/Fanart enrichment and series-level manual matching.

## 2026-08-28

### Streaming-style catalog and dynamic compatibility HLS

- Added automatic exclusive pt-BR genre sections and two-level Home menus/gallery sections.
- Added dynamic fMP4 HLS for Web remux/audio compatibility with bounded session cache/prefetch and immediate cleanup on playback close.
- Direct Play remains zero-HLS-cache and source-video stream-copy remains mandatory on the compatibility path.

## 2026-08-27

### Web Player v4, queues and catalog identity

- Added Web Player v4 controls, managed compatibility cache, exact audio selection and ordered profile progress.
- Added persistent serialized scan jobs, queue cancellation/recovery, category hierarchy and SQLite read optimizations.
- Added `media_series_identity`, `animation_series` / `anime_series`, metadata fallback/enrichment and principal-series matching.
- Expanded Jellyfin discovery/auth/catalog/playback/session compatibility while preserving StormFlix-native API authority.
