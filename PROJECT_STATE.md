# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file, `AGENTS.md` and `ENTERTAINMENT_ROADMAP.md` before changing code. Update this document after meaningful architecture, compatibility, schema, playback or deployment changes.

Last architecture update: **2026-08-31**.

## Deployment

Primary deployment is an Unraid checkout:

```bash
cd /mnt/user/appdata/stormflix
git pull --ff-only origin main
docker compose down
docker compose up -d --build
curl -s http://127.0.0.1:8090/healthz
echo
```

Server HTTP port: **8090**, normally behind an HTTPS reverse proxy.

## Current clients

- Server code line: **`0.22.0-web-playback-v53`**.
- Web Player: v5 line, based on the v5.3 continuous Web playback-session architecture plus v5.4 presentation/TV controls.
- Android package: `cloud.stormflix.app`, **0.6.3 / versionCode 21**, minSdk 23, targetSdk 36, Java 17.
- Android phone/tablet, Android TV and Fire TV keep native StormFlix catalog/navigation but delegate video playback to the hosted StormFlix Web Player inside `PlayerActivity` WebView.
- Samsung Tizen: `apps/tizen` 0.1.0 thin shell; final WGT requires the developer's Samsung/Tizen signing profile.
- LG webOS: `apps/webos` 0.1.0 thin shell; CI can package the Developer Mode IPK.
- Jellyfin compatibility facade remains isolated from native `/api/v1` state.
- SQLite with WAL remains the production database. PostgreSQL remains a separate future migration.

## Non-negotiable invariants

1. Native `/api/v1` is the StormFlix source of truth.
2. **Direct Play is always evaluated first.** Video transcode is never a silent bypass.
3. If only audio is incompatible, source video remains stream-copy and only selected audio may become AAC-LC.
4. Video transcode is chosen only by PlaybackPlan when device/codec/quality actually requires it.
5. Scanner-owned series identity (library root → show → season → episode) precedes metadata-provider guesses.
6. Manual episodic matches are stored at principal-series level whenever possible.
7. Profile/kids/library authorization must survive browse, caches, collections, integrations, downloads and future games.
8. Progress is profile/session/sequence aware; source/version/quality/audio changes preserve intended position.
9. External providers enrich the system but must not be required for login, Home, local progress or playback.
10. Temporary/offline FUSE/rclone sources must not cause destructive catalog disappearance.

## Playback architecture

```text
1. Direct Play
   ↓ only if required
2. Direct Stream / Remux
   ↓ only if selected audio alone is incompatible
3. Audio compatibility: video copy + selected audio → AAC-LC
   ↓ only if video/device/quality requires it
4. Video transcode
   ↓ if no safe route exists
5. Unsupported
```

Plan modes remain `direct_play`, `remux`, `audio_compatibility`, `video_transcode` and `unsupported`.

### UHD / transcode cost policy

- Compatible UHD stays original-resolution Direct Play.
- Audio-only incompatibility never becomes a 4K video encode.
- If an UHD source is incompatible and video encoding is unavoidable under Auto/Original, automatic compatibility transcode is capped at **1080p / 8 Mbps** instead of 4K→4K.
- Explicit 2160p is an intentional user request and is not silently replaced by the automatic guard.
- Dedicated UHD smart shelves use device resolution/codec hints; normal catalog/search keeps UHD titles visible because PlaybackPlan may provide a safe lower-resolution route.
- Hardware encoders exposed to the container/FFmpeg (NVENC/QSV/VAAPI) are preferred; CPU remains the reliability fallback.
- CPU H.264 live fallback uses the lower-cost `superfast` preset and UHD→1080 scaling uses a low-latency scaler.

### Web/TV player

The browser is the reference video implementation. Compatible files use HTTP Range Direct Play. Compatibility playback keeps a stable long-running Web session instead of exposing technical retry loops to users. Quality/audio/source changes preserve progress.

Android/Fire/Android TV and the Tizen/webOS shells converge on the hosted Web Player. `tv-remote.js` normalizes remote/media keys while hardware volume stays OS-owned.

Real-device behavior is authoritative for startup/stall/remote QA; CI validates code/build logic, not remote-mount latency.

## Libraries, sources and scanner safety

A logical library can own multiple physical `library_sources`. `ScanMulti` merges enabled sources into one catalog and preserves previous rows for temporarily offline sources.

Different logical libraries may intentionally use parent/child roots. Ownership follows the **most-specific configured source root**: the parent scanner prunes a subtree owned by another library. Exact duplicate roots across libraries and redundant parent/child roots inside one logical library remain blocked.

Deployment-managed movie roots use:

```text
STORMFLIX_MANAGED_MOVIE_LIBRARY_NAME
STORMFLIX_MANAGED_MOVIE_PATHS
```

Reconciliation is additive/idempotent and does not remove administrator roots. Admin folder browsing is sandboxed to the explicitly authorized media roots and never exposes the container filesystem root.

## Catalog identity, metadata and collections

`media_series_identity` owns episodic source root/series/season/episode identity. TMDB/TheTVDB/AniList/HAMA/Fanart enrich that identity.

`media_technical` caches ffprobe technical information by media/source modification state. Background probing is intentionally serialized to protect remotes.

Movie collections use TMDB `belongs_to_collection` rather than title heuristics. Phase 14 stores collection identity and the movie TMDB ID used to derive it. Existing matched movies are backfilled by a low-rate worker. Web **Coleções** appears only when at least two accessible local logical films share a collection; collection grouping never bypasses profile/library/kids filters.

The collection worker is delayed after the first Home and processes one item at a time so TMDB enrichment does not compete with initial navigation.

## Home performance and profiles

Home latency is a product SLO, not a cosmetic optimization.

Current architecture:
- selected profile is resolved **before** the expensive Home request;
- a single-profile account no longer loads Home once before auto-selection and again afterward;
- a multi-profile account no longer loads a hidden Home behind “Quem está assistindo?”;
- static/grouped Home uses a **2-minute fresh + 10-minute stale-while-revalidate** cache;
- stale valid Home can return immediately while a single background refresh rebuilds the grouped catalog;
- Phase 15 adds covering indexes for selected artwork, available/recent media, collections and collection backfill.

Dynamic/profile-sensitive rails such as Continue Watching remain outside the static grouped snapshot and are read independently.

Target documented in `ENTERTAINMENT_ROADMAP.md`: cached Home server response p95 below 500 ms on a large catalog. Future work should add explicit timing/cache-hit diagnostics and mutation-driven cache invalidation.

### Profile avatars

Uploaded local profile avatars are first-class referenced assets. Admin safe cleanup now includes local `profiles.avatar_url` references, so an active avatar cannot be classified as an orphan. External avatar URLs are not treated as local files. If an avatar was already deleted by an older build, Web falls back to the configured profile color/initial instead of rendering a broken image; the original custom photo must be uploaded again if desired.

## Trakt per profile

Phase 15 adds profile-scoped Trakt OAuth state. The model deliberately separates **application credentials** from **user authorization**:

```text
Admin configures one Trakt application
             ↓
Profile A ─ Device OAuth ─ Trakt account A
Profile B ─ Device OAuth ─ Trakt account B
Profile C ─ Device OAuth ─ Trakt account C
```

Administrator configuration:
- Client ID, Client Secret and redirect URI can be configured in Admin → Configurações;
- environment variables remain supported as defaults: `STORMFLIX_TRAKT_CLIENT_ID`, `STORMFLIX_TRAKT_CLIENT_SECRET`, `STORMFLIX_TRAKT_REDIRECT_URI`;
- persisted Client ID/Secret are encrypted with the existing AES-GCM settings key;
- updating Admin credentials takes effect without restarting the server.

Profile authorization:
- existing profiles expose Connect/Disconnect Trakt in the profile editor;
- Device OAuth displays Trakt verification URL + user code and polls at the provider-supplied interval;
- access/refresh tokens are encrypted independently per profile;
- token refresh replaces both returned tokens atomically, which is required for single-use refresh-token rotation;
- profile ownership is checked on every Trakt endpoint.

Playback/scrobble:
- local StormFlix progress is committed first;
- Trakt scrobble runs asynchronously with timeout and throttling;
- Trakt outage or rate limiting cannot block playback/progress;
- movies use TMDB movie identity; episodic scrobble uses TMDB show identity plus season/episode numbers;
- full bidirectional history/watchlist import is future explicit queued work, not part of the playback critical path.

## Assets and cleanup

Artwork optimization remains lossless by default. Byte-identical artwork can be consolidated using SHA-256 + hard links when supported, preserving every public/database path and original image bytes.

Admin cleanup reports logical bytes, unique physical inode bytes and deduplication savings. **Otimizar assets sem perda** is independent from destructive orphan/temp cleanup.

Admin → Limpeza has one page-loader owner. Do not reintroduce another global `show()` wrapper or competing renderer.

Profile avatar assets are now included in the cleanup reference set.

## Scan queues, safety and backups

`scan_jobs` is persistent FIFO and scans are serialized. Preview/dry-run follows the same source ownership rules without mutating catalog availability. Catalog-changing scan/path/category operations use SQLite safety backups; restore is staged and verified before activation.

## Games and entertainment roadmap

`ENTERTAINMENT_ROADMAP.md` is the executable product roadmap. Major planned work includes:
- Skip Intro / Skip Credits with profile and per-show behavior;
- rewind-on-resume and still-watching protection;
- configurable autoplay countdown;
- Smart Downloads on mobile;
- smart playlists;
- Watch Party / synchronized playback;
- improved editions/versions/extras;
- OIDC/optional stronger authentication;
- reuse of expensive media analysis;
- native **Jogos** module with scanner/platform/hash identity, browser/WASM emulation, per-profile saves, gamepad/TV/mobile support, metadata adapters and Continue Jogando.

RetroAssembly (MIT) is an architectural reference for browser retro emulation. RomM is a product/reference source for metadata breadth and game-management concepts but is AGPL-3.0; do not copy RomM source into StormFlix without an intentional compatible licensing decision.

## Jellyfin compatibility

Official Jellyfin clients remain supported through an isolated compatibility facade. Native StormFlix APIs/catalog rules remain authoritative. The dedicated StormFlix clients are the path for StormFlix-branded UI on Android/Fire/Tizen/webOS.

## Release/build channels

- `.github/workflows/ci.yml`: Go format/tests, Web/Admin JavaScript syntax, playback/streaming race tests, server build.
- `.github/workflows/android.yml`: Android APK build and versioned release on `main` when Android files change.
- `.github/workflows/smart-tv.yml`: Tizen validation/source artifact plus webOS validation/IPK packaging/release.

No signing passwords, API keys, OAuth tokens, private certificates or private media paths belong in repository documentation.

## Required validation before merge/release

For relevant changes validate the exact PR head with:
- JavaScript syntax;
- `go test ./...`;
- `go test -race ./internal/playback ./internal/webcompat ./internal/transcode ./internal/streaming`;
- `go build -trimpath ./cmd/stormflix`;
- platform-specific Android/Tizen/webOS workflow when those clients change;
- post-merge `main` workflow green before presenting the package as production-ready.

Real-device QA remains mandatory for browser playback, Fire/Android TV remotes, Tizen/webOS key behavior and remote/rclone throughput.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Roadmap changes belong in `ENTERTAINMENT_ROADMAP.md`. Never store secrets, credentials or private media paths in these documents.
