# StormFlix Unified Playback Architecture

Status: foundation design for the native StormFlix playback stack.

## Goal

StormFlix should behave as one streaming platform with platform-specific players, not as three clients that independently rediscover playback rules.

The target split is:

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 native API ──> Playback Core ──> Media
StormFlix TV/Fire ───┘             │
                                    ├── Profiles / progress
                                    ├── Library access
                                    └── Metadata / subtitles

Jellyfin clients ──> Jellyfin compatibility facade ──> StormFlix core
```

The Jellyfin facade remains a compatibility boundary. Native StormFlix clients must not depend on Jellyfin endpoints.

## Reference study

The Jellyfin projects were studied as architectural references only. No Jellyfin implementation is copied into StormFlix.

Useful concepts adopted:

- the web player is a player implementation behind a playback manager instead of playback behavior being only a raw HTML `<video>` assignment;
- device/profile capabilities participate in source selection;
- server-side playback decisions and client-side player execution are separate responsibilities;
- Android can expose a native player to the application while sharing server playback semantics;
- Android TV has TV-specific playback/navigation orchestration instead of treating television as a stretched touch UI.

StormFlix keeps an original implementation and its existing native `/api/v1` contract.

## Current StormFlix state

### Server

Playback currently spans several areas:

- `GET /api/v1/media/{id}/stream` serves original media and is the Direct Play path;
- `internal/webcompat` probes streams and can materialize a seekable MP4;
- audio compatibility keeps video stream-copy and converts only the selected audio track to AAC-LC;
- `/api/v1/media/{id}/compatibility` exposes the legacy web-oriented compatibility decision;
- `/api/v1/media/{id}/playback` stores monitoring and profile progress;
- ordered progress already supports playback session, sequence and event ordering so stale events cannot overwrite newer progress.

These are valuable primitives, but capability/selection policy is not yet centralized.

### Web

The main web application starts playback by assigning `/api/v1/media/{id}/stream` directly to the HTML video element. `webcompat.js`, `player-monitor.js`, `continue-watching.js` and `player-v3.js` then wrap or patch playback behavior.

This works, but source selection, compatibility, resume and monitoring are distributed across global JavaScript hooks.

### Android

The native application already uses Media3 and has device capability inspection. It should be retained and migrated to consume the same native Playback Plan used by the web client instead of owning a separate set of playback rules.

### TV / Fire

TV/Fire behavior must remain remote/D-pad-first. Its player should be native and consume the same Playback Plan contract, while TV navigation and focus behavior remain platform-specific.

## Non-negotiable policy

1. **Direct Play first.**
2. Never silently transcode video.
3. If video is supported and only audio is incompatible, prefer `AUDIO_COMPATIBILITY`: video stream-copy + AAC-LC audio.
4. A container-only incompatibility may use `REMUX` with video and audio copy when possible.
5. Unsupported video must be reported explicitly until an opt-in video-transcoding policy is designed.
6. Profile language preference keeps the existing Portuguese priority (`pt-BR`, `pt`, `por`, plus Portuguese/Dublado/Brasil labels).
7. Library/profile access checks occur before a playback plan or source URL is returned.
8. Ordered progress semantics must not regress.

## Playback Core

The native Playback Core is split conceptually into:

- **Source Probe**: describes the actual media streams without deciding for a specific client;
- **Capability Model**: describes what a client can consume;
- **Decision Engine**: combines source + capabilities + preferences;
- **Playback Plan**: immutable result executed by a client;
- **Playback Session**: runtime state/progress contract.

The first implementation deliberately keeps these pieces small and testable.

## Playback Request

The initial native request contains:

- media id (from the URL);
- client kind (`web`, `android`, `tv`);
- client name/version when available;
- supported containers;
- supported video codecs;
- supported audio codecs;
- whether seekable MP4 remux/audio compatibility is supported;
- preferred audio language;
- resume position supplied by the server/profile state.

Future additions may include HDR/Dolby Vision profiles, maximum resolution/frame-rate, passthrough, bitrate/network constraints and subtitle rendering capabilities.

## Playback Plan

The plan uses explicit modes:

- `DIRECT_PLAY`: original source, no re-encode;
- `REMUX`: container repackaging, video/audio copied;
- `AUDIO_COMPATIBILITY`: video copied, selected audio encoded to AAC;
- `UNSUPPORTED`: no silent video transcode is allowed.

A plan also carries selected streams/codecs, reason codes/text and the URL the client should execute.

## Web player migration

The first visible migration target is StormFlix Web.

Instead of every layer rewriting `player.src`, a `StormFlixWebPlayer` controller will own:

1. browser capability detection;
2. Playback Plan request;
3. source preparation when the plan requires cached compatibility media;
4. source loading and resume;
5. heartbeat/progress/session lifecycle;
6. audio/subtitle controls;
7. Media Session, PiP, fullscreen and keyboard behavior;
8. error/retry reporting.

During migration the existing UI controls remain usable. Compatibility endpoints remain available as adapters until all clients use the new core.

## Browser capability detection

The web client should prefer runtime capability APIs over User-Agent assumptions:

- `HTMLMediaElement.canPlayType` for broad container/codec support;
- `MediaCapabilities.decodingInfo` where available for codec decoding confidence;
- feature detection for PiP, fullscreen and Media Session.

User-Agent may be included only as descriptive telemetry, not as the primary codec policy.

## Android migration

Media3 remains the primary Android player. Android will publish codec/container capabilities and request the same Playback Plan. The server decides source/track/mode; Media3 executes it.

Android-specific work remains client-side: lifecycle, PiP, MediaSession, touch controls, orientation and MediaCodec capability discovery.

## TV / Fire migration

TV/Fire uses the same Playback Plan contract but a TV-specific client/player shell. D-pad focus, overlays, remote keys, lean-back layouts and TV lifecycle stay outside the shared server decision engine.

## Migration phases

### Phase A — audit and contract

- document current paths and invariants;
- introduce capability/source/plan contracts;
- introduce deterministic decision tests.

### Phase B — Playback Core

- add source probe adapter;
- add native plan endpoint under `/api/v1`;
- preserve old stream/remux endpoints as execution primitives;
- keep all access checks.

### Phase C — Web player

- add browser capability detector;
- introduce one web player controller;
- route initial playback through Playback Plan;
- fold compatibility and monitoring wrappers into the controller incrementally.

### Phase D — Android

- publish MediaCodec/Media3 capabilities;
- consume Playback Plan;
- remove duplicated source-selection rules only after parity tests.

### Phase E — TV / Fire

- consume Playback Plan from the native TV player;
- keep D-pad/focus navigation independent from touch UI.

### Phase F — advanced capabilities

- HDR/DV/passthrough policy;
- bandwidth/quality policy;
- richer subtitle capability planning;
- opt-in video transcoding design if ever required.

## Testing strategy

Decision-engine unit tests are required for at least:

- H.264 + AAC + MP4 supported => Direct Play;
- H.264 + DTS with AAC available => audio-only compatibility;
- HEVC unsupported by web => Unsupported, never silent video transcode;
- HEVC supported by Android => Direct Play;
- Portuguese language preference/fallback;
- resume data carried into the plan;
- existing ordered progress tests continue passing.

Integration tests should verify that authorization is checked before plan creation and that generated plan URLs stay under native `/api/v1`.

## License boundary

Jellyfin is used only to study public architecture and behavior. StormFlix does not import or paste Jellyfin GPL implementation code. If literal reuse is ever considered, licensing impact must be reviewed before code enters this repository.
