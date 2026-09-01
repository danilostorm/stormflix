/* StormFlix Games G2: native profile-aware catalog + browser player. */
(function(){
  const root=document.querySelector('#games-view');
  const nav=document.querySelector('#games-nav');
  if(!root||!nav)return;

  let home=null,games=[],tab='home',platform='',query='',loading=false;
  const labels={nes:'Nintendo',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'};
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const attr=s=>esc(s).replace(/`/g,'&#96;');
  const playtime=s=>{s=Math.max(0,Math.floor(Number(s)||0));if(!s)return '';const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${Math.max(1,m)} min`};

  async function openGames(){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    if(typeof stopTheme==='function')stopTheme();
    document.querySelector('#hero')?.classList.add('hidden');
    document.querySelector('#search-view')?.classList.add('hidden');
    document.querySelector('#catalog-view')?.classList.add('hidden');
    document.querySelector('#music-view')?.classList.add('hidden');
    root.classList.remove('hidden');
    document.querySelectorAll('.main-nav button').forEach(b=>b.classList.toggle('active',b===nav));
    window.scrollTo({top:0,behavior:'auto'});
    await load();
  }

  function closeGames(){root.classList.add('hidden')}

  async function load(){
    if(loading)return;
    loading=true;
    root.innerHTML='<div class="games-shell"><div class="games-empty"><span class="games-console">▣</span><h2>Carregando sua coleção…</h2><p>Organizando plataformas, saves e identidade das ROMs.</p></div></div>';
    try{
      [home,games]=await Promise.all([request('/games/home'),request('/games?limit=500')]);
      render();
    }catch(err){
      root.innerHTML=`<div class="games-shell"><div class="games-empty"><span class="games-console">!</span><h2>Não foi possível abrir Jogos</h2><p>${esc(err.message)}</p></div></div>`;
    }finally{loading=false}
  }

  function visibleGames(){
    let list=[...games];
    if(tab==='favorites')list=list.filter(g=>g.favorite);
    if(platform)list=list.filter(g=>g.platform===platform);
    if(query){const q=query.toLocaleLowerCase('pt-BR');list=list.filter(g=>String(g.title||'').toLocaleLowerCase('pt-BR').includes(q)||String(g.library||'').toLocaleLowerCase('pt-BR').includes(q)||String(labels[g.platform]||g.platform||'').toLocaleLowerCase('pt-BR').includes(q))}
    return list;
  }

  function render(){
    if(!games.length){
      root.innerHTML=`<div class="games-shell"><header class="games-hero"><div><p class="games-kicker">STORMFLIX JOGOS</p><h1>Seu arcade pessoal começa aqui.</h1><p>Catálogo nativo por plataforma e SHA-256, com perfis separados e player RetroArch/WASM integrado.</p></div></header><div class="games-empty"><span class="games-console">▣</span><h2>Nenhum jogo catalogado</h2><p>No Admin, crie uma biblioteca do tipo <b>Jogos</b>, escolha a pasta das suas ROMs e execute o scan. O StormFlix não inclui nem distribui ROMs ou BIOS.</p></div></div>`;
      return;
    }
    root.innerHTML=`<div class="games-shell">
      <header class="games-hero"><div><p class="games-kicker">STORMFLIX JOGOS · G2</p><h1>Seu catálogo. Seus saves. Seu perfil.</h1><p>Player RetroArch/WASM no navegador, gamepad, save state/SRAM sincronizados e tempo jogado por perfil.</p></div><label class="games-search"><span>⌕</span><input id="games-search" value="${attr(query)}" placeholder="Buscar jogo ou plataforma…" autocomplete="off"></label></header>
      <nav class="games-tabs" aria-label="Seções de jogos"><button class="${tab==='home'?'active':''}" data-games-tab="home">Início</button><button class="${tab==='all'?'active':''}" data-games-tab="all">Todos</button><button class="${tab==='favorites'?'active':''}" data-games-tab="favorites">Favoritos</button></nav>
      <div class="games-platforms"><button class="${platform===''?'active':''}" data-platform="">Todas</button>${(home?.platforms||[]).map(p=>`<button class="${platform===p.platform?'active':''}" data-platform="${attr(p.platform)}">${esc(p.label)} <span>${Number(p.count||0)}</span></button>`).join('')}</div>
      <div id="games-content">${contentHTML()}</div>
    </div>`;
    root.querySelector('#games-search').oninput=e=>{query=e.target.value.trim();renderContent()};
    root.querySelectorAll('[data-games-tab]').forEach(b=>b.onclick=()=>{tab=b.dataset.gamesTab;render()});
    root.querySelectorAll('[data-platform]').forEach(b=>b.onclick=()=>{platform=b.dataset.platform||'';render()});
    bindCards();
  }

  function contentHTML(){
    const filtered=visibleGames();
    if(query||platform||tab==='all'||tab==='favorites'){
      const title=tab==='favorites'?'Favoritos':platform?(labels[platform]||platform):'Todos os jogos';
      return section(title,`${filtered.length} jogo(s)`,grid(filtered));
    }
    const parts=[];
    const continued=(home?.continue_playing||[]).filter(applyCurrentFilter);
    const favorites=(home?.favorites||[]).filter(applyCurrentFilter);
    const recent=(home?.recently_added||[]).filter(applyCurrentFilter);
    if(continued.length)parts.push(section('Continuar jogando','Retome seus jogos e saves neste perfil',rail(continued)));
    if(favorites.length)parts.push(section('Favoritos','Sua seleção neste perfil',rail(favorites)));
    if(recent.length)parts.push(section('Adicionados recentemente',`${recent.length} jogo(s)`,grid(recent)));
    return parts.join('')||'<div class="games-empty"><h2>Nenhum jogo neste filtro</h2></div>';
  }

  function applyCurrentFilter(g){return !platform||g.platform===platform}
  function renderContent(){const node=root.querySelector('#games-content');if(!node)return;node.innerHTML=contentHTML();bindCards()}
  function section(title,sub,body){return `<section class="games-section"><div class="games-section-head"><div><h2>${esc(title)}</h2><p>${esc(sub)}</p></div></div>${body}</section>`}
  function rail(items){return `<div class="games-rail">${items.slice(0,18).map(card).join('')}</div>`}
  function grid(items){return items.length?`<div class="games-grid">${items.map(card).join('')}</div>`:'<div class="games-empty compact"><h3>Nenhum jogo encontrado</h3><p>Ajuste a busca ou o filtro de plataforma.</p></div>'}

  function card(g){
    const cover=g.cover_url?`<img src="${attr(g.cover_url)}" alt="" loading="lazy">`:`<span class="games-cover-fallback"><b>${esc(shortPlatform(g.platform))}</b><i>STORMFLIX</i></span>`;
    const time=playtime(g.play_seconds);
    return `<article class="game-card" data-game="${Number(g.id)}"><button class="game-card-main" type="button" data-game-open="${Number(g.id)}"><span class="game-cover">${cover}<span class="game-platform-chip">${esc(shortPlatform(g.platform))}</span>${g.last_played_at?'<span class="game-resume-chip">▶ Continuar</span>':''}</span><span class="game-copy"><strong>${esc(g.title)}</strong><small>${esc(labels[g.platform]||g.platform)} · ${esc(g.library||'Jogos')}${time?` · ${esc(time)}`:''}</small></span></button><button class="game-favorite ${g.favorite?'on':''}" type="button" data-game-favorite="${Number(g.id)}" aria-label="${g.favorite?'Remover dos favoritos':'Adicionar aos favoritos'}">${g.favorite?'♥':'♡'}</button></article>`;
  }

  function shortPlatform(p){return ({nes:'NES',snes:'SNES',genesis:'MD',gb:'GB',gbc:'GBC',gba:'GBA'}[p]||String(p||'GAME').toUpperCase())}

  function bindCards(){
    root.querySelectorAll('.game-cover img').forEach(img=>img.onerror=()=>{const wrap=img.closest('.game-cover');if(wrap){img.remove();const f=document.createElement('span');f.className='games-cover-fallback';f.innerHTML='<b>GAME</b><i>STORMFLIX</i>';wrap.prepend(f)}});
    root.querySelectorAll('[data-game-open]').forEach(b=>b.onclick=()=>openDetail(Number(b.dataset.gameOpen)));
    root.querySelectorAll('[data-game-favorite]').forEach(b=>b.onclick=()=>toggleFavorite(Number(b.dataset.gameFavorite)));
  }

  async function toggleFavorite(id){
    const game=games.find(g=>Number(g.id)===id);if(!game)return;
    const next=!game.favorite;
    try{
      await request(`/games/${id}/favorite`,{method:'POST',body:JSON.stringify({favorite:next})});
      game.favorite=next;
      for(const group of [home?.favorites,home?.recently_added,home?.continue_playing])for(const item of group||[])if(Number(item.id)===id)item.favorite=next;
      if(home){home.favorites=(home.favorites||[]).filter(x=>Number(x.id)!==id);if(next)home.favorites.unshift({...game})}
      render();
    }catch(err){if(typeof sfToast==='function')sfToast(err.message)}
  }

  async function openDetail(id){
    let game=games.find(g=>Number(g.id)===id);
    try{game=await request(`/games/${id}`)}catch{}
    if(!game)return;
    const cover=game.cover_url?`<img src="${attr(game.cover_url)}" alt="">`:`<span class="games-cover-fallback large"><b>${esc(shortPlatform(game.platform))}</b><i>STORMFLIX</i></span>`;
    const time=playtime(game.play_seconds);
    const saveState=game.saves?.state?.exists?`Save state v${Number(game.saves.state.version||0)}`:'Sem save state';
    const sram=game.saves?.sram?.exists?`SRAM v${Number(game.saves.sram.version||0)}`:'Sem SRAM';
    const modal=document.createElement('div');modal.className='game-detail-overlay';modal.innerHTML=`<article class="game-detail" role="dialog" aria-modal="true" aria-label="${attr(game.title)}"><button class="game-detail-close" type="button" aria-label="Fechar">✕</button><div class="game-detail-cover">${cover}</div><div class="game-detail-copy"><p class="games-kicker">${esc(labels[game.platform]||game.platform)}</p><h2>${esc(game.title)}</h2><div class="game-detail-meta"><span>${esc(game.library||'Jogos')}</span>${game.release_year?`<span>${game.release_year}</span>`:''}${time?`<span>${esc(time)} jogados</span>`:''}<span>${esc(saveState)}</span><span>${esc(sram)}</span></div><p>${esc(game.overview||'Jogo identificado localmente pelo StormFlix. A identidade por hash permanece independente dos metadados externos.')}</p><div class="game-detail-actions"><button class="game-play-now primary" type="button" ${game.playable?'':'disabled'}>▶ ${game.saves?.state?.exists?'Continuar jogando':'Jogar'}</button><button class="game-favorite-detail ${game.favorite?'on':''}" type="button">${game.favorite?'♥ Favorito':'♡ Favoritar'}</button></div><small>${game.playable?'ROM autenticada pelo StormFlix · runtime fixado e armazenado localmente após o primeiro uso.':'Esta ROM não está disponível para o player G2.'}</small></div></article>`;
    document.body.appendChild(modal);
    const close=()=>modal.remove();modal.querySelector('.game-detail-close').onclick=close;modal.onclick=e=>{if(e.target===modal)close()};
    modal.querySelector('.game-favorite-detail').onclick=async()=>{close();await toggleFavorite(id)};
    const play=modal.querySelector('.game-play-now');if(play)play.onclick=()=>{close();if(window.StormFlixGamePlayer)window.StormFlixGamePlayer.open(game);else if(typeof sfToast==='function')sfToast('Game Player ainda não foi carregado.')};
    play?.focus();
  }

  nav.addEventListener('click',openGames);
  document.querySelector('#brand-home')?.addEventListener('click',closeGames);
  document.querySelector('.main-nav')?.addEventListener('click',e=>{const b=e.target.closest('button');if(b&&b!==nav)closeGames()});
  window.addEventListener('stormflix:profile',()=>{if(!root.classList.contains('hidden'))load()});
  window.addEventListener('stormflix:game-closed',()=>{if(!root.classList.contains('hidden'))load()});
})();