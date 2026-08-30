# StormFlix — Project State / Handoff

> **Authoritative continuation note.** Any coding agent/session continuing StormFlix must read this file and `AGENTS.md` before changing code. Update this document after meaningful changes.

Last architecture update: **2026-08-29**.

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

- Server: `0.20.0-platform-automation`.
- Android package: `cloud.stormflix.app`, version **0.4.0**, versionCode 12, minSdk 23, targetSdk 36, Java 17, Media3 1.11.0.
- Jellyfin compatibility facade advertises numeric compatibility version `10.11.6` to satisfy current official-client version parsing; this does **not** turn StormFlix into Jellyfin or change the native server version.
- SQLite remains the native catalog database with WAL, `synchronous=NORMAL`, busy timeout, bounded connection pool and targeted indexes.

## Non-negotiable invariants

1. Native `/api/v1` is the StormFlix source of truth.
2. **Direct Play first. Never silently transcode video.**
3. Web, Android and TV/Fire use `/api/v1/media/{id}/playback/plan`.
4. Jellyfin compatibility is an isolated facade and must not redefine native catalog/playback state.
5. Scanner-owned episodic identity is authoritative before metadata providers.
6. Episodic manual matching is principal-series level only.
7. Profile/kids/library access controls must survive every browse, smart-section and playback path.
8. Exact selected audio stream remains authoritative. If only audio is incompatible, video stays stream-copy and only audio may be converted to AAC-LC.
9. Unsupported video capability returns unsupported; no hidden transcode/tone-map fallback.
10. Ordered progress remains session/sequence aware.

## Native playback architecture

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 ──> Playback Core ──> Media
StormFlix TV/Fire ───┘        │
                              ├── Profiles / progress
                              ├── Library access
                              ├── Metadata / subtitles
                              ├── Dynamic HLS session cache (Web)
                              └── Seekable compatibility MP4 cache (native/manual fallback)

Official Jellyfin clients ──> isolated compatibility facade ──> StormFlix core
```

Native modes remain `direct_play`, `remux`, `audio_compatibility` and `unsupported`.

### Web Player v4

`playback-core.js` owns planning/execution. `player-v4.js`/`player-v4.css` own presentation: custom played/buffered timeline, seek preview, mode/resolution indicators, ±10s, audio/subtitles/settings, previous/next episode, fullscreen, native PiP, Media Session and responsive controls.

Compatible media uses HTTP Range Direct Play and creates zero compatibility cache. Web remux/audio compatibility uses session-scoped fMP4 HLS with exact selected streams and mandatory video stream-copy.

### Dynamic HLS and diagnostics

HLS fragments live under `<DataDir>/hls-cache/<playback-session>/` with a 5 GiB global budget, 30-minute crash/idle fallback TTL and 10 GiB or 5% minimum free-disk reserve. Normal close/end cancels the FFmpeg worker and removes the session directory immediately.

Web telemetry reports playback mode, source bitrate, buffer seconds, estimated read Mbps, codecs, cache bytes and errors. Adaptive prefetch changes only speculative headroom: normally one batch, at most three small batches during low-buffer/slow-read conditions. Global SSD budget/free-space reserve always wins. Video is never transcoded by this mechanism.

Admin “Reproduzindo agora” returns the enriched playback superset while preserving the original endpoint shape for older Admin consumers.

### Managed legacy compatibility MP4 cache

`internal/webcompat/materialize.go` + `CacheManager` remain for Android/TV/manual compatibility paths. Defaults: 20 GiB max, 48h TTL, 15m cleanup, LRU target 85%, free-space reserve, active-file protection and short-lived oversize artifacts.

## StormFlix Android / Android TV / Fire TV

The native app is one package with touch + remote/Leanback behavior. Media3 consumes the same PlaybackPlan contract, publishes MediaCodec capabilities, preserves native multi-audio Direct Play where possible, supports source/version selection and ordered progress, and does not silently bypass the planner.

Version **0.4.0** changes Home navigation from hard-coded `Filmes / Séries / Animes` to server-driven root categories from `/api/v1/categories` plus the selected profile's `/api/v1/profiles/home-menus` visibility/order. Custom roots such as **Desenhos** therefore appear automatically on phone, Android TV and Fire TV.

`CategoryBrowseActivity` is the generic two-level native catalog browser. It loads the root aggregate and resolves each direct child through `/categories/{slug}/smart`, so gallery sections such as **Animes Dublados**, **Animes Legendados**, **Filmes Animes** or custom sections use the same server rules as Web instead of duplicating classification logic in Android.

## Official Jellyfin Android / Android TV / Fire TV compatibility

The compatibility work is based on current upstream official clients, not guessed route names.

### Official Jellyfin Android phone/tablet

The official Android app is a WebView wrapper. Its current `JellyfinWebViewClient`:

1. loads the configured server root `/`;
2. intercepts the first `main.*.bundle.js` request and injects Jellyfin's native shell;
3. watches `Sessions/Capabilities/Full`;
4. reads `localStorage.jellyfin_credentials` and extracts `Servers[0].UserId` + `AccessToken` for native session setup.

StormFlix deliberately emits `/main.stormflix.bundle.js` only when a Jellyfin native JS interface is present. The deferred bridge keeps the **StormFlix Web layout** rendered, polls the same-origin authenticated bridge `/api/v1/compat/jellyfin-mobile-bridge`, writes the credentials shape expected by the official wrapper, and triggers `/Sessions/Capabilities/Full`. Before StormFlix login it remains harmless; after logout/401 it removes only the credentials belonging to the current StormFlix origin.

The bridge endpoint never trusts a browser-supplied user ID. It derives the current user and token from the already-authenticated StormFlix HttpOnly session and is itself protected by native StormFlix authentication.

### Official Jellyfin Android TV / Fire TV

The official Jellyfin TV application is a **native Android/Leanback application**, not a WebView. A server cannot legitimately replace that native UI with StormFlix HTML after login. Therefore:

- official Jellyfin TV/Fire keeps Jellyfin's native UI and consumes the compatibility facade;
- the StormFlix native app is the supported path when the desired TV/Fire UI is the StormFlix layout;
- the facade is expanded around the current official SDK calls (`UserViews`, `Items`, `Latest`, `Resume`, `NextUp`, `PlaybackInfo`, sessions/progress, images, seasons/episodes, direct streams and optional discovery/query surfaces).

Optional official-client features StormFlix does not implement return correctly shaped empty DTOs instead of malformed fake data. `QuickConnect/Enabled` explicitly returns false because StormFlix supports its own username/password authentication rather than advertising an unsupported Jellyfin protocol.

Every recognized Jellyfin request is traced without logging auth secrets. HTTP 404 and 5xx compatibility requests are tagged `JELLYFIN_COMPAT_GAP`, allowing real Android/TV/Fire traffic to reveal future endpoint gaps in Admin logs without weakening the native API.

## Scanner identity, metadata and sources

Supported video library kinds include `movies`, `series`, `anime`, `mixed`, `anime_series`, `animation_series` and existing special kinds. `media_series_identity` owns source root, stable series key/title, season, episode and absolute episode. External metadata enriches but does not redefine folder identity blindly.

Western series/cartoons prefer `Scanner → TMDB TV → TheTVDB v4 → Fanart.tv`. Anime with seasons combines scanner identity with TMDB TV, TheTVDB, AniList/AniDB/MAL recovery and the native HAMA-style Anime-Lists bridge.

Changing a Drive/rclone source root preserves media IDs when the replacement is unambiguous. Metadata, artwork, subtitles and progress stay attached to the same `media_id`. Ambiguous relocations/collisions are not guessed.

## Principal-series manual matching

Admin → Catálogo defaults to **Obras principais**. Episodic manual matching lives in `series_metadata_overrides`; queued `series_refresh` updates current episodes and future episodes inherit the override. Normal episode rows are diagnostic children, not independent manual matches.

## Scan queue and safe simulation

`scan_jobs` is persistent FIFO and scans are serialized to protect rclone/FUSE + SQLite. Duplicate active jobs are avoided; queued/running work can be cancelled; restart requeues unfinished jobs; unreachable mounts preserve prior catalog.

`PreviewMulti` is the dry-run path used by Admin **Simular scan**. It traverses the same enabled roots as `ScanMulti`, reports existing/discovered/new/changed/missing/unchanged and never writes media availability or inserts/deletes catalog rows. Offline roots are handled conservatively.

Protected scan, scan-all, library-path and recommended-category operations require a successful automatic database backup before execution. Repeated scan work may reuse a recent snapshot to avoid unnecessary SQLite `VACUUM` pressure, while structural library/source-path changes and recommended Home reorganization always create a fresh exact pre-operation snapshot.

## Smart Home menus and sections

`library_categories.parent_id` keeps the two-level presentation contract:

- `parent_id IS NULL` = **Menu da Home** in top navigation;
- direct child = **Seção da galeria** / rail inside that menu.

The parent/general rail is never replaced by child sections. Child sections are stacked by `sort_order`; empty sections are omitted. A title may appear once in the general rail and again in a relevant child rail.

### Persistent smart rules

Phase 13 adds `rule_mode` and `rules_json` to categories. A child section can use:

- `libraries` — selected library IDs only;
- `rules` — inherit the menu scope and filter automatically;
- `both` — selected libraries plus rules.

Supported filters: genres, media types, year range, minimum rating, recently added days, minimum/maximum resolution, HDR/SDR/HDR10/HLG, Brazilian-Portuguese audio, Brazilian-Portuguese subtitles, technical dub/sub classification and metadata-required.

Admin → **Menus da Home** supports drag/drop ordering and live preview before save. The recommended organizer creates technically driven 4K/UHD and Anime Dublado/Legendado shelves; custom items are not deleted.

### Profile-specific Home

`profile_home_menus` stores root-menu visibility and order per profile. Web and Android navigation merge these preferences with active root categories. Profile/kids/library restrictions remain authoritative after smart filtering.

## Technical catalog index

`media_technical` caches real stream inspection keyed by `media_id` + source `modified_unix`:

- video codec/resolution/HDR/bitrate/duration;
- audio languages;
- subtitle languages;
- pt-BR audio/subtitle flags;
- technical `dublado`, `legendado` or `original` classification;
- probe status/error/timestamp and serialized source probe.

The background indexer runs **one ffprobe at a time** so remote mounts are not hammered. Changed files invalidate their cache. Pending work is automatic; transient probe errors become eligible for retry after 30 minutes. Admin can explicitly requeue all technical analysis.

PlaybackPlan reuses the serialized technical `playback.Source` when its `modified_unix` still matches, avoiding a second ffprobe/rclone read for already indexed media. The technical table remains an optimization only: when a live probe succeeds, a temporary SQLite cache-write failure must not turn that otherwise valid playback into a playback failure.

Physical versions keep independent technical snapshots. Logical catalog cards remain deduplicated while `/media/{id}/versions` exposes source/quality alternatives.

## Catalog health and duplicate identity

Admin → **Saúde & Automação** exposes totals for missing metadata, cover, genre, `Outros`, unavailable media, technical backlog and duplicate physical versions.

Duplicate grouping follows logical identity. TMDB-backed episodic keys include media type + season + episode; fallback keys include normalized title + year + media type + season + episode. Different episodes must never be classified as duplicate copies.

`catalog_changes` records protected catalog/admin actions for audit/history.

## Backups and restore

`system_backups` registers SQLite snapshots stored under the database directory's `backups/` folder.

- Scan safety backups may be reused for up to 30 minutes; structural path/category operations create a fresh snapshot.
- Automatic retention keeps the newest 10 successfully registered snapshots and does not orphan a file if filesystem deletion fails.
- A registry row whose file is missing cannot satisfy the safety gate.
- Manual backup is available from Admin.
- Restore never overwrites the live DB. Admin stages `<database>.restore`, fsyncs it, and requires a restart for activation.
- On next startup, before opening the primary DB, StormFlix runs SQLite `quick_check` on the staged file.
- An invalid/corrupt staged restore is renamed to `.restore.invalid-<timestamp>` and the current database continues starting normally instead of entering a restart/502 loop.
- For a valid restore, the previous main DB plus any `-wal`/`-shm` sidecars are moved to a timestamped pre-restore safety copy.
- If final activation fails, StormFlix rolls the previous database/sidecars back into place.

Media files and asset folders are not part of this SQLite backup mechanism and are never deleted by restore.

## Large-catalog Web performance / artwork caching

`catalog-performance.js` renders at most 28 cards per rail initially and loads further chunks with `IntersectionObserver`/manual load-more. Card images use lazy loading, async decode and low fetch priority.

Authenticated local `/assets/` responses use private one-day browser caching with stale-while-revalidate. When `AssetPublicBaseURL` is configured, that external URL remains the intended CDN layer and controls its own public cache policy.

## Schema history

- Phase 9: `media_series_identity`.
- Phase 10: `series_metadata_overrides`, category `parent_id`, performance indexes.
- Phase 11: ordered series-child index and legacy episode-manual cleanup.
- Phase 12: persistent `scan_jobs` and metadata job type/series/provider fields.
- **Phase 13:** category smart rules, `media_technical`, `catalog_changes`, `profile_home_menus`, `system_backups` and playback diagnostic columns.

Migrations are forward/startup migrations. Android/Jellyfin compatibility v2 adds no destructive database migration.

## Required validation before merge/release

- JavaScript syntax checks for public Web + Admin, including the deferred Jellyfin Android bridge bundle.
- `go test ./...`.
- `go test -race ./internal/playback ./internal/webcompat` when playback/cache concurrency changes.
- `go build -trimpath ./cmd/stormflix`.
- Android Gradle debug APK build when Android source/version changes.
- Exact PR-head CI **and** Android workflow must be green before merge for this release.
- Post-merge `main` CI **and** Android workflow must also be green before deployment is presented as ready.

Compatibility regression coverage includes official-client route registration and the embedded Android WebView credential bridge. Existing automation/playback/scanner regression coverage remains mandatory.

## Real-world QA after deployment

Architecture/tests do not replace device/storage validation. Check representative mounted media for Direct Play, remux/audio-compatibility, multi-audio pt-BR selection, 4K/HDR where supported, dynamic-HLS buffer diagnostics, SSD budget/cleanup, smart Dublado/Legendado/4K rails, profile-specific menu ordering, dry-run scan against real mounts and backup/restore from the Admin flow.

For this Android/Jellyfin phase additionally test:

- official Jellyfin Android phone: server connect → StormFlix login page → StormFlix Home layout → credentials/native shell setup;
- official Jellyfin Android TV and Fire TV: login, Home views, item details, resume/latest/next-up, PlaybackInfo, direct stream, audio/subtitle selection and progress;
- StormFlix Android 0.4.0 phone + Android TV + Fire TV: custom root menus (including Desenhos), child gallery sections, profile menu order/visibility, search/detail/player.

## Documentation rule

After meaningful architecture/compatibility/schema/playback/deployment changes, update this file and `CHANGELOG.md`. Never store secrets, credentials or private media paths.
