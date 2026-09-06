import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

const script=await readFile(new URL('../internal/webui/static/device-capabilities.js',import.meta.url),'utf8');
function device({height=1080,decode,native,timeout=setTimeout}={}){
  const window={};
  if(native)window.StormFlixShell={catalogQuery:()=>native};
  const context=vm.createContext({window,document:{createElement:()=>({canPlayType:()=> 'probably'})},navigator:decode?{mediaCapabilities:{decodingInfo:decode}}:{},screen:{width:height*16/9,height},innerWidth:1920,innerHeight:1080,devicePixelRatio:1,URLSearchParams,setTimeout:timeout,clearTimeout});
  vm.runInContext(script,context);
  return window;
}

for(const height of [480,1080,2160]){
  const client=device({height});
  await client.sfCatalogCapabilityQuery();
  assert.equal(client.sfDeviceCapabilitySnapshot().max_height,1080,'unknown UHD support must stay FHD');
  const caps=client.sfConstrainPlaybackCapabilities({video_codecs:['h264']});
  assert.equal(caps.video_profiles[0].max_frame_rate,0,'FHD 60fps must not get a new 30fps limit');
}

let probes=0;
const uhd=device({height:2160,decode:async config=>{probes++;return {supported:true,smooth:config.video.contentType.includes('hvc1')}}});
await Promise.all([uhd.sfCatalogCapabilityQuery(),uhd.sfCatalogCapabilityQuery()]);
assert.equal(probes,4,'capability probes must be cached and shared');
assert.equal(uhd.sfDeviceCapabilitySnapshot().max_height,2160);
assert.deepEqual(Array.from(uhd.sfDeviceCapabilitySnapshot().video_codecs),['hevc']);
const mixed=uhd.sfConstrainPlaybackCapabilities({video_codecs:['hevc','h264']});
assert.equal(mixed.video_profiles[0].max_height,2160);
assert.equal(mixed.video_profiles[1].max_height,1080,'UHD support is per codec');
uhd.sfRejectUHDCodec('hevc');
assert.equal(uhd.sfConstrainPlaybackCapabilities({video_codecs:['hevc']}).video_profiles[0].max_height,1080);
assert.equal(new URLSearchParams(await uhd.sfCatalogCapabilityQuery()).get('client_video_codecs'),null,'failed UHD codec must leave subsequent catalog hints');

const timeoutClient=device({height:2160,decode:()=>new Promise(()=>{}),timeout:callback=>setTimeout(callback,1)});
await timeoutClient.sfCatalogCapabilityQuery();
assert.equal(timeoutClient.sfDeviceCapabilitySnapshot().max_height,1080,'hung probes must fail closed');
const rejected=device({height:2160,decode:async()=>{throw new Error('API unavailable')}});
await rejected.sfCatalogCapabilityQuery();
assert.equal(rejected.sfDeviceCapabilitySnapshot().max_height,1080);

const android=device({native:'?client_max_height=2160&client_video_codecs=hevc&client_hdr_known=0'});
await android.sfCatalogCapabilityQuery();
const browser={video_codecs:['h264','hevc'],containers:['mp4'],audio_codecs:['aac'],native_audio_track_selection:false,server_selects_audio:true};
const constrained=android.sfConstrainPlaybackCapabilities(browser,{video_codecs:['hevc'],containers:['mkv'],audio_codecs:['ac3'],native_audio_track_selection:true,server_selects_audio:false,video_profiles:[{codec:'hevc',max_width:3840,max_height:2160,max_frame_rate:60}]});
for(const key of ['containers','audio_codecs','native_audio_track_selection','server_selects_audio'])assert.deepEqual(constrained[key],browser[key],`native limits must not replace WebView ${key}`);
assert.equal(constrained.video_profiles[0].max_height,1080);
assert.equal(constrained.video_profiles[1].max_height,2160);
console.log('Device capability runtime tests passed');
