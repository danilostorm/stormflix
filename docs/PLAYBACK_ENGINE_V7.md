# StormFlix Playback Engine v7 — local-origin

Playback Engine v7 implements Package 3 without replacing the proven routes.
Native Direct Play remains the first choice. If the browser cannot consume the
source natively, an eligible desktop can receive the authenticated original file
with HTTP Range and perform demux, decode and render locally.

## Route order

1. native Direct Play;
2. local-origin: original Range source + libmedia on the client;
3. v6 HEVC/HLS client compatibility;
4. remux or video-copy/audio-AAC compatibility;
5. server video transcode;
6. unsupported.

`local_origin=true` is additive to the existing `local_decode` plan mode.
`transport=original_range` is required before the HTTP handler bypasses the Web
session manager. The returned URL is `/api/v1/media/{id}/stream`, with the same
authentication, profile, kids and library checks as Direct Play. No FFmpeg
binary is detected or started for this route.

## Client engine

The Web client self-hosts `@libmedia/avplayer` 1.3.1. It:

- demuxes MKV, MP4 and WebM in the browser;
- enables WebCodecs as the first decoder;
- falls back to pinned WebAssembly SIMD decoders;
- renders through the local browser graphics stack;
- decodes AAC, AC3, EAC3, DTS, MP3, Opus, FLAC and Vorbis locally;
- switches embedded audio tracks without restarting a server stream;
- handles embedded and StormFlix VTT subtitles;
- seeks against the original byte-range source;
- limits preload to three seconds and tears down the player, workers and canvas
  on close/source change.

The runtime runs in worker mode without SharedArrayBuffer. This intentionally
avoids global COOP/COEP headers that could break Cast and TV/external-player
integrations. WebAssembly SIMD is mandatory for this first guarded route.

## Automatic eligibility and fallback

There is no user or administrator enable/disable switch. PlaybackPlan requires:

- secure desktop Web context (localhost also qualifies);
- WebAssembly + SIMD, Worker and WebGL;
- a supported source container/video/audio matrix;
- the existing automatic resolution budget.

Direct Play is always evaluated first. Explicit lower-quality or bitrate limits
continue to mean server adaptation. HDR is excluded until its local color path
is proven on representative devices. Android WebView, mobile Web, Tizen, webOS,
TV, Cast and DLNA do not advertise this browser engine.

If runtime loading, demux, decode or rendering fails, the client records the
failure for the current browser runtime, destroys local resources and asks for
one fresh PlaybackPlan without local-origin capability. That naturally selects
v6 compatibility or the established server fallback and avoids a retry loop.

## Supply chain and licensing

`static/vendor-libmedia/UPSTREAM.txt` pins the npm integrity and source commit.
`COPYING.LGPLv3` ships the complete LGPL notice, and `SHA256SUMS` covers every
embedded JavaScript/WASM artifact. Only player/decoder/resampler modules are
included; GPL x264/x265 and all other encoder modules are excluded.

## Validation boundary

Automated tests cover decision order, Range transport selection, the absence of
FFmpeg setup on a local-origin plan, capability fallback, HDR exclusion, audio
selection, runtime wiring, asset integrity/MIME/cache behavior and teardown.

Production observation is still required for long HEVC Main/Main10, 1080p/4K,
repeated seeks, external/embedded subtitles, AAC/AC3/DTS, memory stability, CPU,
dropped frames and HDR color behavior. Player diagnostics expose local buffer
and dropped-frame values for that acceptance. A failing client falls back
automatically rather than making server stability depend on the experiment.
