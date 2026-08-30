/* StormFlix Admin: catalog health, technical automation, backups, scan preview and profile Home. */
(function(){
  let playbackTimer=null;
  let profileOverview=null;
  let currentHealthIssue='';

  function init(){
    const nav=document.querySelector('nav [data-page="automation"]');
    if(!nav)return;
    nav.addEventListener('click',()=>setTimeout(()=>{document.querySelector('#page-title').textContent='Saúde & Automação';loadAutomation()},0));
  }

  async function loadAutomation(){
    clearInterval(playbackTimer);playbackTimer=null;
    const root=$('#automation');if(!root)return;
    root.innerHTML='<div class="catalog-admin-loading">Analisando catálogo e automações…</div>';
    try{
      if(!Array.isArray(libs)||!libs.length)libs=await req('/admin/storage');
      const [health,technical,duplicates,backups,history,profiles,playbacks]=await Promise.all([
        req('/admin/catalog/health'),req('/admin/catalog/technical/status'),req('/admin/catalog/duplicates'),req('/admin/backups'),req('/admin/catalog/history?limit=80'),req('/admin/profile-home'),req('/admin/playbacks')
      ]);
      profileOverview=profiles;
      renderAutomation(root,{health,technical,duplicates,backups,history,playbacks});
      playbackTimer=setInterval(async()=>{
        if($('#automation')?.classList.contains('hidden')){clearInterval(playbackTimer);playbackTimer=null;return}
        try{renderPlaybackDiagnostics(await req('/admin/playbacks'))}catch{}
      },5000);
    }catch(err){root.innerHTML=`<div class="panel"><p class="offline">${esc(err.message)}</p></div>`}
  }

  function renderAutomation(root,data){
    const h=data.health||{},t=data.technical||{};
    root.innerHTML=`<div class="automation-shell">
      <section class="automation-hero panel"><div><p class="kicker">Controle automático</p><h2>Saúde do catálogo</h2><p>Problemas acionáveis, análise dos streams, duplicados, scans seguros, backups e Home por perfil em um só lugar.</p></div><button class="primary" id="automation-refresh">Atualizar tudo</button></section>
      <div class="health-grid">
        ${healthCard('total','Títulos disponíveis',h.total||0,'ok')}
        ${healthCard('sem_metadados','Sem metadados',h.sem_metadados||0,'warn')}
        ${healthCard('sem_capa','Sem capa',h.sem_capa||0,'warn')}
        ${healthCard('sem_genero','Sem gênero',h.sem_genero||0,'warn')}
        ${healthCard('outros','Em Outros',h.outros||0,'info')}
        ${healthCard('indisponiveis','Indisponíveis',h.indisponiveis||0,'danger')}
        ${healthCard('duplicados','Grupos duplicados',h.duplicados||0,'info')}
        ${healthCard('tecnico_pendente','Análise técnica pendente',h.tecnico_pendente||0,'info')}
      </div>
      <div id="health-drilldown"></div>

      <div class="automation-grid-two">
        <section class="panel"><div class="panel-head"><div><h2>Análise técnica automática</h2><p>Detecta resolução, HDR, codec, áudio e legenda diretamente do arquivo sem transcodificar vídeo.</p></div><button id="technical-rescan">Reanalisar tudo</button></div>
          <div class="technical-meter"><div><b>${Number(t.ready||0)}</b><span>Prontos</span></div><div><b>${Number(t.pending||0)}</b><span>Pendentes</span></div><div><b>${Number(t.failed||0)}</b><span>Falhas</span></div><div><b>${Number(t.total||0)}</b><span>Total</span></div></div>
          <p class="phase2-hint">O indexador trabalha com um ffprobe por vez para não massacrar Google Drive/rclone. Seções Dublado/Legendado/4K/HDR passam a se preencher sozinhas conforme a análise avança.</p>
        </section>
        <section class="panel"><div class="panel-head"><div><h2>Simular scan</h2><p>Veja o que mudaria antes de tocar no catálogo.</p></div></div>
          <div class="scan-preview-controls"><select id="scan-preview-library">${(libs||[]).filter(l=>l.kind!=='music').map(l=>`<option value="${l.id}">${esc(l.name)}</option>`).join('')}</select><button id="scan-preview-run">Simular</button></div><div id="scan-preview-result"><small>Nenhuma simulação executada.</small></div>
        </section>
      </div>

      <section class="panel"><div class="panel-head"><div><h2>Reproduzindo agora · diagnóstico</h2><p>Direct Play, HLS, buffer, velocidade de leitura e cache por sessão.</p></div></div><div id="automation-playbacks"></div></section>
      <section class="panel"><div class="panel-head"><div><h2>Duplicados e versões</h2><p>Um card lógico pode ter várias cópias físicas/qualidades; o player continua podendo escolher a melhor fonte.</p></div></div>${renderDuplicates(data.duplicates||[])}</section>
      <section class="panel"><div class="panel-head"><div><h2>Home por perfil</h2><p>Escolha quais menus cada perfil vê e a ordem deles.</p></div><button id="profile-home-save">Salvar perfil</button></div><div id="profile-home-editor">${renderProfileHome()}</div></section>
      <section class="panel"><div class="panel-head"><div><h2>Backups automáticos</h2><p>Scans grandes, mudanças de caminho e reorganização criam backup com limite de retenção. Restaurar é aplicado com segurança no próximo reinício.</p></div><button id="backup-create">Criar backup agora</button></div><div id="backup-list">${renderBackups(data.backups||[])}</div></section>
      <section class="panel"><div class="panel-head"><div><h2>Histórico do catálogo</h2><p>Auditoria das mudanças automáticas e administrativas.</p></div></div><div class="history-list">${renderHistory(data.history||[])}</div></section>
    </div>`;

    $('#automation-refresh').onclick=loadAutomation;
    root.querySelectorAll('[data-health-issue]').forEach(card=>card.onclick=()=>loadHealthIssue(card.dataset.healthIssue));
    $('#technical-rescan').onclick=async()=>{if(!confirm('Recolocar todos os arquivos na fila de análise técnica? O processo continua em segundo plano e usa apenas um ffprobe por vez.'))return;try{await req('/admin/catalog/technical/scan',{method:'POST',body:'{}'});notice('Reanálise técnica iniciada em segundo plano.',true);setTimeout(loadAutomation,500)}catch(err){notice(err.message)}};
    $('#scan-preview-run').onclick=runScanPreview;
    $('#profile-home-save').onclick=saveProfileHome;
    $('#profile-home-select')?.addEventListener('change',()=>{$('#profile-home-editor').innerHTML=renderProfileHome(Number($('#profile-home-select').value));wireProfileRows()});
    $('#backup-create').onclick=async()=>{try{await req('/admin/backups',{method:'POST',body:'{}'});notice('Backup criado.',true);loadAutomation()}catch(err){notice(err.message)}};
    root.querySelectorAll('[data-backup-restore]').forEach(b=>b.onclick=()=>restoreBackup(Number(b.dataset.backupRestore)));
    renderPlaybackDiagnostics(data.playbacks||[]);wireProfileRows();
  }

  function healthCard(issue,label,value,tone){return`<button class="health-card ${tone}" data-health-issue="${issue}"><strong>${Number(value||0)}</strong><span>${esc(label)}</span><small>Clique para ver</small></button>`}

  async function loadHealthIssue(issue){
    currentHealthIssue=issue;const root=$('#health-drilldown');if(!root)return;
    if(issue==='duplicados'){root.innerHTML='<div class="phase2-hint">Os grupos duplicados estão na seção “Duplicados e versões” abaixo.</div>';return}
    if(issue==='tecnico_pendente'){root.innerHTML='<div class="phase2-hint">A análise técnica roda automaticamente em segundo plano. Use “Reanalisar tudo” somente quando quiser invalidar o cache atual.</div>';return}
    root.innerHTML='<div class="catalog-admin-loading">Carregando títulos…</div>';
    try{
      const result=await req(`/admin/catalog/health/items?issue=${encodeURIComponent(issue)}&limit=100`);
      root.innerHTML=`<section class="panel health-results"><div class="panel-head"><h3>${esc(issue.replaceAll('_',' '))} · ${result.total||0}</h3><button id="health-close">Fechar</button></div><div class="health-result-grid">${(result.items||[]).map(item=>`<article>${item.poster_url?`<img src="${esc(item.poster_url)}" loading="lazy" decoding="async">`:'<div class="poster-mini">?</div>'}<div><b>${esc(item.title)}</b><small>${esc(item.library)} · ${item.year||'—'} · ${esc(item.metadata_status||'pending')}</small></div></article>`).join('')||'<p>Nenhum título.</p>'}</div></section>`;
      $('#health-close').onclick=()=>{$('#health-drilldown').innerHTML='';currentHealthIssue=''};
    }catch(err){root.innerHTML=`<p class="offline">${esc(err.message)}</p>`}
  }

  async function runScanPreview(){
    const id=Number($('#scan-preview-library').value),root=$('#scan-preview-result');if(!id||!root)return;
    root.innerHTML='<small>Listando o mount sem alterar o catálogo…</small>';
    try{
      const p=await req(`/admin/libraries/${id}/scan-preview`,{method:'POST',body:'{}'});
      root.innerHTML=`<div class="scan-preview-stats"><span><b>${p.new||0}</b> novos</span><span><b>${p.changed||0}</b> alterados</span><span><b>${p.missing||0}</b> ausentes</span><span><b>${p.unchanged||0}</b> iguais</span><span><b>${p.sources_offline||0}</b> origem(ns) offline</span></div>${(p.samples||[]).length?`<details><summary>Exemplos das mudanças</summary><div class="scan-preview-samples">${p.samples.map(x=>`<div><b>${esc(x.change)}</b><span>${esc(x.title||x.path)}</span></div>`).join('')}</div></details>`:''}<button class="primary" id="scan-preview-apply">Executar scan real</button>`;
      $('#scan-preview-apply').onclick=async()=>{if(!confirm(`Executar o scan real? ${p.new||0} novos, ${p.changed||0} alterados e ${p.missing||0} ausentes. Um backup automático será criado antes.`))return;try{await req(`/libraries/${id}/scan`,{method:'POST',body:'{}'});notice('Scan colocado na fila.',true)}catch(err){notice(err.message)}};
    }catch(err){root.innerHTML=`<p class="offline">${esc(err.message)}</p>`}
  }

  function renderPlaybackDiagnostics(items){
    const root=$('#automation-playbacks');if(!root)return;
    if(!items.length){root.innerHTML='<p><small>Ninguém reproduzindo agora.</small></p>';return}
    root.innerHTML=`<div class="playback-diagnostic-grid">${items.map(p=>{const buffer=Number(p.buffer_seconds||0),tone=buffer<8?'danger':buffer<18?'warn':'ok';return`<article><div class="playback-diagnostic-head"><div><b>${esc(p.title)}</b><small>${esc(p.display_name)} · ${esc(p.device)}</small></div><span class="mode-chip">${playModePT(p.mode)}</span></div><div class="playback-diagnostic-stats"><span><b>${buffer.toFixed(1)}s</b> buffer</span><span class="${tone}"><b>${Number(p.read_mbps||0).toFixed(1)}</b> Mb/s</span><span><b>${formatBytes(p.cache_bytes||0)}</b> cache</span><span><b>${Number(p.bitrate_kbps||0)?(Number(p.bitrate_kbps)/1000).toFixed(1):'—'}</b> Mb/s mídia</span></div><small>${esc(p.video_codec||'')} ${p.audio_codec?'· '+esc(p.audio_codec):''}${p.last_error?' · ERRO: '+esc(p.last_error):''}</small></article>`}).join('')}</div>`;
  }
  function playModePT(mode){return({direct_play:'Direct Play',remux:'Remux HLS',audio_compatibility:'Áudio AAC',dynamic_hls:'HLS',web_remux:'Remux HLS',direct_stream_audio_aac:'Áudio AAC',music:'Música'})[mode]||mode||'Direct Play'}

  function renderDuplicates(groups){if(!groups.length)return'<p><small>Nenhum grupo duplicado detectado.</small></p>';return`<div class="duplicate-list">${groups.slice(0,50).map(g=>`<article><div><b>${esc(g.title)}</b><small>${g.year||''} · ${esc(g.media_type||'')} · ${g.copies.length} versões físicas</small></div><div class="duplicate-copies">${g.copies.map(c=>`<span>${esc(c.library)} · #${c.id}</span>`).join('')}</div></article>`).join('')}</div>`}

  function renderProfileHome(selectedID){
    const data=profileOverview||{profiles:[],menus:[]},profiles=data.profiles||[];if(!profiles.length)return'<small>Nenhum perfil.</small>';
    const selected=profiles.find(p=>Number(p.id)===Number(selectedID))||profiles[0];
    const config=new Map((selected.menus||[]).map(x=>[Number(x.category_id),x]));
    const ordered=[...(data.menus||[])].sort((a,b)=>Number(config.get(Number(a.id))?.sort_order||a.sort_order||0)-Number(config.get(Number(b.id))?.sort_order||b.sort_order||0));
    return `<label class="profile-home-select"><span>Perfil</span><select id="profile-home-select">${profiles.map(p=>`<option value="${p.id}" ${Number(p.id)===Number(selected.id)?'selected':''}>${esc(p.user)} · ${esc(p.name)}${p.is_kids?' · Infantil':''}</option>`).join('')}</select></label><div class="profile-home-sort" id="profile-home-sort">${ordered.map((menu,i)=>{const pref=config.get(Number(menu.id)),visible=pref?pref.visible!==false:true;return`<div class="profile-home-row" draggable="true" data-menu-id="${menu.id}"><span class="home-drag-handle">⋮⋮</span><label><input type="checkbox" data-profile-menu-visible ${visible?'checked':''}><b>${esc(menu.name)}</b></label><small>${i+1}º</small></div>`}).join('')}</div>`;
  }

  function wireProfileRows(){
    const root=$('#profile-home-sort');if(!root)return;let dragged=null;
    root.querySelectorAll('.profile-home-row').forEach(row=>{row.ondragstart=()=>{dragged=row;row.classList.add('is-dragging')};row.ondragend=()=>{row.classList.remove('is-dragging');dragged=null;refreshProfilePositionLabels()};row.ondragover=e=>{if(!dragged||dragged===row)return;e.preventDefault();const rect=row.getBoundingClientRect();root.insertBefore(dragged,e.clientY<rect.top+rect.height/2?row:row.nextSibling)}});
  }
  function refreshProfilePositionLabels(){$$('#profile-home-sort .profile-home-row').forEach((row,i)=>{const s=row.querySelector('small');if(s)s.textContent=`${i+1}º`})}
  async function saveProfileHome(){const profileID=Number($('#profile-home-select')?.value);if(!profileID)return;const menus=$$('#profile-home-sort .profile-home-row').map((row,i)=>({category_id:Number(row.dataset.menuId),visible:Boolean(row.querySelector('[data-profile-menu-visible]')?.checked),sort_order:(i+1)*10}));try{await req(`/admin/profiles/${profileID}/home-menus`,{method:'PUT',body:JSON.stringify({menus})});notice('Home do perfil salva.',true);loadAutomation()}catch(err){notice(err.message)}}

  function renderBackups(items){if(!items.length)return'<small>Nenhum backup registrado.</small>';return`<div class="backup-list">${items.map(b=>`<article><div><b>${esc(b.name)}</b><small>${esc(b.kind==='auto'?'Automático':'Manual')} · ${formatBytes(b.size_bytes)} · ${esc(b.created_at)}${b.note?' · '+esc(b.note):''}</small></div><button class="danger" data-backup-restore="${b.id}">Restaurar</button></article>`).join('')}</div>`}
  async function restoreBackup(id){if(!confirm('Preparar este backup para restauração? O banco atual só será trocado no próximo reinício do container e uma cópia de segurança do estado atual será preservada.'))return;try{const r=await req(`/admin/backups/${id}/restore`,{method:'POST',body:'{}'});notice(r.message||'Restauração agendada.',true)}catch(err){notice(err.message)}}
  function renderHistory(items){if(!items.length)return'<small>Sem alterações registradas ainda.</small>';return items.map(x=>`<article><div><b>${esc(x.summary||x.action)}</b><small>${esc(x.entity_type)} ${esc(x.entity_id||'')} · ${esc(x.user||'sistema')}</small></div><time>${esc(x.created_at)}</time></article>`).join('')}
  function formatBytes(n){n=Number(n||0);if(!n)return'0 B';const u=['B','KB','MB','GB','TB'];const i=Math.min(Math.floor(Math.log(n)/Math.log(1024)),u.length-1);return`${(n/1024**i).toFixed(i?1:0)} ${u[i]}`}

  window.loadAutomation=loadAutomation;
  document.addEventListener('DOMContentLoaded',init);
})();
