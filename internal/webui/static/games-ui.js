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