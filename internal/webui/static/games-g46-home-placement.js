/* StormFlix Games G4.6: balance Games with the native Home discovery rails. */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  let observer=null,timer=0,placing=false;

  function homeActive(){
    return $('[data-nav="home"]')?.classList.contains('active') &&
      !document.body.classList.contains('games-mode') &&
      !$('#catalog-view')?.classList.contains('hidden');
  }
  function nativeRow(title){
    const rows=$('#rows');if(!rows)return null;
    return [...rows.querySelectorAll('.content-row:not([data-g44-home-row])')]
      .find(row=>row.querySelector('.row-head h2')?.textContent.trim()===title)||null;
  }
  function schedule(delay=50){clearTimeout(timer);timer=setTimeout(placeRails,delay)}

  function placeRails(){
    if(!homeActive()||placing)return;
    const rows=$('#rows');if(!rows)return;
    const continued=rows.querySelector('[data-g44-home-row="continue"]');
    const recent=rows.querySelector('[data-g44-home-row="recent"]');
    if(!continued&&!recent)return;

    /* Home strategy:
     *   Em alta agora
     *   Continuar jogando
     *   Em alta nesta semana
     *   Jogos adicionados recentemente
     *   Lançamentos / remaining native catalog...
     *
     * This keeps Games visible near the top without allowing two game rails to
     * push the native movie/series/anime discovery rows down on small screens. */
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

  function installObserver(){
    const rows=$('#rows');if(!rows||observer)return;
    observer=new MutationObserver(()=>{if(!placing)schedule(35)});
    observer.observe(rows,{childList:true});
  }

  document.addEventListener('click',e=>{
    if(e.target.closest?.('[data-nav="home"],#brand-home'))schedule(180);
  },true);
  window.addEventListener('stormflix:profile',()=>schedule(220));
  window.addEventListener('stormflix:game-closed',()=>schedule(160));

  function boot(){installObserver();schedule(650);setTimeout(()=>{installObserver();schedule(0)},1500)}
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();
