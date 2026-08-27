# StormFlix Changelog

This file records user-visible and architectural changes. `PROJECT_STATE.md` is the authoritative current-state handoff.

## 2026-08-27

### Catalog identity and metadata

- Added scanner-owned episodic identity (`media_series_identity`) so folder hierarchy determines show/season/episode before external metadata providers.
- Added `animation_series` for western cartoons and `anime_series` for anime organized as television seasons.
- Improved parsing for technical subfolders, compact episode names and Brazilian season folders.
- Added TheTVDB v4 as optional series/cartoon fallback with Admin settings for API Key and optional PIN.
- Added native HAMA-style Anime-Lists bridge between AniDB/AniList/MAL and TVDB/TMDB identifiers.
- Added series-level manual metadata overrides.
- **Corrected the manual-match workflow to be principal-series only:** Admin → Catálogo now defaults to `Obras principais`, grouping an episodic library into one card per `series_key` instead of one card per episode.
- Matching a principal series stores one provider decision, immediately rebuilds scanner identities and refreshes the current episodes in the background; future episodes inherit the same series override.
- `Arquivos / diagnóstico` remains available for low-level inspection, but episodic rows no longer expose manual matching and instead point back to the principal work.
- Added phase11 cleanup so legacy builds that marked every episode `manual_match=1` are normalized; series protection now lives only in `series_metadata_overrides`.
- Added a regression test requiring two episodes of the same scanner series to appear as one principal work in the Admin catalog.

### Admin UI

- Fixed a race in `Metadados & Capas` that could render the **Agentes de Música** panel twice.

### Categories

- Added parent/child category hierarchy.
- Parent categories aggregate descendant libraries while child categories remain individually browsable.
- Added Admin category tree editor and secondary client subcategory navigation.

### Home performance

- Added phase10 SQLite read indexes for artwork, recent/title ordering, ratings/status and series identity.
- Added 20-second access-scope-aware cache for the static grouped Home feed.
- Continue Watching remains uncached and profile-specific.
- Parallelized independent Home reads (static grouped catalog, Continue Watching, trending windows and Releases).

### Jellyfin compatibility

- Jellyfin discovery now advertises a numeric compatible server version and supports official-client root paths.
- Added mobile WebView startup bridge.
- Added Android TV/Fire post-login endpoints, modern `/UserViews`, `/UserItems/Resume`, `/Items/Latest` aliases and client crash logging.
- Fixed required `UserDto` fields and non-null Jellyfin `UserData` objects.
- Fixed Jellyfin artwork delivery for clients whose image loader does not carry the native StormFlix session.

### Playback

- Restored preferred Portuguese audio selection before codec fallback.
- AAC compatibility fallback keeps video stream-copy and prepares seekable cached MP4 output with HTTP range support.
- Ordered playback progress prevents stale requests from overwriting newer resume positions while allowing intentional backward seeks.
- Replaced repeated Fire TV seek Toasts with bounded in-player seek feedback.

## Documentation maintenance

From this point forward, meaningful architecture/compatibility/schema/playback updates should update this file and `PROJECT_STATE.md` in the same development round.
