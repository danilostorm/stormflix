/* StormFlix Games G4.10: zero-flash Home gate + safe strategic Games placement. */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  let observer=null,repairTimer=0,placementTimer=0,repairing=false,placing=false,repairEpoch=0,readyAnnounced=false;

  const gate=document.createElement('style');
  gate.textContent=`body:not(.sf-native-home-ready) #rows > [data-g44-home-row]{display:none!important}`;
  document.head.appendChild(gate);

  function homeSelected(){return !!$('[data-nav="home"]')?.classList.contains('active')}
  function gamesOpen(){return document.body.classList.contains('games-mode')&&!$('#games-view')?.classList.contains('hidden')}
  function homeVisible(){return homeSelected()&&!gamesOpen()&&!$('#catalog-view')?.classList.contains('hidden')}
  function gameRows(){return $$('#rows > [data-g44-home-row]')}
  function nativeRows(){return $$('#rows > .content-row:not([data-g44-home-row])')}
  function nativeRow(title){return nativeRows().find(row=>row.querySelector('.row-head h2')?.textContent.trim()===title)||null}

  function syncPaintGate(){
    const ready=homeSelected()&&!gamesOpen()&&nativeRows().length>=2;
    document.body.classList.toggle('sf-native-home-ready',ready);
    if(ready&&!readyAnnounced){
      readyAnnounced=true;
      window.dispatchEvent(new CustomEvent('stormflix:native-home-ready'));
      window.sfGamesInstantCache?.warm?.();
    }
    if(!ready)readyAnnounced=false;
    return ready;
  }

  function scheduleRepair(delay=0){clearTimeout(repairTimer);repairTimer=setTimeout(restoreNativeHome,delay)}
  function schedulePlacement(delay=50){clearTimeout(placementTimer);placementTimer=setTimeout(placeHomeGameRows,delay)}
  async function nextPaint(){await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))}

  async function restoreNativeHome(){
    syncPaintGate();
    if(!homeSelected()||gamesOpen()||repairing)return;
    if(nativeRows().length>=2){syncPaintGate();schedulePlacement(20);return}

    const epoch=++repairEpoch;
    repairing=true;
    try{
      /* Games can be fetched early, but they are never allowed to paint before
       * the native movie/series/anime Home. Remove temporary rails, restore the
       * in-memory native feed synchronously, then allow Games after two rails. */
      gameRows().forEach(node=>node.remove());
      syncPaintGate();
      if(typeof window.showHome==='function')window.showHome();
      await nextPaint();
      if(epoch!==repairEpoch||!homeSelected()||gamesOpen())return;

      if(nativeRows().length<2&&typeof window.loadHome==='function'){
        await window.loadHome();
        if(epoch===repairEpoch&&homeSelected()&&!gamesOpen()&&typeof window.showHome==='function')window.showHome();
        await nextPaint();
      }
    }catch(err){
      console.warn('[StormFlix Games G4.10] falha ao restaurar Home nativa',err);
    }finally{
      repairing=false;
      if(syncPaintGate())schedulePlacement(25);
    }
  }

  function placeHomeGameRows(){
    if(!homeVisible()||repairing||placing)return;
    const rows=$('#rows');if(!rows)return;
    const natives=nativeRows(),games=gameRows();

    if(natives.length<2){syncPaintGate();if(games.length)scheduleRepair(0);return}
    syncPaintGate();
    if(!games.length)return;

    const continued=rows.querySelector(':scope > [data-g44-home-row="continue"]');
    const recent=rows.querySelector(':scope > [data-g44-home-row="recent"]');
    const trendingNow=nativeRow('Em alta agora');
    const trendingWeek=nativeRow('Em alta nesta semana');
    const releases=nativeRow('Lançamentos');
    const firstNative=natives[0]||null;

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
    }finally{placing=false;syncPaintGate()}
  }

  function removeGameRowsOutsideHome(){
    if(homeSelected())return;
    gameRows().forEach(node=>node.remove());
    document.body.classList.remove('sf-native-home-ready');
  }

  function onRowsMutation(){
    if(gamesOpen()||repairing||placing){syncPaintGate();return}
    if(!homeSelected()){removeGameRowsOutsideHome();return}
    const nativeCount=nativeRows().length,gameCount=gameRows().length;

    /* MutationObserver runs before browser paint. Toggling this class here
     * guarantees a fast Games response cannot flash on screen by itself. */
    syncPaintGate();
    if(gameCount>0&&nativeCount<2){scheduleRepair(0);return}
    if(nativeCount>=2&&gameCount>0)schedulePlacement(20);
  }

  function installObserver(){
    const rows=$('#rows');if(!rows||observer)return;
    observer=new MutationObserver(onRowsMutation);
    observer.observe(rows,{childList:true});
  }

  document.addEventListener('click',e=>{
    const nav=e.target.closest?.('[data-nav]');
    if(nav){
      if(nav.dataset.nav==='home')setTimeout(()=>syncPaintGate()?schedulePlacement(10):scheduleRepair(0),60);
      else setTimeout(removeGameRowsOutsideHome,0);
      return;
    }
    if(e.target.closest?.('#brand-home'))setTimeout(()=>syncPaintGate()?schedulePlacement(10):scheduleRepair(0),70);
  },true);

  window.addEventListener('stormflix:profile',()=>setTimeout(()=>syncPaintGate()?schedulePlacement(15):scheduleRepair(0),100));
  window.addEventListener('stormflix:game-closed',()=>setTimeout(()=>syncPaintGate()?schedulePlacement(15):scheduleRepair(0),90));

  function boot(){
    installObserver();syncPaintGate();
    setTimeout(()=>homeSelected()?(syncPaintGate()?schedulePlacement(0):scheduleRepair(0)):removeGameRowsOutsideHome(),180);
    setTimeout(()=>{installObserver();if(homeSelected())syncPaintGate()?schedulePlacement(0):scheduleRepair(0)},700);
  }
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();
