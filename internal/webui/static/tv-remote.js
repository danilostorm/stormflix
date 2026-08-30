/* StormFlix TV Remote v1 — Jellyfin-style command abstraction for Android TV,
   Fire TV, Samsung Tizen and LG webOS.

   Physical remote keys are translated to semantic commands first. The player
   never receives D-pad Up/Down as native <video> keyboard events, preventing
   browsers/WebViews from interpreting those keys as volume changes. */
(function(){
  'use strict';

  const modal=document.querySelector('#player-modal');
  const video=document.querySelector('#player');
  if(!modal||!video||window.sfTvRemote)return;

  const params=new URLSearchParams(location.search);
  const ua=String(navigator.userAgent||'');
  const nativePlayer=params.get('stormflix_native_player')==='1';
  const androidTv=params.get('stormflix_tv')==='1';
  const tizen=Boolean(window.tizen)||/Tizen/i.test(ua);
  const webos=Boolean(window.PalmSystem)||/Web0S|webOS|NetCast/i.test(ua);
  const browserTv=tizen||webos||/SMART-TV|SmartTV|HbbTV/i.test(ua);
  const tvMode=androidTv||browserTv;

  // Same important Tizen/webOS numeric mappings used by Jellyfin Web.
  const keyCodes={
    13:'select',27:'back',37:'left',38:'up',39:'right',40:'down',
    19:'pause',
    412:'rewind',413:'stop',415:'play',417:'fastforward',
    461:'back',                  // webOS Back
    10009:'back',                // Tizen Return
    10232:'previoustrack',       // Tizen Previous
    10233:'nexttrack',           // Tizen Next
    10252:'playpause'            // Tizen Play/Pause
  };

  const keyNames={
    ArrowLeft:'left',ArrowRight:'right',ArrowUp:'up',ArrowDown:'down',
    Enter:'select',Select:'select',Escape:'back',Back:'back',GoBack:'back',
    MediaPlay:'play',MediaPause:'pause',MediaPlayPause:'playpause',
    MediaRewind:'rewind',MediaFastForward:'fastforward',MediaStop:'stop',
    MediaTrackPrevious:'previoustrack',MediaTrackNext:'nexttrack',
    ContextMenu:'menu',Menu:'menu'
  };

  let hideTimer=0;
  let lastCommandAt=0;

  function playerOpen(){return !modal.classList.contains('hidden')}
  function visible(el){
    if(!el||el.disabled)return false;
    if(el.classList?.contains('hidden'))return false;
    const style=getComputedStyle(el);
    return style.display!=='none'&&style.visibility!=='hidden'&&Number(style.opacity)!==0&&el.getClientRects().length>0;
  }
  function controlsHidden(){return modal.classList.contains('sf-controls-hidden')}
  function menuElements(){
    return [
      '#sf-player-settings-panel','#sf-v5-quality-menu','#sf-v5-diagnostics',
      '#sf-v53-audio-menu','#sf-v54-screen-menu'
    ].map(selector=>document.querySelector(selector)).filter(visible);
  }
  function menuOpen(){return menuElements().length>0}

  function installTvStyle(){
    if(!tvMode||document.querySelector('#sf-tv-remote-style'))return;
    modal.classList.add('sf-tv-remote');
    const style=document.createElement('style');
    style.id='sf-tv-remote-style';
    style.textContent=`
      .sf-tv-remote button:focus,.sf-tv-remote [role="button"]:focus,.sf-tv-remote .sf-setting-option:focus,
      .sf-tv-remote #sf-progress:focus{outline:3px solid rgba(255,255,255,.96)!important;outline-offset:3px!important;box-shadow:0 0 0 6px rgba(255,54,95,.28)!important}
      .sf-tv-remote .sf-setting-option:focus{background:rgba(255,255,255,.16)!important}
      .sf-tv-remote #sf-volume{pointer-events:none!important}
      .sf-tv-remote.sf-controls-hidden{cursor:none}
    `;
    document.head.appendChild(style);
  }

  function resetHideTimer(){
    clearTimeout(hideTimer);
    hideTimer=setTimeout(()=>{
      if(!playerOpen()||menuOpen()||video.paused)return;
      const active=document.activeElement;
      if(active&&modal.contains(active)&&active!==video)active.blur?.();
      modal.classList.add('sf-controls-hidden');
    },6500);
  }

  function showControls(){
    if(!playerOpen())return;
    modal.classList.remove('sf-controls-hidden');
    resetHideTimer();
  }

  function hideControls(){
    clearTimeout(hideTimer);
    if(menuOpen())return;
    const active=document.activeElement;
    if(active&&modal.contains(active)&&active!==video)active.blur?.();
    modal.classList.add('sf-controls-hidden');
  }

  function makeFocusable(el){
    if(!el)return el;
    if(el.matches?.('.sf-setting-option')&&!el.hasAttribute('tabindex'))el.tabIndex=0;
    return el;
  }

  function focusables(){
    const selectors=[
      'button:not([disabled])',
      '[role="button"]',
      '.sf-setting-option',
      '#sf-progress'
    ];
    const seen=new Set();
    return [...modal.querySelectorAll(selectors.join(','))].map(makeFocusable).filter(el=>{
      if(seen.has(el)||!visible(el)||el.id==='sf-volume')return false;
      if(el.closest('.hidden'))return false;
      seen.add(el);return true;
    });
  }

  function defaultFocus(){
    const preferred=['#sf-center-play','#sf-play','#sf-settings','#sf-v4-close'];
    for(const selector of preferred){const el=document.querySelector(selector);if(visible(el)){makeFocusable(el).focus();return el}}
    const first=focusables()[0];first?.focus();return first||null;
  }

  function focusFirstInOpenMenu(){
    const menus=menuElements();
    const menu=menus[menus.length-1];
    if(!menu)return false;
    const item=[...menu.querySelectorAll('button:not([disabled]),[role="button"],.sf-setting-option')].map(makeFocusable).find(visible);
    if(item){item.focus();return true}
    return false;
  }

  function move(direction){
    showControls();
    const items=focusables();
    if(!items.length)return false;
    let active=document.activeElement;
    if(!active||!modal.contains(active)||!visible(active))active=defaultFocus();
    if(!active)return false;

    // The timeline is an intentional interactive target: left/right seek while
    // focus remains on it instead of allowing the browser range default.
    if(active.id==='sf-progress'&&(direction==='left'||direction==='right')){
      seek(direction==='left'?-10:10);return true;
    }

    const a=active.getBoundingClientRect();
    const ax=a.left+a.width/2,ay=a.top+a.height/2;
    let winner=null,best=Infinity;
    for(const item of items){
      if(item===active)continue;
      const r=item.getBoundingClientRect();
      const x=r.left+r.width/2,y=r.top+r.height/2;
      const dx=x-ax,dy=y-ay;
      if(direction==='left'&&dx>=-2)continue;
      if(direction==='right'&&dx<=2)continue;
      if(direction==='up'&&dy>=-2)continue;
      if(direction==='down'&&dy<=2)continue;
      const primary=(direction==='left'||direction==='right')?Math.abs(dx):Math.abs(dy);
      const secondary=(direction==='left'||direction==='right')?Math.abs(dy):Math.abs(dx);
      const score=primary+(secondary*2.35)+(secondary>primary*1.8?primary*2:0);
      if(score<best){best=score;winner=item}
    }
    if(winner){winner.focus();winner.scrollIntoView?.({block:'nearest',inline:'nearest'});return true}
    return false;
  }

  function seek(delta){
    const duration=Number(video.duration);
    const current=Number(video.currentTime)||0;
    const next=Math.max(0,Number.isFinite(duration)?Math.min(duration,current+delta):current+delta);
    try{video.currentTime=next}catch(_){ }
    showControls();
    window.sfToast?.(`${delta>0?'+':''}${delta}s`);
  }

  function play(){video.play().catch(()=>{})}
  function pause(){video.pause()}
  function playPause(){video.paused?play():pause()}

  function clickControl(selector){
    showControls();
    const el=document.querySelector(selector);
    if(!visible(el))return false;
    makeFocusable(el).focus();el.click();return true;
  }

  function openSettings(){
    showControls();
    if(menuOpen()){
      const settings=document.querySelector('#sf-player-settings-panel');
      if(visible(settings)){focusFirstInOpenMenu();return true}
    }
    const button=document.querySelector('#sf-settings')||document.querySelector('#player-settings-top');
    if(!visible(button))return false;
    makeFocusable(button).focus();button.click();
    setTimeout(()=>focusFirstInOpenMenu(),30);
    return true;
  }

  function closeTopMenu(){
    const menus=menuElements();
    const top=menus[menus.length-1];
    if(!top)return false;
    top.classList.add('hidden');
    showControls();
    setTimeout(()=>{
      const target=document.querySelector('#sf-settings')||document.querySelector('#sf-v4-audio')||document.querySelector('#sf-v54-screen');
      if(visible(target))target.focus();
    },0);
    return true;
  }

  function back(){
    if(closeTopMenu())return true;
    if(!controlsHidden()){
      hideControls();return true;
    }
    try{
      if(typeof closePlayer==='function')closePlayer();
      else if(window.StormFlixShell?.close)window.StormFlixShell.close();
    }catch(_){try{window.StormFlixShell?.close?.()}catch(__){}}
    return true;
  }

  function select(){
    const wasHidden=controlsHidden();
    showControls();
    const active=document.activeElement;
    if(wasHidden||!active||!modal.contains(active)||active===video||active===document.body){
      defaultFocus();
      return true;
    }
    const target=active.closest?.('button,[role="button"],.sf-setting-option');
    if(target&&visible(target)){target.click();resetHideTimer();return true}
    defaultFocus();return true;
  }

  function nextTrack(){
    if(!clickControl('#sf-v4-next'))window.sfToast?.('Não há próximo episódio');
  }
  function previousTrack(){
    if(!clickControl('#sf-v4-previous'))window.sfToast?.('Não há episódio anterior');
  }

  function handleCommand(command,options={}){
    if(!playerOpen())return false;
    command=String(command||'').toLowerCase();
    lastCommandAt=Date.now();
    switch(command){
      case 'up':return move('up');
      case 'down':return move('down');
      case 'left':
        if(controlsHidden()&&!menuOpen()){seek(-10);return true}
        return move('left')||true;
      case 'right':
        if(controlsHidden()&&!menuOpen()){seek(10);return true}
        return move('right')||true;
      case 'select':return select();
      case 'menu':case 'settings':return openSettings();
      case 'back':return back();
      case 'play':play();showControls();return true;
      case 'pause':pause();showControls();return true;
      case 'playpause':playPause();showControls();return true;
      case 'rewind':seek(options.large?-30:-10);return true;
      case 'fastforward':seek(options.large?30:10);return true;
      case 'stop':pause();try{typeof closePlayer==='function'?closePlayer():window.StormFlixShell?.close?.()}catch(_){ }return true;
      case 'previoustrack':previousTrack();return true;
      case 'nexttrack':nextTrack();return true;
      case 'audio':return clickControl('#sf-v4-audio');
      case 'subtitles':return clickControl('#sf-subtitle');
      case 'fullscreen':return clickControl('#sf-fullscreen');
      default:return false;
    }
  }

  function commandFromEvent(event){
    const numeric=keyCodes[Number(event.keyCode||event.which||0)];
    if(numeric)return numeric;
    const direct=keyNames[event.key]||keyNames[event.code];
    if(direct)return direct;
    const key=String(event.key||'').toLowerCase();
    if(key==='backspace'&&browserTv)return'back';
    return'';
  }

  function browserKeyHandler(event){
    if(!playerOpen())return;
    // Native Android/Fire TV events are consumed in PlayerActivity before they
    // reach WebView. This path is for Tizen/webOS and TV browser keyboards.
    if(!(tvMode||nativePlayer))return;
    if(event.ctrlKey||event.altKey||event.metaKey||event.shiftKey)return;
    const command=commandFromEvent(event);if(!command)return;
    event.preventDefault();event.stopPropagation();event.stopImmediatePropagation?.();
    handleCommand(command,{repeat:Boolean(event.repeat)});
  }

  function registerTizenMediaKeys(){
    if(!window.tizen?.tvinputdevice?.registerKey)return;
    const keys=['MediaPlay','MediaPause','MediaStop','MediaTrackPrevious','MediaTrackNext','MediaRewind','MediaFastForward','MediaPlayPause'];
    for(const key of keys){try{window.tizen.tvinputdevice.registerKey(key)}catch(_){}}
  }

  window.sfTvRemote={
    handleCommand,
    handleNativeKey:(command,repeat)=>handleCommand(command,{repeat:Boolean(repeat)}),
    showControls,
    hideControls,
    openSettings,
    platform:()=>({nativePlayer,androidTv,tizen,webos,browserTv,tvMode,lastCommandAt})
  };

  window.addEventListener('keydown',browserKeyHandler,true);
  video.addEventListener('play',resetHideTimer,{passive:true});
  video.addEventListener('pause',showControls,{passive:true});
  modal.addEventListener('mousemove',()=>{if(tvMode)showControls()},{passive:true});
  modal.addEventListener('click',()=>{if(tvMode)resetHideTimer()},{passive:true});
  registerTizenMediaKeys();
  installTvStyle();
})();
