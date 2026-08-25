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
      <div class="panel-head"><div><h2>Agentes de metadados</h2><small>Os agentes podem trabalhar juntos. Anime prioriza AniList; filmes/séries priorizam TMDB; Fanart.tv enriquece artwork.</small></div></div>
      <div class="agent-grid">${(agents.metadata||[]).map(renderAgent).join('')}</div>
      ${!(agents.metadata||[]).some(a=>a.name==='TMDB'&&a.ready)?'<div class="phase2-hint">Para filmes e séries configure <b>STORMFLIX_TMDB_TOKEN</b> ou <b>STORMFLIX_TMDB_API_KEY</b>.</div>':''}
    </div>
    <div class="panel">
      <div class="panel-head"><div><h2>Escanear capas e informações</h2><small>O trabalho roda em segundo plano e não interrompe streaming.</small></div></div>
      <div class="table-wrap"><table><thead><tr><th>Biblioteca</th><th>Tipo</th><th>Mídias</th><th>Ações</th></tr></thead><tbody>
        ${libs.map(l=>`<tr><td><b>${esc(l.name)}</b></td><td>${esc(l.kind)}</td><td>${l.media_count||0}</td><td class="actions"><button data-meta-scan="${l.id}">Buscar metadados</button><button data-meta-refresh="${l.id}">Atualizar tudo</button></td></tr>`).join('')||'<tr><td colspan="4">Nenhuma biblioteca.</td></tr>'}
      </tbody></table></div>
    </div>
    ${renderMetadataJobs(jobs)}
  `;
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
    <div class="panel"><div class="panel-head"><div><h2>Agentes de legenda</h2><small>Download automático é opcional e somente acontece quando você inicia um job.</small></div></div><div class="agent-grid">${(agents.subtitles||[]).map(renderAgent).join('')}</div></div>
    <div class="panel">
      <div class="panel-head"><div><h2>Baixar automaticamente</h2><small>Requer metadados identificados para usar TMDB/IMDb e melhorar a correspondência.</small></div></div>
      <div class="table-wrap"><table><thead><tr><th>Biblioteca</th><th>Mídias</th><th>Idioma</th><th>Ação</th></tr></thead><tbody>
      ${libs.map(l=>`<tr><td><b>${esc(l.name)}</b></td><td>${l.media_count||0}</td><td><select id="sub-lang-${l.id}"><option value="pt-BR">Português (Brasil)</option><option value="pt">Português</option><option value="en">Inglês</option><option value="es">Espanhol</option></select></td><td class="actions"><button data-sub-job="${l.id}">Buscar e baixar</button></td></tr>`).join('')||'<tr><td colspan="4">Nenhuma biblioteca.</td></tr>'}
      </tbody></table></div>
      <div class="phase2-hint">StormFlix não altera seus vídeos no Google Drive. As legendas baixadas ficam no storage de assets e são associadas ao catálogo.</div>
    </div>
    ${renderSubtitleJobs(jobs)}
  `;
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
      <h2>Storage de capas, fanart e legendas</h2>
      <p>${esc(a.note||'')}</p>
      <div class="phase2-hint">O StormFlix trata isto como o origin dos assets. Pode ser disco local ou uma pasta montada via rclone em Google Drive, FTP, S3, WebDAV e outros. Se houver um domínio/CDN na frente, configure a URL pública.</div>
    </div>
    <div class="panel">
      <h2>Configuração</h2>
      <pre class="config-code"># Local (padrão)\nSTORMFLIX_ASSET_DIR=/data/assets\n\n# Google Drive/FTP montado por rclone\nSTORMFLIX_ASSET_DIR=/assets-remote\n\n# Opcional: domínio externo/CDN que serve a mesma pasta\nSTORMFLIX_ASSET_PUBLIC_BASE_URL=https://cdn.seudominio.com/stormflix\n\n# Agentes de metadata\nSTORMFLIX_TMDB_TOKEN=...\nSTORMFLIX_FANART_API_KEY=...\nSTORMFLIX_FANART_CLIENT_KEY=...\n\n# Legendas\nSTORMFLIX_OPENSUBTITLES_API_KEY=...\nSTORMFLIX_OPENSUBTITLES_USERNAME=...\nSTORMFLIX_OPENSUBTITLES_PASSWORD=...\nSTORMFLIX_SUBDL_API_KEY=...</pre>
    </div>`;
}

function jobClass(status){
  if(status==='running'||status==='queued')return'job-running';
  if(status==='completed')return'job-completed';
  return status.includes('error')||status==='failed'?'job-error':'';
}
