/* StormFlix Web Compatibility Mode */
(function(){
  let attemptedMediaID=0;
  let remuxActive=false;

  const basePlayMedia=playMedia;
  playMedia=function(item){
    attemptedMediaID=0;
    remuxActive=false;
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
    if(attemptedMediaID===Number(item.id))return;
    attemptedMediaID=Number(item.id);

    try{
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

  function showCompatibilityError(message){
    const help=document.querySelector('#player-help');
    if(help){
      help.textContent=message;
      help.classList.remove('hidden');
    }
    setPlaybackBadge('FORMATO NÃO SUPORTADO');
  }
})();
