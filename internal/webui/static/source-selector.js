/* StormFlix logical source selector: one title, many physical servers. */
(function(){
  let detailVersions=[];
  let selectedVersionID=0;
  let anywhereAnchor=null;

  const baseOpenDetail=openDetail;
  openDetail=async function(id){
    await baseOpenDetail(id);
    if(!currentDetail)return;
    window.sfSelectedDetailMedia={...currentDetail,_logical_id:Number(currentDetail.id),id:Number(currentDetail.id)};
    await renderDetailSources(currentDetail).catch(()=>{});
  };

  async function renderDetailSources(detail){
    detailVersions=await request(`/media/${detail.id}/versions`).catch(()=>[]);
    selectedVersionID=Number(detail.id);
    let root=document.querySelector('#detail-sources');
    if(root)root.remove();
    if(!detailVersions.length){window.sfSelectedDetailMedia={...detail,_logical_id:Number(detail.id),id:Number(detail.id)};return}

    const servers=[];
    const seen=new Set();
    for(const version of detailVersions){
      const key=version.server_label||`Servidor ${version.source_index||1}`;
      if(seen.has(key))continue;
      seen.add(key);
      servers.push({key,version});
    }

    const current=detailVersions.find(v=>Number(v.id)===Number(detail.id));
    if(current){
      const currentKey=current.server_label||`Servidor ${current.source_index||1}`;
      const server=servers.find(s=>s.key===currentKey);
      if(server)selectedVersionID=Number(server.version.id);
    }else if(servers.length){
      selectedVersionID=Number(servers[0].version.id);
    }

    if(servers.length<=1){applySelectedSource(detail);return}

    root=document.createElement('div');
    root.id='detail-sources';
    root.className='detail-sources';
    root.innerHTML=`<div class="detail-source-label"><span>Fonte de reprodução</span></div><div class="detail-source-buttons">${servers.map(({key,version})=>`<button type="button" class="detail-source-btn ${Number(version.id)===selectedVersionID?'active':''}" data-source-media="${version.id}"><b>${escapeHTML(key)}</b><small>${escapeHTML(version.label||'Original')} · ${escapeHTML(String(version.extension||'').replace('.','').toUpperCase())}</small></button>`).join('')}</div>`;
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
    if(!version){window.sfSelectedDetailMedia={...detail,_logical_id:Number(detail.id),id:Number(detail.id)};return}
    const selected={...detail,_logical_id:Number(detail.id),id:Number(version.id),extension:version.extension,size_bytes:version.size_bytes,server_label:version.server_label,source_index:version.source_index};
    window.sfSelectedDetailMedia=selected;
    const play=document.querySelector('#detail-play');
    if(play){
      const server=version.server_label||`Servidor ${version.source_index||1}`;
      play.textContent=`▶ Assistir · ${server}`;
      play.onclick=()=>playMedia(selected);
    }
    const format=document.querySelector('#detail-format');
    if(format)format.textContent=`${String(version.extension||detail.extension||'').replace('.','').toUpperCase()} · ${version.label||'Original'} · DIRECT PLAY`;
  }

  function clearAnywhereInlinePosition(panel){
    for(const property of ['left','right','top','bottom'])panel.style.removeProperty(property);
  }

  function positionAnywhere(trigger=anywhereAnchor){
    const panel=document.querySelector('#sf-anywhere');
    if(!panel||panel.classList.contains('hidden')||!trigger?.isConnected)return;
    if(window.innerWidth<=700){clearAnywhereInlinePosition(panel);return}
    const rect=trigger.getBoundingClientRect();
    const box=panel.getBoundingClientRect();
    const margin=16,gap=12;
    let left;
    if(rect.right+gap+box.width<=window.innerWidth-margin)left=rect.right+gap;
    else if(rect.left-box.width-gap>=margin)left=rect.left-box.width-gap;
    else left=Math.min(Math.max(margin,rect.left),Math.max(margin,window.innerWidth-box.width-margin));
    const centered=rect.top+(rect.height/2)-(box.height/2);
    const top=Math.min(Math.max(70,centered),Math.max(70,window.innerHeight-box.height-margin));
    panel.style.left=`${Math.round(left)}px`;
    panel.style.right='auto';
    panel.style.top=`${Math.round(top)}px`;
    panel.style.bottom='auto';
  }

  document.addEventListener('click',event=>{
    const trigger=event.target.closest?.('#detail-anywhere,#sf-anywhere-toggle');
    if(!trigger)return;
    anywhereAnchor=trigger;
    requestAnimationFrame(()=>requestAnimationFrame(()=>positionAnywhere(trigger)));
  });
  document.addEventListener('click',event=>{
    if(event.target.closest?.('[data-any-close]'))anywhereAnchor=null;
  });
  window.addEventListener('resize',()=>positionAnywhere(),{passive:true});
  window.addEventListener('scroll',()=>positionAnywhere(),{passive:true});

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
