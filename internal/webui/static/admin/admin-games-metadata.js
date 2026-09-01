/* Games metadata queue controls layered onto the G2.5 Admin hub. */
(function(){
  const root=document.querySelector('#gamesadmin');if(!root)return;
  let timer=null,enterTimer=null,busy=false;
  const safe=v=>typeof esc==='function'?esc(v):String(v??'');
  const implemented=new Set(['igdb','mobygames','steamgriddb']);

  document.addEventListener('click',e=>{
    const tab=e.target.closest('[data-games-admin-tab]');
    if(tab){
      if(tab.dataset.gamesAdminTab==='metadata')scheduleEnhance(30,6);
      else stopPolling();
    }
    const nav=e.target.closest('nav [data-page="gamesadmin"]');if(nav)scheduleEnhance(120,6);
    const all=e.target.closest('[data-game-meta-all]');if(all)start(0,all.dataset.gameMetaAll==='refresh');
    const lib=e.target.closest('[data-game-meta-lib]');if(lib)start(Number(lib.dataset.gameMetaLib),lib.dataset.refresh==='1');
    const lock=e.target.closest('[data-game-lock]');if(lock)toggleLock(lock.dataset.gameLock==='1');
  });

  function stopPolling(){
    if(timer){clearTimeout(timer);timer=null}
    if(enterTimer){clearTimeout(enterTimer);enterTimer=null}
  }

  function scheduleEnhance(delay=0,retries=0){
    if(enterTimer)clearTimeout(enterTimer);
    enterTimer=setTimeout(()=>{
      enterTimer=null;
      if(metadataVisible()){enhance();return}
      if(retries>0&&!root.classList.contains('hidden'))scheduleEnhance(120,retries-1);
    },delay);
  }

  function metadataVisible(){return !root.classList.contains('hidden')&&!!root.querySelector('[data-games-admin-tab="metadata"].active')}

  function decorateProviderStages(){
    root.querySelectorAll('[data-edit-provider]').forEach(button=>{
      const key=button.dataset.editProvider;if(implemented.has(key))return;
      const card=button.closest('.games-admin-provider');const badge=card?.querySelector('.games-admin-provider-title>b');
      if(badge&&badge.textContent!=='PRÓXIMA INTEGRAÇÃO')badge.textContent='PRÓXIMA INTEGRAÇÃO';
      if(button.textContent!=='Em breve')button.textContent='Em breve';
      if(!button.disabled)button.disabled=true;
      const title='O cofre/schema já conhece este provedor, mas o scraper específico ainda não está habilitado.';
      if(button.title!==title)button.title=title;
    });
  }

  async function enhance(){
    if(!metadataVisible()||busy)return;busy=true;
    try{
      const [jobs,libs,catalog]=await Promise.all([req('/admin/games/metadata/jobs?limit=30'),req('/admin/storage'),req('/admin/games/catalog?limit=500')]);
      if(!metadataVisible())return;
      const gameLibs=(libs||[]).filter(l=>String(l.kind||'').toLowerCase()==='games');
      let panel=root.querySelector('.games-admin-metadata-jobs');
      if(!panel){panel=document.createElement('article');panel.className='games-admin-panel games-admin-metadata-jobs';root.querySelector('.games-admin-content')?.appendChild(panel)}
      panel.innerHTML=`<div class="games-admin-panel-head"><div><p class="games-admin-kicker">Enriquecimento automático</p><h3>Fila de metadados</h3><p>IGDB → MobyGames como fallback; SteamGridDB pode substituir a capa. O job pausa durante playback/jogo ativo.</p></div><div class="games-admin-meta-actions"><button data-game-meta-all="pending">Buscar pendentes</button><button data-game-meta-all="refresh">Atualizar tudo</button></div></div>
        <div class="games-admin-library-cards">${gameLibs.map(l=>`<div><b>${safe(l.name)}</b><small>${safe(l.path||'')}</small><span class="${l.online?'ok':'bad'}">${l.online?'ONLINE':'OFFLINE'}</span><em>${Number(l.media_count||0)} jogo(s)</em><div><button data-game-meta-lib="${l.id}">Pendentes</button><button data-game-meta-lib="${l.id}" data-refresh="1">Atualizar tudo</button></div></div>`).join('')||'<p class="games-admin-muted">Nenhuma biblioteca de Games.</p>'}</div>
        <div class="games-admin-scan-list games-admin-meta-job-list">${(jobs||[]).map(j=>`<div><span class="games-admin-job-dot ${safe(j.status)}"></span><b>${safe(j.library||'Games')}</b><small>${safe(j.message||j.provider||j.status)}</small><em>${Number(j.progress||0)}% · ${Number(j.matched||0)} ok · ${Number(j.failed||0)} erro(s)</em></div>`).join('')||'<p class="games-admin-muted">Nenhum job de metadata executado ainda.</p>'}</div>
        <div class="games-admin-meta-lock"><div><b>Metadata lock</b><small>Proteja ajustes manuais para que um refresh automático não os substitua.</small></div><select data-game-lock-select>${(catalog||[]).map(g=>`<option value="${g.id}">${g.metadata_locked?'🔒 ':''}${safe(g.title)} · ${safe(String(g.platform||'').toUpperCase())}</option>`).join('')}</select><button data-game-lock="1">Travar</button><button data-game-lock="0">Destravar</button></div>`;
      decorateProviderStages();
      if(timer){clearTimeout(timer);timer=null}
      if((jobs||[]).some(j=>j.status==='queued'||j.status==='running'))timer=setTimeout(enhance,2200);
    }catch(err){if(typeof notice==='function')notice(err.message)}finally{busy=false}
  }

  async function start(libraryID,refresh){
    try{
      const path=libraryID?`/admin/games/libraries/${libraryID}/metadata${refresh?'?refresh=1':''}`:`/admin/games/metadata${refresh?'?refresh=1':''}`;
      await req(path,{method:'POST',body:'{}'});if(typeof notice==='function')notice(refresh?'Atualização completa adicionada à fila.':'Metadata pendente adicionada à fila.',true);await enhance()
    }catch(err){if(typeof notice==='function')notice(err.message)}
  }

  async function toggleLock(locked){
    const select=root.querySelector('[data-game-lock-select]');const id=Number(select?.value||0);if(!id)return;
    try{await req(`/admin/games/catalog/${id}/metadata-lock`,{method:'PUT',body:JSON.stringify({locked})});if(typeof notice==='function')notice(locked?'Metadata travada.':'Metadata destravada.',true);await enhance()}catch(err){if(typeof notice==='function')notice(err.message)}
  }
})();