/* StormFlix unified web playback client. */
(function(){
  let planGeneration=0;
  let activePlan=null;

  function canPlay(mediaType){
    try{return Boolean(player.canPlayType(mediaType))}catch{return false}
  }

  function browserCapabilities(){
    const containers=[];
    if(canPlay('video/mp4'))containers.push('mp4');
    if(canPlay('video/webm'))containers.push('webm');

    const videoCodecs=[];
    if(canPlay('video/mp4; codecs="avc1.42E01E"'))videoCodecs.push('h264');
    if(canPlay('video/mp4; codecs="hvc1.1.6.L93.B0"')||canPlay('video/mp4; codecs="hev1.1.6.L93.B0"'))videoCodecs.push('hevc');
    if(canPlay('video/mp4; codecs="av01.0.05M.08"')||canPlay('video/webm; codecs="av01.0.05M.08"'))videoCodecs.push('av1');
    if(canPlay('video/webm; codecs="vp09.00.10.08"'))videoCodecs.push('vp9');

    const audioCodecs=[];
    if(canPlay('audio/mp4; codecs="mp4a.40.2"')||canPlay('audio/aac'))audioCodecs.push('aac');
    if(canPlay('audio/mpeg'))audioCodecs.push('mp3');
    if(canPlay('audio/webm; codecs="opus"'))audioCodecs.push('opus');
    if(canPlay('audio/mp4; codecs="ac-3"'))audioCodecs.push('ac3');
    if(canPlay('audio/mp4; codecs="ec-3"'))audioCodecs.push('eac3');

    return {
      containers:[...new Set(containers)],
      video_codecs:[...new Set(videoCodecs)],
      audio_codecs:[...new Set(audioCodecs)],
      allow_remux:containers.includes('mp4'),
      allow_audio_compatibility:containers.includes('mp4')&&audioCodecs.includes('aac')
    };
  }

  function clientRequest(){
    return {
      client_kind:'web',
      client_name:'StormFlix Web',
      client_version:'0.17',
      capabilities:browserCapabilities()
    };
  }

  function compatibilityMode(mode){
    if(mode==='audio_compatibility')return'direct_stream_audio_aac';
    if(mode==='remux')return'web_remux';
    if(mode==='unsupported')return'unsupported';
    return'direct_play';
  }

  function applyPlanState(plan){
    activePlan=plan||null;
    window.sfLastPlaybackPlan=plan||null;
    window.sfLastCompatibilityPlan=plan||null;
    window.sfPlaybackSessionID=plan?.playback_session_id||'';
    window.sfPlaybackMode=compatibilityMode(plan?.mode);
  }

  function setHelp(message,visible){
    const help=document.querySelector('#player-help');
    if(!help)return;
    if(message)help.textContent=message;
    help.classList.toggle('hidden',!visible);
  }

  async function preparePlan(plan){
    if(!plan?.prepare_url)return null;
    const path=String(plan.prepare_url).replace(/^\/api\/v1/,'');
    const prepared=await request(path,{method:'POST',body:'{}'});
    if(!prepared?.ready)throw new Error('O StormFlix não conseguiu preparar esta fonte.');
    if(!prepared.seekable)throw new Error('A fonte preparada não ficou seekable.');
    if(prepared.audio_transcode){
      plan.mode='audio_compatibility';
      plan.audio_transcode=true;
      plan.audio_codec=prepared.audio_codec||'aac';
      plan.source_audio_codec=prepared.source_audio_codec||plan.source_audio_codec||'';
      applyPlanState(plan);
    }
    return prepared;
  }

  function mediaSession(item){
    if(!('mediaSession' in navigator))return;
    try{
      navigator.mediaSession.metadata=new MediaMetadata({
        title:item?.title||'StormFlix',
        artist:item?.series_title||item?.library_name||'StormFlix',
        artwork:item?.poster_url?[{src:item.poster_url}]:[]
      });
      navigator.mediaSession.setActionHandler('play',()=>player.play().catch(()=>{}));
      navigator.mediaSession.setActionHandler('pause',()=>player.pause());
      navigator.mediaSession.setActionHandler('seekbackward',details=>{player.currentTime=Math.max(0,(player.currentTime||0)-(details.seekOffset||10))});
      navigator.mediaSession.setActionHandler('seekforward',details=>{player.currentTime=Math.min(player.duration||Infinity,(player.currentTime||0)+(details.seekOffset||10))});
      navigator.mediaSession.setActionHandler('seekto',details=>{if(Number.isFinite(details.seekTime))player.currentTime=details.seekTime});
    }catch{}
  }

  function loadSource(url,resume,autoplay,generation){
    if(generation!==planGeneration)return;
    const source=String(url||'');
    if(!source)throw new Error('O plano de reprodução não retornou uma fonte.');
    player.src=source.startsWith('/api/')?source:`${api}${source}`;
    player.load();
    player.addEventListener('loadedmetadata',function restore(){
      if(generation!==planGeneration)return;
      const position=Number(resume||0);
      if(position>=0&&Number.isFinite(player.duration)&&position<player.duration-3)player.currentTime=position;
      if(autoplay)player.play().catch(()=>{});
    },{once:true});
  }

  async function start(item,options={}){
    if(!item?.id)return null;
    const generation=++planGeneration;
    applyPlanState(null);
    setHelp('Analisando a melhor forma de reproduzir neste navegador…',true);

    let plan;
    try{
      plan=await request(`/media/${Number(item.id)}/playback/plan`,{
        method:'POST',
        body:JSON.stringify(clientRequest())
      });
    }catch(err){
      if(generation!==planGeneration)return null;
      // A planner outage must not make an otherwise healthy Direct Play path
      // unusable during migration.
      plan={available:true,mode:'direct_play',url:`/api/v1/media/${Number(item.id)}/stream`,reason:`Planner indisponível: ${err.message||err}`};
    }
    if(generation!==planGeneration)return plan;
    applyPlanState(plan);

    if(!plan?.available){
      player.pause();
      player.removeAttribute('src');
      player.load();
      setHelp(plan?.reason||'Este formato não é suportado pelo navegador sem transcodificar vídeo.',true);
      if(typeof sfToast==='function')sfToast('Formato não suportado neste navegador');
      return plan;
    }

    let prepared=null;
    if(plan.prepare_url){
      setHelp(plan.mode==='audio_compatibility'?'Preparando áudio AAC sem recodificar o vídeo…':'Preparando remux sem recodificar o vídeo…',true);
      prepared=await preparePlan(plan);
      if(generation!==planGeneration)return plan;
    }
    const resume=Number.isFinite(options.resumePosition)?options.resumePosition:Number(plan.resume_position_seconds||item.position_seconds||0);
    const sourceURL=prepared?.url||plan.url;
    setHelp('',false);
    loadSource(sourceURL,resume,options.autoplay!==false,generation);
    mediaSession(item);
    return plan;
  }

  async function playPlanned(item){
    stopTheme();
    if(typeof sfBuildPlayer==='function')sfBuildPlayer();
    if(typeof sfCurrentMedia!=='undefined')sfCurrentMedia={...item};
    const title=document.querySelector('#player-title');if(title)title.textContent=item.title||'StormFlix';
    const modal=document.querySelector('#player-modal');if(modal){modal.classList.remove('hidden');modal.classList.remove('sf-controls-hidden')}
    if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(item.id);
    if(typeof sfShowControls==='function')sfShowControls();
    return start(item,{autoplay:true});
  }

  playMedia=playPlanned;

  if(typeof sfSelectVersion==='function'){
    sfSelectVersion=async function(id){
      if(!id||Number(id)===Number(sfCurrentMedia?.id))return;
      const version=(sfVersions||[]).find(v=>Number(v.id)===Number(id));
      if(!version)return;
      const oldTime=Number.isFinite(player.currentTime)?player.currentTime:0;
      const wasPlaying=!player.paused;
      const next={...sfCurrentMedia,...version,id:Number(id)};
      sfCurrentMedia=next;
      if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(id);
      await start(next,{resumePosition:oldTime,autoplay:wasPlaying});
      if(typeof sfToast==='function')sfToast(`${version.label||'Versão'} · ${compatibilityMode(activePlan?.mode).replaceAll('_',' ')}`);
      if(typeof sfRenderSettings==='function')sfRenderSettings();
    };
  }

  const previousClosePlayer=closePlayer;
  closePlayer=function(){
    // Invalidate any in-flight planning/preparation request before the legacy
    // close routine clears the media element. A late response must never start
    // hidden playback after the user has left the player.
    planGeneration++;
    applyPlanState(null);
    return previousClosePlayer();
  };
  const closeButton=document.querySelector('#player-close');if(closeButton)closeButton.onclick=closePlayer;

  // Disable the old automatic web-only planner. Manual AAC controls remain as
  // an explicit escape hatch while the UI migration is completed.
  window.sfEnsureWebAudioCompatibility=function(){return Promise.resolve(activePlan)};

  window.sfPlaybackCore={
    start,
    capabilities:browserCapabilities,
    currentPlan:()=>activePlan,
    sessionID:()=>String(activePlan?.playback_session_id||'')
  };
})();
