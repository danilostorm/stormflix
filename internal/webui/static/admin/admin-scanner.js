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
      if(!scanCell)return;
      const detail=String(lib.last_error||'').trim();
      if(detail){
        const progress=document.createElement('small');
        progress.className=lib.last_scan_status==='running'?'scan-progress':'scan-detail';
        progress.textContent=detail;
        progress.style.display='block';
        progress.style.maxWidth='360px';
        progress.style.marginTop='4px';
        scanCell.appendChild(progress);
      }
      const button=row.querySelector('button[onclick^="scanLib"]');
      if(button&&lib.last_scan_status==='running'){
        button.disabled=true;
        button.textContent='Escaneando…';
      }
    });
  };

  window.scanLib=async function(id){
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
