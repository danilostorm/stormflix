# StormFlix Unified Playback Architecture

Status: **implemented** for StormFlix Web, Android, Android TV and Fire TV.

## Goal

StormFlix behaves as one streaming platform with platform-specific players, not as clients that independently rediscover playback rules.

```text
StormFlix Web ───────┐
StormFlix Android ───┼──> /api/v1 native API ──> Playback Core ──> Media
StormFlix TV/Fire ───┘             │
                                    ├── Profiles / progress
                                    ├── Library access
                                    └── Metadata / subtitles

Jellyfin clients ──> Jellyfin compatibility facade ──> StormFlix core
```

The Jellyfin facade is a compatibility boundary. Native StormFlix clients do not depend on Jellyfin playback endpoints.

## Reference study and license boundary

The public Jellyfin projects were studied as architectural references only. StormFlix keeps an original implementation and its native `/api/v1` contract. No Jellyfin GPL implementation is copied into the native Playback Core.

Concepts adopted at architecture level:

- player implementations consume a shared playback decision instead of owning server policy;
- device/profile capabilities participate in source selection;
- server-side playback planning is separate from client-side execution;
- Android uses a native Media3 player while sharing server semantics with Web;
- Android TV/Fire keeps remote/D-pad-specific orchestration instead of becoming a stretched touch UI.

## Non-negotiable policy

1. **Direct Play first.**
2. Never silently transcode video.
3. Never silently tone-map HDR.
4. If video is supported and only selected audio is incompatible, use `audio_compatibility`: video stream-copy + AAC-LC audio.
5. A container-only incompatibility may use `remux` with stream copy when MP4 compatibility permits it.
6. A browser that cannot reliably select a non-default multi-audio track may use a stream-copy remux to pin the profile-preferred track.
7. Unsupported video codec/resolution/frame-rate/HDR/explicit bitrate policy is reported explicitly.
8. Library/profile/kids access checks occur before a plan/source is returned.
9. Playback session + sequence/event ordering must remain monotonic so stale progress cannot overwrite a newer seek position.
10. Any future video-transcoding policy must be a separate, explicit product decision.

## Playback Core

`internal/playback` contains the shared native policy layer:

- **Source Probe** — describes the actual source without client policy;
- **Capability Model** — describes what a client can consume;
- **Decision Engine** — combines source, capabilities and profile preference;
- **Playback Plan** — immutable source/mode/track decision executed by a client;
- **Playback Session** — stable runtime identity used by progress/monitoring.

`POST /api/v1/media/{id}/playback/plan` is the authoritative native planning endpoint.

## Source Probe

The probe currently records:

- container;
- duration;
- source bitrate when available;
- video/audio/subtitle stream indexes;
- codecs;
- audio language/title/default disposition;
- video width/height;
- video frame rate;
- HDR transfer classification (`hdr10`, `hlg`) when ffprobe reports it.

Probe data describes the source only. It does not decide whether a client should receive Direct Play or compatibility media.

## Capability Model

Clients can publish:

- containers;
- video codecs;
- audio codecs;
- per-codec maximum width/height/frame-rate decode profiles;
- known HDR types;
- subtitle formats;
- audio passthrough telemetry;
- remux support;
- AAC audio-compatibility support;
- native multi-audio track selection;
- server-side preferred audio selection requirement;
- optional explicit Direct Play bitrate ceiling;
- Picture-in-Picture support;
- Media Session support.

Capabilities are declarative. Advertising a capability never enables video transcoding.

## Playback Plan modes

### `direct_play`

Original source. No re-encode.

Used when container/video/audio policy is compatible, or when a native Media3 client can keep a multi-audio source and select a locally decodable track itself.

### `remux`

Seekable MP4 compatibility output with video/audio stream copy.

Used for:

- container-only compatibility;
- browser server-side selection of a preferred non-default audio track when that selected audio can be copied to MP4.

### `audio_compatibility`

Seekable MP4 compatibility output where:

- video remains stream-copy;
- the exact selected audio stream is converted to AAC-LC.

This is used when selected audio cannot be decoded/copied in the required compatibility path but video itself is supported.

### `unsupported`

Explicit refusal. The plan carries a reason code such as unsupported codec, resolution, frame rate, HDR profile or explicit bitrate policy.

This mode is deliberately preferred over hidden video transcoding.

## Exact audio-stream execution

PlaybackPlan owns track choice. The execution adapter must not make a different language decision later.

For remux/AAC modes:

1. Playback Core selects an exact global `audio_stream` index;
2. plan `prepare_url` and `url` carry that index;
3. `internal/webcompat.ProbeWithAudioStream` makes that index authoritative;
4. the compatibility cache key includes the selected stream/codec/transcode choice;
5. range-served output therefore matches the profile decision end-to-end.

This also allows non-Portuguese profile preferences to work correctly; Portuguese is a default preference/fallback, not a hard-coded execution rule.

## StormFlix Web

`internal/webui/static/playback-core.js` owns automatic browser source policy.

It:

- derives broad browser support using `HTMLMediaElement.canPlayType`;
- requests PlaybackPlan before initial playback and each source/version switch;
- declares `server_selects_audio` because HTML multi-audio APIs are not consistent across browsers;
- executes exact `prepare_url` and source `url` returned by the server;
- preserves the same playback session across source/version changes;
- restores seek position after source replacement;
- drives Media Session actions and position state;
- exposes Picture-in-Picture when available;
- reports explicit planner/player errors instead of falling back to a second hidden source-selection policy;
- invalidates in-flight requests when the player closes.

Legacy `/compatibility`/`/remux` endpoints remain execution/manual-compatibility surfaces, not automatic policy owners.

## Android / Android TV / Fire TV

The native app uses Media3 and consumes the same PlaybackPlan contract.

`PlaybackCapabilities.java` publishes MediaCodec information for the current device, including supported video/audio codecs and common maximum resolution/frame-rate decode profiles.

`PlayerActivity`:

- requests PlaybackPlan before every source;
- carries one playback session through plan/source changes;
- allows Media3 native multi-audio selection when at least one local decoder can handle a track;
- uses server AAC compatibility when no usable local audio path exists or an explicitly chosen unsupported track needs recovery;
- tests alternative physical versions through PlaybackPlan;
- preserves seek/resume during replacement;
- retains audio/subtitle controls;
- retains previous/next episode behavior;
- provides Media3 `MediaSession`;
- provides PiP on supported phone/tablet devices;
- retains TV/Fire remote/D-pad/media-key-first behavior through `RemoteUi` and native player key handling.

TV/Fire shares playback semantics with Android but keeps TV navigation/focus behavior outside the server decision engine.

## Progress and session continuity

The planner accepts a safe client playback session ID and returns it when valid; otherwise it creates a new session ID.

Clients send:

- `playback_session_id`;
- monotonic `progress_sequence`;
- `progress_event_ms`;
- position/duration/state/mode/codec telemetry.

Server ordered-progress logic uses event/session/sequence ordering, not position ordering. A legitimate backward seek remains valid while a late asynchronous request cannot overwrite a newer event.

## Compatibility execution

`internal/webcompat` is intentionally an execution adapter:

- FFmpeg remux/audio-only conversion;
- seekable cached MP4 materialization;
- HTTP range delivery;
- exact audio-stream execution.

It is not the long-term policy owner.

## Testing requirements

Regression coverage includes:

- H.264 + AAC + MP4 => Direct Play;
- H.264 + DTS => audio-only AAC compatibility when needed;
- DTS that cannot be safely copied into MP4 does not masquerade as pure remux;
- unsupported HEVC/video capability => explicit Unsupported, no hidden video transcode;
- supported Android HEVC => Direct Play;
- resolution/frame-rate/HDR profile enforcement;
- explicit bitrate policy enforcement;
- Portuguese preference/fallback;
- browser server-side non-default preferred audio selection;
- native multi-audio Media3 selection;
- native audio selection cannot bypass video capability limits;
- exact selected `audio_stream` in execution URLs;
- playback session normalization/continuity;
- ordered progress regression tests.

CI must run Go formatting/tests/build and JavaScript syntax. Android CI must compile the native APK before playback changes are considered ready.

## Operational validation

Code architecture is complete. Device/library validation remains normal deployment QA: test representative H.264/AAC, multi-audio, DTS/TrueHD, HEVC/AV1, HDR and 4K files on available browsers, Android devices, Android TV and Fire TV. Tune capability declarations from observed device behavior; do not solve device-specific failures by adding silent video transcoding.
