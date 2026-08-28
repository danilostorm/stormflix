# StormFlix Unified Playback Architecture

Status: implemented native playback stack for Web, Android and TV/Fire.

## Goal

StormFlix behaves as one streaming platform with platform-specific players and one shared playback policy.

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 native API ──> Playback Core ──> Media
StormFlix TV/Fire ───┘             │
                                    ├── Profiles / progress
                                    ├── Library access
                                    ├── Metadata / subtitles
                                    └── Compatibility Cache Manager

Jellyfin clients ──> Jellyfin compatibility facade ──> StormFlix core
```

The Jellyfin facade is a compatibility boundary only. Native StormFlix clients do not depend on Jellyfin endpoints.

## Reference boundary

Jellyfin projects were studied for architecture and behavior only. StormFlix implements its own `/api/v1` playback stack and does not copy Jellyfin GPL implementation code.

Adopted concepts include:

- player implementations behind a common playback policy;
- device capabilities participating in source selection;
- server planning separated from client execution;
- native Media3 playback on Android/TV;
- TV-specific remote/focus behavior rather than stretching a touch UI;
- managed lifecycle for generated compatibility media.

## Non-negotiable policy

1. Direct Play first.
2. Never silently transcode video.
3. If video is supported and only audio is incompatible, use audio-only compatibility: video stream-copy + AAC-LC audio.
4. A container-only mismatch may use remux with stream copy when MP4 compatibility permits it.
5. Unsupported video is explicit until a separate opt-in video-transcoding product policy exists.
6. Profile audio preference is authoritative; `pt-BR → pt → por` and Portuguese/Dublado/Brasil labels remain understood.
7. Library/profile access is checked before a plan or source URL is returned.
8. Ordered progress semantics must not regress.
9. Exact selected `audio_stream` must survive from planning through FFmpeg execution.
10. Generated compatibility media is cache, not permanent library data.

## Playback Core

The native Playback Core is split into:

- **Source Probe** — describes actual media streams;
- **Capability Model** — describes what a client can consume;
- **Decision Engine** — combines source + capabilities + profile preferences;
- **Playback Plan** — immutable source/mode decision executed by clients;
- **Playback Session** — ordered runtime progress identity;
- **Compatibility Execution Adapter** — seekable remux/AAC materialization;
- **Compatibility Cache Manager** — owns lifecycle/storage safety for generated MP4 artifacts.

## PlaybackPlan modes

- `direct_play` — original source, no re-encode;
- `remux` — repack/container or exact audio-track selection while streams are copied where valid;
- `audio_compatibility` — video copied, exact selected audio converted to AAC-LC;
- `unsupported` — client cannot consume video under the no-silent-video-transcode policy.

`POST /api/v1/media/{id}/playback/plan` performs access checks, resume lookup, source probe, profile language selection and capability evaluation. It returns execution/prepare URLs, exact stream choices and a playback-session ID.

## Capabilities

### Browser

StormFlix Web prefers runtime detection over User-Agent assumptions:

- `HTMLMediaElement.canPlayType` for container/codec support;
- feature detection for Picture-in-Picture, fullscreen and Media Session;
- server-side selected-audio execution when browser multi-audio APIs are unreliable.

### Android / TV

Media3 clients publish MediaCodec-derived audio/video decoder capabilities, common resolution/frame-rate profiles, container support and native track-selection support. TV/Fire uses the same plan contract but keeps TV-specific focus/D-pad orchestration.

## StormFlix Web Player v4

`playback-core.js` is the browser policy/execution controller. `player-v4.js` and `player-v4.css` are the visible player shell.

The v4 shell intentionally replaces the old-looking v2/v3 presentation. It provides:

- cinematic top/bottom overlays;
- custom SVG controls;
- played/buffered seek bar and pointer time preview;
- title/series/episode context;
- playback mode and resolution/quality indicators;
- play/pause, ±10s, volume/mute;
- audio/subtitle/settings controls;
- previous/next episode actions;
- fullscreen and native Picture-in-Picture;
- desktop/mobile responsive presentation;
- automatic control hiding during playback.

The visual shell never makes codec/transcode policy decisions. All source policy remains in PlaybackPlan.

## Android / Android TV / Fire TV

Media3 is the primary native player.

The native client:

- requests PlaybackPlan before each source;
- publishes real local decoder capabilities;
- keeps original multi-audio Direct Play when a usable decoder exists;
- requests AAC compatibility only when needed;
- preserves playback session and seek position across source switches;
- tests alternative physical versions through PlaybackPlan;
- keeps native audio/subtitle menus, MediaSession and phone/tablet PiP;
- keeps TV/Fire D-pad and remote-first behavior.

## Compatibility execution

`internal/webcompat` is an execution adapter, not a second policy engine.

For remux/audio compatibility StormFlix produces a normal seekable MP4 so `http.ServeContent` can provide:

- `Content-Length`;
- HTTP Range requests;
- `206 Partial Content`;
- stable seek timeline.

Video remains stream-copy. Audio is copied when valid for MP4 or encoded AAC-LC only when the plan requires it.

The cache key contains media/source revision, exact video/audio stream and transcode choice so different selected languages cannot collide.

## Managed compatibility cache

Compatibility MP4s live under:

`<DataDir>/compat-cache`

They are generated data and may be safely evicted. Database, artwork and original media are never cache-manager targets.

### Default policy

- maximum: 20 GiB;
- TTL: 48h;
- cleanup interval: 15m;
- LRU eviction target: 85% of maximum;
- abandoned temp threshold: 1h;
- oversize idle TTL: 15m;
- minimum free disk: 10 GiB or 5% of filesystem, whichever is larger.

Environment overrides:

- `STORMFLIX_COMPAT_CACHE_MAX_BYTES`
- `STORMFLIX_COMPAT_CACHE_TTL`
- `STORMFLIX_COMPAT_CACHE_AUTO_CLEANUP`
- `STORMFLIX_COMPAT_CACHE_CLEANUP_INTERVAL`
- `STORMFLIX_COMPAT_CACHE_OVERSIZE_TTL`
- `STORMFLIX_MIN_FREE_DISK_BYTES`
- `STORMFLIX_MIN_FREE_DISK_PERCENT`

The same principal settings are available in Admin and persisted through the generic settings table.

### LRU metadata

The cache does not rely on filesystem atime. It persists a small manifest:

`.stormflix-cache.json`

Each artifact records:

- deterministic filename/key;
- size;
- creation time;
- last-used time;
- oversize status.

Last-used timestamps are refreshed when a cached artifact is reused/served, not per byte.

### Active-file safety

The manager keeps an in-memory active reference count for cache artifacts.

An artifact cannot be evicted while:

- it is being materialized;
- it is being served by a remux/Range request;
- its matching FFmpeg `.tmp` file is actively being written.

Temporary files older than the abandoned threshold are removed only when no matching final artifact is active.

### Oversize files

A single source can be larger than the entire configured cache maximum. Example: a 42 GiB source with a 20 GiB cache budget.

StormFlix does **not** pretend such a file fits the persistent budget. It may materialize the source when sufficient disk reserve exists because seekable Range playback still requires a normal artifact, but the resulting entry is marked oversize and becomes short-lived. Once idle, it is removed on the oversize TTL rather than remaining permanently above budget.

This policy preserves correct playback while preventing one very large movie from becoming permanent cache data.

### Disk-pressure protection

Before creating a new compatibility artifact, the manager estimates size from the original file when possible.

It then:

1. applies normal cleanup;
2. evicts inactive LRU entries when the new item would exceed budget;
3. checks filesystem free space;
4. evicts additional inactive cache if the free-space reserve would be violated;
5. refuses materialization with a friendly server error if reserve still cannot be maintained.

FFmpeg must not be allowed to fill the filesystem blindly.

### Startup and periodic cleanup

At startup the cache directory is reconciled with the persisted manifest. Legacy `.mp4` files created before Cache Manager are adopted using file modification time as initial last-use evidence.

Heavy startup cleanup runs asynchronously so server startup is not blocked. Periodic cleanup applies TTL, oversize expiry, LRU maximum and abandoned-temp cleanup.

This means a legacy cache that already grew far beyond the new limit is reduced after deployment without deleting unrelated StormFlix data.

### Admin surface

`GET /api/v1/admin/playback/cache`

Returns usage, maximum, TTL, auto-cleanup state, file/active counts, oldest entry, free disk and last cleanup statistics.

`POST /api/v1/admin/playback/cache/cleanup`

Runs manual cleanup of inactive compatibility artifacts only and returns files/bytes removed.

Admin → Configurações → Playback · Cache de compatibilidade exposes status, max size, TTL, automatic cleanup, disk reserve and manual cleanup.

## Concurrency guarantees

- Materialization remains serialized per deterministic cache key.
- Cache eviction and HTTP serving coordinate through active references.
- Cleanup never deletes an active final artifact.
- Cleanup never deletes a matching active FFmpeg temp artifact.
- Race tests run for `internal/playback` and `internal/webcompat` in CI.

## Testing requirements

Core regression coverage includes:

- H.264/AAC direct play;
- audio-only AAC compatibility;
- unsupported video never silently transcoded;
- Android native HEVC/direct play capability;
- language fallback order;
- exact selected-audio execution;
- resolution/frame-rate/HDR/bitrate limits;
- cache below/above maximum;
- LRU order and target;
- TTL;
- active final-file safety;
- active and abandoned temp behavior;
- oversize expiry;
- manual cleanup isolation;
- path confinement;
- unlimited mode;
- race detector for playback/cache packages.

## License boundary

Jellyfin is only an architectural/behavior reference. StormFlix does not import or paste Jellyfin GPL implementation code. Literal reuse would require an explicit license review before entering the repository.
