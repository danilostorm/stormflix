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
- Matching a principal series stores one provider decision and rebuilds scanner identities; the episode reorganization is now an observable queued `series_refresh` job instead of an invisible background goroutine.
- Future episodes inherit the same series override automatically.
- `Arquivos / diagnóstico` remains available for low-level inspection, but episodic rows no longer expose manual matching and instead point back to the principal work.
- Added phase11 cleanup so legacy builds that marked every episode `manual_match=1` are normalized; series protection now lives only in `series_metadata_overrides`.
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
