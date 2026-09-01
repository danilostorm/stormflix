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
