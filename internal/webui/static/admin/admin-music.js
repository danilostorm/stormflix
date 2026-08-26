/* StormFlix Admin: music library support + live organizer progress. */
(function(){
  let progressTimer=null;
  let lastStatus=null;
  let organizeChain=false;
  let chaining=false;

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

  const baseLoadLibraries=window.loadLibraries;
  if(typeof baseLoadLibraries==='function')window.loadLibraries=async function(){
    await baseLoadLibraries();
    await decorateMusicLibraryProgress();
  };

  const baseMetadata=window.loadMetadataPhase2;
  if(typeof baseMetadata==='function')window.loadMetadataPhase2=async function(){await baseMetadata();await decorateMusicMetadata()};

  const baseSubtitles=window.loadSubtitlesPhase2;
  if(typeof baseSubtitles==='function')window.loadSubtitlesPhase2=async function(){await baseSubtitles();decorateMusicSubtitleRows()};

  function musicLibraries(){return (libs||[]).filter(l=>l.kind==='music')}

  window.organizeMusicNow=async function(button){
    if(button)button.disabled=true;
    organizeChain=true;
    try{
      const r=await req('/admin/music/index',{method:'POST',body:'{}'});
      notice(r.started?'Organização da biblioteca de música iniciada. O StormFlix continuará os lotes automaticamente até terminar.':'A biblioteca de música já está sendo organizada; o acompanhamento continuará até terminar.',true);
      await refreshMusicStatus(true);
    }catch(err){organizeChain=false;notice(err.message)}finally{if(button)button.disabled=false}
  };

  async function decorateMusicLibraryProgress(){
    if(!musicLibraries().length){stopProgressPoll();return}
    const group=document.querySelector('#libraries .library-group.music');
    if(!group)return;
    let panel=group.querySelector('[data-music-progress-panel]');
    if(!panel){
      panel=document.createElement('div');
      panel.className='music-index-panel';
      panel.dataset.musicProgressPanel='1';
      panel.innerHTML='<div data-music-status>'+statusLoadingHTML()+'</div>';
      const head=group.querySelector('.library-group-head');
      if(head)head.after(panel);else group.prepend(panel);
    }
    await refreshMusicStatus(false);
  }

  async function decorateMusicMetadata(){
    const page=document.querySelector('#metadata');if(!page)return;
    const music=musicLibraries();if(!music.length)return;
    page.querySelectorAll('[data-music-agents]').forEach(x=>x.remove());
    try{
      const agents=await req('/admin/agents');
      lastStatus=agents.music_status||lastStatus;
      const panel=document.createElement('div');panel.className='panel';panel.dataset.musicAgents='1';
      panel.innerHTML=`<div class="panel-head"><div><h2>Agentes de Música</h2><small>Separados dos agentes de filmes e séries para não misturar TMDB com sua discoteca.</small></div><button class="primary" data-music-index-now>Organizar música</button></div><div class="agent-grid">${(agents.music||[]).map(renderAgent).join('')}</div><div class="music-index-panel metadata" data-music-status>${musicStatusHTML(lastStatus)}</div>`;
      const first=page.querySelector('.panel');if(first)first.after(panel);else page.prepend(panel);bindIndexButtons(panel);
      scheduleProgressPoll(lastStatus);
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

  async function refreshMusicStatus(forcePoll){
    try{
      const agents=await req('/admin/agents');
      lastStatus=agents.music_status||{};
      document.querySelectorAll('[data-music-status]').forEach(el=>el.innerHTML=musicStatusHTML(lastStatus));
      await continueOrganizerIfNeeded(lastStatus);
      scheduleProgressPoll(lastStatus,forcePoll);
      return lastStatus;
    }catch(err){
      document.querySelectorAll('[data-music-status]').forEach(el=>el.innerHTML=`<div class="music-index-error">Não foi possível consultar o progresso: ${esc(err.message)}</div>`);
      return null;
    }
  }

  async function continueOrganizerIfNeeded(status){
    if(!organizeChain||chaining||status?.indexing)return;
    const pending=Number(status?.pending_tracks||0)+Number(status?.pending_albums||0);
    if(pending<=0){organizeChain=false;notice('Organização de música concluída.',true);return}
    chaining=true;
    try{
      const r=await req('/admin/music/index',{method:'POST',body:'{}'});
      if(r.started)notice(`Continuando organização · ${Number(status.pending_tracks||0).toLocaleString('pt-BR')} faixa(s) e ${Number(status.pending_albums||0).toLocaleString('pt-BR')} álbum(ns) pendentes.`,true);
    }catch(err){organizeChain=false;notice(`A continuação automática parou: ${err.message}`)}finally{chaining=false}
  }

  function scheduleProgressPoll(status,force){
    if(progressTimer){clearTimeout(progressTimer);progressTimer=null}
    if(status?.indexing||force||organizeChain){
      progressTimer=setTimeout(async()=>{
        const next=await refreshMusicStatus(false);
        if(next?.indexing||organizeChain)scheduleProgressPoll(next,false);
      },1600);
    }
  }

  function stopProgressPoll(){if(progressTimer){clearTimeout(progressTimer);progressTimer=null}}

  function musicStatusHTML(s){
    if(!s)return statusLoadingHTML();
    const pct=Math.max(0,Math.min(100,Number(s.progress||0)));
    const tags=`${Number(s.indexed_tracks||0).toLocaleString('pt-BR')} / ${Number(s.total_tracks||0).toLocaleString('pt-BR')}`;
    const albums=`${Number(s.enriched_albums||0).toLocaleString('pt-BR')} / ${Number(s.total_albums||0).toLocaleString('pt-BR')}`;
    const state=s.indexing?'running':s.phase==='completed'?'done':s.phase==='waiting'||s.phase==='waiting_albums'?'waiting':'idle';
    const stateLabel=s.indexing?'EM ANDAMENTO':s.phase==='completed'?'CONCLUÍDO':s.phase==='waiting'||s.phase==='waiting_albums'?'PENDENTE':'PARADO';
    const activity=s.phase==='tags'?`Faixas processadas: <b>${tags}</b>`:s.phase==='albums'||s.phase==='waiting_albums'?`Álbuns enriquecidos: <b>${albums}</b>`:`Faixas organizadas: <b>${tags}</b>`;
    const secondary=s.phase==='tags'?`Depois: MusicBrainz + capas · ${Number(s.pending_albums||0).toLocaleString('pt-BR')} álbum(ns) pendentes`:`Capas encontradas: <b>${Number(s.albums_with_cover||0).toLocaleString('pt-BR')}</b> · faixas com fallback: <b>${Number(s.fallback_tracks||0).toLocaleString('pt-BR')}</b>`;
    const updated=s.phase==='tags'?s.last_track_update_at:s.last_album_update_at;
    return `<div class="music-index-status ${state}">
      <div class="music-index-status-head"><div><span class="music-index-live"><i></i>${stateLabel}</span><h3>${esc(s.phase_label||'Organização de música')}</h3><p>${esc(s.message||'')}</p></div><strong>${Math.round(pct)}%</strong></div>
      <div class="music-index-progress"><span style="width:${pct}%"></span></div>
      <div class="music-index-stats"><span>${activity}</span><span>${secondary}</span>${updated?`<span>Última atualização: <b>${esc(updated)}</b></span>`:''}</div>
    </div>`;
  }

  function statusLoadingHTML(){return '<div class="music-index-status idle"><div class="music-index-status-head"><div><span class="music-index-live"><i></i>CONSULTANDO</span><h3>Progresso da organização</h3><p>Carregando estado do FFprobe, MusicBrainz e capas…</p></div></div></div>'}

  document.addEventListener('click',e=>{
    const button=e.target.closest('button[data-page]');if(!button)return;
    if(button.dataset.page==='libraries')setTimeout(()=>decorateMusicLibraryProgress(),250);
    if(button.dataset.page==='metadata')setTimeout(decorateMusicMetadata,350);
    if(button.dataset.page==='subtitles')setTimeout(decorateMusicSubtitleRows,350);
  });
})();