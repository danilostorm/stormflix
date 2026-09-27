import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

// Execute production capability/recovery functions with a deterministic browser
// matrix. Reproduces a failed HEVC/MP4/AAC source poisoning all MP4/AAC playback.
const source = fs.readFileSync(new URL('../internal/webui/static/playback-core-v53.js', import.meta.url), 'utf8');
function between(start, end) {
  const from = source.indexOf(start), to = source.indexOf(end, from + start.length);
  assert(from >= 0 && to > from, `missing production function ${start}`);
  return source.slice(from, to);
}
const runtime = vm.createContext({
  canPlay: type => /mp4|aac|mpeg/.test(type) && !/ac-3|ec-3|fLaC|av01/.test(type),
  player: {}, document: {}, navigator: {},
  window: {sfDeviceCapabilitySnapshot: () => ({max_height:1080, video_codecs:['h264', 'hevc']})},
  estimatedTranscodeBitrate: () => 20000,
  browserLocalDecodeCapabilities: () => ({max_height:1080, max_width:1920}),
});
vm.runInContext('let nativeSourceRejected=false;' +
  between('  function browserCapabilities()', '  function hasWebGL()') +
  between('  function rejectNativePlan(plan)', '  async function recoverRuntime') +
  between('  function clientRequest(', '  function compatibilityMode('), runtime);
const evaluate = code => JSON.parse(JSON.stringify(vm.runInContext(code, runtime)));
const before = evaluate('browserCapabilities()');
assert.equal(evaluate("rejectNativePlan({mode:'direct_play',container:'mp4',video_codec:'hevc',audio_codec:'aac'})"), true);
const after = evaluate("clientRequest('', '480p', 42, 2)");
assert.deepEqual(after.capabilities.containers, before.containers);
assert.deepEqual(after.capabilities.video_codecs, before.video_codecs);
assert.deepEqual(after.capabilities.audio_codecs, before.audio_codecs);
assert.equal(after.native_source_rejected, true);
assert.equal(after.quality, 'original');
assert.equal(after.original_only, true);
assert.equal(after.start_position_seconds, 42);
assert.equal(after.audio_stream, 2);
for (const flag of ['allow_remux', 'allow_audio_compatibility', 'allow_video_transcode']) assert.equal(after.capabilities[flag], false);
assert.equal(evaluate("rejectNativePlan({mode:'direct_play'})"), false, 'recovery must be bounded');
console.log('Original playback: failed HEVC keeps MP4/AAC claims; client conversions disabled.');
