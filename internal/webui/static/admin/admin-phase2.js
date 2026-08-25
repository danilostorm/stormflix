let phase2Timer=null;

for(const name of ['metadata','subtitles','cdn']){
  const button=document.querySelector(`nav [data-page="${name}"]`);
  if(button)button.onclick=()=>showPhase2(name);
}

async function showPhase2(name){
  if(phase2Timer){clearTimeout(phase2Timer);phase2Timer=null}
  $$('nav [data-page]').forEach(x=>x.classList.toggle('active',x.dataset.page===name));
  $$('.page').forEach(x=>x.classList.add('hidden'));
  $('#'+name).classList.remove('hidden');
  $('#page-title').textContent={metadata:'Metadados & Capas',subtitles:'Legendas',cdn:'CDN / Assets'}[name]||name;
  try{
    if(name==='metadata')await loadMetadataPhase2();
    if(name==='subtitles')await loadSubtitlesPhase2();
    if(name==='cdn')await loadCDNPhase2();
  }catch(err){notice(err.message)}
}

async function ensurePhase2Libraries(){
  if(!Array.isArray(libs)||!libs.length)libs=await req('/admin/storage');
  return libs;
}

function renderAgent(agent){
  return `<div class="agent-card"><div class="agent-status ${agent.ready?'ready':'missing'}">${agent.ready?'● PRONTO':'● CONFIGURAR'}</div><h3>${esc(agent.name)}</h3><p>${esc(agent.description||'')}</p></div>`;
}

async function loadMetadataPhase2(){
  await ensurePhase2Libraries();
  const [agents,status,jobs]=await Promise.all([req('/admin/agents'),req('/admin/metadata/status'),req('/admin/metadata/jobs?limit=20')]);
  const root=$('#metadata');
  root.innerHTML=`
    <div class="cards">
      ${card('Identificadas',status.matched||0)}
      ${card('Pendentes',status.pending||0)}
      ${card('Com erro',status.error||0)}
    </div>
    <div class="panel">
      <div class="panel-head"><div><h2>Agentes de metadados</h2><small>Anime prioriza AniList; filmes/séries priorizam TMDB; Fanart.tv enriquece o artwork.</small></div>${me?.role==='admin'?'<button class="primary" data-open-settings>Configurar agentes</button>':''}</div>
      <div class="agent-grid">${(agents.metadata||[]).map(renderAgent).join('')}</div>
      ${!(agents.metadata||[]).some(a=>a.name==='TMDB'&&a.ready)?'<div class="phase2-hint">TMDB ainda não está configurado. Use <b>Configurações → Metadados & Capas</b>; não é necessário editar Docker Compose.</div>':''}
    </div>
    <div class="panel">
      <div class="panel-head"><div><h2>Escanear capas e informações</h2><small>O trabalho roda em segundo plano e não interrompe streaming.</small></div></div>
      <div class="table-wrap"><table><thead><tr><th>Biblioteca</th><th>Tipo</th><th>Mídias</th><th>Ações</th></tr></thead><tbody>
        ${libs.map(l=>`<tr><td><b>${esc(l.name)}</b></td><td>${esc(l.kind)}</td><td>${l.media_count||0}</td><td class="actions"><button data-meta-scan="${l.id}">Buscar metadados</button><button data-meta-refresh="${l.id}">Atualizar tudo</button></td></tr>`).join('')||'<tr><td colspan="4">Nenhuma biblioteca.</td></tr>'}
      </tbody></table></div>
    </div>
    ${renderMetadataJobs(jobs)}
  `;
  $$('[data-open-settings]').forEach(b=>b.onclick=()=>showSettings());
  $$('[data-meta-scan]').forEach(b=>b.onclick=()=>startMetadataPhase2(+b.dataset.metaScan,false));
  $$('[data-meta-refresh]').forEach(b=>b.onclick=()=>startMetadataPhase2(+b.dataset.metaRefresh,true));
  if((jobs||[]).some(j=>j.status==='queued'||j.status==='running'))phase2Timer=setTimeout(()=>{if(!$('#metadata').classList.contains('hidden'))loadMetadataPhase2()},2000);
}

function renderMetadataJobs(jobs){
  return `<div class="panel"><div class="panel-head"><h2>Jobs de metadados</h2></div><div class="table-wrap"><table><thead><tr><th>Biblioteca</th><th>Status</th><th>Progresso</th><th>OK</th><th>Erros</th><th>Mensagem</th></tr></thead><tbody>${(jobs||[]).map(j=>{const pct=j.total?Math.round(j.processed*100/j.total):0;return `<tr><td>${esc(j.library)}</td><td class="${jobClass(j.status)}">${esc(j.status)}</td><td><div class="progress"><span style="width:${pct}%"></span></div><small>${j.processed}/${j.total} · ${pct}%</small></td><td>${j.matched}</td><td>${j.failed}</td><td><small>${esc(j.message||'')}</small></td></tr>`}).join('')||'<tr><td colspan="6"><small>Nenhum job executado.</small></td></tr>'}</tbody></table></div></div>`;
}

async function startMetadataPhase2(libraryID,refresh){
  notice(refresh?'Atualização completa iniciada...':'Busca de metadados iniciada...',true);
  try{
    await req(`/admin/libraries/${libraryID}/metadata${refresh?'?refresh=1':''}`,{method:'POST'});
    await loadMetadataPhase2();
  }catch(err){notice(err.message)}
}

async function loadSubtitlesPhase2(){
  await ensurePhase2Libraries();
  const [agents,jobs]=await Promise.all([req('/admin/agents'),req('/admin/subtitles/jobs?limit=20')]);
  $('#subtitles').innerHTML=`
    <div class="panel"><div class="panel-head"><div><h2>Agentes de legenda</h2><small>Download automático é opcional e somente acontece quando você inicia um job.</small></div>${me?.role==='admin'?'<button class="primary" data-open-settings>Configurar agentes</button>':''}</div><div class="agent-grid">${(agents.subtitles||[]).map(renderAgent).join('')}</div></div>
    <div class="panel">
      <div class="panel-head"><div><h2>Baixar automaticamente</h2><small>Requer metadados identificados para usar TMDB/IMDb e melhorar a correspondência.</small></div></div>
      <div class="table-wrap"><table><thead><tr><th>Biblioteca</th><th>Mídias</th><th>Idioma</th><th>Ação</th></tr></thead><tbody>
      ${libs.map(l=>`<tr><td><b>${esc(l.name)}</b></td><td>${l.media_count||0}</td><td><select id="sub-lang-${l.id}"><option value="pt-BR">Português (Brasil)</option><option value="pt">Português</option><option value="en">Inglês</option><option value="es">Espanhol</option></select></td><td class="actions"><button data-sub-job="${l.id}">Buscar e baixar</button></td></tr>`).join('')||'<tr><td colspan="4">Nenhuma biblioteca.</td></tr>'}
      </tbody></table></div>
      <div class="phase2-hint">StormFlix não altera seus vídeos. As legendas baixadas ficam no storage de assets e são associadas ao catálogo.</div>
    </div>
    ${renderSubtitleJobs(jobs)}
  `;
  $$('[data-open-settings]').forEach(b=>b.onclick=()=>showSettings());
  $$('[data-sub-job]').forEach(b=>b.onclick=()=>startSubtitlePhase2(+b.dataset.subJob,$(`#sub-lang-${b.dataset.subJob}`).value));
  if((jobs||[]).some(j=>j.status==='queued'||j.status==='running'))phase2Timer=setTimeout(()=>{if(!$('#subtitles').classList.contains('hidden'))loadSubtitlesPhase2()},2000);
}

function renderSubtitleJobs(jobs){
  return `<div class="panel"><h2>Jobs de legendas</h2><div class="table-wrap"><table><thead><tr><th>Biblioteca</th><th>Idioma</th><th>Status</th><th>Progresso</th><th>Baixadas</th><th>Erros</th></tr></thead><tbody>${(jobs||[]).map(j=>{const pct=j.total?Math.round(j.processed*100/j.total):0;return `<tr><td>${esc(j.library)}</td><td>${esc(j.language)}</td><td class="${jobClass(j.status)}">${esc(j.status)}</td><td><div class="progress"><span style="width:${pct}%"></span></div><small>${j.processed}/${j.total}</small></td><td>${j.downloaded}</td><td>${j.failed}</td></tr>`}).join('')||'<tr><td colspan="6"><small>Nenhum job executado.</small></td></tr>'}</tbody></table></div></div>`;
}

async function startSubtitlePhase2(libraryID,language){
  try{
    notice('Job de legendas iniciado.',true);
    await req(`/admin/libraries/${libraryID}/subtitles`,{method:'POST',body:JSON.stringify({language})});
    await loadSubtitlesPhase2();
  }catch(err){notice(err.message)}
}

async function loadCDNPhase2(){
  const agents=await req('/admin/agents');
  const a=agents.assets||{};
  $('#cdn').innerHTML=`
    <div class="cdn-box">
      ${card('Modo',esc(a.mode||'local'))}
      ${card('Diretório',esc(a.directory||''))}
      ${card('URL pública',esc(a.public_base_url||'StormFlix /assets'))}
    </div>
    <div class="panel">
      <div class="panel-head"><div><h2>Storage de capas, fanart e legendas</h2><p>${esc(a.note||'')}</p></div>${me?.role==='admin'?'<button class="primary" data-open-settings>Configurar CDN / Assets</button>':''}</div>
      <div class="phase2-hint">Tudo é configurado em <b>Configurações → CDN / Assets</b>. O diretório pode ser local ou um mount visível no container, incluindo Google Drive, FTP, S3 ou WebDAV via rclone.</div>
    </div>`;
  $$('[data-open-settings]').forEach(b=>b.onclick=()=>showSettings());
}

function jobClass(status){
  if(status==='running'||status==='queued')return'job-running';
  if(status==='completed')return'job-completed';
  return status.includes('error')||status==='failed'?'job-error':'';
}
