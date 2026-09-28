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
  if(url.pathname==='/assets/poster.png'){
    if(url.searchParams.has('w')){res.writeHead(404);res.end();return}
    res.setHeader('Content-Type','image/png');
    res.end(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC','base64'));return;
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
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_EXECUTABLE||undefined,headless:true,args:['--no-sandbox','--autoplay-policy=no-user-gesture-required','--host-resolver-rules=MAP stormflix.test 127.0.0.1','--no-proxy-server']});
try{
  for(const host of (process.env.DECODER_TEST_HOSTS||'localhost,stormflix.test').split(',')){
    const page=await browser.newPage();
    page.on('pageerror',e=>{failures.push(String(e));console.error('page error',String(e))});
    page.on('console',msg=>{if(msg.type()==='error'||process.env.DECODER_DEBUG)console.error(msg.text())});
    await page.goto(`http://${host}:${server.address().port}`);
    assert.equal(await page.evaluate(()=>isSecureContext),host==='localhost');
    const app=fs.readFileSync(path.join(root,'app.js'),'utf8');
    await page.addScriptTag({content:app.slice(app.indexOf('function responsiveImageURL('),app.indexOf('function renderHero('))});
    await page.evaluate(()=>{
      const img=document.createElement('img');img.id='cover-test';
      img.src='/assets/poster.png?w=360&format=webp';
      img.srcset='/assets/poster.png?w=240&format=webp 240w, /assets/poster.png?w=500&format=webp 500w';
      document.body.appendChild(img);
    });
    await page.waitForFunction(()=>document.querySelector('#cover-test').naturalWidth>0,{},{timeout:5000}).catch(async error=>{console.error(await page.locator('#cover-test').evaluate(img=>({src:img.src,currentSrc:img.currentSrc,srcset:img.srcset,retry:img.dataset.originalRetry,w:img.naturalWidth,h:img.naturalHeight,complete:img.complete})));throw error});
    assert.equal(await page.locator('#cover-test').getAttribute('srcset'),null);
    assert.equal(await page.locator('#cover-test').getAttribute('src'),'/assets/poster.png');

    for(const name of ['sample.mp4','sample.mkv']){
      await page.evaluate(async name=>{
        window.errors=[];document.querySelector('#player').addEventListener('error',()=>window.errors.push(window.sfPlaybackLastError));
        await Promise.race([window.sfLocalOrigin.load('/'+name,{media_id:1,source_container:name.split('.').pop(),audio_stream:2},{autoplay:true,resume:3}),new Promise((_,reject)=>setTimeout(()=>reject(new Error('Decoder startup timeout: '+JSON.stringify(window.sfLocalDecodeStats))),30000))]);
      },name);
      await page.waitForFunction(()=>window.sfLocalDecodeStats?.decoded_frames>0&&window.sfLocalDecodeStats?.decoded_audio_frames>0&&window.sfLocalDecodeStats?.audio_stream===2&&document.querySelector('#player').currentTime>3.1,{},{timeout:20000});
      assert.deepEqual(await page.evaluate(()=>window.errors),[]);
      await page.evaluate(()=>{document.querySelector('#player').currentTime=7});
      await page.waitForFunction(()=>document.querySelector('#player').currentTime>7.2,{},{timeout:15000});
      const pausedAt=await page.evaluate(()=>document.querySelector('#player').currentTime);
      await page.evaluate(async()=>{
        document.querySelector('#player').pause();
        await window.sfLocalOrigin.selectAudio(1);
        await document.querySelector('#player').play();
      });
      await page.waitForFunction(time=>window.sfLocalDecodeStats?.audio_stream===1&&document.querySelector('#player').currentTime>time+.25,pausedAt,{timeout:5000}).catch(async error=>{console.error('resume failed',await page.evaluate(()=>({stats:window.sfLocalDecodeStats,time:document.querySelector('#player').currentTime,paused:document.querySelector('#player').paused,engines:window.AVPlayer?.Instances?.map(e=>({status:e.getStatus(),time:String(e.currentTime),audio:e.getSelectedAudioStreamId()}))})));throw error});
      assert.deepEqual(await page.evaluate(()=>window.errors),[]);
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
