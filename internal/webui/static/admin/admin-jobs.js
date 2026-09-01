/* StormFlix Admin unified operational queue: scans + metadata + intro detection. */
(function(){
  let timer=null;
  const activeStatus=s=>['queued','running','cancelling'].includes(String(s||''));
  const statusLabel=s=>({queued:'NA FILA',running:'EXECUTANDO',cancelling:'CANCELANDO',completed:'CONCLUÍDO',completed_with_errors:'CONCLUÍDO COM ERROS',failed:'ERRO',error:'ERRO',cancelled:'CANCELADO',timeout:'TIMEOUT'}[s]||String(s||'').toUpperCase());

  const nav=document.querySelector('nav [data-page="jobs"]');
  if(nav)nav.addEventListener('click',()=>setTimeout(()=>loadJobs(true),0));

  const baseLoadLibraries=window.loadLibraries;
  if(typeof baseLoadLibraries==='function')window.loadLibraries=async function(){
    await baseLoadLibraries();
    await decorateLibraryQueue();
  };

  // The v4 library card still calls the global scanLib function. Override it
  // after v4 loads so one-library scans use the same persistent queue as
  // "Escanear todas" and immediately take the operator to observable status.
  window.scanLib=async function(id){
    id=Number(id);
    try{
      const r=await req(`/libraries/${id}/scan`,{method:'POST',body:'{}'});
      notice(r.status==='queued'?`Biblioteca adicionada à fila · ${r.message||''}`:`Scan ${r.status||'iniciado'}.`,true);
      await window.loadLibraries?.();
      await decorateLibraryQueue();
    }catch(err){notice(err.message)}
  };

  async function loadJobs(forcePoll=false){
    const root=document.querySelector('#jobs');if(!root)return;
    const title=document.querySelector('#page-title');if(title)title.textContent='Fila & atividades';
    if(root.innerHTML.trim()==='')root.innerHTML='<div class="job-empty">Carregando fila…</div>';
    try{
      const jobs=await req('/admin/jobs?limit=100');
      renderJobs(root,jobs);
      if(forcePoll||jobs.some(j=>activeStatus(j.status)))schedule();else stop();
    }catch(err){root.innerHTML=`<div class="panel"><p class="offline">${esc(err.message)}</p></div>`;stop()}
  }

  function renderJobs(root,jobs){
    const running=jobs.filter(j=>j.status==='running'||j.status==='cancelling').length;
    const queued=jobs.filter(j=>j.status==='queued').length;
    const errors=jobs.filter(j=>['failed','error','completed_with_errors','timeout'].includes(j.status)).length;
    root.innerHTML=`<div class="job-queue-head"><div><p class="kicker">TRABALHOS EM SEGUNDO PLANO</p><h2>Fila & atividades</h2><p>Acompanhe scans, metadados, reorganização de episódios e a detecção automática de introduções.</p></div><div class="job-queue-summary"><span>${running} executando</span><span>${queued} na fila</span><span>${errors} com alerta</span></div></div><div class="panel"><div class="panel-head"><div><h2>Fila operacional</h2><small>Trabalhos pesados são serializados e a detecção de intros pausa para priorizar reproduções ativas.</small></div><button class="primary" data-scan-all>Escanear todas</button></div><div class="job-list">${jobs.length?jobs.map(jobCard).join(''):'<div class="job-empty">Nenhum trabalho registrado ainda.</div>'}</div></div>`;
    root.querySelector('[data-scan-all]')?.addEventListener('click',scanAll);
    root.querySelectorAll('[data-cancel-scan]').forEach(b=>b.onclick=()=>cancelScan(Number(b.dataset.cancelScan)));
  }

  function jobCard(j){
    const active=activeStatus(j.status);
    const pct=Math.max(0,Math.min(100,Number(j.progress||0)));
    const count=j.total>0?`${Number(j.current||0).toLocaleString('pt-BR')} / ${Number(j.total||0).toLocaleString('pt-BR')}`:(j.kind==='scan'?`${Number(j.current||0).toLocaleString('pt-BR')} arquivo(s)`:Number(j.current||0).toLocaleString('pt-BR'));
    return `<article class="job-card ${active?'active':''}"><div class="job-card-head"><div><h3>${esc(j.label)}</h3><p>${esc(j.library)} · ${esc(j.kind)}</p></div><span class="job-status ${esc(j.status)}">${esc(statusLabel(j.status))}</span></div><div class="job-progress"><span style="width:${active&&pct===0?3:pct}%"></span></div><div class="job-card-meta"><span>${count}${j.total>0?` · ${pct}%`:''}</span><span>OK ${Number(j.success||0).toLocaleString('pt-BR')} · Erros ${Number(j.failed||0).toLocaleString('pt-BR')}</span></div><div class="job-message">${esc(j.message||'')}</div>${j.kind==='scan'&&active?`<div class="actions" style="margin-top:10px"><button class="danger" data-cancel-scan="${j.library_id}">Cancelar scan</button></div>`:''}</article>`;
  }

  async function scanAll(){
    if(!confirm('Colocar todas as bibliotecas ativas na fila de scan? Elas serão processadas uma por vez.'))return;
    try{const r=await req('/admin/libraries/scan-all',{method:'POST',body:'{}'});notice(`${r.queued||0} biblioteca(s) adicionada(s) à fila.`,true);await loadJobs(true);await decorateLibraryQueue()}catch(err){notice(err.message)}
  }

  async function cancelScan(libraryID){
    try{await req(`/libraries/${libraryID}/scan/cancel`,{method:'POST'});notice('Cancelamento solicitado.',true);await loadJobs(true);await decorateLibraryQueue()}catch(err){notice(err.message)}
  }

  async function decorateLibraryQueue(){
    const page=document.querySelector('#libraries');if(!page)return;
    page.querySelectorAll('[data-library-queue-strip]').forEach(x=>x.remove());
    try{
      const jobs=await req('/admin/jobs?limit=40');
      const scanJobs=jobs.filter(j=>j.kind==='scan'&&activeStatus(j.status));
      const running=scanJobs.find(j=>j.status==='running'||j.status==='cancelling');
      const queued=scanJobs.filter(j=>j.status==='queued').length;
      const strip=document.createElement('div');strip.className='library-queue-strip';strip.dataset.libraryQueueStrip='1';
      strip.innerHTML=`<div><strong>${running?`Escaneando: ${esc(running.library)}`:'Nenhum scan executando'}</strong><small>${running?esc(running.message||'Em andamento'):`${queued} biblioteca(s) aguardando na fila`}</small></div><div class="library-queue-actions"><button data-library-scan-all class="primary">Escanear todas</button><button data-open-jobs>Ver fila completa</button></div>`;
      const intro=page.querySelector('.library-page-intro')||page.firstElementChild;if(intro)intro.after(strip);else page.prepend(strip);
      strip.querySelector('[data-library-scan-all]').onclick=scanAll;
      strip.querySelector('[data-open-jobs]').onclick=()=>document.querySelector('nav [data-page="jobs"]')?.click();

      // Reflect queued/running state directly on each library card, not only on
      // the separate queue page.
      for(const job of scanJobs){
        const card=page.querySelector(`[data-library-card="${job.library_id}"]`);if(!card)continue;
        const actions=card.querySelector('.library-actions');
        const scanButton=actions?.querySelector('button');
        if(scanButton){scanButton.disabled=true;scanButton.textContent=job.status==='queued'?'Na fila':job.status==='cancelling'?'Cancelando…':'Escaneando…'}
        if(actions&&!actions.querySelector('[data-queue-cancel]')){
          const cancel=document.createElement('button');cancel.className='danger';cancel.dataset.queueCancel='1';cancel.textContent=job.status==='queued'?'Remover da fila':'Cancelar scan';cancel.onclick=()=>cancelScan(job.library_id);actions.insertBefore(cancel,actions.children[1]||null);
        }
      }
      if(scanJobs.length)scheduleLibraryRefresh();
    }catch{}
  }

  function schedule(){stop();timer=setTimeout(()=>{if(!document.querySelector('#jobs')?.classList.contains('hidden'))loadJobs(true)},1500)}
  function scheduleLibraryRefresh(){setTimeout(()=>{if(!document.querySelector('#libraries')?.classList.contains('hidden')){window.loadLibraries?.().catch(()=>{})}},1800)}
  function stop(){if(timer){clearTimeout(timer);timer=null}}

  window.sfAdminJobs={reload:()=>loadJobs(true),scanAll};
})();
