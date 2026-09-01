/* StormFlix Games G4.3 presentation layer: responsive cinema shell + reliable auto-hide chrome. */
(function(){
  let overlay=null,hideTimer=0,abort=null,panelObserver=null;
  const $=(s,r=document)=>r.querySelector(s);

  function running(){return !!overlay&&!$('[data-game-controls]',overlay)?.classList.contains('hidden')}
  function menuOpen(){return !!$('[data-g4-panel]:not(.hidden)',overlay||document)}
  function clearTimer(){if(hideTimer)clearTimeout(hideTimer);hideTimer=0}
  function normalizeLabels(){
    if(!overlay)return;const kicker=$('.game-player-kicker',overlay),menuKicker=$('.sf-g4-card>header small',overlay);
    if(kicker)kicker.textContent='STORMFLIX GAME PLAYER G4.3';if(menuKicker)menuKicker.textContent='STORMFLIX GAME PLAYER G4.3';
  }
  function hideChrome(){if(overlay&&running()&&!menuOpen()){overlay.classList.add('sf-g41-ui-hidden');window.StormFlixGamePlayer?.resize?.()}}
  function showChrome(autoHide=true){
    if(!overlay)return;overlay.classList.remove('sf-g41-ui-hidden');clearTimer();window.StormFlixGamePlayer?.resize?.();
    if(autoHide&&running()&&!menuOpen())hideTimer=setTimeout(hideChrome,2200);
  }
  function keepChrome(){showChrome(false)}
  function install(){
    const next=$('#game-player-overlay');if(!next||next===overlay)return;cleanup();overlay=next;overlay.classList.add('sf-game-g41');normalizeLabels();
    const runtime=$('.game-runtime-badge',overlay);if(runtime)runtime.setAttribute('aria-hidden','true');
    abort=new AbortController();const signal=abort.signal;const stage=$('[data-game-stage]',overlay);
    stage?.addEventListener('pointermove',e=>{if(e.pointerType==='mouse'||e.pointerType==='pen')showChrome(true)},{signal});
    stage?.addEventListener('pointerdown',e=>{if(!e.target.closest?.('[data-g4-touch]'))showChrome(true)},{signal});
    overlay.addEventListener('keydown',()=>{if(!menuOpen())showChrome(true)},{capture:true,signal});
    overlay.addEventListener('focusin',e=>{if(e.target.closest?.('.sf-g4-panel'))keepChrome();else if(e.target.closest?.('.game-player-top,.game-player-controls'))showChrome(true)},{signal});
    overlay.addEventListener('focusout',()=>{if(!menuOpen())showChrome(true)},{signal});
    $('[data-game-menu]',overlay)?.addEventListener('click',keepChrome,{signal});
    $('[data-game-fullscreen]',overlay)?.addEventListener('click',()=>setTimeout(()=>showChrome(true),100),{signal});
    $('.game-player-top',overlay)?.addEventListener('pointerleave',()=>showChrome(true),{signal});
    $('.game-player-controls',overlay)?.addEventListener('pointerleave',()=>showChrome(true),{signal});
    panelObserver=new MutationObserver(()=>{normalizeLabels();if(menuOpen())keepChrome();else showChrome(true)});
    const panel=$('[data-g4-panel]',overlay);if(panel)panelObserver.observe(panel,{attributes:true,attributeFilter:['class'],childList:true,subtree:false});
    showChrome(true);
  }
  function cleanup(){clearTimer();abort?.abort();abort=null;panelObserver?.disconnect();panelObserver=null;if(overlay)overlay.classList.remove('sf-g41-ui-hidden','sf-game-g41');overlay=null}

  window.addEventListener('stormflix:game-started',()=>{install();showChrome(true)});
  window.addEventListener('stormflix:game-menu-request',()=>{setTimeout(normalizeLabels,0);keepChrome()});
  window.addEventListener('stormflix:game-closed',cleanup);
  document.addEventListener('fullscreenchange',()=>setTimeout(()=>showChrome(true),80));
  const observer=new MutationObserver(()=>{if($('#game-player-overlay')&&!overlay)install()});observer.observe(document.documentElement,{childList:true,subtree:true});
})();
