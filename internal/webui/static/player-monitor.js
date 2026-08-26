/* StormFlix playback heartbeat for Tautulli-style monitoring */
(function(){
  let timer=null,lastMediaID=0,technical={};
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
  function resolution(){return player.videoWidth&&player.videoHeight?`${player.videoWidth}x${player.videoHeight}`:''}
  function audioLanguage(){
    try{const tracks=player.audioTracks?[...player.audioTracks]:[];const active=tracks.find(t=>t.enabled);return active?.language||active?.label||''}catch{return''}
  }
  function subtitleLanguage(){
    try{const tracks=[...player.textTracks];const active=tracks.find(t=>t.mode==='showing');return active?.language||active?.label||''}catch{return''}
  }

  async function loadTechnical(id){
    technical={};
    try{
      const suffix=mode()==='direct_stream_audio_aac'?'?audio=aac':'';
      const plan=await request(`/media/${id}/compatibility${suffix}`);
      technical={video_codec:plan.video_codec||'',audio_codec:plan.audio_codec||'',source_audio_codec:plan.source_audio_codec||''};
    }catch{}
  }

  async function heartbeat(force=false){
    const item=media();if(!item?.id)return;
    if(Number(item.id)!==lastMediaID){lastMediaID=Number(item.id);loadTechnical(lastMediaID)}
    if(!force&&document.hidden)return;
    const body={
      position_seconds:Number.isFinite(player.currentTime)?player.currentTime:0,
      duration_seconds:Number.isFinite(player.duration)?player.duration:0,
      state:state(),mode:mode(),resolution:resolution(),video_codec:technical.video_codec||'',audio_codec:technical.audio_codec||'',audio_language:audioLanguage(),subtitle_language:subtitleLanguage()
    };
    try{await request(`/media/${item.id}/playback`,{method:'POST',body:JSON.stringify(body)})}catch{}
  }

  function start(){clearInterval(timer);heartbeat(true);timer=setInterval(()=>heartbeat(false),10000)}
  function stopTimer(){clearInterval(timer);timer=null}
  async function finish(item){
    if(!item?.id)return;
    await heartbeat(true);
    try{await request(`/media/${item.id}/playback`,{method:'DELETE'})}catch{}
  }

  player.addEventListener('playing',start);
  player.addEventListener('pause',()=>heartbeat(true));
  player.addEventListener('seeked',()=>heartbeat(true));
  player.addEventListener('loadedmetadata',()=>heartbeat(true));
  player.addEventListener('ended',async()=>{const item=media();stopTimer();await finish(item)});
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)heartbeat(true)});

  closePlayer=function(){
    const item=media();stopTimer();finish(item);
    baseClosePlayer();lastMediaID=0;technical={};
  };
  const close=document.querySelector('#player-close');if(close)close.onclick=closePlayer;

  window.addEventListener('beforeunload',()=>{
    const item=media();if(!item?.id)return;
    const body=JSON.stringify({position_seconds:player.currentTime||0,duration_seconds:player.duration||0,state:state(),mode:mode(),resolution:resolution(),video_codec:technical.video_codec||'',audio_codec:technical.audio_codec||'',audio_language:audioLanguage(),subtitle_language:subtitleLanguage()});
    fetch(`${api}/media/${item.id}/playback`,{method:'POST',headers:{'Content-Type':'application/json'},body,credentials:'same-origin',keepalive:true}).catch(()=>{});
  });
})();
