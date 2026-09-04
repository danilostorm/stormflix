/* StormFlix Playback Engine v7: original HTTP Range -> local demux/decode. */
(function(){
  'use strict';
  const video=document.querySelector('#player');
  const surface=document.querySelector('#sf-local-origin-surface');
  if(!video||!surface)return;

  const BASE='/vendor-libmedia/';
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
  let active=false;
  let generation=0;
  let statsTimer=0;
  let state={time:0,duration:0,paused:true,volume:1,muted:false,rate:1,width:0,height:0,buffered:0};

  function dispatch(name,detail){
    video.dispatchEvent(detail===undefined?new Event(name):new CustomEvent(name,{detail}));
  }
  function wasmSIMD(){
    try{return WebAssembly.validate(Uint8Array.from(atob('AGFzbQEAAAABBQFgAAF7AhIBA2VudgZtZW1vcnkCAwGAgAIDAgEACgoBCABBAP0ABAAL'),c=>c.charCodeAt(0)))}catch{return false}
  }
  function script(url,marker){
    const found=document.querySelector(`script[data-${marker}]`);
    if(found?.dataset.loaded==='1')return Promise.resolve();
    return new Promise((resolve,reject)=>{
      const node=found||document.createElement('script');
      node.src=url;node.async=true;node.dataset[marker]='1';
      node.onload=()=>{node.dataset.loaded='1';resolve()};
      node.onerror=()=>reject(new Error(`runtime local indisponível: ${url}`));
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
      currentTime:{configurable:true,get:()=>state.time,set:value=>seek(value)},
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
    await engine.play();state.paused=false;dispatch('play');dispatch('playing');
  }
  function pause(){
    if(!engine)return;
    state.paused=true;Promise.resolve(engine.pause()).catch(()=>{});dispatch('pause');
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
    on(Events.FIRST_VIDEO_RENDERED||'firstVideoRendered',()=>dispatch('stormflix:local-origin-first-frame'));
    on(Events.ERROR||'error',error=>{window.sfPlaybackLastError=String(error?.message||error||'Falha no decode local');dispatch('error',error)});
  }
  function wasmURL(type,codecId){
    if(type==='resampler')return`${BASE}resample-simd.wasm`;
    if(type==='stretchpitcher')return`${BASE}stretchpitch-simd.wasm`;
    const name=CODEC_WASM.get(Number(codecId));
    if(!name)throw new Error(`codec local sem módulo permitido: ${codecId}`);
    return`${BASE}${name}-simd.wasm`;
  }
  function bestStream(streams,mediaType,audioStream){
    if(!Array.isArray(streams)||!streams.length)return undefined;
    if(Number(mediaType)===1&&Number.isInteger(audioStream))return streams.find(s=>Number(s.index)===audioStream||Number(s.id)===audioStream)||streams[0];
    return streams.find(s=>Boolean(s?.disposition?.default))||streams[0];
  }
  function startStats(){
    clearInterval(statsTimer);
    statsTimer=setInterval(()=>{
      if(!engine)return;
      const raw=engine.getStats?.()||{};
      window.sfLocalDecodeStats={engine:'libmedia',transport:'original_range',codec:String(window.sfLastPlaybackPlan?.source_video_codec||''),current_seconds:state.time,duration_seconds:state.duration,dropped_frames:Number(raw.videoFrameDrop||raw.video_frame_drop||0),decoded_frames:Number(raw.videoFrameDecode||raw.video_frame_decode||0),buffer_seconds:Math.max(0,state.buffered-state.time),updated_at:Date.now()};
      window.dispatchEvent(new CustomEvent('stormflix:local-decode-stat',{detail:window.sfLocalDecodeStats}));
    },2000);
  }
  async function load(url,plan,options={}){
    const token=++generation;
    await destroy();generation=token;
    if(!wasmSIMD())throw new Error('WebAssembly SIMD indisponível');
    const AVPlayer=await ensureRuntime();
    if(token!==generation)throw new Error('inicialização local cancelada');
    try{native.pause();video.removeAttribute('src');native.load()}catch{}
    state={time:0,duration:0,paused:true,volume:Number(video.volume||1),muted:Boolean(video.muted),rate:1,width:Number(plan?.video_width||0),height:Number(plan?.video_height||0),buffered:0};
    installAdapter();
    const requestedAudio=Number.isInteger(options.audioStream)?options.audioStream:Number(plan?.audio_stream);
    engine=new AVPlayer({
      container:surface,getWasm:wasmURL,checkUseMSE:()=>false,
      enableHardware:true,enableWebCodecs:true,enableWebGPU:Boolean(navigator.gpu),enableWorker:true,enableAudioWorklet:true,
      lowLatency:false,preLoadTime:3,audioWorkletBufferLength:14,
      findBestStream:(streams,mediaType)=>bestStream(streams,mediaType,requestedAudio)
    });
    bindEvents(engine,AVPlayer,token);setEngineVolume();startStats();
    const subtitleRows=typeof sfSubtitles!=='undefined'&&Array.isArray(sfSubtitles)?sfSubtitles:[];
    const externalSubtitles=subtitleRows.map(row=>({source:`/api/v1/media/${Number(plan?.media_id)}/subtitles/${Number(row.id)}/vtt`,lang:String(row.language||''),title:String(row.provider||row.language||'Legenda')}));
    await engine.load(url,{ext:String(plan?.source_container||'').replace(/^matroska$/,'mkv'),externalSubtitles,http:{credentials:'same-origin'}});
    if(token!==generation)throw new Error('carregamento local cancelado');
    state.duration=Number(engine.getDuration?.()||0n)/1000;
    engine.setSubtitleEnable?.(false);window.sfLocalSubtitleID=0;
    if(Number(options.resume)>0)await engine.seek(BigInt(Math.round(Number(options.resume)*1000)));
    if(options.autoplay!==false)await play();
    return true;
  }
  async function selectAudio(index){
    if(!engine)return false;
    const streams=engine.getStreams?.()||[];
    const stream=streams.find(s=>(s.mediaType==='audio'||Number(s?.codecparProxy?.codecType)===1)&&(Number(s.index)===Number(index)||Number(s.id)===Number(index)));
    await engine.selectAudio(Number(stream?.id??index));
    return true;
  }
  async function selectSubtitle(id){
    if(!engine)return false;
    id=Number(id||0);window.sfLocalSubtitleID=id;
    if(!id){engine.setSubtitleEnable?.(false);return true}
    const rows=typeof sfSubtitles!=='undefined'&&Array.isArray(sfSubtitles)?sfSubtitles:[];
    const rowIndex=rows.findIndex(row=>Number(row.id)===id);
    const streams=(engine.getStreams?.()||[]).filter(s=>s.mediaType==='subtitle'||Number(s?.codecparProxy?.codecType)===3);
    const externalCount=rows.length;
    const stream=rowIndex>=0?streams[Math.max(0,streams.length-externalCount)+rowIndex]:undefined;
    if(stream)await engine.selectSubtitle(Number(stream.id));
    engine.setSubtitleEnable?.(true);return true;
  }
  async function destroy(){
    generation++;clearInterval(statsTimer);statsTimer=0;
    const old=engine;engine=null;
    if(old){try{await old.destroy()}catch{}}
    surface.replaceChildren();removeAdapter();
    window.sfLocalDecodeStats=null;window.sfLocalSubtitleID=0;
  }

  window.sfLocalOrigin={load,destroy,selectAudio,selectSubtitle,isActive:()=>active,isSupported:wasmSIMD};
})();
