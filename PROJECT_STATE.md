# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file and `AGENTS.md` before changing code. Update this document after meaningful architecture, compatibility, schema, playback or deployment changes.

Last architecture update: **2026-08-31**.

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

## Current versions and clients

- Server: **`0.21.0-playback-v5`**.
- Web Player: current v5 line, with the v5.3 continuous Web playback session architecture plus later UI/TV-control improvements.
- Android package: `cloud.stormflix.app`, **0.6.2 / versionCode 20**, minSdk 23, targetSdk 36, Java 17.
- Android phone/tablet, Android TV and Fire TV keep the native StormFlix catalog UI but video playback is delegated to the hosted StormFlix Web Player inside `PlayerActivity` WebView. Media3 remains only where still required by the native music module.
- Samsung Tizen client: `apps/tizen`, **0.1.0**, a small TV shell that hands control to the hosted StormFlix Web UI/Web Player. Final `.wgt` installation requires the developer's Samsung/Tizen signing profile.
- LG webOS client: `apps/webos`, **0.1.0**, a small TV shell that hands control to the hosted StormFlix Web UI/Web Player and can be packaged as `.ipk` for Developer Mode installation.
- Jellyfin compatibility facade advertises numeric compatibility version `10.11.6`; it remains an isolated compatibility surface and does not redefine native StormFlix state.
- SQLite remains the production database with WAL, bounded connections and safety backup/restore. PostgreSQL remains a separate future migration phase.

## Non-negotiable invariants

1. Native `/api/v1` is the StormFlix source of truth.
2. **Direct Play is always evaluated first.** Video transcoding is never a silent bypass.
3. Exact selected audio stream is authoritative. If only audio is incompatible, video stays stream-copy and only audio may become AAC-LC.
4. Video transcode is selected only by the authoritative PlaybackPlan when the client/device/quality actually requires it.
5. Scanner-owned series identity (library root → show → season → episode) is authoritative before metadata providers.
6. A manual episodic match is stored at principal-series level whenever possible.
7. Profile/kids/library access controls must survive every browse, smart-section and playback path.
8. Progress is session/sequence aware; source/version/quality/audio changes preserve the intended position.
9. Jellyfin compatibility stays isolated from the native API and catalog rules.
10. Temporary/offline FUSE/rclone sources must not make scans destructively erase previously valid catalog state.

## Playback architecture

Authoritative policy:

```text
1. Direct Play
   ↓ only if required
2. Direct Stream / Remux (video copy, compatible selected audio copy)
   ↓ only if audio alone is incompatible
3. Audio compatibility (video copy, selected audio → AAC-LC)
   ↓ only if video/device/quality requires it
4. Video transcode
   ↓ if no safe route exists
5. Unsupported
```

Native plan modes remain `direct_play`, `remux`, `audio_compatibility`, `video_transcode` and `unsupported`.

### Web playback

The browser is the reference playback implementation. The current Web path is designed around a stable playback session rather than user-visible retry/fallback loops:

- compatible media uses HTTP Range Direct Play;
- Direct Stream/audio compatibility keeps source video in stream-copy when possible;
- when FFmpeg is required for Web playback, the Web v5.3 session architecture keeps one continuous execution associated with the playback session instead of restarting FFmpeg for small HLS batches;
- resume/seek, audio changes and real quality changes preserve the current position;
- audio menus expose real source tracks rather than an artificial Original/AAC selector;
- source-aware quality menus never offer resolutions above the actual source;
- screen modes (`Ajustar`, `16:9`, `Preencher/Zoom`, `Esticar`) are presentation-only and do not restart the stream;
- hls.js/native HLS remains available where required by the browser transport.

The user's real-browser test is the final authority for startup and stall behavior; CI proves build/logic safety, not remote-mount latency.

### Android / Fire TV / Android TV

The native shell keeps the complete StormFlix catalog/navigation. Opening video launches the same hosted Web Player so phone, tablet and TV no longer maintain a separate video playback engine.

For TV remotes, physical Android/Fire key codes are intercepted before System WebView/HTML video and translated into semantic commands. The shared `tv-remote.js` then owns player behavior. This prevents D-pad Up/Down from changing HTML-video volume and provides navigation for OK/select, Back, Menu/Settings/Info, Play/Pause, rewind/fast-forward, previous/next, captions and audio. Hardware volume remains owned by the operating system.

### Samsung Tizen / LG webOS

Both TV clients follow the same principle used by mature hosted-Web TV clients: keep the platform package small and let the hosted StormFlix Web UI/Web Player remain authoritative.

```text
Tizen .wgt ──┐
webOS .ipk ──┼──> hosted StormFlix Web UI ──> Web Player ──> /api/v1 PlaybackPlan
Android TV ──┘           ↑
                     tv-remote.js
```

`tv-remote.js` contains platform key normalization for Android/Fire, Samsung Tizen and LG webOS, including Back/media-key handling and Tizen media-key registration when the platform API is available.

Tizen packaging is certificate-specific: CI validates the project and publishes a certificate-ready source artifact, while the final WGT is signed locally with the developer's Samsung certificate profile. webOS packaging does not require embedding a private developer certificate in the repository; CI can produce the test IPK.

## Libraries, multiple sources and managed deployment roots

A logical library may contain multiple physical `library_sources`. `ScanMulti` walks enabled sources and merges their files into one catalog identity. A temporarily unavailable source preserves the previous catalog under that root rather than marking everything missing.

Different logical libraries may intentionally use parent/child source roots. Ownership follows the **most-specific configured source root**: a broad parent scanner does not descend into a subtree reserved by another library, while that child library scans the subtree normally. Exact duplicate roots across libraries remain invalid, and redundant parent/child roots inside the same logical library remain invalid. When a child library is introduced after the parent already indexed that subtree, the next successful parent scan marks the old parent rows unavailable so the title is not duplicated across libraries. Preview uses the same discovery ownership rules.

Deployment may append movie roots automatically with:

```text
STORMFLIX_MANAGED_MOVIE_LIBRARY_NAME
STORMFLIX_MANAGED_MOVIE_PATHS
```

`STORMFLIX_MANAGED_MOVIE_PATHS` is a comma-separated list. Startup reconciliation is additive and idempotent:

- it creates the named movie library only when missing;
- otherwise it appends managed roots without deleting administrator-configured roots;
- already-covered roots are not duplicated;
- unsafe overlapping parent roots are rejected instead of guessed;
- existing library kind/enabled state is preserved;
- a reconciliation warning does not prevent the server from starting;
- new media appears after the normal library scan, which retains the existing offline-source safety rules.

The Admin folder picker is sandboxed to explicitly authorized media roots. It exposes the configured `MediaRoot` plus deployment-managed movie roots as switchable storage locations, allows normal navigation inside the selected root, and never opens the container filesystem root or unrelated system directories. Nested authorized roots use the most-specific boundary for Back navigation.

Host media mounts should be exposed read-only inside the StormFlix container whenever the server only needs playback/scanning access.

## Scanner identity and metadata

Supported video library kinds include `movies`, `series`, `anime`, `mixed`, `anime_series`, `animation_series` and existing special kinds. `media_series_identity` owns source root, stable series key/title, season, episode and absolute episode. External metadata enriches but does not blindly redefine folder identity.

Western series/cartoons prefer scanner identity followed by TMDB TV, TheTVDB and Fanart.tv enrichment. Anime combines scanner identity with the configured TMDB/TVDB/anime-provider recovery flow. Series-level manual overrides propagate to current and future episodes through queued refresh work.

Changing a source root preserves media IDs when replacement is an unambiguous one-for-one relocation. Metadata, artwork, subtitles and progress therefore remain attached to the same media identity.

## Scan queue and safety

`scan_jobs` is persistent FIFO and scans are serialized to protect rclone/FUSE plus SQLite. Duplicate active jobs are avoided; queued/running work can be cancelled; restart requeues unfinished jobs.

`PreviewMulti` is the dry-run path used by Admin **Simular scan** and follows the same enabled roots as the real scan without mutating catalog rows. Protected scan/path/category operations continue to use automatic SQLite safety backups.

## Home, profiles and technical catalog

`library_categories.parent_id` keeps the two-level Home model: root categories are top navigation menus and direct children are gallery rails. Persistent smart rules can filter by library, genre, media type, year/rating, recency, resolution, HDR/SDR, pt-BR audio/subtitles, dub/sub classification and metadata readiness.

Profile Home preferences control root visibility/order without bypassing kids/library permissions.

`media_technical` caches ffprobe stream information by `media_id` and source modification time. Background probing is intentionally serialized to protect remote mounts. PlaybackPlan reuses valid technical snapshots and falls back to live probing when required.

## Jellyfin compatibility

Official Jellyfin clients remain supported through the compatibility facade for users who want that ecosystem. The facade exposes the required discovery/auth/catalog/playback/session/image/series surfaces while native StormFlix APIs remain authoritative.

A Jellyfin Android phone/tablet may use its own WebView shell; Jellyfin Android TV/Fire remains Jellyfin's native UI. The dedicated StormFlix clients are the route for StormFlix-branded interfaces on Android/Fire, Samsung Tizen and LG webOS.

## Release/build channels

- `.github/workflows/ci.yml`: Go format/tests, JavaScript syntax, playback/streaming race tests and server build.
- `.github/workflows/android.yml`: Android APK build and versioned APK/SHA-256 release on `main` when Android files change.
- `.github/workflows/smart-tv.yml`: validates Tizen JavaScript/manifest, publishes a certificate-ready Tizen source artifact, validates/packages LG webOS and publishes a webOS IPK/SHA-256 release on `main`.

No signing passwords, API keys, account credentials, Samsung certificates or private media paths belong in repository documentation.

## Required validation before merge/release

For a change touching these areas, validate the exact PR head with the applicable workflows:

- JavaScript syntax;
- `go test ./...`;
- `go test -race ./internal/playback ./internal/webcompat ./internal/transcode`;
- `go build -trimpath ./cmd/stormflix`;
- Android Gradle build when Android files/version change;
- Tizen manifest/JavaScript validation when Tizen files change;
- webOS `ares-package --check` and IPK packaging when webOS files change;
- post-merge `main` workflow(s) green before presenting a package as production-ready.

Real-device QA remains mandatory for browser playback, Fire/Android TV remote behavior, Samsung Tizen key behavior, LG webOS key behavior and remote/rclone throughput.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Never store secrets, credentials or private media paths in these documents.
