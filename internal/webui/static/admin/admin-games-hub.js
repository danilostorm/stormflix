/* StormFlix Games Admin G2.5: one place for libraries, ROMs, scans, metadata, saves and runtime. */
(function(){
  const nav=document.querySelector('nav [data-page="gamesadmin"]');
  const root=document.querySelector('#gamesadmin');
  if(!nav||!root)return;

  let tab='overview',overview=null,catalog=[],providers=[],storage=[],jobs=[];
  let q='',platform='';
  const tabs=[
    ['overview','Visão geral'],['library','Bibliotecas & ROMs'],['queue','Fila & scans'],
    ['metadata','Metadados'],['saves','Saves'],['emulators','Emuladores'],['settings','Configurações']
  ];
  const labels={nes:'Nintendo Entertainment System',snes:'Super Nintendo',genesis:'Mega Drive / Genesis',gb:'Game Boy',gbc:'Game Boy Color',gba:'Game Boy Advance'};
  const fmtSeconds=s=>{s=Math.max(0,Number(s)||0);const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h?`${h}h ${String(m).padStart(2,'0')}m`:`${m} min`};
  const fmtBytes=n=>{n=Number(n)||0;if(n<1024)return`${n} B`;if(n<1048576)return`${(n/1024).toFixed(1)} KB`;if(n<1073741824)return`${(n/1048576).toFixed(1)} MB`;return`${(n/1073741824).toFixed(1)} GB`};
  const safe=v=>typeof esc==='function'?esc(v):String(v??'');

  nav.onclick=()=>open();

  async function open(){
    if(window.phase2Timer){clearTimeout(window.phase2Timer);window.phase2Timer=null}
    document.querySelectorAll('nav [data-page]').forEach(x=>x.classList.toggle('active',x===nav));
    document.querySelectorAll('.page').forEach(x=>x.classList.add('hidden'));
    root.classList.remove('hidden');
    const title=document.querySelector('#page-title');if(title)title.textContent='Games';
    await load(true);
  }

  async function load(full=false){
    root.innerHTML='<div class="games-admin-loading"><span></span><b>Carregando central de Games…</b></div>';
    try{
      const requests=[req('/admin/games/overview'),req('/admin/games/catalog?limit=800'),req('/admin/storage'),req('/admin/jobs')];
      if(me?.role==='admin'||full)requests.push(req('/admin/games/providers'));
      const data=await Promise.all(requests);
      overview=data[0]||{};catalog=data[1]||[];storage=(data[2]||[]).filter(x=>String(x.kind||'').toLowerCase()==='games');jobs=(data[3]||[]).filter(isGameJob);
      providers=(data[4]?.providers||providers||[]);
      render();
    }catch(err){root.innerHTML=`<div class="games-admin-error"><b>Não foi possível carregar Games</b><p>${safe(err.message)}</p></div>`}
  }

  function isGameJob(j){return String(j.kind||'').includes('game')||String(j.label||'').toLowerCase().includes('jogo')||storage.some(l=>Number(l.id)===Number(j.library_id))}

  function render(){
    root.innerHTML=`<div class="games-admin-shell">
      <section class="games-admin-hero">
        <div><p class="games-admin-kicker">STORMFLIX GAMES · G2.5</p><h2>Central de Games</h2><p>ROMs, bibliotecas, scans, saves, runtime e metadados em um painel separado do catálogo de vídeo.</p></div>
        <div class="games-admin-live"><span class="${Number(overview?.active_players)>0?'on':''}"></span><b>${Number(overview?.active_players||0)}</b><small>jogando agora</small></div>
      </section>
      <nav class="games-admin-tabs">${tabs.map(([id,label])=>`<button class="${tab===id?'active':''}" data-games-admin-tab="${id}">${safe(label)}</button>`).join('')}</nav>
      <section class="games-admin-content">${tabHTML()}</section>
    </div>`;
    root.querySelectorAll('[data-games-admin-tab]').forEach(b=>b.onclick=()=>{tab=b.dataset.gamesAdminTab;render()});
    bind();
  }

  function tabHTML(){
    if(tab==='library')return libraryHTML();
    if(tab==='queue')return queueHTML();
    if(tab==='metadata')return metadataHTML();
    if(tab==='saves')return savesHTML();
    if(tab==='emulators')return emulatorsHTML();
    if(tab==='settings')return settingsHTML();
    return overviewHTML();
  }

  function metric(value,label,sub=''){return`<article class="games-admin-metric"><strong>${safe(value)}</strong><span>${safe(label)}</span>${sub?`<small>${safe(sub)}</small>`:''}</article>`}

  function overviewHTML(){
    const platforms=overview?.platforms||[];
    return `<div class="games-admin-metrics">
      ${metric(overview?.games||0,'Jogos')}${metric(overview?.files||0,'ROMs disponíveis')}${metric(overview?.saves||0,'Saves','state + SRAM')}${metric(fmtSeconds(overview?.play_seconds||0),'Tempo jogado')}${metric(overview?.metadata_rows||0,'Com metadata')}${metric(overview?.locked_metadata||0,'Metadata travada')}
    </div>
    <div class="games-admin-columns">
      <article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Biblioteca</p><h3>Plataformas</h3></div><button data-jump="library">Gerenciar ROMs</button></div>
        <div class="games-admin-platform-list">${platforms.map(p=>`<button data-platform-jump="${safe(p.platform)}"><span>${safe(shortPlatform(p.platform))}</span><b>${safe(p.label||labels[p.platform]||p.platform)}</b><em>${Number(p.count||0)}</em></button>`).join('')||'<p class="games-admin-muted">Nenhum jogo catalogado ainda.</p>'}</div>
      </article>
      <article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Atividade</p><h3>Últimos scans</h3></div><button data-jump="queue">Abrir fila</button></div>${scanRows(overview?.recent_scans||[])}</article>
    </div>
    <article class="games-admin-panel games-admin-roadmap"><div><p class="games-admin-kicker">Próximas capacidades</p><h3>Base pronta para crescer sem virar outro produto dentro do StormFlix</h3></div><div class="games-admin-roadmap-grid"><span>✓ Browser Player + gamepad</span><span>✓ Saves por perfil</span><span>✓ Hash SHA-256</span><span>✓ Cofre de provedores</span><span>→ Enriquecimento automático</span><span>→ BIOS / ROMset diagnostics</span><span>→ Multi-disc / DLC</span><span>→ RetroAchievements</span></div></article>`;
  }

  function libraryHTML(){
    const platforms=[...new Set(catalog.map(g=>g.platform))].sort();
    const list=catalog.filter(g=>(!platform||g.platform===platform)&&(!q||String(g.title).toLowerCase().includes(q.toLowerCase())));
    return `<article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Origens</p><h3>Bibliotecas de Games</h3></div><button class="primary" data-new-game-lib>+ Biblioteca de Games</button></div>
      <div class="games-admin-library-cards">${storage.map(l=>`<div><b>${safe(l.name)}</b><small>${safe(l.path||'')}</small><span class="${l.online?'ok':'bad'}">${l.online?'ONLINE':'OFFLINE'}</span><em>${Number(l.media_count||0)} item(ns)</em><div><button data-game-scan="${l.id}">Escanear</button><button data-game-edit-lib="${l.id}">Editar</button></div></div>`).join('')||'<p class="games-admin-muted">Crie uma biblioteca do tipo Jogos para começar.</p>'}</div>
    </article>
    <article class="games-admin-panel"><div class="games-admin-catalog-tools"><label><span>⌕</span><input data-games-catalog-search value="${safe(q)}" placeholder="Buscar ROM…"></label><select data-games-platform-filter><option value="">Todas as plataformas</option>${platforms.map(p=>`<option value="${safe(p)}" ${platform===p?'selected':''}>${safe(labels[p]||p)}</option>`).join('')}</select><b>${list.length} jogo(s)</b></div>
      <div class="games-admin-rom-table"><table><thead><tr><th>Jogo</th><th>Plataforma</th><th>Biblioteca</th><th>Arquivos</th><th>Saves</th><th>Tempo</th><th>Metadata</th><th>SHA-256</th></tr></thead><tbody>${list.map(g=>`<tr><td><b>${safe(g.title)}</b>${g.release_year?`<small>${g.release_year}</small>`:''}</td><td><span class="games-admin-system">${safe(shortPlatform(g.platform))}</span></td><td>${safe(g.library)}</td><td>${Number(g.available_files||0)}/${Number(g.file_count||0)}</td><td>${Number(g.save_count||0)}</td><td>${safe(fmtSeconds(g.play_seconds||0))}</td><td>${g.provider?`<span class="games-admin-provider-ok">${safe(g.provider)}${g.metadata_locked?' · 🔒':''}</span>`:'<span class="games-admin-muted">local</span>'}</td><td><code title="${safe(g.content_hash)}">${safe(String(g.content_hash||'').slice(0,12))}…</code></td></tr>`).join('')||'<tr><td colspan="8">Nenhum jogo neste filtro.</td></tr>'}</tbody></table></div>
    </article>`;
  }

  function scanRows(items){return`<div class="games-admin-scan-list">${(items||[]).map(j=>`<div><span class="games-admin-job-dot ${safe(j.status)}"></span><b>${safe(j.library||j.label||'Games')}</b><small>${safe(j.message||j.status||'')}</small><em>${Number(j.progress||0)}%</em></div>`).join('')||'<p class="games-admin-muted">Nenhum scan executado ainda.</p>'}</div>`}

  function queueHTML(){
    return `<article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Operações</p><h3>Fila de Games</h3><p>O hash pesado cede prioridade quando há vídeo ou jogo ativo.</p></div><button data-refresh-games>Atualizar</button></div>${scanRows(jobs)}</article>
      <article class="games-admin-panel"><div class="games-admin-panel-head"><div><h3>Iniciar scan</h3><p>Um job por biblioteca; o catálogo anterior é preservado se uma origem ficar offline.</p></div></div><div class="games-admin-library-cards">${storage.map(l=>`<div><b>${safe(l.name)}</b><small>${safe(l.last_scan_at||'Nunca escaneada')}</small><em>${safe(l.last_scan_status||'')}</em><div><button data-game-scan="${l.id}">Adicionar à fila</button><button data-game-cancel="${l.id}">Cancelar</button></div></div>`).join('')}</div></article>`;
  }

  function metadataHTML(){
    return `<article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Metadata stack</p><h3>Provedores</h3><p>O catálogo mantém SHA-256 como identidade. Provedores enriquecem título, capas, hero, gêneros, ratings e IDs sem substituir essa identidade.</p></div></div><div class="games-admin-provider-grid">${providers.map(providerCard).join('')}</div></article>
    <article class="games-admin-panel games-admin-info"><b>Prioridade planejada</b><p>Hash/IDs confiáveis primeiro; depois correspondência por plataforma + nome normalizado. Artwork pode vir de uma fonte diferente do metadata principal. Metadata travada pelo administrador não será sobrescrita por rescan.</p></article>`;
  }

  function providerCard(p){
    const state=p.stage==='planejado'?'planejado':(p.configured?'pronto':'configurar');
    return `<article class="games-admin-provider ${state}"><div class="games-admin-provider-title"><div><span>${safe(p.kind||'metadata')}</span><h4>${safe(p.name)}</h4></div><b>${state==='pronto'?'● PRONTO':state==='planejado'?'ROADMAP':'● CONFIGURAR'}</b></div><p>${safe(p.description||'')}</p><div class="games-admin-provider-foot"><small>${p.enabled?'Ativo':'Inativo'}</small>${p.stage!=='planejado'&&me?.role==='admin'?`<button data-edit-provider="${safe(p.key)}">Configurar</button>`:''}</div></article>`;
  }

  function savesHTML(){
    return `<article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Persistência</p><h3>Saves por perfil</h3><p>${Number(overview?.saves||0)} arquivo(s) registrados em ${Number(overview?.profiles_with_saves||0)} perfil(is). Save-state e SRAM ficam fora do SQLite, com versionamento e recovery.</p></div></div><div class="games-admin-save-architecture"><span><b>State</b><small>snapshot do emulador</small></span><i>+</i><span><b>SRAM</b><small>save nativo do jogo</small></span><i>→</i><span><b>3 backups</b><small>.bak1 · .bak2 · .bak3</small></span></div><div class="games-admin-info"><b>Privacidade por perfil</b><p>O Admin mostra contagens e saúde. O conteúdo de save continua pertencendo ao perfil; não é misturado entre usuários.</p></div></article>`;
  }

  function emulatorsHTML(){
    const matrix=[['NES','fceumm','.nes'],['SNES','snes9x','.sfc · .smc'],['Mega Drive','genesis_plus_gx','.md · .gen · .smd'],['Game Boy / Color','mgba','.gb · .gbc'],['Game Boy Advance','mgba','.gba']];
    return `<article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Browser runtime</p><h3>Nostalgist + RetroArch WASM</h3><p>Assets de runtime são fixados por versão e armazenados localmente no servidor após o primeiro uso.</p></div><span class="games-admin-runtime-badge">Nostalgist 0.21.1 · RetroArch v1.22.2</span></div><div class="games-admin-emulator-grid">${matrix.map(x=>`<div><b>${x[0]}</b><code>${x[1]}</code><small>${x[2]}</small><span>✓ Browser</span></div>`).join('')}</div></article>
      <article class="games-admin-panel games-admin-info"><b>BIOS / ROMsets</b><p>Neo Geo, arcade e consoles de disco entram na próxima matriz com diagnóstico explícito de BIOS/ROMset/core. Isso evita o erro genérico “não abre” quando a causa real é incompatibilidade do conjunto de ROMs com o core.</p></article>`;
  }

  function settingsHTML(){
    return `<article class="games-admin-panel"><div class="games-admin-panel-head"><div><p class="games-admin-kicker">Administração</p><h3>Políticas de Games</h3></div></div><div class="games-admin-settings-list"><div><b>Identidade</b><p>SHA-256 permanece a chave local, mesmo quando título/capa mudam.</p><span>Obrigatório</span></div><div><b>Metadata lock</b><p>Estrutura pronta para bloquear ajustes manuais contra futuros rescans.</p><span>Phase 22</span></div><div><b>Scan inteligente</b><p>Novos arquivos devem ser detectados sem rehash de ROM conhecida quando tamanho/mtime não mudaram.</p><span>Próxima otimização</span></div><div><b>Segredos</b><p>Credenciais de provedores usam AES-GCM e nunca são devolvidas pelo endpoint público.</p><span>Ativo</span></div></div></article>`;
  }

  function shortPlatform(p){return({nes:'NES',snes:'SNES',genesis:'GEN',gb:'GB',gbc:'GBC',gba:'GBA'}[p]||String(p||'GAME').toUpperCase())}

  function bind(){
    root.querySelectorAll('[data-jump]').forEach(b=>b.onclick=()=>{tab=b.dataset.jump;render()});
    root.querySelectorAll('[data-platform-jump]').forEach(b=>b.onclick=()=>{platform=b.dataset.platformJump;tab='library';render()});
    root.querySelector('[data-games-catalog-search]')?.addEventListener('input',e=>{q=e.target.value.trim();render()});
    root.querySelector('[data-games-platform-filter]')?.addEventListener('change',e=>{platform=e.target.value;render()});
    root.querySelectorAll('[data-game-scan]').forEach(b=>b.onclick=()=>scan(Number(b.dataset.gameScan)));
    root.querySelectorAll('[data-game-cancel]').forEach(b=>b.onclick=()=>cancel(Number(b.dataset.gameCancel)));
    root.querySelectorAll('[data-game-edit-lib]').forEach(b=>b.onclick=()=>{if(typeof editLibrary==='function'){show('libraries');setTimeout(()=>editLibrary(Number(b.dataset.gameEditLib)),50)}});
    root.querySelector('[data-new-game-lib]')?.addEventListener('click',()=>{if(typeof editLibrary==='function'){show('libraries');setTimeout(()=>editLibrary(undefined,'games'),50)}});
    root.querySelectorAll('[data-edit-provider]').forEach(b=>b.onclick=()=>editProvider(b.dataset.editProvider));
    root.querySelector('[data-refresh-games]')?.addEventListener('click',()=>load());
  }

  async function scan(id){try{notice('Scan de Games adicionado à fila…',true);await req(`/libraries/${id}/scan`,{method:'POST',body:'{}'});await load()}catch(err){notice(err.message)}}
  async function cancel(id){try{await req(`/libraries/${id}/scan/cancel`,{method:'POST',body:'{}'});notice('Cancelamento solicitado.',true);await load()}catch(err){notice(err.message)}}

  function editProvider(key){
    const p=providers.find(x=>x.key===key);if(!p)return;
    const dialog=document.createElement('div');dialog.className='games-admin-modal';
    dialog.innerHTML=`<form class="games-admin-provider-editor"><button type="button" class="games-admin-modal-close">✕</button><p class="games-admin-kicker">Metadata provider</p><h3>${safe(p.name)}</h3><p>${safe(p.description||'')}</p><label class="games-admin-toggle"><input name="enabled" type="checkbox" ${p.enabled?'checked':''}><span>Ativar provedor</span></label><div class="games-admin-provider-fields">${(p.fields||[]).map(f=>`<label><span>${safe(f.label)}${f.required?' *':''}</span><input name="${safe(f.key)}" type="${f.secret?'password':'text'}" value="${f.secret?'':safe(p.public?.[f.key]||'')}" placeholder="${f.secret&&p.secrets?.[f.key]?'Configurado · deixe vazio para manter':safe(f.placeholder||'')}">${f.secret&&p.secrets?.[f.key]?'<small>✓ segredo já configurado</small>':''}</label>`).join('')}</div><div class="games-admin-modal-actions"><button type="button" data-cancel-provider>Cancelar</button><button class="primary" type="submit">Salvar com criptografia</button></div></form>`;
    document.body.appendChild(dialog);const form=dialog.querySelector('form');const close=()=>dialog.remove();dialog.querySelector('.games-admin-modal-close').onclick=close;dialog.querySelector('[data-cancel-provider]').onclick=close;dialog.onclick=e=>{if(e.target===dialog)close()};
    form.onsubmit=async e=>{e.preventDefault();const values={};for(const f of p.fields||[])values[f.key]=form.elements[f.key]?.value||'';try{await req(`/admin/games/providers/${encodeURIComponent(p.key)}`,{method:'PUT',body:JSON.stringify({enabled:form.elements.enabled.checked,values})});notice(`${p.name} salvo com segurança.`,true);close();const data=await req('/admin/games/providers');providers=data.providers||[];render()}catch(err){notice(err.message)}};
    form.querySelector('input')?.focus();
  }
})();