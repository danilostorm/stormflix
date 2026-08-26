/* StormFlix Admin: music library support without mixing it with the video catalog. */
(function(){
  const baseEdit=window.editLibrary;
  if(typeof baseEdit==='function'){
    window.editLibrary=function(id,preferredKind){
      const kind=id?(libs.find(x=>Number(x.id)===Number(id))?.kind||'movies'):(preferredKind||'movies');
      baseEdit(id);
      const form=document.querySelector('#library-editor');
      const select=form?.querySelector('#lib-kind');
      if(!select)return;
      if(![...select.options].some(o=>o.value==='music')){
        const option=document.createElement('option');option.value='music';option.textContent='Música';select.appendChild(option);
      }
      select.value=kind;
      const firstPath=form.querySelector('[data-source-path]');
      const hint=form.querySelector('#library-kind-hint');
      const syncMusicUI=()=>{
        const music=select.value==='music';
        if(firstPath)firstPath.placeholder=music?'/media/Musicas':'/media/Filmes';
        if(music&&hint)hint.innerHTML='<b>Estratégia:</b> FFprobe lê tags locais; MusicBrainz e Cover Art Archive organizam artista, álbum e capa; LRCLIB fornece letras sob demanda.';
      };
      select.addEventListener('change',syncMusicUI);
      select.dispatchEvent(new Event('change'));
      syncMusicUI();
    };
  }

  const baseMetadata=window.loadMetadataPhase2;
  if(typeof baseMetadata==='function')window.loadMetadataPhase2=async function(){await baseMetadata();await decorateMusicMetadata()};

  const baseSubtitles=window.loadSubtitlesPhase2;
  if(typeof baseSubtitles==='function')window.loadSubtitlesPhase2=async function(){await baseSubtitles();decorateMusicSubtitleRows()};

  function musicLibraries(){return (libs||[]).filter(l=>l.kind==='music')}

  window.organizeMusicNow=async function(button){
    if(button)button.disabled=true;
    try{
      const r=await req('/admin/music/index',{method:'POST',body:'{}'});
      notice(r.started?'Organização da biblioteca de música iniciada.':'A biblioteca de música já está sendo organizada.',true);
    }catch(err){notice(err.message)}finally{if(button)button.disabled=false}
  };

  async function decorateMusicMetadata(){
    const page=document.querySelector('#metadata');if(!page)return;
    const music=musicLibraries();if(!music.length)return;
    page.querySelectorAll('[data-music-agents]').forEach(x=>x.remove());
    try{
      const agents=await req('/admin/agents');
      const panel=document.createElement('div');panel.className='panel';panel.dataset.musicAgents='1';
      panel.innerHTML=`<div class="panel-head"><div><h2>Agentes de Música</h2><small>Separados dos agentes de filmes e séries para não misturar TMDB com sua discoteca.</small></div><button class="primary" data-music-index-now>Organizar música</button></div><div class="agent-grid">${(agents.music||[]).map(renderAgent).join('')}</div>`;
      const first=page.querySelector('.panel');if(first)first.after(panel);else page.prepend(panel);bindIndexButtons(panel);
    }catch{}
    for(const l of music){
      const button=page.querySelector(`[data-meta-scan="${l.id}"]`);const row=button?.closest('tr');if(!row)continue;
      const actions=row.querySelector('td:last-child');if(actions)actions.innerHTML=`<button data-music-index-now>Organizar música</button>`;
    }
    bindIndexButtons(page);
  }

  function decorateMusicSubtitleRows(){
    const page=document.querySelector('#subtitles');if(!page)return;
    for(const l of musicLibraries()){
      const button=page.querySelector(`[data-sub-job="${l.id}"]`);const row=button?.closest('tr');if(!row)continue;
      const cells=row.querySelectorAll('td');if(cells[2])cells[2].innerHTML='<small>Letras via LRCLIB</small>';if(cells[3])cells[3].innerHTML='<small>Use a área Música</small>';
    }
  }

  function bindIndexButtons(scope){scope.querySelectorAll('[data-music-index-now]').forEach(button=>button.onclick=()=>organizeMusicNow(button))}

  document.addEventListener('click',e=>{
    const button=e.target.closest('button[data-page]');if(!button)return;
    if(button.dataset.page==='metadata')setTimeout(decorateMusicMetadata,350);
    if(button.dataset.page==='subtitles')setTimeout(decorateMusicSubtitleRows,350);
  });
})();
