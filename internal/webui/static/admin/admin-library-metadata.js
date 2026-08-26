/* StormFlix Admin: metadata controls directly on video libraries. */
(function(){
  const running=new Map();
  let allRunning=false;

  const baseLoadLibraries=window.loadLibraries;
  if(typeof baseLoadLibraries==='function')window.loadLibraries=async function(){
    await baseLoadLibraries();
    await decorateVideoMetadataControls();
  };

  async function decorateVideoMetadataControls(){
    const group=document.querySelector('#libraries .library-group.video');
    if(!group)return;
    const videoLibs=(window.libs||[]).filter(l=>l.kind!=='music');
    const groupActions=group.querySelector('.library-group-actions');
    if(groupActions&&!groupActions.querySelector('[data-video-metadata-all]')){
      const button=document.createElement('button');
      button.type='button';button.className='library-tool';button.dataset.videoMetadataAll='1';button.textContent='Metadados de todas';
      button.onclick=()=>runAllVideoMetadata(button);
      groupActions.prepend(button);
    }
    for(const lib of videoLibs){
      const actions=group.querySelector(`[data-library-card="${lib.id}"] .library-actions`);if(!actions||actions.querySelector(`[data-video-metadata="${lib.id}"]`))continue;
      const button=document.createElement('button');
      button.type='button';button.dataset.videoMetadata=String(lib.id);button.textContent='Metadados';button.title='Buscar metadados e capas pendentes desta biblioteca';
      button.onclick=()=>runLibraryMetadata(Number(lib.id),button);
      const edit=actions.querySelector(`button[onclick="editLibrary(${lib.id})"]`);
      if(edit)actions.insertBefore(button,edit);else actions.appendChild(button);
    }
    await syncMetadataButtons().catch(()=>{});
  }

  async function runLibraryMetadata(id,button){
    if(running.has(id))return;
    if(button){button.disabled=true;button.textContent='Iniciando…'}
    try{
      let job;
      try{job=await req(`/admin/libraries/${id}/metadata`,{method:'POST',body:'{}'})}
      catch(err){
        if(err.status!==409)throw err;
      }
      notice(job?.library?`${job.library}: busca de metadados iniciada.`:'Esta biblioteca já possui uma busca de metadados em andamento.',true);
      await watchMetadataJob(id,button);
    }catch(err){notice(err.message);if(button){button.disabled=false;button.textContent='Metadados'}}
  }

  async function runAllVideoMetadata(button){
    if(allRunning)return;
    allRunning=true;if(button){button.disabled=true;button.textContent='Metadados 0%'}
    const videoLibs=(window.libs||[]).filter(l=>l.kind!=='music');
    let done=0,errors=0;
    try{
      for(const lib of videoLibs){
        const cardButton=document.querySelector(`[data-video-metadata="${lib.id}"]`);
        try{
          if(!running.has(Number(lib.id))){
            try{await req(`/admin/libraries/${lib.id}/metadata`,{method:'POST',body:'{}'})}catch(err){if(err.status!==409)throw err}
          }
          await watchMetadataJob(Number(lib.id),cardButton,true);
        }catch(err){errors++;notice(`${lib.name}: ${err.message}`)}
        done++;
        if(button)button.textContent=`Metadados ${Math.round(done*100/Math.max(1,videoLibs.length))}%`;
      }
      notice(`Metadados de vídeo concluídos em ${done} biblioteca(s)${errors?` · ${errors} com erro`:''}.`,errors===0);
    }finally{
      allRunning=false;if(button){button.disabled=false;button.textContent='Metadados de todas'}
    }
  }

  async function watchMetadataJob(libraryID,button,quiet=false){
    libraryID=Number(libraryID);
    if(running.has(libraryID))return running.get(libraryID);
    const task=(async()=>{
      const started=Date.now();
      while(Date.now()-started<40*60*1000){
        const jobs=await req('/admin/metadata/jobs?limit=120');
        const job=(jobs||[]).filter(j=>Number(j.library_id)===libraryID).sort((a,b)=>Number(b.id)-Number(a.id))[0];
        if(!job){await sleep(1200);continue}
        const active=job.status==='queued'||job.status==='running';
        if(button){
          button.disabled=active;
          button.textContent=active?`Metadados ${Number(job.processed||0)}/${Number(job.total||0)}`:'Metadados';
        }
        if(!active){
          if(!quiet)notice(`${job.library}: ${Number(job.matched||0)} correspondências · ${Number(job.failed||0)} falhas.`,job.status==='completed');
          return job;
        }
        await sleep(1700);
      }
      throw new Error('tempo limite excedido acompanhando metadados');
    })();
    running.set(libraryID,task);
    try{return await task}finally{running.delete(libraryID);if(button){button.disabled=false;button.textContent='Metadados'}}
  }

  async function syncMetadataButtons(){
    const jobs=await req('/admin/metadata/jobs?limit=120');
    const latest=new Map();
    for(const job of jobs||[]){const id=Number(job.library_id);if(!latest.has(id)||Number(job.id)>Number(latest.get(id).id))latest.set(id,job)}
    for(const [id,job] of latest){
      if(job.status!=='queued'&&job.status!=='running')continue;
      const button=document.querySelector(`[data-video-metadata="${id}"]`);if(!button)continue;
      button.disabled=true;button.textContent=`Metadados ${Number(job.processed||0)}/${Number(job.total||0)}`;
      watchMetadataJob(id,button,true).catch(()=>{});
    }
  }

  const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
  setTimeout(()=>decorateVideoMetadataControls().catch(()=>{}),150);
})();
