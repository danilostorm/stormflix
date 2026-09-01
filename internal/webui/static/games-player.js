/* StormFlix Games G4: single-owner Nostalgist/RetroArch browser session. */
(function(){
  const RUNTIME_LABEL='Nostalgist 0.21.1 · RetroArch cores v1.22.2';
  const PREF_KEY='stormflix.games.g4.preferences';
  const coreByPlatform={nes:'fceumm',snes:'snes9x',genesis:'genesis_plus_gx',gb:'mgba',gbc:'mgba',gba:'mgba'};
  const retroInputs=['up','down','left','right','a','b','x','y','l','r','l2','r2','select','start'];
  const validInputs=new Set(retroInputs),pressedInputs=new Set(),keyboardPressed=new Map();
  const internalKeyboardConfig={
    input_player1_a:'x',input_player1_b:'z',input_player1_x:'s',input_player1_y:'a',
    input_player1_l:'q',input_player1_r:'w',input_player1_l2:'e',input_player1_r2:'r',
    input_player1_up:'up',input_player1_down:'down',input_player1_left:'left',input_player1_right:'right',
    input_player1_select:'rshift',input_player1_start:'enter',
  };
  const defaultKeyboard={
    up:'ArrowUp',down:'ArrowDown',left:'ArrowLeft',right:'ArrowRight',
    a:'KeyX',b:'KeyZ',x:'KeyS',y:'KeyA',l:'KeyQ',r:'KeyW',l2:'KeyE',r2:'KeyR',select:'ShiftRight',start:'Enter',
  };
  const defaultGamepad={
    input_player1_a_btn:'1',input_player1_b_btn:'0',input_player1_x_btn:'3',input_player1_y_btn:'2',
    input_player1_select_btn:'8',input_player1_start_btn:'9',input_player1_up_btn:'12',input_player1_down_btn:'13',
    input_player1_left_btn:'14',input_player1_right_btn:'15',input_player1_l_btn:'4',input_player1_r_btn:'5',
    input_player1_l1_btn:'4',input_player1_r1_btn:'5',input_player1_l2_btn:'6',input_player1_r2_btn:'7',
    input_player1_l3_btn:'10',input_player1_r3_btn:'11',
  };
  const defaultPrefs={
    keyboard:defaultKeyboard,gamepads:{},touch:{mode:'auto',haptics:true,mapping:{}},
    video:{smooth:false,integerScale:false,display:'fit',saturation:100},
    emulator:{rewind:true,autoSaveSeconds:120,fullscreen:false},
  };
  let overlay=null,canvas=null,instance=null,current=null,prepareMode='normal',preparePromise=null;
  let sessionID='',elapsed=0,lastTick=0,tickTimer=null,heartbeatTimer=null,autosaveTimer=null,gamepadTimer=null;
  let savePromise=null,closing=false,scriptPromise=null,romPromise=null,resizeObserver=null,resizeTimer=null;
  let keyboardAbort=null,viewportAbort=null;
  const $=(sel,root=document)=>root.querySelector(sel);
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot',"'":'&#39;'}[c]));
  const platformLabel=p=>({nes:'Nintendo Entertainment System',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'}[p]||String(p||'').toUpperCase());
  const fmtBytes=n=>{n=Number(n||0);if(n<1024)return `${n} B`;if(n<1048576)return `${(n/1024).toFixed(1)} KB`;return `${(n/1048576).toFixed(1)} MB`};
  const clock=s=>{s=Math.max(0,Math.floor(Number(s)||0));const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${m} min`};

  function clone(v){return JSON.parse(JSON.stringify(v))}
  function merge(base,extra){
    const out=clone(base);if(!extra||typeof extra!=='object')return out;
    for(const [k,v] of Object.entries(extra)){if(v&&typeof v==='object'&&!Array.isArray(v)&&out[k]&&typeof out[k]==='object'&&!Array.isArray(out[k]))out[k]=merge(out[k],v);else out[k]=v}
    return out;
  }
  function preferences(){try{return merge(defaultPrefs,JSON.parse(localStorage.getItem(PREF_KEY)||'{}'))}catch{return clone(defaultPrefs)}}
  function savePreferences(next){localStorage.setItem(PREF_KEY,JSON.stringify(merge(defaultPrefs,next)));window.dispatchEvent(new CustomEvent('stormflix:game-preferences-changed',{detail:preferences()}));applyPresentation();return preferences()}
  function patchPreferences(patch){return savePreferences(merge(preferences(),patch))}
  function resetPreferences(){localStorage.removeItem(PREF_KEY);window.dispatchEvent(new CustomEvent('stormflix:game-preferences-changed',{detail:preferences()}));applyPresentation();return preferences()}
  function firstGamepad(){try{return [...(navigator.getGamepads?.()||[])].find(Boolean)||null}catch{return null}}
  function gamepadConfig(){const p=preferences(),pad=firstGamepad(),custom=pad?.id?p.gamepads?.[pad.id]:null;return {...defaultGamepad,...(custom||{})}}

  function randomSession(){if(globalThis.crypto?.randomUUID)return crypto.randomUUID();return `sf-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`}
  async function jsonFetch(url,options){const r=await fetch(url,{credentials:'same-origin',...(options||{})});const text=await r.text();let data={};try{data=text?JSON.parse(text):{}}catch{}if(!r.ok)throw new Error(data.error||`HTTP ${r.status}`);return data}
  async function blobFetch(url,optional=false){const r=await fetch(url,{credentials:'same-origin',cache:'no-store'});if(optional&&r.status===404)return null;if(!r.ok){let msg=`HTTP ${r.status}`;try{const d=await r.json();msg=d.error||msg}catch{}throw new Error(msg)}return r.blob()}

  function ensureRuntime(){
    if(globalThis.Nostalgist)return Promise.resolve(globalThis.Nostalgist);if(scriptPromise)return scriptPromise;
    scriptPromise=new Promise((resolve,reject)=>{const existing=document.querySelector('script[data-stormflix-nostalgist]');if(existing){existing.addEventListener('load',()=>resolve(globalThis.Nostalgist),{once:true});existing.addEventListener('error',()=>reject(new Error('Falha ao carregar runtime de jogos')),{once:true});return}const s=document.createElement('script');s.src='/api/v1/games/runtime/nostalgist.js';s.async=true;s.dataset.stormflixNostalgist='1';s.onload=()=>globalThis.Nostalgist?resolve(globalThis.Nostalgist):reject(new Error('Runtime Nostalgist não inicializou'));s.onerror=()=>reject(new Error('Não foi possível preparar o runtime de jogos'));document.head.appendChild(s)});
    return scriptPromise;
  }

  function createOverlay(game){
    if(overlay)overlay.remove();overlay=document.createElement('section');overlay.id='game-player-overlay';overlay.className='game-player-overlay sf-game-g4';overlay.setAttribute('role','dialog');overlay.setAttribute('aria-modal','true');overlay.setAttribute('aria-label',`Jogando ${game.title}`);
    overlay.innerHTML=`<header class="game-player-top"><div><span class="game-player-kicker">STORMFLIX GAME PLAYER G4</span><strong data-game-player-title>${esc(game.title)}</strong></div><div class="game-player-top-actions"><button type="button" data-game-menu aria-label="Configurações do jogo">☰</button><span class="game-runtime-badge">${esc(RUNTIME_LABEL)}</span><button type="button" data-game-fullscreen aria-label="Tela cheia">⛶</button><button type="button" data-game-close aria-label="Sair do jogo">✕</button></div></header><div class="game-player-stage" data-game-stage><canvas id="stormflix-game-canvas" class="game-player-canvas" tabindex="0"></canvas><div class="game-launch-panel" data-game-launch></div><div class="game-player-toast hidden" data-game-toast></div></div><footer class="game-player-controls hidden" data-game-controls><div class="game-player-status"><span class="gamepad-dot" data-gamepad-dot></span><span data-gamepad-label>Teclado pronto</span><span data-game-input-label>Input: —</span><span data-playtime>Tempo deste perfil: ${clock(game.play_seconds)}</span></div><div class="game-player-buttons"><button type="button" data-game-pause>Pausar</button><button type="button" data-game-save>Salvar agora</button><button type="button" data-game-exit>Salvar e sair</button></div></footer>`;
    document.body.appendChild(overlay);canvas=$('#stormflix-game-canvas',overlay);
    $('[data-game-close]',overlay).onclick=()=>close(true);$('[data-game-exit]',overlay).onclick=()=>close(true);$('[data-game-fullscreen]',overlay).onclick=toggleFullscreen;$('[data-game-pause]',overlay).onclick=togglePause;$('[data-game-save]',overlay).onclick=()=>saveAll(true);$('[data-game-menu]',overlay).onclick=()=>window.dispatchEvent(new CustomEvent('stormflix:game-menu-request'));
    const stage=$('[data-game-stage]',overlay);stage?.addEventListener('pointerdown',e=>{if(e.target===stage||e.target===canvas)focusGame()});updateGamepadLabel();applyPresentation();
  }
  function loadingHTML(game,mode){return `<div class="game-launch-card loading"><span class="game-loader"></span><p>${esc(platformLabel(game.platform))}</p><h2>Abrindo ${esc(game.title)}…</h2><div class="game-launch-facts"><span>${esc(game.rom_name||'ROM')}</span><span>${fmtBytes(game.rom_size_bytes)}</span><span>${esc(game.core||coreByPlatform[game.platform]||'core')}</span></div><p>${mode==='continue'?'Restaurando seu save state e carregando o emulador.':'Carregando ROM e emulador.'}</p><small>Teclado, gamepad e touch usam a mesma camada de input do G4.</small></div>`}
  function showLaunchError(message,mode){const launch=$('[data-game-launch]',overlay);if(!launch)return;launch.classList.remove('hidden');launch.innerHTML=`<div class="game-launch-card error"><span class="game-ready-icon">!</span><h2>Não foi possível abrir o jogo</h2><p>${esc(message)}</p><button class="primary" type="button" data-game-retry>Tentar novamente</button></div>`;$('[data-game-retry]',launch).onclick=()=>prepare(mode)}

  async function open(gameInput){
    if(closing)return;if(instance||overlay)await close(false);pauseOtherMedia();releaseAllInputs();let game;
    try{game=await jsonFetch(`/api/v1/games/${Number(gameInput.id||gameInput)}`)}catch(err){notify(err.message);return}
    if(!game.playable||!game.core){notify('Este jogo não está disponível no navegador.');return}
    current=game;sessionID=randomSession();elapsed=0;preparePromise=null;prepareMode=game.saves?.state?.exists?'continue':'normal';savePromise=null;closing=false;createOverlay(game);romPromise=blobFetch(`/api/v1/games/${game.id}/rom`);$('[data-game-launch]',overlay).innerHTML=loadingHTML(game,prepareMode);void prepare(prepareMode);
  }
  async function prepare(mode){
    if(!current||preparePromise)return;prepareMode=mode==='continue'?'continue':'normal';const launch=$('[data-game-launch]',overlay);if(!launch)return;launch.classList.remove('hidden');launch.innerHTML=loadingHTML(current,prepareMode);preparePromise=prepareEmulator(prepareMode);
    try{await preparePromise}catch(err){preparePromise=null;if(instance){try{instance.exit()}catch{}instance=null}showLaunchError(err?.message||'Falha ao preparar o emulador.',prepareMode);return}
    try{await startPrepared()}catch(err){launch.innerHTML=`<div class="game-launch-card ready"><span class="game-ready-icon">✓</span><h2>Jogo carregado</h2><p>O navegador exige uma interação para iniciar áudio e controles.</p><button class="primary big" type="button" data-game-start>Iniciar jogo</button></div>`;const b=$('[data-game-start]',launch);b.onclick=async()=>{b.disabled=true;try{await startPrepared()}catch(e){b.disabled=false;toast(e?.message||'Não foi possível iniciar',true)}}}
  }
  async function prepareEmulator(mode){
    const core=current.core||coreByPlatform[current.platform];if(!core)throw new Error('Core não configurado para esta plataforma.');const p=preferences();
    const [Nostalgist,romBlob,coreJS,coreWASM]=await Promise.all([ensureRuntime(),romPromise||blobFetch(`/api/v1/games/${current.id}/rom`),blobFetch(`/api/v1/games/runtime/cores/${encodeURIComponent(core)}.js`),blobFetch(`/api/v1/games/runtime/cores/${encodeURIComponent(core)}.wasm`)]);
    const rom=typeof File==='function'?new File([romBlob],current.rom_name||`game.${current.platform}`,{type:'application/octet-stream'}):{fileName:current.rom_name||`game.${current.platform}`,fileContent:romBlob};let state=null,sram=null;if(current.saves?.sram?.exists)sram=await blobFetch(`/api/v1/games/${current.id}/saves/sram`,true);if(mode==='continue'&&current.saves?.state?.exists)state=await blobFetch(`/api/v1/games/${current.id}/saves/state`,true);
    const options={element:canvas,core:{name:core,js:{fileName:`${core}_libretro.js`,fileContent:coreJS},wasm:{fileName:`${core}_libretro.wasm`,fileContent:coreWASM}},rom,cache:{core:true,rom:false,bios:false,shader:false},respondToGlobalEvents:false,retroarchConfig:{savestate_thumbnail_enable:false,input_auto_game_focus:true,input_player1_analog_dpad_mode:1,video_force_aspect:p.video.display!=='stretch',video_smooth:!!p.video.smooth,video_scale_integer:!!p.video.integerScale,rewind_enable:!!p.emulator.rewind,...internalKeyboardConfig,...gamepadConfig()}};
    if(sram)options.sram=sram;if(state)options.state=state;instance=await Nostalgist.prepare(options);
  }
  async function startPrepared(){
    if(!instance||!overlay)throw new Error('Emulador não está preparado.');await instance.start();releaseAllInputs();$('[data-game-launch]',overlay)?.classList.add('hidden');$('[data-game-controls]',overlay)?.classList.remove('hidden');installKeyboardOwner();installViewportOwner();focusGame();startTracking();await heartbeat();window.dispatchEvent(new CustomEvent('stormflix:game-started',{detail:{id:current?.id,platform:current?.platform,core:current?.core}}));queueResize(0);queueResize(160);if(preferences().emulator.fullscreen&&!document.fullscreenElement)void toggleFullscreen();toast('G4 ativo · teclado, gamepad e touch prontos');
  }

  function focusGame(){try{canvas?.focus({preventScroll:true})}catch{canvas?.focus()}}
  function traceInput(button,down,source='api'){const label=$('[data-game-input-label]',overlay);if(label)label.textContent=`Input: ${button.toUpperCase()} ${down?'↓':'↑'} · ${source}`;if(localStorage.getItem('stormflix.games.input-debug')==='1')console.debug(`[StormFlix G4] ${source} ${button} ${down?'DOWN':'UP'}`);window.dispatchEvent(new CustomEvent('stormflix:game-input',{detail:{button,down,source}}))}
  function pressDown(button,source='api'){if(!instance||instance.getStatus?.()==='terminated'||!validInputs.has(button))return false;if(pressedInputs.has(button))return true;try{instance.pressDown({button,player:1});pressedInputs.add(button);traceInput(button,true,source);return true}catch{return false}}
  function pressUp(button,source='api'){if(!instance||instance.getStatus?.()==='terminated'||!validInputs.has(button))return false;try{instance.pressUp({button,player:1});pressedInputs.delete(button);traceInput(button,false,source);return true}catch{return false}}
  function releaseAllInputs(){if(instance){for(const b of retroInputs){try{instance.pressUp({button:b,player:1})}catch{}}}pressedInputs.clear();keyboardPressed.clear()}

  function isInteractiveTarget(target){return target instanceof HTMLInputElement||target instanceof HTMLTextAreaElement||target instanceof HTMLSelectElement||target instanceof HTMLButtonElement||target instanceof HTMLAnchorElement||target?.isContentEditable}
  function menuOpen(){return !!document.querySelector('[data-g4-panel]:not(.hidden)')}
  function keyboardButton(code){const map=preferences().keyboard||{};return Object.keys(map).find(input=>map[input]===code&&validInputs.has(input))||''}
  function handleKeyboard(event,down){
    if(!instance||instance.getStatus?.()!=='running'||menuOpen())return;if(event.code==='Escape'&&down){event.preventDefault();event.stopImmediatePropagation();window.dispatchEvent(new CustomEvent('stormflix:game-menu-request'));return}if(isInteractiveTarget(event.target)&&event.target!==canvas)return;const button=keyboardButton(event.code);if(!button)return;event.preventDefault();event.stopImmediatePropagation();focusGame();if(down){if(event.repeat)return;keyboardPressed.set(event.code,button);pressDown(button,'teclado')}else{const held=keyboardPressed.get(event.code)||button;keyboardPressed.delete(event.code);pressUp(held,'teclado')}
  }
  function installKeyboardOwner(){if(keyboardAbort)keyboardAbort.abort();keyboardAbort=new AbortController();window.addEventListener('keydown',e=>handleKeyboard(e,true),{capture:true,signal:keyboardAbort.signal});window.addEventListener('keyup',e=>handleKeyboard(e,false),{capture:true,signal:keyboardAbort.signal});window.addEventListener('blur',releaseAllInputs,{signal:keyboardAbort.signal});document.addEventListener('visibilitychange',()=>{if(document.hidden)releaseAllInputs()},{signal:keyboardAbort.signal})}

  function applyPresentation(){if(!overlay||!canvas)return;const p=preferences();overlay.dataset.g4Display=p.video.display||'fit';canvas.style.filter=`saturate(${Math.max(50,Math.min(150,Number(p.video.saturation)||100))}%)`;canvas.style.imageRendering=p.video.smooth?'auto':'pixelated';queueResize(30)}
  function resizeEmulator(){if(!instance||!canvas||!overlay||instance.getStatus?.()==='terminated')return false;const stage=$('[data-game-stage]',overlay);if(!stage)return false;const rect=stage.getBoundingClientRect();if(rect.width<2||rect.height<2)return false;const dpr=Math.min(1.5,Math.max(1,Number(devicePixelRatio)||1));let width=Math.round(rect.width*dpr),height=Math.round(rect.height*dpr);const scale=Math.min(1,1920/width,1200/height);width=Math.max(320,Math.round(width*scale));height=Math.max(240,Math.round(height*scale));try{instance.resize({width,height});return true}catch{return false}}
  function queueResize(delay=40){clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{resizeTimer=null;requestAnimationFrame(resizeEmulator)},Math.max(0,delay))}
  function installViewportOwner(){resizeObserver?.disconnect();const stage=$('[data-game-stage]',overlay);if(stage&&'ResizeObserver'in window){resizeObserver=new ResizeObserver(()=>queueResize(20));resizeObserver.observe(stage)}if(viewportAbort)viewportAbort.abort();viewportAbort=new AbortController();window.addEventListener('resize',()=>queueResize(30),{signal:viewportAbort.signal});window.addEventListener('orientationchange',()=>queueResize(180),{signal:viewportAbort.signal});document.addEventListener('fullscreenchange',()=>{queueResize(80);setTimeout(focusGame,90)},{signal:viewportAbort.signal});applyPresentation()}

  function startTracking(){stopTracking();lastTick=performance.now();tickTimer=setInterval(()=>{const now=performance.now(),delta=Math.min(2,Math.max(0,(now-lastTick)/1000));lastTick=now;if(instance?.getStatus?.()==='running'&&!document.hidden)elapsed+=delta},1000);heartbeatTimer=setInterval(heartbeat,15000);const seconds=Math.max(30,Number(preferences().emulator.autoSaveSeconds)||120);autosaveTimer=setInterval(()=>saveAll(false),seconds*1000);gamepadTimer=setInterval(updateGamepadLabel,900)}
  function stopTracking(){for(const id of [tickTimer,heartbeatTimer,autosaveTimer,gamepadTimer])if(id)clearInterval(id);tickTimer=heartbeatTimer=autosaveTimer=gamepadTimer=null}
  async function heartbeat(){if(!current||!sessionID)return;try{const data=await jsonFetch(`/api/v1/games/${current.id}/playback`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionID,elapsed_seconds:Math.floor(elapsed)})});current.play_seconds=Number(data.play_seconds||current.play_seconds||0);const label=$('[data-playtime]',overlay);if(label)label.textContent=`Tempo deste perfil: ${clock(current.play_seconds)}`}catch{}}
  async function uploadSave(kind,blob){if(!blob||!blob.size)return null;const r=await fetch(`/api/v1/games/${current.id}/saves/${kind}`,{method:'PUT',credentials:'same-origin',headers:{'Content-Type':'application/octet-stream'},body:blob});const text=await r.text();let data={};try{data=text?JSON.parse(text):{}}catch{}if(!r.ok)throw new Error(data.error||`Falha ao salvar ${kind}`);return data}
  async function saveAll(manual){if(savePromise){try{await savePromise}catch{}if(!manual)return}if(!instance||!current||instance.getStatus?.()==='terminated')return;if(manual)toast('Salvando estado e SRAM…');const target=instance,game=current;savePromise=(async()=>{const errors=[];let stateInfo=null,sramInfo=null;try{const result=await target.saveState();if(result?.state?.size&&current===game)stateInfo=await uploadSave('state',result.state)}catch(e){errors.push(`estado: ${e.message}`)}try{const sram=await target.saveSRAM();if(sram?.size&&current===game)sramInfo=await uploadSave('sram',sram)}catch(e){errors.push(`SRAM: ${e.message}`)}if(current===game){current.saves=current.saves||{};if(stateInfo)current.saves.state=stateInfo;if(sramInfo)current.saves.sram=sramInfo}if(manual&&overlay)toast(errors.length?`Save parcial · ${errors.join(' · ')}`:'Save sincronizado com o perfil ✓',errors.length>0)})();try{return await savePromise}finally{savePromise=null}}

  async function togglePause(){if(!instance)return;const b=$('[data-game-pause]',overlay);try{if(instance.getStatus?.()==='paused'){await instance.resume();if(b)b.textContent='Pausar';lastTick=performance.now();focusGame();toast('Jogo retomado')}else{releaseAllInputs();await instance.pause();if(b)b.textContent='Continuar';await heartbeat();toast('Jogo pausado')}}catch(e){toast(e.message,true)}}
  async function toggleFullscreen(){try{if(document.fullscreenElement)await document.exitFullscreen();else await overlay?.requestFullscreen?.();setTimeout(()=>{queueResize(0);focusGame()},80)}catch{toast('Tela cheia não disponível neste navegador',true)}}
  async function close(saveFirst){if(closing)return;closing=true;stopTracking();releaseAllInputs();keyboardAbort?.abort();keyboardAbort=null;viewportAbort?.abort();viewportAbort=null;resizeObserver?.disconnect();resizeObserver=null;clearTimeout(resizeTimer);try{if(saveFirst&&instance){toast('Salvando antes de sair…');await saveAll(true)}else if(savePromise){try{await savePromise}catch{}}await heartbeat();if(instance){try{instance.exit()}catch{}}}finally{instance=null;current=null;preparePromise=null;romPromise=null;savePromise=null;sessionID='';elapsed=0;if(overlay){overlay.remove();overlay=null;canvas=null}closing=false;window.dispatchEvent(new CustomEvent('stormflix:game-closed'))}}

  function updateGamepadLabel(){if(!overlay)return;const pads=[...(navigator.getGamepads?.()||[])].filter(Boolean),label=$('[data-gamepad-label]',overlay),dot=$('[data-gamepad-dot]',overlay);if(label)label.textContent=pads.length?`${pads.length} gamepad(s) · ${pads[0].id||'controle conectado'}`:'Teclado pronto · nenhum gamepad';if(dot)dot.classList.toggle('online',pads.length>0);window.dispatchEvent(new CustomEvent('stormflix:gamepads-changed',{detail:pads.map(p=>({id:p.id,index:p.index,mapping:p.mapping}))}))}
  function pauseOtherMedia(){try{const v=document.querySelector('#player');if(v&&!v.paused)v.pause()}catch{}try{const a=document.querySelector('#music-audio');if(a&&!a.paused)a.pause()}catch{}try{if(typeof stopTheme==='function')stopTheme()}catch{}}
  function toast(message,error=false){const node=$('[data-game-toast]',overlay);if(!node)return;node.textContent=message;node.classList.toggle('error',!!error);node.classList.remove('hidden');clearTimeout(node._timer);node._timer=setTimeout(()=>node.classList.add('hidden'),error?5000:2600)}
  function notify(message){if(typeof sfToast==='function')sfToast(message);else console.warn(message)}

  addEventListener('gamepadconnected',updateGamepadLabel);addEventListener('gamepaddisconnected',updateGamepadLabel);document.addEventListener('visibilitychange',()=>{if(document.hidden&&instance)saveAll(false)});addEventListener('beforeunload',()=>{if(!current||!sessionID||!navigator.sendBeacon)return;const body=new Blob([JSON.stringify({session_id:sessionID,elapsed_seconds:Math.floor(elapsed)})],{type:'application/json'});navigator.sendBeacon(`/api/v1/games/${current.id}/playback`,body)});

  window.StormFlixGamePlayer={
    open,close:()=>close(true),active:()=>!!instance,pressDown:(b)=>pressDown(b,'virtual'),pressUp:(b)=>pressUp(b,'virtual'),resize:()=>queueResize(0),focus:focusGame,save:()=>saveAll(true),pause:togglePause,fullscreen:toggleFullscreen,
    preferences,patchPreferences,resetPreferences,defaultPreferences:()=>clone(defaultPrefs),gamepads:()=>[...(navigator.getGamepads?.()||[])].filter(Boolean),
    current:()=>current?{id:current.id,title:current.title,platform:current.platform,core:current.core}:null,
  };
})();
