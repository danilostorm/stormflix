// Real full-page playback startup, including stalled metadata and cancellation.
// Requires playwright 1.51.1, Chromium, and ffmpeg (CI installs them).
import assert from 'node:assert/strict';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(import.meta.dirname,'../internal/webui/static');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'stormflix-decoder-'));
for(const [name,codec] of [['sample.mkv','ac3'],['sample-hevc.mkv','ac3']]){
  execFileSync('ffmpeg',['-hide_banner','-loglevel','error','-f','lavfi','-i','testsrc2=size=320x180:rate=24',
    '-f','lavfi','-i','sine=frequency=440:sample_rate=48000','-f','lavfi','-i','sine=frequency=880:sample_rate=48000',
    '-map','0:v','-map','1:a','-map','2:a','-t','12','-c:v',name.includes('hevc')?'libx265':'libx264','-preset','ultrafast','-pix_fmt',name.includes('hevc')?'yuv420p10le':'yuv420p',
    ...(name.includes('hevc')?['-x265-params','pools=1:frame-threads=1:log-level=error']:[]),
    '-c:a',codec,'-metadata:s:a:0','language=eng','-metadata:s:a:1','language=por',path.join(temp,name)]);
}
const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://localhost');
  if(url.pathname==='/'){
    res.setHeader('Content-Type','text/html');res.end(fs.readFileSync(path.join(root,'index.html')));return;
  }
  if(url.pathname.startsWith('/api/')){res.setHeader('Content-Type','application/json');if(url.pathname==='/api/v1/auth/me')res.statusCode=401;res.end(url.pathname==='/api/v1/setup/status'?'{}':'[]');return}
  const media=url.pathname==='/sample.mp4'||url.pathname==='/sample.mkv'||url.pathname==='/sample-hevc.mkv';
  const file=path.join(media?temp:root,url.pathname);
  if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
  const bytes=fs.readFileSync(file);
  res.setHeader('Content-Type',file.endsWith('.wasm')?'application/wasm':file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':file.endsWith('.json')?'application/json':file.endsWith('.mp4')?'video/mp4':'video/x-matroska');
  res.setHeader('Accept-Ranges','bytes');
  const range=/bytes=(\d+)-(\d*)/.exec(req.headers.range||'');
  if(range){
    const start=Number(range[1]),end=Math.min(bytes.length-1,range[2]?Number(range[2]):bytes.length-1);
    if(start>=bytes.length){res.writeHead(416,{'Content-Range':`bytes */${bytes.length}`});res.end();return}
    res.writeHead(206,{'Content-Range':`bytes ${start}-${end}/${bytes.length}`,'Content-Length':end-start+1});res.end(bytes.subarray(start,end+1));
  }else{res.setHeader('Content-Length',bytes.length);res.end(bytes)}
});
await new Promise(resolve=>server.listen(0,'0.0.0.0',resolve));
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_EXECUTABLE||undefined,headless:true,args:['--no-sandbox','--host-resolver-rules=MAP stormflix.test 127.0.0.1','--no-proxy-server']});
try {
 const page=await browser.newPage({viewport:{width:1280,height:720}});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(`http://localhost:${server.address().port}`);
 await page.waitForSelector('#sf-simple-speed',{state:'attached'});
 // Real playMedia entry point; all optional metadata requests remain pending.
 // Before the fix the planner/decoder were never reached.
 for (const [id,name] of [[1,'sample.mkv'],[2,'sample-hevc.mkv']]) {
  await page.evaluate(({id,name})=>{
   document.querySelector('#profile-picker')?.classList.add('hidden');
   window.request=async url=>{
    if(url.includes('/playback/plan'))return {available:true,mode:'local_decode',local_origin:true,media_id:id,url:location.origin+'/'+name,source_container:'mkv',audio_stream:1,video_width:320,video_height:180};
    if(/\/(versions|subtitles|streams)$/.test(url))return new Promise(()=>{});
    return [];
   };
   window.started=playMedia({id,title:'Original '+name});
  },{id,name});
  await page.waitForFunction(()=>window.sfLocalDecodeStats?.decoded_frames>10&&player.currentTime>.2,{},{timeout:50000});
  await page.evaluate(()=>window.started);
  assert.equal(await page.locator('#player-help').isVisible(),false);
  assert.equal(await page.evaluate(()=>player.paused),false);
  assert.equal(await page.locator('#sf-local-origin-surface canvas').isVisible(),true);
  await page.screenshot({path:path.join(temp,'full-'+name+'.png')});
  await page.evaluate(()=>closePlayer());
 }
 // Errors must be visible rather than leaving an empty black player.
 await page.evaluate(async()=>{
  window.request=async url=>{if(url.includes('/playback/plan'))throw new Error('Arquivo indisponível no servidor');return []};
  await playMedia({id:3,title:'Unavailable'});
 });
 assert.equal(await page.locator('#player-help').innerText(),'Arquivo indisponível no servidor');
 assert.equal(await page.locator('#player-help').isVisible(),true);
 await page.evaluate(()=>closePlayer());
 // Closing while the plan is pending must never reopen the player.
 await page.evaluate(()=>{
  window.request=url=>url.includes('/playback/plan')?new Promise(resolve=>window.releasePlan=resolve):Promise.resolve([]);
  window.pendingStart=playMedia({id:5,title:'Cancelled'});
 });
 await page.waitForFunction(()=>typeof window.releasePlan==='function');
 await page.evaluate(async()=>{closePlayer();releasePlan({available:true,local_origin:true,url:location.origin+'/sample.mkv'});await pendingStart});
 assert.equal(await page.locator('#player-modal').isVisible(),false);
 assert.equal(await page.evaluate(()=>sfLocalOrigin.isActive()),false);
 // A stuck planner also has a visible, bounded failure.
 await page.evaluate(async()=>{
  const original=setTimeout;window.setTimeout=(fn,ms,...args)=>original(fn,ms===30000?50:ms,...args);
  window.request=url=>url.includes('/playback/plan')?new Promise(()=>{}):Promise.resolve([]);
  try {await playMedia({id:6,title:'Slow server'})} finally {window.setTimeout=original}
 });
 assert.match(await page.locator('#player-help').innerText(),/servidor demorou/);
 await page.evaluate(()=>closePlayer());
 // Timed-out local initialization must reject even if libmedia never settles.
 await page.evaluate(async()=>{
  window.AVPlayer=class {on(){} setVolume(){} load(){return new Promise(()=>{})} destroy(){return Promise.resolve()}};
  const original=setTimeout;
  window.setTimeout=(fn,ms,...args)=>original(fn,ms>=20000?50:ms,...args);
  try {await sfLocalOrigin.load('/never.mkv',{media_id:4,source_container:'mkv'});throw new Error('Expected timeout')}
  catch(error){if(error.code!=='SOURCE_TIMEOUT')throw error}
  finally {window.setTimeout=original;await sfLocalOrigin.destroy()}
 });
 assert.deepEqual(errors,[]);
 console.log('Full Web MKV/HEVC startup, pending optional metadata, visible errors and local timeout passed:',temp);
} finally { await browser.close(); await new Promise(r=>server.close(r)) }
