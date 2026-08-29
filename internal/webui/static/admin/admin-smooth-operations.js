/* StormFlix Admin: stable polling for scans/metadata + bulk metadata controls. */
(function(){
  const originalLibraryLoader=window.loadLibraries;
  const originalMetadataLoader=window.loadMetadataPhase2;
  let metadataInitialized=false;
  let metadataBulkRunning=false;

  const activeStatus=value=>['queued','running','cancelling'].includes(String(value||''));
  const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));

  function sourcePaths(lib){
    if(Array.isArray(lib?.sources)&&lib.sources.length)return lib.sources.map(s=>String(s.path||''));
    if(Array.isArray(lib?.paths)&&lib.paths.length)return lib.paths.map(String);
    return lib?.path?[String(lib.path)]:[];
  }

  function libraryStructure(items){
    return (items||[]).map(lib=>[
      Number(lib.id),String(lib.name||''),String(lib.kind||''),Boolean(lib.enabled),sourcePaths(lib).join('|')
    ].join('::')).join(';;');
  }

  function patchLibraryCard(lib,scanJob){
    const card=document.querySelector(`[data-library-card="${Number(lib.id)}"]`);
    if(!card)return;
    const stats=card.querySelectorAll('.library-stat strong');
    if(stats[0])stats[0].textContent=Number(lib.media_count||0).toLocaleString('pt-BR');
    if(stats[1])stats[1].textContent=Number(lib.source_count||sourcePaths(lib).length||1).toLocaleString('pt-BR');
    if(stats[2])stats[2].textContent=Number(lib.online_sources||0).toLocaleString('pt-BR');

    const topNote=card.querySelector('.library-card-top p');
    if(topNote)topNote.textContent=`${lib.enabled?'Biblioteca ativa':'Biblioteca desativada'} · ${lib.last_scan_at||'Nunca escaneada'}`;

    const sources=Array.isArray(lib.sources)&&lib.sources.length?lib.sources:sourcePaths(lib).map((path,index)=>({path,label:`Origem ${index+1}`,online:Boolean(lib.online)}));
    const sourceNodes=card.querySelectorAll('.library-source-item');
    if(sourceNodes.length===sources.length){
      sources.forEach((source,index)=>{
        const node=sourceNodes[index];
        node.classList.toggle('online',Boolean(source.online));
        node.title=source.path||'';
        const code=node.querySelector('code');if(code)code.textContent=source.path||'';
        const small=node.querySelector('small');if(small)small.textContent=source.online?'ONLINE':'OFFLINE';
      });
    }

    const queued=scanJob?.status==='queued';
    const cancelling=scanJob?.status==='cancelling'||lib.last_scan_status==='cancelling';
    const running=scanJob?.status==='running'||lib.last_scan_status==='running';
    const active=queued||running||cancelling;
    const health=!lib.enabled?'offline':Number(lib.online_sources||0)<=0?'offline':Number(lib.offline_sources||0)>0?'partial':'';
    const healthText=!lib.enabled?'Desativada':Number(lib.online_sources||0)<=0?'Offline':Number(lib.offline_sources||0)>0?'Parcial':'Online';
    const scanNote=card.querySelector('.library-scan-note');
    if(scanNote){
      const detail=scanJob?.message||lib.last_error||'';
      scanNote.innerHTML=`<span class="source-health ${health}">${esc(healthText)}</span> · Scan: ${esc(scanJob?.status||lib.last_scan_status||'never')}${detail?` · ${esc(detail)}`:''}`;
    }
    const scanButton=card.querySelector('.library-actions button[onclick^="scanLib("]');
    if(scanButton){
      scanButton.disabled=active;
      scanButton.textContent=queued?'Na fila':cancelling?'Cancelando…':running?'Escaneando…':'Escanear agora';
    }
  }

  async function smoothLoadLibraries(){
    if(typeof originalLibraryLoader!=='function')return;
    let fresh;
    try{fresh=await req('/admin/storage')}catch{return originalLibraryLoader()}
    const current=Array.isArray(libs)?libs:[];
    const hasCards=document.querySelector('#libraries .library-groups');
    if(!hasCards||libraryStructure(current)!==libraryStructure(fresh)){
      return originalLibraryLoader();
    }
    libs=fresh;
    let jobs=[];
    try{jobs=await req('/admin/jobs?limit=80')}catch{}
    const latestScan=new Map();
    for(const job of jobs||[]){
      if(job.kind!=='scan'||!activeStatus(job.status))continue;
      const id=Number(job.library_id);
      if(!latestScan.has(id)||Number(job.id)>Number(latestScan.get(id).id))latestScan.set(id,job);
    }
    for(const lib of fresh)patchLibraryCard(lib,latestScan.get(Number(lib.id)));

    const strip=document.querySelector('#libraries [data-library-queue-strip]');
    if(strip){
      const active=[...latestScan.values()];
      const running=active.find(j=>j.status==='running'||j.status==='cancelling');
      const queued=active.filter(j=>j.status==='queued').length;
      const strong=strip.querySelector('strong');
      const small=strip.querySelector('small');
      if(strong)strong.textContent=running?`Escaneando: ${running.library}`:'Nenhum scan executando';
      if(small)small.textContent=running?(running.message||'Em andamento'):`${queued} biblioteca(s) aguardando na fila`;
    }
  }

  if(typeof originalLibraryLoader==='function')window.loadLibraries=smoothLoadLibraries;

  function metadataCards(root,status){
    const values={Identificadas:Number(status.matched||0),Pendentes:Number(status.pending||0),'Com erro':Number(status.error||0)};
    root.querySelectorAll('.cards .card').forEach(card=>{
      const label=card.querySelector('span')?.textContent?.trim();
      if(!(label in values))return;
      const strong=card.querySelector('strong');if(strong)strong.textContent=values[label].toLocaleString('pt-BR');
    });
  }

  function metadataJobRows(jobs){
    return (jobs||[]).map(j=>{
      const pct=j.total?Math.round(Number(j.processed||0)*100/Number(j.total||1)):0;
      return `<tr data-meta-job="${Number(j.id)}"><td>${esc(j.library)}</td><td class="${jobClass(j.status)}">${esc(j.status)}</td><td><div class="progress"><span style="width:${pct}%"></span></div><small>${Number(j.processed||0).toLocaleString('pt-BR')}/${Number(j.total||0).toLocaleString('pt-BR')} · ${pct}%</small></td><td>${Number(j.matched||0).toLocaleString('pt-BR')}</td><td>${Number(j.failed||0).toLocaleString('pt-BR')}</td><td><small>${esc(j.message||'')}</small></td></tr>`;
    }).join('')||'<tr><td colspan="6"><small>Nenhum job executado.</small></td></tr>';
  }

  function patchMetadataJobs(root,jobs){
    const panels=[...root.querySelectorAll('.panel')];
    const panel=panels.find(p=>p.querySelector('h2')?.textContent?.trim()==='Jobs de metadados');
    const body=panel?.querySelector('tbody');
    if(body)body.innerHTML=metadataJobRows(jobs);

    const latest=new Map();
    for(const job of jobs||[]){
      const id=Number(job.library_id);
      if(!latest.has(id)||Number(job.id)>Number(latest.get(id).id))latest.set(id,job);
    }
    root.querySelectorAll('[data-meta-scan]').forEach(scan=>{
      const id=Number(scan.dataset.metaScan),job=latest.get(id),active=job&&activeStatus(job.status);
      const row=scan.closest('tr');
      row?.querySelectorAll('button').forEach(button=>button.disabled=Boolean(active));
      scan.textContent=active?`Em andamento ${Number(job.processed||0)}/${Number(job.total||0)}`:'Buscar metadados';
    });
  }

  function installMetadataBulk(root){
    const panel=[...root.querySelectorAll('.panel')].find(p=>p.querySelector('h2')?.textContent?.trim()==='Escanear capas e informações');
    const head=panel?.querySelector('.panel-head');
    if(!head||head.querySelector('[data-meta-bulk-actions]'))return;
    const actions=document.createElement('div');
    actions.className='actions';actions.dataset.metaBulkActions='1';
    actions.innerHTML='<button class="primary" data-meta-all-scan>Buscar em todas</button><button data-meta-all-refresh>Atualizar todas</button>';
    head.appendChild(actions);
    actions.querySelector('[data-meta-all-scan]').onclick=e=>runMetadataAll(false,e.currentTarget);
    actions.querySelector('[data-meta-all-refresh]').onclick=e=>runMetadataAll(true,e.currentTarget);
  }

  async function waitMetadataJob(libraryID,jobID){
    const deadline=Date.now()+45*60*1000;
    while(Date.now()<deadline){
      const jobs=await req('/admin/metadata/jobs?limit=160');
      let job=jobID?(jobs||[]).find(j=>Number(j.id)===Number(jobID)):null;
      if(!job)job=(jobs||[]).filter(j=>Number(j.library_id)===Number(libraryID)).sort((a,b)=>Number(b.id)-Number(a.id))[0];
      if(job&&!activeStatus(job.status))return job;
      await wait(1600);
    }
    throw new Error('tempo limite excedido acompanhando metadados');
  }

  async function runMetadataAll(refresh,button){
    if(metadataBulkRunning)return;
    const videoLibs=(Array.isArray(libs)?libs:[]).filter(lib=>lib.kind!=='music'&&lib.enabled!==false);
    if(!videoLibs.length){notice('Nenhuma biblioteca de vídeo ativa.');return}
    const question=refresh
      ?`Atualizar novamente metadados e capas de todas as ${videoLibs.length} bibliotecas de vídeo?`
      :`Buscar metadados pendentes em todas as ${videoLibs.length} bibliotecas de vídeo? Títulos já identificados serão preservados.`;
    if(!confirm(question))return;
    metadataBulkRunning=true;
    const allButtons=document.querySelectorAll('#metadata [data-meta-all-scan],#metadata [data-meta-all-refresh]');
    allButtons.forEach(b=>b.disabled=true);
    let completed=0,errors=0;
    try{
      for(const lib of videoLibs){
        button.textContent=`${refresh?'Atualizando':'Buscando'} ${completed+1}/${videoLibs.length}`;
        try{
          let job=null;
          try{job=await req(`/admin/libraries/${Number(lib.id)}/metadata${refresh?'?refresh=1':''}`,{method:'POST',body:'{}'})}
          catch(err){if(err.status!==409)throw err}
          await waitMetadataJob(Number(lib.id),job?.id);
        }catch(err){errors++;notice(`${lib.name}: ${err.message}`)}
        completed++;
        await smoothMetadataLoad();
      }
      notice(`${refresh?'Atualização':'Busca'} de metadados concluída em ${completed} biblioteca(s)${errors?` · ${errors} com erro`:''}.`,errors===0);
    }finally{
      metadataBulkRunning=false;
      allButtons.forEach(b=>b.disabled=false);
      const scan=document.querySelector('#metadata [data-meta-all-scan]');if(scan)scan.textContent='Buscar em todas';
      const refreshButton=document.querySelector('#metadata [data-meta-all-refresh]');if(refreshButton)refreshButton.textContent='Atualizar todas';
    }
  }

  async function smoothMetadataLoad(){
    if(typeof originalMetadataLoader!=='function')return;
    const root=document.querySelector('#metadata');
    if(!metadataInitialized||!root?.querySelector('.panel')){
      await originalMetadataLoader();
      metadataInitialized=true;
      installMetadataBulk(root);
      return;
    }
    const [status,jobs]=await Promise.all([req('/admin/metadata/status'),req('/admin/metadata/jobs?limit=40')]);
    metadataCards(root,status);
    patchMetadataJobs(root,jobs);
    installMetadataBulk(root);
    if(phase2Timer){clearTimeout(phase2Timer);phase2Timer=null}
    if((jobs||[]).some(j=>activeStatus(j.status))){
      phase2Timer=setTimeout(()=>{if(!root.classList.contains('hidden'))smoothMetadataLoad().catch(err=>notice(err.message))},2000);
    }
  }

  if(typeof originalMetadataLoader==='function'){
    window.loadMetadataPhase2=smoothMetadataLoad;
  }

  window.sfSmoothAdminOperations={reloadLibraries:smoothLoadLibraries,reloadMetadata:smoothMetadataLoad};
})();
