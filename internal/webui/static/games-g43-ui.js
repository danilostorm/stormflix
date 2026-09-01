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
