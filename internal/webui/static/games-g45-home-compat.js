/* StormFlix Games G4.6: keep Games additive and place game rails strategically on Home. */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  let repairing=false,lastRepairAt=0,observer=null,repairTimer=0,placementTimer=0,placing=false;
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
  function homeActive(){return activeMode()==='home'&&!gamesOpen()&&!$('#catalog-view')?.classList.contains('hidden')}
  function nativeRow(title){
    const rows=$('#rows');if(!rows)return null;
    return [...rows.querySelectorAll('.content-row:not([data-g44-home-row])')]
      .find(row=>row.querySelector('.row-head h2')?.textContent.trim()===title)||null;
  }

  async function repairNativeView(force=false){
    const mode=activeMode();
    if(!nativeModes.has(mode)||gamesOpen()||repairing)return;
    const rows=$('#rows');if(!rows)return;
    if(!force&&nativeRows()>0){schedulePlacement(40);return}
    const now=Date.now();if(!force&&now-lastRepairAt<2500)return;
    lastRepairAt=now;repairing=true;
    try{
      /* Games may append rails, but it must never become the owner of #rows.
       * Reload the canonical /home feed through app.js first when necessary. */
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
    }catch(err){console.warn('[StormFlix Games G4.6] falha ao restaurar catálogo nativo',err)}
    finally{repairing=false;schedulePlacement(90)}
  }

  function scheduleRepair(delay=120,force=false){clearTimeout(repairTimer);repairTimer=setTimeout(()=>repairNativeView(force),delay)}
  function schedulePlacement(delay=50){clearTimeout(placementTimer);placementTimer=setTimeout(placeHomeGameRows,delay)}
  function removeHomeGameRowsOutsideHome(){if(activeMode()==='home')return;$('#rows')?.querySelectorAll('[data-g44-home-row]').forEach(n=>n.remove())}

  function placeHomeGameRows(){
    if(!homeActive()||repairing||placing)return;
    const rows=$('#rows');if(!rows)return;
    const continued=rows.querySelector('[data-g44-home-row="continue"]');
    const recent=rows.querySelector('[data-g44-home-row="recent"]');
    if(!continued&&!recent)return;

    /* Strategic Home order:
     *   Em alta agora
     *   Continuar jogando
     *   Em alta nesta semana
     *   Jogos adicionados recentemente
     *   Lançamentos / remaining native catalog...
     *
     * Alternating native media and Games keeps Games visible near the top while
     * avoiding two consecutive game rails pushing movies/series/anime down,
     * especially on mobile. */
    const trendingNow=nativeRow('Em alta agora');
    const trendingWeek=nativeRow('Em alta nesta semana');
    const releases=nativeRow('Lançamentos');
    const firstNative=rows.querySelector('.content-row:not([data-g44-home-row])');

    placing=true;
    try{
      if(continued){
        const anchor=trendingNow||firstNative;
        if(anchor&&anchor.nextElementSibling!==continued)anchor.insertAdjacentElement('afterend',continued);
      }
      if(recent){
        const anchor=trendingWeek||releases||continued||trendingNow||firstNative;
        if(anchor&&anchor.nextElementSibling!==recent)anchor.insertAdjacentElement('afterend',recent);
      }
    }finally{placing=false}
  }

  document.addEventListener('click',e=>{
    const nav=e.target.closest?.('[data-nav]');
    if(nav){setTimeout(removeHomeGameRowsOutsideHome,0);scheduleRepair(160,true);if(nav.dataset.nav==='home')schedulePlacement(280);return}
    if(e.target.closest?.('#brand-home')){scheduleRepair(180,true);schedulePlacement(300)}
  });
  window.addEventListener('stormflix:profile',()=>{scheduleRepair(220,true);schedulePlacement(360)});
  window.addEventListener('stormflix:game-closed',()=>{scheduleRepair(180,false);schedulePlacement(280)});

  function installObserver(){
    const rows=$('#rows');if(!rows||observer)return;
    observer=new MutationObserver(()=>{
      const mode=activeMode();if(!nativeModes.has(mode)||gamesOpen()||repairing||placing)return;
      /* If Games rails are the only surviving rows while the media feed exists,
       * restore the native StormFlix rows. Otherwise only rebalance their order. */
      if(nativeRows()===0&&(mediaCount()>0||rows.querySelector('[data-g44-home-row]')))scheduleRepair(90,false);
      else if(mode==='home'&&rows.querySelector('[data-g44-home-row]'))schedulePlacement(35);
    });
    observer.observe(rows,{childList:true});
  }

  function boot(){installObserver();scheduleRepair(700,false);schedulePlacement(850);setTimeout(()=>{installObserver();scheduleRepair(0,false);schedulePlacement(120)},1600)}
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();
