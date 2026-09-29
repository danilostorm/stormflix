// Original diagnostic ROM: A paints red, Select green, idle blue. No commercial ROMs.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import http from 'node:http';
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url),{chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(import.meta.dirname,'../internal/webui/static'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'sf-input-'));
const runtime=process.env.GAME_TEST_RUNTIME||temp;
if(!process.env.GAME_TEST_RUNTIME){
 execFileSync('curl',['-fsSL','https://cdn.jsdelivr.net/npm/nostalgist@0.21.1/dist/nostalgist.umd.js','-o',path.join(temp,'nostalgist.js')]);
 execFileSync('curl',['-fsSL','https://cdn.jsdelivr.net/gh/arianrhodsandlot/retroarch-emscripten-build@v1.22.2/retroarch/fceumm_libretro.zip','-o',path.join(temp,'core.zip')]);
 execFileSync('unzip',['-q',path.join(temp,'core.zip'),'-d',temp]);
}
const code=[];const labels={},fix=[];const emit=(...bytes)=>code.push(...bytes),label=n=>labels[n]=code.length;
const branch=(op,n)=>{emit(op,0);fix.push([code.length-1,n,false])},jump=n=>{emit(0x4c,0,0);fix.push([code.length-2,n,true])};
emit(0x78,0xd8,0xa2,0xff,0x9a,0xa9,0,0x8d,0,0x20,0x8d,1,0x20);
label('loop');emit(0xa9,1,0x8d,0x16,0x40,0xa9,0,0x8d,0x16,0x40,0xad,0x16,0x40,0x29,1,0x85,0,0xad,0x16,0x40,0xad,0x16,0x40,0x29,1,0x85,1);
label('vblank');emit(0x2c,2,0x20);branch(0x10,'vblank');emit(0xa5,0);branch(0xf0,'select');emit(0xa9,0x16);jump('palette');
label('select');emit(0xa5,1);branch(0xf0,'idle');emit(0xa9,0x1a);jump('palette');label('idle');emit(0xa9,1);
label('palette');emit(0x48,0xa9,0x3f,0x8d,6,0x20,0xa9,0,0x8d,6,0x20,0x68,0x8d,7,0x20,0xa9,0,0x8d,6,0x20,0x8d,6,0x20);jump('loop');
for(const [i,n,absolute] of fix){if(absolute){code[i]=labels[n]&255;code[i+1]=0x80+(labels[n]>>8)}else code[i]=(labels[n]-i-1)&255}
const rom=Buffer.alloc(16+16384+8192);rom.set([78,69,83,26,1,1]);rom.set(code,16);rom.set([0,128,0,128,0,128],16+16384-6);
const game={id:1,title:'Input diagnostic',platform:'nes',core:'fceumm',playable:true,saves:{},rom_name:'input.nes'};
const server=http.createServer((req,res)=>{
 const u=new URL(req.url,'http://localhost'),name=u.pathname;
 if(name==='/'){res.setHeader('Content-Type','text/html');res.end(`<meta name="viewport" content="width=device-width,initial-scale=1"><style>body{margin:0;background:black}.hidden{display:none!important}</style>${['games.css','games-g4.css','games-g4-polish.css','games-g43.css','games-g44.css','games-android.css'].map(x=>`<link rel="stylesheet" href="/${x}">`).join('')}<body><script>window.request=async()=>(${JSON.stringify(game)})</script>${['games-player.js','games-g4-session.js','games-g4.js','games-g4-polish.js','games-g44.js'].map(x=>`<script src="/${x}"></script>`).join('')}`);return}
 if(name==='/api/v1/games/1'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify(game));return}
 if(name.endsWith('/rom')){res.end(rom);return}
 let file=name.endsWith('/nostalgist.js')?path.join(runtime,'nostalgist.js'):name.includes('/runtime/cores/')?path.join(runtime,'fceumm_libretro.'+(name.endsWith('.wasm')?'wasm':'js')):path.join(root,name);
 if(fs.existsSync(file)&&fs.statSync(file).isFile()){res.setHeader('Content-Type',file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'application/wasm');res.end(fs.readFileSync(file));return}
 res.setHeader('Content-Type','application/json');res.end(JSON.stringify({session_id:'test'}));
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_EXECUTABLE||undefined,args:['--no-sandbox','--autoplay-policy=no-user-gesture-required']});
try{
 const page=await browser.newPage({viewport:{width:390,height:844},hasTouch:true});page.on('pageerror',e=>console.error(e.message));page.on('console',m=>{if(m.type()==='error')console.error(m.text())});
 await page.goto(`http://localhost:${server.address().port}`);await page.evaluate(game=>StormFlixGamePlayer.open(game),game);
 await page.waitForFunction(()=>StormFlixGamePlayer.runtime()?.getStatus()==='running',{},{timeout:30000});
 await page.waitForSelector('[data-g44-input="a"]');
 await page.evaluate(()=>{window.inputs=[];window.addEventListener('stormflix:game-input',e=>inputs.push(e.detail))});
 // Read a scaled copy of the emulator canvas on the next animation frame.
 const colour=()=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>{const c=document.querySelector('.game-player-canvas'),o=document.createElement('canvas');o.width=o.height=1;const ctx=o.getContext('2d');ctx.drawImage(c,0,0,1,1);resolve([...ctx.getImageData(0,0,1,1).data].slice(0,3))})));
 const waitColour=async channel=>{for(let i=0;i<50;i++){const c=await colour();if(c[channel]>40&&c[channel]>c[(channel+1)%3]*1.5&&c[channel]>c[(channel+2)%3]*1.5)return;await page.waitForTimeout(100)}console.log(await page.evaluate(()=>({inputs:window.inputs})));await page.screenshot({path:'/tmp/game-input-fail.png'});throw Error('Incorrect input colour: '+JSON.stringify(await colour()))};
 await waitColour(2);
 const a=await page.locator('[data-g44-input="a"]').boundingBox();
 await page.mouse.move(a.x+a.width/2,a.y+a.height/2);await page.mouse.down();await waitColour(0);
 await page.mouse.move(2,2);await page.mouse.up();await waitColour(2);
 assert.equal(await page.locator('[data-g44-input="a"]').evaluate(el=>el.classList.contains('pressed')),false);
 await page.locator('[data-g44-input="select"]').hover();await page.mouse.down();await waitColour(1);await page.mouse.up();await waitColour(2);
 const touch=await page.context().newCDPSession(page),box=await page.locator('[data-g44-input="a"]').boundingBox();
 await touch.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:box.x+box.width/2,y:box.y+box.height/2}]});await waitColour(0);
 await touch.send('Input.dispatchTouchEvent',{type:'touchCancel',touchPoints:[]});await waitColour(2);
 assert.equal(await page.locator('[data-g44-input="a"]').evaluate(el=>el.classList.contains('pressed')),false);
 console.log('Actual NES core: A and Select distinct; pointer release outside and touch cancellation return to idle');
 await page.evaluate(()=>StormFlixGamePlayer.close());
}finally{await browser.close();await new Promise(r=>server.close(r));fs.rmSync(temp,{recursive:true,force:true})}
