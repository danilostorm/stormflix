/* StormFlix logical source selector: one title, many physical servers. */
(function(){
  let detailVersions=[];
  let selectedVersionID=0;

  const baseOpenDetail=openDetail;
  openDetail=async function(id){
    await baseOpenDetail(id);
    if(!currentDetail)return;
    await renderDetailSources(currentDetail).catch(()=>{});
  };

  async function renderDetailSources(detail){
    detailVersions=await request(`/media/${detail.id}/versions`).catch(()=>[]);
    selectedVersionID=Number(detail.id);
    let root=document.querySelector('#detail-sources');
    if(root)root.remove();
    if(!detailVersions.length)return;

    const servers=[];
    const seen=new Set();
    for(const version of detailVersions){
      const key=version.server_label||`Servidor ${version.source_index||1}`;
      if(seen.has(key))continue;
      seen.add(key);
      servers.push({key,version});
    }
    if(servers.length<=1)return;

    const current=detailVersions.find(v=>Number(v.id)===Number(detail.id));
    if(current){
      const currentKey=current.server_label||`Servidor ${current.source_index||1}`;
      const server=servers.find(s=>s.key===currentKey);
      if(server)selectedVersionID=Number(server.version.id);
    }else{
      selectedVersionID=Number(servers[0].version.id);
    }

    root=document.createElement('div');
    root.id='detail-sources';
    root.className='detail-sources';
    root.innerHTML=`<div class="detail-source-label"><span>Fonte de reprodução</span><small>Escolha o servidor. O título e os metadados são únicos.</small></div><div class="detail-source-buttons">${servers.map(({key,version})=>`<button type="button" class="detail-source-btn ${Number(version.id)===selectedVersionID?'active':''}" data-source-media="${version.id}"><b>${escapeHTML(key)}</b><small>${escapeHTML(version.label||'Original')} · ${escapeHTML(String(version.extension||'').replace('.','').toUpperCase())}</small></button>`).join('')}</div>`;
    document.querySelector('.detail-actions')?.after(root);
    bindSourceButtons(detail,root);
    applySelectedSource(detail);
  }

  function bindSourceButtons(detail,root){
    root.querySelectorAll('[data-source-media]').forEach(button=>button.onclick=()=>{
      selectedVersionID=Number(button.dataset.sourceMedia);
      root.querySelectorAll('[data-source-media]').forEach(x=>x.classList.toggle('active',x===button));
      applySelectedSource(detail);
    });
  }

  function applySelectedSource(detail){
    const version=detailVersions.find(v=>Number(v.id)===selectedVersionID)||detailVersions[0];
    if(!version)return;
    const play=document.querySelector('#detail-play');
    if(play){
      const server=version.server_label||`Servidor ${version.source_index||1}`;
      play.textContent=`▶ Assistir · ${server}`;
      play.onclick=()=>playMedia({...detail,id:Number(version.id),extension:version.extension,size_bytes:version.size_bytes});
    }
    const format=document.querySelector('#detail-format');
    if(format)format.textContent=`${String(version.extension||detail.extension||'').replace('.','').toUpperCase()} · ${version.label||'Original'} · DIRECT PLAY`;
  }

  if(typeof sfRenderSettings==='function'){
    const baseRenderSettings=sfRenderSettings;
    sfRenderSettings=function(){
      baseRenderSettings();
      const panel=document.querySelector('#sf-player-settings-panel');
      const first=panel?.querySelector('.sf-setting-section');
      if(!first)return;
      const heading=first.querySelector('h3');
      if(heading)heading.textContent='Servidor / qualidade · Direct Play';
      first.querySelectorAll('[data-sf-version]').forEach(button=>{
        const version=sfVersions.find(v=>Number(v.id)===Number(button.dataset.sfVersion));
        const span=button.querySelector('span');
        if(version&&span){
          const server=version.server_label||`Servidor ${version.source_index||1}`;
          span.textContent=`${server} · ${version.label||'Original'} · ${String(version.extension||'').replace('.','').toUpperCase()}`;
        }
      });
    };
  }
})();
