// Exercise the shipped rail renderer, including scroll and keyboard paging.
import assert from 'node:assert/strict';
import path from 'node:path';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_EXECUTABLE||undefined,args:['--no-sandbox']});
try {
 const page=await browser.newPage({viewport:{width:1280,height:800}});
 await page.setContent('<style>.row-track{display:flex;overflow:auto;gap:12px}.media-tile{flex:0 0 180px;height:240px}</style><main id="rows"></main>');
 await page.evaluate(()=>{
  window.$=s=>document.querySelector(s);
  window.renderRows=()=>{};
  window.escapeHTML=x=>String(x);
  window.cardHTML=item=>`<button class="media-tile" data-media="${item.id}">${item.title}</button>`;
  window.bindCards=()=>{};
 });
 await page.addScriptTag({path:path.resolve('internal/webui/static/catalog-performance.js')});
 await page.addScriptTag({path:path.resolve('internal/webui/static/rail-nav-v2.js')});
 const render=async count=>page.evaluate(count=>renderRows([{id:'test',title:'Ação',items:Array.from({length:count},(_,i)=>({id:i+1,title:'Filme '+i}))}]),count);
 await render(41);
 assert.equal(await page.locator('.catalog-load-more').count(),0);
 assert.equal(await page.locator('.media-tile').count(),12);
 for(const expected of [24,36,41]) {
  await page.locator('.row-track').evaluate(track=>{track.scrollLeft=track.scrollWidth});
  await page.waitForFunction(n=>document.querySelectorAll('.media-tile').length===n,expected);
 }
 assert.equal(await page.evaluate(()=>new Set([...document.querySelectorAll('.media-tile')].map(x=>x.dataset.media)).size),41);
 await render(30);
 await page.locator('.media-tile').nth(10).focus();
 await page.waitForFunction(()=>document.querySelectorAll('.media-tile').length===24);
 // Very wide displays must not strand cards beyond the initial chunk.
 await page.setViewportSize({width:4000,height:800});
 await render(30);
 await page.waitForFunction(()=>document.querySelectorAll('.media-tile').length===24);
 await page.setViewportSize({width:390,height:844});
 await render(15);
 await page.locator('.row-track').evaluate(track=>{track.scrollLeft=track.scrollWidth});
 await page.waitForFunction(()=>document.querySelectorAll('.media-tile').length===15);
 console.log('Catalog scroll, keyboard, wide-screen and mobile pagination passed');
} finally { await browser.close() }
