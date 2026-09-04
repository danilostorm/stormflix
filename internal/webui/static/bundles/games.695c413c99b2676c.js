/* StormFlix source: games-instant-cache.js */
/* StormFlix Games instant data cache.
 * Native movie/series/anime Home always gets first paint/network priority.
 * Games is warmed immediately afterwards and reused from memory on navigation.
 */
(function(){
  'use strict';
  if(typeof window.request!=='function'||window.__sfGamesInstantCache)return;
  window.__sfGamesInstantCache=true;

  const baseRequest=window.request;
  const baseFetch=window.fetch.bind(window);
  const cache=new Map();
  const ttl=45000;
  const warmPaths=['/games/home','/games?limit=500'];
  let warmTimer=0,observer=null;

  function nativeRowsReady(){
    if(!document.querySelector('[data-nav="home"]')?.classList.contains('active'))return false;
    if(document.body.classList.contains('games-mode'))return false;
    return document.querySelectorAll('#rows > .content-row:not([data-g44-home-row])').length>=2;
  }

  function isBackgroundGamesHome(input,init={}){
    if(document.body.classList.contains('games-mode'))return false;
    const method=String(init?.method||'GET').toUpperCase();if(method!=='GET')return false;
    const raw=typeof input==='string'?input:String(input?.url||'');
    try{
      const parsed=new URL(raw,location.origin);
      return parsed.origin===location.origin&&parsed.pathname==='/api/v1/games/home';
    }catch{return false}
  }

  function waitForNativeHome(maxWait=3500){
    if(nativeRowsReady())return Promise.resolve();
    return new Promise(resolve=>{
      let done=false;
      const finish=()=>{if(done)return;done=true;clearTimeout(timeout);window.removeEventListener('stormflix:native-home-ready',finish);resolve()};
      const timeout=setTimeout(finish,maxWait);
      window.addEventListener('stormflix:native-home-ready',finish,{once:true});
      const tick=()=>{if(done)return;if(nativeRowsReady())finish();else requestAnimationFrame(tick)};requestAnimationFrame(tick);
    });
  }

  /* games-g44 uses fetch() directly for Home rails. Hold only that background
   * request until native Home has painted, so Games never competes with the
   * first /home SQLite/render path. Explicit Jogos navigation bypasses this. */
  window.fetch=function(input,init){
    if(isBackgroundGamesHome(input,init)&&!nativeRowsReady())return waitForNativeHome().then(()=>baseFetch(input,init));
    return baseFetch(input,init);
  };

  function isCacheable(path,opt={}){
    const method=String(opt.method||'GET').toUpperCase();
    return method==='GET'&&warmPaths.includes(String(path||''));
  }
  function remember(path,value){cache.set(path,{value,at:Date.now(),promise:null});return value}
  function peek(path){const entry=cache.get(path);return entry?.value&&Date.now()-entry.at<ttl?entry.value:null}
  function cachedRequest(path,opt={}){
    if(!isCacheable(path,opt))return baseRequest(path,opt);
    const entry=cache.get(path),now=Date.now();
    if(entry?.value&&now-entry.at<ttl)return Promise.resolve(entry.value);
    if(entry?.promise)return entry.promise;
    const promise=baseRequest(path,opt).then(value=>remember(path,value)).catch(err=>{cache.delete(path);throw err});
    cache.set(path,{value:entry?.value||null,at:entry?.at||0,promise});
    return promise;
  }
  window.request=cachedRequest;

  function warm(){
    clearTimeout(warmTimer);
    if(!nativeRowsReady())return;
    warmTimer=setTimeout(()=>Promise.allSettled(warmPaths.map(path=>cachedRequest(path))),20);
  }
  function invalidate(){cache.clear()}
  function invalidateHome(){cache.delete('/games/home');cache.delete('/games?limit=500')}

  function observe(){
    const rows=document.querySelector('#rows');if(!rows||observer)return;
    observer=new MutationObserver(()=>{if(nativeRowsReady())warm()});
    observer.observe(rows,{childList:true});
    if(nativeRowsReady())warm();
  }

  window.addEventListener('stormflix:native-home-ready',warm);
  window.addEventListener('stormflix:profile',()=>{invalidate();setTimeout(warm,120)});
  window.addEventListener('stormflix:game-closed',()=>{invalidateHome();setTimeout(warm,80)});
  document.addEventListener('visibilitychange',()=>{if(!document.hidden&&nativeRowsReady())warm()});
  window.sfGamesInstantCache={warm,invalidate,peek};

  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',observe,{once:true});else observe();
})();

/* StormFlix source: games-ui.js */
/*
 * StormFlix Games G2.5 UI.
 * Visual interaction model adapted from RomMix (MIT, Copyright 2026 Benjamin Leclerc).
 * StormFlix catalog, authorization, saves and browser playback remain native.
 */
(function(){
  const root=document.querySelector('#games-view');
  const nav=document.querySelector('#games-nav');
  if(!root||!nav)return;

  const labels={nes:'Nintendo Entertainment System',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'};
  const short={nes:'NES',snes:'SNES',genesis:'GEN',gb:'GB',gbc:'GBC',gba:'GBA'};
  const cores={nes:'fceumm',snes:'snes9x',genesis:'genesis_plus_gx',gb:'mgba',gbc:'mgba',gba:'mgba'};
  let home=null,games=[],saves=null,screen='home',query='',platform='',loading=false,selected=0;
  let scale=localStorage.getItem('stormflix.games.scale')||'auto';
  let sounds=localStorage.getItem('stormflix.games.navsounds')!=='off';
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const attr=s=>esc(s).replace(/`/g,'&#96;');
  const time=s=>{s=Math.max(0,Math.floor(Number(s)||0));const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${m} min`};
  const bytes=n=>{n=Number(n)||0;if(n<1024)return`${n} B`;if(n<1048576)return`${(n/1024).toFixed(1)} KB`;return`${(n/1048576).toFixed(1)} MB`};

  nav.addEventListener('click',openGames);
  document.querySelector('#brand-home')?.addEventListener('click',()=>closeGames(false));
  document.querySelector('.main-nav')?.addEventListener('click',e=>{const b=e.target.closest('button');if(b&&b!==nav)closeGames(false)});
  window.addEventListener('stormflix:profile',()=>{if(document.body.classList.contains('games-mode'))load()});
  window.addEventListener('stormflix:game-closed',()=>{if(document.body.classList.contains('games-mode'))load(false)});

  async function openGames(){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    if(typeof stopTheme==='function')stopTheme();
    document.querySelector('#hero')?.classList.add('hidden');
    document.querySelector('#search-view')?.classList.add('hidden');
    document.querySelector('#catalog-view')?.classList.add('hidden');
    document.querySelector('#music-view')?.classList.add('hidden');
    root.classList.remove('hidden');document.body.classList.add('games-mode');
    applyScale();window.scrollTo({top:0,behavior:'auto'});
    await load(true);
  }

  function closeGames(goHome=true){
    document.body.classList.remove('games-mode');root.classList.add('hidden');
    if(goHome){document.querySelector('#catalog-view')?.classList.remove('hidden');document.querySelector('[data-nav="home"]')?.click()}
  }

  async function load(reset=false){
    if(loading)return;loading=true;
    if(reset){screen='home';query='';platform='';saves=null}
    root.innerHTML='<div class="gx-loading"><span></span><b>Carregando sua biblioteca…</b><small>StormFlix Games</small></div>';
    try{
      [home,games]=await Promise.all([request('/games/home'),request('/games?limit=500')]);
      render();
    }catch(err){root.innerHTML=`<div class="gx-loading gx-error"><b>Não foi possível abrir Games</b><small>${esc(err.message)}</small><button type="button" data-gx-back>Voltar ao StormFlix</button></div>`;root.querySelector('[data-gx-back]').onclick=()=>closeGames(true)}
    finally{loading=false}
  }

  function render(){
    const current=screen;
    root.innerHTML=`<div class="gx-app">
      <header class="gx-topbar">
        <button class="gx-brand" type="button" data-gx-home><span class="gx-logo-mark">▦</span><strong>Storm<span>Games</span></strong></button>
        <nav class="gx-mainnav" aria-label="Games"><button class="${current==='home'?'active':''}" data-gx-screen="home">⌂ <span>Início</span></button><button class="${current==='library'?'active':''}" data-gx-screen="library">▦ <span>Biblioteca</span></button><button class="${current==='collections'?'active':''}" data-gx-screen="collections">▣ <span>Coleções</span></button><button class="${current==='saves'?'active':''}" data-gx-screen="saves">▤ <span>Saves</span>${saveCount()?`<i>${saveCount()}</i>`:''}</button><button class="${current==='emulators'?'active':''}" data-gx-screen="emulators">⌘ <span>Emuladores</span></button><button class="${current==='settings'?'active':''}" data-gx-screen="settings">⚙ <span>Configurações</span></button></nav>
        <div class="gx-user"><span>${esc(window.sfProfiles?.current?.()?.name||'Perfil')}</span><button type="button" data-gx-exit>← StormFlix</button></div>
      </header>
      <main class="gx-content">${screenHTML()}</main>
      <footer class="gx-hints"><span><kbd>Enter</kbd> Abrir</span><span><kbd>/</kbd> Buscar</span><span><kbd>Esc</kbd> Voltar</span></footer>
    </div>`;
    bindShell();
  }

  function screenHTML(){
    if(!games.length)return emptyHTML();
    if(screen==='library')return libraryHTML();
    if(screen==='collections')return collectionsHTML();
    if(screen==='saves')return savesHTML();
    if(screen==='emulators')return emulatorsHTML();
    if(screen==='settings')return settingsHTML();
    return homeHTML();
  }

  function emptyHTML(){return`<section class="gx-empty"><span class="gx-logo-mark huge">▦</span><h1>Sua biblioteca de Games está vazia</h1><p>No Admin → Games, crie uma biblioteca de ROMs e acompanhe o scan. O StormFlix não fornece ROMs ou BIOS.</p><a href="/admin/">Abrir Games Admin</a></section>`}

  function heroGame(){return(home?.continue_playing||[])[0]||(home?.recently_added||[])[0]||games[0]}
  function homeHTML(){
    const hero=heroGame();const continued=home?.continue_playing||[],favorites=home?.favorites||[],recent=home?.recently_added||[];
    return `<section class="gx-home">${heroHTML(hero,continued.length?'CONTINUAR JOGANDO':'ADICIONADO RECENTEMENTE')}
      ${continued.length?rowHTML('Continuar jogando',continued):''}
      ${rowHTML('Prontos para jogar',games.filter(g=>cores[g.platform]).slice(0,30))}
      ${favorites.length?rowHTML('Favoritos',favorites):''}
      ${recent.length?rowHTML('Adicionados recentemente',recent):''}
    </section>`;
  }

  function heroHTML(g,reason){
    if(!g)return'';const cover=coverHTML(g,true);const year=g.release_year?`<span>◷ ${g.release_year}</span>`:'';
    return `<button class="gx-hero" type="button" data-game-open="${Number(g.id)}" style="--gx-hero-image:${g.cover_url?`url('${attr(g.cover_url)}')`:'none'}"><div class="gx-hero-wash"></div><div class="gx-hero-cover">${cover}</div><div class="gx-hero-copy"><p>${esc(reason)}</p><h1>${esc(g.title)}</h1><div class="gx-hero-meta"><span class="system">${esc(short[g.platform]||g.platform)}</span><span>${esc(labels[g.platform]||g.platform)}</span>${year}${g.play_seconds?`<span>◷ ${esc(time(g.play_seconds))}</span>`:''}</div><p class="gx-hero-summary">${esc(g.overview||'Jogo identificado localmente pelo StormFlix e pronto para sua biblioteca de saves.')}</p><small>Pressione Enter para abrir</small></div></button>`;
  }

  function rowHTML(title,items){return`<section class="gx-section"><h2>${esc(title)}</h2><div class="gx-row">${items.map(cardHTML).join('')}</div></section>`}
  function cardHTML(g){return`<article class="gx-card"><button type="button" data-game-open="${Number(g.id)}"><span class="gx-cover">${coverHTML(g)}${g.last_played_at?'<i class="gx-ready-dot"></i>':''}</span><strong>${esc(g.title)}</strong><small><b>${esc(short[g.platform]||String(g.platform||'').toUpperCase())}</b> ${esc(labels[g.platform]||g.platform||'')}</small></button><button class="gx-heart ${g.favorite?'on':''}" type="button" data-game-favorite="${Number(g.id)}">${g.favorite?'♥':'♡'}</button></article>`}
  function coverHTML(g,large=false){return g.cover_url?`<img src="${attr(g.cover_url)}" alt="" loading="${large?'eager':'lazy'}">`:`<span class="gx-cover-fallback"><b>${esc(short[g.platform]||'GAME')}</b><i>STORMFLIX</i></span>`}

  function libraryHTML(){
    const filtered=games.filter(g=>(!platform||g.platform===platform)&&(!query||String(g.title||'').toLocaleLowerCase('pt-BR').includes(query.toLocaleLowerCase('pt-BR'))));
    return `<section class="gx-page"><div class="gx-page-head"><div><p>SEU CATÁLOGO</p><h1>Biblioteca</h1></div><label class="gx-search"><span>⌕</span><input data-gx-search value="${attr(query)}" placeholder="Buscar jogo…" autofocus></label></div><div class="gx-filters"><button class="${!platform?'active':''}" data-gx-platform="">Todos <i>${games.length}</i></button>${(home?.platforms||[]).map(p=>`<button class="${platform===p.platform?'active':''}" data-gx-platform="${attr(p.platform)}">${esc(short[p.platform]||p.platform)} <i>${p.count}</i></button>`).join('')}</div><div class="gx-grid">${filtered.map(cardHTML).join('')||'<div class="gx-empty small"><h2>Nenhum jogo encontrado</h2></div>'}</div></section>`;
  }

  function collectionsHTML(){
    const groups=(home?.platforms||[]).map(p=>({p,items:games.filter(g=>g.platform===p.platform)}));
    return `<section class="gx-page"><div class="gx-page-head"><div><p>PLATAFORMAS</p><h1>Coleções</h1></div></div><div class="gx-collection-grid">${groups.map(({p,items})=>{const arts=items.slice(0,4);return`<button type="button" data-gx-collection="${attr(p.platform)}"><span class="gx-collection-art">${arts.map(g=>`<i>${coverHTML(g)}</i>`).join('')}</span><strong>${esc(p.label)}</strong><small>${p.count} jogo(s)</small></button>`}).join('')}</div></section>`;
  }

  function saveCount(){return Array.isArray(saves)?saves.length:0}
  function savesHTML(){
    if(saves===null){setTimeout(loadSaves,0);return`<section class="gx-page"><div class="gx-page-head"><div><p>SEU PERFIL</p><h1>Saves</h1></div></div><div class="gx-inline-loader"><span></span>Carregando saves…</div></section>`}
    const byGame=new Map();for(const item of saves){const group=byGame.get(item.game_id)||{...item,items:[]};group.items.push(item);byGame.set(item.game_id,group)}
    return `<section class="gx-page"><div class="gx-page-head"><div><p>SINCRONIZADOS COM ESTE PERFIL</p><h1>Saves</h1><small>Save states e SRAM ficam separados por perfil e possuem recovery no servidor.</small></div></div><div class="gx-save-grid">${[...byGame.values()].map(g=>`<button type="button" data-game-open="${g.game_id}"><span class="gx-save-cover">${g.cover_url?`<img src="${attr(g.cover_url)}" alt="">`:`<span class="gx-cover-fallback"><b>${esc(short[g.platform]||'GAME')}</b></span>`}</span><span class="gx-save-copy"><strong>${esc(g.title)}</strong><small>${esc(labels[g.platform]||g.platform)}</small><span>${g.items.map(x=>`<i>${x.kind==='state'?'Save state':'SRAM'} v${x.version} · ${bytes(x.size_bytes)}</i>`).join('')}</span>${g.play_seconds?`<em>◷ ${esc(time(g.play_seconds))} jogados</em>`:''}</span></button>`).join('')||'<div class="gx-empty small"><h2>Nenhum save neste perfil</h2><p>Jogue e use Salvar agora ou Salvar e sair.</p></div>'}</div></section>`;
  }

  async function loadSaves(){try{saves=await request('/games/saves?limit=300')}catch{saves=[]}if(screen==='saves')render()}

  function emulatorsHTML(){
    const matrix=[['Nintendo Entertainment System','NES','fceumm','.nes'],['Super Nintendo','SNES','snes9x','.sfc · .smc'],['Mega Drive / Genesis','GEN','genesis_plus_gx','.md · .gen · .smd'],['Game Boy / Color','GB/GBC','mgba','.gb · .gbc'],['Game Boy Advance','GBA','mgba','.gba']];
    return `<section class="gx-page"><div class="gx-page-head"><div><p>RETROARCH / WASM</p><h1>Emuladores</h1><small>O StormFlix prepara e guarda os cores fixados no servidor na primeira execução.</small></div></div><div class="gx-emulator-list">${matrix.map(x=>`<article><span class="gx-logo-mark">⌘</span><div><strong>${esc(x[0])}</strong><small>${esc(x[1])} · ${esc(x[3])}</small></div><code>${esc(x[2])}</code><b>Instalado sob demanda</b></article>`).join('')}</div><div class="gx-callout"><b>Arcade, Neo Geo e consoles de disco</b><p>Entrarão com diagnóstico de BIOS, ROMset e compatibilidade de core para mostrar a causa real quando um conjunto não puder iniciar.</p></div></section>`;
  }

  function settingsHTML(){
    return `<section class="gx-page gx-settings"><div class="gx-page-head"><div><h1>Configurações</h1></div></div><div class="gx-settings-tabs"><button class="active">Geral</button><button>Games</button><button>Sistema</button></div><div class="gx-settings-card"><h2>Interface</h2><div class="gx-setting-row"><div><b>Escala</b><small>Auto acompanha a tela; valores fixos aumentam a interface para TV.</small></div><div class="gx-setting-options">${['auto','100','125','150','200'].map(v=>`<button class="${scale===v?'active':''}" data-gx-scale="${v}">${v==='auto'?'Auto':v+'%'}</button>`).join('')}</div></div><div class="gx-setting-row"><div><b>Sons de navegação</b><small>Um clique discreto ao mudar de seção ou abrir um jogo.</small></div><div class="gx-setting-options"><button class="${sounds?'active':''}" data-gx-sounds="on">On</button><button class="${!sounds?'active':''}" data-gx-sounds="off">Off</button></div></div></div><div class="gx-settings-card"><h2>Games</h2><div class="gx-setting-row"><div><b>Player</b><small>Nostalgist 0.21.1 + RetroArch Emscripten v1.22.2</small></div><span class="gx-status-pill">Pronto</span></div><div class="gx-setting-row"><div><b>Saves</b><small>Autosave + state/SRAM por perfil + três gerações de recovery.</small></div><span class="gx-status-pill">Sincronizado</span></div></div><div class="gx-settings-card"><h2>Administração</h2><p>Metadados, provedores, bibliotecas, scans e ROMs agora ficam em <b>Admin → Games</b>.</p><a href="/admin/">Abrir Games Admin</a></div></section>`;
  }

  function bindShell(){
    root.querySelector('[data-gx-home]')?.addEventListener('click',()=>{screen='home';navSound();render()});
    root.querySelector('[data-gx-exit]')?.addEventListener('click',()=>closeGames(true));
    root.querySelectorAll('[data-gx-screen]').forEach(b=>b.onclick=()=>{screen=b.dataset.gxScreen;navSound();render()});
    root.querySelectorAll('[data-game-open]').forEach(b=>b.onclick=()=>{navSound();openDetail(Number(b.dataset.gameOpen))});
    root.querySelectorAll('[data-game-favorite]').forEach(b=>b.onclick=e=>{e.stopPropagation();toggleFavorite(Number(b.dataset.gameFavorite))});
    root.querySelector('[data-gx-search]')?.addEventListener('input',e=>{query=e.target.value.trim();render()});
    root.querySelectorAll('[data-gx-platform]').forEach(b=>b.onclick=()=>{platform=b.dataset.gxPlatform||'';render()});
    root.querySelectorAll('[data-gx-collection]').forEach(b=>b.onclick=()=>{platform=b.dataset.gxCollection;screen='library';navSound();render()});
    root.querySelectorAll('[data-gx-scale]').forEach(b=>b.onclick=()=>{scale=b.dataset.gxScale;localStorage.setItem('stormflix.games.scale',scale);applyScale();render()});
    root.querySelectorAll('[data-gx-sounds]').forEach(b=>b.onclick=()=>{sounds=b.dataset.gxSounds==='on';localStorage.setItem('stormflix.games.navsounds',sounds?'on':'off');navSound();render()});
    root.querySelectorAll('.gx-cover img,.gx-hero-cover img,.gx-save-cover img').forEach(img=>img.onerror=()=>img.classList.add('broken'));
  }

  function applyScale(){document.documentElement.style.setProperty('--gx-scale',scale==='auto'?'1':String(Number(scale)/100))}
  let audioCtx=null;
  function navSound(){if(!sounds)return;try{audioCtx=audioCtx||new(window.AudioContext||window.webkitAudioContext)();const o=audioCtx.createOscillator(),g=audioCtx.createGain();o.frequency.value=520;g.gain.value=.018;o.connect(g);g.connect(audioCtx.destination);o.start();g.gain.exponentialRampToValueAtTime(.0001,audioCtx.currentTime+.035);o.stop(audioCtx.currentTime+.04)}catch{}}

  async function toggleFavorite(id){
    const game=games.find(g=>Number(g.id)===id);if(!game)return;const next=!game.favorite;
    try{await request(`/games/${id}/favorite`,{method:'POST',body:JSON.stringify({favorite:next})});game.favorite=next;await refreshHome();render()}catch(err){if(typeof sfToast==='function')sfToast(err.message)}
  }
  async function refreshHome(){try{home=await request('/games/home')}catch{}}

  async function openDetail(id){
    let game=games.find(g=>Number(g.id)===id);try{game=await request(`/games/${id}`)}catch{}if(!game)return;
    const state=game.saves?.state?.exists?`Save state v${game.saves.state.version}`:'Sem save state';const sram=game.saves?.sram?.exists?`SRAM v${game.saves.sram.version}`:'Sem SRAM';
    const modal=document.createElement('div');modal.className='gx-detail-overlay';modal.innerHTML=`<article class="gx-detail" role="dialog" aria-modal="true"><div class="gx-detail-backdrop" style="--gx-detail-image:${game.cover_url?`url('${attr(game.cover_url)}')`:'none'}"></div><button class="gx-detail-close" type="button">✕</button><div class="gx-detail-cover">${coverHTML(game,true)}</div><div class="gx-detail-copy"><p>${esc(labels[game.platform]||game.platform)}</p><h2>${esc(game.title)}</h2><div class="gx-detail-chips"><span>${esc(short[game.platform]||game.platform)}</span>${game.release_year?`<span>${game.release_year}</span>`:''}${game.play_seconds?`<span>◷ ${esc(time(game.play_seconds))}</span>`:''}</div><p class="gx-detail-summary">${esc(game.overview||'Jogo identificado localmente pelo StormFlix. A identidade SHA-256 não muda quando os metadados externos forem enriquecidos.')}</p><div class="gx-detail-save"><span>${esc(state)}</span><span>${esc(sram)}</span></div><div class="gx-detail-actions"><button class="primary" type="button" data-gx-play ${game.playable?'':'disabled'}>▶ ${game.saves?.state?.exists?'Continuar jogando':'Jogar'}</button><button type="button" data-gx-favorite>${game.favorite?'♥ Favorito':'♡ Favoritar'}</button></div><small>${game.playable?'ROM autenticada · browser player nativo StormFlix':'Fora da matriz de browser play atual'}</small></div></article>`;
    document.body.appendChild(modal);const close=()=>modal.remove();modal.querySelector('.gx-detail-close').onclick=close;modal.onclick=e=>{if(e.target===modal)close()};modal.querySelector('[data-gx-favorite]').onclick=async()=>{close();await toggleFavorite(id)};modal.querySelector('[data-gx-play]').onclick=()=>{close();if(window.StormFlixGamePlayer)window.StormFlixGamePlayer.open(game);else if(typeof sfToast==='function')sfToast('Game Player não carregado')};modal.querySelector('[data-gx-play]')?.focus();
  }

  document.addEventListener('keydown',e=>{
    if(!document.body.classList.contains('games-mode')||document.querySelector('.gx-detail-overlay')||document.querySelector('#game-player-overlay'))return;
    if(e.key==='Escape'){e.preventDefault();if(screen!=='home'){screen='home';navSound();render()}else closeGames(true)}
    if(e.key==='/'&&screen!=='library'){e.preventDefault();screen='library';render();setTimeout(()=>root.querySelector('[data-gx-search]')?.focus(),0)}
  });
})();

/* StormFlix source: games-player.js */
/* StormFlix Games G4.3: single-owner Nostalgist/RetroArch browser session. */
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
    video:{smooth:false,filter:'pixel',integerScale:false,display:'fit',saturation:100},
    emulator:{rewind:true,autoSaveSeconds:120,fullscreen:false},
  };
  let overlay=null,canvas=null,instance=null,current=null,prepareMode='normal',preparePromise=null;
  let sessionID='',elapsed=0,lastTick=0,tickTimer=null,heartbeatTimer=null,autosaveTimer=null,gamepadTimer=null;
  let savePromise=null,closing=false,scriptPromise=null,romPromise=null,resizeObserver=null,resizeTimer=null;
  let keyboardAbort=null,viewportAbort=null,lastGamepadSignature='';
  const $=(sel,root=document)=>root.querySelector(sel);
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
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

  function platformAspect(platform){return ({gba:3/2,gb:10/9,gbc:10/9}[platform]||4/3)}
  function emulatorSurfaceSize(platform){
    if(platform==='gba')return {width:1200,height:800};
    if(platform==='gb'||platform==='gbc')return {width:1000,height:900};
    return {width:1280,height:960};
  }
  function withTimeout(promise,ms,label='Operação'){let timer;return Promise.race([Promise.resolve(promise),new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error(`${label}: tempo limite excedido`)),ms)})]).finally(()=>clearTimeout(timer))}

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
    overlay.innerHTML=`<header class="game-player-top"><div><span class="game-player-kicker">STORMFLIX GAME PLAYER G4.3</span><strong data-game-player-title>${esc(game.title)}</strong></div><div class="game-player-top-actions"><button type="button" data-game-menu aria-label="Configurações do jogo">☰</button><span class="game-runtime-badge">${esc(RUNTIME_LABEL)}</span><button type="button" data-game-fullscreen aria-label="Tela cheia">⛶</button><button type="button" data-game-close aria-label="Salvar e fechar o jogo">✕</button></div></header><div class="game-player-stage" data-game-stage><canvas id="stormflix-game-canvas" class="game-player-canvas" tabindex="0"></canvas><div class="game-launch-panel" data-game-launch></div><div class="game-player-toast hidden" data-game-toast></div></div><footer class="game-player-controls hidden" data-game-controls><div class="game-player-status"><span class="gamepad-dot" data-gamepad-dot></span><span data-gamepad-label>Teclado pronto</span><span data-game-input-label>Input: —</span><span data-playtime>Tempo deste perfil: ${clock(game.play_seconds)}</span></div><div class="game-player-buttons"><button type="button" data-game-pause>Pausar</button><button type="button" data-game-save>Salvar agora</button><button type="button" data-game-load ${game.saves?.state?.exists?'':'disabled'}>Carregar save</button><button type="button" data-game-exit>Salvar e sair</button></div></footer>`;
    document.body.appendChild(overlay);canvas=$('#stormflix-game-canvas',overlay);
    $('[data-game-close]',overlay).onclick=()=>close(true);$('[data-game-exit]',overlay).onclick=()=>close(true);$('[data-game-fullscreen]',overlay).onclick=toggleFullscreen;$('[data-game-pause]',overlay).onclick=togglePause;$('[data-game-save]',overlay).onclick=()=>saveAll(true);$('[data-game-load]',overlay).onclick=()=>loadSave();$('[data-game-menu]',overlay).onclick=()=>window.dispatchEvent(new CustomEvent('stormflix:game-menu-request'));
    const stage=$('[data-game-stage]',overlay);stage?.addEventListener('pointerdown',e=>{if(e.target===stage||e.target===canvas)focusGame()});updateGamepadLabel();applyPresentation();
  }
  function loadingHTML(game,mode){const action=mode==='continue'?'Restaurando seu save state e carregando o emulador.':mode==='new'?'Iniciando sem carregar o save anterior.':'Carregando ROM e emulador.';return `<div class="game-launch-card loading"><span class="game-loader"></span><p>${esc(platformLabel(game.platform))}</p><h2>Abrindo ${esc(game.title)}…</h2><div class="game-launch-facts"><span>${esc(game.rom_name||'ROM')}</span><span>${fmtBytes(game.rom_size_bytes)}</span><span>${esc(game.core||coreByPlatform[game.platform]||'core')}</span></div><p>${action}</p><small>Teclado, gamepad, touch e filtros usam a mesma sessão RetroArch do G4.3.</small></div>`}
  function startChoiceHTML(game){const state=game.saves?.state,sram=game.saves?.sram;return `<div class="game-launch-card game-start-choice"><p>${esc(platformLabel(game.platform))}</p><h2>Como você quer iniciar?</h2><p>Encontramos um save deste perfil para <b>${esc(game.title)}</b>.</p><div class="game-save-summary">${state?.exists?`<span>Save state v${Number(state.version||1)}</span>`:''}${sram?.exists?`<span>SRAM v${Number(sram.version||1)}</span>`:''}</div><div class="game-launch-actions"><button class="primary big" type="button" data-game-continue>▶ Continuar do último save</button><button class="big" type="button" data-game-new>↻ Iniciar novo jogo</button></div><small>Novo jogo não carrega o save anterior. O save existente só será substituído quando você salvar novamente.</small></div>`}
  function showStartChoice(game){const launch=$('[data-game-launch]',overlay);if(!launch)return;launch.classList.remove('hidden');launch.innerHTML=startChoiceHTML(game);$('[data-game-continue]',launch).onclick=()=>prepare('continue');$('[data-game-new]',launch).onclick=()=>prepare('new')}
  function showLaunchError(message,mode){const launch=$('[data-game-launch]',overlay);if(!launch)return;launch.classList.remove('hidden');launch.innerHTML=`<div class="game-launch-card error"><span class="game-ready-icon">!</span><h2>Não foi possível abrir o jogo</h2><p>${esc(message)}</p><button class="primary" type="button" data-game-retry>Tentar novamente</button></div>`;$('[data-game-retry]',launch).onclick=()=>prepare(mode)}

  async function open(gameInput){
    if(closing)return;if(instance||overlay)await close(false);pauseOtherMedia();releaseAllInputs();let game;
    try{game=await jsonFetch(`/api/v1/games/${Number(gameInput.id||gameInput)}`)}catch(err){notify(err.message);return}
    if(!game.playable||!game.core){notify('Este jogo não está disponível no navegador.');return}
    current=game;sessionID=randomSession();elapsed=0;preparePromise=null;prepareMode='normal';savePromise=null;closing=false;lastGamepadSignature='';createOverlay(game);romPromise=blobFetch(`/api/v1/games/${game.id}/rom`);
    if(game.saves?.state?.exists||game.saves?.sram?.exists)showStartChoice(game);else{$('[data-game-launch]',overlay).innerHTML=loadingHTML(game,'normal');void prepare('normal')}
  }
  async function prepare(mode){
    if(!current||preparePromise)return;prepareMode=mode==='continue'?'continue':mode==='new'?'new':'normal';const launch=$('[data-game-launch]',overlay);if(!launch)return;launch.classList.remove('hidden');launch.innerHTML=loadingHTML(current,prepareMode);preparePromise=prepareEmulator(prepareMode);
    try{await preparePromise}catch(err){preparePromise=null;if(instance){try{instance.exit()}catch{}instance=null}showLaunchError(err?.message||'Falha ao preparar o emulador.',prepareMode);return}
    try{await startPrepared()}catch(err){launch.innerHTML=`<div class="game-launch-card ready"><span class="game-ready-icon">✓</span><h2>Jogo carregado</h2><p>O navegador exige uma interação para iniciar áudio e controles.</p><button class="primary big" type="button" data-game-start>Iniciar jogo</button></div>`;const b=$('[data-game-start]',launch);b.onclick=async()=>{b.disabled=true;try{await startPrepared()}catch(e){b.disabled=false;toast(e?.message||'Não foi possível iniciar',true)}}}
  }
  async function prepareEmulator(mode){
    const core=current.core||coreByPlatform[current.platform];if(!core)throw new Error('Core não configurado para esta plataforma.');const p=preferences();
    const [Nostalgist,romBlob,coreJS,coreWASM]=await Promise.all([ensureRuntime(),romPromise||blobFetch(`/api/v1/games/${current.id}/rom`),blobFetch(`/api/v1/games/runtime/cores/${encodeURIComponent(core)}.js`),blobFetch(`/api/v1/games/runtime/cores/${encodeURIComponent(core)}.wasm`)]);
    const rom=typeof File==='function'?new File([romBlob],current.rom_name||`game.${current.platform}`,{type:'application/octet-stream'}):{fileName:current.rom_name||`game.${current.platform}`,fileContent:romBlob};let state=null,sram=null;if(mode!=='new'&&current.saves?.sram?.exists)sram=await blobFetch(`/api/v1/games/${current.id}/saves/sram`,true);if(mode==='continue'&&current.saves?.state?.exists)state=await blobFetch(`/api/v1/games/${current.id}/saves/state`,true);
    const options={element:canvas,size:emulatorSurfaceSize(current.platform),core:{name:core,js:{fileName:`${core}_libretro.js`,fileContent:coreJS},wasm:{fileName:`${core}_libretro.wasm`,fileContent:coreWASM}},rom,cache:{core:true,rom:false,bios:false,shader:true},respondToGlobalEvents:false,retroarchConfig:{savestate_thumbnail_enable:false,input_auto_game_focus:true,input_player1_analog_dpad_mode:1,video_force_aspect:true,video_smooth:false,video_scale_integer:!!p.video.integerScale,rewind_enable:!!p.emulator.rewind,...internalKeyboardConfig,...gamepadConfig()}};
    if(sram)options.sram=sram;if(state)options.state=state;
    try{window.StormFlixGameVideo?.decorateOptions?.(options,p.video||{})}catch(error){throw new Error(`Não foi possível preparar vídeo: ${error?.message||error}`)}
    instance=await Nostalgist.prepare(options);
  }
  async function startPrepared(){
    if(!instance||!overlay)throw new Error('Emulador não está preparado.');await instance.start();releaseAllInputs();$('[data-game-launch]',overlay)?.classList.add('hidden');$('[data-game-controls]',overlay)?.classList.remove('hidden');installKeyboardOwner();installViewportOwner();focusGame();startTracking();await heartbeat();window.dispatchEvent(new CustomEvent('stormflix:game-started',{detail:{id:current?.id,platform:current?.platform,core:current?.core}}));queueResize(0);queueResize(160);if(preferences().emulator.fullscreen&&!document.fullscreenElement)void toggleFullscreen();toast('G4.3 ativo · player responsivo e saves prontos')
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

  function applyPresentation(){
    if(!overlay||!canvas)return;const p=preferences(),filter=String(p.video.filter||'pixel');overlay.dataset.g4Display=p.video.display||'fit';canvas.style.filter=`saturate(${Math.max(50,Math.min(150,Number(p.video.saturation)||100))}%)`;canvas.style.imageRendering=filter==='pixel'?'pixelated':'auto';queueResize(16);
  }
  function fitCanvas(){
    if(!canvas||!overlay||!current)return false;const stage=$('[data-game-stage]',overlay);if(!stage)return false;const rect=stage.getBoundingClientRect();if(rect.width<2||rect.height<2)return false;
    const p=preferences(),edge=Math.max(8,Math.min(28,Math.round(Math.min(rect.width,rect.height)*.022)));let width=Math.max(1,rect.width-edge*2),height=Math.max(1,rect.height-edge*2);
    if((p.video.display||'fit')!=='stretch'){
      const ratio=platformAspect(current.platform);width=Math.max(1,rect.width-edge*2);height=width/ratio;if(height>rect.height-edge*2){height=Math.max(1,rect.height-edge*2);width=height*ratio}
    }
    width=Math.max(1,Math.floor(width));height=Math.max(1,Math.floor(height));
    canvas.style.setProperty('width',`${width}px`,'important');canvas.style.setProperty('height',`${height}px`,'important');canvas.style.setProperty('max-width',`calc(100% - ${edge*2}px)`,'important');canvas.style.setProperty('max-height',`calc(100% - ${edge*2}px)`,'important');canvas.style.setProperty('flex','0 0 auto','important');return true;
  }
  function queueResize(delay=40){clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{resizeTimer=null;requestAnimationFrame(fitCanvas)},Math.max(0,delay))}
  function installViewportOwner(){resizeObserver?.disconnect();const stage=$('[data-game-stage]',overlay);if(stage&&'ResizeObserver'in window){resizeObserver=new ResizeObserver(()=>queueResize(20));resizeObserver.observe(stage)}if(viewportAbort)viewportAbort.abort();viewportAbort=new AbortController();window.addEventListener('resize',()=>queueResize(30),{signal:viewportAbort.signal});window.addEventListener('orientationchange',()=>queueResize(180),{signal:viewportAbort.signal});document.addEventListener('fullscreenchange',()=>{queueResize(80);setTimeout(focusGame,90)},{signal:viewportAbort.signal});applyPresentation()}

  function startTracking(){stopTracking();lastTick=performance.now();tickTimer=setInterval(()=>{const now=performance.now(),delta=Math.min(2,Math.max(0,(now-lastTick)/1000));lastTick=now;if(instance?.getStatus?.()==='running'&&!document.hidden)elapsed+=delta},1000);heartbeatTimer=setInterval(heartbeat,15000);const seconds=Math.max(30,Number(preferences().emulator.autoSaveSeconds)||120);autosaveTimer=setInterval(()=>saveAll(false),seconds*1000);gamepadTimer=setInterval(updateGamepadLabel,900)}
  function stopTracking(){for(const id of [tickTimer,heartbeatTimer,autosaveTimer,gamepadTimer])if(id)clearInterval(id);tickTimer=heartbeatTimer=autosaveTimer=gamepadTimer=null}
  async function heartbeat(){if(!current||!sessionID)return;try{const data=await jsonFetch(`/api/v1/games/${current.id}/playback`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionID,elapsed_seconds:Math.floor(elapsed)})});current.play_seconds=Number(data.play_seconds||current.play_seconds||0);const label=$('[data-playtime]',overlay);if(label)label.textContent=`Tempo deste perfil: ${clock(current.play_seconds)}`}catch{}}
  async function uploadSave(kind,blob){if(!blob||!blob.size)return null;const r=await fetch(`/api/v1/games/${current.id}/saves/${kind}`,{method:'PUT',credentials:'same-origin',headers:{'Content-Type':'application/octet-stream'},body:blob});const text=await r.text();let data={};try{data=text?JSON.parse(text):{}}catch{}if(!r.ok)throw new Error(data.error||`Falha ao salvar ${kind}`);return data}
  async function saveAll(manual){if(savePromise){try{await savePromise}catch{}if(!manual)return}if(!instance||!current||instance.getStatus?.()==='terminated')return;if(manual)toast('Salvando estado e SRAM…');const target=instance,game=current;savePromise=(async()=>{const errors=[];let stateInfo=null,sramInfo=null;try{const result=await target.saveState();if(result?.state?.size&&current===game)stateInfo=await uploadSave('state',result.state)}catch(e){errors.push(`estado: ${e.message}`)}try{const sram=await target.saveSRAM();if(sram?.size&&current===game)sramInfo=await uploadSave('sram',sram)}catch(e){errors.push(`SRAM: ${e.message}`)}if(current===game){current.saves=current.saves||{};if(stateInfo)current.saves.state=stateInfo;if(sramInfo)current.saves.sram=sramInfo;const load=$('[data-game-load]',overlay);if(load&&current.saves?.state?.exists)load.disabled=false}if(manual&&overlay)toast(errors.length?`Save parcial · ${errors.join(' · ')}`:'Save sincronizado com o perfil ✓',errors.length>0)})();try{return await savePromise}finally{savePromise=null}}
  async function loadSave(){if(!instance||!current||instance.getStatus?.()==='terminated')return;if(!current.saves?.state?.exists){toast('Ainda não existe save state para carregar.',true);return}try{toast('Carregando último save…');const state=await withTimeout(blobFetch(`/api/v1/games/${current.id}/saves/state`,true),3500,'Download do save');if(!state)throw new Error('Save state não encontrado');await withTimeout(instance.loadState(state),4500,'Carregamento do save');focusGame();toast('Último save carregado ✓')}catch(e){toast(e?.message||'Não foi possível carregar o save.',true)}}

  async function togglePause(){if(!instance)return;const b=$('[data-game-pause]',overlay);try{if(instance.getStatus?.()==='paused'){await instance.resume();if(b)b.textContent='Pausar';lastTick=performance.now();focusGame();toast('Jogo retomado')}else{releaseAllInputs();await instance.pause();if(b)b.textContent='Continuar';await heartbeat();toast('Jogo pausado')}}catch(e){toast(e.message,true)}}
  async function toggleFullscreen(){try{if(document.fullscreenElement)await document.exitFullscreen();else await overlay?.requestFullscreen?.();setTimeout(()=>{queueResize(0);focusGame()},80)}catch{toast('Tela cheia não disponível neste navegador',true)}}
  async function close(saveFirst){if(closing)return;closing=true;stopTracking();releaseAllInputs();keyboardAbort?.abort();keyboardAbort=null;viewportAbort?.abort();viewportAbort=null;resizeObserver?.disconnect();resizeObserver=null;clearTimeout(resizeTimer);const target=instance;try{if(saveFirst&&target){toast('Salvando antes de sair…');try{await withTimeout(saveAll(true),5000,'Salvamento ao sair')}catch(e){console.warn('[StormFlix G4.3] encerrando após falha/timeout de save',e)}}else if(savePromise){try{await withTimeout(savePromise,2500,'Save pendente')}catch{}}try{await withTimeout(heartbeat(),1400,'Atualização de sessão')}catch{}if(target){try{target.exit()}catch{}}}finally{instance=null;current=null;preparePromise=null;romPromise=null;savePromise=null;sessionID='';elapsed=0;lastGamepadSignature='';if(overlay){overlay.remove();overlay=null;canvas=null}closing=false;window.dispatchEvent(new CustomEvent('stormflix:game-closed'))}}

  function updateGamepadLabel(){if(!overlay)return;const pads=[...(navigator.getGamepads?.()||[])].filter(Boolean),label=$('[data-gamepad-label]',overlay),dot=$('[data-gamepad-dot]',overlay);if(label)label.textContent=pads.length?`${pads.length} gamepad(s) · ${pads[0].id||'controle conectado'}`:'Teclado pronto · nenhum gamepad';if(dot)dot.classList.toggle('online',pads.length>0);const detail=pads.map(p=>({id:p.id,index:p.index,mapping:p.mapping})),signature=JSON.stringify(detail);if(signature!==lastGamepadSignature){lastGamepadSignature=signature;window.dispatchEvent(new CustomEvent('stormflix:gamepads-changed',{detail}))}}
  function pauseOtherMedia(){try{const v=document.querySelector('#player');if(v&&!v.paused)v.pause()}catch{}try{const a=document.querySelector('#music-audio');if(a&&!a.paused)a.pause()}catch{}try{if(typeof stopTheme==='function')stopTheme()}catch{}}
  function toast(message,error=false){const node=$('[data-game-toast]',overlay);if(!node)return;node.textContent=message;node.classList.toggle('error',!!error);node.classList.remove('hidden');clearTimeout(node._timer);node._timer=setTimeout(()=>node.classList.add('hidden'),error?5000:2600)}
  function notify(message){if(typeof sfToast==='function')sfToast(message);else console.warn(message)}

  addEventListener('gamepadconnected',updateGamepadLabel);addEventListener('gamepaddisconnected',updateGamepadLabel);document.addEventListener('visibilitychange',()=>{if(document.hidden&&instance)saveAll(false)});addEventListener('beforeunload',()=>{if(!current||!sessionID||!navigator.sendBeacon)return;const body=new Blob([JSON.stringify({session_id:sessionID,elapsed_seconds:Math.floor(elapsed)})],{type:'application/json'});navigator.sendBeacon(`/api/v1/games/${current.id}/playback`,body)});

  window.StormFlixGamePlayer={
    open,close:()=>close(true),active:()=>!!instance,pressDown:(b)=>pressDown(b,'virtual'),pressUp:(b)=>pressUp(b,'virtual'),resize:()=>queueResize(0),focus:focusGame,save:()=>saveAll(true),loadSave,pause:togglePause,fullscreen:toggleFullscreen,
    preferences,patchPreferences,resetPreferences,defaultPreferences:()=>clone(defaultPrefs),gamepads:()=>[...(navigator.getGamepads?.()||[])].filter(Boolean),runtime:()=>instance,toast,
    current:()=>current?{id:current.id,title:current.title,platform:current.platform,core:current.core,hasState:!!current.saves?.state?.exists,hasSram:!!current.saves?.sram?.exists}:null,
  };
})();

/* StormFlix source: games-g4-video.js */
/* StormFlix Games G4.2 video filters: live RetroArch GLSL switching without restarting. */
(function(){
  const SHADER_REV='235448f244bf676d135f7b25ea6b8e1eae41c4e4';
  const CDN=`https://cdn.jsdelivr.net/gh/libretro/glsl-shaders@${SHADER_REV}`;
  const packageCache=new Map();
  let liveFilterId=null,liveApplyPromise=null;
  const catalog=[
    {id:'pixel',name:'Pixel perfeito',short:'Pixel',kind:'native',cost:'Leve',description:'Nearest-neighbor nítido, sem suavização. Ideal para preservar cada pixel.'},
    {id:'bilinear',name:'Bilinear',short:'Bilinear',kind:'native',cost:'Leve',description:'Suavização simples no canvas. Reduz blocos sem alterar os sprites.'},
    {id:'scanlines',name:'Scanlines',short:'Scanlines',kind:'shader',cost:'Leve',description:'Linhas de varredura leves para lembrar uma tela CRT sem pesar no navegador.',preset:'scanlines/res-independent-scanlines.glslp',assets:['scanlines/shaders/res-independent-scanlines.glsl']},
    {id:'crt',name:'CRT EasyMode',short:'CRT',kind:'shader',cost:'Médio',description:'Curvatura, máscara e scanlines de CRT com bom equilíbrio entre aparência e desempenho.',preset:'crt/crt-easymode.glslp',assets:['crt/shaders/crt-easymode.glsl']},
    {id:'ntsc',name:'NTSC S-Video',short:'NTSC',kind:'shader',cost:'Médio',description:'Mistura de cor e sinal analógico no estilo de uma TV antiga ligada por S-Video.',preset:'ntsc/ntsc-320px-svideo.glslp',assets:['ntsc/shaders/ntsc-pass1-svideo-2phase.glsl','ntsc/shaders/ntsc-pass2-2phase-gamma.glsl']},
    {id:'hq2x',name:'HQ2x',short:'HQ2x',kind:'shader',cost:'Médio',description:'Suaviza diagonais e contornos em 2x mantendo a leitura do pixel art.',preset:'hqx/hq2x.glslp',assets:['hqx/shader-files/hqx-pass1.glsl','hqx/shader-files/hqx-pass2.glsl','hqx/resources/hq2x.png'],rewrites:{'shader-files/hqx-pass1.glsl':'shaders/hqx-pass1.glsl','shader-files/hqx-pass2.glsl':'shaders/hqx-pass2.glsl','resources/hq2x.png':'shaders/hq2x.png'}},
    {id:'hq4x',name:'HQ4x',short:'HQ4x',kind:'shader',cost:'Pesado',description:'Versão 4x do HQx para telas grandes. Mais lisa e mais exigente na GPU.',preset:'hqx/hq4x.glslp',assets:['hqx/shader-files/hqx-pass1.glsl','hqx/shader-files/hqx-pass2.glsl','hqx/resources/hq4x.png'],rewrites:{'shader-files/hqx-pass1.glsl':'shaders/hqx-pass1.glsl','shader-files/hqx-pass2.glsl':'shaders/hqx-pass2.glsl','resources/hq4x.png':'shaders/hq4x.png'}},
    {id:'xbrz',name:'xBRZ Adaptativo',short:'xBRZ',kind:'shader',cost:'Médio',description:'Escala xBRZ livre, limpa curvas e diagonais conforme o tamanho do viewport.',preset:'xbrz/xbrz-freescale.glslp',assets:['xbrz/shaders/xbrz-freescale.glsl']},
    {id:'xbrz4',name:'xBRZ 4x',short:'xBRZ 4x',kind:'shader',cost:'Pesado',description:'xBRZ em 4x com acabamento HD para sprites, recomendado para desktop.',preset:'xbrz/4xbrz-linear.glslp',assets:['xbrz/shaders/4xbrz.glsl','stock.glsl'],rewrites:{'../stock.glsl':'shaders/stock.glsl'}},
  ];
  const byId=new Map(catalog.map(item=>[item.id,item]));

  function player(){return window.StormFlixGamePlayer}
  function selected(video){const id=String(video?.filter||'').trim();if(byId.has(id))return id;return video?.smooth?'bilinear':'pixel'}
  function fileName(path){return path.split('/').pop()||path}
  function esc(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function costClass(cost){return String(cost||'').toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/[^a-z]/g,'')}
  function makeFile(name,content){if(typeof File==='function')return new File([content],name,{type:'application/octet-stream'});return {fileName:name,fileContent:content}}

  async function fetchAsset(path){
    const response=await fetch(`${CDN}/${path}`,{cache:'force-cache',mode:'cors'});
    if(!response.ok)throw new Error(`Filtro gráfico indisponível (${response.status})`);
    return response.blob();
  }
  function rewritePreset(text,rewrites){let out=text;for(const [from,to] of Object.entries(rewrites||{}))out=out.split(from).join(to);return out}
  function assetDestination(spec,path){
    const parts=path.split('/'),relative=parts.length>1?parts.slice(1).join('/'):path,base=fileName(path);
    if(spec.rewrites?.[relative])return spec.rewrites[relative];
    const match=Object.entries(spec.rewrites||{}).find(([from])=>from===base||from.endsWith(`/${base}`));
    if(match)return match[1];
    if(relative.startsWith('shaders/'))return relative;
    return `shaders/${base}`;
  }
  async function loadPackage(id){
    if(packageCache.has(id))return packageCache.get(id);
    const spec=byId.get(id);if(!spec||spec.kind!=='shader')return null;
    const promise=(async()=>{
      const [presetBlob,...assetBlobs]=await Promise.all([fetchAsset(spec.preset),...spec.assets.map(fetchAsset)]);
      const presetText=rewritePreset(await presetBlob.text(),spec.rewrites);
      const assets=await Promise.all(spec.assets.map(async(path,index)=>({name:assetDestination(spec,path),data:new Uint8Array(await assetBlobs[index].arrayBuffer())})));
      return {id,presetName:`stormflix-${id}.glslp`,presetText,assets};
    })().catch(error=>{packageCache.delete(id);throw error});
    packageCache.set(id,promise);return promise;
  }
  async function buildBundle(id){
    const pkg=await loadPackage(id);if(!pkg)return [];
    return [makeFile(pkg.presetName,pkg.presetText),...pkg.assets.map(asset=>makeFile(fileName(asset.name),asset.data))];
  }

  function runtimeContext(){
    const runtime=player()?.runtime?.();if(!runtime||runtime.getStatus?.()==='terminated')throw new Error('Emulador ainda não está pronto');
    const ctx=runtime.getEmscripten?.();if(!ctx)throw new Error('Runtime RetroArch indisponível');return ctx;
  }
  function ensureDir(FS,path){
    if(FS.mkdirTree){FS.mkdirTree(path);return}
    let current='';for(const part of path.split('/').filter(Boolean)){current+=`/${part}`;try{FS.mkdir(current)}catch{}}
  }
  function callSetShader(ctx,path){
    const Module=ctx.Module||ctx,fn=Module._cmd_set_shader||ctx._cmd_set_shader,alloc=Module.stringToNewUTF8||ctx.stringToNewUTF8,free=Module._free||ctx._free;
    if(typeof fn!=='function'){
      if(!path)return true;
      throw new Error('Este core Web não expõe troca de shader em tempo real');
    }
    if(typeof alloc==='function'){
      const ptr=alloc(path);try{const result=fn(ptr);if(path&&result===0)throw new Error('RetroArch recusou o shader selecionado');return result!==0}finally{if(ptr&&typeof free==='function')free(ptr)}
    }
    if(typeof Module.ccall==='function'){
      const result=Module.ccall('cmd_set_shader','number',['string'],[path]);if(path&&result===0)throw new Error('RetroArch recusou o shader selecionado');return result!==0;
    }
    throw new Error('Runtime Web sem alocador de string para shader');
  }
  function writeRuntimePackage(ctx,pkg){
    const Module=ctx.Module||ctx,FS=Module.FS||ctx.FS;if(!FS?.writeFile)throw new Error('Filesystem do RetroArch indisponível');
    const root=`/stormflix-shaders/${pkg.id}`;ensureDir(FS,root);ensureDir(FS,`${root}/shaders`);
    FS.writeFile(`${root}/${pkg.presetName}`,pkg.presetText);
    for(const asset of pkg.assets){const slash=asset.name.lastIndexOf('/');if(slash>0)ensureDir(FS,`${root}/${asset.name.slice(0,slash)}`);FS.writeFile(`${root}/${asset.name}`,asset.data)}
    return `${root}/${pkg.presetName}`;
  }
  async function applyLive(id){
    if(liveApplyPromise)await liveApplyPromise;
    const spec=byId.get(id);if(!spec)throw new Error('Filtro desconhecido');
    liveApplyPromise=(async()=>{
      const ctx=runtimeContext();
      if(spec.kind==='native'){
        callSetShader(ctx,'');liveFilterId=id;window.dispatchEvent(new CustomEvent('stormflix:game-filter-applied',{detail:{id,live:true}}));return true;
      }
      const pkg=await loadPackage(id),path=writeRuntimePackage(ctx,pkg);callSetShader(ctx,path);liveFilterId=id;window.dispatchEvent(new CustomEvent('stormflix:game-filter-applied',{detail:{id,live:true,path}}));return true;
    })();
    try{return await liveApplyPromise}finally{liveApplyPromise=null}
  }

  function decorateOptions(options,video){
    const id=selected(video);
    options.retroarchConfig={...(options.retroarchConfig||{}),video_smooth:false};
    options.cache={...(options.cache||{}),shader:true};
    delete options.shader;delete options.resolveShader;
    options.__stormflixVideoFilter=id;
    return options;
  }
  function expose(){
    const api=player();if(!api)return;
    api.videoFilters=()=>catalog.map(item=>({...item,assets:undefined,rewrites:undefined,preset:undefined}));
    api.videoFilter=()=>selected(api.preferences?.().video||{});
    api.preloadVideoFilter=id=>loadPackage(id);
    api.applyVideoFilter=id=>applyLive(id);
  }
  function filterGridHTML(active){
    return `<div class="sf-g41-filter-grid">${catalog.map(item=>`<button type="button" class="sf-g41-filter-card ${active===item.id?'active':''}" data-g41-filter="${item.id}" data-live="${liveFilterId===item.id?'1':'0'}"><span class="sf-g41-filter-head"><b>${esc(item.name)}</b><i class="${costClass(item.cost)}">${esc(item.cost)}</i></span><small>${esc(item.description)}</small></button>`).join('')}</div>`;
  }
  function markLive(row,id){row.querySelectorAll('[data-g41-filter]').forEach(b=>{b.classList.toggle('active',b.dataset.g41Filter===id);b.dataset.live=b.dataset.g41Filter===id?'1':'0'})}
  function enhanceVideoPanel(){
    const panel=document.querySelector('[data-g4-panel]:not(.hidden)');if(!panel)return;
    const smooth=panel.querySelector('[data-video-smooth]');if(!smooth)return;
    const row=smooth.closest('.sf-g4-setting');if(!row||row.dataset.g41Filters==='1')return;
    const active=selected(player()?.preferences?.().video||{});
    row.dataset.g41Filters='1';row.classList.add('sf-g41-filter-setting');
    row.innerHTML=`<span class="sf-g41-filter-title"><b>Filtro gráfico em tempo real</b><small>Mesmo princípio do EmulatorJS/RomM: o preset é escrito no RetroArch já aberto e trocado sem reiniciar o jogo.</small></span>${filterGridHTML(active)}`;
    row.querySelectorAll('[data-g41-filter]').forEach(button=>button.addEventListener('click',async()=>{
      const id=button.dataset.g41Filter,spec=byId.get(id);if(!spec)return;
      row.querySelectorAll('[data-g41-filter]').forEach(b=>b.disabled=true);button.classList.add('loading');
      try{
        await applyLive(id);
        player()?.patchPreferences?.({video:{filter:id,smooth:id==='bilinear'}});
        markLive(row,id);player()?.toast?.(`${spec.name} aplicado em tempo real ✓`);
      }catch(error){
        button.title=error?.message||'Não foi possível aplicar o filtro';player()?.toast?.(button.title,true);
      }finally{row.querySelectorAll('[data-g41-filter]').forEach(b=>{b.disabled=false;b.classList.remove('loading')})}
    }));
    const note=[...panel.querySelectorAll('.sf-g4-note')].find(node=>/Filtro|escala inteira|aspecto/i.test(node.textContent||''));
    if(note){note.classList.add('sf-g41-live-note');note.textContent='Filtros, Fit/Esticar e saturação agora mudam na hora. Apenas “Escala inteira” continua exigindo reinício do core.'}
    const apply=panel.querySelector('[data-g4-apply]');if(apply){apply.textContent='Reiniciar para aplicar Escala inteira';apply.title='O reinício só é necessário para opções internas do core, não para os filtros gráficos.'}
  }

  async function applySavedFilter(){
    expose();const api=player();if(!api?.active?.())return;const id=selected(api.preferences?.().video||{});
    try{await applyLive(id);api.patchPreferences?.({video:{filter:id,smooth:id==='bilinear'}});api.resize?.()}catch(error){api.toast?.(`Filtro ${byId.get(id)?.name||id}: ${error?.message||error}`,true)}
  }

  window.StormFlixGameVideo={catalog,selected,decorateOptions,preload:loadPackage,buildBundle,apply:applyLive,shaderRevision:SHADER_REV};
  expose();
  const observer=new MutationObserver(()=>{expose();enhanceVideoPanel()});observer.observe(document.documentElement,{childList:true,subtree:true});
  window.addEventListener('stormflix:game-menu-request',()=>setTimeout(enhanceVideoPanel,0));
  window.addEventListener('stormflix:game-preferences-changed',enhanceVideoPanel);
  window.addEventListener('stormflix:game-started',()=>{liveFilterId=null;setTimeout(applySavedFilter,120)});
  window.addEventListener('stormflix:game-closed',()=>{liveFilterId=null;liveApplyPromise=null});
})();

/* StormFlix source: games-g4-session.js */
/* StormFlix Games G4 session bridge: safe settings restart + page scroll lock. */
(function(){
  const api=window.StormFlixGamePlayer;if(!api)return;
  api.reload=async function(){
    const game=api.current?.();if(!game?.id)return false;
    try{await api.save?.()}catch{}
    const id=game.id;
    await api.close?.();
    await new Promise(resolve=>setTimeout(resolve,80));
    await api.open?.(id);
    return true;
  };
  const lock=()=>{
    document.body.classList.add('sf-game-playing');
    // games-player creates the button, while games-g4 owns the actual menu.
    // Remove the base onclick dispatcher so one click cannot open/pause twice.
    const menu=document.querySelector('#game-player-overlay [data-game-menu]');
    if(menu)menu.onclick=null;
  };
  const unlock=()=>document.body.classList.remove('sf-game-playing');
  window.addEventListener('stormflix:game-started',lock);
  window.addEventListener('stormflix:game-closed',unlock);
  if(api.active?.())lock();
})();

/* StormFlix source: games-g4.js */
/* StormFlix Games G4 controls/settings shell. One input layer for keyboard, gamepad and touch. */
(function(){
  const $=(s,r=document)=>r.querySelector(s),$$=(s,r=document)=>[...r.querySelectorAll(s)];
  const inputs=['up','down','left','right','a','b','x','y','l','r','l2','r2','select','start'];
  const labels={up:'↑',down:'↓',left:'←',right:'→',a:'A',b:'B',x:'X',y:'Y',l:'L1',r:'R1',l2:'L2',r2:'R2',select:'SELECT',start:'START'};
  const gamepadKeys={up:'input_player1_up_btn',down:'input_player1_down_btn',left:'input_player1_left_btn',right:'input_player1_right_btn',a:'input_player1_a_btn',b:'input_player1_b_btn',x:'input_player1_x_btn',y:'input_player1_y_btn',l:'input_player1_l1_btn',r:'input_player1_r1_btn',l2:'input_player1_l2_btn',r2:'input_player1_r2_btn',select:'input_player1_select_btn',start:'input_player1_start_btn'};
  const standardButtons={b:0,a:1,y:2,x:3,l:4,r:5,l2:6,r2:7,select:8,start:9,up:12,down:13,left:14,right:15};
  const layouts={
    nes:{name:'NES',faces:['b','a'],shoulders:[]},gb:{name:'Game Boy',faces:['b','a'],shoulders:[]},gbc:{name:'Game Boy Color',faces:['b','a'],shoulders:[]},gba:{name:'Game Boy Advance',faces:['b','a'],shoulders:['l','r']},
    snes:{name:'Super Nintendo',faces:['y','x','b','a'],shoulders:['l','r']},genesis:{name:'Mega Drive / Genesis',faces:['y','b','a','x','l','r'],shoulders:[]},
  };
  let panel=null,pad=null,activeTab='quick',resumeAfterMenu=false,captureRAF=0,captureState=null,monitorRAF=0,lastPadState=[];
  const buttonRegistry=new Map(),pointerOwner=new Map();

  function player(){return window.StormFlixGamePlayer}
  function prefs(){return player()?.preferences?.()||{}}
  function current(){return player()?.current?.()||null}
  function coarse(){return !!matchMedia?.('(pointer: coarse)').matches||innerWidth<=900}
  function touchEnabled(){const mode=prefs().touch?.mode||'auto';return mode==='on'||(mode==='auto'&&coarse())}
  function esc(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function codeLabel(code){return String(code||'—').replace(/^Key/,'').replace(/^Arrow/,'').replace('ShiftRight','Shift dir.').replace('ShiftLeft','Shift esq.')}
  function running(){const api=player();return !!api?.active?.()&&!$('#game-player-overlay [data-game-controls]')?.classList.contains('hidden')}

  function install(){
    const overlay=$('#game-player-overlay');if(!overlay||overlay.dataset.g4Shell==='1')return;overlay.dataset.g4Shell='1';
    panel=document.createElement('div');panel.className='sf-g4-panel hidden';panel.dataset.g4Panel='1';overlay.appendChild(panel);
    $('[data-game-menu]',overlay)?.addEventListener('click',openMenu);
    rebuildTouch();startGamepadMonitor();
  }
  function cleanup(){cancelAnimationFrame(captureRAF);cancelAnimationFrame(monitorRAF);captureRAF=monitorRAF=0;captureState=null;releaseAllPointers();panel?.remove();pad?.remove();panel=pad=null;lastPadState=[]}

  function openMenu(){
    if(!panel||!running())return;releaseAllPointers();resumeAfterMenu=!/Continuar/i.test($('#game-player-overlay [data-game-pause]')?.textContent||'');if(resumeAfterMenu)player()?.pause?.();activeTab='quick';renderPanel();panel.classList.remove('hidden');setTimeout(()=>panel.querySelector('button')?.focus(),20);
  }
  function closeMenu(){if(!panel||panel.classList.contains('hidden'))return;cancelCapture();panel.classList.add('hidden');if(resumeAfterMenu&&/Continuar/i.test($('#game-player-overlay [data-game-pause]')?.textContent||''))player()?.pause?.();resumeAfterMenu=false;player()?.focus?.()}
  function renderPanel(){
    if(!panel)return;const game=current();
    panel.innerHTML=`<section class="sf-g4-card" role="dialog" aria-modal="true"><header><div><small>STORMFLIX GAME PLAYER G4</small><h2>${esc(game?.title||'Jogo')}</h2></div><button data-g4-close aria-label="Fechar">✕</button></header><nav>${[['quick','Jogo'],['controls','Controles'],['video','Vídeo'],['emulator','Emulador']].map(([id,label])=>`<button data-g4-tab="${id}" class="${activeTab===id?'active':''}">${label}</button>`).join('')}</nav><div class="sf-g4-content">${tabHTML()}</div></section>`;
    $('[data-g4-close]',panel).onclick=closeMenu;$$('[data-g4-tab]',panel).forEach(b=>b.onclick=()=>{activeTab=b.dataset.g4Tab;renderPanel()});bindTab();
  }
  function tabHTML(){if(activeTab==='controls')return controlsHTML();if(activeTab==='video')return videoHTML();if(activeTab==='emulator')return emulatorHTML();return quickHTML()}
  function quickHTML(){const pads=player()?.gamepads?.()||[];return `<div class="sf-g4-quick-grid"><button class="primary" data-g4-resume>▶ Continuar</button><button data-g4-save>💾 Salvar agora</button><button data-g4-fullscreen>⛶ Tela cheia</button><button data-g4-touch>🎮 Touch: ${touchModeLabel()}</button></div><div class="sf-g4-info"><b>Input ativo</b><span>Teclado G4 · ${pads.length?`${pads.length} gamepad(s) conectado(s)`:'nenhum gamepad conectado'}</span><small>Esc abre este menu. As setas não rolam mais a página durante o jogo.</small></div><button class="danger" data-g4-exit>Salvar e sair</button>`}
  function keyboardRows(){const map=prefs().keyboard||{};return inputs.map(i=>`<div class="sf-g4-map-row"><b>${labels[i]}</b><button data-key-map="${i}">${codeLabel(map[i])}</button></div>`).join('')}
  function gamepadRows(){const pad=(player()?.gamepads?.()||[])[0];if(!pad)return `<div class="sf-g4-empty">Conecte o controle e pressione qualquer botão. O navegador vai detectá-lo automaticamente.</div>`;const custom=prefs().gamepads?.[pad.id]||{};return `<div class="sf-g4-pad-name"><b>${esc(pad.id)}</b><small>Player 1 · índice ${pad.index} · ${pad.mapping||'mapeamento do navegador'}</small></div>${inputs.map(i=>{const key=gamepadKeys[i],value=custom[key]??standardButtons[i]??'';return `<div class="sf-g4-map-row"><b>${labels[i]}</b><button data-pad-map="${i}">Botão ${value}</button></div>`}).join('')}`}
  function controlsHTML(){const p=prefs();return `<div class="sf-g4-section"><h3>Teclado</h3><p>Clique em uma função e pressione a tecla desejada.</p><div class="sf-g4-mapping">${keyboardRows()}</div><button data-key-reset>Restaurar teclado padrão</button></div><div class="sf-g4-section"><h3>Gamepad</h3><p>O primeiro controle conectado é Player 1. Clique em uma função e depois pressione o botão físico.</p><div class="sf-g4-mapping" data-pad-mapping>${gamepadRows()}</div><button data-pad-reset>Restaurar gamepad padrão</button></div><div class="sf-g4-section"><h3>Controle touch</h3><div class="sf-g4-choice">${['auto','on','off'].map(v=>`<button data-touch-mode="${v}" class="${(p.touch?.mode||'auto')===v?'active':''}">${v==='auto'?'Automático':v==='on'?'Ligado':'Desligado'}</button>`).join('')}</div><label class="sf-g4-toggle"><input type="checkbox" data-touch-haptics ${p.touch?.haptics!==false?'checked':''}> Vibração tátil</label></div><div class="sf-g4-note">Alterações de teclado e touch valem imediatamente. Remapeamento do gamepad é aplicado ao reabrir/reiniciar o emulador.</div><button class="primary" data-g4-apply>Aplicar e reiniciar o jogo</button>`}
  function videoHTML(){const v=prefs().video||{};return `<div class="sf-g4-section"><h3>Imagem</h3><div class="sf-g4-setting"><span><b>Encaixe da tela</b><small>Fit mantém toda a imagem visível. Esticar ocupa tudo.</small></span><div class="sf-g4-choice"><button data-video-display="fit" class="${v.display!=='stretch'?'active':''}">Fit</button><button data-video-display="stretch" class="${v.display==='stretch'?'active':''}">Esticar</button></div></div><div class="sf-g4-setting"><span><b>Filtro</b><small>Pixel nítido para jogos retrô ou suavização bilinear.</small></span><div class="sf-g4-choice"><button data-video-smooth="off" class="${!v.smooth?'active':''}">Pixel</button><button data-video-smooth="on" class="${v.smooth?'active':''}">Suave</button></div></div><div class="sf-g4-setting"><span><b>Escala inteira</b><small>Evita tamanhos fracionários quando o core permitir.</small></span><div class="sf-g4-choice"><button data-video-integer="off" class="${!v.integerScale?'active':''}">Off</button><button data-video-integer="on" class="${v.integerScale?'active':''}">On</button></div></div><div class="sf-g4-setting"><span><b>Saturação</b><small>Aplicada imediatamente à imagem do canvas.</small></span><div class="sf-g4-choice">${[80,100,120].map(n=>`<button data-video-saturation="${n}" class="${Number(v.saturation||100)===n?'active':''}">${n}%</button>`).join('')}</div></div></div><div class="sf-g4-note">Filtro, escala inteira e modo de aspecto são configurações do RetroArch e entram completamente após reiniciar.</div><button class="primary" data-g4-apply>Aplicar e reiniciar o jogo</button>`}
  function emulatorHTML(){const e=prefs().emulator||{},game=current();return `<div class="sf-g4-section"><h3>RetroArch / Core</h3><div class="sf-g4-runtime"><span><b>Runtime</b><code>Nostalgist 0.21.1</code></span><span><b>Core</b><code>${esc(game?.core||'—')}</code></span><span><b>Plataforma</b><code>${esc(game?.platform||'—')}</code></span></div><div class="sf-g4-setting"><span><b>Rewind</b><small>Mantém o buffer de retrocesso do RetroArch.</small></span><div class="sf-g4-choice"><button data-emu-rewind="on" class="${e.rewind!==false?'active':''}">On</button><button data-emu-rewind="off" class="${e.rewind===false?'active':''}">Off</button></div></div><div class="sf-g4-setting"><span><b>Autosave</b><small>Intervalo de save-state/SRAM enquanto joga.</small></span><div class="sf-g4-choice">${[[60,'1 min'],[120,'2 min'],[300,'5 min']].map(([n,l])=>`<button data-emu-autosave="${n}" class="${Number(e.autoSaveSeconds||120)===n?'active':''}">${l}</button>`).join('')}</div></div><label class="sf-g4-toggle"><input type="checkbox" data-emu-fullscreen ${e.fullscreen?'checked':''}> Abrir jogos em tela cheia</label></div><button data-g4-reset-all>Restaurar todas as configurações G4</button><button class="primary" data-g4-apply>Aplicar e reiniciar o jogo</button>`}

  function bindTab(){
    $('[data-g4-resume]',panel)?.addEventListener('click',closeMenu);$('[data-g4-save]',panel)?.addEventListener('click',()=>player()?.save?.());$('[data-g4-fullscreen]',panel)?.addEventListener('click',()=>player()?.fullscreen?.());$('[data-g4-exit]',panel)?.addEventListener('click',()=>player()?.close?.());$('[data-g4-touch]',panel)?.addEventListener('click',cycleTouchMode);
    $$('[data-key-map]',panel).forEach(b=>b.onclick=()=>captureKeyboard(b.dataset.keyMap,b));$('[data-key-reset]',panel)?.addEventListener('click',()=>{const d=player().defaultPreferences();player().patchPreferences({keyboard:d.keyboard});renderPanel()});
    $$('[data-pad-map]',panel).forEach(b=>b.onclick=()=>captureGamepad(b.dataset.padMap,b));$('[data-pad-reset]',panel)?.addEventListener('click',resetGamepad);
    $$('[data-touch-mode]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({touch:{mode:b.dataset.touchMode}});rebuildTouch();renderPanel()});$('[data-touch-haptics]',panel)?.addEventListener('change',e=>player().patchPreferences({touch:{haptics:e.target.checked}}));
    $$('[data-video-display]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({video:{display:b.dataset.videoDisplay}});renderPanel()});$$('[data-video-smooth]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({video:{smooth:b.dataset.videoSmooth==='on'}});renderPanel()});$$('[data-video-integer]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({video:{integerScale:b.dataset.videoInteger==='on'}});renderPanel()});$$('[data-video-saturation]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({video:{saturation:Number(b.dataset.videoSaturation)}});renderPanel()});
    $$('[data-emu-rewind]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({emulator:{rewind:b.dataset.emuRewind==='on'}});renderPanel()});$$('[data-emu-autosave]',panel).forEach(b=>b.onclick=()=>{player().patchPreferences({emulator:{autoSaveSeconds:Number(b.dataset.emuAutosave)}});renderPanel()});$('[data-emu-fullscreen]',panel)?.addEventListener('change',e=>player().patchPreferences({emulator:{fullscreen:e.target.checked}}));
    $('[data-g4-reset-all]',panel)?.addEventListener('click',()=>{player().resetPreferences();rebuildTouch();renderPanel()});$$('[data-g4-apply]',panel).forEach(b=>b.onclick=applyRestart);
  }

  function captureKeyboard(input,button){cancelCapture();button.textContent='Pressione uma tecla…';const handler=e=>{e.preventDefault();e.stopImmediatePropagation();if(e.code==='Escape'){button.textContent=codeLabel(prefs().keyboard?.[input]);document.removeEventListener('keydown',handler,true);return}const map={...(prefs().keyboard||{})};for(const key of Object.keys(map))if(map[key]===e.code&&key!==input)map[key]='';map[input]=e.code;player().patchPreferences({keyboard:map});document.removeEventListener('keydown',handler,true);renderPanel()};document.addEventListener('keydown',handler,true);captureState={kind:'keyboard',handler}}
  function cancelCapture(){if(captureState?.kind==='keyboard')document.removeEventListener('keydown',captureState.handler,true);captureState=null;cancelAnimationFrame(captureRAF);captureRAF=0}
  function captureGamepad(input,button){
    cancelCapture();const pad=(player()?.gamepads?.()||[])[0];if(!pad){button.textContent='Conecte um controle';return}button.textContent='Pressione um botão…';const baseline=pad.buttons.map(x=>!!x.pressed);captureState={kind:'gamepad'};
    const poll=()=>{const currentPad=(navigator.getGamepads?.()||[])[pad.index];if(!currentPad||captureState?.kind!=='gamepad')return;for(let i=0;i<currentPad.buttons.length;i++){if(currentPad.buttons[i].pressed&&!baseline[i]){setGamepadMapping(currentPad,input,i);captureState=null;renderPanel();return}}captureRAF=requestAnimationFrame(poll)};captureRAF=requestAnimationFrame(poll);
  }
  function setGamepadMapping(pad,input,index){const all={...(prefs().gamepads||{})},mapping={...(all[pad.id]||{})},target=gamepadKeys[input];for(const [i,key] of Object.entries(gamepadKeys))if(i!==input&&String(mapping[key]??standardButtons[i])===String(index))mapping[key]='nul';mapping[target]=String(index);all[pad.id]=mapping;player().patchPreferences({gamepads:all})}
  function resetGamepad(){const pad=(player()?.gamepads?.()||[])[0];if(!pad)return;const all={...(prefs().gamepads||{})};delete all[pad.id];player().patchPreferences({gamepads:all});renderPanel()}
  async function applyRestart(){cancelCapture();const api=player();if(!api?.reload)return;const b=$('[data-g4-apply]',panel);if(b){b.disabled=true;b.textContent='Salvando e reiniciando…'}resumeAfterMenu=false;panel.classList.add('hidden');await api.reload()}

  function touchModeLabel(){return ({auto:'Auto',on:'Ligado',off:'Desligado'}[prefs().touch?.mode||'auto']||'Auto')}
  function cycleTouchMode(){const modes=['auto','on','off'],now=prefs().touch?.mode||'auto',next=modes[(modes.indexOf(now)+1)%modes.length];player().patchPreferences({touch:{mode:next}});rebuildTouch();renderPanel()}

  function buttonId(node){return node.dataset.g4ButtonId}
  function registerButton(node,inputList){const id=`g4-${Math.random().toString(36).slice(2)}`;node.dataset.g4ButtonId=id;buttonRegistry.set(id,{node,inputs:inputList,pointers:new Set()});node.addEventListener('pointerdown',e=>{e.preventDefault();e.stopPropagation();try{if(node.hasPointerCapture(e.pointerId))node.releasePointerCapture(e.pointerId)}catch{}pressButton(id,e.pointerId);if(prefs().touch?.haptics!==false)navigator.vibrate?.(8)},{passive:false});node.addEventListener('pointermove',e=>{if(!pointerOwner.has(e.pointerId))return;e.preventDefault();if(e.buttons===0){releasePointer(e.pointerId);return}const next=document.elementFromPoint(e.clientX,e.clientY)?.closest?.('[data-g4-button-id]');if(next)pressButton(buttonId(next),e.pointerId);else releasePointer(e.pointerId)},{passive:false});node.addEventListener('pointerup',e=>releasePointer(e.pointerId));node.addEventListener('pointercancel',e=>releasePointer(e.pointerId));node.addEventListener('contextmenu',e=>e.preventDefault())}
  function pressButton(id,pointerId){if(pointerOwner.get(pointerId)===id)return;releasePointer(pointerId);const b=buttonRegistry.get(id);if(!b)return;const first=b.pointers.size===0;b.pointers.add(pointerId);pointerOwner.set(pointerId,id);b.node.classList.add('pressed');if(first)for(const input of b.inputs)player()?.pressDown?.(input)}
  function releasePointer(pointerId){const id=pointerOwner.get(pointerId);if(!id)return;pointerOwner.delete(pointerId);const b=buttonRegistry.get(id);if(!b)return;b.pointers.delete(pointerId);if(b.pointers.size===0){for(const input of b.inputs)player()?.pressUp?.(input);b.node.classList.remove('pressed')}}
  function releaseAllPointers(){for(const id of [...pointerOwner.keys()])releasePointer(id);buttonRegistry.clear()}
  function touchButton(input,text,cls=''){return `<button type="button" class="${cls}" data-g4-touch-input="${input}">${text}</button>`}
  function rebuildTouch(){releaseAllPointers();pad?.remove();pad=null;const overlay=$('#game-player-overlay');if(!overlay||!running()||!touchEnabled()){overlay?.classList.remove('sf-g4-touch-on');player()?.resize?.();return}const game=current(),layout=layouts[game?.platform]||layouts.nes;overlay.classList.add('sf-g4-touch-on');pad=document.createElement('div');pad.className='sf-g4-touch';pad.dataset.g4Touch='1';const face=layout.faces.length===4?`<div class="sf-g4-faces diamond">${touchButton('x','X','x')}${touchButton('y','Y','y')}${touchButton('a','A','a')}${touchButton('b','B','b')}</div>`:layout.faces.length===6?`<div class="sf-g4-faces six">${layout.faces.map(x=>touchButton(x,labels[x])).join('')}</div>`:`<div class="sf-g4-faces two">${layout.faces.map(x=>touchButton(x,labels[x],x)).join('')}</div>`;pad.innerHTML=`<div class="sf-g4-left">${layout.shoulders.length?`<div class="sf-g4-shoulders">${layout.shoulders.map(x=>touchButton(x,labels[x])).join('')}</div>`:''}<div class="sf-g4-dpad">${touchButton('up,left','↖','ul')}${touchButton('up','↑','up')}${touchButton('up,right','↗','ur')}${touchButton('left','←','left')}<span></span>${touchButton('right','→','right')}${touchButton('down,left','↙','dl')}${touchButton('down','↓','down')}${touchButton('down,right','↘','dr')}</div>${touchButton('select','SELECT','select')}</div><div class="sf-g4-right">${face}${touchButton('start','START','start')}</div>`;$('#game-player-overlay [data-game-stage]')?.appendChild(pad);$$('[data-g4-touch-input]',pad).forEach(node=>registerButton(node,node.dataset.g4TouchInput.split(',')));player()?.resize?.()}

  function startGamepadMonitor(){cancelAnimationFrame(monitorRAF);const loop=()=>{if(!running()){monitorRAF=requestAnimationFrame(loop);return}const pad=(navigator.getGamepads?.()||[])[0];if(pad){const now=pad.buttons.map(b=>!!b.pressed);for(let i=0;i<now.length;i++)if(now[i]&&!lastPadState[i]){const label=$('#game-player-overlay [data-game-input-label]');if(label)label.textContent=`Input: botão ${i} · gamepad`}lastPadState=now}else lastPadState=[];monitorRAF=requestAnimationFrame(loop)};monitorRAF=requestAnimationFrame(loop)}

  document.body.addEventListener('pointerup',e=>releasePointer(e.pointerId),true);document.body.addEventListener('pointercancel',e=>releasePointer(e.pointerId),true);
  window.addEventListener('stormflix:game-menu-request',openMenu);window.addEventListener('stormflix:game-started',()=>{install();rebuildTouch()});window.addEventListener('stormflix:game-closed',cleanup);window.addEventListener('stormflix:game-preferences-changed',rebuildTouch);window.addEventListener('stormflix:gamepads-changed',()=>{if(panel&&!panel.classList.contains('hidden')&&activeTab==='controls')renderPanel()});window.addEventListener('resize',()=>setTimeout(rebuildTouch,80));window.addEventListener('orientationchange',()=>setTimeout(rebuildTouch,180));
  const observer=new MutationObserver(()=>{if($('#game-player-overlay')&&!panel)install()});observer.observe(document.documentElement,{childList:true,subtree:true});
})();

/* StormFlix source: games-g4-polish.js */
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

/* StormFlix source: games-g43-ui.js */
/* StormFlix Games G4.3: RomMix-inspired full game page + save controls. */
(function(){
  const $=(s,r=document)=>r.querySelector(s),$$=(s,r=document)=>[...r.querySelectorAll(s)];
  const labels={nes:'Nintendo Entertainment System',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'};
  const short={nes:'NES',snes:'SNES',genesis:'GEN',gb:'GB',gbc:'GBC',gba:'GBA'};
  let detailGame=null,returnScreen='home',activeTab='details',loadingId=0;
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const attr=s=>esc(s).replace(/`/g,'&#96;');
  const bytes=n=>{n=Number(n)||0;if(n<1024)return`${n} B`;if(n<1048576)return`${(n/1024).toFixed(1)} KB`;return`${(n/1048576).toFixed(1)} MB`};
  const time=s=>{s=Math.max(0,Math.floor(Number(s)||0));const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${m} min`};
  const date=s=>{if(!s)return'—';try{return new Intl.DateTimeFormat('pt-BR',{dateStyle:'medium'}).format(new Date(s))}catch{return'—'}};
  function content(){return $('#games-view .gx-content')}
  function inGames(){return document.body.classList.contains('games-mode')&&!$('#games-view')?.classList.contains('hidden')}
  async function gameFetch(id){const r=await fetch(`/api/v1/games/${Number(id)}`,{credentials:'same-origin',cache:'no-store'});const text=await r.text();let data={};try{data=JSON.parse(text)}catch{}if(!r.ok)throw new Error(data.error||`HTTP ${r.status}`);return data}

  function cover(game){return game.cover_url?`<img src="${attr(game.cover_url)}" alt="Capa de ${attr(game.title)}">`:`<span class="gx-g43-cover-fallback"><b>${esc(short[game.platform]||'GAME')}</b><small>STORMFLIX</small></span>`}
  function chip(text,cls=''){return `<span class="gx-g43-chip ${cls}">${esc(text)}</span>`}
  function detailRows(game){return `<div class="gx-g43-facts">
    <div><span>Plataforma</span><b>${esc(labels[game.platform]||game.platform||'—')}</b></div>
    <div><span>Lançamento</span><b>${game.release_year?esc(String(game.release_year)):'—'}</b></div>
    <div><span>Core</span><b>${esc(game.core||'—')}</b></div>
    <div><span>Compatibilidade</span><b>${game.playable?'Pronto para jogar no navegador':'Não disponível neste navegador'}</b></div>
    <div><span>ROM</span><b>${esc(game.rom_name||'—')}</b></div>
    <div><span>Tamanho</span><b>${bytes(game.rom_size_bytes)}</b></div>
    <div><span>Tempo jogado</span><b>${time(game.play_seconds||0)}</b></div>
    <div><span>Última sessão</span><b>${date(game.last_played_at)}</b></div>
  </div>`}
  function savesTab(game){const state=game.saves?.state,sram=game.saves?.sram;return `<section class="gx-g43-tab-panel"><div class="gx-g43-save-grid">
    <article><span class="gx-g43-save-icon">▣</span><div><h3>Save state</h3><p>${state?.exists?`Versão ${Number(state.version||1)} · ${bytes(state.size_bytes)}`:'Nenhum save state criado ainda.'}</p>${state?.updated_at?`<small>Atualizado em ${date(state.updated_at)}</small>`:''}</div></article>
    <article><span class="gx-g43-save-icon">◆</span><div><h3>SRAM</h3><p>${sram?.exists?`Versão ${Number(sram.version||1)} · ${bytes(sram.size_bytes)}`:'Nenhuma SRAM sincronizada ainda.'}</p>${sram?.updated_at?`<small>Atualizada em ${date(sram.updated_at)}</small>`:''}</div></article>
  </div><p class="gx-g43-help">Os saves ficam vinculados ao perfil atual. Ao abrir um jogo com save existente, o StormFlix pergunta se você quer continuar ou iniciar uma sessão nova.</p></section>`}
  function fileTab(game){return `<section class="gx-g43-tab-panel">${detailRows(game)}<p class="gx-g43-help">A ROM é usada diretamente pelo player do StormFlix. Esta tela não oferece download.</p></section>`}
  function tabBody(game){if(activeTab==='saves')return savesTab(game);if(activeTab==='file')return fileTab(game);return `<section class="gx-g43-tab-panel"><h2>Sobre o jogo</h2><p class="gx-g43-about">${esc(game.overview||'Jogo identificado na biblioteca do StormFlix e pronto para ser executado no player do navegador.')}</p>${detailRows(game)}</section>`}

  function renderDetail(){const host=content(),game=detailGame;if(!host||!game)return;const hasSave=!!(game.saves?.state?.exists||game.saves?.sram?.exists);host.innerHTML=`<section class="gx-game-page" data-g43-game-page="${Number(game.id)}">
    <div class="gx-g43-hero" style="--gx-g43-image:${game.cover_url?`url('${attr(game.cover_url)}')`:'none'}">
      <div class="gx-g43-backdrop"></div><div class="gx-g43-shade"></div>
      <div class="gx-g43-hero-body"><div class="gx-g43-art">${cover(game)}</div><div class="gx-g43-copy">
        <p class="gx-g43-kicker">${esc(labels[game.platform]||game.platform||'JOGO')}</p><h1>${esc(game.title)}</h1>
        <div class="gx-g43-meta">${chip(short[game.platform]||String(game.platform||'').toUpperCase(),'system')}${game.release_year?chip(String(game.release_year)):''}${chip(bytes(game.rom_size_bytes))}${game.play_seconds?chip(`◷ ${time(game.play_seconds)}`):''}${hasSave?chip('Save disponível','save'):''}</div>
        <p class="gx-g43-summary">${esc(game.overview||'Jogo identificado localmente pelo StormFlix e pronto para sua biblioteca de saves.')}</p>
        <div class="gx-g43-actions"><button class="primary" type="button" data-g43-play ${game.playable?'':'disabled'}>▶ Jogar</button><button type="button" data-g43-favorite>${game.favorite?'♥ Favorito':'♡ Favoritar'}</button><button type="button" data-g43-back>← Voltar</button></div>
      </div></div>
    </div>
    <nav class="gx-g43-tabs" aria-label="Informações do jogo"><button class="${activeTab==='details'?'active':''}" data-g43-tab="details">ⓘ Detalhes</button><button class="${activeTab==='saves'?'active':''}" data-g43-tab="saves">▣ Saves ${hasSave?'<i>•</i>':''}</button><button class="${activeTab==='file'?'active':''}" data-g43-tab="file">▤ Arquivo</button></nav>
    <div class="gx-g43-body">${tabBody(game)}</div>
  </section>`;
    $('[data-g43-back]',host)?.addEventListener('click',goBack);$('[data-g43-play]',host)?.addEventListener('click',()=>window.StormFlixGamePlayer?.open?.(game));$('[data-g43-favorite]',host)?.addEventListener('click',toggleFavorite);
    $$('[data-g43-tab]',host).forEach(b=>b.addEventListener('click',()=>{activeTab=b.dataset.g43Tab;renderDetail()}));
    host.querySelectorAll('img').forEach(img=>img.addEventListener('error',()=>img.closest('.gx-g43-art')?.classList.add('broken'),{once:true}));window.scrollTo({top:0,behavior:'auto'});
  }
  async function toggleFavorite(){if(!detailGame)return;const next=!detailGame.favorite;try{const r=await fetch(`/api/v1/games/${detailGame.id}/favorite`,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify({favorite:next})});if(!r.ok)throw new Error(`HTTP ${r.status}`);detailGame.favorite=next;renderDetail()}catch(e){window.sfToast?.(e.message||'Não foi possível alterar o favorito')}}
  function goBack(){detailGame=null;activeTab='details';const target=$(`#games-view [data-gx-screen="${returnScreen}"]`)||$('#games-view [data-gx-screen="home"]');target?.click()}
  async function openDetail(id){if(!inGames())return;const host=content();if(!host)return;returnScreen=$('#games-view [data-gx-screen].active')?.dataset.gxScreen||returnScreen||'home';activeTab='details';const ticket=++loadingId;host.innerHTML='<div class="gx-inline-loader gx-g43-loading"><span></span>Carregando informações do jogo…</div>';try{const game=await gameFetch(id);if(ticket!==loadingId)return;detailGame=game;renderDetail()}catch(e){if(ticket!==loadingId)return;host.innerHTML=`<section class="gx-empty small"><h2>Não foi possível abrir o jogo</h2><p>${esc(e.message)}</p><button type="button" data-g43-error-back>Voltar</button></section>`;$('[data-g43-error-back]',host)?.addEventListener('click',goBack)}}

  function upgradeQuickMenu(){const panel=$('[data-g4-panel]:not(.hidden)');if(!panel)return;const grid=$('.sf-g4-quick-grid',panel);if(!grid||$('[data-g43-load-save]',grid))return;const current=window.StormFlixGamePlayer?.current?.(),b=document.createElement('button');b.type='button';b.dataset.g43LoadSave='1';b.textContent='↥ Carregar save';b.disabled=!current?.hasState;b.title=current?.hasState?'Carregar o último save state sem reiniciar o jogo':'Nenhum save state disponível';const save=$('[data-g4-save]',grid);if(save?.nextSibling)grid.insertBefore(b,save.nextSibling);else grid.appendChild(b);b.addEventListener('click',()=>window.StormFlixGamePlayer?.loadSave?.())}

  document.addEventListener('click',e=>{const target=e.target.closest?.('#games-view [data-game-open]');if(!target||!inGames()||$('#game-player-overlay'))return;e.preventDefault();e.stopPropagation();e.stopImmediatePropagation();openDetail(Number(target.dataset.gameOpen))},true);
  document.addEventListener('keydown',e=>{if(!detailGame||!inGames()||$('#game-player-overlay'))return;if(e.key==='Escape'){e.preventDefault();e.stopImmediatePropagation();goBack()}},true);
  window.addEventListener('stormflix:game-closed',async()=>{if(!detailGame)return;try{detailGame=await gameFetch(detailGame.id);renderDetail()}catch{}});
  const observer=new MutationObserver(upgradeQuickMenu);observer.observe(document.documentElement,{childList:true,subtree:true});window.addEventListener('stormflix:game-menu-request',()=>setTimeout(upgradeQuickMenu,0));
})();

/* StormFlix source: games-g44.js */
/* StormFlix Games G4.4: native-shell alignment helpers, home game rails,
 * metadata media, save-choice exit and the low-overhead G3 circular touch stick.
 */
(function(){
  const $=(s,r=document)=>r.querySelector(s),$$=(s,r=document)=>[...r.querySelectorAll(s)];
  const player=()=>window.StormFlixGamePlayer;
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const attr=s=>esc(s).replace(/`/g,'&#96;');
  const time=s=>{s=Math.max(0,Math.floor(Number(s)||0));const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${m} min`};
  const short={nes:'NES',snes:'SNES',genesis:'GEN',gb:'GB',gbc:'GBC',gba:'GBA'};
  const layouts={
    nes:{face:[['b','B'],['a','A']]},gb:{face:[['b','B'],['a','A']]},gbc:{face:[['b','B'],['a','A']]},
    gba:{shoulders:[['l','L'],['r','R']],face:[['b','B'],['a','A']]},
    snes:{shoulders:[['l','L'],['r','R']],face:[['y','Y'],['x','X'],['b','B'],['a','A']]},
    genesis:{face:[['y','A'],['b','B'],['a','C'],['x','X'],['l','Y'],['r','Z']],genesis:true},
  };

  /* ---------- Circular mobile touch controller ---------- */
  const activePointers=new Map();
  let touchPad=null,stick={pointerId:null,inputs:[],node:null,rect:null},stickRAF=0,pendingPoint=null;
  const STICK_DEADZONE=.20;
  function coarse(){return !!window.matchMedia?.('(pointer: coarse)').matches||innerWidth<=900}
  function touchEnabled(){const mode=player()?.preferences?.().touch?.mode||'auto';return mode==='on'||(mode==='auto'&&coarse())}
  function gameplayVisible(){const overlay=$('#game-player-overlay');return !!overlay&&!$('[data-game-controls]',overlay)?.classList.contains('hidden')&&!!player()?.active?.()}
  function press(input,down){return down?player()?.pressDown?.(input)!==false:player()?.pressUp?.(input)!==false}
  function releaseButton(id){const entry=activePointers.get(id);if(!entry)return;activePointers.delete(id);for(const input of entry.inputs)press(input,false);entry.node?.classList.remove('pressed')}
  function bindButton(node,inputs){
    node.addEventListener('pointerdown',e=>{e.preventDefault();e.stopPropagation();releaseButton(e.pointerId);activePointers.set(e.pointerId,{node,inputs});node.classList.add('pressed');for(const input of inputs)press(input,true);if(player()?.preferences?.().touch?.haptics!==false)navigator.vibrate?.(8)},{passive:false});
    node.addEventListener('pointerup',e=>{e.preventDefault();releaseButton(e.pointerId)},{passive:false});
    node.addEventListener('pointercancel',e=>releaseButton(e.pointerId));node.addEventListener('lostpointercapture',e=>releaseButton(e.pointerId));node.addEventListener('contextmenu',e=>e.preventDefault());
  }
  function setStickInputs(next){const previous=stick.inputs;if(previous.join(',')===next.join(','))return;for(const input of previous)if(!next.includes(input))press(input,false);for(const input of next)if(!previous.includes(input))press(input,true);stick.inputs=next}
  function stickInputs(angle){const tau=Math.PI*2,sector=Math.round((((angle%tau)+tau)%tau)/(Math.PI/4))%8;return [['right'],['down','right'],['down'],['down','left'],['left'],['up','left'],['up'],['up','right']][sector]}
  function flushStick(){stickRAF=0;if(!stick.node||!stick.rect||!pendingPoint)return;const point=pendingPoint;pendingPoint=null;const rect=stick.rect,radius=Math.max(1,Math.min(rect.width,rect.height)/2);const dx=point.x-(rect.left+rect.width/2),dy=point.y-(rect.top+rect.height/2),distance=Math.hypot(dx,dy),strength=Math.min(1,distance/radius),travel=Math.max(1,radius*.48),scale=distance>travel?travel/distance:1;const thumb=$('[data-g44-stick-thumb]',stick.node);if(thumb)thumb.style.transform=`translate(${(dx*scale).toFixed(1)}px,${(dy*scale).toFixed(1)}px)`;setStickInputs(strength<STICK_DEADZONE?[]:stickInputs(Math.atan2(dy,dx)))}
  function queueStick(e){pendingPoint={x:e.clientX,y:e.clientY};if(!stickRAF)stickRAF=requestAnimationFrame(flushStick)}
  function releaseStick(id){if(stick.pointerId===null||((id??stick.pointerId)!==stick.pointerId))return;setStickInputs([]);if(stickRAF)cancelAnimationFrame(stickRAF);stickRAF=0;pendingPoint=null;const thumb=$('[data-g44-stick-thumb]',stick.node);if(thumb)thumb.style.transform='translate(0px,0px)';stick.node?.classList.remove('active');stick={pointerId:null,inputs:[],node:null,rect:null}}
  function bindStick(node){node.addEventListener('pointerdown',e=>{if(stick.pointerId!==null&&stick.pointerId!==e.pointerId)return;e.preventDefault();e.stopPropagation();stick={pointerId:e.pointerId,inputs:[],node,rect:node.getBoundingClientRect()};node.classList.add('active');try{node.setPointerCapture?.(e.pointerId)}catch{}if(player()?.preferences?.().touch?.haptics!==false)navigator.vibrate?.(8);queueStick(e)},{passive:false});node.addEventListener('pointermove',e=>{if(stick.pointerId!==e.pointerId)return;e.preventDefault();queueStick(e)},{passive:false});node.addEventListener('pointerup',e=>releaseStick(e.pointerId));node.addEventListener('pointercancel',e=>releaseStick(e.pointerId));node.addEventListener('lostpointercapture',e=>releaseStick(e.pointerId));node.addEventListener('contextmenu',e=>e.preventDefault())}
  function cleanupTouch(){for(const id of [...activePointers.keys()])releaseButton(id);releaseStick();touchPad?.remove();touchPad=null;const overlay=$('#game-player-overlay');overlay?.classList.remove('sf-virtual-pad-on');$$('[data-g4-touch]',overlay||document).forEach(n=>n.remove());overlay?.classList.remove('sf-g4-touch-on')}
  function button(input,label,cls=''){return `<button type="button" class="${cls}" data-g44-input="${input}">${label}</button>`}
  function faceHTML(layout){if(layout.genesis){const top=layout.face.slice(3),bottom=layout.face.slice(0,3);return `<div class="sf-g44-face genesis"><div>${top.map(([i,l])=>button(i,l)).join('')}</div><div>${bottom.map(([i,l])=>button(i,l)).join('')}</div></div>`}if(layout.face.length===4){const m=Object.fromEntries(layout.face);return `<div class="sf-g44-face diamond">${button('x',m.x||'X','x')}${button('y',m.y||'Y','y')}${button('a',m.a||'A','a')}${button('b',m.b||'B','b')}</div>`}return `<div class="sf-g44-face two">${layout.face.map(([i,l])=>button(i,l,i)).join('')}</div>`}
  function syncTouch(){
    const overlay=$('#game-player-overlay');$$('[data-g4-touch]',overlay||document).forEach(n=>n.remove());overlay?.classList.remove('sf-g4-touch-on');
    if(!overlay||!gameplayVisible()||!touchEnabled()){cleanupTouch();player()?.resize?.();return}
    if(touchPad?.isConnected)return;
    const game=player()?.current?.(),layout=layouts[game?.platform]||layouts.nes;touchPad=document.createElement('div');touchPad.className='sf-g44-pad';touchPad.dataset.g44Pad='1';touchPad.innerHTML=`${layout.shoulders?.length?`<div class="sf-g44-shoulders">${layout.shoulders.map(([i,l])=>button(i,l)).join('')}</div>`:''}<div class="sf-g44-main"><div class="sf-g44-stick" data-g44-stick><span class="sf-g44-stick-orbit"></span><span class="sf-g44-stick-thumb" data-g44-stick-thumb></span></div><div class="sf-g44-center">${button('select','SELECT')}${button('start','START')}</div>${faceHTML(layout)}</div>`;overlay.classList.add('sf-virtual-pad-on');$('[data-game-stage]',overlay)?.appendChild(touchPad);$$('[data-g44-input]',touchPad).forEach(n=>bindButton(n,String(n.dataset.g44Input||'').split(',').filter(Boolean)));bindStick($('[data-g44-stick]',touchPad));setTimeout(()=>player()?.resize?.(),30)
  }
  document.addEventListener('pointerup',e=>{releaseButton(e.pointerId);releaseStick(e.pointerId)},true);document.addEventListener('pointercancel',e=>{releaseButton(e.pointerId);releaseStick(e.pointerId)},true);window.addEventListener('blur',()=>{for(const id of [...activePointers.keys()])releaseButton(id);releaseStick()});
  window.addEventListener('stormflix:game-started',()=>setTimeout(syncTouch,0));window.addEventListener('stormflix:game-preferences-changed',()=>setTimeout(syncTouch,0));window.addEventListener('stormflix:game-closed',cleanupTouch);window.addEventListener('resize',()=>setTimeout(syncTouch,150),{passive:true});window.addEventListener('orientationchange',()=>setTimeout(syncTouch,260));

  /* ---------- X asks whether the current point should be saved ---------- */
  function closeExitDialog(){const d=$('#game-player-overlay [data-g44-exit-dialog]');d?.remove();player()?.focus?.()}
  function noSaveExit(){const p=player(),game=p?.current?.();if(game?.id)sessionStorage.setItem('stormflix.games.g44.return-detail',String(game.id));const runtime=p?.runtime?.();try{runtime?.exit?.()}catch{}setTimeout(()=>location.reload(),60)}
  function showExitDialog(){const overlay=$('#game-player-overlay');if(!overlay||$('[data-g44-exit-dialog]',overlay))return;const dialog=document.createElement('div');dialog.className='sf-g44-exit-dialog';dialog.dataset.g44ExitDialog='1';dialog.innerHTML=`<section role="dialog" aria-modal="true" aria-label="Sair do jogo"><h2>Sair do jogo?</h2><p>Você quer salvar exatamente o ponto atual antes de fechar?</p><div><button type="button" class="primary" data-g44-save-exit>Salvar e sair</button><button type="button" class="danger" data-g44-nosave-exit>Sair sem salvar</button><button type="button" data-g44-cancel>Cancelar</button></div></section>`;overlay.appendChild(dialog);$('[data-g44-save-exit]',dialog).onclick=()=>player()?.close?.();$('[data-g44-nosave-exit]',dialog).onclick=noSaveExit;$('[data-g44-cancel]',dialog).onclick=closeExitDialog;setTimeout(()=>$('[data-g44-save-exit]',dialog)?.focus(),20)}
  document.addEventListener('click',e=>{const close=e.target.closest?.('#game-player-overlay [data-game-close]');if(!close)return;e.preventDefault();e.stopPropagation();e.stopImmediatePropagation();showExitDialog()},true);

  /* ---------- Games rails on the normal StormFlix home ---------- */
  let homeCache=null,homeCacheAt=0,homeBusy=false,rowsObserver=null;
  async function gameHome(){if(homeCache&&Date.now()-homeCacheAt<30000)return homeCache;const r=await fetch('/api/v1/games/home',{credentials:'same-origin',cache:'no-store'});if(!r.ok)throw new Error(`HTTP ${r.status}`);homeCache=await r.json();homeCacheAt=Date.now();return homeCache}
  function gameHomeCard(g){return `<article class="sf-home-game-card"><button type="button" data-g44-home-game="${Number(g.id)}"><span class="sf-home-game-cover">${g.cover_url?`<img src="${attr(g.cover_url)}" alt="" loading="lazy">`:`<i>${esc(short[g.platform]||'GAME')}</i>`}${g.play_seconds?'<em>Continuar</em>':''}</span><strong>${esc(g.title)}</strong><small>${esc(short[g.platform]||String(g.platform||'').toUpperCase())}${g.play_seconds?` · ${esc(time(g.play_seconds))}`:''}</small></button></article>`}
  function gameHomeRow(title,items,kind){return `<section class="content-row sf-home-games-row" data-g44-home-row="${kind}"><div class="row-head"><h2>${esc(title)}</h2><span>${items.length} jogo${items.length===1?'':'s'}</span></div><div class="sf-home-games-track">${items.slice(0,20).map(gameHomeCard).join('')}</div></section>`}
  async function injectHomeRows(){const rows=$('#rows');if(!rows||document.body.classList.contains('games-mode')||!$('[data-nav="home"]')?.classList.contains('active')||homeBusy)return;homeBusy=true;try{const home=await gameHome();rows.querySelectorAll('[data-g44-home-row]').forEach(n=>n.remove());const continued=home?.continue_playing||[],recent=home?.recently_added||[];if(!continued.length&&!recent.length)return;const box=document.createElement('div');box.className='sf-home-games-insert';box.innerHTML=`${continued.length?gameHomeRow('Continuar jogando',continued,'continue'):''}${recent.length?gameHomeRow('Jogos adicionados recentemente',recent,'recent'):''}`;rows.append(...box.children);$$('[data-g44-home-game]',rows).forEach(b=>b.onclick=()=>openGameDetail(Number(b.dataset.g44HomeGame)))}catch{}finally{homeBusy=false}}
  function startRowsObserver(){const rows=$('#rows');if(!rows||rowsObserver)return;let timer=0;rowsObserver=new MutationObserver(mutations=>{const external=mutations.some(m=>[...m.addedNodes,...m.removedNodes].some(n=>!(n.nodeType===1&&n.matches?.('[data-g44-home-row]'))));if(!external)return;clearTimeout(timer);timer=setTimeout(injectHomeRows,80)});rowsObserver.observe(rows,{childList:true});injectHomeRows()}
  function openGameDetail(id){const nav=$('#games-nav');if(!nav)return;nav.click();let tries=0;const seek=()=>{const card=$(`#games-view [data-game-open="${Number(id)}"]`);if(card){card.click();return}if(++tries===20)$('#games-view [data-gx-screen="library"]')?.click();if(tries<70)setTimeout(seek,70)};setTimeout(seek,80)}
  function restoreDetailAfterNoSave(){const raw=sessionStorage.getItem('stormflix.games.g44.return-detail');if(!raw)return;sessionStorage.removeItem('stormflix.games.g44.return-detail');let tries=0;const wait=()=>{if(!$('#shell')?.classList.contains('hidden')&&$('#games-nav')){openGameDetail(Number(raw));return}if(++tries<80)setTimeout(wait,80)};wait()}
  window.addEventListener('stormflix:game-closed',()=>{homeCache=null;homeCacheAt=0;setTimeout(injectHomeRows,100)});window.addEventListener('stormflix:profile',()=>{homeCache=null;homeCacheAt=0;setTimeout(injectHomeRows,100)});

  /* ---------- Metadata screenshots + trailer in the G4.3 detail page ---------- */
  const detailMediaCache=new Map();let detailObserver=null,detailTimer=0;
  async function detailMedia(id){if(detailMediaCache.has(id))return detailMediaCache.get(id);const r=await fetch(`/api/v1/games/${Number(id)}`,{credentials:'same-origin',cache:'no-store'});if(!r.ok)throw new Error(`HTTP ${r.status}`);const data=await r.json();detailMediaCache.set(id,data);return data}
  function showShotLightbox(url,title){let light=$('[data-g44-lightbox]');if(light)light.remove();light=document.createElement('div');light.className='sf-g44-lightbox';light.dataset.g44Lightbox='1';light.innerHTML=`<button type="button" aria-label="Fechar">✕</button><img src="${attr(url)}" alt="Screenshot de ${attr(title)}">`;document.body.appendChild(light);light.onclick=e=>{if(e.target===light||e.target.closest('button'))light.remove()}}
  async function enhanceDetail(){const page=$('#games-view .gx-game-page');if(!page)return;const id=Number(page.dataset.g43GamePage);if(!id||page.dataset.g44Media==='loading'||page.dataset.g44Media==='done')return;page.dataset.g44Media='loading';try{const game=await detailMedia(id);if(!page.isConnected||Number(page.dataset.g43GamePage)!==id)return;const actions=$('.gx-g43-actions',page);if(game.trailer_url&&actions&&!$('[data-g44-trailer]',page)){const a=document.createElement('a');a.className='gx-g44-trailer';a.dataset.g44Trailer='1';a.href=game.trailer_url;a.target='_blank';a.rel='noopener noreferrer';a.textContent='▷ Trailer';actions.appendChild(a)}const shots=(game.screenshots||[]).filter(Boolean).slice(0,8);if(shots.length&&!$('[data-g44-media]',page)){const media=document.createElement('section');media.className='gx-g44-media';media.dataset.g44Media='1';media.innerHTML=`<div class="gx-g44-media-head"><div><p>MÍDIA DO JOGO</p><h2>Screenshots</h2></div>${game.genres?.length?`<span>${game.genres.slice(0,4).map(esc).join(' · ')}</span>`:''}</div><div class="gx-g44-shot-track">${shots.map((src,i)=>`<button type="button" data-g44-shot="${i}"><img src="${attr(src)}" alt="Screenshot ${i+1}" loading="lazy"></button>`).join('')}</div>`;page.appendChild(media);$$('[data-g44-shot]',media).forEach(b=>b.onclick=()=>showShotLightbox(shots[Number(b.dataset.g44Shot)],game.title))}page.dataset.g44Media='done'}catch{page.dataset.g44Media='done'}}
  function startDetailObserver(){const host=$('#games-view .gx-content');if(!host||detailObserver)return;detailObserver=new MutationObserver(()=>{clearTimeout(detailTimer);detailTimer=setTimeout(enhanceDetail,40)});detailObserver.observe(host,{childList:true,subtree:false});enhanceDetail()}

  document.addEventListener('click',e=>{if(e.target.closest?.('[data-nav="home"],#brand-home'))setTimeout(injectHomeRows,100);if(e.target.closest?.('#games-nav')){setTimeout(startDetailObserver,120);setTimeout(enhanceDetail,320)}},true);

  function boot(){startRowsObserver();startDetailObserver();restoreDetailAfterNoSave();setTimeout(()=>{startRowsObserver();startDetailObserver();injectHomeRows()},500)}
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();

/* StormFlix source: games-g45-home-compat.js */
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

/* StormFlix source: games-g48-home.js */
/* StormFlix Games G4.8: personalized Games Home dashboard.
 * Keeps the latest session as the only hero, moves the remaining active games
 * into a compact activity panel and progressively de-duplicates every rail.
 */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  let observer=null,timer=0;

  function sectionByTitle(home,title){
    return $$(':scope > .gx-section',home).find(section=>$('h2',section)?.textContent.trim()===title)||null;
  }
  function gameID(node){
    const button=node?.matches?.('[data-game-open]')?node:node?.querySelector?.('[data-game-open]');
    const id=Number(button?.dataset.gameOpen||0);
    return Number.isFinite(id)&&id>0?id:0;
  }
  function removeDuplicateCards(section,seen){
    if(!section)return;
    for(const card of $$('.gx-card',section)){
      const id=gameID(card);
      if(!id||seen.has(id)){card.remove();continue}
      seen.add(id);
    }
    if(!section.querySelector('.gx-card'))section.remove();
  }
  function quickTile(screen,icon,title,copy){
    const button=document.createElement('button');
    button.type='button';button.className='g48-quick-card';button.dataset.g48Screen=screen;
    button.innerHTML=`<span>${icon}</span><strong>${title}</strong><small>${copy}</small>`;
    button.onclick=()=>document.querySelector(`#games-view [data-gx-screen="${screen}"]`)?.click();
    return button;
  }
  function buildActivityPanel(activeCards){
    const aside=document.createElement('aside');aside.className='g48-activity';
    const head=document.createElement('div');head.className='g48-activity-head';
    head.innerHTML=`<div><p>SUA ATIVIDADE</p><h2>${activeCards.length?'Continue seus outros jogos':'Acesso rápido'}</h2></div><small>${activeCards.length?'Retome outra partida sem repetir o destaque.':'Sua biblioteca e seus saves em um toque.'}</small>`;
    const grid=document.createElement('div');grid.className='g48-activity-grid';
    activeCards.forEach(card=>{card.classList.add('g48-continue-card');grid.appendChild(card)});
    const shortcuts=[
      ['library','▦','Biblioteca','Ver todos os jogos'],
      ['saves','▤','Saves','Continuar de outro save'],
      ['collections','▣','Coleções','Explorar séries e franquias']
    ];
    let i=0;
    while(grid.children.length<4&&i<shortcuts.length){const q=shortcuts[i++];grid.appendChild(quickTile(...q))}
    aside.append(head,grid);return aside;
  }
  function enhanceHome(){
    const home=$('#games-view .gx-home');
    if(!home||home.dataset.g48Enhanced==='1')return;
    const hero=$(':scope > .gx-hero',home);
    if(!hero)return;
    home.dataset.g48Enhanced='1';

    const continued=sectionByTitle(home,'Continuar jogando');
    const heroId=gameID(hero);
    const continuedCards=continued?$$('.gx-card',continued):[];
    const hasProgress=continuedCards.some(card=>gameID(card)===heroId)||/CONTINUAR/i.test($('.gx-hero-copy>p:first-child',hero)?.textContent||'');
    const otherActive=[];
    for(const card of continuedCards){
      if(gameID(card)===heroId)card.remove();
      else otherActive.push(card);
    }
    const shownActive=otherActive.slice(0,4);
    continued?.remove();

    const eyebrow=$('.gx-hero-copy>p:first-child',hero);
    if(eyebrow)eyebrow.textContent=hasProgress?'RETOMAR ÚLTIMA PARTIDA':'DESTAQUE DA SUA BIBLIOTECA';
    const hint=$('.gx-hero-copy>small',hero);
    if(hint){hint.classList.add('g48-hero-action');hint.textContent=hasProgress?'▶ Continuar partida':'▶ Abrir jogo'}

    const dashboard=document.createElement('section');dashboard.className='g48-dashboard';
    hero.before(dashboard);dashboard.append(hero,buildActivityPanel(shownActive));

    const seen=new Set();if(heroId)seen.add(heroId);shownActive.forEach(card=>{const id=gameID(card);if(id)seen.add(id)});
    const favorites=sectionByTitle(home,'Favoritos');
    const recent=sectionByTitle(home,'Adicionados recentemente');
    const ready=sectionByTitle(home,'Prontos para jogar');
    removeDuplicateCards(favorites,seen);
    removeDuplicateCards(recent,seen);
    removeDuplicateCards(ready,seen);

    const readyAlive=ready?.isConnected?ready:null;
    if(readyAlive){const title=$('h2',readyAlive);if(title)title.textContent='Explore sua biblioteca'}

    /* Priority is intentional: active session -> favorites -> recent -> discovery.
       This prevents the catch-all library rail from consuming every game before
       the curated rows get a chance to show something useful. */
    for(const section of [favorites,recent,readyAlive])if(section?.isConnected)home.appendChild(section);
  }
  function schedule(){clearTimeout(timer);timer=setTimeout(enhanceHome,30)}
  function boot(){
    const root=$('#games-view');if(!root)return;
    observer=new MutationObserver(schedule);observer.observe(root,{childList:true,subtree:true});
    document.addEventListener('click',e=>{if(e.target.closest?.('#games-nav,[data-gx-screen="home"]'))setTimeout(schedule,60)},true);
    window.addEventListener('stormflix:profile',()=>setTimeout(schedule,80));
    schedule();
  }
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();

/* G4.9 is loaded from G4.8 so older cached shells keep receiving the new
 * Collections behavior after the normal hard refresh used for deployments. */
(function(){
  if(document.querySelector('script[data-games-g49]'))return;
  const script=document.createElement('script');
  script.src='/games-g49-collections.js?v=g49';script.defer=true;script.dataset.gamesG49='1';
  document.head.appendChild(script);
})();

/* StormFlix source: games-g49-collections.js */
/* StormFlix Games G4.9: smart, cross-platform collections.
 * Replaces the old "one platform = one collection" view with automatic
 * title-family collections. The algorithm is intentionally local: it never
 * exposes metadata provider credentials and works with the catalog already in
 * the browser. Games from different platforms can belong to the same family.
 */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  const labels={nes:'Nintendo Entertainment System',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'};
  const short={nes:'NES',snes:'SNES',genesis:'GEN',gb:'GB',gbc:'GBC',gba:'GBA'};
  let gamesCache=null,loading=null,timer=0,observer=null;
  const sequelToken=/^(?:\d{1,4}|[ivxlcdm]{1,7})$/i;
  const sequelWord=/^(?:part|parte|episode|episodio|episódio|chapter|capitulo|capítulo|volume|vol)$/i;
  const editionWords=new Set(['edition','edicao','edição','version','versao','versão','remastered','remaster','rev','beta','demo','prototype']);
  const genericPairs=new Set(['super nintendo','sega genesis','mega drive','game boy','video game']);
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const attr=s=>esc(s).replace(/`/g,'&#96;');

  function inCollections(){
    return document.body.classList.contains('games-mode')&&$('#games-view [data-gx-screen="collections"]')?.classList.contains('active');
  }
  function fold(value){
    return String(value||'').normalize('NFD').replace(/[\u0300-\u036f]/g,'').toLocaleLowerCase('pt-BR').replace(/&/g,' and ').replace(/[^a-z0-9]+/g,' ').trim().replace(/\s+/g,' ');
  }
  function tokens(game){return fold(game?.title).split(' ').filter(Boolean)}
  function trimEditionTail(list){
    const out=[...list];
    while(out.length>1&&editionWords.has(out[out.length-1]))out.pop();
    return out;
  }
  function sequelBase(list){
    let out=trimEditionTail(list);
    if(out.length>1&&sequelToken.test(out[out.length-1]))out=out.slice(0,-1);
    if(out.length>2&&sequelWord.test(out[out.length-1]))out=out.slice(0,-1);
    if(out.length>2&&sequelWord.test(out[out.length-2])&&sequelToken.test(out[out.length-1]))out=out.slice(0,-2);
    return out;
  }
  function commonPrefix(a,b){
    const n=Math.min(a.length,b.length);let i=0;
    for(;i<n&&a[i]===b[i];i++);
    return a.slice(0,i);
  }
  function allowedPrefix(prefix,a,b){
    if(prefix.length>=2){
      const key=prefix.join(' ');
      return key.length>=6&&!genericPairs.has(key);
    }
    if(prefix.length!==1)return false;
    const base=prefix[0];
    const aTail=a.slice(1),bTail=b.slice(1);
    const sequelish=x=>x.length===0||(x.length===1&&sequelToken.test(x[0]))||(x.length===2&&sequelWord.test(x[0])&&sequelToken.test(x[1]));
    return base.length>=5&&sequelish(aTail)&&sequelish(bTail);
  }
  function displayName(key,members){
    const count=key.split(' ').length;
    for(const game of members){
      const raw=String(game.title||'').replace(/[_\.]+/g,' ').trim();
      const words=raw.split(/\s+/);
      if(words.length>=count)return words.slice(0,count).join(' ').replace(/[\s:;,-]+$/,'');
    }
    return key.replace(/\b\w/g,c=>c.toUpperCase());
  }
  function buildCollections(games){
    const rows=games.map(game=>({game,tokens:trimEditionTail(tokens(game))})).filter(x=>x.tokens.length);
    const candidates=new Map();
    const add=(key,...members)=>{
      key=String(key||'').trim();if(!key)return;
      let set=candidates.get(key);if(!set){set=new Map();candidates.set(key,set)}
      for(const row of members)if(row?.game?.id)set.set(Number(row.game.id),row.game);
    };

    // Explicit sequel families (ActRaiser / ActRaiser 2, Metroid / Metroid II…).
    for(const row of rows){
      const base=sequelBase(row.tokens);
      if(base.length===row.tokens.length)continue;
      const key=base.join(' ');
      for(const other of rows){
        const otherBase=sequelBase(other.tokens).join(' ');
        if(otherBase===key)add(key,row,other);
      }
    }

    // Franchise-style common prefixes. This is what lets Donkey Kong titles
    // from SNES, Game Boy and other platforms land in the same collection.
    for(let i=0;i<rows.length;i++)for(let j=i+1;j<rows.length;j++){
      const prefix=commonPrefix(rows[i].tokens,rows[j].tokens);
      if(!allowedPrefix(prefix,rows[i].tokens,rows[j].tokens))continue;
      add(prefix.join(' '),rows[i],rows[j]);
    }

    // Expand every candidate to every compatible game, independent of platform.
    for(const [key,set] of candidates){
      const prefix=key.split(' ');
      for(const row of rows){
        const base=sequelBase(row.tokens);
        const exactBase=base.join(' ')===key;
        const starts=row.tokens.length>=prefix.length&&prefix.every((token,i)=>row.tokens[i]===token);
        if(exactBase||starts)set.set(Number(row.game.id),row.game);
      }
    }

    let groups=[...candidates.entries()].map(([key,set])=>({key,games:[...set.values()]})).filter(g=>g.games.length>=2);
    groups.sort((a,b)=>a.key.split(' ').length-b.key.split(' ').length||b.games.length-a.games.length||a.key.localeCompare(b.key,'pt-BR'));
    const accepted=[];
    for(const group of groups){
      const ids=new Set(group.games.map(g=>Number(g.id)));
      const redundant=accepted.some(parent=>{
        const pids=new Set(parent.games.map(g=>Number(g.id)));let overlap=0;
        ids.forEach(id=>{if(pids.has(id))overlap++});
        return overlap/ids.size>=.8&&parent.key.split(' ').length<=group.key.split(' ').length;
      });
      if(redundant)continue;
      group.name=displayName(group.key,group.games);
      group.platforms=[...new Set(group.games.map(g=>g.platform).filter(Boolean))].sort((a,b)=>(labels[a]||a).localeCompare(labels[b]||b,'pt-BR'));
      accepted.push(group);
    }
    return accepted.sort((a,b)=>b.games.length-a.games.length||a.name.localeCompare(b.name,'pt-BR'));
  }
  async function allGames(){
    if(gamesCache)return gamesCache;
    if(loading)return loading;
    loading=(async()=>{
      const r=await fetch('/api/v1/games?limit=500',{credentials:'same-origin',cache:'no-store'});
      const text=await r.text();let data=[];try{data=JSON.parse(text)}catch{}
      if(!r.ok)throw new Error(data?.error||`HTTP ${r.status}`);
      gamesCache=Array.isArray(data)?data:(Array.isArray(data?.items)?data.items:[]);
      return gamesCache;
    })().finally(()=>loading=null);
    return loading;
  }
  function cover(game){return game.cover_url?`<img src="${attr(game.cover_url)}" alt="" loading="lazy">`:`<span class="g49-cover-fallback"><b>${esc(short[game.platform]||'GAME')}</b><small>STORMFLIX</small></span>`}
  function mosaic(games){
    const art=games.filter(Boolean).slice(0,4);
    while(art.length<4)art.push(null);
    return `<span class="g49-mosaic">${art.map(g=>`<i>${g?cover(g):'<span class="g49-empty-art"></span>'}</i>`).join('')}</span>`;
  }
  function platformChips(platforms){return platforms.slice(0,4).map(p=>`<span>${esc(short[p]||String(p).toUpperCase())}</span>`).join('')+(platforms.length>4?`<span>+${platforms.length-4}</span>`:'')}
  function gameCard(game){return `<article class="gx-card g49-game-card"><button type="button" data-game-open="${Number(game.id)}"><span class="gx-cover">${cover(game)}</span><strong>${esc(game.title)}</strong><small><b>${esc(short[game.platform]||String(game.platform||'').toUpperCase())}</b> ${esc(labels[game.platform]||game.platform||'')}</small></button></article>`}

  function renderCollectionDetail(host,group,groups,games){
    host.dataset.g49Mode='detail';
    host.innerHTML=`<section class="gx-page g49-page"><div class="g49-detail-head"><button type="button" data-g49-back>← Coleções</button><div><p>SÉRIE / FRANQUIA</p><h1>${esc(group.name)}</h1><div class="g49-platform-chips">${platformChips(group.platforms)}</div><small>${group.games.length} jogo(s) encontrados em ${group.platforms.length} plataforma(s).</small></div></div><div class="g49-game-grid">${group.games.slice().sort((a,b)=>(a.release_year||9999)-(b.release_year||9999)||String(a.title).localeCompare(String(b.title),'pt-BR')).map(gameCard).join('')}</div></section>`;
    $('[data-g49-back]',host)?.addEventListener('click',()=>renderIndex(host,groups,games));
    window.scrollTo({top:0,behavior:'auto'});
  }
  function renderPlatformDetail(host,platform,groups,games){
    const items=games.filter(g=>g.platform===platform);
    const group={name:labels[platform]||platform,games:items,platforms:[platform]};
    renderCollectionDetail(host,group,groups,games);
    const kicker=$('.g49-detail-head p',host);if(kicker)kicker.textContent='PLATAFORMA';
  }
  function renderIndex(host,groups,games){
    host.dataset.g49Mode='index';
    const counts=new Map();for(const g of games)counts.set(g.platform,(counts.get(g.platform)||0)+1);
    const platforms=[...counts.entries()].sort((a,b)=>(labels[a[0]]||a[0]).localeCompare(labels[b[0]]||b[0],'pt-BR'));
    host.innerHTML=`<section class="gx-page g49-page"><div class="gx-page-head g49-head"><div><p>SÉRIES E FRANQUIAS</p><h1>Coleções</h1><small>Jogos relacionados são agrupados pelo título, mesmo quando estão em plataformas diferentes.</small></div><label class="gx-search g49-search"><span>⌕</span><input data-g49-search placeholder="Buscar coleção…" autocomplete="off"></label></div><div class="g49-section-head"><div><h2>Coleções automáticas</h2><small>${groups.length} coleção(ões) detectada(s)</small></div></div><div class="g49-collection-grid" data-g49-grid>${groups.map((group,index)=>`<button class="g49-collection-card" type="button" data-g49-collection="${index}">${mosaic(group.games)}<span class="g49-collection-copy"><strong>${esc(group.name)}</strong><small>${group.games.length} jogo(s)</small><span class="g49-platform-chips">${platformChips(group.platforms)}</span></span></button>`).join('')||'<div class="gx-empty small g49-empty"><h2>Nenhuma série detectada ainda</h2><p>Quando existirem dois ou mais títulos relacionados, eles aparecerão aqui automaticamente.</p></div>'}</div><div class="g49-section-head g49-platform-title"><div><h2>Explorar por plataforma</h2><small>Plataforma é um filtro de catálogo, não uma coleção.</small></div></div><div class="g49-platform-grid">${platforms.map(([platform,count])=>{const items=games.filter(g=>g.platform===platform);return`<button type="button" data-g49-platform="${attr(platform)}">${mosaic(items)}<span><strong>${esc(labels[platform]||platform)}</strong><small>${count} jogo(s)</small></span></button>`}).join('')}</div></section>`;
    $$('[data-g49-collection]',host).forEach(button=>button.addEventListener('click',()=>renderCollectionDetail(host,groups[Number(button.dataset.g49Collection)],groups,games)));
    $$('[data-g49-platform]',host).forEach(button=>button.addEventListener('click',()=>renderPlatformDetail(host,button.dataset.g49Platform,groups,games)));
    const input=$('[data-g49-search]',host);input?.addEventListener('input',()=>{
      const q=fold(input.value);$$('[data-g49-collection]',host).forEach(button=>{const group=groups[Number(button.dataset.g49Collection)];button.hidden=!!q&&!fold(`${group.name} ${group.platforms.map(p=>labels[p]||p).join(' ')}`).includes(q)});
    });
    host.querySelectorAll('img').forEach(img=>img.addEventListener('error',()=>img.classList.add('broken'),{once:true}));
  }
  async function enhance(){
    if(!inCollections())return;
    const host=$('#games-view .gx-content');if(!host||host.dataset.g49Mode)return;
    host.dataset.g49Mode='loading';
    host.innerHTML='<div class="gx-inline-loader"><span></span>Organizando séries e franquias…</div>';
    try{const games=await allGames();if(!inCollections())return;renderIndex(host,buildCollections(games),games)}catch(err){host.dataset.g49Mode='error';host.innerHTML=`<section class="gx-empty small"><h2>Não foi possível montar as coleções</h2><p>${esc(err.message||err)}</p></section>`}
  }
  function schedule(){clearTimeout(timer);timer=setTimeout(()=>{const host=$('#games-view .gx-content');if(host&&!inCollections())delete host.dataset.g49Mode;enhance()},45)}
  function boot(){
    const root=$('#games-view');if(!root)return;
    observer=new MutationObserver(schedule);observer.observe(root,{childList:true,subtree:true});
    document.addEventListener('click',e=>{if(e.target.closest?.('#games-view [data-gx-screen="collections"]'))setTimeout(()=>{const host=$('#games-view .gx-content');if(host)delete host.dataset.g49Mode;schedule()},20)},true);
    window.addEventListener('stormflix:profile',()=>{gamesCache=null;setTimeout(schedule,80)});
    schedule();
  }
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();
