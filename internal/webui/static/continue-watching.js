/* StormFlix profile progress + server-synced Playback Delight state. */
(function(){
  const baseCardHTML=cardHTML;
  cardHTML=function(item){
    let html=baseCardHTML(item);
    const progress=Math.max(0,Math.min(100,Number(item?.progress_percent||0)));
    if(progress>0&&progress<92){
      html=html.replace('<div class="tile-shade"></div>',`<div class="tile-shade"></div><div class="watch-progress" aria-label="${Math.round(progress)}% assistido"><span style="width:${progress.toFixed(1)}%"></span></div>`);
    }
    return html;
  };

  // Resume itself is owned only by PlaybackPlan/Core. This layer preloads the
  // selected profile's delight state before playMedia reaches the controller.
  let selectedProfile=null,profilePromise=null;
  const serverMarkerCount=new Map();
  const currentMediaID=()=>Number((typeof sfCurrentMedia!=='undefined'?sfCurrentMedia:null)?.id||0);
  const prefKey=id=>`stormflix.playback.profile.${Number(id)||'account'}`;
  const markerKey=id=>`stormflix.playback.markers.${Number(id)||0}`;

  window.addEventListener('stormflix:profile',event=>{if(event.detail?.id)selectedProfile=event.detail});

  async function ensureProfile(){
    if(selectedProfile?.id)return selectedProfile;
    if(profilePromise)return profilePromise;
    profilePromise=(async()=>{
      try{
        const data=await request('/profiles');
        selectedProfile=(data?.profiles||[]).find(p=>Number(p.id)===Number(data?.selected_profile_id))||null;
        if(selectedProfile)window.dispatchEvent(new CustomEvent('stormflix:profile',{detail:selectedProfile}));
      }catch{}
      return selectedProfile;
    })().finally(()=>{profilePromise=null});
    return profilePromise;
  }

  async function delightRPC(mediaID,operation,payload={}){
    if(!mediaID)return null;
    return request(`/media/${mediaID}/playback/telemetry`,{method:'POST',body:JSON.stringify({operation,client_kind:'web',...payload})});
  }

  function readJSON(key,fallback){try{const v=JSON.parse(localStorage.getItem(key)||'null');return v&&typeof v==='object'?v:fallback}catch{return fallback}}
  function writeJSON(key,value){try{localStorage.setItem(key,JSON.stringify(value))}catch{}}
  const validInterval=m=>{const start=Number(m?.start),end=Number(m?.end);return Number.isFinite(start)&&Number.isFinite(end)&&end>start+.25?{start,end}:null};
  const markerList=value=>(Array.isArray(value)?value:[value]).map(validInterval).filter(Boolean).sort((a,b)=>a.start-b.start);

  async function syncPreferences(mediaID,prefs){
    if(!mediaID||!prefs)return;
    try{await delightRPC(mediaID,'playback_preferences_set',{playback_preferences:prefs})}catch(e){console.warn('StormFlix playback preference sync failed',e)}
  }

  async function syncMarkers(mediaID,source='manual'){
    if(!mediaID)return;
    const local=readJSON(markerKey(mediaID),{}),jobs=[];
    const intro=markerList(local.intro)[0];
    if(intro)jobs.push(delightRPC(mediaID,'marker_upsert',{marker:{kind:'intro',start_seconds:intro.start,end_seconds:intro.end,source}}));
    const recap=markerList(local.recap)[0];
    if(recap)jobs.push(delightRPC(mediaID,'marker_upsert',{marker:{kind:'recap',start_seconds:recap.start,end_seconds:recap.end,source}}));
    const credits=markerList(local.credits);
    if(credits.length){
      // Replace the credit set atomically from the client's point of view. Each
      // interval receives a stable index so another device gets the same gaps.
      await delightRPC(mediaID,'marker_delete',{marker:{kind:'credits'}}).catch(()=>{});
      credits.forEach((m,index)=>jobs.push(delightRPC(mediaID,'marker_upsert',{marker:{kind:'credits',segment_index:index,start_seconds:m.start,end_seconds:m.end,source}})));
    }
    try{await Promise.all(jobs)}catch(e){console.warn('StormFlix marker sync failed',e)}
  }

  async function applyServerState(mediaID,state){
    const p=await ensureProfile(),profileID=Number(state?.profile_id||p?.id||0);
    if(profileID&&state?.playback_preferences){
      const key=prefKey(profileID),migration=`stormflix.playback.server-migrated.${profileID}`,raw=localStorage.getItem(key);
      const serverHasPreferences=state?.playback_preferences_persisted===true;
      if(raw&&localStorage.getItem(migration)!=='1'&&!serverHasPreferences){
        const local=readJSON(key,null);
        if(local)await syncPreferences(mediaID,local);
      }else writeJSON(key,state.playback_preferences);
      try{localStorage.setItem(migration,'1')}catch{}
    }

    const serverMarkers=Array.isArray(state?.markers)?state.markers:[];
    serverMarkerCount.set(mediaID,serverMarkers.length);
    if(serverMarkers.length){
      const mapped={};
      serverMarkers.forEach(m=>{
        if(!m?.kind||Number(m.end_seconds)<=Number(m.start_seconds))return;
        const interval={start:Number(m.start_seconds),end:Number(m.end_seconds)};
        if(m.kind==='credits')mapped.credits=[...(mapped.credits||[]),interval];
        else mapped[m.kind]=interval;
      });
      if(mapped.credits)mapped.credits.sort((a,b)=>a.start-b.start);
      writeJSON(markerKey(mediaID),mapped);
    }else{
      const local=readJSON(markerKey(mediaID),{});
      if(Object.keys(local).length)await syncMarkers(mediaID,'manual');
    }
    if(p)window.dispatchEvent(new CustomEvent('stormflix:profile',{detail:p}));
  }

  async function preloadDelight(mediaID){
    await ensureProfile();
    if(!mediaID)return;
    try{const state=await delightRPC(mediaID,'playback_state_get');if(state)await applyServerState(mediaID,state)}catch(e){console.warn('StormFlix playback delight preload failed',e)}
  }

  // autoplay-next.js wraps the authoritative Playback Core first. Wrapping it
  // here means server state is loaded before that controller computes rewind
  // and before it reads the title's skip markers.
  const basePlayMedia=playMedia;
  playMedia=async function(item){
    if(item?.id)await preloadDelight(Number(item.id));
    return basePlayMedia(item);
  };

  // Persist profile-wide controls after the Playback Delight controller has
  // updated localStorage. Event bubbling guarantees its onchange runs first.
  document.addEventListener('change',event=>{
    const control=event.target.closest?.('#sf-playback-delight-settings [data-d]');
    if(!control||control.dataset.d==='autoplay')return;
    setTimeout(async()=>{
      const id=currentMediaID(),p=await ensureProfile();if(!id||!p?.id)return;
      await syncPreferences(id,readJSON(prefKey(p.id),null));
    },0);
  });

  // Manual corrections are server authoritative. A credit correction is only
  // synced after its explicit end is marked, so a half-created interval never
  // becomes a skip range on another device.
  document.addEventListener('click',event=>{
    const button=event.target.closest?.('#sf-playback-delight-settings [data-m]');
    if(!button)return;
    setTimeout(async()=>{
      const id=currentMediaID();if(!id)return;
      if(button.dataset.m==='clear'){
        await Promise.all(['intro','credits','recap'].map(kind=>delightRPC(id,'marker_delete',{marker:{kind}}).catch(()=>{})));
        serverMarkerCount.set(id,0);return;
      }
      if(button.dataset.m==='end'||button.dataset.m==='credits-end')await syncMarkers(id,'manual');
    },0);
  });

  // If the browser exposes useful chapter cues and no server marker exists,
  // autoplay-next.js imports them shortly after metadata. Promote them once so
  // another device can reuse them too; manual corrections always win later.
  const player=document.querySelector('#player');
  player?.addEventListener('loadedmetadata',()=>{
    const id=currentMediaID();if(!id||serverMarkerCount.get(id)>0)return;
    setTimeout(()=>syncMarkers(id,'chapter'),900);
  },{passive:true});

  ensureProfile();
})();