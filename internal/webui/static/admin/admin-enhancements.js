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
      if(select.value==='mixed')hint.innerHTML='<b>Identificação híbrida:</b> TMDB + AniDB + AniList. Ideal para pastas com filmes de anime e animações ocidentais misturados.';
      else if(select.value==='anime')hint.innerHTML='<b>Anime:</b> AniList é o agente principal; AniDB ajuda a resolver títulos e TMDB/Fanart enriquecem dados e artes.';
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
        card.innerHTML='<div class="agent-status ready">● ATIVO</div><h3>AniDB</h3><p>Resolvedor de títulos para anime. Trabalha junto com AniList e TMDB em bibliotecas Anime ou Filmes + Anime misto.</p>';
        grid.insertBefore(card,grid.children[2]||null);
      }
      document.querySelectorAll('#metadata tbody tr').forEach(row=>{
        const name=row.querySelector('td b')?.textContent||'';const lib=libs.find(x=>x.name===name);if(!lib)return;
        const typeCell=row.children[1];if(!typeCell)return;
        const strategy=lib.kind==='mixed'?'TMDB + AniDB + AniList':lib.kind==='anime'?'AniList + AniDB + TMDB':lib.kind==='series'?'TMDB (séries/episódios)':'TMDB + Fanart';
        if(!typeCell.querySelector('.agent-strategy'))typeCell.insertAdjacentHTML('beforeend',`<small class="agent-strategy">${esc(strategy)}</small>`);
      });
    };
  }
})();
