/* StormFlix Games instant data cache.
 * Warms Games only after the native movie/series Home has painted, so Games
 * never wins the first-render race and opening Jogos can resolve from memory.
 */
(function(){
  'use strict';
  if(typeof window.request!=='function'||window.__sfGamesInstantCache)return;
  window.__sfGamesInstantCache=true;

  const base=window.request;
  const cache=new Map();
  const ttl=45000;
  const warmPaths=['/games/home','/games?limit=500'];
  let warmTimer=0,observer=null;

  function isCacheable(path,opt={}){
    const method=String(opt.method||'GET').toUpperCase();
    return method==='GET'&&warmPaths.includes(String(path||''));
  }
  function remember(path,value){cache.set(path,{value,at:Date.now(),promise:null});return value}
  function cachedRequest(path,opt={}){
    if(!isCacheable(path,opt))return base(path,opt);
    const entry=cache.get(path),now=Date.now();
    if(entry?.value&&now-entry.at<ttl)return Promise.resolve(entry.value);
    if(entry?.promise)return entry.promise;
    const promise=base(path,opt).then(value=>remember(path,value)).catch(err=>{cache.delete(path);throw err});
    cache.set(path,{value:entry?.value||null,at:entry?.at||0,promise});
    return promise;
  }
  window.request=cachedRequest;

  function nativeRowsReady(){
    if(!document.querySelector('[data-nav="home"]')?.classList.contains('active'))return false;
    if(document.body.classList.contains('games-mode'))return false;
    return document.querySelectorAll('#rows > .content-row:not([data-g44-home-row])').length>=2;
  }
  function warm(){
    clearTimeout(warmTimer);
    if(!nativeRowsReady())return;
    warmTimer=setTimeout(()=>Promise.allSettled(warmPaths.map(path=>cachedRequest(path))),30);
  }
  function invalidate(){cache.clear()}
  function invalidateHome(){cache.delete('/games/home');cache.delete('/games?limit=500')}

  function observe(){
    const rows=document.querySelector('#rows');if(!rows||observer)return;
    observer=new MutationObserver(()=>{if(nativeRowsReady())warm()});
    observer.observe(rows,{childList:true});
    if(nativeRowsReady())warm();
  }

  window.addEventListener('stormflix:profile',()=>{invalidate();setTimeout(warm,180)});
  window.addEventListener('stormflix:game-closed',()=>{invalidateHome();setTimeout(warm,120)});
  document.addEventListener('visibilitychange',()=>{if(!document.hidden&&nativeRowsReady())warm()});
  window.sfGamesInstantCache={warm,invalidate};

  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',observe,{once:true});else observe();
})();
