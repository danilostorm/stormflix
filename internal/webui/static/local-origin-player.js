/* StormFlix Playback Engine v7: original HTTP Range -> local demux/decode. */
(function(){
  'use strict';
  const video=document.querySelector('#player');
  const surface=document.querySelector('#sf-local-origin-surface');
  if(!video||!surface)return;

  const BASE=new URL('/vendor-libmedia/',location.href).href;
  const CODEC_WASM=new Map([
    [27,'h264'],[173,'hevc'],[225,'av1'],[86017,'mp3'],[86018,'aac'],
    [86019,'ac3'],[86020,'dca'],[86021,'vorbis'],[86028,'flac'],
    [86056,'eac3'],[86076,'opus']
  ]);
  const native={
    play:video.play.bind(video),pause:video.pause.bind(video),load:video.load.bind(video)
  };
  let runtimePromise=null;
  let engine=null;
  let firstFrame=false;
  let active=false;
  let generation=0;
  let statsTimer=0;
  let pendingAudioStream=null,pendingResume=0;
  let pausePending=Promise.resolve();
  let captionCues=[],captionNode=null,captionGeneration=0;
  let state={time:0,duration:0,paused:true,volume:1,muted:false,rate:1,width:0,height:0,buffered:0};

  function dispatch(name,detail){
    video.dispatchEvent(detail===undefined?new Event(name):new CustomEvent(name,{detail}));
  }
  function wasmSIMD(){
    try{return WebAssembly.validate(Uint8Array.from(atob('AGFzbQEAAAABBQFgAAF7AhIBA2VudgZtZW1vcnkCAwGAgAIDAgEACgoBCABBAP0ABAAL'),c=>c.charCodeAt(0)))}catch{return false}
  }
  function script(url,marker){
    const attribute='data-'+marker.replace(/[A-Z]/g,c=>'-'+c.toLowerCase());
    const found=document.querySelector(`script[${attribute}]`);
    if(found?.dataset.loaded==='1')return Promise.resolve();
    return new Promise((resolve,reject)=>{
      const node=found||document.createElement('script');
      node.src=url;node.async=true;node.dataset[marker]='1';
      node.onload=()=>{node.dataset.loaded='1';resolve()};
      node.onerror=()=>{node.remove();reject(new Error(`runtime local indisponível: ${url}`))};
      if(!found)document.head.appendChild(node);
    });
  }
  async function ensureRuntime(){
    if(window.AVPlayer)return window.AVPlayer;
    if(runtimePromise)return runtimePromise;
    runtimePromise=(async()=>{
      // Single-thread mode avoids requiring global COOP/COEP headers, keeping
      // Cast, TV bridges and external integrations intact. Workers still split
      // demux/audio/video pipelines and SIMD remains mandatory.
      window.CHEAP_DISABLE_THREAD=true;
      await script(`${BASE}cheap-polyfill.js`,'stormflixCheap');
      await script(`${BASE}avplayer.js`,'stormflixLibmedia');
      if(typeof window.AVPlayer!=='function')throw new Error('libmedia não inicializou');
      return window.AVPlayer;
    })().catch(error=>{runtimePromise=null;throw error});
    return runtimePromise;
  }
  function fakeRanges(){
    const end=Math.max(state.time,state.buffered);
    return{length:end>0?1:0,start:i=>{if(i!==0)throw new DOMException('IndexSizeError');return 0},end:i=>{if(i!==0||end<=0)throw new DOMException('IndexSizeError');return end}};
  }
  function setEngineVolume(){
    try{engine?.setVolume(state.muted?0:state.volume,true)}catch{}
    dispatch('volumechange');
  }
  function installAdapter(){
    if(active)return;
    active=true;
    Object.defineProperties(video,{
      currentTime:{configurable:true,get:currentTime,set:value=>seek(value)},
      duration:{configurable:true,get:()=>state.duration},
      paused:{configurable:true,get:()=>state.paused},
      volume:{configurable:true,get:()=>state.volume,set:value=>{state.volume=Math.max(0,Math.min(1,Number(value)||0));setEngineVolume()}},
      muted:{configurable:true,get:()=>state.muted,set:value=>{state.muted=Boolean(value);setEngineVolume()}},
      playbackRate:{configurable:true,get:()=>state.rate,set:value=>{state.rate=Math.max(.5,Math.min(2,Number(value)||1));try{engine?.setPlaybackRate(state.rate)}catch{}dispatch('ratechange')}},
      videoWidth:{configurable:true,get:()=>state.width},
      videoHeight:{configurable:true,get:()=>state.height},
      buffered:{configurable:true,get:fakeRanges},
      play:{configurable:true,value:play},pause:{configurable:true,value:pause},load:{configurable:true,value:function(){}}
    });
    video.classList.add('sf-native-player-hidden');
    surface.hidden=false;surface.classList.add('active');
  }
  function removeAdapter(){
    if(!active)return;
    for(const key of ['currentTime','duration','paused','volume','muted','playbackRate','videoWidth','videoHeight','buffered','play','pause','load'])delete video[key];
    active=false;video.classList.remove('sf-native-player-hidden');surface.hidden=true;surface.classList.remove('active');
  }
  async function play(){
    if(!engine)return Promise.reject(new Error('player local não carregado'));
    const instance=engine,token=generation;
    await pausePending;
    if(token!==generation)return;
    await instance.resume?.();
    await instance.play();
    if(token!==generation)return;
    // libmedia creates decoder pipelines in play(), not load(). Apply track
    // selection and resume only once those pipelines actually exist.
    if(pendingAudioStream!==null){const index=pendingAudioStream;pendingAudioStream=null;await selectAudio(index)}
    if(token!==generation)return;
    if(pendingResume>0){const resume=pendingResume;pendingResume=0;await instance.seek(BigInt(Math.round(resume*1000)))}
    if(token!==generation)return;
    instance.setSubtitleEnable?.(String(window.sfLocalSubtitleID).startsWith('local:'));
    state.paused=false;dispatch('play');dispatch('playing');
    window.dispatchEvent(new Event('stormflix:local-tracks'));
  }
  function pause(){
    if(!engine)return;
    state.paused=true;pausePending=Promise.resolve(engine.pause()).catch(()=>{});dispatch('pause');
  }
  function currentTime(){
    if(pendingResume>0)return pendingResume;
    try{return engine?Number(engine.currentTime)/1000:state.time}catch{return state.time}
  }
  function seek(seconds){
    const target=Math.max(0,Math.min(state.duration||Infinity,Number(seconds)||0));
    state.time=target;dispatch('seeking');
    if(engine)Promise.resolve(engine.seek(BigInt(Math.round(target*1000)))).catch(error=>dispatch('error',error));
  }
  function bindEvents(instance,AVPlayer,token){
    const Events=AVPlayer.Events||{};
    const on=(event,handler)=>{if(event)instance.on(event,(...args)=>{if(token===generation)handler(...args)})};
    on(Events.LOADED||'loaded',()=>{
      state.duration=Number(instance.getDuration?.()||0n)/1000;
      dispatch('loadedmetadata');dispatch('durationchange');dispatch('loadeddata');dispatch('canplay');
    });
    on(Events.PLAYING||'playing',()=>{state.paused=false;dispatch('playing')});
    on(Events.PLAYED||'played',()=>{state.paused=false;dispatch('play')});
    on(Events.PAUSED||'paused',()=>{state.paused=true;dispatch('pause')});
    on(Events.SEEKING||'seeking',()=>dispatch('seeking'));
    on(Events.SEEKED||'seeked',()=>dispatch('seeked'));
    on(Events.ENDED||'ended',()=>{state.paused=true;dispatch('ended')});
    on(Events.RESUME||'resume',()=>dispatch('playing'));
    on(Events.TIME||'time',milliseconds=>{state.time=Number(milliseconds||0)/1000;state.buffered=Math.max(state.buffered,state.time+3);dispatch('timeupdate');dispatch('progress')});
    on(Events.VOLUME_CHANGE||'volumeChange',()=>dispatch('volumechange'));
    on(Events.FIRST_VIDEO_RENDERED||'firstVideoRendered',()=>{firstFrame=true;dispatch('stormflix:local-origin-first-frame')});
    on(Events.ERROR||'error',error=>{window.sfPlaybackLastError=String(error?.message||error||'Falha no decode local');dispatch('error',error)});
  }
  function wasmURL(type,codecId){
    if(type==='resampler')return`${BASE}resample-simd.wasm`;
    if(type==='stretchpitcher')return`${BASE}stretchpitch-simd.wasm`;
    const name=CODEC_WASM.get(Number(codecId));
    if(!name)throw new Error(`codec local sem módulo permitido: ${codecId}`);
    return`${BASE}${name}-simd.wasm`;
  }
  function startStats(){
    clearInterval(statsTimer);
    statsTimer=setInterval(()=>{
      if(!engine)return;
      // Some libmedia track switches stop TIME events. Read the engine clock
      // so the controls and progress heartbeats continue after audio changes.
      const time=currentTime();
      if(Number.isFinite(time)&&time!==state.time){state.time=time;dispatch('timeupdate')}
      const raw=engine.getStats?.()||{};
      window.sfLocalDecodeStats={engine:'libmedia',transport:'original_range',codec:String(window.sfLastPlaybackPlan?.source_video_codec||''),current_seconds:state.time,duration_seconds:state.duration,dropped_frames:Number(raw.videoFrameDropCount||0),decoded_frames:Number(raw.videoFrameDecodeCount||0),decoded_audio_frames:Number(raw.audioFrameDecodeCount||0),audio_stream:Number(engine.getStreams?.().find(s=>s.id===engine.getSelectedAudioStreamId?.())?.index??-1),buffer_seconds:Math.max(0,state.buffered-state.time),updated_at:Date.now()};
      window.dispatchEvent(new CustomEvent('stormflix:local-decode-stat',{detail:window.sfLocalDecodeStats}));
    },1000);
  }
  function waitForFirstFrame(token){
    if(firstFrame)return Promise.resolve();
    return new Promise((resolve,reject)=>{
      const started=performance.now();
      const timer=setInterval(()=>{
        if(token!==generation){clearInterval(timer);reject(new Error('reprodução cancelada'));return}
        if(firstFrame){clearInterval(timer);resolve();return}
        if(performance.now()-started>20000){clearInterval(timer);const error=new Error('O decoder não apresentou imagem');error.code='NO_VIDEO_FRAME';reject(error)}
      },100);
    });
  }
  async function load(url,plan,options={}){
    const expected=generation+1;
    try{return await loadAttempt(url,plan,options)}catch(error){
      if(error.code!=='NO_VIDEO_FRAME'||generation!==expected||options.software)throw error;
      // A working audio clock does not prove that WebCodecs rendered video.
      return loadAttempt(url,plan,{...options,software:true});
    }
  }
  async function loadAttempt(url,plan,options={}){
    const cleanup=destroy();
    firstFrame=false;
    loadedMediaID=Number(plan?.media_id||0);
    const token=generation;
    await cleanup;
    if(token!==generation)throw new Error('inicialização local cancelada');
    if(!wasmSIMD())throw new Error('WebAssembly SIMD indisponível');
    const AVPlayer=await ensureRuntime();
    if(token!==generation)throw new Error('inicialização local cancelada');
    try{native.pause();video.removeAttribute('src');native.load()}catch{}
    state={time:0,duration:0,paused:true,volume:Number(video.volume??1),muted:Boolean(video.muted),rate:1,width:Number(plan?.video_width||0),height:Number(plan?.video_height||0),buffered:0};
    installAdapter();
    const requestedAudio=Number.isInteger(options.audioStream)?options.audioStream:Number(plan?.audio_stream);
    const instance=new AVPlayer({
      container:surface,getWasm:wasmURL,checkUseMSE:()=>false,
      enableHardware:!options.software,enableWebCodecs:!options.software,enableWebGPU:!options.software&&Boolean(navigator.gpu),enableWorker:true,enableAudioWorklet:Boolean(window.isSecureContext&&window.AudioWorkletNode),
      lowLatency:false,preLoadTime:3,audioWorkletBufferLength:14
    });
    engine=instance;
    bindEvents(instance,AVPlayer,token);setEngineVolume();startStats();
    // Blob workers have no page-relative base URL. Resolve media and sidecars
    // before handing them to libmedia's network worker.
    const source=new URL(url,location.href).href;
    await instance.load(source,{ext:String(plan?.source_container||'').replace(/^matroska$/,'mkv'),http:{credentials:'same-origin'}});
    if(token!==generation)throw new Error('carregamento local cancelado');
    state.duration=Number(instance.getDuration?.()||0n)/1000;
    // The constructor callback receives ALL raw streams, not typed streams.
    // Let libmedia choose valid audio/video first, then apply an explicit track.
    pendingAudioStream=Number.isInteger(requestedAudio)&&requestedAudio>=0?requestedAudio:null;
    window.sfLocalSubtitleID=0;
    pendingResume=Math.max(0,Number(options.resume)||0);
    if(options.autoplay!==false){await play();await waitForFirstFrame(token)}
    return true;
  }
  async function selectAudio(index){
    const token=generation;
    await pausePending;
    if(!engine||token!==generation)return false;
    if(engine.getSelectedAudioStreamId?.()<0){pendingAudioStream=index;return true}
    const streams=engine.getStreams?.()||[];
    const stream=streams.find(s=>(s.mediaType==='audio'||Number(s?.codecparProxy?.codecType)===1)&&(Number(s.index)===Number(index)||Number(s.id)===Number(index)));
    await engine.selectAudio(Number(stream?.id??index));
    return true;
  }
  async function selectSubtitle(id){
    if(!engine)return false;
    if(!id||id==='0'){captionGeneration++;captionCues=[];renderCaption();engine.setSubtitleEnable?.(false);window.sfLocalSubtitleID=0;return true}
    const token=++captionGeneration;
    captionCues=[];renderCaption();
    const rows=typeof sfSubtitles!=='undefined'&&Array.isArray(sfSubtitles)?sfSubtitles:[];
    if(!String(id).startsWith('local:')){
      const row=rows.find(r=>String(r.id)===String(id));
      if(!row)throw new Error('Faixa de legenda indisponível');
      const mediaID=Number(window.sfLastPlaybackPlan?.media_id||loadedMediaID);
      const response=await fetch(`/api/v1/media/${mediaID}/subtitles/${Number(row.id)}/vtt`,{credentials:'same-origin'});
      if(!response.ok)throw new Error('Não foi possível carregar a legenda');
      const text=await response.text();if(token!==captionGeneration)return false;
      const cues=parseVTT(text);if(!cues.length)throw new Error('A legenda não contém texto compatível');captionCues=cues;engine.setSubtitleEnable?.(false);window.sfLocalSubtitleID=id;renderCaption();return true;
    }
    const streams=(engine.getStreams?.()||[]).filter(s=>s.mediaType==='subtitle'||Number(s?.codecparProxy?.codecType)===3);
    const stream=streams.find(s=>String(s.id)===String(id).slice(6));
    if(!stream)throw new Error('Faixa de legenda indisponível');
    await engine.selectSubtitle(Number(stream.id));
    if(token!==captionGeneration)return false;
    if(engine.getSelectedSubtitleStreamId?.()!==stream.id)throw new Error('O decoder não selecionou a legenda');
    engine.setSubtitleEnable?.(true);window.sfLocalSubtitleID=id;return true;
  }

  let loadedMediaID=0;
  function parseVTT(text){
    const time=value=>value.split(':').reduce((total,part)=>total*60+Number(part.replace(',','.')),0);
    return String(text).replace(/\r/g,'').split(/\n\s*\n/).flatMap(block=>{
      const lines=block.split('\n'),index=lines.findIndex(l=>l.includes('-->'));if(index<0)return[];
      const m=lines[index].match(/([\d:.,]+)\s+-->\s+([\d:.,]+)/);if(!m)return[];
      const doc=new DOMParser().parseFromString(lines.slice(index+1).join('\n'),'text/html');
      return[{start:time(m[1]),end:time(m[2]),text:doc.body.textContent||''}];
    });
  }
  function renderCaption(){
    if(!active&&!captionNode)return;
    if(!captionNode){captionNode=document.createElement('div');captionNode.id='sf-external-caption';captionNode.style.cssText='position:absolute;bottom:18%;left:8%;right:8%;z-index:12;text-align:center;white-space:pre-line;color:white;font:600 clamp(18px,2.3vw,32px)/1.35 sans-serif;text-shadow:0 2px 4px black,1px 0 2px black;pointer-events:none';(document.querySelector('#player-modal')||surface.parentElement).appendChild(captionNode)}
    const time=currentTime();captionNode.textContent=captionCues.filter(c=>time>=c.start&&time<c.end).map(c=>c.text).join('\n');captionNode.hidden=!captionNode.textContent;
  }
  video.addEventListener('timeupdate',renderCaption);video.addEventListener('seeked',renderCaption);
  async function destroy(){
    captionGeneration++;captionCues=[];if(captionNode){captionNode.remove();captionNode=null}
    generation++;clearInterval(statsTimer);statsTimer=0;pendingAudioStream=null;pendingResume=0;pausePending=Promise.resolve();
    const old=engine;engine=null;
    // Synchronous detach prevents an older destroy from removing a new player.
    surface.replaceChildren();removeAdapter();
    window.sfLocalDecodeStats=null;window.sfLocalSubtitleID=0;
    if(old){try{await old.destroy()}catch{}}
  }

  function subtitleTracks(){
    const embedded=(engine?.getStreams?.()||[]).filter(s=>s.mediaType==='subtitle'||Number(s?.codecparProxy?.codecType)===3).map(s=>({id:'local:'+s.id,label:s.metadata?.title||s.metadata?.language||'Legenda '+(s.index+1)}));
    const sidecars=typeof sfSubtitles!=='undefined'&&Array.isArray(sfSubtitles)?sfSubtitles:[];
    return [...embedded,...sidecars.map(s=>({id:s.id,label:s.language||'Legenda externa'}))];
  }
  function unlockAudio(){return engine?.resume?.()}
  for(const event of ['pointerdown','keydown'])document.addEventListener(event,()=>{if(active&&engine?.isSuspended?.())void unlockAudio()},true);
  window.sfLocalOrigin={load,destroy,selectAudio,selectSubtitle,subtitleTracks,unlockAudio,isAudioSuspended:()=>Boolean(engine?.isSuspended?.()),isActive:()=>active,isSupported:wasmSIMD};
})();
