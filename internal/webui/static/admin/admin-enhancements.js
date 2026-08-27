/* StormFlix metadata agent hints. Library editor is handled by admin-browser.js. */
(function(){
  if(typeof loadMetadataPhase2!=='function')return;
  const baseLoadMetadata=loadMetadataPhase2;
  loadMetadataPhase2=async function(){
    await baseLoadMetadata();
    const grid=document.querySelector('#metadata .agent-grid');
    if(grid&&!grid.querySelector('[data-anidb-agent]')){
      const card=document.createElement('div');card.className='agent-card';card.dataset.anidbAgent='1';
      card.innerHTML='<div class="agent-status ready">● ATIVO</div><h3>AniDB</h3><p>Resolvedor de nomes e aliases de anime antes dos fallbacks de identificação.</p>';
      grid.insertBefore(card,grid.children[2]||null);
    }
    if(grid&&!grid.querySelector('[data-mal-agent]')){
      const card=document.createElement('div');card.className='agent-card';card.dataset.malAgent='1';
      card.innerHTML='<div class="agent-status ready">● ATIVO</div><h3>MyAnimeList</h3><p>Fallback para animes que AniList/TMDB não encontraram, recuperando identidade, sinopse, nota e capas.</p>';
      grid.insertBefore(card,grid.children[3]||null);
    }
    if(grid&&!grid.querySelector('[data-animeapi-agent]')){
      const card=document.createElement('div');card.className='agent-card';card.dataset.animeapiAgent='1';
      card.innerHTML='<div class="agent-status ready">● FALLBACK</div><h3>AnimeAPI</h3><p>Ponte opcional entre IDs AniList, MAL, AniDB, TMDB, TVDB e IMDb. Se estiver offline, o scan continua normalmente.</p>';
      grid.insertBefore(card,grid.children[4]||null);
    }
    document.querySelectorAll('#metadata tbody tr').forEach(row=>{
      const name=row.querySelector('td b')?.textContent||'';const lib=libs.find(x=>x.name===name);if(!lib)return;
      const typeCell=row.children[1];if(!typeCell)return;
      const strategy=lib.kind==='mixed'?'TMDB → AniDB → MyAnimeList → HAMA/Anime-Lists → Fanart':lib.kind==='anime_series'?'Scanner de pasta → TMDB Séries → TheTVDB → AniList/AniDB/MAL → HAMA/Anime-Lists → Fanart':lib.kind==='animation_series'?'Scanner de pasta → TMDB TV → TheTVDB → Fanart':lib.kind==='anime'?'AniList → HAMA/Anime-Lists → TMDB/TheTVDB → Fanart':lib.kind==='series'?'Scanner de pasta → TMDB TV → TheTVDB':'TMDB + Fanart';
      if(!typeCell.querySelector('.agent-strategy'))typeCell.insertAdjacentHTML('beforeend',`<small class="agent-strategy">${esc(strategy)}</small>`);
    });
  };
})();