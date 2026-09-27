// Real decoder smoke test: original HTTP Range + bundled WASM, including HTTP LAN.
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
const failures=[];
for(const [name,codec] of [['sample.mp4','aac'],['sample.mkv','ac3']]){
  execFileSync('ffmpeg',['-hide_banner','-loglevel','error','-f','lavfi','-i','testsrc2=size=320x180:rate=24',
    '-f','lavfi','-i','sine=frequency=440:sample_rate=48000','-f','lavfi','-i','sine=frequency=880:sample_rate=48000',
    '-map','0:v','-map','1:a','-map','2:a','-t','12','-c:v','libx264','-preset','ultrafast','-pix_fmt','yuv420p',
    '-c:a',codec,'-metadata:s:a:0','language=eng','-metadata:s:a:1','language=por',path.join(temp,name)]);
}
let rangeRequests=0;
const server=http.createServer((req,res)=>{
  const url=new URL(req.url,'http://localhost');
  if(url.pathname==='/'){
    res.setHeader('Content-Type','text/html');
    res.end('<video id="player"></video><div id="sf-local-origin-surface" style="width:640px;height:360px" hidden></div><script src="/local-origin-player.js"></script>');return;
  }
  const media=url.pathname==='/sample.mp4'||url.pathname==='/sample.mkv';
  const file=path.join(media?temp:root,url.pathname);
  if(!fs.existsSync(file)){res.writeHead(404);res.end();return}
  const bytes=fs.readFileSync(file);
  res.setHeader('Content-Type',file.endsWith('.wasm')?'application/wasm':file.endsWith('.js')?'text/javascript':file.endsWith('.mp4')?'video/mp4':'video/x-matroska');
  res.setHeader('Accept-Ranges','bytes');
  const range=/bytes=(\d+)-(\d*)/.exec(req.headers.range||'');
  if(range){
    if(media)rangeRequests++;
    const start=Number(range[1]),end=Math.min(bytes.length-1,range[2]?Number(range[2]):bytes.length-1);
    if(start>=bytes.length){res.writeHead(416,{'Content-Range':`bytes */${bytes.length}`});res.end();return}
    res.writeHead(206,{'Content-Range':`bytes ${start}-${end}/${bytes.length}`,'Content-Length':end-start+1});res.end(bytes.subarray(start,end+1));
  }else{res.setHeader('Content-Length',bytes.length);res.end(bytes)}
});
await new Promise(resolve=>server.listen(0,'0.0.0.0',resolve));
const browser=await chromium.launch({headless:true,args:['--no-sandbox','--autoplay-policy=no-user-gesture-required','--host-resolver-rules=MAP stormflix.test 127.0.0.1','--no-proxy-server']});
try{
  for(const host of ['localhost','stormflix.test']){
    const page=await browser.newPage();
    page.on('pageerror',e=>failures.push(String(e)));
    page.on('console',msg=>{if(msg.type()==='error')console.error(msg.text())});
    await page.goto(`http://${host}:${server.address().port}`);
    assert.equal(await page.evaluate(()=>isSecureContext),host==='localhost');
    for(const name of ['sample.mp4','sample.mkv']){
      await page.evaluate(async name=>{
        window.errors=[];document.querySelector('#player').addEventListener('error',()=>window.errors.push(window.sfPlaybackLastError));
        await window.sfLocalOrigin.load('/'+name,{media_id:1,source_container:name.split('.').pop(),audio_stream:2},{autoplay:true});
      },name);
      await page.waitForFunction(()=>window.sfLocalDecodeStats?.decoded_frames>0&&document.querySelector('#player').currentTime>1,{},{timeout:20000});
      assert.deepEqual(await page.evaluate(()=>window.errors),[]);
      await page.evaluate(()=>{document.querySelector('#player').currentTime=7});
      await page.waitForFunction(()=>document.querySelector('#player').currentTime>7.2,{},{timeout:15000});
      await page.evaluate(async()=>{
        document.querySelector('#player').pause();
        await window.sfLocalOrigin.selectAudio(1);
        await document.querySelector('#player').play();
      });
      await page.waitForTimeout(500);
      await page.evaluate(()=>window.sfLocalOrigin.destroy());
      assert.equal(await page.evaluate(()=>window.sfLocalOrigin.isActive()),false);
      console.log(`Decoded + seek + audio selection + teardown: ${host}/${name}`);
    }
    await page.close();
  }
  assert(rangeRequests>0,'player must request original byte ranges');
  assert.deepEqual(failures,[]);
}finally{
  await browser.close();await new Promise(resolve=>server.close(resolve));fs.rmSync(temp,{recursive:true,force:true});
}
