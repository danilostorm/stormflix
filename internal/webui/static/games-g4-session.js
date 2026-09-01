/* StormFlix Games G4 session bridge: safe settings restart + page scroll lock. */
(function(){
  const api=window.StormFlixGamePlayer;if(!api)return;
  api.reload=async function(){
    const game=api.current?.();if(!game?.id)return false;
    try{await api.save?.()}catch{}
    const id=game.id;
    await api.close?.();
    await new Promise(resolve=>setTimeout(resolve,80));
    await api.open?.(id);
    return true;
  };
  const lock=()=>{
    document.body.classList.add('sf-game-playing');
    // games-player creates the button, while games-g4 owns the actual menu.
    // Remove the base onclick dispatcher so one click cannot open/pause twice.
    const menu=document.querySelector('#game-player-overlay [data-game-menu]');
    if(menu)menu.onclick=null;
  };
  const unlock=()=>document.body.classList.remove('sf-game-playing');
  window.addEventListener('stormflix:game-started',lock);
  window.addEventListener('stormflix:game-closed',unlock);
  if(api.active?.())lock();
})();