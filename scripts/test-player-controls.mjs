// Exercise the real Web page and APK input bridge in Chromium.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import os from 'node:os';
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(import.meta.dirname,'../internal/webui/static');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'stormflix-controls-'));
execFileSync('ffmpeg',['-loglevel','error','-f','lavfi','-i','testsrc2=size=640x360:rate=24','-t','30','-c:v','libvpx','-deadline','realtime',path.join(temp,'sample.webm')]);
const server=http.createServer((req,res)=>{
 const url=new URL(req.url,'http://localhost');
 if(url.pathname==='/captions.vtt'){res.setHeader('Content-Type','text/vtt');res.end('WEBVTT\n\n00:00:00.000 --> 00:00:30.000\nCAPTION MENU TEST\n');return}
 if(url.pathname.startsWith('/api/')){res.setHeader('Content-Type','application/json');if(url.pathname==='/api/v1/setup/status')res.end('{"needs_setup":false}');else{res.statusCode=401;res.end('{"error":"Test session"}')}return}
 const file=url.pathname==='/sample.webm'?path.join(temp,'sample.webm'):path.join(root,url.pathname==='/'?'index.html':url.pathname);
 if(!fs.existsSync(file)||!fs.statSync(file).isFile()){res.writeHead(404);res.end();return}
 const data=fs.readFileSync(file),ext=path.extname(file);res.setHeader('Content-Type',({'.js':'text/javascript','.css':'text/css','.html':'text/html','.webm':'video/webm','.json':'application/json'})[ext]||'application/octet-stream');
 const range=/bytes=(\d+)-(\d*)/.exec(req.headers.range||'');if(range){const a=+range[1],b=Math.min(data.length-1,range[2]?+range[2]:data.length-1);res.writeHead(206,{'Content-Range':`bytes ${a}-${b}/${data.length}`,'Accept-Ranges':'bytes'});res.end(data.subarray(a,b+1))}else res.end(data);
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_EXECUTABLE||undefined,args:['--no-sandbox','--autoplay-policy=no-user-gesture-required']});
try{
 const page=await browser.newPage({viewport:{width:1280,height:800}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(`http://localhost:${server.address().port}`);
 await page.waitForSelector('#sf-simple-speed',{state:'attached'});
 await page.evaluate(async()=>{document.querySelector('#player-modal').classList.remove('hidden');document.querySelector('#sf-v4-title').textContent='StormFlix · Player';const v=document.querySelector('#player');v.src='/sample.webm';await v.play()});
 await page.waitForFunction(()=>document.querySelector('#player').currentTime>.1);
 await page.evaluate(()=>{const t=document.createElement('track');t.kind='subtitles';t.label='Português';t.srclang='pt';t.dataset.subtitleId='10';t.src='/captions.vtt';document.querySelector('#player').appendChild(t)});
 await page.locator('#sf-subtitle').click();await page.locator('[data-caption-id="10"]').click();
 await page.waitForFunction(()=>document.querySelector('track[data-subtitle-id="10"]').track.mode==='showing'&&document.querySelector('track[data-subtitle-id="10"]').track.activeCues?.length>0);
 await page.locator('#sf-subtitle').click();await page.locator('[data-caption-id="0"]').click();
 assert.equal(await page.evaluate(()=>document.querySelector('track[data-subtitle-id="10"]').track.mode),'disabled');
 assert.equal(await page.locator('#sf-settings').isVisible(),false);
 assert.equal(await page.locator('.sf-v4-status').isVisible(),false);
 await page.locator('#sf-simple-speed').click();
 assert.equal(await page.evaluate(()=>document.querySelector('#player').playbackRate),1.25);
 const before=await page.evaluate(()=>document.querySelector('#player').currentTime);
 await page.locator('#sf-v54-screen').click();
 await page.locator('[data-v54-screen="zoom"]').click();
 assert.ok(await page.evaluate(()=>parseFloat(document.querySelector('#player-modal').style.getPropertyValue('--sf-frame-x'))>1));
 assert.ok(await page.evaluate(()=>document.querySelector('#player').currentTime)>=before);
 await page.locator('#sf-v54-screen').click();
 await page.locator('[data-framing="zoom"]').fill('1.5');
 await page.locator('[data-framing="x"]').fill('50');
 assert.notEqual(await page.evaluate(()=>document.querySelector('#player-modal').style.getPropertyValue('--sf-pan-x')),'0px');
 await page.locator('[data-framing-reset]').click();
 await page.locator('[data-v54-close]').click();
 await page.locator('#sf-simple-lock').click();
 await page.waitForTimeout(2900);
 const paused=await page.evaluate(()=>document.querySelector('#player').paused);
 await page.keyboard.press('Space');
 assert.equal(await page.evaluate(()=>document.querySelector('#player').paused),paused);
 await page.locator('#sf-simple-lock').click();
 await page.evaluate(()=>sfShowControls());
 await page.screenshot({path:path.join(temp,'web-player.png')});
 console.log('Player screenshot:',path.join(temp,'web-player.png'));
 await page.setViewportSize({width:740,height:360});
 await page.evaluate(()=>sfShowControls());
 assert.equal(await page.locator('#sf-v54-screen').isVisible(),true);
 await page.screenshot({path:path.join(temp,'mobile-player.png')});
 // Render the actual APK catalog with representative fixture data.
 await page.setViewportSize({width:390,height:844});
 await page.goto(`http://localhost:${server.address().port}/?stormflix_native_games=1`);
 await page.evaluate(async()=>{
   const games=['Aventura na floresta','Corrida espacial','Mundo dos dinossauros','Jornada do herói'].map((title,i)=>({id:i+1,title,platform:'snes',playable:true,play_seconds:3600,last_played_at:'2026-09-29',saves:{},cover_url:'data:image/svg+xml,'+encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="160"><rect width="120" height="160" fill="${['#24765d','#244f92','#a34337','#8b6722'][i]}"/><text x="12" y="85" fill="white" font-size="40">${i+1}</text></svg>`)}));
   window.request=async url=>url==='/games/home'?{continue_playing:games,platforms:[{platform:'snes',count:4}]}:games;
   document.querySelector('#shell').classList.remove('hidden');document.querySelector('#login')?.classList.add('hidden');document.querySelector('#profile-picker')?.classList.add('hidden');
   await sfLoadScreenBundle('games');document.querySelector('#games-nav').click();
 });
 await page.waitForSelector('.g48-continue-card');
 await page.waitForTimeout(750);
 const top=await page.locator('.gx-topbar').boundingBox(),hero=await page.locator('.g48-dashboard>.gx-hero').boundingBox();
 assert(top.y>=0&&top.y<5,'APK navigation must start at the top');assert(hero.y>=top.y+top.height,'navigation must not overlap hero');
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
 await page.screenshot({path:path.join(temp,'games-mobile.png')});console.log('Games screenshot:',path.join(temp,'games-mobile.png'));
 // Verify native input forwarding and pause/save ordering without copyrighted ROMs.
 await page.goto(`http://localhost:${server.address().port}/?stormflix_native_games=1`);
 await page.waitForFunction(()=>!!window.sfAndroidGames);
 await page.evaluate(async()=>{
   await sfLoadScreenBundle('games');
   window.calls=[];let status='running';
   window.StormFlixGamePlayer={active:()=>true,runtime:()=>({getStatus:()=>status}),pressDown:b=>calls.push('down:'+b),pressUp:b=>calls.push('up:'+b),pause:async()=>{status='paused';calls.push('pause')},save:async()=>calls.push('save')};
   sfAndroidGames.key('b',true);sfAndroidGames.key('b',true);sfAndroidGames.key('b',false);
   sfAndroidGames.key('confirm',true);sfAndroidGames.key('confirm',false);
 });
 assert.deepEqual(await page.evaluate(()=>calls),['down:b','up:b','down:a','up:a']);
 await page.evaluate(()=>sfAndroidGames.background());
 assert.deepEqual(await page.evaluate(()=>calls.slice(-2)),['pause','save']);
 assert.deepEqual(errors,[]);
 console.log('Web controls, zoom/pan, lock, mobile layout and APK input/lifecycle passed');
}finally{await browser.close();await new Promise(r=>server.close(r))}
