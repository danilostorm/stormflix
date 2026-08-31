#!/usr/bin/env bash
set -euo pipefail

: "${TIZEN_PROFILE:?Set TIZEN_PROFILE to the Samsung/Tizen certificate profile name}"

ROOT=$(cd "$(dirname "$0")" && pwd)
cd "$ROOT"
rm -rf .buildResult build
mkdir -p build

tizen build-web -- .
tizen package -t wgt -s "$TIZEN_PROFILE" -o build -- .buildResult

echo "StormFlix Tizen package created in: $ROOT/build"
