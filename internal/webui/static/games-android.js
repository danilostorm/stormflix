/* APK Games host: authenticated browser runtime with native remote/gamepad input. */
(()=>{
  const params=new URLSearchParams(location.search);
  if(params.get('stormflix_native_games')!=='1')return;
  document.body.classList.add('sf-android-games');
  const style=document.createElement('style');style.textContent=`
    .sf-android-games.games-mode #shell>.topbar,.sf-android-games.games-mode #hero,.sf-android-games.games-mode #rows{display:none!important}
    .sf-android-games.games-mode #games-view{padding-top:0!important}
    .sf-android-games button:focus,.sf-android-games [tabindex]:focus{outline:3px solid white!important;outline-offset:3px}
    .sf-android-games [data-gx-exit]{display:none!important}`;document.head.appendChild(style);
  const api=()=>window.StormFlixGamePlayer;
  const held=new Set();let booted=false,starting=false,backgroundWork=null,nativeBackground=false;
  function visible(el){return el&&getComputedStyle(el).visibility!=='hidden'&&el.getClientRects().length>0&&!el.closest('.hidden')}
  function menu(){return document.querySelector('.sf-g4-panel:not(.hidden)')}
  function release(){for(const key of held)api()?.pressUp(key);held.clear()}
  function focusScope(){return menu()||document.querySelector('.gx-detail-overlay')||document.querySelector('#game-player-overlay')||document.querySelector('#games-view')||document.body}
  function navigate(direction){
    const scope=focusScope(),items=[...scope.querySelectorAll('button,input,[tabindex="0"]')].filter(el=>visible(el)&&!el.disabled);
    if(!items.length)return;const current=document.activeElement;
    if(!items.includes(current)){items[0].focus();return}
    const r=current.getBoundingClientRect(),cx=r.x+r.width/2,cy=r.y+r.height/2;
    let best=null,score=Infinity;
    for(const item of items){if(item===current)continue;const b=item.getBoundingClientRect(),dx=b.x+b.width/2-cx,dy=b.y+b.height/2-cy;
      const primary=direction==='right'?dx:direction==='left'?-dx:direction==='down'?dy:-dy;
      const cross=direction==='left'||direction==='right'?Math.abs(dy):Math.abs(dx);
      if(primary>2&&primary+cross*3<score){score=primary+cross*3;best=item}}
    best?.focus();best?.scrollIntoView({block:'nearest',inline:'nearest'});
  }
  function key(button,down){
    if(!down){if(held.has(button)){api()?.pressUp(button);held.delete(button)}return}
    if(button==='menu'){release();window.dispatchEvent(new Event('stormflix:game-menu-request'));return}
    if(api()?.active()&&!menu()&&api()?.runtime()?.getStatus?.()==='running'){
      const input=button==='confirm'?'start':button;
      if(!held.has(input)){api().pressDown(input);held.add(input)}return;
    }
    if(['up','down','left','right'].includes(button)){navigate(button);return}
    if(button==='confirm'||button==='b'||button==='start'){if(visible(document.activeElement)&&focusScope().contains(document.activeElement))document.activeElement.click();else navigate('down')}
    if(button==='a')back();
  }
  function back(){
    release();
    if(document.querySelector('#game-player-overlay')){
      if(menu()||!api()?.active()){void api()?.close()}else window.dispatchEvent(new Event('stormflix:game-menu-request'));
      return true;
    }
    const detail=document.querySelector('.gx-detail-overlay');if(detail){detail.remove();return true}
    return false;
  }
  async function background(){
    nativeBackground=true;
    release();if(backgroundWork)return backgroundWork;
    backgroundWork=(async()=>{if(api()?.runtime()?.getStatus?.()==='running')await api().pause();if(api()?.active())await api().save()})();
    try{await backgroundWork}finally{backgroundWork=null}
  }
  window.sfAndroidGames={key:(b,d)=>key(b==='confirm'&&!d?'start':b,d),back,background,foreground:()=>{nativeBackground=false}};
  window.addEventListener('blur',release);
  document.addEventListener('visibilitychange',()=>{if(document.hidden)void background()});
  window.addEventListener('stormflix:game-started',()=>{if(params.get('stormflix_tv')==='1')api()?.patchPreferences({touch:{mode:'off'}});if(document.hidden||nativeBackground)void background()});
  window.addEventListener('stormflix:game-closed',release);
  async function boot(){
    if(booted||starting||!visible(document.querySelector('#shell'))||visible(document.querySelector('#profile-picker'))||typeof window.sfLoadScreenBundle!=='function')return;
    starting=true;
    try{await window.sfLoadScreenBundle('games');document.querySelector('#games-nav')?.click();booted=true;setTimeout(()=>navigate('down'),600)}
    catch(error){console.error('Games APK:',error);window.sfToast?.('Não foi possível carregar Jogos. Volte e tente novamente.')}
    finally{starting=false}
  }
  const bootTimer=setInterval(()=>{void boot();if(booted)clearInterval(bootTimer)},250);
  setTimeout(()=>clearInterval(bootTimer),60000);void boot();
})();
