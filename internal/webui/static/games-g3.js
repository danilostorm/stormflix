/* StormFlix Games G3: mobile virtual controls, TV/gamepad navigation and save previews. */
(function(){
  const STORAGE_CONTROLS='stormflix.games.virtual-controls';
  const STORAGE_HAPTICS='stormflix.games.haptics';
  const MAX_PREVIEW=2*1024*1024;
  const keySpec={
    up:['ArrowUp','ArrowUp',38],down:['ArrowDown','ArrowDown',40],left:['ArrowLeft','ArrowLeft',37],right:['ArrowRight','ArrowRight',39],
    a:['z','KeyZ',90],b:['x','KeyX',88],start:['Enter','Enter',13],select:['Shift','ShiftLeft',16]
  };
  let activeGame=null,playerOverlay=null,quickMenu=null,controller=null,previewTimer=null,padTimer=null;
  let comboSince=0,comboLatched=false,lastPadButtons=[],lastAxes=[false,false,false,false],pressedPointers=new Map();
  let fetchPatched=false,playerPatched=false;
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

  function controlsMode(){return localStorage.getItem(STORAGE_CONTROLS)||'auto'}
  function haptics(){return localStorage.getItem(STORAGE_HAPTICS)!=='off'}
  function coarsePointer(){return !!window.matchMedia?.('(pointer: coarse)').matches||innerWidth<=900}
  function shouldShowController(){const mode=controlsMode();return mode==='on'||(mode==='auto'&&coarsePointer())}

  function patchFetch(){
    if(fetchPatched)return;fetchPatched=true;
    const original=window.fetch.bind(window);
    window.fetch=async function(input,init){
      const response=await original(input,init);
      try{
        const raw=typeof input==='string'?input:input?.url||'';
        const url=new URL(raw,location.href);
        const method=String(init?.method||input?.method||'GET').toUpperCase();
        if(method==='GET'&&url.pathname==='/api/v1/games/saves'&&response.ok){
          const data=await response.clone().json();
          if(Array.isArray(data)&&data.some(x=>x?.kind==='preview')){
            const headers=new Headers(response.headers);headers.delete('content-length');
            return new Response(JSON.stringify(data.filter(x=>x?.kind!=='preview')),{status:response.status,statusText:response.statusText,headers});
          }
        }
      }catch{}
      return response;
    };
  }

  function patchPlayer(){
    if(playerPatched||!window.StormFlixGamePlayer)return false;
    playerPatched=true;
    const originalOpen=window.StormFlixGamePlayer.open.bind(window.StormFlixGamePlayer);
    const originalClose=window.StormFlixGamePlayer.close?.bind(window.StormFlixGamePlayer);
    window.StormFlixGamePlayer.open=async function(game){
      activeGame=typeof game==='object'?game:{id:Number(game)};
      const result=await originalOpen(game);
      waitForPlayer();
      return result;
    };
    if(originalClose)window.StormFlixGamePlayer.close=async function(){releaseAll();clearPreviewTimer();const result=await originalClose();cleanupPlayerEnhancements();return result};
    return true;
  }

  function waitForPlayer(){
    let tries=0;const timer=setInterval(()=>{
      const overlay=$('#game-player-overlay');
      if(overlay){clearInterval(timer);installPlayerEnhancements(overlay)}
      else if(++tries>80)clearInterval(timer);
    },50);
  }

  function installPlayerEnhancements(overlay){
    if(!overlay||overlay.dataset.g3Ready==='1')return;
    const game=activeGame;cleanupPlayerEnhancements();activeGame=game;playerOverlay=overlay;overlay.dataset.g3Ready='1';overlay.classList.add('game-player-g3');
    const kicker=$('.game-player-kicker',overlay);if(kicker)kicker.textContent='STORMFLIX GAME PLAYER G3';
    const actions=$('.game-player-top-actions',overlay);
    if(actions){const menu=document.createElement('button');menu.type='button';menu.dataset.g3Menu='1';menu.className='g3-menu-button';menu.setAttribute('aria-label','Menu rápido');menu.textContent='☰';menu.onclick=openQuickMenu;actions.insertBefore(menu,actions.firstChild)}
    injectVirtualController(overlay);injectQuickMenu(overlay);bindSavePreview(overlay);updateControllerVisibility();
    clearPreviewTimer();previewTimer=setInterval(()=>{if(isRunning())uploadPreview().catch(()=>{})},125000);
    setTimeout(enhanceSaveGallery,100);
  }

  function injectVirtualController(overlay){
    controller=document.createElement('div');controller.className='g3-virtual-controller';controller.dataset.g3Controller='1';
    controller.innerHTML=`<div class="g3-dpad" aria-label="Direcional"><button data-vkey="up" aria-label="Cima">▲</button><button data-vkey="left" aria-label="Esquerda">◀</button><span></span><button data-vkey="right" aria-label="Direita">▶</button><button data-vkey="down" aria-label="Baixo">▼</button></div><div class="g3-center-controls"><button data-vkey="select">SELECT</button><button data-vkey="start">START</button></div><div class="g3-action-pad"><button class="g3-b" data-vkey="b">B</button><button class="g3-a" data-vkey="a">A</button></div>`;
    $('.game-player-stage',overlay)?.appendChild(controller);
    $$('[data-vkey]',controller).forEach(button=>{
      const down=e=>{e.preventDefault();e.stopPropagation();if(haptics()&&navigator.vibrate)navigator.vibrate(9);button.classList.add('pressed');button.setPointerCapture?.(e.pointerId);pressedPointers.set(e.pointerId,button.dataset.vkey);sendVirtualKey(button.dataset.vkey,true)};
      const up=e=>{e.preventDefault();e.stopPropagation();const key=pressedPointers.get(e.pointerId)||button.dataset.vkey;pressedPointers.delete(e.pointerId);button.classList.remove('pressed');sendVirtualKey(key,false)};
      button.addEventListener('pointerdown',down,{passive:false});button.addEventListener('pointerup',up,{passive:false});button.addEventListener('pointercancel',up,{passive:false});button.addEventListener('contextmenu',e=>e.preventDefault());
    });
  }

  function makeKeyEvent(type,key,code,keyCode){
    const event=new KeyboardEvent(type,{key,code,bubbles:true,cancelable:true,composed:true});
    try{Object.defineProperty(event,'keyCode',{get:()=>keyCode});Object.defineProperty(event,'which',{get:()=>keyCode})}catch{}
    return event;
  }
  function sendVirtualKey(name,down){
    const spec=keySpec[name],canvas=$('#stormflix-game-canvas');if(!spec||!canvas)return;
    canvas.focus({preventScroll:true});canvas.dispatchEvent(makeKeyEvent(down?'keydown':'keyup',...spec));
  }
  function releaseAll(){for(const key of new Set(pressedPointers.values()))sendVirtualKey(key,false);pressedPointers.clear();$$('[data-vkey].pressed',controller||document).forEach(x=>x.classList.remove('pressed'))}

  function injectQuickMenu(overlay){
    quickMenu=document.createElement('div');quickMenu.className='g3-quick-menu hidden';quickMenu.dataset.g3QuickMenu='1';
    quickMenu.innerHTML=`<article class="g3-quick-card" role="dialog" aria-modal="true"><p>STORMFLIX G3</p><h2>${esc(activeGame?.title||'Jogo')}</h2><small>Menu rápido · no gamepad segure SELECT + START</small><div class="g3-quick-preview"><canvas data-g3-preview-canvas></canvas><span>Preview do save-state</span></div><div class="g3-quick-actions"><button class="primary" data-g3-resume>Continuar</button><button data-g3-save>Salvar agora</button><button data-g3-fullscreen>Tela cheia</button><button data-g3-controls>Controle virtual: <b></b></button><button class="danger" data-g3-exit>Salvar e sair</button></div></article>`;
    $('.game-player-stage',overlay)?.appendChild(quickMenu);
    $('[data-g3-resume]',quickMenu).onclick=closeQuickMenu;
    $('[data-g3-save]',quickMenu).onclick=async()=>{const b=$('[data-game-save]',overlay);b?.click();await delay(650);await uploadPreview();refreshMenuPreview();focusMenu('[data-g3-resume]')};
    $('[data-g3-fullscreen]',quickMenu).onclick=()=>{$('[data-game-fullscreen]',overlay)?.click();focusMenu('[data-g3-resume]')};
    $('[data-g3-controls]',quickMenu).onclick=cycleControllerMode;
    $('[data-g3-exit]',quickMenu).onclick=async()=>{await uploadPreview().catch(()=>{});$('[data-game-exit]',overlay)?.click()};
    syncControlsLabel();
  }

  function isRunning(){return !!playerOverlay&&!$('[data-game-controls]',playerOverlay)?.classList.contains('hidden')}
  function isPaused(){const b=$('[data-game-pause]',playerOverlay);return /continuar/i.test(b?.textContent||'')}
  function openQuickMenu(){
    if(!playerOverlay||!isRunning()||!quickMenu)return;
    releaseAll();if(!isPaused())$('[data-game-pause]',playerOverlay)?.click();
    quickMenu.classList.remove('hidden');refreshMenuPreview();syncControlsLabel();setTimeout(()=>focusMenu('[data-g3-resume]'),60);
  }
  function closeQuickMenu(){if(!quickMenu||quickMenu.classList.contains('hidden'))return;quickMenu.classList.add('hidden');if(isPaused())$('[data-game-pause]',playerOverlay)?.click();$('#stormflix-game-canvas')?.focus({preventScroll:true})}
  function focusMenu(sel){$(sel,quickMenu)?.focus()}

  function cycleControllerMode(){
    const modes=['auto','on','off'],current=controlsMode(),next=modes[(modes.indexOf(current)+1)%modes.length];localStorage.setItem(STORAGE_CONTROLS,next);updateControllerVisibility();syncControlsLabel();enhanceSettings();
  }
  function syncControlsLabel(){const b=$('[data-g3-controls] b',quickMenu);if(b)b.textContent=({auto:'Auto',on:'Ligado',off:'Desligado'}[controlsMode()]||'Auto')}
  function updateControllerVisibility(){if(controller)controller.classList.toggle('hidden',!shouldShowController())}

  async function capturePreviewBlob(maxWidth=640,quality=.82){
    const source=$('#stormflix-game-canvas');if(!source||!source.width||!source.height)return null;
    const ratio=Math.min(1,maxWidth/source.width);const width=Math.max(1,Math.round(source.width*ratio)),height=Math.max(1,Math.round(source.height*ratio));
    const out=document.createElement('canvas');out.width=width;out.height=height;const ctx=out.getContext('2d',{alpha:false});if(!ctx)return null;
    ctx.imageSmoothingEnabled=true;ctx.drawImage(source,0,0,width,height);
    let blob=await new Promise(resolve=>out.toBlob(resolve,'image/webp',quality));
    if(!blob)blob=await new Promise(resolve=>out.toBlob(resolve,'image/png'));
    if(blob?.size>MAX_PREVIEW&&maxWidth>360)return capturePreviewBlob(Math.round(maxWidth*.75),Math.max(.58,quality-.12));
    return blob&&blob.size<=MAX_PREVIEW?blob:null;
  }
  async function uploadPreview(){
    const id=Number(activeGame?.id||0);if(!id||!isRunning())return null;
    const blob=await capturePreviewBlob();if(!blob)return null;
    const response=await fetch(`/api/v1/games/${id}/saves/preview`,{method:'PUT',credentials:'same-origin',headers:{'Content-Type':'application/octet-stream'},body:blob});
    if(!response.ok)throw new Error('preview do save não pôde ser sincronizado');
    return response.json().catch(()=>({}));
  }
  function bindSavePreview(overlay){
    $('[data-game-save]',overlay)?.addEventListener('click',()=>setTimeout(()=>uploadPreview().catch(()=>{}),1200));
    $('[data-game-exit]',overlay)?.addEventListener('click',()=>uploadPreview().catch(()=>{}),{capture:true});
  }
  function clearPreviewTimer(){if(previewTimer)clearInterval(previewTimer);previewTimer=null}
  function delay(ms){return new Promise(resolve=>setTimeout(resolve,ms))}

  async function refreshMenuPreview(){
    const target=$('[data-g3-preview-canvas]',quickMenu),source=$('#stormflix-game-canvas');if(!target||!source||!source.width)return;
    target.width=Math.min(480,source.width);target.height=Math.max(1,Math.round(source.height*(target.width/source.width)));target.getContext('2d')?.drawImage(source,0,0,target.width,target.height);
  }

  async function loadPreviewInto(img,id){
    if(!img||!id||img.dataset.g3Loaded==='1')return;img.dataset.g3Loaded='1';
    try{const r=await fetch(`/api/v1/games/${id}/saves/preview`,{credentials:'same-origin',cache:'no-store'});if(!r.ok)return;const raw=await r.blob();if(!raw.size)return;const blob=new Blob([raw],{type:'image/webp'}),url=URL.createObjectURL(blob);img.onload=()=>URL.revokeObjectURL(url);img.src=url;img.classList.add('ready')}catch{}
  }
  function enhanceSaveGallery(){
    if(!document.body.classList.contains('games-mode'))return;
    $$('.gx-save-grid [data-game-open]').forEach(card=>{
      const id=Number(card.dataset.gameOpen||0),cover=$('.gx-save-cover',card);if(!id||!cover||cover.querySelector('[data-g3-save-preview]'))return;
      const img=document.createElement('img');img.dataset.g3SavePreview='1';img.className='g3-save-preview';img.alt='Preview do save-state';cover.prepend(img);loadPreviewInto(img,id);
    });
    enhanceSettings();
  }

  function enhanceSettings(){
    const page=$('.gx-settings');if(!page||page.querySelector('[data-g3-settings]'))return;
    const card=document.createElement('div');card.className='gx-settings-card g3-settings-card';card.dataset.g3Settings='1';
    card.innerHTML=`<h2>Controles G3</h2><div class="gx-setting-row"><div><b>Controle virtual no celular</b><small>Auto aparece em telas touch; Ligado força exibição; Desligado deixa somente teclado/gamepad.</small></div><div class="gx-setting-options">${['auto','on','off'].map(v=>`<button class="${controlsMode()===v?'active':''}" data-g3-control-mode="${v}">${v==='auto'?'Auto':v==='on'?'Ligado':'Desligado'}</button>`).join('')}</div></div><div class="gx-setting-row"><div><b>Vibração dos botões touch</b><small>Feedback tátil curto em dispositivos compatíveis.</small></div><div class="gx-setting-options"><button class="${haptics()?'active':''}" data-g3-haptics="on">On</button><button class="${!haptics()?'active':''}" data-g3-haptics="off">Off</button></div></div><div class="gx-setting-row"><div><b>Menu rápido de TV</b><small>Durante o jogo: Back/Esc abre o menu. No gamepad, segure SELECT + START.</small></div><span class="gx-status">ATIVO</span></div>`;
    page.appendChild(card);
    $$('[data-g3-control-mode]',card).forEach(b=>b.onclick=()=>{localStorage.setItem(STORAGE_CONTROLS,b.dataset.g3ControlMode);page.querySelector('[data-g3-settings]')?.remove();enhanceSettings();updateControllerVisibility()});
    $$('[data-g3-haptics]',card).forEach(b=>b.onclick=()=>{localStorage.setItem(STORAGE_HAPTICS,b.dataset.g3Haptics==='off'?'off':'on');page.querySelector('[data-g3-settings]')?.remove();enhanceSettings()});
  }

  function visibleFocusable(root=document){return $$('button:not([disabled]),a[href],input:not([disabled]),[tabindex="0"]',root).filter(el=>{const r=el.getBoundingClientRect();const s=getComputedStyle(el);return r.width>0&&r.height>0&&s.visibility!=='hidden'&&s.display!=='none'})}
  function moveFocus(direction,scope){
    const items=visibleFocusable(scope||document),current=document.activeElement;if(!items.length)return;
    if(!items.includes(current)){items[0].focus();return}
    const cr=current.getBoundingClientRect(),cx=cr.left+cr.width/2,cy=cr.top+cr.height/2;let best=null,bestScore=Infinity;
    for(const el of items){if(el===current)continue;const r=el.getBoundingClientRect(),x=r.left+r.width/2,y=r.top+r.height/2,dx=x-cx,dy=y-cy;
      if((direction==='left'&&dx>=-4)||(direction==='right'&&dx<=4)||(direction==='up'&&dy>=-4)||(direction==='down'&&dy<=4))continue;
      const primary=(direction==='left'||direction==='right')?Math.abs(dx):Math.abs(dy),cross=(direction==='left'||direction==='right')?Math.abs(dy):Math.abs(dx),score=primary+cross*2.35;if(score<bestScore){bestScore=score;best=el}}
    (best||current).focus({preventScroll:true});best?.scrollIntoView({block:'nearest',inline:'nearest'});
  }

  function gamepadTick(){
    const pad=[...(navigator.getGamepads?.()||[])].find(Boolean);if(!pad){lastPadButtons=[];comboSince=0;comboLatched=false;return}
    const pressed=i=>!!pad.buttons?.[i]?.pressed;
    const combo=pressed(8)&&pressed(9);if(combo){if(!comboSince)comboSince=performance.now();if(!comboLatched&&performance.now()-comboSince>650){comboLatched=true;if(isRunning())openQuickMenu()}}else{comboSince=0;comboLatched=false}
    const left=pressed(14)||(pad.axes?.[0]??0)<-.62,right=pressed(15)||(pad.axes?.[0]??0)>.62,up=pressed(12)||(pad.axes?.[1]??0)<-.62,down=pressed(13)||(pad.axes?.[1]??0)>.62;
    const dirs=[left,right,up,down],names=['left','right','up','down'];
    const scope=quickMenu&&!quickMenu.classList.contains('hidden')?quickMenu:(!isRunning()&&playerOverlay?playerOverlay:(document.body.classList.contains('games-mode')?$('#games-view'):null));
    if(scope)dirs.forEach((v,i)=>{if(v&&!lastAxes[i])moveFocus(names[i],scope)});lastAxes=dirs;
    const a=pressed(0),b=pressed(1);if(scope&&a&&!lastPadButtons[0]){const el=document.activeElement;if(scope.contains(el)&&typeof el.click==='function')el.click()}
    if(b&&!lastPadButtons[1]){if(quickMenu&&!quickMenu.classList.contains('hidden'))closeQuickMenu();else if(!isRunning()&&playerOverlay)$('[data-game-close]',playerOverlay)?.click();else if(document.body.classList.contains('games-mode'))dispatchEscape()}
    lastPadButtons=pad.buttons?.map(x=>!!x.pressed)||[];
  }

  function dispatchEscape(){const target=document.activeElement||document;target.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',code:'Escape',bubbles:true,cancelable:true}))}

  document.addEventListener('keydown',event=>{
    if(event.key!=='Escape'||!playerOverlay||!isRunning())return;
    event.preventDefault();event.stopImmediatePropagation();
    if(quickMenu&&!quickMenu.classList.contains('hidden'))closeQuickMenu();else openQuickMenu();
  },true);

  document.addEventListener('click',event=>{if(event.target.closest('[data-gx-screen="saves"]'))setTimeout(enhanceSaveGallery,250);if(event.target.closest('[data-gx-screen="settings"]'))setTimeout(enhanceSettings,100)});
  const observer=new MutationObserver(()=>{if($('#game-player-overlay')&&!playerOverlay)installPlayerEnhancements($('#game-player-overlay'));if(playerOverlay&&!document.body.contains(playerOverlay))cleanupPlayerEnhancements();if(document.body.classList.contains('games-mode'))queueMicrotask(enhanceSaveGallery)});
  observer.observe(document.documentElement,{childList:true,subtree:true});
  addEventListener('resize',updateControllerVisibility);addEventListener('orientationchange',()=>setTimeout(updateControllerVisibility,150));
  addEventListener('blur',releaseAll);document.addEventListener('visibilitychange',()=>{if(document.hidden){releaseAll();if(isRunning())uploadPreview().catch(()=>{})}});
  window.addEventListener('stormflix:game-closed',cleanupPlayerEnhancements);

  function cleanupPlayerEnhancements(){releaseAll();clearPreviewTimer();playerOverlay=null;quickMenu=null;controller=null;activeGame=null}

  patchFetch();
  let patchTries=0;const patchTimer=setInterval(()=>{if(patchPlayer()||++patchTries>100)clearInterval(patchTimer)},40);
  padTimer=setInterval(gamepadTick,80);
})();
