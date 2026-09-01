/* StormFlix Games G4.5: keep Games additive to the native Home/media catalog. */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  let repairing=false,lastRepairAt=0,observer=null,timer=0;
  const nativeModes=new Set(['home','movie','series','anime']);

  function activeMode(){return $('[data-nav].active')?.dataset.nav||'home'}
  function gamesOpen(){return document.body.classList.contains('games-mode')&&!$('#games-view')?.classList.contains('hidden')}
  function nativeRows(){return $('#rows')?.querySelectorAll('.content-row:not([data-g44-home-row])').length||0}
  function mediaCount(){try{return typeof window.allFeedItems==='function'?window.allFeedItems().length:0}catch{return 0}}
  function modeItems(mode){
    const items=typeof window.allFeedItems==='function'?window.allFeedItems():[];
    if(mode==='movie')return items.filter(item=>item.media_type==='movie');
    if(mode==='anime')return items.filter(item=>item.media_type==='anime');
    if(mode==='series')return items.filter(item=>item.media_type==='series');
    return items;
  }
  function rowTitle(mode){return {movie:'Filmes',series:'Séries',anime:'Animes'}[mode]||'Início'}

  async function repairNativeView(force=false){
    const mode=activeMode();
    if(!nativeModes.has(mode)||gamesOpen()||repairing)return;
    const rows=$('#rows');if(!rows)return;
    if(!force&&nativeRows()>0)return;
    const now=Date.now();if(!force&&now-lastRepairAt<2500)return;
    lastRepairAt=now;repairing=true;
    try{
      /* G4.4 may legitimately append game rails, but it must never become the
       * owner of #rows. Reload the canonical /home feed through app.js first. */
      if(typeof window.loadHome==='function')await window.loadHome();
      if(mode==='home'){
        if(typeof window.showHome==='function')window.showHome();
      }else{
        const items=modeItems(mode);
        $('#hero')?.classList.add('hidden');
        $('#search-view')?.classList.add('hidden');
        $('#catalog-view')?.classList.remove('hidden');
        if(typeof window.renderRows==='function')window.renderRows([{id:mode,title:rowTitle(mode),items}]);
        document.querySelectorAll('[data-nav]').forEach(b=>b.classList.toggle('active',b.dataset.nav===mode));
      }
    }catch(err){console.warn('[StormFlix Games G4.5] falha ao restaurar catálogo nativo',err)}
    finally{repairing=false}
  }

  function scheduleRepair(delay=120,force=false){clearTimeout(timer);timer=setTimeout(()=>repairNativeView(force),delay)}
  function removeHomeGameRowsOutsideHome(){if(activeMode()==='home')return;$('#rows')?.querySelectorAll('[data-g44-home-row]').forEach(n=>n.remove())}

  document.addEventListener('click',e=>{
    const nav=e.target.closest?.('[data-nav]');
    if(nav){setTimeout(removeHomeGameRowsOutsideHome,0);scheduleRepair(160,true);return}
    if(e.target.closest?.('#brand-home'))scheduleRepair(180,true);
  });
  window.addEventListener('stormflix:profile',()=>scheduleRepair(220,true));
  window.addEventListener('stormflix:game-closed',()=>scheduleRepair(180,false));

  function installObserver(){
    const rows=$('#rows');if(!rows||observer)return;
    observer=new MutationObserver(()=>{
      const mode=activeMode();if(!nativeModes.has(mode)||gamesOpen()||repairing)return;
      /* If Games rails are the only surviving rows while the media feed exists,
       * restore the native StormFlix rows and let G4.4 append Games afterward. */
      if(nativeRows()===0&&(mediaCount()>0||rows.querySelector('[data-g44-home-row]')))scheduleRepair(90,false);
    });
    observer.observe(rows,{childList:true});
  }

  function boot(){installObserver();scheduleRepair(700,false);setTimeout(()=>{installObserver();scheduleRepair(0,false)},1600)}
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();
