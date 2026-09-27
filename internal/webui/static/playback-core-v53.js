/* StormFlix Web Playback Core v7 — original Direct Play and local decode. */
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
  let localDecodeRuntimeFailed=false;
  let localOriginRuntimeFailed=false;
  let hevcSupportPromise=null;
  let hevcSupportHandle=null;
  let rejectionMediaID=0;
  let nativeSourceRejected=false;
  let startupMetrics={plan_ms:0,first_frame_ms:0,startup_ms:0,stall_count:0,last_stall_ms:0};
  let stallStartedAt=0;
  let preferredQuality='original';

  const START_TIMEOUT_MS=14000;
  const HLS_LOCAL_URL='/vendor-hls-1.7.1.min.js';
  const HEVC_PLUGIN_URL='/vendor-hevc-hls-plugin-0.1.2.js';
  const HEVC_WORKER_URL='/vendor-hevc-transcode-worker-1.4.2.js';
  const HEVC_WASM_GLUE_URL='/vendor-hevc-decode-1.4.2.js';
  const HEVC_WASM_BINARY_URL='/vendor-hevc-decode-1.4.2.wasm';

  function canPlay(mediaType){try{return Boolean(player.canPlayType(mediaType))}catch{return false}}

  function beginStartupMetrics(){
    startupMetrics={started_at:performance.now(),plan_ms:0,first_frame_ms:0,startup_ms:0,stall_count:0,last_stall_ms:0};
    window.sfPlaybackStartupMetrics=startupMetrics;
  }
  async function finishStartupMetrics(){
    if(typeof player.requestVideoFrameCallback==='function'){
      await new Promise(resolve=>{let done=false;const finish=()=>{if(done)return;done=true;resolve()};player.requestVideoFrameCallback(finish);setTimeout(finish,250)});
    }
    const elapsed=Math.max(0,performance.now()-Number(startupMetrics.started_at||performance.now()));
    startupMetrics.first_frame_ms=elapsed;startupMetrics.startup_ms=elapsed;window.sfPlaybackStartupMetrics=startupMetrics;
  }

  player.addEventListener('waiting',()=>{if(activeItem&&!stallStartedAt)stallStartedAt=performance.now()});
  player.addEventListener('playing',()=>{if(!stallStartedAt)return;startupMetrics.stall_count=Number(startupMetrics.stall_count||0)+1;startupMetrics.last_stall_ms=Math.max(0,performance.now()-stallStartedAt);stallStartedAt=0;window.sfPlaybackStartupMetrics=startupMetrics});

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

  function availableQualities(){return ['original']}
  function effectiveQuality(){return 'original'}

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
    const audioCodecs=[];
    if(canPlay('audio/mp4; codecs="mp4a.40.2"')||canPlay('audio/aac'))audioCodecs.push('aac');
    if(canPlay('audio/mpeg'))audioCodecs.push('mp3');
    if(canPlay('audio/webm; codecs="opus"'))audioCodecs.push('opus');
    if(canPlay('audio/mp4; codecs="ac-3"'))audioCodecs.push('ac3');
    if(canPlay('audio/mp4; codecs="ec-3"'))audioCodecs.push('eac3');
    if(canPlay('audio/flac')||canPlay('audio/mp4; codecs="fLaC"'))audioCodecs.push('flac');
    const nativeContainers=[...new Set(containers)];
    const nativeVideoCodecs=[...new Set(videoCodecs)];
    const nativeAudioCodecs=[...new Set(audioCodecs)];
    const device=typeof window.sfDeviceCapabilitySnapshot==='function'?window.sfDeviceCapabilitySnapshot():{max_height:1080,video_codecs:[]};
    const advertised4K=new Set(device.video_codecs||[]),deviceHeight=Math.max(480,Number(device.max_height||1080));
    const videoProfiles=nativeVideoCodecs.map(codec=>{
      const maxHeight=deviceHeight>=2000&&!advertised4K.has(codec)?1080:deviceHeight;
      return{codec,max_width:maxHeight>=2000?3840:maxHeight>=1300?2560:maxHeight>=900?1920:1280,max_height:maxHeight,hdr_known:false,hdr_types:[]};
    });
    return{
      containers:nativeContainers,video_codecs:nativeVideoCodecs,audio_codecs:nativeAudioCodecs,subtitle_formats:['vtt'],
      video_profiles:videoProfiles,
      allow_remux:false,allow_audio_compatibility:false,allow_video_transcode:false,
      max_transcode_bitrate_kbps:estimatedTranscodeBitrate(),native_audio_track_selection:false,server_selects_audio:true,
      picture_in_picture:Boolean(document.pictureInPictureEnabled&&player.requestPictureInPicture),media_session:'mediaSession'in navigator
    };
  }

  function hasWebGL(){
    try{const c=document.createElement('canvas');return Boolean(c.getContext('webgl2')||c.getContext('webgl'))}catch{return false}
  }

  function hasWasmSIMD(){
    try{return WebAssembly.validate(Uint8Array.from(atob('AGFzbQEAAAABBQFgAAF7AhIBA2VudgZtZW1vcnkCAwGAgAIDAgEACgoBCABBAP0ABAAL'),c=>c.charCodeAt(0)))}catch{return false}
  }

  function localDecodeClientKind(){
    const ua=String(navigator.userAgent||'').toLowerCase();
    if(ua.includes('android')||ua.includes('; wv')||window.NativePlaybackAnywhere)return'android_webview';
    if(/tizen|web0s|webos|smart-tv|smarttv|hbbtv|netcast/.test(ua))return'tv';
    if(navigator.userAgentData?.mobile||/iphone|ipad|ipod|mobile/.test(ua)||(/macintosh/.test(ua)&&Number(navigator.maxTouchPoints||0)>1))return'mobile_web';
    return'web';
  }

  function browserLocalDecodeCapabilities(){
    const wasm=typeof WebAssembly==='object'&&typeof WebAssembly.instantiate==='function';
    const worker=typeof Worker==='function';
    const webcodecs=typeof VideoEncoder==='function'&&typeof VideoDecoder==='function';
    const secure=Boolean(window.isSecureContext||location.hostname==='localhost'||location.hostname==='127.0.0.1');
    const cores=Math.max(1,Number(navigator.hardwareConcurrency||2));
    const memory=Math.max(0,Number(navigator.deviceMemory||0));
    const kind=localDecodeClientKind();
    const automatic4K=cores>=12&&(memory===0||memory>=8);
    let maxHeight=720;
    if(cores>=6)maxHeight=1080;
    if(automatic4K)maxHeight=2160;
    const maxWidth=maxHeight>=2160?3840:maxHeight>=1080?1920:1280;
    const enabled=!localDecodeRuntimeFailed&&kind==='web';
    return{
      kind,enabled,wasm,worker,webgl:hasWebGL(),webgpu:Boolean(navigator.gpu),webcodecs,secure_context:secure,
      hevc_wasm:false,av1_wasm:false,hdr:false,
      original_file:enabled&&!localOriginRuntimeFailed&&wasm&&hasWasmSIMD()&&worker&&secure&&hasWebGL(),wasm_simd:hasWasmSIMD(),
      max_width:maxWidth,max_height:maxHeight,hardware_concurrency:cores,device_memory_gb:memory,
      codecs:['h264','hevc','av1'],containers:['mkv','mp4','webm'],audio_codecs:['aac','ac3','eac3','dts','mp3','opus','flac','vorbis'],subtitle_formats:['vtt','srt','ass','ssa']
    };
  }

  function clientRequest(sessionID,quality,startPosition,audioStream){
    const body={client_kind:'web',client_name:'StormFlix Web',client_version:'0.8.0',original_only:true,native_source_rejected:nativeSourceRejected,playback_session_id:String(sessionID||''),capabilities:browserCapabilities(),local_decode:browserLocalDecodeCapabilities()};
    let native=null;
    if(typeof window.StormFlixShell?.playbackRequest==='function'){
      try{native=JSON.parse(String(window.StormFlixShell.playbackRequest(String(sessionID||''))||''))}catch{}
    }
    if(typeof window.sfConstrainPlaybackCapabilities==='function')body.capabilities=window.sfConstrainPlaybackCapabilities(body.capabilities,native?.capabilities);
    const device=window.sfDeviceCapabilitySnapshot?.();
    if(device){
      body.local_decode.max_height=Math.min(body.local_decode.max_height,device.max_height||1080);
      body.local_decode.max_width=Math.min(body.local_decode.max_width,body.local_decode.max_height>=2000?3840:body.local_decode.max_height>=1300?2560:1920);
      // The v6 engine only supports HEVC. Keep its whole local budget at FHD
      // when UHD HEVC failed, even if another native UHD codec still works.
      if(body.local_decode.max_height>=2000&&!device.video_codecs.includes('hevc')){body.local_decode.max_height=1080;body.local_decode.max_width=1920}
    }
    if(native){body.client_name='StormFlix Android Web Player';body.client_version=native.client_version;body.preferred_audio_language=native.preferred_audio_language||''}
    body.quality='original';
    if(Number.isFinite(startPosition))body.start_position_seconds=Math.max(0,Number(startPosition));
    if(Number.isInteger(audioStream)&&audioStream>=0)body.audio_stream=audioStream;
    return body;
  }

  function compatibilityMode(plan){
    if(plan?.local_origin)return'original_local_decode';
    if(plan?.local_decode)return'wasm_local_decode';
    const mode=plan?.mode;
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
    window.sfPlaybackMode=compatibilityMode(plan);
    const pip=document.querySelector('#sf-pip');if(pip)pip.classList.toggle('hidden',Boolean(plan?.local_origin));
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
  async function destroyLocalOrigin(){if(window.sfLocalOrigin?.isActive?.())await window.sfLocalOrigin.destroy()}

  function ensureHlsLibrary(){
    if(window.Hls)return Promise.resolve(window.Hls);
    if(hlsLibraryPromise)return hlsLibraryPromise;
    hlsLibraryPromise=new Promise((resolve,reject)=>{
      const script=document.createElement('script');script.src=HLS_LOCAL_URL;script.async=true;script.dataset.stormflixHls='1';
      script.onload=()=>window.Hls?resolve(window.Hls):reject(new Error('hls.js não inicializou'));script.onerror=()=>reject(new Error('hls.js indisponível'));document.head.appendChild(script);
    }).catch(err=>{hlsLibraryPromise=null;throw err});
    return hlsLibraryPromise;
  }

  async function ensureHevcWasmSupport(){
    if(hevcSupportHandle)return hevcSupportHandle;
    if(hevcSupportPromise)return hevcSupportPromise;
    const caps=browserLocalDecodeCapabilities();
    if(!caps.hevc_wasm)throw new Error('Este navegador não oferece os recursos seguros necessários para decode HEVC local.');
    hevcSupportPromise=new Promise((resolve,reject)=>{
      if(globalThis.HevcHls)return resolve(globalThis.HevcHls);
      const script=document.createElement('script');script.src=HEVC_PLUGIN_URL;script.async=true;script.dataset.stormflixHevc='1';
      script.onload=()=>globalThis.HevcHls?resolve(globalThis.HevcHls):reject(new Error('Runtime HEVC WASM não inicializou'));
      script.onerror=()=>reject(new Error('Runtime HEVC WASM local indisponível'));
      document.head.appendChild(script);
    }).then(async mod=>{
      if(typeof mod?.attachHevcSupport!=='function')throw new Error('Runtime HEVC WASM inválido.');
      const handle=await mod.attachHevcSupport({
        workerUrl:HEVC_WORKER_URL,
        wasmUrl:HEVC_WASM_GLUE_URL,
        wasmBinaryUrl:HEVC_WASM_BINARY_URL,
        adaptiveCompute:{
          targetSpeedX:1.25,
          onObservation:(stat,avg,capIndex,reason)=>{
            window.sfLocalDecodeStats={engine:'hevc.js',codec:'hevc',speed_x:Number(stat?.speedX||0),average_speed_x:Number(avg||0),width:Number(stat?.width||0),height:Number(stat?.height||0),cap_index:Number(capIndex??-1),reason:String(reason||''),updated_at:Date.now()};
            window.dispatchEvent(new CustomEvent('stormflix:local-decode-stat',{detail:window.sfLocalDecodeStats}));
          }
        }
      });
      hevcSupportHandle=handle||{};
      return hevcSupportHandle;
    }).catch(err=>{hevcSupportPromise=null;localDecodeRuntimeFailed=true;throw err});
    return hevcSupportPromise;
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
    await destroyLocalOrigin();destroyHls();
    const source=absoluteSourceURL(url);if(!source)throw new Error('A sessão não retornou manifesto HLS');
    const nativeHls=canPlay('application/vnd.apple.mpegurl')||canPlay('application/x-mpegURL');
    let HlsCtor=null;
    if('MediaSource'in window){try{HlsCtor=await ensureHlsLibrary()}catch(err){if(!nativeHls)throw err}}
    if(generation!==planGeneration)return;
    if(HlsCtor&&HlsCtor.isSupported?.()){
      let localHandle=null;
      if(activePlan?.local_decode){
        try{localHandle=await ensureHevcWasmSupport()}catch(err){localDecodeRuntimeFailed=true;throw new Error(`Decode HEVC local indisponível: ${err?.message||err}`)}
        if(generation!==planGeneration)return;
      }
      const hls=new HlsCtor({
        enableWorker:true,progressive:true,startPosition:resume>0?resume:-1,startFragPrefetch:true,
        maxBufferLength:50,maxMaxBufferLength:90,backBufferLength:90,maxBufferHole:0.5,lowLatencyMode:false,
        manifestLoadingTimeOut:10000,fragLoadingTimeOut:30000,manifestLoadingMaxRetry:2,fragLoadingMaxRetry:4,levelLoadingMaxRetry:2,
        ...(typeof MediaSource!=='undefined'?{preferManagedMediaSource:false}:{})
      });
      try{localHandle?.attachComputeAware?.(hls)}catch{}
      activeHls=hls;
      let networkRecoveries=0,mediaRecoveries=0;
      hls.on(HlsCtor.Events.ERROR,(_event,data)=>{
        if(!data?.fatal||activeHls!==hls)return;
        window.sfPlaybackLastError=data.details||'Falha HLS';
        if(data.type===HlsCtor.ErrorTypes.NETWORK_ERROR&&networkRecoveries++<2){try{hls.startLoad(player.currentTime||resume||-1)}catch{}return}
        if(data.type===HlsCtor.ErrorTypes.MEDIA_ERROR&&mediaRecoveries++<2){try{hls.recoverMediaError()}catch{}return}
        if(activePlan?.local_decode)localDecodeRuntimeFailed=true;
        if(!startupInProgress)recoverRuntime(generation,data.details||'Falha HLS');
      });
      await new Promise((resolve,reject)=>{
        let settled=false;
        const fail=(_e,d)=>{if(settled||!d?.fatal)return;settled=true;if(activePlan?.local_decode&&d.type===HlsCtor.ErrorTypes.MEDIA_ERROR)localDecodeRuntimeFailed=true;reject(new Error(d.details||'Falha ao abrir HLS'))};
        hls.on(HlsCtor.Events.ERROR,fail);
        hls.on(HlsCtor.Events.MEDIA_ATTACHED,()=>{if(generation===planGeneration)hls.loadSource(source)});
        hls.on(HlsCtor.Events.MANIFEST_PARSED,()=>{if(settled)return;settled=true;if(resume>0){try{player.currentTime=resume}catch{}}if(autoplay)player.play().catch(()=>{});resolve()});
        hls.attachMedia(player);
      });
      await waitForPlayable(generation);
      return;
    }
    if(nativeHls&&!activePlan?.local_decode){
      restoreProgressivePosition(resume,autoplay,generation);player.src=source;player.load();await waitForPlayable(generation);return;
    }
    throw new Error(activePlan?.local_decode?'O runtime local requer MediaSource/hls.js.':'Este navegador não oferece HLS/MSE');
  }

  async function loadProgressive(url,resume,autoplay,generation){
    await destroyLocalOrigin();destroyHls();const source=absoluteSourceURL(url);if(!source)throw new Error('O plano não retornou fonte');
    restoreProgressivePosition(resume,autoplay,generation);player.src=source;player.load();if(autoplay)player.play().catch(()=>{});await waitForPlayable(generation);
  }

  async function loadLocalOrigin(plan,resume,autoplay,generation){
    destroyHls();
    if(!window.sfLocalOrigin)throw new Error('Runtime de arquivo original não carregou');
    const source=absoluteSourceURL(plan?.url);if(!source)throw new Error('O plano local não retornou a fonte original');
    await window.sfLocalOrigin.load(source,plan,{resume,autoplay,audioStream:Number(plan?.audio_stream)});
    if(generation!==planGeneration)await window.sfLocalOrigin.destroy();
  }

  function rejectNativePlan(plan){
    if(plan?.mode!=='direct_play'||nativeSourceRejected)return false;
    nativeSourceRejected=true;
    return true;
  }
  async function recoverRuntime(generation,detail){
    if(startupInProgress||generation!==planGeneration||!activeItem)return;
    if(runtimeRecoveryCount>=2){visibleFailure('A reprodução foi interrompida. Tente novamente.');return}
    if(Number(activePlan?.video_width)>=3200||Number(activePlan?.video_height)>=2000)window.sfRejectUHDCodec?.(activePlan?.source_video_codec);
    rejectNativePlan(activePlan);
    runtimeRecoveryCount++;
    const position=Number.isFinite(player.currentTime)?player.currentTime:0,autoplay=!player.paused;
    try{
      await start(activeItem,{resumePosition:position,autoplay,quality:preferredQuality,audioStream:activeAudioStream,recovery:true});
    }catch(err){window.sfPlaybackLastError=String(err?.message||detail||err);visibleFailure('A reprodução foi interrompida. Tente novamente.')}
  }

  function bindPlayerErrors(){
    if(playerErrorBound)return;playerErrorBound=true;
    player.addEventListener('error',()=>{if(!activeItem||startupInProgress)return;if(activePlan?.local_origin)localOriginRuntimeFailed=true;else if(activePlan?.local_decode)localDecodeRuntimeFailed=true;recoverRuntime(planGeneration,player.error?.message||'Erro de reprodução')});
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
    if(activePlan?.local_origin)return false;
    if(!document.pictureInPictureEnabled||!player.requestPictureInPicture)return false;
    try{if(document.pictureInPictureElement){await document.exitPictureInPicture();return false}await player.requestPictureInPicture();return true}catch{return false}
  }
  function ensurePiPControl(){
    if(!document.pictureInPictureEnabled||!player.requestPictureInPicture||document.querySelector('#sf-pip'))return;
    const fullscreen=document.querySelector('#sf-fullscreen');if(!fullscreen?.parentElement)return;
    const b=document.createElement('button');b.className='sf-control-btn';b.id='sf-pip';b.type='button';b.setAttribute('aria-label','Picture-in-Picture');b.textContent='▣';b.onclick=()=>togglePictureInPicture();b.classList.toggle('hidden',Boolean(activePlan?.local_origin));fullscreen.parentElement.insertBefore(b,fullscreen);
  }

  async function start(item,options={}){
    if(!item?.id)return null;
    const generation=++planGeneration;
    const previousSession=options.sessionID||activePlan?.playback_session_id||window.sfPlaybackSessionID||'';
    const hasResume=Number.isFinite(options.resumePosition),requestedPosition=hasResume?Number(options.resumePosition):undefined;
    const requestedAudio=Number.isInteger(options.audioStream)?Number(options.audioStream):null;
    if(Number(item.id)!==rejectionMediaID){rejectionMediaID=Number(item.id);nativeSourceRejected=false;localOriginRuntimeFailed=false;localDecodeRuntimeFailed=false}
    activeItem=item;if(!options.recovery)runtimeRecoveryCount=0;startupInProgress=true;beginStartupMetrics();stallStartedAt=0;applyPlanState(null);setHelp('',false);window.sfPlaybackLastError='';
    let plan;
    try{
      if(typeof window.sfCatalogCapabilityQuery==='function')await window.sfCatalogCapabilityQuery();
      plan=await request(`/media/${Number(item.id)}/playback/plan`,{method:'POST',body:JSON.stringify(clientRequest(previousSession,options.quality||preferredQuality,requestedPosition,requestedAudio))});
      startupMetrics.plan_ms=Math.max(0,performance.now()-startupMetrics.started_at);window.sfPlaybackStartupMetrics=startupMetrics;
    }catch(err){
      if(generation!==planGeneration)return null;startupInProgress=false;window.sfPlaybackLastError=String(err?.message||err);visibleFailure('Não foi possível iniciar este vídeo.');return null;
    }
    if(generation!==planGeneration)return plan;
    applyPlanState(plan);
    if(!plan?.available){startupInProgress=false;visibleFailure(plan?.reason||'Este arquivo não possui uma rota compatível.');return plan}
    if(plan.auto_selected_version&&typeof sfLoadPlayerOptions==='function')await sfLoadPlayerOptions(plan.selected_media_id);
    if(generation!==planGeneration)return plan;
    const resume=hasResume?requestedPosition:Number(plan.resume_position_seconds||item.position_seconds||0),autoplay=options.autoplay!==false;
    try{
      if(plan?.local_origin)await loadLocalOrigin(plan,resume,autoplay,generation);else if(isHLSSource(plan.url))await loadHls(plan.url,resume,autoplay,generation);else await loadProgressive(plan.url,resume,autoplay,generation);
    }catch(err){
      if(generation!==planGeneration)return plan;startupInProgress=false;window.sfPlaybackLastError=String(err?.message||err);
      if(plan?.local_origin)localOriginRuntimeFailed=true;else if(plan?.local_decode)localDecodeRuntimeFailed=true;
      if(Number(plan?.video_width)>=3200||Number(plan?.video_height)>=2000)window.sfRejectUHDCodec?.(plan?.source_video_codec);
      // Retry this source with the local decoder without invalidating the
      // browser's support for every other MP4/AAC title.
      rejectNativePlan(plan);
      if(runtimeRecoveryCount>=2)visibleFailure('Não foi possível iniciar este vídeo.');else{runtimeRecoveryCount++;return start(item,{resumePosition:resume,autoplay,quality:preferredQuality,audioStream:activeAudioStream,recovery:true})}
      return plan;
    }
    if(generation!==planGeneration)return plan;
    await finishStartupMetrics();
    startupInProgress=false;setHelp('',false);mediaSession(item);ensurePiPControl();bindPlayerErrors();return plan;
  }

  function canKeepCurrentRoute(nextQuality){
    if(!activePlan||activePlan.mode==='video_transcode')return false;
    const sourceHeight=Number(activePlan.video_height||0),requested=qualityHeight(nextQuality);
    if(requested===0)return true;
    return sourceHeight>0&&requested>=sourceHeight;
  }

  async function setQuality(){return activePlan}

  async function setAudioStream(index){
    index=Number(index);if(!Number.isInteger(index)||index<0||!activeItem)return activePlan;
    if(Number(activePlan?.audio_stream)===index)return activePlan;
    if(activePlan?.local_origin&&window.sfLocalOrigin?.isActive?.()){
      await window.sfLocalOrigin.selectAudio(index);activeAudioStream=index;activePlan.audio_stream=index;applyPlanState(activePlan);return activePlan;
    }
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
  closePlayer=function(){planGeneration++;activeItem=null;activeAudioStream=null;startupInProgress=false;runtimeRecoveryCount=0;rejectionMediaID=0;nativeSourceRejected=false;localOriginRuntimeFailed=false;localDecodeRuntimeFailed=false;destroyHls();window.sfLocalOrigin?.destroy?.();applyPlanState(null);if(document.pictureInPictureElement)document.exitPictureInPicture().catch(()=>{});return previousClosePlayer()};
  const closeButton=document.querySelector('#player-close');if(closeButton)closeButton.onclick=closePlayer;

  window.sfEnsureWebAudioCompatibility=function(){return Promise.resolve(activePlan)};
  window.sfTogglePictureInPicture=togglePictureInPicture;
  window.sfPlaybackCore={start,capabilities:browserCapabilities,localDecodeCapabilities:browserLocalDecodeCapabilities,currentPlan:()=>activePlan,sessionID:()=>String(activePlan?.playback_session_id||''),currentQuality:()=>effectiveQuality(activePlan,preferredQuality),preferredQuality:()=>preferredQuality,availableQualities:()=>availableQualities(activePlan),currentAudioStream:()=>activeAudioStream,setQuality,setAudioStream,togglePictureInPicture};
})();
