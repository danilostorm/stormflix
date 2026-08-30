/* StormFlix Web Playback Core v5.2 — instant-start, Direct Play first. */
(function(){
  let planGeneration=0;
  let activePlan=null;
  let activeItem=null;
  let activeHls=null;
  let hlsLibraryPromise=null;
  let mediaSessionBound=false;
  let playerErrorBound=false;
  let startupInProgress=false;
  let runtimeRecoveryCount=0;
  let preferredQuality=normalizeQuality(localStorage.getItem('stormflix.player.quality')||'auto');

  const START_TIMEOUT_MS=6000;
  const RETRY_TIMEOUT_MS=7000;

  function canPlay(mediaType){
    try{return Boolean(player.canPlayType(mediaType))}catch{return false}
  }

  function delay(ms){return new Promise(resolve=>setTimeout(resolve,ms))}

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
      client_version:'0.5.2',
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

  function isDirectStreamPlan(plan){return plan?.mode==='remux'||plan?.mode==='audio_compatibility'}

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

  function failVisible(message){
    setHelp(message||'Não foi possível iniciar este vídeo.',true);
    if(typeof sfToast==='function')sfToast('Não foi possível iniciar o vídeo');
  }

  async function preparePlan(plan){
    if(!plan?.prepare_url)return null;
    const path=String(plan.prepare_url).replace(/^\/api\/v1/,'');
    const prepared=await request(path,{method:'POST',body:'{}'});
    if(!prepared?.ready)throw new Error('A fonte de compatibilidade não ficou pronta.');
    if(!prepared.seekable)throw new Error('A fonte de compatibilidade não ficou seekable.');
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
        existing.addEventListener('error',()=>reject(new Error('hls.js indisponível.')),{once:true});
        return;
      }
      const script=document.createElement('script');
      script.src='https://cdn.jsdelivr.net/npm/hls.js@1.7.1/dist/hls.min.js';
      script.async=true;
      script.dataset.stormflixHls='1';
      script.onload=()=>window.Hls?resolve(window.Hls):reject(new Error('hls.js não inicializou.'));
      script.onerror=()=>reject(new Error('hls.js indisponível.'));
      document.head.appendChild(script);
    }).catch(err=>{hlsLibraryPromise=null;throw err});
    return hlsLibraryPromise;
  }

  function installRestore(resume,autoplay,generation){
    const restore=()=>{
      if(generation!==planGeneration)return;
      const position=Number(resume||0);
      if(position>0&&Number.isFinite(player.duration)&&position<player.duration-3){
        try{player.currentTime=position}catch{}
      }
      if(autoplay)player.play().catch(()=>{});
      updateMediaSessionPosition();
    };
    player.addEventListener('loadedmetadata',restore,{once:true});
  }

  function waitForPlayable(generation,timeoutMs){
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
      const ready=()=>finish(resolve);
      const failed=()=>finish(reject,new Error(player.error?.message||'O navegador recusou a fonte.'));
      const timer=setTimeout(()=>finish(reject,new Error('tempo excedido aguardando o primeiro quadro')),timeoutMs||START_TIMEOUT_MS);
      events.forEach(name=>player.addEventListener(name,ready,{once:true}));
      player.addEventListener('error',failed,{once:true});
    });
  }

  async function attachHls(url,generation){
    destroyHls();
    const nativeHls=canPlay('application/vnd.apple.mpegurl')||canPlay('application/x-mpegURL');
    let HlsCtor=null;
    if('MediaSource' in window){
      try{HlsCtor=await ensureHlsLibrary()}catch(err){if(!nativeHls)throw err}
    }
    if(generation!==planGeneration)return;

    if(HlsCtor&&typeof HlsCtor.isSupported==='function'&&HlsCtor.isSupported()){
      const hls=new HlsCtor({
        enableWorker:true,
        progressive:true,
        startFragPrefetch:true,
        maxBufferLength:36,
        maxMaxBufferLength:60,
        backBufferLength:15,
        maxBufferHole:0.5,
        lowLatencyMode:false,
        manifestLoadingTimeOut:6000,
        fragLoadingTimeOut:20000,
        manifestLoadingMaxRetry:1,
        fragLoadingMaxRetry:2,
        levelLoadingMaxRetry:1
      });
      activeHls=hls;
      let manifestReady=false,networkRecoveries=0,mediaRecoveries=0;
      hls.on(HlsCtor.Events.ERROR,(_event,data)=>{
        if(!data?.fatal||activeHls!==hls)return;
        window.sfPlaybackLastError=data.details||'Falha no HLS';
        if(!manifestReady)return;
        if(data.type===HlsCtor.ErrorTypes.NETWORK_ERROR&&networkRecoveries++<1){try{hls.startLoad()}catch{};return}
        if(data.type===HlsCtor.ErrorTypes.MEDIA_ERROR&&mediaRecoveries++<1){try{hls.recoverMediaError()}catch{};return}
        if(!startupInProgress)window.dispatchEvent(new CustomEvent('stormflix:web-hls-fatal',{detail:{generation,message:data.details||'Falha no HLS'}}));
      });
      await new Promise((resolve,reject)=>{
        let settled=false;
        const fail=(_event,data)=>{if(settled||!data?.fatal)return;settled=true;reject(new Error(data.details||'Falha ao abrir HLS.'))};
        hls.on(HlsCtor.Events.ERROR,fail);
        hls.on(HlsCtor.Events.MEDIA_ATTACHED,()=>{
          if(generation!==planGeneration){if(!settled){settled=true;resolve()}return}
          hls.loadSource(url);
        });
        hls.on(HlsCtor.Events.MANIFEST_PARSED,()=>{
          manifestReady=true;
          if(!settled){settled=true;resolve()}
        });
        hls.attachMedia(player);
      });
      return;
    }

    if(nativeHls){player.src=url;player.load();return}
    throw new Error('Este navegador não possui suporte HLS/MSE.');
  }

  async function startHlsWithWarmRetry(url,resume,autoplay,generation){
    const source=absoluteSourceURL(url);
    if(!source)throw new Error('O plano não retornou uma fonte HLS.');
    installRestore(resume,autoplay,generation);
    await attachHls(source,generation);
    if(generation!==planGeneration)return;
    if(autoplay)player.play().catch(()=>{});
    try{
      await waitForPlayable(generation,START_TIMEOUT_MS);
      return;
    }catch(firstError){
      if(generation!==planGeneration)return;
      // The server has been priming the same session in parallel. Reattaching
      // once uses those now-warm fragments instead of starting a different
      // quality or materializing the entire movie.
      destroyHls();
      await delay(120);
      if(generation!==planGeneration)return;
      await attachHls(source,generation);
      if(autoplay)player.play().catch(()=>{});
      try{
        await waitForPlayable(generation,RETRY_TIMEOUT_MS);
        return;
      }catch(secondError){
        secondError.cause=firstError;
        throw secondError;
      }
    }
  }

  async function loadProgressive(url,resume,autoplay,generation){
    const source=absoluteSourceURL(url);
    if(!source)throw new Error('O plano não retornou uma fonte.');
    destroyHls();
    installRestore(resume,autoplay,generation);
    player.src=source;
    player.load();
    if(autoplay)player.play().catch(()=>{});
    await waitForPlayable(generation,RETRY_TIMEOUT_MS);
  }

  async function recoverRuntimeHls(message,generation){
    if(startupInProgress||generation!==planGeneration||!activeItem||!activePlan||!isHLSSource(activePlan.url))return;
    if(runtimeRecoveryCount>=1){
      failVisible('A reprodução foi interrompida. Tente novamente.');
      return;
    }
    runtimeRecoveryCount++;
    const position=Number.isFinite(player.currentTime)?player.currentTime:0;
    const autoplay=!player.paused;
    try{
      setHelp('',false);
      await startHlsWithWarmRetry(activePlan.url,position,autoplay,generation);
      if(generation!==planGeneration)return;
      setHelp('',false);
    }catch(err){
      if(generation!==planGeneration)return;
      failVisible('A reprodução foi interrompida. Tente novamente.');
      window.sfPlaybackLastError=String(err?.message||message||err||'Falha no HLS');
    }
  }

  window.addEventListener('stormflix:web-hls-fatal',event=>{
    recoverRuntimeHls(event.detail?.message,Number(event.detail?.generation||planGeneration));
  });

  function bindPlayerErrors(){
    if(playerErrorBound)return;
    playerErrorBound=true;
    player.addEventListener('error',()=>{
      if(!activeItem||startupInProgress)return;
      if(isHLSSource(activePlan?.url)){
        recoverRuntimeHls(player.error?.message||'Erro HLS',planGeneration);
        return;
      }
      failVisible(activePlan?.reason||'Não foi possível reproduzir esta fonte.');
    });
  }

  async function start(item,options={}){
    if(!item?.id)return null;
    const generation=++planGeneration;
    const previousSession=options.sessionID||activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    const requestedQuality=normalizeQuality(options.quality||preferredQuality);
    activeItem=item;
    runtimeRecoveryCount=0;
    startupInProgress=true;
    applyPlanState(null);
    setHelp('',false);
    window.sfPlaybackLastError='';

    let plan;
    try{
      plan=await request(`/media/${Number(item.id)}/playback/plan`,{method:'POST',body:JSON.stringify(clientRequest(previousSession,requestedQuality))});
    }catch(err){
      if(generation!==planGeneration)return null;
      destroyHls();player.pause();player.removeAttribute('src');player.load();
      startupInProgress=false;
      failVisible('Não foi possível iniciar este vídeo.');
      window.sfPlaybackLastError=String(err?.message||err);
      return null;
    }
    if(generation!==planGeneration)return plan;
    applyPlanState(plan);

    if(!plan?.available){
      destroyHls();player.pause();player.removeAttribute('src');player.load();
      startupInProgress=false;
      failVisible(plan?.reason||'Este arquivo não possui uma rota compatível.');
      return plan;
    }

    let prepared=null;
    try{
      if(plan.prepare_url)prepared=await preparePlan(plan);
      if(generation!==planGeneration)return plan;
      const resume=Number.isFinite(options.resumePosition)?options.resumePosition:Number(plan.resume_position_seconds||item.position_seconds||0);
      const sourceURL=prepared?.url||plan.url;
      const autoplay=options.autoplay!==false;
      const hls=isHLSSource(sourceURL);
      if(hls)plan.transport='hls';
      else if(isDirectStreamPlan(plan))plan.transport='progressive_mp4';
      applyPlanState(plan);

      if(hls)await startHlsWithWarmRetry(sourceURL,resume,autoplay,generation);
      else await loadProgressive(sourceURL,resume,autoplay,generation);
    }catch(err){
      if(generation!==planGeneration)return plan;
      startupInProgress=false;
      window.sfPlaybackLastError=String(err?.message||err);
      failVisible('Não foi possível iniciar este vídeo.');
      return plan;
    }

    if(generation!==planGeneration)return plan;
    startupInProgress=false;
    setHelp('',false);
    mediaSession(item);
    ensurePiPControl();
    bindPlayerErrors();
    return plan;
  }

  function canKeepCurrentRoute(nextQuality){
    if(!activePlan||activePlan.mode==='video_transcode')return false;
    const sourceHeight=Number(activePlan.video_height||0);
    const requestedHeight=qualityHeight(nextQuality);
    if(requestedHeight===0)return true;
    return sourceHeight>0&&requestedHeight>=sourceHeight;
  }

  async function setQuality(value){
    const quality=normalizeQuality(value);
    const before=preferredQuality;
    preferredQuality=quality;
    localStorage.setItem('stormflix.player.quality',quality);
    if(!activeItem||quality===before)return activePlan;

    // Auto, Original and the source's own resolution are presentation choices
    // when the current route is already Direct Play/Direct Stream. Do not tear
    // down a healthy stream just because the visitor opened the quality menu.
    if(canKeepCurrentRoute(quality)){
      if(activePlan){activePlan.quality=quality;applyPlanState(activePlan)}
      return activePlan;
    }

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
      if(typeof sfToast==='function')sfToast(version.label||'Versão alterada');
      if(typeof sfRenderSettings==='function')sfRenderSettings();
    };
  }

  const previousClosePlayer=closePlayer;
  closePlayer=function(){
    planGeneration++;
    activeItem=null;
    startupInProgress=false;
    runtimeRecoveryCount=0;
    destroyHls();
    applyPlanState(null);
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