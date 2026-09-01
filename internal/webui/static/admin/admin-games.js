/* StormFlix Admin: native Games library editor integration. */
(function(){
  const baseEdit=window.editLibrary;
  if(typeof baseEdit!=='function')return;

  window.editLibrary=function(id,preferredKind){
    const known=id?(libs.find(x=>Number(x.id)===Number(id))?.kind||'movies'):(preferredKind||'movies');
    baseEdit(id,preferredKind);
    const form=document.querySelector('#library-editor');
    const select=form?.querySelector('#lib-kind');
    if(!select)return;
    if(![...select.options].some(o=>o.value==='games')){
      const option=document.createElement('option');
      option.value='games';
      option.textContent='Jogos';
      select.appendChild(option);
    }
    select.value=known;
    const firstPath=form.querySelector('[data-source-path]');
    const hint=form.querySelector('#library-kind-hint');
    const sync=()=>{
      if(select.value!=='games')return;
      if(firstPath)firstPath.placeholder='/media/Jogos';
      if(hint)hint.innerHTML='<b>Jogos nativos:</b> o StormFlix cataloga ROMs fornecidas pelo dono do servidor por plataforma + SHA-256, sem misturar com filmes. G1 suporta NES, SNES, Mega Drive/Genesis, Game Boy, Game Boy Color e GBA. Capas locais podem usar o mesmo nome da ROM em JPG, PNG ou WebP. ROMs e BIOS não são fornecidas pelo StormFlix.';
    };
    select.addEventListener('change',sync);
    sync();
  };
})();