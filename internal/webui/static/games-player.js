/* StormFlix Games G2: pinned Nostalgist/RetroArch browser player + profile saves. */
(function(){
  const RUNTIME_LABEL='Nostalgist 0.21.1 · RetroArch cores v1.22.2';
  const coreByPlatform={nes:'fceumm',snes:'snes9x',genesis:'genesis_plus_gx',gb:'mgba',gbc:'mgba',gba:'mgba'};
  let overlay=null,canvas=null,instance=null,current=null,prepareMode='normal',preparePromise=null;
  let sessionID='',elapsed=0,lastTick=0,tickTimer=null,heartbeatTimer=null,autosaveTimer=null;
  let savePromise=null,closing=false,scriptPromise=null,romPromise=null;
  const $=(sel,root=document)=>root.querySelector(sel);
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const platformLabel=p=>({nes:'Nintendo Entertainment System',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'}[p]||String(p||'').toUpperCase());
  const fmtBytes=n=>{n=Number(n||0);if(n<1024)return `${n} B`;if(n<1048576)return `${(n/1024).toFixed(1)} KB`;return `${(n/1048576).toFixed(1)} MB`};
  const clock=s=>{s=Math.max(0,Math.floor(Number(s)||0));const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${m} min`};

  function randomSession(){
    if(globalThis.crypto?.randomUUID)return crypto.randomUUID();
    return `sf-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}-${Math.random().toString(36).slice(2)}`;
  }

  async function jsonFetch(url,options){
    const response=await fetch(url,{credentials:'same-origin',...(options||{})});
    const text=await response.text();let data={};try{data=text?JSON.parse(text):{}}catch{}
    if(!response.ok)throw new Error(data.error||`HTTP ${response.status}`);
    return data;
  }

  async function blobFetch(url,optional=false){
    const response=await fetch(url,{credentials:'same-origin',cache:'no-store'});
    if(optional&&response.status===404)return null;
    if(!response.ok){let message=`HTTP ${response.status}`;try{const data=await response.json();message=data.error||message}catch{}throw new Error(message)}
    return response.blob();
  }

  function ensureRuntime(){
    if(globalThis.Nostalgist)return Promise.resolve(globalThis.Nostalgist);
    if(scriptPromise)return scriptPromise;
    scriptPromise=new Promise((resolve,reject)=>{
      const existing=document.querySelector('script[data-stormflix-nostalgist]');
      if(existing){existing.addEventListener('load',()=>resolve(globalThis.Nostalgist),{once:true});existing.addEventListener('error',()=>reject(new Error('Falha ao carregar runtime de jogos')),{once:true});return}
      const script=document.createElement('script');script.src='/api/v1/games/runtime/nostalgist.js';script.async=true;script.dataset.stormflixNostalgist='1';
      script.onload=()=>globalThis.Nostalgist?resolve(globalThis.Nostalgist):reject(new Error('Runtime Nostalgist não inicializou'));
      script.onerror=()=>reject(new Error('Não foi possível preparar o runtime de jogos'));
      document.head.appendChild(script);
    });
    return scriptPromise;
  }

  function createOverlay(game){
    if(overlay)overlay.remove();
    overlay=document.createElement('section');overlay.id='game-player-overlay';overlay.className='game-player-overlay';overlay.setAttribute('role','dialog');overlay.setAttribute('aria-modal','true');overlay.setAttribute('aria-label',`Jogando ${game.title}`);
    overlay.innerHTML=`<header class="game-player-top"><div><span class="game-player-kicker">STORMFLIX GAME PLAYER G2</span><strong data-game-player-title>${esc(game.title)}</strong></div><div class="game-player-top-actions"><span class="game-runtime-badge">${esc(RUNTIME_LABEL)}</span><button type="button" data-game-fullscreen aria-label="Tela cheia">⛶</button><button type="button" data-game-close aria-label="Sair do jogo">✕</button></div></header>
      <div class="game-player-stage"><canvas id="stormflix-game-canvas" class="game-player-canvas" tabindex="0"></canvas><div class="game-launch-panel" data-game-launch></div><div class="game-player-toast hidden" data-game-toast></div></div>
      <footer class="game-player-controls hidden" data-game-controls><div class="game-player-status"><span class="gamepad-dot" data-gamepad-dot></span><span data-gamepad-label>Nenhum gamepad detectado · teclado ativo</span><span data-playtime>Tempo deste perfil: ${clock(game.play_seconds)}</span></div><div class="game-player-buttons"><button type="button" data-game-pause>Pausar</button><button type="button" data-game-save>Salvar agora</button><button type="button" data-game-exit>Salvar e sair</button></div></footer>`;
    document.body.appendChild(overlay);canvas=$('#stormflix-game-canvas',overlay);
    $('[data-game-close]',overlay).onclick=()=>close(true);
    $('[data-game-exit]',overlay).onclick=()=>close(true);
    $('[data-game-fullscreen]',overlay).onclick=toggleFullscreen;
    $('[data-game-pause]',overlay).onclick=togglePause;
    $('[data-game-save]',overlay).onclick=()=>saveAll(true);
    overlay.addEventListener('keydown',captureGameKeys,true);
    updateGamepadLabel();
  }

  function captureGameKeys(event){
    if(!overlay)return;
    if(['ArrowUp','ArrowDown','ArrowLeft','ArrowRight','Enter',' ','Escape','Shift','z','x','a','s','Z','X','A','S'].includes(event.key))event.stopPropagation();
    if(event.key==='Escape'){event.preventDefault();close(true)}
  }

  function launchHTML(game){
    const state=game.saves?.state||{},sram=game.saves?.sram||{};
    const stateMeta=state.exists?`Save state v${Number(state.version||0)} · ${fmtBytes(state.size_bytes)}`:'Sem save state';
    const sramMeta=sram.exists?`SRAM v${Number(sram.version||0)} · ${fmtBytes(sram.size_bytes)}`:'Sem SRAM salvo';
    return `<div class="game-launch-card"><p>${esc(platformLabel(game.platform))}</p><h2>${esc(game.title)}</h2><div class="game-launch-facts"><span>${esc(game.rom_name||'ROM')}</span><span>${fmtBytes(game.rom_size_bytes)}</span><span>${esc(game.core||coreByPlatform[game.platform]||'core')}</span></div><div class="game-save-summary"><span>${esc(stateMeta)}</span><span>${esc(sramMeta)}</span></div><p class="game-launch-note">Na primeira execução deste core, o servidor baixa a versão fixada e mantém uma cópia local. ROM e saves nunca são enviados ao CDN.</p><div class="game-launch-actions">${state.exists?'<button class="primary" type="button" data-game-mode="continue">Continuar do save state</button><button type="button" data-game-mode="normal">Abrir normalmente (SRAM)</button>':'<button class="primary" type="button" data-game-mode="normal">Preparar jogo</button>'}</div><small>Gamepad USB/Bluetooth é reconhecido pelo RetroArch. Teclado: setas, Enter/Shift e Z/X/A/S.</small></div>`;
  }

  async function open(gameInput){
    if(closing)return;
    if(instance||overlay)await close(false);
    pauseOtherMedia();
    let game;
    try{game=await jsonFetch(`/api/v1/games/${Number(gameInput.id||gameInput)}`)}catch(err){notify(err.message);return}
    if(!game.playable||!game.core){notify('Este jogo não está disponível na matriz G2 do navegador.');return}
    current=game;sessionID=randomSession();elapsed=0;preparePromise=null;prepareMode='normal';savePromise=null;closing=false;
    createOverlay(game);
    const launch=$('[data-game-launch]',overlay);launch.innerHTML=launchHTML(game);
    launch.querySelectorAll('[data-game-mode]').forEach(button=>button.onclick=()=>prepare(button.dataset.gameMode));
    romPromise=blobFetch(`/api/v1/games/${game.id}/rom`);
    ensureRuntime().catch(()=>{});
    $('[data-game-close]',overlay)?.focus();
  }

  async function prepare(mode){
    if(!current||preparePromise)return;
    prepareMode=mode==='continue'?'continue':'normal';
    const launch=$('[data-game-launch]',overlay);launch.innerHTML=`<div class="game-launch-card loading"><span class="game-loader"></span><h2>Preparando ${esc(current.title)}…</h2><p>Carregando ROM autenticada, runtime local e ${prepareMode==='continue'?'seu save state':'seu SRAM'}.</p><small>O primeiro uso de um core pode demorar mais; depois ele fica no cache do servidor.</small></div>`;
    preparePromise=prepareEmulator(prepareMode);
    try{
      await preparePromise;
      launch.innerHTML=`<div class="game-launch-card ready"><span class="game-ready-icon">✓</span><h2>Pronto para jogar</h2><p>${prepareMode==='continue'?'Seu save state foi carregado.':'O jogo será iniciado normalmente, preservando o SRAM existente.'}</p><button class="primary big" type="button" data-game-start>Iniciar agora</button><small>Use este clique para liberar áudio e controles no navegador.</small></div>`;
      $('[data-game-start]',launch).onclick=startPrepared;
      $('[data-game-start]',launch).focus();
    }catch(err){
      preparePromise=null;
      launch.innerHTML=`<div class="game-launch-card error"><span class="game-ready-icon">!</span><h2>Não foi possível preparar o jogo</h2><p>${esc(err.message)}</p><button type="button" data-game-retry>Tentar novamente</button></div>`;
      $('[data-game-retry]',launch).onclick=()=>prepare(mode);
    }
  }

  async function prepareEmulator(mode){
    const [Nostalgist,romBlob]=await Promise.all([ensureRuntime(),romPromise||blobFetch(`/api/v1/games/${current.id}/rom`)]);
    const core=current.core||coreByPlatform[current.platform];if(!core)throw new Error('Core não configurado para esta plataforma.');
    const rom=typeof File==='function'?new File([romBlob],current.rom_name||`game.${current.platform}`,{type:'application/octet-stream'}):{fileName:current.rom_name||`game.${current.platform}`,fileContent:romBlob};
    let state=null,sram=null;
    if(current.saves?.sram?.exists)sram=await blobFetch(`/api/v1/games/${current.id}/saves/sram`,true);
    if(mode==='continue'&&current.saves?.state?.exists)state=await blobFetch(`/api/v1/games/${current.id}/saves/state`,true);
    const options={
      element:canvas,
      core:{name:core,js:`/api/v1/games/runtime/cores/${core}.js`,wasm:`/api/v1/games/runtime/cores/${core}.wasm`},
      rom,
      cache:{core:true,rom:false,bios:false,shader:false},
      respondToGlobalEvents:false,
      retroarchConfig:{savestate_thumbnail_enable:false},
    };
    if(sram)options.sram=sram;
    if(state)options.state=state;
    instance=await Nostalgist.prepare(options);
  }

  async function startPrepared(){
    if(!instance||!overlay)return;
    const button=$('[data-game-start]',overlay);if(button){button.disabled=true;button.textContent='Iniciando…'}
    try{
      await instance.start();
      $('[data-game-launch]',overlay)?.classList.add('hidden');
      $('[data-game-controls]',overlay)?.classList.remove('hidden');
      canvas?.focus();
      startTracking();
      await heartbeat();
      toast('Jogo iniciado · saves sincronizam com este perfil');
    }catch(err){
      if(button){button.disabled=false;button.textContent='Tentar iniciar novamente'}
      toast(err.message,true);
    }
  }

  function startTracking(){
    stopTracking();lastTick=performance.now();
    tickTimer=setInterval(()=>{
      const now=performance.now();const delta=Math.min(2,Math.max(0,(now-lastTick)/1000));lastTick=now;
      if(instance?.getStatus?.()==='running'&&!document.hidden)elapsed+=delta;
    },1000);
    heartbeatTimer=setInterval(heartbeat,15000);
    autosaveTimer=setInterval(()=>saveAll(false),120000);
  }

  function stopTracking(){
    if(tickTimer)clearInterval(tickTimer);if(heartbeatTimer)clearInterval(heartbeatTimer);if(autosaveTimer)clearInterval(autosaveTimer);
    tickTimer=heartbeatTimer=autosaveTimer=null;
  }

  async function heartbeat(){
    if(!current||!sessionID)return;
    try{
      const data=await jsonFetch(`/api/v1/games/${current.id}/playback`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionID,elapsed_seconds:Math.floor(elapsed)})});
      current.play_seconds=Number(data.play_seconds||current.play_seconds||0);
      const label=$('[data-playtime]',overlay);if(label)label.textContent=`Tempo deste perfil: ${clock(current.play_seconds)}`;
    }catch{}
  }

  async function uploadSave(kind,blob){
    if(!blob||!blob.size)return null;
    const response=await fetch(`/api/v1/games/${current.id}/saves/${kind}`,{method:'PUT',credentials:'same-origin',headers:{'Content-Type':'application/octet-stream'},body:blob});
    const text=await response.text();let data={};try{data=text?JSON.parse(text):{}}catch{}
    if(!response.ok)throw new Error(data.error||`Falha ao salvar ${kind}`);
    return data;
  }

  async function saveAll(manual){
    if(savePromise){
      try{await savePromise}catch{}
      if(!manual)return;
    }
    if(!instance||!current||instance.getStatus?.()==='terminated')return;
    if(manual)toast('Salvando estado e SRAM…');
    const targetInstance=instance,targetGame=current;
    savePromise=(async()=>{
      const errors=[];let stateInfo=null,sramInfo=null;
      try{const result=await targetInstance.saveState();if(result?.state?.size&&current===targetGame)stateInfo=await uploadSave('state',result.state)}catch(err){errors.push(`estado: ${err.message}`)}
      try{const sram=await targetInstance.saveSRAM();if(sram?.size&&current===targetGame)sramInfo=await uploadSave('sram',sram)}catch(err){errors.push(`SRAM: ${err.message}`)}
      if(current===targetGame){
        if(stateInfo){current.saves=current.saves||{};current.saves.state=stateInfo}
        if(sramInfo){current.saves=current.saves||{};current.saves.sram=sramInfo}
      }
      if(manual&&overlay){if(errors.length)toast(`Save parcial · ${errors.join(' · ')}`,true);else toast('Save sincronizado com o perfil ✓')}
    })();
    try{return await savePromise}finally{savePromise=null}
  }

  async function togglePause(){
    if(!instance)return;const button=$('[data-game-pause]',overlay);
    try{
      if(instance.getStatus?.()==='paused'){await instance.resume();if(button)button.textContent='Pausar';lastTick=performance.now();toast('Jogo retomado')}
      else{await instance.pause();if(button)button.textContent='Continuar';await heartbeat();toast('Jogo pausado')}
    }catch(err){toast(err.message,true)}
  }

  async function toggleFullscreen(){
    try{
      if(document.fullscreenElement)await document.exitFullscreen();else await overlay?.requestFullscreen?.();
    }catch(err){toast('Tela cheia não disponível neste navegador',true)}
  }

  async function close(saveFirst){
    if(closing)return;closing=true;
    stopTracking();
    try{
      if(saveFirst&&instance){toast('Salvando antes de sair…');await saveAll(true)}
      else if(savePromise){try{await savePromise}catch{}}
      await heartbeat();
      if(instance){try{await instance.exit()}catch{}}
    }finally{
      instance=null;current=null;preparePromise=null;romPromise=null;savePromise=null;sessionID='';elapsed=0;
      if(overlay){overlay.remove();overlay=null;canvas=null}
      closing=false;
      window.dispatchEvent(new CustomEvent('stormflix:game-closed'));
    }
  }

  function pauseOtherMedia(){
    try{const video=document.querySelector('#player');if(video&&!video.paused)video.pause()}catch{}
    try{const audio=document.querySelector('#music-audio');if(audio&&!audio.paused)audio.pause()}catch{}
    try{if(typeof stopTheme==='function')stopTheme()}catch{}
  }

  function toast(message,error=false){
    const node=$('[data-game-toast]',overlay);if(!node)return;node.textContent=message;node.classList.toggle('error',!!error);node.classList.remove('hidden');clearTimeout(node._timer);node._timer=setTimeout(()=>node.classList.add('hidden'),error?5000:2600);
  }

  function notify(message){if(typeof sfToast==='function')sfToast(message);else console.warn(message)}

  function updateGamepadLabel(){
    if(!overlay)return;const pads=navigator.getGamepads?.()||[];const count=[...pads].filter(Boolean).length;
    const label=$('[data-gamepad-label]',overlay),dot=$('[data-gamepad-dot]',overlay);if(label)label.textContent=count?`${count} gamepad(s) conectado(s) · teclado também ativo`:'Nenhum gamepad detectado · teclado ativo';if(dot)dot.classList.toggle('online',count>0);
  }

  addEventListener('gamepadconnected',updateGamepadLabel);addEventListener('gamepaddisconnected',updateGamepadLabel);
  document.addEventListener('visibilitychange',()=>{if(document.hidden&&instance)saveAll(false)});
  addEventListener('beforeunload',()=>{
    if(!current||!sessionID||!navigator.sendBeacon)return;
    const body=new Blob([JSON.stringify({session_id:sessionID,elapsed_seconds:Math.floor(elapsed)})],{type:'application/json'});
    navigator.sendBeacon(`/api/v1/games/${current.id}/playback`,body);
  });

  window.StormFlixGamePlayer={open,close:()=>close(true),active:()=>!!instance};
})();
