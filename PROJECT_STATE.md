# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file, `AGENTS.md` and `ENTERTAINMENT_ROADMAP.md` before changing code. Update this document after meaningful architecture, compatibility, schema, playback or deployment changes.

Last architecture update: **2026-09-03**.

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

- Server code line: **`0.25.0-playback-anywhere`**.
- Web Player: **Playback Engine v6**, retaining the v5.3 continuous Web session and v5.4 presentation controls, with adaptive local HEVC decode before server video transcode.
- Games Web Player: G2 browser/WASM runtime plus G2.5 dedicated Admin/metadata and RomMix-inspired browsing; G3 adds virtual mobile controls, TV/gamepad focus/menu behavior and profile-owned save-state previews. Games metadata uses Metadata Stack v2.
- Android package: `cloud.stormflix.app`, **0.6.5 / versionCode 23**, minSdk 23, targetSdk 36, Java 17.
- Android phone/tablet, Android TV and Fire TV keep native StormFlix catalog/navigation and their native/device playback capability paths. Browser WASM local video decode is not forced onto native Android/TV/Cast/DLNA routes.
- Samsung Tizen: `apps/tizen` 0.1.0 thin shell; final WGT requires the developer's Samsung/Tizen signing profile.
- LG webOS: `apps/webos` 0.1.0 thin shell; CI can package the Developer Mode IPK.
- Playback Anywhere v4 includes native DLNA/UPnP discovery/control across Web/server and Android, plus Jellyfin Play To compatibility.
- Jellyfin compatibility facade remains isolated from native `/api/v1` state.
- SQLite with WAL remains the production database. PostgreSQL remains a separate future migration.

## Non-negotiable invariants

1. Native `/api/v1` is the StormFlix source of truth.
2. **Direct Play is always evaluated first.** Video transcode is never a silent bypass.
3. Client-side local decode is an explicit compatibility route, never a reason to skip a valid native Direct Play route.
4. If only audio is incompatible, source video remains stream-copy and only selected audio may become AAC-LC.
5. Video transcode is chosen only by PlaybackPlan when device/codec/quality actually requires it.
6. Scanner-owned series identity (library root → show → season → episode) precedes metadata-provider guesses.
7. Manual episodic matches are stored at principal-series level whenever possible.
8. Profile/kids/library authorization must survive browse, caches, collections, integrations, downloads, external playback and games.
9. Progress is profile/session/sequence aware; source/version/quality/audio changes preserve intended position.
10. External providers/runtimes enrich compatibility but must not be required for login, Home, local progress or normal native Direct Play.
11. Temporary/offline FUSE/rclone sources must not cause destructive catalog disappearance.

## Playback architecture

```text
1. Direct Play
   ↓ only if native video compatibility is missing
2. Local decode (Web HEVC; video remains server stream-copy)
   ↓ when local decode is unavailable/ineligible
3. Direct Stream / Remux
   ↓ only if selected audio alone is incompatible
4. Audio compatibility: video copy + selected audio → AAC-LC
   ↓ only if video/device/quality/HDR/bitrate requires it
5. Video transcode (GPU preferred, CPU fallback)
   ↓ if no safe route exists
6. Unsupported
```

Plan modes are `direct_play`, `local_decode`, `remux`, `audio_compatibility`, `video_transcode` and `unsupported`.

### Playback Engine v6 local decode

The first v6 local-decode engine targets **HEVC/H.265 in Web browsers**. It uses the MIT-licensed hevc.js runtime (`@hevcjs/hlsjs-plugin` 0.1.2 and `@hevcjs/core` 1.4.2). PlaybackPlan selects it only when native HEVC is unavailable and the client explicitly advertises WebAssembly + Worker + WebCodecs + secure-context support.

Server behavior on a local-decode plan:
- the existing continuous fMP4/HLS session is reused;
- source HEVC video remains **stream-copy** (`-c:v copy`) and is not encoded on the StormFlix server;
- selected audio may become AAC independently when the muxed browser path requires it;
- authorization/session/profile rules remain the same as other native `/api/v1` playback routes.

Client behavior:
- HEVC is decoded with WebAssembly and converted to a browser-consumable stream through WebCodecs on the client;
- only desktop Web browsers advertise this local engine; Android/WebView, mobile Web and Tizen/webOS/TV clients retain their native/server fallback routes;
- compute-aware statistics are exposed in Player diagnostics;
- local routing is internal and automatic, with no user-facing enable/disable control;
- local budget is capability-based: low-power desktops cap at 720p, stronger desktops at 1080p and high-core/high-memory desktops advertise up to 3840×2160 automatically;
- local runtime failure is remembered for the active runtime session and the player requests a fresh PlaybackPlan, naturally falling back to server transcode rather than looping.

Intentional limits in the first v6 delivery:
- **HDR local decode is disabled** until the local color pipeline is proven; incompatible HDR remains on the server tone-mapping path;
- AV1 can still Direct Play natively when supported, but no AV1 WASM engine is advertised yet;
- explicit lower-quality requests and explicit bitrate/device limits remain server adaptation requests;
- local decode requires HTTPS/secure context (localhost is accepted); plain LAN HTTP normally falls back to the server route;
- hevc.js assets are pinned to exact versions but are loaded from public package CDNs on first local-decode use. If unavailable, normal server fallback remains usable. Future hardening may cache/self-host these pinned assets.

See `docs/PLAYBACK_ENGINE_V6.md` and `THIRD_PARTY_NOTICES.md`.

### UHD / transcode cost policy

- Compatible UHD stays original-resolution Direct Play.
- Eligible non-HDR HEVC on a sufficiently strong secure Web client may use local decode without server video encoding.
- Audio-only incompatibility never becomes a 4K server video encode.
- If an UHD source is incompatible and server video encoding is unavoidable under Auto/Original, automatic compatibility transcode is capped at **1080p / 8 Mbps** instead of 4K→4K.
- Explicit 2160p is an intentional user request and is not silently replaced by the automatic guard.
- Dedicated UHD smart shelves use device resolution/codec hints; normal catalog/search keeps UHD titles visible because PlaybackPlan may provide a safe compatibility route.
- Hardware encoders exposed to the container/FFmpeg (NVENC/QSV/VAAPI) are preferred; CPU remains the reliability fallback.
- CPU H.264 live fallback uses the lower-cost `superfast` preset and UHD→1080 uses a low-latency scaler.

### Web/TV player

The browser is the reference video implementation. Compatible files use HTTP Range Direct Play. Compatibility playback keeps a stable long-running Web session instead of exposing technical retry loops to users. Quality/audio/source changes preserve progress.

Player diagnostics distinguish native Direct Play, WASM local decode, Direct Stream/AAC and server video transcode. Local decode is a hidden automatic PlaybackPlan decision rather than a user preference: the quality panel has no decode control, and Admin → Reprodução is read-only for desktop-client eligibility, automatic resolution budget and WASM/WebCodecs/secure-context requirements.

Android/Fire/Android TV and the Tizen/webOS shells retain their device-native playback capability paths. `tv-remote.js` normalizes remote/media keys while hardware volume stays OS-owned.

Playback Anywhere v4 keeps the server-owned PlaybackPlan + short-lived HMAC playback-grant model. Google Cast uses Web Sender in compatible secure desktop browsers and the native Cast framework in Android. StormFlix also implements native DLNA/UPnP SSDP discovery plus AVTransport control for Web/server and Android, and exposes compatible renderers through Jellyfin Play To. External players receive only temporary authorized playback URLs; StormFlix cookies/passwords are not handed to receivers.

Real-device behavior is authoritative for startup/stall/remote/Cast/DLNA QA; CI validates code/build logic, not receiver-network behavior or remote-mount latency.

### Intro and credits markers

Automatic episodic marker analysis is local-first and budgeted. Intro detection compares recurring audio fingerprints near the beginning of episodes. Credits detection analyzes recurring segments in the latter half/tail and may store **multiple independent credit intervals**.

Important invariants:
- automatic detection finds the interval; the selected profile's `manual` / `automatic` / `disabled` preference decides player behavior;
- manual and chapter-derived markers take precedence over automatic analysis;
- intro analysis state is independent from credits analysis state;
- credits are not represented as one unconditional “start → file end” interval when several recurring blocks are detected;
- a unique scene between two recurring credit blocks remains outside both automatic intervals, preserving post-credit scenes;
- heavy technical/marker work yields to active playback;
- intro/credits jobs and progress are visible in Admin → **Fila & atividades**.

CI validates algorithms/migrations/build safety. Real-world marker accuracy still requires representative media QA across codecs, dubs and rclone/FUSE latency.

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

`scan_jobs` is persistent FIFO and media scans are serialized. Game libraries use their own persistent `game_scan_jobs` queue so ROM files never enter the movie/series scanner. Both queues remain visible through the unified Admin → **Fila & atividades** view.

Game scans hash supported cartridges with SHA-256, pause while video playback or a browser game session is active and preserve the previous game catalog for unavailable sources. Preview/dry-run for media follows the source ownership rules without mutating catalog availability. Catalog-changing scan/path/category operations use SQLite safety backups; restore is staged and verified before activation.

## Games — current G1/G2/G2.5/G3 architecture

Games are a first-class StormFlix media domain, not an iframe and not rows in the video `media` table.

### G1 catalog

Current cartridge matrix:
- NES: `.nes`
- SNES: `.sfc`, `.smc`
- Mega Drive / Genesis: `.md`, `.gen`, `.smd`
- Game Boy: `.gb`
- Game Boy Color: `.gbc`
- Game Boy Advance: `.gba`

The scanner identifies a game by platform + SHA-256, keeps physical ROM paths private, supports optional local sidecar cover files and exposes favorites/playtime/last-played per selected profile. A `.zip` containing exactly one supported cartridge ROM is accepted as a container; its platform and SHA-256 identity come from the uncompressed inner ROM. ZIPs with zero or multiple supported ROMs are ignored instead of guessed, and disc images remain outside this cartridge matrix.

### G2 browser player

The Web client launches supported games using **Nostalgist 0.21.1** with explicitly mapped RetroArch cores from Emscripten build **v1.22.2**:

```text
NES       → fceumm
SNES      → snes9x
Mega Drive→ genesis_plus_gx
GB/GBC/GBA→ mgba
```

Runtime policy:
- browser never loads arbitrary emulator URLs;
- server accepts only the fixed Nostalgist version and allowlisted core names/extensions;
- RetroArch build v1.22.2 publishes each allowed core as `<core>_libretro.zip`; StormFlix downloads that pinned bundle on first use, validates/bounds it, extracts the JS/WASM pair and caches both under `DataDir/game-runtime/retroarch-v1.22.2/`;
- core JS/WASM and the Nostalgist runtime are then served same-origin through authenticated StormFlix endpoints with immutable caching;
- once the pinned runtime/core files exist in the server cache, another client does not need to reach the runtime CDN for that core;
- ROM and save payloads are never uploaded to the runtime CDN.

ROM access:
- `/api/v1/games/{id}/rom` is authenticated and library-permission-aware;
- actual filesystem paths are never returned to Web clients;
- single-ROM ZIP cartridges are resolved lazily into `DataDir/game-rom-cache/<platform>/<sha256>.<ext>` for playback so the browser/core receives the native cartridge extension while the original archive remains untouched;
- the initial cartridge limit remains 512 MiB.

Profile saves:
- save state, battery SRAM and G3 save preview are stored under `DataDir/game-saves/profile-<id>/<platform>/<sha256>/`;
- SQLite stores only bounded metadata/version rows in `game_saves`;
- save files use atomic temp-file replacement and retain three recovery generations;
- state limit is 32 MiB, SRAM limit is 8 MiB and preview limit is 2 MiB;
- two profiles playing the same ROM use independent directories and state/previews.

Playtime:
- opaque browser sessions report monotonic elapsed seconds;
- the server credits only positive deltas and bounds a single jump to 120 seconds;
- hidden browser time does not advance the client counter;
- `game_profile_state.play_seconds` and `last_played_at` feed **Continuar jogando**.

Player UX:
- selecting **Play** immediately opens the game overlay, starts ROM/runtime/save preparation and attempts emulator start without a mandatory `Preparar jogo` or `Iniciar agora` step;
- if a profile save-state exists, direct launch resumes it automatically; otherwise the game boots normally while preserving battery SRAM;
- only if a browser rejects automatic start after asynchronous WASM/runtime loading does the player show a final **Iniciar jogo** interaction fallback;
- ROM/runtime/core fetch failures are surfaced with their StormFlix HTTP error instead of collapsing into Nostalgist's generic `Failed to load response` message;
- fullscreen, pause/resume, gamepad status, keyboard focus, autosave, manual save and Save-and-exit are built in;
- Save-and-exit waits for an in-flight autosave and performs a final save before terminating the emulator.

### G2.5 Games administration, library UI and metadata

Admin has a dedicated **Games** group/page with tabs for Overview, Libraries & ROMs, Queue & scans, Metadata, Saves, Emulators and Settings. Games metadata jobs are also mirrored into the global **Fila & atividades** view so long work stays observable.

The public Games shell adapts the visual/interaction language of **RomMix** (MIT): full-screen controller-oriented navigation, Continue Jogando hero, horizontal rows, Library, platform Collections, profile Saves, Emulators and Settings. StormFlix still owns the catalog, authorization, WASM player, ROM delivery and saves. Attribution and the complete RomMix MIT notice live in `THIRD_PARTY_NOTICES.md`.

Phase 22 adds:
- `game_metadata` for provider IDs, rich metadata/artwork references, errors and `metadata_locked`;
- `game_provider_settings` for enabled/provider configuration;
- `game_metadata_jobs` for persistent enrichment work.

The canonical game identity remains **platform + SHA-256**. External metadata can rename/enrich a card but cannot replace that local identity. Administrator metadata lock removes a game from automatic refresh candidates. Metadata Stack v2 additionally calculates bounded MD5/SHA-1/CRC32 only for provider lookup/correlation. For a supported single-ROM ZIP, these lookup hashes come from the native inner ROM bytes, not the ZIP container.

Provider security:
- Games provider secrets are AES-GCM encrypted before SQLite persistence;
- the encryption key is a server-local `DataDir/game-providers.key` file with mode 0600;
- API responses expose only public fields plus boolean “secret configured” state;
- provider secrets are never committed to Git and should be entered/rotated in Admin after deployment.

Implemented automatic metadata path (Metadata Stack v2):
1. **Hasheous** — optional MD5/SHA-1/CRC32 identity bridge that can correlate IGDB, RetroAchievements and TheGamesDB IDs without replacing local identity;
2. **ScreenScraper** — hash-aware retro title/summary/date/genre/company/media lookup when its developer and account credentials are configured;
3. **IGDB** — conservative primary fallback for title/summary/date/genres/companies/rating/cover/screenshots;
4. **MobyGames** — primary fallback when the prior sources have no acceptable match;
5. **TheGamesDB** — complementary primary fallback and boxart source;
6. **RetroAchievements** — enrichment only from a verified RA game ID (for example Hasheous or an explicit `(ra-ID)` tag); ordinary file MD5/SHA-1 is never assumed to be an RA hash;
7. **SteamGridDB** — preferred polished portrait artwork enrichment;
8. **Libretro Thumbnails** — public/no-key artwork fallback aligned with the RetroArch ecosystem.

A metadata job requires at least one configured identification source among ScreenScraper, IGDB, MobyGames, TheGamesDB or Hasheous. SteamGridDB, RetroAchievements and Libretro are enrichment-only and cannot by themselves identify the whole catalog. Work is persistent, resumes queued/running jobs after server restart, rate-limits provider calls and pauses while video playback or gameplay is active. Cross-provider IDs are stored independently in the existing `game_metadata` columns so metadata, community identity and artwork can come from different providers for the same local ROM.

The provider registry still models PlayMatch, LaunchBox, Flashpoint, HLTB, Demozoo, Pouët and CSDb, but these remain **planned** until provider-specific runtime adapters are wired. Admin deliberately labels non-wired providers as future integrations rather than presenting them as functional.

### G3 living-room/mobile controls and save previews

G3 is a presentation/control layer over the same authenticated G2 player rather than a second emulator implementation.

Mobile/touch:
- optional virtual controller with D-pad, A/B, SELECT and START;
- `Auto` shows controls on coarse-pointer/mobile screens, with explicit On/Off overrides;
- pointer-based multi-touch allows direction + action combinations and optional short vibration feedback;
- controller placement adapts to portrait/landscape and safe areas and stays hidden while the ROM/runtime preparation screen is visible.

Living room/gamepad:
- the Games shell supports spatial focus movement from standard gamepad D-pad/axes;
- A activates focused UI; B backs out while outside active gameplay;
- during gameplay Back/Escape opens a quick menu instead of immediately terminating the game;
- holding SELECT + START opens the same quick menu on a standard gamepad;
- the quick menu offers resume, save, fullscreen, virtual-controller mode and Save-and-exit.

Save previews:
- Phase 23 extends the versioned `game_saves` family with `preview` and is guarded so reopening an already-upgraded database is a no-op;
- the browser captures a bounded WebP representation of the emulator canvas and stores it beside the same profile/game save identity;
- previews use the same authenticated/library-permission-aware save endpoint, atomic replacement and recovery generations as state/SRAM;
- the profile Saves gallery overlays the preview when present without exposing another profile's data.

G3 CI validates migration idempotence, preview version/recovery, profile isolation, JavaScript syntax and server build. Actual multi-touch event behavior, controller mapping, audio activation and TV/browser key delivery still require real-device QA.

Community-driven next work after G3 includes smarter no-rehash scans for unchanged ROMs, explicit BIOS/ROMset diagnostics (especially arcade/Neo Geo), multidisc/DLC structure, specialized long-tail metadata providers, richer achievement presentation and the G4 Games ecosystem.

RetroAssembly (MIT) remains an architectural reference for browser retro emulation. RomM is a product/reference source for metadata breadth and game-management concepts but is AGPL-3.0; do not copy RomM source into StormFlix without an intentional compatible licensing decision.

## Entertainment roadmap

`ENTERTAINMENT_ROADMAP.md` is the executable product roadmap. Games G1/G2/G2.5/G3, Metadata Stack v2, Playback Anywhere v4, Playback Engine v6 local HEVC decode and automatic intro/credit foundations are implemented. Remaining major roadmap work includes Smart Downloads, smart playlists, Watch Party, improved editions/versions/extras, OIDC/optional stronger authentication, reuse of expensive media analysis, specialized long-tail Games providers, no-rehash scanning, BIOS/ROMset diagnostics and the G4 rich Games ecosystem.

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

Real-device QA remains mandatory for browser playback, gamepad/emulator behavior, Fire/Android TV remotes, Tizen/webOS key behavior, Cast/DLNA routing and remote/rclone throughput.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Roadmap changes belong in `ENTERTAINMENT_ROADMAP.md`. Never store secrets, credentials or private media paths in these documents.
