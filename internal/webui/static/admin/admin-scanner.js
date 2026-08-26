/* StormFlix scanner UX: async rclone scans with visible progress */
(function(){
  const baseLoadLibraries=loadLibraries;
  const pollers=new Map();

  loadLibraries=async function(){
    await baseLoadLibraries();
    const rows=[...document.querySelectorAll('#libraries tbody tr')];
    rows.forEach((row,index)=>{
      const lib=libs[index];
      if(!lib)return;
      const scanCell=row.children[4];
      const pathCell=row.children[1];
      const actions=row.children[5];
      const button=row.querySelector('button[onclick^="scanLib"]');
      const detail=String(lib.last_error||'').trim();
      const active=lib.last_scan_status==='running'||lib.last_scan_status==='cancelling';

      if(scanCell&&detail){
        const progress=document.createElement('small');
        progress.className=active?'scan-progress':'scan-detail';
        progress.textContent=detail;
        progress.style.display='block';
        progress.style.maxWidth='360px';
        progress.style.marginTop='4px';
        scanCell.appendChild(progress);
      }

      const children=libs.filter(other=>Number(other.id)!==Number(lib.id)&&isInside(other.path,lib.path));
      const parent=libs.find(other=>Number(other.id)!==Number(lib.id)&&isInside(lib.path,other.path));
      if(pathCell&&(children.length||parent)){
        const warning=document.createElement('small');
        warning.style.display='block';
        warning.style.marginTop='5px';
        warning.style.color='#ff9b69';
        warning.style.fontWeight='700';
        warning.textContent=children.length
          ?`⚠ Pasta contém outra biblioteca: ${children.map(x=>x.name).join(', ')}`
          :`⚠ Dentro da biblioteca: ${parent.name}`;
        pathCell.appendChild(warning);
      }

      if(button&&active){
        button.disabled=true;
        button.textContent=lib.last_scan_status==='cancelling'?'Cancelando…':'Escaneando…';
        if(actions&&!actions.querySelector(`[data-cancel-scan="${lib.id}"]`)){
          const cancel=document.createElement('button');
          cancel.dataset.cancelScan=String(lib.id);
          cancel.className='danger';
          cancel.textContent='Cancelar scan';
          cancel.onclick=()=>cancelScan(lib.id);
          actions.insertBefore(cancel,actions.children[1]||null);
        }
        if(!pollers.has(Number(lib.id)))pollScan(Number(lib.id));
      }else if(button&&children.length){
        button.disabled=true;
        button.title='Edite esta biblioteca e selecione a pasta específica. O caminho atual contém outras bibliotecas.';
        button.textContent='Corrigir pasta';
      }
    });
  };

  window.scanLib=async function(id){
    id=Number(id);
    const lib=libs.find(x=>Number(x.id)===id);
    if(lib){
      const children=libs.filter(other=>Number(other.id)!==id&&isInside(other.path,lib.path));
      if(children.length){
        notice(`A pasta de ${lib.name} contém outras bibliotecas (${children.map(x=>x.name).join(', ')}). Edite e escolha a pasta específica.`);
        return;
      }
    }
    try{
      await req(`/libraries/${id}/scan`,{method:'POST'});
      notice('Scan iniciado em segundo plano.',true);
      await loadLibraries();
      pollScan(id,true);
    }catch(err){
      notice(err.message);
      await loadLibraries().catch(()=>{});
    }
  };

  window.cancelScan=async function(id){
    id=Number(id);
    try{
      await req(`/libraries/${id}/scan/cancel`,{method:'POST'});
      notice('Cancelamento solicitado. O catálogo atual será preservado.',true);
      await loadLibraries();
    }catch(err){notice(err.message)}
  };

  function cleanPath(value){
    let path=String(value||'').replace(/\\/g,'/').replace(/\/+$/,'');
    return path||'/';
  }
  function isInside(path,root){
    path=cleanPath(path);root=cleanPath(root);
    if(path===root)return true;
    return root==='/'?path.startsWith('/'):path.startsWith(root+'/');
  }
  const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));

  async function pollScan(id,replace=false){
    id=Number(id);
    if(pollers.has(id)&&!replace)return;
    const token=Symbol('scan-poll');
    pollers.set(id,token);
    const started=Date.now();
    try{
      while(pollers.get(id)===token&&Date.now()-started<46*60*1000){
        await wait(1800);
        if(pollers.get(id)!==token)return;
        try{await loadLibraries()}catch{continue}
        const lib=libs.find(x=>Number(x.id)===id);
        if(!lib)return;
        if(lib.last_scan_status==='running'||lib.last_scan_status==='cancelling')continue;
        if(lib.last_scan_status==='ok')notice(`${lib.media_count} arquivos catalogados.`,true);
        else notice(lib.last_error||`Scan finalizado: ${lib.last_scan_status}`);
        return;
      }
      if(pollers.get(id)===token)notice('O acompanhamento do scan chegou ao limite de 46 minutos. Atualize Bibliotecas para consultar o estado atual.');
    }finally{
      if(pollers.get(id)===token)pollers.delete(id);
    }
  }
})();
