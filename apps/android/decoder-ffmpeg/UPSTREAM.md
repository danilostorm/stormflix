# Media3 FFmpeg audio decoder

Sources: androidx/media tag `1.11.0`, commit `2bc207851df311340767e913931ca7b28cab1794`,
`libraries/decoder_ffmpeg/src/main` and `proguard-rules.txt` (Apache-2.0).
https://github.com/androidx/media/tree/2bc207851df311340767e913931ca7b28cab1794/libraries/decoder_ffmpeg

This is the same local audio-decoding architecture used by Just Player. No
Just Player UI or application code is copied. The native StormFlix controller
continues to own profiles, progress, markers and playback handoff.

FFmpeg 6.0.1 source: https://ffmpeg.org/releases/ffmpeg-6.0.1.tar.xz
SHA-256: 9b16b8731d78e596b4be0d720428ca42df642bb2d78342881ff7f5bc29fc9623
The upstream module recommends FFmpeg 6.0. The build enables only audio
decoders; no encoders, demuxers, protocols, GPL or nonfree components are enabled.
FFmpeg remains separately licensed under LGPL-2.1-or-later. Full corresponding
FFmpeg source is packaged alongside APK artifacts/releases, with this repository
providing the JNI/module sources and build recipe. The APK includes the notices.
Users may rebuild and replace the library/app for their own use and debugging.

Local changes: standalone Android-library Gradle file; JNI linker alignment of
16 KiB; build script bounds parallel make jobs through STORMFLIX_DECODER_JOBS.
The Java decoder and JNI implementation are unchanged upstream sources.

Build on Linux with Java 17, Android SDK 36, NDK 26.1.10909125, CMake 3.22.1,
and Gradle 9.5. Run `bash apps/android/build-audio-decoder.sh` from the repository
root before `gradle -p apps/android :app:assembleDebug`. Set ANDROID_SDK_ROOT if
ANDROID_HOME is not present. The script verifies the archive checksum. Rebuilding
from the source release is also supported by placing the verified archive at
`apps/android/decoder-ffmpeg/build/ffmpeg-6.0.1.tar.xz` before running the script.

Hardware video decoders are still required for video formats/levels/HDR.
Decoded multichannel audio is not a promise of Atmos/DTS bitstream passthrough.
