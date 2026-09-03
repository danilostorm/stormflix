# StormFlix Playback Engine v6 — Adaptive Local Decode

Playback Engine v6 adds a client-side compatibility route without weakening the existing Direct Play-first policy.

## Decision order

For normal Web playback, PlaybackPlan evaluates routes in this order:

1. **Direct Play** when container, video, audio, bitrate and device profile are natively compatible.
2. **Local decode** when the only video problem is an unsupported HEVC codec and the Web client explicitly advertises a safe local decoder.
3. **Remux / audio compatibility** when video is compatible but the container, selected audio or server-side audio selection requires mediation.
4. **Video transcode** when codec/device/resolution/framerate/HDR/bitrate or an explicit lower quality requires server-side adaptation.
5. **Unsupported** when no safe route is available.

The new PlaybackPlan mode is `local_decode`. Plans also expose `local_decode`, `local_decode_engine` and `local_decode_codec` so UI and diagnostics do not confuse client work with server video transcoding.

## HEVC Web local decode

The first v6 local engine is HEVC/H.265 for the browser. StormFlix uses the MIT-licensed `hevc.js` HLS plugin/runtime:

- `@hevcjs/hlsjs-plugin` 0.1.2;
- `@hevcjs/core` 1.4.2.

When native HEVC playback is unavailable but local decode is eligible, the server reuses the existing continuous Web fMP4/HLS session with **video stream-copy**. The HEVC bitstream reaches the browser without server video re-encoding. The hevc.js runtime decodes HEVC through WebAssembly and uses WebCodecs to produce a browser-consumable H.264 stream locally. When muxed audio is not AAC, StormFlix may convert only the selected audio track to AAC while preserving video copy.

This is intentionally different from Direct Play: CPU/GPU work still happens, but it happens on the client rather than consuming the StormFlix server video encoder.

## Capability and safety gates

Local HEVC is selected only when all of the following are true:

- client kind is Web/desktop;
- the internal automatic local-decode policy considers the client eligible;
- WebAssembly is available;
- Worker is available;
- WebCodecs `VideoDecoder` and `VideoEncoder` are available;
- classic MediaSource is available for the HLS path;
- the page is a secure context (HTTPS) or localhost;
- source resolution is within the client-advertised local budget;
- source HDR is not requested unless a future engine explicitly advertises safe HDR handling;
- the incompatibility reason is the video codec itself, not an explicit bandwidth/resolution/device limit.

The desktop browser estimates a conservative automatic budget from `hardwareConcurrency` and `deviceMemory`: low-power desktops cap at 720p, stronger clients at 1080p and high-core/high-memory desktops advertise up to 3840×2160 automatically. There is no user-facing decode or 4K switch. Android/WebView, mobile Web and Tizen/webOS/TV clients never advertise this browser-local engine and retain their native/server fallback routes.

## What still goes to server transcode

PlaybackPlan deliberately bypasses local decode for:

- explicit lower-quality requests such as 720p from a 4K source;
- explicit Direct Play bitrate ceilings;
- HDR when local HDR handling has not been proven;
- resolution beyond the client local-decode budget;
- non-Web clients such as native Android/TV/Cast/DLNA routes;
- unsupported local runtime/browser environments.

Those paths continue through the mature NVENC/QSV/VAAPI/CPU server pipeline.

## Runtime failure fallback

If the local runtime cannot initialize, its CDN assets cannot be loaded, hls.js reports a fatal media error, or playback cannot reach the first frame, the Web client marks local decode unavailable for that runtime session and asks PlaybackPlan again. The second plan naturally selects the existing server compatibility/transcode path.

There is no silent retry loop that continually reselects a known-failed local runtime.

## Admin and player diagnostics

The player shows `WASM LOCAL DECODE` as a distinct mode and exposes:

- source codec/resolution;
- local decoder engine;
- observed local processing speed when supplied by the runtime;
- server video hardware as “not used for video” on a local plan;
- any audio-only AAC conversion separately.

The quality menu does not expose local-decode controls. Admin → Reprodução/Transcodificação shows the local desktop Web-client capability state and automatic resolution budget as read-only diagnostics. PlaybackPlan owns the route so a normal user cannot disable, force or misconfigure WASM.

## Third-party runtime delivery

The v6 implementation pins exact hevc.js package versions but, in this first delivery, loads the plugin/Worker/WASM assets from public package CDNs on first local-decode use. The normal server transcode path remains available if those assets are unreachable. A later hardening step may cache/self-host these pinned assets similarly to the Games runtime.

See `THIRD_PARTY_NOTICES.md` for the hevc.js MIT license notice.
