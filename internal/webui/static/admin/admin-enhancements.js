/* StormFlix admin enhancements: mixed animation strategy */
(function(){
  const baseEditLibrary=window.editLibrary;
  window.editLibrary=function(id){
    baseEditLibrary(id);
    const form=document.querySelector('#library-editor');if(!form)return;
    const select=form.elements[1];
    if(select&&!select.querySelector('option[value="mixed"]')){
      const option=document.createElement('option');option.value='mixed';option.textContent='Filmes + Anime misto';
      const anime=select.querySelector('option[value="anime"]');if(anime)anime.after(option);else select.appendChild(option);
    }
    const lib=libs.find(x=>Number(x.id)===Number(id));
    if(lib?.kind==='mixed')select.value='mixed';
    const hint=document.createElement('div');hint.className='wide phase2-hint library-agent-hint';
    const sync=()=>{
      if(select.value==='mixed')hint.innerHTML='<b>Identificação híbrida:</b> TMDB tenta primeiro; AniDB + MyAnimeList recuperam animes e AnimeAPI cruza IDs com TMDB/TVDB/IMDb. Fanart.tv complementa as artes.';
      else if(select.value==='anime')hint.innerHTML='<b>Anime:</b> AniList é o principal; AniDB resolve nomes, MyAnimeList recupera títulos/capas e AnimeAPI faz a ponte entre IDs. TMDB/Fanart complementam quando possível.';
      else if(select.value==='series')hint.innerHTML='<b>Séries:</b> TMDB organiza série → temporadas → episódios automaticamente.';
      else hint.innerHTML='<b>Filmes:</b> TMDB + Fanart.tv.';
    };
    select.addEventListener('change',sync);sync();form.appendChild(hint);
  };

  if(typeof loadMetadataPhase2==='function'){
    const baseLoadMetadata=loadMetadataPhase2;
    loadMetadataPhase2=async function(){
      await baseLoadMetadata();
      const grid=document.querySelector('#metadata .agent-grid');
      if(grid&&!grid.querySelector('[data-anidb-agent]')){
        const card=document.createElement('div');card.className='agent-card';card.dataset.anidbAgent='1';
        card.innerHTML='<div class="agent-status ready">● ATIVO</div><h3>AniDB</h3><p>Resolvedor de nomes e aliases de anime. É usado antes do fallback MyAnimeList para melhorar a correspondência.</p>';
        grid.insertBefore(card,grid.children[2]||null);
      }
      if(grid&&!grid.querySelector('[data-mal-agent]')){
        const card=document.createElement('div');card.className='agent-card';card.dataset.malAgent='1';
        card.innerHTML='<div class="agent-status ready">● ATIVO</div><h3>MyAnimeList</h3><p>Fallback para animes que AniList/TMDB não encontraram. Recupera identidade, sinopse, nota e principalmente capas via Jikan/MAL.</p>';
        grid.insertBefore(card,grid.children[3]||null);
      }
      if(grid&&!grid.querySelector('[data-animeapi-agent]')){
        const card=document.createElement('div');card.className='agent-card';card.dataset.animeapiAgent='1';
        card.innerHTML='<div class="agent-status ready">● FALLBACK</div><h3>AnimeAPI</h3><p>Ponte opcional de IDs entre AniDB, AniList, MyAnimeList, TMDB, TVDB e IMDb. Se estiver fora do ar, o scan continua normalmente.</p>';
        grid.insertBefore(card,grid.children[4]||null);
      }
      document.querySelectorAll('#metadata tbody tr').forEach(row=>{
        const name=row.querySelector('td b')?.textContent||'';const lib=libs.find(x=>x.name===name);if(!lib)return;
        const typeCell=row.children[1];if(!typeCell)return;
        const strategy=lib.kind==='mixed'?'TMDB → AniDB → MyAnimeList → AnimeAPI → Fanart':lib.kind==='anime'?'AniList → AniDB → MyAnimeList → AnimeAPI → TMDB':lib.kind==='series'?'TMDB (séries/episódios)':'TMDB + Fanart';
        if(!typeCell.querySelector('.agent-strategy'))typeCell.insertAdjacentHTML('beforeend',`<small class="agent-strategy">${esc(strategy)}</small>`);
      });
    };
  }
})();