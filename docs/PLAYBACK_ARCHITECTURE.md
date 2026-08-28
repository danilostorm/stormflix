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
                                    ├── Dynamic HLS session cache (Web)
                                    └── Compatibility MP4 cache (native/manual fallback)

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
- direct file streaming for compatible mounted media;
- dynamic HLS/fMP4 for browser compatibility instead of waiting for a whole rewritten movie;
- managed lifecycle for generated compatibility media.

## Non-negotiable policy

1. Direct Play first.
2. Never silently transcode video.
3. If video is supported and only audio is incompatible, use audio-only compatibility: video stream-copy + AAC-LC audio.
4. A container-only mismatch may use remux with stream copy when the client can consume the resulting stream.
5. Unsupported video is explicit until a separate opt-in video-transcoding product policy exists.
6. Profile audio preference is authoritative; `pt-BR → pt → por` and Portuguese/Dublado/Brasil labels remain understood.
7. Library/profile access is checked before a plan or source URL is returned.
8. Ordered progress semantics must not regress.
9. Exact selected `audio_stream` must survive from planning through FFmpeg execution.
10. Generated compatibility media is disposable cache, not permanent library data.
11. Storage origin is not playback policy: local disk, rclone/FUSE, Google Drive, NFS and other mounted sources are evaluated by their media streams, not by provider name.

## Playback Core

The native Playback Core is split into:

- **Source Probe** — describes actual media streams;
- **Capability Model** — describes what a client can consume;
- **Decision Engine** — combines source + capabilities + profile preferences;
- **Playback Plan** — immutable source/mode decision executed by clients;
- **Playback Session** — ordered runtime progress identity;
- **Dynamic HLS Execution Adapter** — browser remux/audio compatibility in small on-demand fMP4 batches;
- **HLS Session Cache Manager** — hard global budget and immediate session cleanup;
- **Compatibility MP4 Adapter/Cache** — retained for Android/TV/manual compatibility paths that still require a normal seekable file.

## PlaybackPlan modes

- `direct_play` — original source, no re-encode;
- `remux` — repack/container or exact audio-track selection while streams are copied where valid;
- `audio_compatibility` — video copied, exact selected audio converted to AAC-LC;
- `unsupported` — client cannot consume video under the no-silent-video-transcode policy.

`POST /api/v1/media/{id}/playback/plan` performs access checks, resume lookup, source probe, profile language selection and capability evaluation. It returns execution URLs, exact stream choices and a playback-session ID.

For StormFlix Web, non-Direct-Play plans are adapted to a session-scoped dynamic HLS URL. Direct Play continues to return the original `/stream` endpoint.

## Capabilities

### Browser

StormFlix Web prefers runtime detection over User-Agent assumptions:

- `HTMLMediaElement.canPlayType` for container/codec support;
- H.264, HEVC, AV1, VP9 and common audio probes including AAC, MP3, Opus, AC3, EAC3, DTS and FLAC;
- feature detection for Picture-in-Picture, fullscreen and Media Session;
- server-side selected-audio execution when browser multi-audio APIs are unreliable.

### Android / TV

Media3 clients publish MediaCodec-derived audio/video decoder capabilities, common resolution/frame-rate profiles, container support and native track-selection support. TV/Fire uses the same plan contract but keeps TV-specific focus/D-pad orchestration.

## StormFlix Web Player v4

`playback-core.js` is the browser policy/execution controller. `player-v4.js` and `player-v4.css` are the visible player shell.

The v4 shell provides:

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

### Direct Play

For compatible media, Web loads the original mounted source through `/api/v1/media/{id}/stream`.

The server opens the mounted path and serves it with `http.ServeContent` and HTTP Range support. StormFlix creates **no HLS or compatibility-file cache** for this path. A Google Drive/rclone mount is therefore consumed like any other filesystem source; rclone/VFS remains responsible for its own remote-read caching.

### Dynamic HLS compatibility

Web remux/audio compatibility no longer waits for a complete MP4 materialization.

The server exposes:

- `GET /api/v1/media/{id}/hls/{session}/index.m3u8`
- `GET /api/v1/media/{id}/hls/{session}/init/{batch}.mp4`
- `GET /api/v1/media/{id}/hls/{session}/segment/{segment}.m4s`

The playlist is a VOD fMP4 HLS timeline. Media batches are generated only when the browser requests them. Default execution uses:

- 6-second HLS segments;
- 4 segments per FFmpeg batch (about 24 seconds at a time);
- `-c:v copy` always;
- exact selected video/audio stream indexes;
- audio stream-copy when fMP4-compatible;
- AAC-LC only when the selected audio needs browser/fMP4 compatibility;
- input seeking to the requested batch position, which avoids reading or rewriting the complete remote movie before playback starts.

The browser uses pinned hls.js for MSE-capable browsers and native HLS when the browser provides it.

No web compatibility request is allowed to turn this into silent video transcoding.

## HLS session cache and SSD safety

Dynamic HLS fragments live under:

`<DataDir>/hls-cache/<playback-session>/`

This directory is disposable session state, not persistent media cache.

### Default global policy

- hard global maximum: **5 GiB**;
- segment duration: **6s**;
- batch size: **4 segments / ~24s**;
- idle-session fallback TTL: **30 minutes**;
- cleanup scan: 1 minute;
- minimum free filesystem reserve: the same **10 GiB or 5%**, whichever is larger;
- old fragments behind the active playback point are continuously removed.

Environment overrides:

- `STORMFLIX_HLS_CACHE_MAX_BYTES`
- `STORMFLIX_HLS_CACHE_IDLE_TTL`
- `STORMFLIX_HLS_SEGMENT_DURATION`
- `STORMFLIX_HLS_BATCH_SEGMENTS`
- `STORMFLIX_MIN_FREE_DISK_BYTES`
- `STORMFLIX_MIN_FREE_DISK_PERCENT`

The global maximum is shared by all users; it is not a per-user allowance.

### Immediate session cleanup

When the user closes the movie, the movie ends, or the browser sends its unload cleanup:

1. the client sends the playback session ID with `DELETE /api/v1/media/{id}/playback?session=<id>`;
2. the server verifies that the session belongs to that authenticated user;
3. any active FFmpeg batch for that session is cancelled;
4. the complete session directory is removed immediately.

A source/version switch using the same logical playback session also deletes the previous HLS fragments before creating the replacement session data. Switching from HLS compatibility to Direct Play deletes the previous HLS cache as well.

The 30-minute idle TTL is only a crash/disconnect safety net when normal close/unload signalling never reaches the server.

### Global pressure protection

Before a new HLS batch starts, the manager estimates that batch's size from source bitrate and reserves capacity **before** FFmpeg writes it.

It then:

1. measures current HLS usage;
2. evicts oldest disposable fragments if `usage + estimated batch` would exceed the global budget;
3. preserves sessions with an active FFmpeg batch from eviction;
4. checks the filesystem free-space reserve;
5. refuses the new batch if enough safe space cannot be made.

This makes the HLS maximum a capacity budget rather than a cleanup target noticed after the SSD is already full.

At server startup, the dedicated `hls-cache` directory is emptied because fragments from a previous process/session are never authoritative or permanent.

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

Android/TV still use the existing normal seekable MP4 compatibility executor when a server-side compatibility file is required. The dynamic HLS change is intentionally scoped to StormFlix Web in this architecture round.

## Legacy compatibility MP4 execution

`internal/webcompat/materialize.go` remains for native/manual compatibility flows that require a normal seekable MP4.

For that path StormFlix preserves:

- `Content-Length`;
- HTTP Range requests;
- `206 Partial Content`;
- stable seek timeline;
- video stream-copy;
- exact selected audio stream;
- AAC-LC only when required.

Those artifacts remain under `<DataDir>/compat-cache` and use the existing managed 20 GiB/48h LRU/TTL policy. They are no longer the automatic Web execution path.

## Concurrency guarantees

- One FFmpeg worker is coordinated per HLS session/batch.
- Concurrent requests for a segment in the active batch join the same worker.
- Source/session closure cancels the worker before removing the session directory.
- Global HLS pressure eviction skips sessions with a running FFmpeg worker.
- Session ownership is checked before HLS playback or cache deletion.
- HLS session IDs are path-confined and traversal-safe.
- Legacy compatibility MP4 materialization remains serialized per deterministic cache key.
- Race tests run for `internal/playback` and `internal/webcompat` in CI.

## Testing requirements

Core regression coverage includes:

- H.264/AAC Direct Play;
- audio-only AAC compatibility;
- unsupported video never silently transcoded;
- Android native HEVC/Direct Play capability;
- language fallback order;
- exact selected-audio execution;
- resolution/frame-rate/HDR/bitrate limits;
- HLS playlist/batch layout;
- immediate HLS session deletion on close;
- cross-user HLS session deletion rejection;
- source-switch HLS cleanup;
- idle/crashed-session HLS cleanup;
- hard global HLS budget eviction;
- HLS session path confinement;
- legacy compatibility-cache LRU/TTL/active-file safety;
- race detector for playback/cache packages.

## License boundary

Jellyfin is only an architectural/behavior reference. StormFlix does not import or paste Jellyfin GPL implementation code. Literal reuse would require an explicit license review before entering the repository.
