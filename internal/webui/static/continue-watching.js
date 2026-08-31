/* StormFlix profile progress UI. Playback resume is owned by PlaybackPlan/Core. */
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

  // Older builds restored item.position_seconds again on loadedmetadata. That
  // second seek could override PlaybackPlan resume, quality recovery and the
  // profile rewind-on-resume rule. The unified Playback Core is authoritative.

  // profiles.js can finish bootstrap before the late player scripts execute.
  // Re-emit the selected profile once so playback-only controllers always get
  // the server-side autoplay preference and the correct profile namespace.
  Promise.resolve().then(async()=>{
    try{
      const data=await request('/profiles');
      const selected=(data?.profiles||[]).find(p=>Number(p.id)===Number(data?.selected_profile_id));
      if(selected)window.dispatchEvent(new CustomEvent('stormflix:profile',{detail:selected}));
    }catch{}
  });
})();
