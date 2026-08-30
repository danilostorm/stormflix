/* StormFlix unified web playback client — PlaybackPlan v5.1. */
(function(){
  let planGeneration=0;
  let activePlan=null;
  let activeItem=null;
  let activeHls=null;
  let hlsLibraryPromise=null;
  let mediaSessionBound=false;
  let playerErrorBound=false;
  let directStreamFallbackInProgress=false;
  let preferredQuality=normalizeQuality(localStorage.getItem('stormflix.player.quality')||'auto');

  function canPlay(mediaType){
    try{return Boolean(player.canPlayType(mediaType))}catch{return false}
  }

  function normalizeQuality(value){
    value=String(value||'').trim().toLowerCase();
    if(['auto','original','2160p','1440p','1080p','720p','480p'].includes(value))return value;
    if(value==='4k'||value==='uhd')return'2160p';
    return'auto';
  }

  function qualityHeight(value){
    value=normalizeQuality(value);
    if(value==='auto'||value==='original')return 0;
    const height=Number.parseInt(value,10);
    return Number.isFinite(height)?height:0;
  }

  function fallbackQualities(plan){
    const height=Number(plan?.video_height||player.videoHeight||0);
    const values=['auto','original'];
    for(const [minimum,value] of [[2160,'2160p'],[1440,'1440p'],[1080,'1080p'],[720,'720p'],[480,'480p']]){
      if(height>=minimum)values.push(value);
    }
    return values;
  }

  function availableQualities(plan=activePlan){
    const allowed=new Set(['auto','original','2160p','1440p','1080p','720p','480p']);
    const supplied=Array.isArray(plan?.available_qualities)?plan.available_qualities.map(normalizeQuality).filter(v=>allowed.has(v)):[];
    const unique=[...new Set(supplied)];
    if(unique.includes('auto')&&unique.includes('original'))return unique;
    return fallbackQualities(plan);
  }

  function effectiveQuality(plan=activePlan,preferred=preferredQuality){
    const quality=normalizeQuality(preferred);
    if(quality==='auto'||quality==='original'||!plan)return quality;
    const values=availableQualities(plan);
    if(values.includes(quality))return quality;
    const sourceHeight=Number(plan?.video_height||0);
    const requestedHeight=qualityHeight(quality);
    if(requestedHeight>sourceHeight&&sourceHeight>0){
      const exact=`${sourceHeight}p`;
      if(values.includes(exact))return exact;
      return'original';
    }
    return values.includes('original')?'original':'auto';
  }

  function estimatedTranscodeBitrate(){
    const downlink=Number(navigator.connection?.downlink||0);
    if(!Number.isFinite(downlink)||downlink<=0)return 20000;
    return Math.max(2500,Math.min(30000,Math.round(downlink*1000*0.8)));
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
    // PlaybackPlan always needs at least one universal transcode target, but
    // this fallback is never preferred over a compatible original video path.
    if(containers.includes('mp4')&&!videoCodecs.includes('h264'))videoCodecs.push('h264');

    const audioCodecs=[];
    if(canPlay('audio/mp4; codecs="mp4a.40.2"')||canPlay('audio/aac'))audioCodecs.push('aac');
    if(canPlay('audio/mpeg'))audioCodecs.push('mp3');
    if(canPlay('audio/webm; codecs="opus"'))audioCodecs.push('opus');
    if(canPlay('audio/mp4; codecs="ac-3"'))audioCodecs.push('ac3');
    if(canPlay('audio/mp4; codecs="ec-3"'))audioCodecs.push('eac3');
    if(canPlay('audio/mp4; codecs="dts-"')||canPlay('audio/mp4; codecs="dts+"'))audioCodecs.push('dts');
    if(canPlay('audio/flac')||canPlay('audio/mp4; codecs="fLaC"'))audioCodecs.push('flac');
    if(containers.includes('mp4')&&!audioCodecs.includes('aac'))audioCodecs.push('aac');

    return {
      containers:[...new Set(containers)],
      video_codecs:[...new Set(videoCodecs)],
      audio_codecs:[...new Set(audioCodecs)],
      subtitle_formats:['vtt'],
      allow_remux:containers.includes('mp4'),
      allow_audio_compatibility:containers.includes('mp4')&&audioCodecs.includes('aac'),
      allow_video_transcode:containers.includes('mp4')&&videoCodecs.includes('h264'),
      max_transcode_bitrate_kbps:estimatedTranscodeBitrate(),
      native_audio_track_selection:false,
      server_selects_audio:true,
      picture_in_picture:Boolean(document.pictureInPictureEnabled&&player.requestPictureInPicture),
      media_session:'mediaSession' in navigator
    };
  }

  function clientRequest(sessionID,quality){
    return {
      client_kind:'web',
      client_name:'StormFlix Web',
      client_version:'0.5.1',
      playback_session_id:String(sessionID||''),
      quality:normalizeQuality(quality||preferredQuality),
      capabilities:browserCapabilities()
    };
  }

  function compatibilityMode(mode){
    if(mode==='video_transcode')return'video_transcode';
    if(mode==='audio_compatibility')return'direct_stream_audio_aac';
    if(mode==='remux')return'direct_stream_remux';
    if(mode==='unsupported')return'unsupported';
    return'direct_play';
  }

  function isDirectStreamPlan(plan){
    return plan?.mode==='remux'||plan?.mode==='audio_compatibility';
  }

  function applyPlanState(plan){
    activePlan=plan||null;
    window.sfLastPlaybackPlan=plan||null;
    window.sfLastCompatibilityPlan=plan||null;
    window.sfPlaybackSessionID=plan?.playback_session_id||'';
    window.sfPlaybackMode=compatibilityMode(plan?.mode);
    window.dispatchEvent(new CustomEvent('stormflix:playback-plan',{detail:plan||null}));
  }

  function setHelp(message,visible){
    const help=document.querySelector('#player-help');
    if(!help)return;
    if(message)help.textContent=message;
    help.classList.toggle('hidden',!visible);
  }

  function applyPreparedState(plan,prepared){
    if(prepared?.audio_transcode){
      plan.mode='audio_compatibility';
      plan.audio_transcode=true;
      plan.audio_codec=prepared.audio_codec||'aac';
      plan.source_audio_codec=prepared.source_audio_codec||plan.source_audio_codec||'';
    }
    if(Number.isInteger(prepared?.audio_stream))plan.audio_stream=prepared.audio_stream;
    if(prepared?.audio_language)plan.audio_language=prepared.audio_language;
    applyPlanState(plan);
  }

  async function prepareURL(plan,prepareURL){
    if(!prepareURL)return null;
    const path=String(prepareURL).replace(/^\/api\/v1/,'');
    const prepared=await request(path,{method:'POST',body:'{}'});
    if(!prepared?.ready)throw new Error('O StormFlix não conseguiu preparar esta fonte.');
    if(!prepared.seekable)throw new Error('A fonte preparada não ficou seekable.');
    applyPreparedState(plan,prepared);
    return prepared;
  }

  async function preparePlan(plan){
    return prepareURL(plan,plan?.prepare_url);
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
    player.addEventListener('error',async()=>{
      if(!activeItem)return;
      if(isDirectStreamPlan(activePlan)&&activePlan?.transport==='hls'&&!directStreamFallbackInProgress){
        const generation=planGeneration;
        const position=Number.isFinite(player.currentTime)?player.currentTime:0;
        const autoplay=!player.paused;
        try{
          if(await loadDirectStreamFallback(activePlan,position,autoplay,generation)){
            setHelp('',false);
            return;
          }
        }catch(err){
          if(generation!==planGeneration)return;
          setHelp(`Falha no Direct Stream: ${err.message||err}`,true);
        }
      }
      const detail=player.error?.message||activePlan?.reason||'O dispositivo não conseguiu decodificar a fonte planejada.';
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

  function waitForPlayable(generation,timeoutMs=12000){
    if(generation!==planGeneration)return Promise.resolve();
    if(player.readyState>=HTMLMediaElement.HAVE_CURRENT_DATA)return Promise.resolve();
    return new Promise((resolve,reject)=>{
      let settled=false;
      const events=['loadeddata','canplay','playing'];
      const cleanup=()=>{
        clearTimeout(timer);
        events.forEach(name=>player.removeEventListener(name,ready));
        player.removeEventListener('error',failed);
      };
      const finish=(fn,value)=>{if(settled)return;settled=true;cleanup();fn(value)};
      const ready=()=>{if(generation===planGeneration)finish(resolve);else finish(resolve)};
      const failed=()=>finish(reject,new Error(player.error?.message||'O navegador recusou a fonte de vídeo.'));
      const timer=setTimeout(()=>finish(reject,new Error('o streaming não entregou o primeiro quadro dentro do tempo esperado')),timeoutMs);
      events.forEach(name=>player.addEventListener(name,ready,{once:true}));
      player.addEventListener('error',failed,{once:true});
    });
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
      const hls=new HlsCtor({enableWorker:true,maxBufferLength:28,maxMaxBufferLength:44,backBufferLength:10,maxBufferHole:0.5,lowLatencyMode:false});
      activeHls=hls;
      let manifestReady=false,networkRecoveries=0,mediaRecoveries=0;
      hls.on(HlsCtor.Events.ERROR,(_event,data)=>{
        if(!data?.fatal||activeHls!==hls)return;
        // Before the manifest is ready the startup promise below rejects and
        // the normal Direct Stream fallback path takes over.
        if(!manifestReady)return;
        if(data.type===HlsCtor.ErrorTypes.NETWORK_ERROR&&networkRecoveries++<1){try{hls.startLoad()}catch{};return}
        if(data.type===HlsCtor.ErrorTypes.MEDIA_ERROR&&mediaRecoveries++<1){try{hls.recoverMediaError()}catch{};return}
        setHelp(data.details||'Falha fatal no streaming HLS.',true);
        window.dispatchEvent(new CustomEvent('stormflix:hls-fatal',{detail:{message:data.details||'Falha fatal no HLS.',generation}}));
      });
      await new Promise((resolve,reject)=>{
        let settled=false;
        const fail=(_event,data)=>{if(settled||!data?.fatal)return;settled=true;reject(new Error(data.details||'Falha ao abrir o HLS.'))};
        hls.on(HlsCtor.Events.ERROR,fail);
        hls.on(HlsCtor.Events.MEDIA_ATTACHED,()=>{if(generation!==planGeneration){if(!settled){settled=true;resolve()}return}hls.loadSource(url)});
        hls.on(HlsCtor.Events.MANIFEST_PARSED,()=>{manifestReady=true;if(!settled){settled=true;resolve()}});
        hls.attachMedia(player);
      });
      return;
    }

    if(nativeHls){player.src=url;player.load();return}
    throw new Error('Este navegador não possui suporte MSE/HLS para o modo Direct Stream.');
  }

  async function loadSource(url,resume,autoplay,generation){
    if(generation!==planGeneration)return;
    const source=absoluteSourceURL(url);
    if(!source)throw new Error('O plano de reprodução não retornou uma fonte.');
    if(isHLSSource(source)){await loadHlsSource(source,resume,autoplay,generation);return}
    destroyHls();
    restoreOnMetadata(resume,autoplay,generation);
    player.src=source;
    player.load();
  }

  async function loadDirectStreamFallback(plan,resume,autoplay,generation){
    if(generation!==planGeneration||!isDirectStreamPlan(plan)||directStreamFallbackInProgress)return false;
    const prepare=String(plan?.fallback_prepare_url||'');
    const fallback=String(plan?.fallback_url||'');
    if(!prepare&&!fallback)return false;
    directStreamFallbackInProgress=true;
    try{
      setHelp('Direct Stream HLS não iniciou. Tentando Direct Stream MP4 sem recodificar o vídeo…',true);
      destroyHls();
      const prepared=prepare?await prepareURL(plan,prepare):null;
      if(generation!==planGeneration)return true;
      const url=prepared?.url||fallback;
      if(!url)throw new Error('o servidor não forneceu a rota Direct Stream alternativa');
      plan.transport='progressive_mp4';
      applyPlanState(plan);
      await loadSource(url,resume,autoplay,generation);
      await waitForPlayable(generation,15000);
      if(generation!==planGeneration)return true;
      if(typeof sfToast==='function')sfToast('Direct Stream MP4 ativado');
      return true;
    }finally{
      directStreamFallbackInProgress=false;
    }
  }

  async function recoverActiveDirectStream(message,generation){
    if(generation!==planGeneration||!activeItem||!isDirectStreamPlan(activePlan)||activePlan?.transport!=='hls'||directStreamFallbackInProgress)return;
    const position=Number.isFinite(player.currentTime)?player.currentTime:0;
    const autoplay=!player.paused;
    try{
      if(await loadDirectStreamFallback(activePlan,position,autoplay,generation))setHelp('',false);
    }catch(err){
      if(generation!==planGeneration)return;
      setHelp(`Falha no Direct Stream: ${err.message||message||err}`,true);
      if(typeof sfToast==='function')sfToast('Falha no Direct Stream');
    }
  }

  window.addEventListener('stormflix:hls-fatal',event=>{
    const generation=Number(event.detail?.generation||planGeneration);
    recoverActiveDirectStream(event.detail?.message,generation);
  });

  async function start(item,options={}){
    if(!item?.id)return null;
    const generation=++planGeneration;
    const previousSession=options.sessionID||activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    activeItem=item;
    directStreamFallbackInProgress=false;
    applyPlanState(null);
    setHelp('Analisando Direct Play, Direct Stream e compatibilidade do dispositivo…',true);

    let plan;
    try{
      plan=await request(`/media/${Number(item.id)}/playback/plan`,{method:'POST',body:JSON.stringify(clientRequest(previousSession,options.quality||preferredQuality))});
    }catch(err){
      if(generation!==planGeneration)return null;
      destroyHls();player.pause();player.removeAttribute('src');player.load();
      setHelp(`Não foi possível obter o plano de reprodução: ${err.message||err}`,true);
      if(typeof sfToast==='function')sfToast('Não foi possível planejar a reprodução');
      return null;
    }
    if(generation!==planGeneration)return plan;
    applyPlanState(plan);

    if(!plan?.available){
      destroyHls();player.pause();player.removeAttribute('src');player.load();
      setHelp(plan?.reason||'O StormFlix não encontrou uma rota compatível para este arquivo.',true);
      if(typeof sfToast==='function')sfToast('Não foi possível reproduzir este formato');
      return plan;
    }

    let prepared=null;
    try{
      if(plan.prepare_url){
        setHelp(plan.mode==='audio_compatibility'?'Preparando Direct Stream com áudio AAC sem recodificar o vídeo…':'Preparando Direct Stream sem recodificar o vídeo…',true);
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
    const hls=isHLSSource(sourceURL);
    if(hls)plan.transport='hls';
    else if(isDirectStreamPlan(plan))plan.transport='progressive_mp4';
    applyPlanState(plan);
    if(plan.mode==='video_transcode')setHelp(`Otimizando vídeo para ${plan.target_video_height?plan.target_video_height+'p':'este dispositivo'}${plan.encoder?' · '+plan.encoder:''}…`,true);
    else if(isDirectStreamPlan(plan))setHelp(hls?'Iniciando Direct Stream sob demanda…':'Iniciando Direct Stream…',true);
    else setHelp('',false);

    try{
      await loadSource(sourceURL,resume,options.autoplay!==false,generation);
      if(hls&&isDirectStreamPlan(plan))await waitForPlayable(generation,12000);
    }catch(err){
      if(generation!==planGeneration)return plan;
      if(hls&&isDirectStreamPlan(plan)){
        try{
          if(await loadDirectStreamFallback(plan,resume,options.autoplay!==false,generation)){
            if(generation!==planGeneration)return plan;
            setHelp('',false);
            mediaSession(item);ensurePiPControl();bindPlayerErrors();
            return plan;
          }
        }catch(fallbackErr){
          if(generation!==planGeneration)return plan;
          setHelp(`Falha no Direct Stream HLS e MP4: ${fallbackErr.message||fallbackErr}`,true);
          if(typeof sfToast==='function')sfToast('Falha ao iniciar Direct Stream');
          return plan;
        }
      }
      setHelp(`Falha ao iniciar a fonte: ${err.message||err}`,true);
      if(typeof sfToast==='function')sfToast('Falha ao iniciar reprodução');
      return plan;
    }
    if(generation!==planGeneration)return plan;
    setHelp('',false);
    mediaSession(item);ensurePiPControl();bindPlayerErrors();
    return plan;
  }

  async function setQuality(value){
    const quality=normalizeQuality(value);
    preferredQuality=quality;
    localStorage.setItem('stormflix.player.quality',quality);
    if(!activeItem)return null;
    const position=Number.isFinite(player.currentTime)?player.currentTime:0;
    const autoplay=!player.paused;
    const session=activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    return start({...activeItem},{resumePosition:position,autoplay,sessionID:session,quality});
  }

  async function playPlanned(item){
    stopTheme();
    if(typeof sfBuildPlayer==='function')sfBuildPlayer();
    if(typeof sfCurrentMedia!=='undefined')sfCurrentMedia={...item};
    const title=document.querySelector('#player-title');if(title)title.textContent=item.title||'StormFlix';
    const modal=document.querySelector('#player-modal');if(modal){modal.classList.remove('hidden');modal.classList.remove('sf-controls-hidden')}
    if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(item.id);
    if(typeof sfShowControls==='function')sfShowControls();
    return start(item,{autoplay:true,quality:preferredQuality});
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
      sfCurrentMedia=next;activeItem=next;
      if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(id);
      await start(next,{resumePosition:oldTime,autoplay:wasPlaying,sessionID:session,quality:preferredQuality});
      if(typeof sfToast==='function')sfToast(`${version.label||'Versão'} · ${compatibilityMode(activePlan?.mode).replaceAll('_',' ')}`);
      if(typeof sfRenderSettings==='function')sfRenderSettings();
    };
  }

  const previousClosePlayer=closePlayer;
  closePlayer=function(){
    planGeneration++;activeItem=null;directStreamFallbackInProgress=false;destroyHls();applyPlanState(null);
    if(document.pictureInPictureElement)document.exitPictureInPicture().catch(()=>{});
    return previousClosePlayer();
  };
  const closeButton=document.querySelector('#player-close');if(closeButton)closeButton.onclick=closePlayer;

  window.sfEnsureWebAudioCompatibility=function(){return Promise.resolve(activePlan)};
  window.sfTogglePictureInPicture=togglePictureInPicture;

  window.sfPlaybackCore={
    start,
    capabilities:browserCapabilities,
    currentPlan:()=>activePlan,
    sessionID:()=>String(activePlan?.playback_session_id||''),
    currentQuality:()=>effectiveQuality(activePlan,preferredQuality),
    preferredQuality:()=>preferredQuality,
    availableQualities:()=>availableQualities(activePlan),
    setQuality,
    togglePictureInPicture
  };
})();