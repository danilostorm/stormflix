/* StormFlix Games instant data cache.
 * Native movie/series/anime Home always gets first paint/network priority.
 * Games is warmed immediately afterwards and reused from memory on navigation.
 */
(function(){
  'use strict';
  if(typeof window.request!=='function'||window.__sfGamesInstantCache)return;
  window.__sfGamesInstantCache=true;

  const baseRequest=window.request;
  const baseFetch=window.fetch.bind(window);
  const cache=new Map();
  const ttl=45000;
  const warmPaths=['/games/home','/games?limit=500'];
  let warmTimer=0,observer=null;

  function nativeRowsReady(){
    if(!document.querySelector('[data-nav="home"]')?.classList.contains('active'))return false;
    if(document.body.classList.contains('games-mode'))return false;
    return document.querySelectorAll('#rows > .content-row:not([data-g44-home-row])').length>=2;
  }

  function isBackgroundGamesHome(input,init={}){
    if(document.body.classList.contains('games-mode'))return false;
    const method=String(init?.method||'GET').toUpperCase();if(method!=='GET')return false;
    const raw=typeof input==='string'?input:String(input?.url||'');
    try{
      const parsed=new URL(raw,location.origin);
      return parsed.origin===location.origin&&parsed.pathname==='/api/v1/games/home';
    }catch{return false}
  }

  function waitForNativeHome(maxWait=3500){
    if(nativeRowsReady())return Promise.resolve();
    return new Promise(resolve=>{
      let done=false;
      const finish=()=>{if(done)return;done=true;clearTimeout(timeout);window.removeEventListener('stormflix:native-home-ready',finish);resolve()};
      const timeout=setTimeout(finish,maxWait);
      window.addEventListener('stormflix:native-home-ready',finish,{once:true});
      const tick=()=>{if(done)return;if(nativeRowsReady())finish();else requestAnimationFrame(tick)};requestAnimationFrame(tick);
    });
  }

  /* games-g44 uses fetch() directly for Home rails. Hold only that background
   * request until native Home has painted, so Games never competes with the
   * first /home SQLite/render path. Explicit Jogos navigation bypasses this. */
  window.fetch=function(input,init){
    if(isBackgroundGamesHome(input,init)&&!nativeRowsReady())return waitForNativeHome().then(()=>baseFetch(input,init));
    return baseFetch(input,init);
  };

  function isCacheable(path,opt={}){
    const method=String(opt.method||'GET').toUpperCase();
    return method==='GET'&&warmPaths.includes(String(path||''));
  }
  function remember(path,value){cache.set(path,{value,at:Date.now(),promise:null});return value}
  function peek(path){const entry=cache.get(path);return entry?.value&&Date.now()-entry.at<ttl?entry.value:null}
  function cachedRequest(path,opt={}){
    if(!isCacheable(path,opt))return baseRequest(path,opt);
    const entry=cache.get(path),now=Date.now();
    if(entry?.value&&now-entry.at<ttl)return Promise.resolve(entry.value);
    if(entry?.promise)return entry.promise;
    const promise=baseRequest(path,opt).then(value=>remember(path,value)).catch(err=>{cache.delete(path);throw err});
    cache.set(path,{value:entry?.value||null,at:entry?.at||0,promise});
    return promise;
  }
  window.request=cachedRequest;

  function warm(){
    clearTimeout(warmTimer);
    if(!nativeRowsReady())return;
    warmTimer=setTimeout(()=>Promise.allSettled(warmPaths.map(path=>cachedRequest(path))),20);
  }
  function invalidate(){cache.clear()}
  function invalidateHome(){cache.delete('/games/home');cache.delete('/games?limit=500')}

  function observe(){
    const rows=document.querySelector('#rows');if(!rows||observer)return;
    observer=new MutationObserver(()=>{if(nativeRowsReady())warm()});
    observer.observe(rows,{childList:true});
    if(nativeRowsReady())warm();
  }

  window.addEventListener('stormflix:native-home-ready',warm);
  window.addEventListener('stormflix:profile',()=>{invalidate();setTimeout(warm,120)});
  window.addEventListener('stormflix:game-closed',()=>{invalidateHome();setTimeout(warm,80)});
  document.addEventListener('visibilitychange',()=>{if(!document.hidden&&nativeRowsReady())warm()});
  window.sfGamesInstantCache={warm,invalidate,peek};

  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',observe,{once:true});else observe();
})();
