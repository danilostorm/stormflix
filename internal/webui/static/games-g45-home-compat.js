/* StormFlix Games G4.7: Home watchdog + safe strategic Games placement. */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  let observer=null,repairTimer=0,placementTimer=0,repairing=false,placing=false,repairEpoch=0;

  function homeSelected(){return !!$('[data-nav="home"]')?.classList.contains('active')}
  function gamesOpen(){return document.body.classList.contains('games-mode')&&!$('#games-view')?.classList.contains('hidden')}
  function homeVisible(){return homeSelected()&&!gamesOpen()&&!$('#catalog-view')?.classList.contains('hidden')}
  function gameRows(){return $$('#rows > [data-g44-home-row]')}
  function nativeRows(){return $$('#rows > .content-row:not([data-g44-home-row])')}
  function nativeRow(title){return nativeRows().find(row=>row.querySelector('.row-head h2')?.textContent.trim()===title)||null}

  function scheduleRepair(delay=0){
    clearTimeout(repairTimer);
    repairTimer=setTimeout(restoreNativeHome,delay);
  }
  function schedulePlacement(delay=50){
    clearTimeout(placementTimer);
    placementTimer=setTimeout(placeHomeGameRows,delay);
  }

  async function nextPaint(){
    await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  }

  async function restoreNativeHome(){
    if(!homeSelected()||gamesOpen()||repairing)return;
    if(nativeRows().length){schedulePlacement(40);return}

    const epoch=++repairEpoch;
    repairing=true;
    try{
      /* Critical G4.7 invariant: Games is NEVER allowed to be the only owner
       * of #rows. Remove game rails first, then restore the already-loaded
       * StormFlix feed synchronously through showHome(). */
      gameRows().forEach(node=>node.remove());
      if(typeof window.showHome==='function')window.showHome();
      await nextPaint();
      if(epoch!==repairEpoch||!homeSelected()||gamesOpen())return;

      /* Normally showHome() is enough because app.js keeps the native feed in
       * memory. Only refetch /home if the feed was genuinely unavailable. */
      if(nativeRows().length===0&&typeof window.loadHome==='function'){
        await window.loadHome();
        if(epoch===repairEpoch&&homeSelected()&&!gamesOpen()&&typeof window.showHome==='function')window.showHome();
        await nextPaint();
      }
    }catch(err){
      console.warn('[StormFlix Games G4.7] falha ao restaurar Home nativa',err);
    }finally{
      repairing=false;
      schedulePlacement(100);
    }
  }

  function placeHomeGameRows(){
    if(!homeVisible()||repairing||placing)return;
    const rows=$('#rows');if(!rows)return;
    const natives=nativeRows();
    const games=gameRows();

    /* Never rearrange during the brief renderRows() window where native media
     * is absent. That race was what allowed a game-only Home to appear. */
    if(games.length&&natives.length===0){scheduleRepair(0);return}
    if(natives.length<2||!games.length)return;

    const continued=rows.querySelector(':scope > [data-g44-home-row="continue"]');
    const recent=rows.querySelector(':scope > [data-g44-home-row="recent"]');
    const trendingNow=nativeRow('Em alta agora');
    const trendingWeek=nativeRow('Em alta nesta semana');
    const releases=nativeRow('Lançamentos');
    const firstNative=natives[0]||null;

    placing=true;
    try{
      /* Desktop/mobile strategy:
       * Em alta agora -> Continuar jogando -> Em alta nesta semana ->
       * Jogos adicionados recentemente -> Lançamentos/restante.
       * If a discovery rail is missing, use a conservative nearby fallback. */
      if(continued){
        const anchor=trendingNow||firstNative;
        if(anchor&&anchor.nextElementSibling!==continued)anchor.insertAdjacentElement('afterend',continued);
      }
      if(recent){
        const anchor=trendingWeek||releases||continued||trendingNow||firstNative;
        if(anchor&&anchor.nextElementSibling!==recent)anchor.insertAdjacentElement('afterend',recent);
      }
    }finally{
      placing=false;
    }
  }

  function removeGameRowsOutsideHome(){
    if(homeSelected())return;
    gameRows().forEach(node=>node.remove());
  }

  function onRowsMutation(){
    if(gamesOpen()||repairing||placing)return;
    if(!homeSelected()){removeGameRowsOutsideHome();return}
    const nativeCount=nativeRows().length;
    const gameCount=gameRows().length;
    if(gameCount>0&&nativeCount===0){scheduleRepair(0);return}
    if(nativeCount>0&&gameCount>0)schedulePlacement(45);
  }

  function installObserver(){
    const rows=$('#rows');
    if(!rows||observer)return;
    observer=new MutationObserver(onRowsMutation);
    observer.observe(rows,{childList:true});
  }

  document.addEventListener('click',e=>{
    const nav=e.target.closest?.('[data-nav]');
    if(nav){
      if(nav.dataset.nav==='home'){
        setTimeout(()=>nativeRows().length?schedulePlacement(20):scheduleRepair(0),100);
      }else{
        setTimeout(removeGameRowsOutsideHome,0);
      }
      return;
    }
    if(e.target.closest?.('#brand-home'))setTimeout(()=>nativeRows().length?schedulePlacement(20):scheduleRepair(0),120);
  },true);

  window.addEventListener('stormflix:profile',()=>setTimeout(()=>nativeRows().length?schedulePlacement(30):scheduleRepair(0),180));
  window.addEventListener('stormflix:game-closed',()=>setTimeout(()=>nativeRows().length?schedulePlacement(30):scheduleRepair(0),160));

  function boot(){
    installObserver();
    setTimeout(()=>homeSelected()?(nativeRows().length?schedulePlacement(0):scheduleRepair(0)):removeGameRowsOutsideHome(),650);
    setTimeout(()=>{installObserver();if(homeSelected())nativeRows().length?schedulePlacement(0):scheduleRepair(0)},1500);
  }
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();
