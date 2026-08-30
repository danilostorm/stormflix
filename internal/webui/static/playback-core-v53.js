/* StormFlix Web Playback Core v5.3 — stable session, Direct Play first. */
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
  let activeAudioStream=null;
  let preferredQuality=normalizeQuality(localStorage.getItem('stormflix.player.quality')||'auto');

  const START_TIMEOUT_MS=14000;

  function canPlay(mediaType){try{return Boolean(player.canPlayType(mediaType))}catch{return false}}

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
    for(const [minimum,value] of [[2160,'2160p'],[1440,'1440p'],[1080,'1080p'],[720,'720p'],[480,'480p']])if(height>=minimum)values.push(value);
    return values;
  }

  function availableQualities(plan=activePlan){
    const allowed=new Set(['auto','original','2160p','1440p','1080p','720p','480p']);
    const supplied=Array.isArray(plan?.available_qualities)?plan.available_qualities.map(normalizeQuality).filter(v=>allowed.has(v)):[];
    const unique=[...new Set(supplied)];
    return unique.includes('auto')&&unique.includes('original')?unique:fallbackQualities(plan);
  }

  function effectiveQuality(plan=activePlan,preferred=preferredQuality){
    const quality=normalizeQuality(preferred);
    if(quality==='auto'||quality==='original'||!plan)return quality;
    const values=availableQualities(plan);
    if(values.includes(quality))return quality;
    const sourceHeight=Number(plan?.video_height||0),requested=qualityHeight(quality);
    if(requested>sourceHeight&&sourceHeight>0){
      const exact=`${sourceHeight}p`;
      return values.includes(exact)?exact:'original';
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
    if(canPlay('audio/flac')||canPlay('audio/mp4; codecs="fLaC"'))audioCodecs.push('flac');
    if(containers.includes('mp4')&&!audioCodecs.includes('aac'))audioCodecs.push('aac');
    return{
      containers:[...new Set(containers)],video_codecs:[...new Set(videoCodecs)],audio_codecs:[...new Set(audioCodecs)],subtitle_formats:['vtt'],
      allow_remux:containers.includes('mp4'),allow_audio_compatibility:containers.includes('mp4')&&audioCodecs.includes('aac'),allow_video_transcode:containers.includes('mp4')&&videoCodecs.includes('h264'),
      max_transcode_bitrate_kbps:estimatedTranscodeBitrate(),native_audio_track_selection:false,server_selects_audio:true,
      picture_in_picture:Boolean(document.pictureInPictureEnabled&&player.requestPictureInPicture),media_session:'mediaSession'in navigator
    };
  }

  function clientRequest(sessionID,quality,startPosition,audioStream){
    const body={client_kind:'web',client_name:'StormFlix Web',client_version:'0.5.3',playback_session_id:String(sessionID||''),quality:normalizeQuality(quality||preferredQuality),capabilities:browserCapabilities()};
    if(Number.isFinite(startPosition))body.start_position_seconds=Math.max(0,Number(startPosition));
    if(Number.isInteger(audioStream)&&audioStream>=0)body.audio_stream=audioStream;
    return body;
  }

  function compatibilityMode(mode){
    if(mode==='video_transcode')return'video_transcode';
    if(mode==='audio_compatibility')return'direct_stream_audio_aac';
    if(mode==='remux')return'direct_stream_remux';
    if(mode==='unsupported')return'unsupported';
    return'direct_play';
  }

  function applyPlanState(plan){
    activePlan=plan||null;
    if(plan&&Number.isInteger(plan.audio_stream))activeAudioStream=plan.audio_stream;
    window.sfLastPlaybackPlan=plan||null;
    window.sfLastCompatibilityPlan=plan||null;
    window.sfPlaybackSessionID=plan?.playback_session_id||'';
    window.sfPlaybackMode=compatibilityMode(plan?.mode);
    window.dispatchEvent(new CustomEvent('stormflix:playback-plan',{detail:plan||null}));
  }

  function setHelp(message,visible){
    const help=document.querySelector('#player-help');if(!help)return;
    if(message)help.textContent=message;
    help.classList.toggle('hidden',!visible);
  }

  function visibleFailure(message){
    setHelp(message||'Não foi possível reproduzir este vídeo.',true);
    if(typeof sfToast==='function')sfToast('Não foi possível reproduzir o vídeo');
  }

  function absoluteSourceURL(source){
    source=String(source||'');if(!source)return'';
    if(/^https?:\/\//i.test(source))return source;
    return source.startsWith('/api/')?source:`${api}${source}`;
  }
  function isHLSSource(source){const v=String(source||'').toLowerCase();return v.includes('.m3u8')||v.includes('/webstream/')||v.includes('/hls/')}
  function destroyHls(){if(activeHls){try{activeHls.destroy()}catch{}activeHls=null}}

  function ensureHlsLibrary(){
    if(window.Hls)return Promise.resolve(window.Hls);
    if(hlsLibraryPromise)return hlsLibraryPromise;
    hlsLibraryPromise=new Promise((resolve,reject)=>{
      const script=document.createElement('script');script.src='https://cdn.jsdelivr.net/npm/hls.js@1.7.1/dist/hls.min.js';script.async=true;script.dataset.stormflixHls='1';
      script.onload=()=>window.Hls?resolve(window.Hls):reject(new Error('hls.js não inicializou'));script.onerror=()=>reject(new Error('hls.js indisponível'));document.head.appendChild(script);
    }).catch(err=>{hlsLibraryPromise=null;throw err});
    return hlsLibraryPromise;
  }

  function waitForPlayable(generation,timeout=START_TIMEOUT_MS){
    if(generation!==planGeneration||player.readyState>=HTMLMediaElement.HAVE_CURRENT_DATA)return Promise.resolve();
    return new Promise((resolve,reject)=>{
      let settled=false;
      const events=['loadeddata','canplay','playing'];
      const cleanup=()=>{clearTimeout(timer);events.forEach(n=>player.removeEventListener(n,ready));player.removeEventListener('error',failed)};
      const finish=(fn,v)=>{if(settled)return;settled=true;cleanup();fn(v)};
      const ready=()=>finish(resolve),failed=()=>finish(reject,new Error(player.error?.message||'O navegador recusou a fonte'));
      const timer=setTimeout(()=>finish(reject,new Error('tempo excedido aguardando o primeiro quadro')),timeout);
      events.forEach(n=>player.addEventListener(n,ready,{once:true}));player.addEventListener('error',failed,{once:true});
    });
  }

  function restoreProgressivePosition(resume,autoplay,generation){
    player.addEventListener('loadedmetadata',function restore(){
      if(generation!==planGeneration)return;
      if(resume>0&&Number.isFinite(player.duration)&&resume<player.duration-1){try{player.currentTime=resume}catch{}}
      if(autoplay)player.play().catch(()=>{});
    },{once:true});
  }

  async function loadHls(url,resume,autoplay,generation){
    destroyHls();
    const source=absoluteSourceURL(url);if(!source)throw new Error('A sessão não retornou manifesto HLS');
    const nativeHls=canPlay('application/vnd.apple.mpegurl')||canPlay('application/x-mpegURL');
    let HlsCtor=null;
    if('MediaSource'in window){try{HlsCtor=await ensureHlsLibrary()}catch(err){if(!nativeHls)throw err}}
    if(generation!==planGeneration)return;
    if(HlsCtor&&HlsCtor.isSupported?.()){
      const hls=new HlsCtor({
        enableWorker:true,progressive:true,startPosition:resume>0?resume:-1,startFragPrefetch:true,
        maxBufferLength:50,maxMaxBufferLength:90,backBufferLength:90,maxBufferHole:0.5,lowLatencyMode:false,
        manifestLoadingTimeOut:10000,fragLoadingTimeOut:30000,manifestLoadingMaxRetry:2,fragLoadingMaxRetry:4,levelLoadingMaxRetry:2
      });
      activeHls=hls;
      let networkRecoveries=0,mediaRecoveries=0;
      hls.on(HlsCtor.Events.ERROR,(_event,data)=>{
        if(!data?.fatal||activeHls!==hls)return;
        window.sfPlaybackLastError=data.details||'Falha HLS';
        if(data.type===HlsCtor.ErrorTypes.NETWORK_ERROR&&networkRecoveries++<2){try{hls.startLoad(player.currentTime||resume||-1)}catch{}return}
        if(data.type===HlsCtor.ErrorTypes.MEDIA_ERROR&&mediaRecoveries++<2){try{hls.recoverMediaError()}catch{}return}
        if(!startupInProgress)recoverRuntime(generation,data.details||'Falha HLS');
      });
      await new Promise((resolve,reject)=>{
        let settled=false;
        const fail=(_e,d)=>{if(settled||!d?.fatal)return;settled=true;reject(new Error(d.details||'Falha ao abrir HLS'))};
        hls.on(HlsCtor.Events.ERROR,fail);
        hls.on(HlsCtor.Events.MEDIA_ATTACHED,()=>{if(generation===planGeneration)hls.loadSource(source)});
        hls.on(HlsCtor.Events.MANIFEST_PARSED,()=>{if(settled)return;settled=true;if(resume>0){try{player.currentTime=resume}catch{}}if(autoplay)player.play().catch(()=>{});resolve()});
        hls.attachMedia(player);
      });
      await waitForPlayable(generation);
      return;
    }
    if(nativeHls){
      restoreProgressivePosition(resume,autoplay,generation);player.src=source;player.load();await waitForPlayable(generation);return;
    }
    throw new Error('Este navegador não oferece HLS/MSE');
  }

  async function loadProgressive(url,resume,autoplay,generation){
    destroyHls();const source=absoluteSourceURL(url);if(!source)throw new Error('O plano não retornou fonte');
    restoreProgressivePosition(resume,autoplay,generation);player.src=source;player.load();if(autoplay)player.play().catch(()=>{});await waitForPlayable(generation);
  }

  async function recoverRuntime(generation,detail){
    if(startupInProgress||generation!==planGeneration||!activeItem)return;
    if(runtimeRecoveryCount>=1){visibleFailure('A reprodução foi interrompida. Tente novamente.');return}
    runtimeRecoveryCount++;
    const position=Number.isFinite(player.currentTime)?player.currentTime:0,autoplay=!player.paused;
    try{
      await start(activeItem,{resumePosition:position,autoplay,quality:preferredQuality,audioStream:activeAudioStream,recovery:true});
    }catch(err){window.sfPlaybackLastError=String(err?.message||detail||err);visibleFailure('A reprodução foi interrompida. Tente novamente.')}
  }

  function bindPlayerErrors(){
    if(playerErrorBound)return;playerErrorBound=true;
    player.addEventListener('error',()=>{if(!activeItem||startupInProgress)return;recoverRuntime(planGeneration,player.error?.message||'Erro de reprodução')});
  }

  function updateMediaSessionPosition(){
    if(!('mediaSession'in navigator)||typeof navigator.mediaSession.setPositionState!=='function')return;
    const duration=Number(player.duration),position=Number(player.currentTime),rate=Number(player.playbackRate||1);if(!Number.isFinite(duration)||duration<=0||!Number.isFinite(position))return;
    try{navigator.mediaSession.setPositionState({duration,position:Math.max(0,Math.min(position,duration)),playbackRate:rate})}catch{}
  }
  function mediaSession(item){
    if(!('mediaSession'in navigator))return;
    try{
      navigator.mediaSession.metadata=new MediaMetadata({title:item?.title||'StormFlix',artist:item?.series_title||item?.library_name||'StormFlix',artwork:item?.poster_url?[{src:item.poster_url}]:[]});
      navigator.mediaSession.setActionHandler('play',()=>player.play().catch(()=>{}));navigator.mediaSession.setActionHandler('pause',()=>player.pause());
      navigator.mediaSession.setActionHandler('seekbackward',d=>{player.currentTime=Math.max(0,(player.currentTime||0)-(d.seekOffset||10))});
      navigator.mediaSession.setActionHandler('seekforward',d=>{player.currentTime=Math.min(player.duration||Infinity,(player.currentTime||0)+(d.seekOffset||10))});
      navigator.mediaSession.setActionHandler('seekto',d=>{if(Number.isFinite(d.seekTime))player.currentTime=d.seekTime});
      if(!mediaSessionBound){mediaSessionBound=true;player.addEventListener('timeupdate',updateMediaSessionPosition,{passive:true});player.addEventListener('durationchange',updateMediaSessionPosition,{passive:true});player.addEventListener('ratechange',updateMediaSessionPosition,{passive:true})}
      updateMediaSessionPosition();
    }catch{}
  }

  async function togglePictureInPicture(){
    if(!document.pictureInPictureEnabled||!player.requestPictureInPicture)return false;
    try{if(document.pictureInPictureElement){await document.exitPictureInPicture();return false}await player.requestPictureInPicture();return true}catch{return false}
  }
  function ensurePiPControl(){
    if(!document.pictureInPictureEnabled||!player.requestPictureInPicture||document.querySelector('#sf-pip'))return;
    const fullscreen=document.querySelector('#sf-fullscreen');if(!fullscreen?.parentElement)return;
    const b=document.createElement('button');b.className='sf-control-btn';b.id='sf-pip';b.type='button';b.setAttribute('aria-label','Picture-in-Picture');b.textContent='▣';b.onclick=()=>togglePictureInPicture();fullscreen.parentElement.insertBefore(b,fullscreen);
  }

  async function start(item,options={}){
    if(!item?.id)return null;
    const generation=++planGeneration;
    const previousSession=options.sessionID||activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    const hasResume=Number.isFinite(options.resumePosition),requestedPosition=hasResume?Number(options.resumePosition):undefined;
    const requestedAudio=Number.isInteger(options.audioStream)?Number(options.audioStream):null;
    activeItem=item;if(!options.recovery)runtimeRecoveryCount=0;startupInProgress=true;applyPlanState(null);setHelp('',false);window.sfPlaybackLastError='';
    let plan;
    try{
      plan=await request(`/media/${Number(item.id)}/playback/plan`,{method:'POST',body:JSON.stringify(clientRequest(previousSession,options.quality||preferredQuality,requestedPosition,requestedAudio))});
    }catch(err){
      if(generation!==planGeneration)return null;startupInProgress=false;window.sfPlaybackLastError=String(err?.message||err);visibleFailure('Não foi possível iniciar este vídeo.');return null;
    }
    if(generation!==planGeneration)return plan;
    applyPlanState(plan);
    if(!plan?.available){startupInProgress=false;visibleFailure(plan?.reason||'Este arquivo não possui uma rota compatível.');return plan}
    const resume=hasResume?requestedPosition:Number(plan.resume_position_seconds||item.position_seconds||0),autoplay=options.autoplay!==false;
    try{
      if(isHLSSource(plan.url))await loadHls(plan.url,resume,autoplay,generation);else await loadProgressive(plan.url,resume,autoplay,generation);
    }catch(err){
      if(generation!==planGeneration)return plan;startupInProgress=false;window.sfPlaybackLastError=String(err?.message||err);
      if(options.recovery||runtimeRecoveryCount>=1)visibleFailure('Não foi possível iniciar este vídeo.');else{runtimeRecoveryCount++;return start(item,{resumePosition:resume,autoplay,quality:preferredQuality,audioStream:activeAudioStream,recovery:true})}
      return plan;
    }
    if(generation!==planGeneration)return plan;
    startupInProgress=false;setHelp('',false);mediaSession(item);ensurePiPControl();bindPlayerErrors();return plan;
  }

  function canKeepCurrentRoute(nextQuality){
    if(!activePlan||activePlan.mode==='video_transcode')return false;
    const sourceHeight=Number(activePlan.video_height||0),requested=qualityHeight(nextQuality);
    if(requested===0)return true;
    return sourceHeight>0&&requested>=sourceHeight;
  }

  async function setQuality(value){
    const quality=normalizeQuality(value),before=preferredQuality;preferredQuality=quality;localStorage.setItem('stormflix.player.quality',quality);
    if(!activeItem||quality===before)return activePlan;
    if(canKeepCurrentRoute(quality)){if(activePlan){activePlan.quality=quality;applyPlanState(activePlan)}return activePlan}
    const position=Number.isFinite(player.currentTime)?player.currentTime:0,autoplay=!player.paused,session=activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    return start({...activeItem},{resumePosition:position,autoplay,sessionID:session,quality,audioStream:activeAudioStream});
  }

  async function setAudioStream(index){
    index=Number(index);if(!Number.isInteger(index)||index<0||!activeItem)return activePlan;
    if(Number(activePlan?.audio_stream)===index)return activePlan;
    const position=Number.isFinite(player.currentTime)?player.currentTime:0,autoplay=!player.paused,session=activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    return start({...activeItem},{resumePosition:position,autoplay,sessionID:session,quality:preferredQuality,audioStream:index});
  }

  async function playPlanned(item){
    stopTheme();if(typeof sfBuildPlayer==='function')sfBuildPlayer();if(typeof sfCurrentMedia!=='undefined')sfCurrentMedia={...item};
    activeAudioStream=null;
    const title=document.querySelector('#player-title');if(title)title.textContent=item.title||'StormFlix';
    const modal=document.querySelector('#player-modal');if(modal){modal.classList.remove('hidden');modal.classList.remove('sf-controls-hidden')}
    if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(item.id);if(typeof sfShowControls==='function')sfShowControls();return start(item,{autoplay:true,quality:preferredQuality});
  }
  playMedia=playPlanned;

  if(typeof sfSelectVersion==='function')sfSelectVersion=async function(id){
    if(!id||Number(id)===Number(sfCurrentMedia?.id))return;const version=(sfVersions||[]).find(v=>Number(v.id)===Number(id));if(!version)return;
    const oldTime=Number.isFinite(player.currentTime)?player.currentTime:0,wasPlaying=!player.paused,session=activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    const next={...sfCurrentMedia,...version,id:Number(id)};sfCurrentMedia=next;activeItem=next;activeAudioStream=null;
    if(typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(id);await start(next,{resumePosition:oldTime,autoplay:wasPlaying,sessionID:session,quality:preferredQuality});if(typeof sfToast==='function')sfToast(version.label||'Versão alterada');if(typeof sfRenderSettings==='function')sfRenderSettings();
  };

  const previousClosePlayer=closePlayer;
  closePlayer=function(){planGeneration++;activeItem=null;activeAudioStream=null;startupInProgress=false;runtimeRecoveryCount=0;destroyHls();applyPlanState(null);if(document.pictureInPictureElement)document.exitPictureInPicture().catch(()=>{});return previousClosePlayer()};
  const closeButton=document.querySelector('#player-close');if(closeButton)closeButton.onclick=closePlayer;

  window.sfEnsureWebAudioCompatibility=function(){return Promise.resolve(activePlan)};
  window.sfTogglePictureInPicture=togglePictureInPicture;
  window.sfPlaybackCore={start,capabilities:browserCapabilities,currentPlan:()=>activePlan,sessionID:()=>String(activePlan?.playback_session_id||''),currentQuality:()=>effectiveQuality(activePlan,preferredQuality),preferredQuality:()=>preferredQuality,availableQualities:()=>availableQualities(activePlan),currentAudioStream:()=>activeAudioStream,setQuality,setAudioStream,togglePictureInPicture};
})();
