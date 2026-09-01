/* StormFlix Games G4.1 presentation layer: cinema shell + auto-hiding chrome. */
(function(){
  let overlay=null,hideTimer=0,abort=null,panelObserver=null;
  const $=(s,r=document)=>r.querySelector(s);

  function running(){return !!overlay&&!$('[data-game-controls]',overlay)?.classList.contains('hidden')}
  function menuOpen(){return !!$('[data-g4-panel]:not(.hidden)',overlay||document)}
  function clearTimer(){if(hideTimer)clearTimeout(hideTimer);hideTimer=0}
  function normalizeLabels(){
    if(!overlay)return;const kicker=$('.game-player-kicker',overlay),menuKicker=$('.sf-g4-card>header small',overlay);
    if(kicker)kicker.textContent='STORMFLIX GAME PLAYER G4.1';if(menuKicker)menuKicker.textContent='STORMFLIX GAME PLAYER G4.1';
  }
  function showChrome(autoHide=true){
    if(!overlay)return;overlay.classList.remove('sf-g41-ui-hidden');clearTimer();
    if(autoHide&&running()&&!menuOpen())hideTimer=setTimeout(()=>{if(overlay&&running()&&!menuOpen())overlay.classList.add('sf-g41-ui-hidden')},2600);
  }
  function keepChrome(){showChrome(false)}
  function install(){
    const next=$('#game-player-overlay');if(!next||next===overlay)return;cleanup();overlay=next;overlay.classList.add('sf-game-g41');normalizeLabels();
    const runtime=$('.game-runtime-badge',overlay);if(runtime)runtime.setAttribute('aria-hidden','true');
    abort=new AbortController();const signal=abort.signal;
    const stage=$('[data-game-stage]',overlay);
    stage?.addEventListener('pointermove',e=>{if(e.pointerType==='mouse'||e.pointerType==='pen')showChrome()},{signal});
    stage?.addEventListener('pointerdown',e=>{if(!e.target.closest?.('[data-g4-touch]'))showChrome()},{signal});
    overlay.addEventListener('focusin',e=>{if(e.target.closest?.('.game-player-top,.game-player-controls,.sf-g4-panel'))keepChrome()},{signal});
    overlay.addEventListener('focusout',()=>showChrome(),{signal});
    $('[data-game-menu]',overlay)?.addEventListener('click',keepChrome,{signal});
    $('[data-game-fullscreen]',overlay)?.addEventListener('click',()=>setTimeout(()=>showChrome(),100),{signal});
    panelObserver=new MutationObserver(()=>{normalizeLabels();if(menuOpen())keepChrome();else showChrome()});
    const panel=$('[data-g4-panel]',overlay);if(panel)panelObserver.observe(panel,{attributes:true,attributeFilter:['class'],childList:true,subtree:false});
    showChrome();
  }
  function cleanup(){clearTimer();abort?.abort();abort=null;panelObserver?.disconnect();panelObserver=null;if(overlay)overlay.classList.remove('sf-g41-ui-hidden','sf-game-g41');overlay=null}

  window.addEventListener('stormflix:game-started',()=>{install();showChrome()});
  window.addEventListener('stormflix:game-menu-request',()=>{setTimeout(normalizeLabels,0);keepChrome()});
  window.addEventListener('stormflix:game-closed',cleanup);
  document.addEventListener('fullscreenchange',()=>setTimeout(showChrome,80));
  const observer=new MutationObserver(()=>{if($('#game-player-overlay')&&!overlay)install()});observer.observe(document.documentElement,{childList:true,subtree:true});
})();
