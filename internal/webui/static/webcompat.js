/* StormFlix Web Compatibility Mode */
(function(){
  let attemptedMediaID=0;
  let remuxActive=false;
  let triedVersions=new Set();

  const basePlayMedia=playMedia;
  playMedia=function(item){
    attemptedMediaID=0;
    remuxActive=false;
    triedVersions=new Set([Number(item.id)]);
    setPlaybackBadge('DIRECT PLAY');
    basePlayMedia(item);
  };

  function setPlaybackBadge(text){
    const badge=document.querySelector('#player-modal .direct-badge');
    if(badge)badge.textContent=text;
  }

  player.addEventListener('playing',()=>{
    if(remuxActive){
      const help=document.querySelector('#player-help');
      if(help)help.classList.add('hidden');
      setPlaybackBadge('WEB REMUX · NO TRANSCODING');
    }
  });

  player.addEventListener('error',async()=>{
    const item=typeof sfCurrentMedia!=='undefined'?sfCurrentMedia:currentDetail;
    if(!item?.id)return;

    const currentSrc=String(player.currentSrc||player.src||'');
    if(remuxActive||currentSrc.includes('/remux')){
      showCompatibilityError('O remux foi tentado, mas este codec ainda não é suportado pelo navegador. Use o StormFlix Desktop, Android ou Android TV para tocar o arquivo original.');
      return;
    }

    try{
      const webVersion=await nextRealWebVersion(Number(item.id));
      if(webVersion){
        triedVersions.add(Number(webVersion.id));
        attemptedMediaID=0;
        if(typeof sfCurrentMedia!=='undefined')sfCurrentMedia={...item,...webVersion,id:Number(webVersion.id)};
        setPlaybackBadge(`${webVersion.label||'WEB'} · DIRECT PLAY`);
        if(typeof sfToast==='function')sfToast(`${webVersion.label||'Versão Web'} · Direct Play`);
        player.src=`${api}/media/${webVersion.id}/stream`;
        player.load();
        player.play().catch(()=>{});
        return;
      }

      if(attemptedMediaID===Number(item.id))return;
      attemptedMediaID=Number(item.id);
      if(typeof sfToast==='function')sfToast('Verificando compatibilidade Web…');
      const plan=await request(`/media/${item.id}/compatibility`);
      if(!plan.available){
        showCompatibilityError(`Direct Play não suportado neste navegador. ${plan.reason||'Remux sem transcodificação não é possível para este arquivo.'}`);
        return;
      }

      remuxActive=true;
      const help=document.querySelector('#player-help');
      if(help){
        help.textContent=plan.confidence==='conditional'
          ?`Modo Compatibilidade Web: remux sem transcodificação. Codec ${String(plan.video_codec||'').toUpperCase()} ainda depende do suporte do navegador/OS.`
          :'Modo Compatibilidade Web ativo: o arquivo está sendo apenas reempacotado para MP4, sem recodificar vídeo ou áudio.';
        help.classList.remove('hidden');
      }
      setPlaybackBadge('WEB REMUX · NO TRANSCODING');
      if(typeof sfToast==='function')sfToast('Web Remux · sem transcodificação');
      player.src=`${api}/media/${item.id}/remux`;
      player.load();
      player.play().catch(()=>{});
    }catch(err){
      showCompatibilityError(`Não foi possível verificar o modo compatibilidade: ${err.message}`);
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
    const help=document.querySelector('#player-help');
    if(help){
      help.textContent=message;
      help.classList.remove('hidden');
    }
    setPlaybackBadge('FORMATO NÃO SUPORTADO');
  }
})();
