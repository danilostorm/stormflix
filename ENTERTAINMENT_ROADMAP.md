# StormFlix — Entertainment Roadmap

This roadmap turns product ideas into bounded engineering work. `PROJECT_STATE.md` remains the source of truth for what is already deployed; this file describes what comes next.

## Product principles

1. **Instant first.** Home/navigation should feel local even when media lives on rclone/FUSE remotes. Network metadata providers never belong on the critical Home or playback-start path.
2. **Direct Play first.** Convenience features must not silently turn compatible media into video transcodes.
3. **Profile-owned experience.** Progress, Trakt, saves, downloads, autoplay, intro/credits behavior and future game state belong to the selected profile.
4. **Local-first reliability.** External services enrich the system; they never become required for login, Home, playback, progress or games already indexed locally.
5. **One catalog identity, many physical versions.** Expensive analysis should be reused across duplicate/alternate versions whenever safe.
6. **TV is a first-class client.** Every important interaction must be usable by remote/D-pad and remain readable at living-room distance.
7. **Measure before optimizing.** New background work gets queue/budget/latency metrics and must yield to active playback.

## P0 — Production experience and latency

### Home performance SLO

Target:
- cached Home server response p95 **< 500 ms** on a large local catalog;
- no duplicate Home request during profile bootstrap;
- profile picker never loads a hidden Home behind it;
- stale catalog snapshot may be returned immediately while a single background refresh rebuilds it;
- TMDB/Trakt/other Internet calls are forbidden from the Home critical path.

Work:
- keep grouped Home stale-while-revalidate cache;
- add/maintain covering indexes for selected artwork, available media, collections and recent media;
- expose Home timing/cache hit metrics in Admin diagnostics;
- invalidate static Home snapshots after catalog/category/library mutations rather than recomputing on every navigation.

### Profile integrity

- Local profile avatars are referenced assets and cannot be deleted by safe cleanup.
- Broken historical avatar files fall back to the configured avatar/initial without a broken-image icon.
- Profile selection happens before the expensive Home request.

### Trakt per profile

- Administrator configures one Trakt application Client ID/Secret.
- Every StormFlix profile authorizes its own Trakt account through Device OAuth.
- Access and refresh tokens are AES-GCM encrypted at rest and never returned to Web clients.
- Refresh-token rotation replaces both tokens atomically.
- Local progress is committed first; Trakt scrobble is asynchronous and bounded by timeout/throttling.
- Future import/sync jobs must be explicit, resumable, rate-limited and profile-scoped.

## P1 — Playback delight

### Skip Intro / Skip Credits

Architecture:
- `media_markers` stores typed intervals (`intro`, `credits`, later `recap`) against stable episodic/logical identity;
- marker analysis runs in a serialized/budgeted background queue and yields to live playback;
- identical physical versions may share a marker result only when duration/fingerprint compatibility is proven;
- manual corrections override automatic markers.

Experience:
- profile preference: `Manual`, `Automático`, `Desativado`;
- optional per-show override;
- Skip button appears in the OSD only while inside a valid marker;
- automatic skip remains undoable for a short period;
- TV remote focus and accessibility are mandatory.

Acceptance:
- marker analysis never triggers video transcode;
- no Internet dependency;
- no raw remote file is fully copied merely to detect a marker when streaming analysis is possible.

### Rewind on Resume

- Profile setting: Off / 5 / 10 / 15 / 30 seconds.
- Applies after a meaningful pause/idle/resume, never before 00:00.
- Does not alter stored canonical progress; it changes only the resumed playback start position.

### Still Watching / passout protection

- Configurable per profile.
- Trigger after N autoplay episodes or N uninterrupted hours.
- Pauses before launching the next item; never marks an unplayed episode watched.

### Configurable autoplay countdown

- Profile-controlled countdown including `imediato`, 5, 10, 15, 30 seconds and disabled.
- Works across Web/Android/TV shells using the same Web Player state machine.

### Editions, versions and extras

- Keep one logical work card while exposing Director's Cut, Extended, 4K/1080p, alternate audio/containers as versions.
- Extras (trailers, featurettes, deleted scenes) live under the work rather than polluting the main library.
- PlaybackPlan chooses among versions without losing progress.

## P1 — Smart Downloads

Initial target: Android phone/tablet.

- Per-show policy: keep next **N** unwatched episodes downloaded.
- Delete watched download after successful local progress sync, then fetch the next episode.
- Respect Wi-Fi-only, charging, storage quota and quality policy.
- If a valid local copy exists, playback uses it transparently without consuming server bandwidth/transcode.
- Offline progress queues locally and merges by ordered session/event timestamps on reconnect.
- Never delete source media on the server.

## P1 — Smart playlists and discovery

Use the existing smart-rule vocabulary as a base:
- library/category/genre/year/rating;
- 4K/HDR/codec;
- pt-BR audio/subtitles/dub status;
- unwatched/recently watched/progress;
- runtime;
- collection/franchise;
- future Trakt watchlist/rating only when the profile explicitly enables it.

Playlists can be manual or rule-driven, profile-owned or administrator-shared.

## P2 — Watch Party / synchronized playback

- Server-local rooms identified by opaque invite code.
- Host controls play/pause/seek and optionally transfers control.
- Joiners use their own authorized media version/PlaybackPlan; synchronization coordinates timeline, not raw stream bytes.
- Episode queues persist across autoplay so a room does not end after every episode.
- Client reports latency/buffer state; server uses bounded drift correction rather than constant seeks.
- When a client has a Smart Download/local copy, it may use that copy while remaining synchronized.
- A participant without library/profile permission can never use a room as an authorization bypass.

## P2 — Authentication and remote access

- OIDC for external identity providers without replacing local emergency admin access.
- Optional TOTP/passkeys for privileged accounts.
- Session/device management remains visible and revocable.
- Reverse-proxy awareness and secure-cookie/origin policy stay explicit.

## P2 — Reuse expensive analysis

Introduce a reusable media-analysis identity derived from stable physical fingerprint + duration/stream signature where safe. Reuse:
- ffprobe technical data;
- intro/credits markers;
- thumbnails/chapter previews;
- loudness/subtitle timing analyses.

Never share analysis across files solely because titles match.

# Games — native StormFlix entertainment module

Games should feel like another StormFlix media type, not an iframe to a second product.

## Architecture

```text
Game library roots
      ↓
Safe ROM scanner
      ↓
Platform + content hash identity
      ↓
Metadata/artwork adapters
      ↓
StormFlix Games catalog
      ↓
Browser/WASM game player
      ↓
Per-profile save + save-state synchronization
```

### Catalog

New first-class entities should remain separate from video `media` where game-specific semantics require it:
- platforms/systems;
- games;
- ROM/disc versions;
- multi-disc sets;
- optional patches/mods/manuals;
- favorite/playtime/last played per profile.

The scanner uses platform rules plus hashes where possible. Filename heuristics are fallback, never the only permanent identity.

Large-library follow-up:
- cache hash identity by path/size/mtime so a normal rescan does not reread unchanged ROM bytes;
- continue to verify changed files before identity mutation;
- preserve offline-source catalog rows exactly as G1 does today.

### Metadata and artwork

G2.5 establishes replaceable provider adapters and an encrypted Admin provider vault.

Implemented first wave:
- **IGDB** primary metadata;
- **MobyGames** primary fallback;
- **SteamGridDB** artwork enrichment.

Next provider waves:
- ScreenScraper hash/media integration;
- RetroAchievements game identity/account/achievement enrichment;
- Hasheous / PlayMatch hash correlation;
- LaunchBox/TheGamesDB/Libretro local/cached artwork and metadata options;
- specialist Flashpoint, HLTB, Demozoo, Pouët and CSDb adapters where they materially improve a platform.

Provider calls run in persistent background queues, resume after server restart, are observable in Admin and yield to active video/game playback. A game with no metadata remains playable from its local platform + SHA-256 identity. Administrator **metadata lock** protects manual fixes from automatic refresh.

### Browser/player interface

RetroAssembly is an MIT-licensed architectural reference for browser retro emulation, save states, gamepad navigation, rewind, shaders and virtual controls. Prefer a narrow StormFlix-owned integration around suitable WASM/RetroArch-compatible cores rather than embedding the entire application.

The G2.5 browsing shell adapts the controller-oriented visual language of **RomMix** under its MIT license. Attribution is retained in `THIRD_PARTY_NOTICES.md`; StormFlix keeps its own catalog, authorization, ROM endpoints, browser player and saves.

RomM is an excellent product/reference source for metadata breadth, multi-disc/DLC/mod/manual concepts, RetroAchievements and client integrations, but its code is AGPL-3.0. StormFlix reimplements those concepts behind its own interfaces rather than copying RomM source.

### Saves and profiles

- Battery saves and save states are profile-owned.
- Cloud/server sync is atomic and versioned; retain a small recovery history.
- A profile can play the same ROM independently from another profile.
- G2.5 exposes a profile-scoped Saves gallery without leaking another profile's state.
- Future handheld/desktop clients may sync saves without exposing arbitrary server filesystem paths.

### BIOS, arcade and ROMsets

Before expanding browser cores to arcade/Neo Geo, add a first-class diagnostics model:
- administrator-visible BIOS inventory/status by platform/core;
- ROMset/core compatibility profile rather than assuming every ZIP belongs to every arcade core;
- explicit missing-BIOS / wrong-ROMset / unsupported-core messages;
- never fix compatibility by silently duplicating BIOS files into every user game archive;
- keep BIOS files administrator-provided and never distribute them from StormFlix.

This work is prioritized from real community pain around Neo Geo/FBNeo sets where a generic launch failure hides an actual BIOS/ROMset mismatch.

### TV/mobile controls

- Catalog remains navigable with the existing TV semantic remote layer.
- Gameplay prefers USB/Bluetooth gamepad; remote keys are not silently mapped to destructive emulator actions.
- Mobile can expose an optional virtual controller.
- Exiting a game flushes saves before returning to the StormFlix catalog.

### Safety/legal boundary

StormFlix indexes and plays user-provided game files. It does not ship or download copyrighted ROMs/BIOS files. BIOS requirements are configured by the administrator and validated without publishing those files.

## Games delivery phases

**G1 — Catalog — implemented:** `games` library kind, scanner, platform/hash identity, covers, game details, favorites.

**G2 — Browser play — implemented:** NES/SNES/Mega Drive/Game Boy/GBC/GBA browser WASM player with gamepad + per-profile state/SRAM and playtime.

**G2.5 — Admin / library UX / metadata — implemented:** dedicated Admin Games hub; RomMix-inspired full-screen Games library; profile Saves gallery; encrypted provider vault; persistent metadata queue; IGDB + MobyGames + SteamGridDB; metadata lock.

**G3 — Living room/mobile — next:** TV focus/gamepad QA, virtual mobile controls, save-state thumbnails, controller mapping polish, explicit BIOS/ROMset diagnostics and initial arcade/Neo Geo expansion after diagnostics exist.

**G4 — Rich ecosystem:** RetroAchievements, manuals, multi-disc, DLC/base-game grouping, patches/mods, specialist providers and optional sync clients.

## Home experience target

The long-term Home can mix entertainment without mixing identity:

```text
Continuar assistindo
Continuar jogando
Em alta / lançamentos locais
Coleções
Séries e novos episódios
Jogos adicionados recentemente
Música
```

Every rail remains profile/library permission-aware and must be cheap to render from local cached state.

# Definition of done for roadmap features

A feature is not considered production-ready until:
- schema migration is forward-safe and tested;
- profile/library/kids authorization is covered;
- external calls are off critical playback/Home paths;
- background work is bounded and yields to playback;
- Web + TV interaction is coherent where applicable;
- CI/tests/build pass on the exact PR head and again on `main` after merge;
- `PROJECT_STATE.md` and `CHANGELOG.md` describe the deployed behavior.
