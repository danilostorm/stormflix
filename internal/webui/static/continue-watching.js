/* StormFlix profile progress UI. */
(function(){
  const baseCardHTML=cardHTML;
  cardHTML=function(item){
    let html=baseCardHTML(item);
    const progress=Math.max(0,Math.min(100,Number(item?.progress_percent||0)));
    if(progress>0&&progress<92){
      html=html.replace('<div class="tile-shade"></div>',`<div class="tile-shade"></div><div class="watch-progress" aria-label="${Math.round(progress)}% assistido"><span style="width:${progress.toFixed(1)}%"></span></div>`);
    }
    return html;
  };

  const basePlayMedia=playMedia;
  playMedia=function(item){
    const resume=Number(item?.position_seconds||0);
    if(resume>=30){
      player.addEventListener('loadedmetadata',function resumePosition(){
        if(Number.isFinite(player.duration)&&resume<player.duration-15){
          player.currentTime=resume;
          if(typeof sfToast==='function')sfToast(`Continuando em ${formatResume(resume)}`);
        }
      },{once:true});
    }
    return basePlayMedia(item);
  };

  function formatResume(seconds){
    seconds=Math.floor(seconds||0);const h=Math.floor(seconds/3600),m=Math.floor(seconds%3600/60);
    return h?`${h}h ${m}min`:`${m}min`;
  }
})();
