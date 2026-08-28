/* StormFlix unified web playback client. */
(function(){
  let planGeneration=0;
  let activePlan=null;
  let activeItem=null;
  let activeHls=null;
  let hlsLibraryPromise=null;
  let mediaSessionBound=false;
  let playerErrorBound=false;

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
    if(canPlay('audio/mp4; codecs="dts-"')||canPlay('audio/mp4; codecs="dts+"'))audioCodecs.push('dts');
    if(canPlay('audio/flac')||canPlay('audio/mp4; codecs="fLaC"'))audioCodecs.push('flac');

    return {
      containers:[...new Set(containers)],
      video_codecs:[...new Set(videoCodecs)],
      audio_codecs:[...new Set(audioCodecs)],
      subtitle_formats:['vtt'],
      allow_remux:containers.includes('mp4'),
      allow_audio_compatibility:containers.includes('mp4')&&audioCodecs.includes('aac'),
      native_audio_track_selection:false,
      // HTMLMediaElement multi-audio selection is not consistently exposed
      // across browsers. Ask the server to pin a non-default preferred track.
      server_selects_audio:true,
      picture_in_picture:Boolean(document.pictureInPictureEnabled&&player.requestPictureInPicture),
      media_session:'mediaSession' in navigator
    };
  }

  function clientRequest(sessionID){
    return {
      client_kind:'web',
      client_name:'StormFlix Web',
      client_version:'0.19',
      playback_session_id:String(sessionID||''),
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
    }
    if(Number.isInteger(prepared.audio_stream))plan.audio_stream=prepared.audio_stream;
    if(prepared.audio_language)plan.audio_language=prepared.audio_language;
    applyPlanState(plan);
    return prepared;
  }

  function updateMediaSessionPosition(){
    if(!('mediaSession' in navigator)||typeof navigator.mediaSession.setPositionState!=='function')return;
    const duration=Number(player.duration),position=Number(player.currentTime),rate=Number(player.playbackRate||1);
    if(!Number.isFinite(duration)||duration<=0||!Number.isFinite(position))return;
    try{navigator.mediaSession.setPositionState({duration,position:Math.max(0,Math.min(position,duration)),playbackRate:rate})}catch{}
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
      if(!mediaSessionBound){
        mediaSessionBound=true;
        player.addEventListener('timeupdate',updateMediaSessionPosition,{passive:true});
        player.addEventListener('durationchange',updateMediaSessionPosition,{passive:true});
        player.addEventListener('ratechange',updateMediaSessionPosition,{passive:true});
      }
      updateMediaSessionPosition();
    }catch{}
  }

  async function togglePictureInPicture(){
    if(!document.pictureInPictureEnabled||!player.requestPictureInPicture)return false;
    try{
      if(document.pictureInPictureElement){await document.exitPictureInPicture();return false}
      await player.requestPictureInPicture();return true;
    }catch{return false}
  }

  function ensurePiPControl(){
    if(!document.pictureInPictureEnabled||!player.requestPictureInPicture||document.querySelector('#sf-pip'))return;
    const fullscreen=document.querySelector('#sf-fullscreen');
    if(!fullscreen?.parentElement)return;
    const button=document.createElement('button');
    button.className='sf-control-btn';button.id='sf-pip';button.type='button';button.setAttribute('aria-label','Picture-in-Picture');button.textContent='▣';
    button.onclick=()=>togglePictureInPicture();
    fullscreen.parentElement.insertBefore(button,fullscreen);
  }

  function bindPlayerErrors(){
    if(playerErrorBound)return;playerErrorBound=true;
    player.addEventListener('error',()=>{
      if(!activeItem)return;
      const detail=player.error?.message||activePlan?.reason||'O navegador não conseguiu decodificar a fonte planejada.';
      setHelp(detail,true);
      if(typeof sfToast==='function')sfToast('Falha ao reproduzir esta fonte');
    });
  }

  function absoluteSourceURL(source){
    source=String(source||'');
    if(!source)return'';
    if(/^https?:\/\//i.test(source))return source;
    return source.startsWith('/api/')?source:`${api}${source}`;
  }

  function isHLSSource(source){
    const value=String(source||'').toLowerCase();
    return value.includes('.m3u8')||value.includes('/hls/');
  }

  function destroyHls(){
    if(activeHls){
      try{activeHls.destroy()}catch{}
      activeHls=null;
    }
  }

  function ensureHlsLibrary(){
    if(window.Hls)return Promise.resolve(window.Hls);
    if(hlsLibraryPromise)return hlsLibraryPromise;
    hlsLibraryPromise=new Promise((resolve,reject)=>{
      const existing=document.querySelector('script[data-stormflix-hls]');
      if(existing){
        existing.addEventListener('load',()=>window.Hls?resolve(window.Hls):reject(new Error('hls.js não inicializou.')),{once:true});
        existing.addEventListener('error',()=>reject(new Error('Não foi possível carregar hls.js.')),{once:true});
        return;
      }
      const script=document.createElement('script');
      script.src='https://cdn.jsdelivr.net/npm/hls.js@1.7.1/dist/hls.min.js';
      script.async=true;
      script.dataset.stormflixHls='1';
      script.onload=()=>window.Hls?resolve(window.Hls):reject(new Error('hls.js não inicializou.'));
      script.onerror=()=>reject(new Error('Não foi possível carregar hls.js.'));
      document.head.appendChild(script);
    }).catch(err=>{hlsLibraryPromise=null;throw err});
    return hlsLibraryPromise;
  }

  function restoreOnMetadata(resume,autoplay,generation){
    player.addEventListener('loadedmetadata',function restore(){
      if(generation!==planGeneration)return;
      const position=Number(resume||0);
      if(position>=0&&Number.isFinite(player.duration)&&position<player.duration-3)player.currentTime=position;
      if(autoplay)player.play().catch(()=>{});
      updateMediaSessionPosition();
    },{once:true});
  }

  async function loadHlsSource(url,resume,autoplay,generation){
    destroyHls();
    restoreOnMetadata(resume,autoplay,generation);
    const nativeHls=canPlay('application/vnd.apple.mpegurl')||canPlay('application/x-mpegURL');
    let HlsCtor=null;
    if('MediaSource'in window){
      try{HlsCtor=await ensureHlsLibrary()}catch(err){if(!nativeHls)throw err}
    }
    if(generation!==planGeneration)return;

    if(HlsCtor&&typeof HlsCtor.isSupported==='function'&&HlsCtor.isSupported()){
      const hls=new HlsCtor({
        enableWorker:true,
        maxBufferLength:24,
        maxMaxBufferLength:36,
        backBufferLength:8,
        maxBufferHole:0.5,
        lowLatencyMode:false
      });
      activeHls=hls;
      hls.on(HlsCtor.Events.ERROR,(_event,data)=>{
        if(!data?.fatal||activeHls!==hls)return;
        if(data.type===HlsCtor.ErrorTypes.NETWORK_ERROR){
          try{hls.startLoad()}catch{}
          return;
        }
        if(data.type===HlsCtor.ErrorTypes.MEDIA_ERROR){
          try{hls.recoverMediaError()}catch{}
          return;
        }
        setHelp(data.details||'Falha fatal no streaming HLS.',true);
        if(typeof sfToast==='function')sfToast('Falha no streaming HLS');
        try{hls.destroy()}catch{}
        if(activeHls===hls)activeHls=null;
      });
      await new Promise((resolve,reject)=>{
        let settled=false;
        const fail=(_event,data)=>{
          if(settled||!data?.fatal)return;
          settled=true;reject(new Error(data.details||'Falha ao abrir o HLS.'));
        };
        hls.on(HlsCtor.Events.ERROR,fail);
        hls.on(HlsCtor.Events.MEDIA_ATTACHED,()=>{
          if(generation!==planGeneration){if(!settled){settled=true;resolve()}return}
          hls.loadSource(url);
        });
        hls.on(HlsCtor.Events.MANIFEST_PARSED,()=>{if(!settled){settled=true;resolve()}});
        hls.attachMedia(player);
      });
      return;
    }

    if(nativeHls){
      player.src=url;
      player.load();
      return;
    }
    throw new Error('Este navegador não possui suporte MSE/HLS para o modo de compatibilidade.');
  }

  async function loadSource(url,resume,autoplay,generation){
    if(generation!==planGeneration)return;
    const source=absoluteSourceURL(url);
    if(!source)throw new Error('O plano de reprodução não retornou uma fonte.');
    if(isHLSSource(source)){
      await loadHlsSource(source,resume,autoplay,generation);
      return;
    }
    destroyHls();
    restoreOnMetadata(resume,autoplay,generation);
    player.src=source;
    player.load();
  }

  async function start(item,options={}){
    if(!item?.id)return null;
    const generation=++planGeneration;
    const previousSession=options.sessionID||activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    activeItem=item;
    applyPlanState(null);
    setHelp('Analisando a melhor forma de reproduzir neste navegador…',true);

    let plan;
    try{
      plan=await request(`/media/${Number(item.id)}/playback/plan`,{
        method:'POST',
        body:JSON.stringify(clientRequest(previousSession))
      });
    }catch(err){
      if(generation!==planGeneration)return null;
      destroyHls();
      player.pause();player.removeAttribute('src');player.load();
      setHelp(`Não foi possível obter o plano de reprodução: ${err.message||err}`,true);
      if(typeof sfToast==='function')sfToast('Não foi possível planejar a reprodução');
      return null;
    }
    if(generation!==planGeneration)return plan;
    applyPlanState(plan);

    if(!plan?.available){
      destroyHls();
      player.pause();
      player.removeAttribute('src');
      player.load();
      setHelp(plan?.reason||'Este formato não é suportado pelo navegador sem transcodificar vídeo.',true);
      if(typeof sfToast==='function')sfToast('Formato não suportado neste navegador');
      return plan;
    }

    let prepared=null;
    try{
      if(plan.prepare_url){
        setHelp(plan.mode==='audio_compatibility'?'Preparando áudio AAC sem recodificar o vídeo…':'Preparando remux sem recodificar o vídeo…',true);
        prepared=await preparePlan(plan);
        if(generation!==planGeneration)return plan;
      }
    }catch(err){
      if(generation!==planGeneration)return plan;
      setHelp(`Falha ao preparar a fonte: ${err.message||err}`,true);
      if(typeof sfToast==='function')sfToast('Falha ao preparar a fonte');
      return plan;
    }
    const resume=Number.isFinite(options.resumePosition)?options.resumePosition:Number(plan.resume_position_seconds||item.position_seconds||0);
    const sourceURL=prepared?.url||plan.url;
    setHelp(isHLSSource(sourceURL)?'Iniciando streaming sob demanda…':'',isHLSSource(sourceURL));
    try{
      await loadSource(sourceURL,resume,options.autoplay!==false,generation);
    }catch(err){
      if(generation!==planGeneration)return plan;
      setHelp(`Falha ao iniciar a fonte: ${err.message||err}`,true);
      if(typeof sfToast==='function')sfToast('Falha ao iniciar reprodução');
      return plan;
    }
    if(generation!==planGeneration)return plan;
    setHelp('',false);
    mediaSession(item);
    ensurePiPControl();
    bindPlayerErrors();
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
      const session=activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
      const next={...sfCurrentMedia,...version,id:Number(id)};
      sfCurrentMedia=next;
      if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(id);
      await start(next,{resumePosition:oldTime,autoplay:wasPlaying,sessionID:session});
      if(typeof sfToast==='function')sfToast(`${version.label||'Versão'} · ${compatibilityMode(activePlan?.mode).replaceAll('_',' ')}`);
      if(typeof sfRenderSettings==='function')sfRenderSettings();
    };
  }

  const previousClosePlayer=closePlayer;
  closePlayer=function(){
    planGeneration++;
    activeItem=null;
    destroyHls();
    applyPlanState(null);
    if(document.pictureInPictureElement)document.exitPictureInPicture().catch(()=>{});
    return previousClosePlayer();
  };
  const closeButton=document.querySelector('#player-close');if(closeButton)closeButton.onclick=closePlayer;

  // The legacy automatic web-only planner is retired. Manual compatibility UI
  // remains a user action, but automatic source policy belongs to Playback Core.
  window.sfEnsureWebAudioCompatibility=function(){return Promise.resolve(activePlan)};
  window.sfTogglePictureInPicture=togglePictureInPicture;

  window.sfPlaybackCore={
    start,
    capabilities:browserCapabilities,
    currentPlan:()=>activePlan,
    sessionID:()=>String(activePlan?.playback_session_id||''),
    togglePictureInPicture
  };
})();
