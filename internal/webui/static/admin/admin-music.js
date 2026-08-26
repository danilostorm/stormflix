/* StormFlix Admin: music library support without mixing it with the video catalog. */
(function(){
  const baseEdit=window.editLibrary;
  if(typeof baseEdit==='function'){
    window.editLibrary=function(id){
      const kind=id?(libs.find(x=>Number(x.id)===Number(id))?.kind||'movies'):'movies';
      baseEdit(id);
      const form=document.querySelector('#library-editor');
      const select=form?.querySelector('select');
      if(!select)return;
      if(![...select.options].some(o=>o.value==='music')){
        const option=document.createElement('option');option.value='music';option.textContent='Música';select.appendChild(option);
      }
      select.value=kind;
      const path=form.querySelector('input.wide');
      if(path&&kind==='music')path.placeholder='/media/Musicas';
      select.addEventListener('change',()=>{if(path)path.placeholder=select.value==='music'?'/media/Musicas':'/media/Filmes'});
    };
  }

  const baseLoadLibraries=window.loadLibraries;
  if(typeof baseLoadLibraries==='function')window.loadLibraries=async function(){await baseLoadLibraries();decorateMusicLibraries()};

  const baseMetadata=window.loadMetadataPhase2;
  if(typeof baseMetadata==='function')window.loadMetadataPhase2=async function(){await baseMetadata();await decorateMusicMetadata()};

  const baseSubtitles=window.loadSubtitlesPhase2;
  if(typeof baseSubtitles==='function')window.loadSubtitlesPhase2=async function(){await baseSubtitles();decorateMusicSubtitleRows()};

  function musicLibraries(){return (libs||[]).filter(l=>l.kind==='music')}

  function decorateMusicLibraries(){
    const page=document.querySelector('#libraries');
    if(!page)return;
    page.querySelectorAll('[data-music-admin]').forEach(x=>x.remove());
    const music=musicLibraries();
    if(!music.length)return;
    const panel=document.createElement('div');panel.className='panel';panel.dataset.musicAdmin='1';
    panel.innerHTML=`<div class="panel-head"><div><h2>StormFlix Música</h2><p style="color:#8791a1;margin:5px 0 0">${music.length} biblioteca(s) de música · tags locais + MusicBrainz + Cover Art Archive + LRCLIB</p></div><button class="primary" data-music-index-now>Organizar metadados</button></div><p style="color:#8d97a6;line-height:1.6">O scan encontra os arquivos no Drive. Depois, a organização lê artista, álbum, faixa, gênero, ano, duração, codec e qualidade sem alterar os arquivos originais. Capas externas são associadas ao catálogo; letras são buscadas somente quando o usuário abre a letra.</p>`;
    page.appendChild(panel);bindIndexButtons(panel);
  }

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

  function bindIndexButtons(scope){scope.querySelectorAll('[data-music-index-now]').forEach(button=>button.onclick=async()=>{button.disabled=true;try{const r=await req('/admin/music/index',{method:'POST',body:'{}'});notice(r.started?'Organização da biblioteca de música iniciada.':'A biblioteca de música já está sendo organizada.',true)}catch(err){notice(err.message)}finally{button.disabled=false}})}

  document.addEventListener('click',e=>{
    const button=e.target.closest('button[data-page]');if(!button)return;
    if(button.dataset.page==='libraries')setTimeout(decorateMusicLibraries,180);
    if(button.dataset.page==='metadata')setTimeout(decorateMusicMetadata,350);
    if(button.dataset.page==='subtitles')setTimeout(decorateMusicSubtitleRows,350);
  });
  setTimeout(decorateMusicLibraries,900);
})();
