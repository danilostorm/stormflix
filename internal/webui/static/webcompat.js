/* StormFlix Web Compatibility Mode */
(function(){
  let attemptedMediaID=0;
  let remuxActive=false;
  let audioFallbackActive=false;
  let triedVersions=new Set();
  let playbackGeneration=0;

  function currentItem(){
    if(typeof sfCurrentMedia!=='undefined'&&sfCurrentMedia)return sfCurrentMedia;
    return typeof currentDetail!=='undefined'?currentDetail:null;
  }

  function setPlaybackMode(mode,plan){
    window.sfPlaybackMode=mode||'direct_play';
    if(plan)window.sfLastCompatibilityPlan=plan;
  }

  function setHelp(message,visible=true){
    const help=document.querySelector('#player-help');
    if(!help)return;
    if(message)help.textContent=message;
    help.classList.toggle('hidden',!visible);
  }

  const basePlayMedia=playMedia;
  playMedia=function(item){
    attemptedMediaID=0;
    remuxActive=false;
    audioFallbackActive=false;
    triedVersions=new Set([Number(item.id)]);
    window.sfLastCompatibilityPlan=null;
    setPlaybackMode('direct_play');
    const generation=++playbackGeneration;
    basePlayMedia(item);
    autoAudioCompatibility(item,generation);
  };

  async function autoAudioCompatibility(item,generation){
    if(!item?.id)return null;
    try{
      const plan=await request(`/media/${Number(item.id)}/compatibility?audio=aac`);
      if(generation!==playbackGeneration)return plan;
      const active=currentItem();
      if(!active||Number(active.id)!==Number(item.id))return plan;
      if(document.querySelector('#player-modal')?.classList.contains('hidden'))return plan;
      window.sfLastCompatibilityPlan=plan;
      if(plan.available&&plan.audio_transcode){
        if(typeof sfToast==='function')sfToast('Áudio não-AAC detectado · preparando compatibilidade');
        await useCompatibility(item,false,plan);
      }
      return plan;
    }catch{return null}
  }

  window.sfEnsureWebAudioCompatibility=function(){
    const item=currentItem();
    const generation=++playbackGeneration;
    return autoAudioCompatibility(item,generation);
  };

  async function prepareCompatibility(id,plan,manual){
    setHelp(plan.audio_transcode
      ?'Preparando áudio AAC compatível. O vídeo original será mantido sem reencode…'
      :'Preparando MP4 compatível sem reencode…',true);
    const prepared=await request(`/media/${id}/remux/prepare?audio=aac`,{method:'POST',body:'{}'});
    if(!prepared?.ready)throw new Error('O arquivo de compatibilidade não ficou pronto.');
    if(prepared.audio_transcode&&String(prepared.audio_codec||'').toLowerCase()!=='aac')throw new Error('O servidor não confirmou áudio AAC no arquivo preparado.');
    if(!prepared.seekable)throw new Error('O arquivo preparado não ficou seekable.');
    if(manual&&typeof sfToast==='function')sfToast('Compatibilidade pronta');
    return prepared;
  }

  async function useCompatibility(item,manual=false,knownPlan=null){
    if(!item?.id)throw new Error('Mídia não disponível');
    const id=Number(item.id);
    const plan=knownPlan||await request(`/media/${id}/compatibility?audio=aac`);
    window.sfLastCompatibilityPlan=plan;
    if(!plan.available)throw new Error(plan.reason||'Modo compatibilidade não disponível para este arquivo.');

    const oldTime=Number.isFinite(player.currentTime)?player.currentTime:0;
    const wasPlaying=!player.paused;
    remuxActive=true;
    audioFallbackActive=Boolean(plan.audio_transcode);
    setPlaybackMode(audioFallbackActive?'direct_stream_audio_aac':'web_remux',plan);

    const prepared=await prepareCompatibility(id,plan,manual);
    if(audioFallbackActive){
      setHelp('Modo compatibilidade: vídeo original sem reencode; somente o áudio foi convertido para AAC.',manual);
      if(typeof sfToast==='function')sfToast('Áudio compatível AAC · vídeo original');
    }else{
      setHelp(plan.confidence==='conditional'
        ?`Modo Compatibilidade Web: remux. Codec ${String(plan.video_codec||'').toUpperCase()} ainda depende do navegador/OS.`
        :'Modo Compatibilidade Web: apenas reempacotando o arquivo para MP4, sem recodificar.',manual);
      if(typeof sfToast==='function')sfToast('Web Remux · vídeo original');
    }

    const compatibilityURL=prepared.url||`/api/v1/media/${id}/remux?audio=aac`;
    player.src=compatibilityURL.startsWith('/api/')?compatibilityURL:`${api}/media/${id}/remux?audio=aac`;
    player.load();
    player.addEventListener('loadedmetadata',function restore(){
      if(oldTime>0&&Number.isFinite(player.duration)&&oldTime<player.duration)player.currentTime=oldTime;
      if(wasPlaying||manual)player.play().catch(()=>{});
    },{once:true});
    if(!oldTime&&(wasPlaying||manual))player.play().catch(()=>{});
    return plan;
  }

  window.sfUseAACCompatibility=async function(){
    const item=currentItem();
    playbackGeneration++;
    return useCompatibility(item,true);
  };

  window.sfUseOriginalStream=function(){
    const item=currentItem();
    if(!item?.id)return;
    playbackGeneration++;
    const oldTime=Number.isFinite(player.currentTime)?player.currentTime:0;
    const wasPlaying=!player.paused;
    remuxActive=false;
    audioFallbackActive=false;
    attemptedMediaID=0;
    setPlaybackMode('direct_play');
    setHelp('',false);
    player.src=`${api}/media/${item.id}/stream`;
    player.load();
    player.addEventListener('loadedmetadata',function restore(){
      if(oldTime>0&&Number.isFinite(player.duration)&&oldTime<player.duration)player.currentTime=oldTime;
      if(wasPlaying)player.play().catch(()=>{});
    },{once:true});
  };

  player.addEventListener('playing',()=>{
    if(remuxActive)setHelp('',false);
  });

  player.addEventListener('error',async()=>{
    const item=currentItem();
    if(!item?.id)return;

    const currentSrc=String(player.currentSrc||player.src||'');
    if(remuxActive||currentSrc.includes('/remux')){
      showCompatibilityError(audioFallbackActive
        ?'O vídeo foi mantido e o áudio foi convertido para AAC, mas este navegador ainda não conseguiu reproduzir o arquivo.'
        :'O remux foi tentado, mas o codec de vídeo ainda não é suportado pelo navegador.');
      return;
    }

    try{
      const webVersion=await nextRealWebVersion(Number(item.id));
      if(webVersion){
        triedVersions.add(Number(webVersion.id));
        attemptedMediaID=0;
        if(typeof sfCurrentMedia!=='undefined')sfCurrentMedia={...item,...webVersion,id:Number(webVersion.id)};
        setPlaybackMode('direct_play');
        if(typeof sfToast==='function')sfToast(`${webVersion.label||'Versão Web'} · Direct Play`);
        player.src=`${api}/media/${webVersion.id}/stream`;
        player.load();
        player.play().catch(()=>{});
        setTimeout(()=>window.sfEnsureWebAudioCompatibility?.(),0);
        return;
      }

      if(attemptedMediaID===Number(item.id))return;
      attemptedMediaID=Number(item.id);
      if(typeof sfToast==='function')sfToast('Tentando áudio compatível AAC…');
      await useCompatibility(item,false);
    }catch(err){
      showCompatibilityError(`Não foi possível abrir o modo compatibilidade: ${err.message}`);
    }
  });

  async function nextRealWebVersion(currentID){
    let versions=[];
    try{
      versions=typeof sfVersions!=='undefined'&&sfVersions.length?sfVersions:await request(`/media/${currentID}/versions`);
    }catch{return null}
    const candidates=(versions||[]).filter(v=>{
      const id=Number(v.id),ext=String(v.extension||'').toLowerCase();
      return id&&id!==currentID&&!triedVersions.has(id)&&['.mp4','.m4v','.webm'].includes(ext);
    });
    candidates.sort((a,b)=>webRank(b)-webRank(a));
    return candidates[0]||null;
  }

  function webRank(version){
    const label=String(version.label||'').toLowerCase();
    const ext=String(version.extension||'').toLowerCase();
    let rank=ext==='.mp4'?30:ext==='.m4v'?25:20;
    if(label.includes('1080'))rank+=50;
    else if(label.includes('720'))rank+=40;
    else if(label.includes('480'))rank+=30;
    else if(label.includes('1440'))rank+=20;
    else if(label.includes('4k')||label.includes('2160'))rank+=10;
    return rank;
  }

  function showCompatibilityError(message){
    setPlaybackMode('unsupported',window.sfLastCompatibilityPlan||null);
    setHelp(message,true);
  }
})();
