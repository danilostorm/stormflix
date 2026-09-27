#!/usr/bin/env python3
"""Measure StormFlix cached Home latency against the production P0 SLO.

Environment:
  STORMFLIX_BASE_URL   e.g. http://127.0.0.1:8090
  STORMFLIX_COOKIE     full Cookie header (preferred)
  STORMFLIX_SESSION    session cookie value (alternative)
  STORMFLIX_PROFILE    profile cookie value (optional)
  STORMFLIX_SAMPLES    measured requests, default 30
  STORMFLIX_WARMUP     warmup requests, default 3

The script uses only the Python standard library and never prints cookie values.
"""

from __future__ import annotations

import os
import statistics
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


def percentile(values: list[float], p: float) -> float:
    if not values:
        return 0.0
    data = sorted(values)
    rank = (len(data) - 1) * p
    lo = int(rank)
    hi = min(len(data) - 1, lo + 1)
    frac = rank - lo
    return data[lo] * (1 - frac) + data[hi] * frac


def cookie_header() -> str:
    direct = os.environ.get("STORMFLIX_COOKIE", "").strip()
    if direct:
        return direct
    session = os.environ.get("STORMFLIX_SESSION", "").strip()
    profile = os.environ.get("STORMFLIX_PROFILE", "").strip()
    parts: list[str] = []
    if session:
        parts.append(f"stormflix_session={session}")
    if profile:
        parts.append(f"stormflix_profile={profile}")
    return "; ".join(parts)


def main() -> int:
    base = os.environ.get("STORMFLIX_BASE_URL", "http://127.0.0.1:8090").rstrip("/")
    samples = max(5, int(os.environ.get("STORMFLIX_SAMPLES", "30")))
    warmup = max(1, int(os.environ.get("STORMFLIX_WARMUP", "3")))
    cookie = cookie_header()
    if not cookie:
        print("Missing authentication. Set STORMFLIX_COOKIE or STORMFLIX_SESSION.", file=sys.stderr)
        return 2

    query = urllib.parse.urlencode(
        {
            "client_max_height": "1080",
            "client_video_codecs": "h264,hevc,av1",
            "client_hdr_known": "0",
        }
    )
    url = f"{base}/api/v1/home?{query}"
    headers = {
        "Accept": "application/json",
        "Cookie": cookie,
        "User-Agent": "StormFlix-Home-SLO/1.0",
        "Cache-Control": "no-cache",
    }

    latencies: list[float] = []
    cache_states: dict[str, int] = {}
    revisions: dict[str, int] = {}
    server_timings: list[str] = []

    total = warmup + samples
    for index in range(total):
        request = urllib.request.Request(url, headers=headers, method="GET")
        started = time.perf_counter()
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                response.read()
                elapsed_ms = (time.perf_counter() - started) * 1000
                cache = response.headers.get("X-StormFlix-Home-Cache", "unknown")
                revision = response.headers.get("X-StormFlix-Catalog-Revision", "unknown")
                timing = response.headers.get("Server-Timing", "")
        except urllib.error.HTTPError as exc:
            print(f"HTTP {exc.code}: {exc.read().decode('utf-8', 'replace')[:300]}", file=sys.stderr)
            return 3
        except Exception as exc:
            print(f"Request failed: {exc}", file=sys.stderr)
            return 4

        if index < warmup:
            print(f"warmup {index + 1}/{warmup}: {elapsed_ms:.1f} ms cache={cache}")
            continue

        latencies.append(elapsed_ms)
        cache_states[cache] = cache_states.get(cache, 0) + 1
        revisions[revision] = revisions.get(revision, 0) + 1
        if timing:
            server_timings.append(timing)
        print(f"sample {index - warmup + 1:02d}/{samples}: {elapsed_ms:.1f} ms cache={cache} rev={revision}")

    p50 = percentile(latencies, 0.50)
    p95 = percentile(latencies, 0.95)
    p99 = percentile(latencies, 0.99)

    print("\nStormFlix Home P0 production measurement")
    print(f"samples: {len(latencies)}")
    print(f"mean:    {statistics.mean(latencies):.1f} ms")
    print(f"p50:     {p50:.1f} ms")
    print(f"p95:     {p95:.1f} ms")
    print(f"p99:     {p99:.1f} ms")
    print(f"cache:   {cache_states}")
    print(f"revs:    {revisions}")
    if server_timings:
        print(f"last Server-Timing: {server_timings[-1]}")

    if p95 < 500:
        print("RESULT: PASS — cached Home p95 is below 500 ms.")
        return 0
    print("RESULT: FAIL — cached Home p95 is at/above 500 ms.")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
