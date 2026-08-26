/* StormFlix Admin v4: dashboard + multi-source library cards. */
(function(){
  const pollers=new Map();
  const kindLabel=value=>({movies:'Filmes',series:'Séries',anime:'Animes',mixed:'Filmes + Anime',shows:'Shows',other:'Outros'})[value]||value||'Conteúdo';
  const statusClass=lib=>!lib.enabled?'offline':lib.online_sources<=0?'offline':lib.offline_sources>0?'partial':'';
  const statusText=lib=>!lib.enabled?'Desativada':lib.online_sources<=0?'Offline':lib.offline_sources>0?'Parcial':'Online';

  loadDashboard=async function(){
    const [d,storage,server,cleanup]=await Promise.all([
      req('/admin/dashboard'),
      req('/admin/storage').catch(()=>[]),
      req('/admin/server').catch(()=>({})),
      me?.role==='admin'?req('/admin/cleanup').catch(()=>null):Promise.resolve(null)
    ]);
    const online=(storage||[]).filter(l=>l.online_sources>0).length;
    const totalSources=(storage||[]).reduce((sum,l)=>sum+(l.source_count||1),0);
    const onlineSources=(storage||[]).reduce((sum,l)=>sum+(l.online_sources||0),0);
    const recent=(storage||[]).slice(0,6);
    $('#dashboard').innerHTML=`<div class="dashboard-shell">
      <div class="dashboard-banner">
        <section class="dashboard-welcome"><p class="kicker">Servidor de mídia</p><h2>StormFlix está pronto</h2><p>Administre conteúdo, remotes e usuários sem misturar a organização lógica das bibliotecas com os caminhos físicos dos seus Drives.</p><div class="status-line"><span class="status-chip"><i class="status-dot"></i> DIRECT PLAY</span><span class="status-chip">${online}/${storage.length} bibliotecas online</span><span class="status-chip">${onlineSources}/${totalSources} origens online</span></div></section>
        <section class="dashboard-system"><div class="system-row"><span>Versão</span><b>${esc(server.version||'—')}</b></div><div class="system-row"><span>CPU</span><b>${server.cpus||'—'} núcleos</b></div><div class="system-row"><span>RAM do processo</span><b>${bytes(server.memory_alloc_bytes||0)}</b></div><div class="system-row"><span>Uptime</span><b>${duration(server.uptime_seconds||0)}</b></div></section>
      </div>
      <div class="metric-grid">
        ${metric('Usuários',d.users,'Contas cadastradas')}${metric('Bibliotecas',d.libraries,'Coleções lógicas')}${metric('Mídias',d.media,'Itens disponíveis')}${metric('Sessões',d.active_sessions,'Logins ativos')}${metric('Reproduzindo',d.active_playbacks,'Agora')}${metric('Origens',totalSources,`${onlineSources} online`)}
      </div>
      <div class="dashboard-columns">
        <section class="dashboard-card"><div class="dashboard-card-head"><h3>Bibliotecas</h3><button onclick="document.querySelector('[data-page=libraries]').click()">Gerenciar</button></div><div class="mini-library-list">${recent.map(l=>`<div class="mini-library-row"><div><b>${esc(l.name)}</b><small>${esc(kindLabel(l.kind))} · ${l.media_count} mídias</small></div><span>${l.source_count||1} origem(ns)</span><span class="source-health ${statusClass(l)}">${statusText(l)}</span></div>`).join('')||'<div class="v3-empty">Nenhuma biblioteca criada.</div>'}</div></section>
        <section class="dashboard-card"><div class="dashboard-card-head"><h3>Acesso rápido</h3></div><div class="quick-actions"><button class="quick-action" onclick="document.querySelector('[data-page=libraries]').click()"><span>Bibliotecas e Drives</span><b>→</b></button><button class="quick-action" onclick="document.querySelector('[data-page=metadata]').click()"><span>Metadados & Capas</span><b>→</b></button>${me?.role==='admin'?`<button class="quick-action" onclick="document.querySelector('[data-page=cleanup]').click()"><span>Limpeza ${cleanup?`· ${cleanup.orphan_asset_files} órfãos`:''}</span><b>→</b></button>`:''}<button class="quick-action" onclick="document.querySelector('[data-page=playbacks]').click()"><span>Reproduzindo agora</span><b>→</b></button></div></section>
      </div>
    </div>`;
  };

  function metric(name,value,note){return `<article class="metric-card"><div><strong>${value??0}</strong><span>${esc(name)}</span></div><small>${esc(note||'')}</small></article>`}

  loadLibraries=async function(){
    libs=await req('/admin/storage');
    const totalSources=libs.reduce((sum,l)=>sum+(l.source_count||1),0);
    const onlineSources=libs.reduce((sum,l)=>sum+(l.online_sources||0),0);
    $('#libraries').innerHTML=`<div class="section-intro"><div><h2>Bibliotecas</h2><p>Cada biblioteca é uma coleção única. Dentro dela você pode adicionar vários Drives, remotes ou pastas como origens.</p></div><div class="v3-toolbar"><span class="status-chip">${libs.length} bibliotecas</span><span class="status-chip">${onlineSources}/${totalSources} origens online</span><button class="primary" onclick="editLibrary()">+ Nova biblioteca</button></div></div><div id="lib-form"></div><div class="library-grid">${libs.map(libraryCard).join('')||'<div class="v3-empty">Nenhuma biblioteca criada.</div>'}</div>`;
    libs.filter(l=>l.last_scan_status==='running'||l.last_scan_status==='cancelling').forEach(l=>pollScan(Number(l.id)));
  };

  function libraryCard(l){
    const sources=(Array.isArray(l.sources)&&l.sources.length?l.sources:(l.paths||[l.path]).filter(Boolean).map((path,index)=>({path,label:`Origem ${index+1}`,online:l.online})));
    const active=l.last_scan_status==='running'||l.last_scan_status==='cancelling';
    const health=statusClass(l);
    return `<article class="library-card" data-library-card="${l.id}">
      <div class="library-card-top"><div><h3>${esc(l.name)}</h3><p>${l.enabled?'Biblioteca ativa':'Biblioteca desativada'} · ${esc(l.last_scan_at||'Nunca escaneada')}</p></div><span class="library-kind-badge">${esc(kindLabel(l.kind))}</span></div>
      <div class="library-stats"><div class="library-stat"><strong>${l.media_count||0}</strong><span>Mídias</span></div><div class="library-stat"><strong>${l.source_count||sources.length||1}</strong><span>Origens</span></div><div class="library-stat"><strong>${l.online_sources||0}</strong><span>Online</span></div></div>
      <div class="library-source-preview">${sources.map((s,index)=>`<div class="library-source-item ${s.online?'online':''}" title="${esc(s.path)}"><i class="source-led"></i><code>${esc(s.path)}</code><small>${s.online?'ONLINE':'OFFLINE'}</small></div>`).join('')}</div>
      <div class="library-scan-note"><span class="source-health ${health}">${statusText(l)}</span> · Scan: ${esc(l.last_scan_status||'never')}${l.last_error?` · ${esc(l.last_error)}`:''}</div>
      <div class="library-actions"><button onclick="scanLib(${l.id})" ${active?'disabled':''}>${active?(l.last_scan_status==='cancelling'?'Cancelando…':'Escaneando…'):'Escanear agora'}</button>${active?`<button class="danger" onclick="cancelScan(${l.id})">Cancelar scan</button>`:''}<button onclick="editLibrary(${l.id})">Editar biblioteca</button><button class="danger" onclick="delLib(${l.id})">Excluir catálogo</button></div>
    </article>`;
  }

  window.scanLib=async function(id){
    id=Number(id);
    try{
      const r=await req(`/libraries/${id}/scan`,{method:'POST'});
      notice(`Scan iniciado · ${r.online_sources||0}/${r.sources||1} origens online.`,true);
      await loadLibraries();pollScan(id,true);
    }catch(err){notice(err.message);await loadLibraries().catch(()=>{})}
  };

  window.cancelScan=async function(id){
    id=Number(id);
    try{await req(`/libraries/${id}/scan/cancel`,{method:'POST'});notice('Cancelamento solicitado. O catálogo atual será preservado.',true);await loadLibraries()}catch(err){notice(err.message)}
  };

  const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
  async function pollScan(id,replace=false){
    id=Number(id);if(pollers.has(id)&&!replace)return;
    const token=Symbol('scan');pollers.set(id,token);const started=Date.now();
    try{
      while(pollers.get(id)===token&&Date.now()-started<21*60*1000){
        await wait(1900);if(pollers.get(id)!==token)return;
        try{libs=await req('/admin/storage')}catch{continue}
        const lib=libs.find(x=>Number(x.id)===id);if(!lib)return;
        if(lib.last_scan_status==='running'||lib.last_scan_status==='cancelling'){
          const card=document.querySelector(`[data-library-card="${id}"] .library-scan-note`);if(card)card.textContent=lib.last_error||'Escaneando…';continue
        }
        if(lib.last_scan_status==='ok'||lib.last_scan_status==='partial')notice(`${lib.name}: ${lib.media_count} mídias catalogadas.`,true);else notice(lib.last_error||`Scan finalizado: ${lib.last_scan_status}`);
        await loadLibraries();return;
      }
    }finally{if(pollers.get(id)===token)pollers.delete(id)}
  }

  // If the admin was already opened while deferred scripts were loading, repaint it with v4.
  setTimeout(()=>{if(!$('#app')?.classList.contains('hidden')){loadDashboard().catch(()=>{});loadLibraries().catch(()=>{})}},0);
})();
