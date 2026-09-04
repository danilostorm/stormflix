# Third-party notices

StormFlix is developed independently. This file records third-party source or visual work that is adapted directly enough to require attribution.

## hls.js

StormFlix self-hosts the official hls.js 1.7.1 browser distribution so HLS playback does not depend on a public CDN at runtime. hls.js is licensed under the Apache License 2.0. The unmodified license is shipped as `vendor-hls-LICENSE.txt` beside the embedded web assets.

Source project: `video-dev/hls.js`, release `v1.7.1`

## RomMix

The StormFlix Games G2.5 browser interface adapts portions of the visual/interaction language of **RomMix**, including its game-home hero, horizontal game rows, controller-oriented navigation cues and settings presentation. StormFlix keeps its own catalog, authorization, browser playback, saves and server APIs.

Source project: `leclercb/rommix`

MIT License

Copyright (c) 2026 Benjamin Leclerc

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## hevc.js

StormFlix Playback Engine v6 uses the MIT-licensed **hevc.js** HLS plugin/runtime as an optional browser-side HEVC compatibility engine. It is activated only when native Direct Play is unavailable and the client explicitly advertises the required secure WebAssembly/Worker/WebCodecs capabilities. StormFlix continues to own PlaybackPlan, media authorization, fMP4 HLS generation, fallback policy and player UX.

Source project: `lid-labs/hevc.js`

MIT License

Copyright (c) 2025 Thibaut Lion

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## RomM

RomM is used as a product/architecture research reference for metadata-provider breadth, library-management concepts, BIOS/save workflows and community feature requests. RomM is AGPL-3.0; StormFlix does **not** copy RomM implementation source into its native Games module. Concepts are reimplemented behind StormFlix-owned Go/SQLite/Web interfaces.
