# StormFlix Performance Foundation v2

This package turns the Home, SQLite and playback resource paths into bounded,
observable production components. It does not replace Direct Play or make an
experimental browser decoder the default for media it cannot reproduce safely.

## Delivered architecture

### Home and catalog

- Phase 25 adds `catalog_entities` and `catalog_entity_members`, a rebuildable
  logical read model containing one card per movie/version group or series.
- Source-table triggers advance `catalog_projection_state.source_revision` on
  media, metadata, artwork, series-identity and library mutations.
- The previous committed projection remains readable while one background
  refresh builds the next revision. The swap happens in one SQLite transaction.
- Home, series lists, category galleries, releases, trending ID resolution and
  related-title candidates read the projection instead of regrouping the full
  physical catalog for every request.
- Recent episodes and next-episode recommendations use bounded indexed SQL
  queries; they no longer materialize every season and episode in Go.
- Static Home data keeps a two-minute fresh cache and a ten-minute
  stale-while-revalidate window. Cache entries carry the catalog revision, so a
  completed projection refresh invalidates the correct snapshot automatically.
- `/api/v1/home` returns `Server-Timing`, `X-StormFlix-Home-Cache` and
  `X-StormFlix-Catalog-Revision`. Admin → Servidor reports the last 256 Home
  latencies (p50/p95/p99) and cache-state counters.

The target remains cached Home p95 below 500 ms on a representative large
catalog. The telemetry makes that target measurable on the real Unraid/rclone
installation; it is not claimed from synthetic unit tests.

### Web delivery

- Embedded JS/CSS/WASM assets have deterministic ETags, one-hour browser cache
  plus stale-while-revalidate, and one-time in-process gzip generation.
- JSON/text API responses negotiate gzip while Range/video responses remain
  byte-exact and uncompressed.
- Secondary Music, Games, profile, detail and Player CSS no longer blocks the
  first Home paint. Rows use bounded incremental DOM rendering and
  `content-visibility` outside the viewport.
- The short session Home snapshot is scoped by account, role, allowed library
  IDs, selected profile and kids/rating state; it cannot leak a previous
  profile's catalog after a permission change.
- New TMDB downloads request display-sized backdrops/logos/stills instead of
  provider originals. Existing local originals are not destructively changed.

### FFmpeg and stream resource budgets

All server FFmpeg work shares one scheduler:

| Setting | Default | Purpose |
|---|---:|---|
| `STORMFLIX_MAX_FFMPEG_PROCESSES` | 4 | Bounds video, audio-copy/remux and compatibility workers together. |
| `STORMFLIX_MAX_VIDEO_TRANSCODES` | 2 | Smaller semaphore for expensive video encodes. |
| `STORMFLIX_TRANSCODE_CPU_THREADS` | 6 | Per-process CPU/filter cap, automatically clamped to available CPUs. |
| `STORMFLIX_WEB_STREAM_CACHE_MAX_BYTES` | 5 GiB | Bounds continuous Web session storage. |
| `STORMFLIX_WEB_STREAM_MAX_AHEAD` | 60 s | Stops an FFmpeg worker that has produced enough future media. |
| `STORMFLIX_WEB_STREAM_KEEP_BEHIND` | 4 min | Removes old segments behind current playback. |
| `STORMFLIX_WEB_STREAM_WORKER_IDLE_TTL` | 90 s | Ends abandoned workers even if a close event never arrives. |

Playback heartbeats update the requested segment and playing/paused state.
Workers use `SIGSTOP`/`SIGCONT` to avoid racing through a whole film and expose
PID, wait time, paused state, encoder and hardware in Admin diagnostics.

NVENC is now eligible after the reliable software HDR→SDR tone-map filter. This
keeps the color conversion conservative while moving the final H.264/HEVC encode
off CPU. QSV/VAAPI HDR surfaces remain disabled until separately capability
tested; all hardware candidates retain automatic CPU fallback.

The base Compose remains portable. A host with NVIDIA Container Toolkit uses:

```bash
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d --build
docker exec stormflix ffmpeg -hide_banner -encoders 2>/dev/null \
  | grep -E 'h264_nvenc|hevc_nvenc'
```

If the encoders are listed but FFmpeg cannot open them, verify the host driver,
NVIDIA Container Toolkit and device permissions. StormFlix will record the
failed hardware attempt and fall back to CPU instead of breaking playback.

## Why the reported 4K HDR playback used the server

The screenshot showed `VIDEO TRANSCODE`, not `WASM LOCAL DECODE`. That is an
intentional safety result for the current local engine:

1. the source is 3840×1600 HEVC/HDR;
2. local HDR output is not yet color-proven, so PlaybackPlan does not advertise
   it as a safe browser route;
3. a browser needing SDR therefore asks the server to tone-map/transcode;
4. the previous base Compose did not expose the RTX to the container, leaving
   CPU FFmpeg as the available fallback.

This package fixes items 3/4 operationally with NVENC-after-tone-map support,
the NVIDIA overlay and CPU/process limits. It does **not** falsely label HDR as
local decode. Non-HDR HEVC on an eligible secure desktop still uses the hidden,
automatic local route and keeps server video as stream-copy.

## SQLite decision

SQLite remains the correct production database for the current StormFlix
architecture: one server process owns a local metadata database while large
video/artwork bytes live outside it. Catalog size alone is not a PostgreSQL
migration reason.

The connection layer now uses a URL-safe modernc DSN, applies WAL,
`synchronous=NORMAL`, foreign keys, a five-second busy timeout and WAL checkpoint
policy to every pooled connection, and deliberately limits the pool to four
connections. Phase 24 adds an audited migration ledger/checksum; Phase 25 is the
first migration recorded after that baseline. Admin reports schema version,
file/WAL size, pool usage and wait count.

Plan PostgreSQL only when at least one real requirement appears:

- more than one independent StormFlix server must write the same catalog;
- high concurrent write volume produces sustained lock waits after profiling;
- database latency, not remote storage or FFmpeg, misses the SLO under measured
  production load;
- operational requirements demand external replication/failover or SQL access
  from other services.

Before any future migration, introduce a repository/query boundary and run a
dual-backend conformance suite. Do not replace SQLite merely because the DB file
grows.

## Browser/WASM research decision

No evaluated repository was copied into this package. Adding a library only to
display “WASM” would increase download, client memory and failure surface while
often performing worse than native hardware decode.

| Project | StormFlix decision | Reason |
|---|---|---|
| [ffmpeg.wasm](https://github.com/ffmpegwasm/ffmpeg.wasm) | Lab utility, not the 4K Player hot path | General browser conversion/recording stack; upstream labels stability experimental. Useful later for small offline clips/probes, expensive for full-film realtime decode. MIT. |
| [ffmpeg-wasm streaming example](https://github.com/ColinEberhardt/ffmpeg-wasm-streaming-video-player) | Reference only | Demonstrates streaming bytes into browser FFmpeg; it is an example, not a maintained media-server playback engine. |
| [WasmVideoPlayer](https://github.com/sonysuqin/WasmVideoPlayer) | Rejected for production | Exploration using FFmpeg 3.3/old Emscripten, MP4/FLV focus and no seek; full-frame YUV/PCM path is CPU/memory heavy. GPL-3.0 also requires an intentional compatibility decision. |
| [webrtc_H265player](https://github.com/xiangxud/webrtc_H265player) | Reference only | Bare FFmpeg 4.1 HEVC→YUV decoder pipeline, not a complete authenticated VOD player. |
| [WXInlinePlayer](https://github.com/ErosZy/WXInlinePlayer) | Rejected | Mobile FLV/live specialization, old toolchain and Anti-996 license; poor fit for MKV/MP4 on-demand libraries. |
| [h265web.js](https://github.com/numberwolf/h265web.js) | Hold | Interesting HEVC Web pipeline, but packaging/licensing and production maintenance must be resolved before evaluation in an isolated adapter. |
| [EasyPlayer.js](https://github.com/EasyDarwin/EasyPlayer.js) | Hold | Broad surveillance/live protocol player. Its MSE/WebCodecs/WASM ideas are useful, but it would duplicate StormFlix authorization, HLS, controls and progress. No verified root license file was found during review. |
| [libmedia](https://github.com/zhaohappy/libmedia) | Best future lab candidate | Modular TypeScript/WASM codecs, MKV/MP4/HLS, WebCodecs and 8/10-bit/HDR render paths. Multi-thread mode needs SharedArrayBuffer via COOP/COEP, which must be isolated because current third-party/Cast flows can be affected. License/integration boundary must be verified first. |
| [dotLottie Web](https://github.com/LottieFiles/dotlottie-web) | Do not add for playback | Animation renderer, unrelated to video codec compatibility. Load lazily only if a future UI feature actually uses `.lottie`. |
| [OxidePlayer](https://github.com/PaulSpaurgen/video-player) | Do not adopt | Small React wrapper with optional per-frame effects; StormFlix is not React-based and this does not solve HEVC/HDR compatibility. MIT. |
| [W3C WebCodecs](https://github.com/w3c/webcodecs) | Standards reference | Continue capability detection around the browser API; it is a specification/incubation repository, not a decoder to bundle. |
| [wasp-hls](https://github.com/peaBerberian/wasp-hls) | Transport research only | Rust/WASM HLS engine can inform demux/ABR work but does not provide the missing HEVC decoder. MIT. |
| [rtsp-wasm-player](https://github.com/ikuokuo/rtsp-wasm-player) | Out of scope | RTSP/WebSocket live ingest/player, whereas StormFlix playback is authenticated VOD over Range/fMP4 HLS. MIT. |

The next local-decoder experiment, if scheduled, should be a separately loaded
`libmedia` adapter behind the existing PlaybackPlan capability contract. It must
prove seek, subtitles, audio selection, 10-bit HDR color, memory ceiling,
recovery and a lower total cost than server NVENC on representative files before
replacing the current route.

## Production checks

After deployment:

```bash
curl -s http://127.0.0.1:8090/healthz
docker stats stormflix
docker logs --tail=200 stormflix
```

Open Admin → Servidor/Reprodução and verify:

- Home `p95_ms` and cache hit/stale/miss counters after normal navigation;
- projection `built_revision == source_revision` after a scan settles;
- SQLite WAL/foreign keys/schema version and connection wait count;
- active FFmpeg process/video counts never exceed configured limits;
- an HDR fallback reports `nvidia`/`h264_nvenc` when the GPU overlay is active;
- eligible non-HDR HEVC reports `WASM LOCAL DECODE`, with no user toggle.
