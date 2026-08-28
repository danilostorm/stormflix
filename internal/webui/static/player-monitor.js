/* StormFlix playback heartbeat for Tautulli-style monitoring */
(function(){
  let timer=null,lastMediaID=0,lastMode='',technical={},progressSequence=0,lastSession='';
  const baseClosePlayer=closePlayer;

  function media(){return typeof sfCurrentMedia!=='undefined'?sfCurrentMedia:null}
  function mode(){
    const explicit=String(window.sfPlaybackMode||'').trim();
    if(explicit)return explicit;
    const src=String(player.currentSrc||player.src||'').toLowerCase();
    if(src.includes('/remux')&&src.includes('audio=aac'))return'direct_stream_audio_aac';
    if(src.includes('/remux'))return'web_remux';
    return'direct_play';
  }
  function state(){return player.paused?'paused':'playing'}
  function sessionID(){return String(window.sfPlaybackSessionID||'').trim()}
  function resolution(){return player.videoWidth&&player.videoHeight?`${player.videoWidth}x${player.videoHeight}`:''}
  function audioLanguage(){
    try{const tracks=player.audioTracks?[...player.audioTracks]:[];const active=tracks.find(t=>t.enabled);return active?.language||active?.label||''}catch{return''}
  }
  function subtitleLanguage(){
    try{const tracks=[...player.textTracks];const active=tracks.find(t=>t.mode==='showing');return active?.language||active?.label||''}catch{return''}
  }

  async function loadTechnical(id,currentMode){
    technical={};
    const planned=window.sfLastPlaybackPlan;
    if(planned&&Number(planned.media_id)===Number(id)){
      technical={video_codec:planned.video_codec||'',audio_codec:planned.audio_codec||'',source_audio_codec:planned.source_audio_codec||''};
      return;
    }
    try{
      const suffix=currentMode==='direct_stream_audio_aac'?'?audio=aac':'';
      const plan=await request(`/media/${id}/compatibility${suffix}`);
      technical={video_codec:plan.video_codec||'',audio_codec:plan.audio_codec||'',source_audio_codec:plan.source_audio_codec||''};
    }catch{}
  }

  function orderedFields(reason){
    const session=sessionID();
    if(!session)return{playback_session_id:'',progress_sequence:0,progress_event_ms:0,progress_reason:reason||'periodic'};
    if(session!==lastSession){lastSession=session;progressSequence=0}
    progressSequence++;
    return{playback_session_id:session,progress_sequence:progressSequence,progress_event_ms:Date.now(),progress_reason:reason||'periodic'};
  }

  async function heartbeat(force=false,reason='periodic'){
    const item=media();if(!item?.id)return;
    const currentMode=mode(),currentSession=sessionID();
    if(Number(item.id)!==lastMediaID||currentMode!==lastMode||currentSession!==lastSession){
      lastMediaID=Number(item.id);lastMode=currentMode;
      if(currentSession!==lastSession){lastSession=currentSession;progressSequence=0}
      loadTechnical(lastMediaID,currentMode);
    }
    if(!force&&document.hidden)return;
    const body={
      position_seconds:Number.isFinite(player.currentTime)?player.currentTime:0,
      duration_seconds:Number.isFinite(player.duration)?player.duration:0,
      state:state(),mode:currentMode,resolution:resolution(),video_codec:technical.video_codec||'',audio_codec:technical.audio_codec||'',source_audio_codec:technical.source_audio_codec||'',audio_language:audioLanguage(),subtitle_language:subtitleLanguage(),
      ...orderedFields(reason)
    };
    try{await request(`/media/${item.id}/playback`,{method:'POST',body:JSON.stringify(body)})}catch{}
  }

  function start(){clearInterval(timer);heartbeat(true,'playing');timer=setInterval(()=>heartbeat(false,'periodic'),10000)}
  function stopTimer(){clearInterval(timer);timer=null}
  async function finish(item,reason='stop'){
    if(!item?.id)return;
    await heartbeat(true,reason);
    try{await request(`/media/${item.id}/playback`,{method:'DELETE'})}catch{}
  }

  player.addEventListener('playing',start);
  player.addEventListener('pause',()=>heartbeat(true,'pause'));
  player.addEventListener('seeked',()=>heartbeat(true,'seeked'));
  player.addEventListener('loadedmetadata',()=>heartbeat(true,'ready'));
  player.addEventListener('ended',async()=>{const item=media();stopTimer();await finish(item,'ended')});
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)heartbeat(true,'visible')});

  closePlayer=function(){
    const item=media();stopTimer();finish(item,'stop');
    baseClosePlayer();lastMediaID=0;lastMode='';technical={};progressSequence=0;lastSession='';
  };
  const close=document.querySelector('#player-close');if(close)close.onclick=closePlayer;

  window.addEventListener('beforeunload',()=>{
    const item=media();if(!item?.id)return;
    const fields=orderedFields('unload');
    const body=JSON.stringify({position_seconds:player.currentTime||0,duration_seconds:player.duration||0,state:state(),mode:mode(),resolution:resolution(),video_codec:technical.video_codec||'',audio_codec:technical.audio_codec||'',source_audio_codec:technical.source_audio_codec||'',audio_language:audioLanguage(),subtitle_language:subtitleLanguage(),...fields});
    fetch(`${api}/media/${item.id}/playback`,{method:'POST',headers:{'Content-Type':'application/json'},body,credentials:'same-origin',keepalive:true}).catch(()=>{});
  });
})();

/* Music uses the same Reproduzindo agora monitor as movies and series. */
(function(){
  let audio=null,timer=null,currentID=0,track={};

  function trackID(){
    if(!audio)return 0;
    const src=String(audio.currentSrc||audio.src||'');
    const m=src.match(/\/music\/tracks\/(\d+)\/stream/i);
    return m?Number(m[1]):0;
  }

  async function loadTrack(id){
    track={};
    try{track=await request(`/music/tracks/${id}`)||{}}catch{}
  }

  async function selectCurrent(){
    const id=trackID();
    if(!id)return 0;
    if(id!==currentID){
      const previous=currentID;
      currentID=id;
      if(previous){try{await request(`/media/${previous}/playback`,{method:'DELETE'})}catch{}}
      await loadTrack(id);
    }
    return id;
  }

  async function heartbeat(){
    if(!audio)return;
    const id=await selectCurrent();if(!id)return;
    const duration=Number.isFinite(audio.duration)?audio.duration:Number(track.duration_seconds)||0;
    const position=Number.isFinite(audio.currentTime)?audio.currentTime:0;
    const body={
      position_seconds:position,
      duration_seconds:duration,
      state:audio.paused?'paused':'playing',
      mode:'music',
      audio_codec:track.codec||'',
      source_audio_codec:track.codec||'',
      bitrate_kbps:Math.max(0,Math.round((Number(track.bitrate)||0)/1000))
    };
    try{await request(`/media/${id}/playback`,{method:'POST',body:JSON.stringify(body)})}catch{}
  }

  async function finish(){
    const id=currentID||trackID();
    clearInterval(timer);timer=null;
    if(!id)return;
    try{await request(`/media/${id}/playback`,{method:'DELETE'})}catch{}
    currentID=0;track={};
  }

  function start(){clearInterval(timer);heartbeat();timer=setInterval(heartbeat,10000)}

  function attach(el){
    if(!el||el.dataset.sfMonitoring==='1')return;
    el.dataset.sfMonitoring='1';audio=el;
    el.addEventListener('play',start);
    el.addEventListener('pause',heartbeat);
    el.addEventListener('seeked',heartbeat);
    el.addEventListener('loadedmetadata',heartbeat);
    el.addEventListener('ended',finish);
  }

  attach(document.querySelector('#music-audio'));
  new MutationObserver(()=>attach(document.querySelector('#music-audio'))).observe(document.body,{childList:true,subtree:true});
})();
