#!/usr/bin/env bash
set -euo pipefail

ANDROID_PROJECT="$(cd "$(dirname "$0")" && pwd)"
DECODER_MAIN="$ANDROID_PROJECT/decoder-ffmpeg/src/main"
DECODER_BUILD="$ANDROID_PROJECT/decoder-ffmpeg/build"
DECODER_NDK="${ANDROID_NDK_HOME:-${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}/ndk/26.1.10909125}"
DECODER_ARCHIVE="$DECODER_BUILD/ffmpeg-6.0.1.tar.xz"
DECODER_SHA=9b16b8731d78e596b4be0d720428ca42df642bb2d78342881ff7f5bc29fc9623

if [[ ! -d "$DECODER_NDK/toolchains/llvm/prebuilt/linux-x86_64" ]]; then
    echo 'Install Android NDK 26.1.10909125 and set ANDROID_SDK_ROOT or ANDROID_NDK_HOME.' >&2
    exit 1
fi
mkdir -p "$DECODER_BUILD"
if [[ ! -f "$DECODER_ARCHIVE" ]]; then
    curl --fail --location --retry 3 --max-time 180 \
        https://ffmpeg.org/releases/ffmpeg-6.0.1.tar.xz -o "$DECODER_ARCHIVE.tmp"
    mv "$DECODER_ARCHIVE.tmp" "$DECODER_ARCHIVE"
fi
echo "$DECODER_SHA  $DECODER_ARCHIVE" | sha256sum --check

# The CI cache key includes this script, NDK version, and upstream JNI sources.
if [[ -f "$DECODER_MAIN/jni/ffmpeg/.stormflix-audio-built" ]]; then
    echo 'Verified FFmpeg source; reusing cached audio decoder objects.'
    exit 0
fi
mkdir -p "$DECODER_MAIN/jni/ffmpeg"
tar -xJf "$DECODER_ARCHIVE" --strip-components=1 -C "$DECODER_MAIN/jni/ffmpeg"
bash "$DECODER_MAIN/jni/build_ffmpeg.sh" "$DECODER_MAIN" "$DECODER_NDK" linux-x86_64 23 \
    vorbis opus flac alac pcm_mulaw pcm_alaw mp3 amrnb amrwb aac ac3 eac3 dca mlp truehd
touch "$DECODER_MAIN/jni/ffmpeg/.stormflix-audio-built"
