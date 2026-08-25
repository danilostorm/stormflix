/* StormFlix scanner UX: async rclone scans with visible progress */
(function(){
  const baseLoadLibraries=loadLibraries;

  loadLibraries=async function(){
    await baseLoadLibraries();
    const rows=[...document.querySelectorAll('#libraries tbody tr')];
    rows.forEach((row,index)=>{
      const lib=libs[index];
      if(!lib)return;
      const scanCell=row.children[4];
      const pathCell=row.children[1];
      const button=row.querySelector('button[onclick^="scanLib"]');
      const detail=String(lib.last_error||'').trim();

      if(scanCell&&detail){
        const progress=document.createElement('small');
        progress.className=lib.last_scan_status==='running'?'scan-progress':'scan-detail';
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

      if(button&&lib.last_scan_status==='running'){
        button.disabled=true;
        button.textContent='Escaneando…';
      }else if(button&&children.length){
        button.disabled=true;
        button.title='Edite esta biblioteca e selecione a pasta específica. O caminho atual contém outras bibliotecas.';
        button.textContent='Corrigir pasta';
      }
    });
  };

  window.scanLib=async function(id){
    const lib=libs.find(x=>Number(x.id)===Number(id));
    if(lib){
      const children=libs.filter(other=>Number(other.id)!==Number(lib.id)&&isInside(other.path,lib.path));
      if(children.length){
        notice(`A pasta de ${lib.name} contém outras bibliotecas (${children.map(x=>x.name).join(', ')}). Edite e escolha a pasta específica.`);
        return;
      }
    }
    try{
      await req(`/libraries/${id}/scan`,{method:'POST'});
      notice('Scan iniciado em segundo plano.',true);
      await loadLibraries();
      pollScan(id,Date.now());
    }catch(err){
      notice(err.message);
      await loadLibraries().catch(()=>{});
    }
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

  async function pollScan(id,started){
    if(Date.now()-started>21*60*1000)return;
    await new Promise(resolve=>setTimeout(resolve,1800));
    try{
      await loadLibraries();
      const lib=libs.find(x=>Number(x.id)===Number(id));
      if(!lib)return;
      if(lib.last_scan_status==='running'){
        pollScan(id,started);
        return;
      }
      if(lib.last_scan_status==='ok')notice(`${lib.media_count} arquivos catalogados.`,true);
      else notice(lib.last_error||`Scan finalizado: ${lib.last_scan_status}`);
    }catch{
      pollScan(id,started);
    }
  }
})();
