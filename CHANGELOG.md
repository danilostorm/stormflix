# StormFlix Changelog

This file records user-visible and architectural changes. `PROJECT_STATE.md` is the authoritative current-state handoff and `ENTERTAINMENT_ROADMAP.md` tracks planned work.

## 2026-09-01

### Playback Anywhere v3 — native Android Cast, app chooser and Web Video Cast handoff

- Android advances to **0.6.4 / versionCode 22** and adds Google Cast Application Framework 22.3.1. Chromecast/Google TV discovery in the APK now uses the native Android Cast route chooser instead of depending on the Web Sender SDK inside Android System WebView.
- Added `NativePlaybackAnywhere`, which receives only the already-authorized, short-lived Playback Anywhere URL produced by the server. Receiver applications never receive StormFlix cookies or passwords.
- Native Android media details now expose **Reproduzir em…** beside Assistir, so a user can cast/open externally without first starting local playback.
- **Abrir com outro player** uses Android `ACTION_VIEW` + `Intent.createChooser`, allowing the operating system to present compatible installed apps such as VLC, MX Player and Web Video Cast rather than opening the stream in a browser tab.
- Added a dedicated **Web Video Cast / Roku / DLNA** handoff. When Web Video Cast (`com.instantbits.cast.webvideo`) is installed, StormFlix sends it the temporary PlaybackPlan/grant URL so its own receiver discovery can target Roku, Fire TV, DLNA, webOS and other devices it supports.
- The normal Web client keeps Google Cast Web Sender as a fallback and now explains the HTTPS/network requirement when Cast discovery is unavailable.
- Playback Anywhere UI remains anchored next to its `Reproduzir em` trigger and the Web asset is cache-busted as v3.
- No proprietary Web Video Cast code is bundled or copied. The integration is Android intent interoperability only. Native DLNA discovery inside StormFlix itself remains separate future work.

### Games Metadata Stack v2 — hash identity and multi-provider enrichment

- Expanded the automatic Games metadata worker from the initial IGDB/MobyGames/SteamGridDB trio to a real **8-provider runtime stack**: ScreenScraper, IGDB, MobyGames, TheGamesDB, Hasheous, RetroAchievements, SteamGridDB and Libretro.
- Added bounded MD5, SHA-1 and CRC32 calculation for provider lookups while preserving **platform + SHA-256** as the only canonical local ROM identity. Single-ROM ZIP cartridges are hashed from the native inner ROM rather than the ZIP container.
- Added **Hasheous** hash correlation to bridge verified IGDB, RetroAchievements and TheGamesDB IDs without replacing local identity.
- Added **ScreenScraper** hash-aware primary lookup with Developer ID/Password plus account credentials stored through the existing encrypted Games provider vault.
- Primary metadata now follows ScreenScraper → IGDB → MobyGames → TheGamesDB, with conservative title matching and Hasheous as an identity/title fallback.
- **RetroAchievements** is enrichment-only and is queried only from a verified RA game ID (for example from Hasheous or an explicit `(ra-ID)` filename tag). StormFlix deliberately does not assume that a normal ROM MD5/SHA1 is a valid RA hash.
- **SteamGridDB** remains the preferred polished portrait-art source and **Libretro Thumbnails** is now a public no-key artwork fallback aligned with the RetroArch ecosystem.
- Provider IDs are persisted independently in the existing `game_metadata` columns so one metadata source and another artwork/community source can coexist for the same ROM.
- Admin → Games → Metadados now exposes only adapters that really execute at runtime; specialized PlayMatch/LaunchBox/Flashpoint/HLTB/Demozoo/Pouët/CSDb entries remain visibly ROADMAP instead of pretending to be active.
- Added regression tests for provider promotion, encrypted configuration state, cross-provider ID persistence, raw-ROM MD5/SHA1/CRC32 and explicit RetroAchievements ID parsing. CI now syntax-checks the Games Admin scripts too.

### Games ZIP cartridges, direct launch and RetroArch runtime fix

- Added bounded **single-ROM ZIP** discovery for the existing NES/SNES/Mega Drive/GB/GBC/GBA cartridge matrix. Platform and SHA-256 identity come from the uncompressed inner ROM; ambiguous multi-ROM ZIPs are ignored instead of guessed.
- ZIP cartridges are resolved lazily into a native-extension server cache for playback, so the emulator receives `.sfc/.smc/.nes/.gba/...` bytes while the original archive remains untouched.
- Fixed real browser game startup against RetroArch Emscripten build **v1.22.2**. That build publishes cores as `<core>_libretro.zip`; StormFlix had incorrectly requested loose `<core>_libretro.js/.wasm` URLs, which returned non-2xx responses and surfaced through Nostalgist only as `Failed to load response`.
- StormFlix now downloads the pinned official core ZIP on first use, validates and extracts its JS/WASM pair server-side, caches those files under the StormFlix data directory and serves them same-origin through the existing authenticated runtime endpoints.
- Clicking **Play** now means play: the game overlay begins ROM/runtime/save preparation immediately and attempts automatic emulator startup without a mandatory **Preparar jogo** or **Iniciar agora** step.
- Existing save-state resumes automatically; otherwise normal boot preserves SRAM. A final **Iniciar jogo** button appears only when the browser requires a renewed user gesture after asynchronous WASM loading.
- Runtime/ROM/core fetch errors now surface their StormFlix HTTP error instead of collapsing into Nostalgist's generic response-loading message.
- Added regression tests for official core-bundle URL selection, JS/WASM extraction and incomplete-bundle rejection.

### Games G3 — mobile controls, living-room gamepad and save previews

- Added an optional **virtual mobile controller** for the native browser game player with D-pad, A/B, SELECT and START, pointer-based multi-touch, Auto/On/Off visibility and optional short haptic feedback.
- Added responsive portrait/landscape/safe-area placement and keep virtual controls hidden while the ROM/runtime preparation screen is still visible.
- Added controller-oriented spatial focus navigation to the Games shell. Standard gamepad D-pad/axes move focus, A activates focused UI and B backs out when outside active gameplay.
- Added a gameplay **quick menu**: Back/Escape opens it instead of immediately exiting, while holding SELECT + START on a standard gamepad opens the same menu. It exposes resume, save, fullscreen, virtual-controller mode and Save-and-exit.
- Phase 23 extends `game_saves` with a profile-owned `preview` artifact while preserving the existing atomic replacement/version/recovery model. The migration detects an already-upgraded CHECK constraint and becomes a no-op on later server restarts.
- Save previews are bounded to 2 MiB, live beside the same profile/platform/SHA-256 save identity and are served only through the authenticated, library-permission-aware save endpoint.
- The browser captures a compact emulator-canvas preview during save/periodic lifecycle points and the profile Saves gallery displays it when available without exposing another profile's data.
- Added migration/reopen tests plus preview version/recovery and cross-profile isolation tests. Real touch/gamepad/TV behavior remains real-device QA even when CI is green.

## 2026-08-31

### Games G2.5 — dedicated Admin, RomMix-style UI and metadata stack

- Added a dedicated **Admin → Games** control center so ROM libraries, scans, catalog identity, metadata, saves, emulator/runtime status and Games settings no longer compete with the movie/series administration pages.
- Rebuilt the public Games experience around the controller-friendly visual/interaction language of **RomMix**: full-screen Games shell, large Continue Jogando hero, horizontal rows, Library, platform Collections, Saves, Emulators and Settings. StormFlix keeps its own authenticated catalog, WASM player and per-profile state; it does not adopt RomMix's download/native-emulator model.
- RomMix is MIT-licensed; the adaptation is attributed in `THIRD_PARTY_NOTICES.md`. RomM remains an AGPL product/architecture research reference and no RomM implementation source is copied into the native Games module.
- Phase 22 adds rich game metadata/provider IDs, metadata lock, encrypted provider settings and persistent Games metadata jobs without changing platform + SHA-256 as the canonical local ROM identity.
- Added an AES-GCM Games provider vault under the StormFlix data directory. Secrets are never returned to the Admin browser and are never stored in repository files.
- Implemented the first automatic metadata pipeline: **IGDB** as primary match, **MobyGames** as fallback and **SteamGridDB** as optional portrait artwork enrichment. ScreenScraper, RetroAchievements, Hasheous, PlayMatch, LaunchBox, TheGamesDB, Flashpoint, HLTB, Demozoo, Pouët, CSDb and Libretro are represented in the provider architecture but remain later integrations unless explicitly marked implemented in Admin.
- Games metadata jobs are persistent, resume after server restart, appear in the global **Fila & atividades**, expose progress/matched/errors and pause while video playback or browser gameplay is active.
- Added **metadata lock** so administrator-corrected game metadata can be excluded from automatic refresh.
- Added a profile-scoped Saves gallery and exact Admin ROM/save/playtime aggregates.
- Added explicit roadmap/UI guidance for arcade/Neo Geo BIOS + ROMset diagnostics instead of presenting generic launch failures when a set/core is incompatible.

### Automatic intros/credits and native Games G1/G2

- Added Plex-inspired automatic intro detection for episodic libraries using lightweight recurring audio fingerprints. Detection is background/budgeted, yields to playback and preserves manual/chapter marker precedence.
- Added automatic credits detection using multiple independent recurring tail segments. StormFlix does not assume “credits start → file end”: separated blocks remain separated so unique post-credit scenes are not swallowed by one automatic skip interval.
- Intro and credits analysis now appears live in Admin → **Fila & atividades**, including progress, detected/failed counts and playback-priority pause messages.
- Added first-class `games` libraries outside the movie/series media table. G1 scans NES, SNES, Mega Drive/Genesis, Game Boy, Game Boy Color and GBA cartridges, identifies content with SHA-256 and keeps game scans in the same observable Admin queue.
- Added native Games Web catalog with platforms, search, favorites per profile, sidecar covers and Continue Jogando/playtime state.
- Added Games G2 browser player architecture using pinned **Nostalgist 0.21.1** and RetroArch Emscripten cores from build **v1.22.2**. Runtime files are fetched by the server only from a fixed allowlist/version on first use, cached under the StormFlix data directory and then served same-origin.
- ROM access is authenticated and permission-aware; the browser never receives a server filesystem path. StormFlix does not download or distribute ROMs/BIOS.
- Added profile-owned save state + SRAM synchronization. Save payloads live outside SQLite under `game-saves/profile-<id>/<platform>/<sha256>/`, use atomic replacement and retain three recovery generations.
- Added bounded play-session heartbeats and profile playtime. Hidden/suspended browser time does not accrue locally and one heartbeat cannot credit an unbounded time jump.
- Game Player G2 pauses competing StormFlix media, prepares network/runtime work before the final user gesture, supports fullscreen, pause/resume, keyboard/gamepad status, autosave, manual save and **Salvar e sair** that waits for in-flight autosave before terminating the emulator.
- Phase 20 introduced the Games catalog/scan tables; Phase 21 adds game save metadata and durable game play-session accounting.

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
- Existing matched movies are backfilled by a single low-rate worker.
- Added permission-aware `/api/v1/media?group=collections&minimum_size=2` and automatic Web **Coleções** navigation.
- Fixed Admin → Limpeza so only one page-loader owner; stale async responses no longer replace the optimization view.

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