/* StormFlix Web Player v4 — native streaming-style presentation over Playback Core. */
(function(){
  const modal=document.querySelector('#player-modal');
  if(!modal)return;
  const ui=modal.querySelector('.sf-player-ui');
  if(!ui||ui.dataset.sfV4==='1')return;
  ui.dataset.sfV4='1';
  modal.classList.add('sf-player-v4');

  let neighbors={previous:null,next:null,series_title:''};
  let neighborGeneration=0;
  let scrubVisible=false;

  const icon={
    play:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5v14l11-7z"/></svg>',
    pause:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 5h4v14H6zm8 0h4v14h-4z"/></svg>',
    back:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11 8V5l-5 4 5 4v-3c3.3 0 5.5 1.4 7 4.1-.6-4-3-6.1-7-6.1z"/><text x="8.1" y="20" font-size="7" font-family="system-ui">10</text></svg>',
    forward:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M13 8V5l5 4-5 4v-3c-3.3 0-5.5 1.4-7 4.1.6-4 3-6.1 7-6.1z"/><text x="8.1" y="20" font-size="7" font-family="system-ui">10</text></svg>',
    volume:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 9v6h4l5 4V5L8 9H4zm11.5-.5v7c1.5-.9 2.5-2.2 2.5-3.5s-1-2.6-2.5-3.5zm0-4v2c3.1 1.1 5 3 5 5.5s-1.9 4.4-5 5.5v2c4.2-1.2 7-4 7-7.5s-2.8-6.3-7-7.5z"/></svg>',
    muted:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 9v6h4l5 4V5L8 9H4zm12.2 1.2L19 13l2.8-2.8 1.2 1.2-2.8 2.8L23 17l-1.2 1.2-2.8-2.8-2.8 2.8L15 17l2.8-2.8-2.8-2.8z"/></svg>',
    cc:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 5h18v14H3V5zm6.5 5.2c-.5-.6-1.1-.9-1.9-.9-1.6 0-2.6 1.1-2.6 2.7s1 2.7 2.6 2.7c.8 0 1.5-.3 1.9-.9l-1.1-.8c-.2.3-.5.5-.9.5-.7 0-1.1-.6-1.1-1.5s.4-1.5 1.1-1.5c.4 0 .7.2.9.5l1.1-.8zm7 0c-.5-.6-1.1-.9-1.9-.9-1.6 0-2.6 1.1-2.6 2.7s1 2.7 2.6 2.7c.8 0 1.5-.3 1.9-.9l-1.1-.8c-.2.3-.5.5-.9.5-.7 0-1.1-.6-1.1-1.5s.4-1.5 1.1-1.5c.4 0 .7.2.9.5l1.1-.8z"/></svg>',
    audio:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v10.6a4 4 0 1 0 2 3.4V8h5V3h-7z"/></svg>',
    settings:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M19.4 13a7.8 7.8 0 0 0 .1-1 7.8 7.8 0 0 0-.1-1l2.1-1.6-2-3.4-2.5 1a8 8 0 0 0-1.7-1l-.4-2.7h-4l-.4 2.7a8 8 0 0 0-1.7 1l-2.5-1-2 3.4L6.4 11a7.8 7.8 0 0 0-.1 1 7.8 7.8 0 0 0 .1 1l-2.1 1.6 2 3.4 2.5-1a8 8 0 0 0 1.7 1l.4 2.7h4l.4-2.7a8 8 0 0 0 1.7-1l2.5 1 2-3.4L19.4 13zM13 15.5A3.5 3.5 0 1 1 13 8a3.5 3.5 0 0 1 0 7.5z"/></svg>',
    pip:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 5h18v14H3V5zm2 2v10h14V7H5zm7 4h6v5h-6v-5z"/></svg>',
    fullscreen:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 4h6v2H6v4H4V4zm10 0h6v6h-2V6h-4V4zM4 14h2v4h4v2H4v-6zm14 0h2v6h-6v-2h4v-4z"/></svg>',
    previous:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 5h2v14H6V5zm3 7 10 7V5L9 12z"/></svg>',
    next:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M16 5h2v14h-2V5zM5 5v14l10-7L5 5z"/></svg>',
    close:'<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6.7 5.3 12 10.6l5.3-5.3 1.4 1.4-5.3 5.3 5.3 5.3-1.4 1.4-5.3-5.3-5.3 5.3-1.4-1.4 5.3-5.3-5.3-5.3z"/></svg>'
  };

  ui.innerHTML=`
    <div class="sf-v4-topbar">
      <button id="sf-v4-close" class="sf-v4-round" type="button" aria-label="Voltar para o catálogo" title="Voltar">${icon.close}</button>
      <div class="sf-v4-heading">
        <strong id="sf-v4-title">StormFlix</strong>
        <span id="sf-v4-context">Reproduzindo</span>
      </div>
      <div class="sf-v4-status">
        <span id="sf-v4-plan" class="sf-v4-chip">Direct Play</span>
        <span id="sf-v4-resolution" class="sf-v4-chip sf-v4-chip-muted"></span>
      </div>
    </div>

    <div class="sf-player-center sf-v4-center">
      <button class="sf-skip sf-v4-center-skip" id="sf-back10" type="button" aria-label="Voltar 10 segundos" title="Voltar 10 segundos">${icon.back}</button>
      <button class="sf-center-play sf-v4-center-play" id="sf-center-play" type="button" aria-label="Reproduzir ou pausar">${icon.play}</button>
      <button class="sf-skip sf-v4-center-skip" id="sf-forward10" type="button" aria-label="Avançar 10 segundos" title="Avançar 10 segundos">${icon.forward}</button>
    </div>

    <div class="sf-player-controls sf-v4-controls">
      <div class="sf-v4-now">
        <div><strong id="sf-v4-now-title">StormFlix</strong><span id="sf-v4-now-subtitle"></span></div>
        <span id="sf-v4-playback-detail"></span>
      </div>
      <div class="sf-progress-wrap sf-v4-progress-wrap">
        <span id="sf-time">0:00</span>
        <div class="sf-v4-progress-shell">
          <input id="sf-progress" class="sf-progress sf-v4-progress" type="range" min="0" max="1000" value="0" aria-label="Posição do vídeo">
          <div id="sf-v4-scrub-preview" class="sf-v4-scrub-preview">0:00</div>
        </div>
        <span id="sf-duration">0:00</span>
      </div>
      <div class="sf-controls-row sf-v4-controls-row">
        <button class="sf-control-btn sf-v4-control-primary" id="sf-play" type="button" aria-label="Reproduzir ou pausar" title="Reproduzir/Pausar">${icon.play}</button>
        <button class="sf-control-btn" id="sf-back" type="button" aria-label="Voltar 10 segundos" title="Voltar 10s">${icon.back}</button>
        <button class="sf-control-btn" id="sf-forward" type="button" aria-label="Avançar 10 segundos" title="Avançar 10s">${icon.forward}</button>
        <div class="sf-v4-volume-group">
          <button class="sf-control-btn" id="sf-mute" type="button" aria-label="Mudo" title="Volume">${icon.volume}</button>
          <input id="sf-volume" class="sf-volume" type="range" min="0" max="1" step="0.02" value="1" aria-label="Volume">
        </div>
        <div class="sf-v4-episode-controls">
          <button class="sf-control-btn hidden" id="sf-v4-previous" type="button" aria-label="Episódio anterior" title="Episódio anterior">${icon.previous}</button>
          <button class="sf-control-btn hidden" id="sf-v4-next" type="button" aria-label="Próximo episódio" title="Próximo episódio">${icon.next}</button>
        </div>
        <div class="sf-control-spacer"></div>
        <button class="sf-control-btn" id="sf-v4-audio" type="button" aria-label="Áudio" title="Áudio">${icon.audio}<span class="sf-v4-control-label">Áudio</span></button>
        <button class="sf-control-btn" id="sf-subtitle" type="button" aria-label="Legendas" title="Legendas">${icon.cc}</button>
        <button class="sf-control-btn sf-v4-quality" id="sf-settings" type="button" aria-label="Qualidade e configurações" title="Qualidade e configurações"><span id="sf-v4-quality-label">AUTO</span>${icon.settings}</button>
        <button class="sf-control-btn" id="sf-v4-pip" type="button" aria-label="Picture in Picture" title="Picture in Picture">${icon.pip}</button>
        <button class="sf-control-btn" id="sf-fullscreen" type="button" aria-label="Tela cheia" title="Tela cheia">${icon.fullscreen}</button>
      </div>
    </div>
    <div id="sf-player-settings-panel" class="sf-player-settings sf-v4-settings hidden"></div>
    <div id="sf-player-toast" class="sf-player-toast"></div>`;

  const $v=id=>document.getElementById(id);
  const progress=$v('sf-progress');

  function safeText(value){return String(value??'').trim()}
  function modeLabel(){
    const plan=window.sfLastPlaybackPlan||window.sfLastCompatibilityPlan||{};
    const mode=safeText(plan.mode||window.sfPlaybackMode).toLowerCase();
    if(mode==='direct_play')return'Direct Play';
    if(mode==='remux'||mode==='web_remux')return'Remux';
    if(mode==='audio_compatibility'||mode==='direct_stream_audio_aac')return'Áudio AAC';
    return mode&&mode!=='unsupported'?mode.replaceAll('_',' '):'StormFlix';
  }
  function qualityLabel(){
    const plan=window.sfLastPlaybackPlan||{};
    const h=Number(plan.video_height||player.videoHeight||0);
    if(h>=2160)return'4K';
    if(h>=1440)return'1440P';
    if(h>=1080)return'1080P';
    if(h>=720)return'720P';
    return h?`${h}P`:'AUTO';
  }
  function mediaContext(item){
    if(!item)return'';
    const series=safeText(item.series_title||neighbors.series_title);
    const season=Number(item.season_number||0),episode=Number(item.episode_number||0);
    const episodeText=season||episode?`T${season||1}:E${episode||1}`:'';
    return [series,episodeText].filter(Boolean).join(' · ');
  }
  function refreshMetadata(item){
    item=item||(typeof sfCurrentMedia!=='undefined'?sfCurrentMedia:null)||{};
    const title=safeText(item.title)||'StormFlix';
    const context=mediaContext(item)||safeText(item.library_name)||'Reproduzindo agora';
    $v('sf-v4-title').textContent=title;
    $v('sf-v4-context').textContent=context;
    $v('sf-v4-now-title').textContent=title;
    $v('sf-v4-now-subtitle').textContent=context;
    $v('sf-v4-plan').textContent=modeLabel();
    $v('sf-v4-quality-label').textContent=qualityLabel();
    const plan=window.sfLastPlaybackPlan||{};
    const video=safeText(plan.video_codec).toUpperCase();
    const audio=safeText(plan.audio_codec).toUpperCase();
    $v('sf-v4-playback-detail').textContent=[modeLabel(),video,audio].filter(Boolean).join(' · ');
    refreshResolution();
  }
  function refreshResolution(){
    const width=Number(player.videoWidth||0),height=Number(player.videoHeight||0);
    const el=$v('sf-v4-resolution');
    el.textContent=width&&height?`${width}×${height}`:'';
    el.classList.toggle('hidden',!(width&&height));
    $v('sf-v4-quality-label').textContent=qualityLabel();
  }
  function updatePlayIcon(){
    const svg=player.paused?icon.play:icon.pause;
    $v('sf-play').innerHTML=svg;
    $v('sf-center-play').innerHTML=svg;
    $v('sf-center-play').setAttribute('aria-label',player.paused?'Reproduzir':'Pausar');
  }
  function updateVolumeIcon(){
    $v('sf-mute').innerHTML=(player.muted||player.volume===0)?icon.muted:icon.volume;
  }
  function updateProgressVisual(){
    const duration=Number.isFinite(player.duration)?player.duration:0;
    const current=Number.isFinite(player.currentTime)?player.currentTime:0;
    const played=duration?Math.max(0,Math.min(100,current/duration*100)):0;
    let buffered=0;
    try{
      if(duration&&player.buffered.length){buffered=Math.max(0,Math.min(100,player.buffered.end(player.buffered.length-1)/duration*100))}
    }catch{}
    progress.style.setProperty('--sf-played',`${played}%`);
    progress.style.setProperty('--sf-buffered',`${buffered}%`);
    try{
      if('mediaSession'in navigator&&navigator.mediaSession.setPositionState&&duration>0){
        navigator.mediaSession.setPositionState({duration,playbackRate:player.playbackRate||1,position:Math.min(current,duration)});
      }
    }catch{}
  }
  async function loadNeighbors(id){
    const generation=++neighborGeneration;
    neighbors={previous:null,next:null,series_title:''};
    updateNeighborButtons();
    if(!id)return;
    try{
      const result=await request(`/media/${Number(id)}/neighbors`);
      if(generation!==neighborGeneration)return;
      neighbors=result||neighbors;
      updateNeighborButtons();
      refreshMetadata();
    }catch{}
  }
  function updateNeighborButtons(){
    const prev=$v('sf-v4-previous'),next=$v('sf-v4-next');
    prev.classList.toggle('hidden',!neighbors.previous);
    next.classList.toggle('hidden',!neighbors.next);
    if(neighbors.previous)prev.title=`Anterior: ${safeText(neighbors.previous.title)||'episódio anterior'}`;
    if(neighbors.next)next.title=`Próximo: ${safeText(neighbors.next.title)||'próximo episódio'}`;
  }
  async function playNeighbor(item){
    if(!item?.id)return;
    sfToggleSettings(false);
    await playMedia(item);
  }
  async function togglePiP(){
    try{
      if(document.pictureInPictureElement){await document.exitPictureInPicture();return}
      if(document.pictureInPictureEnabled&&player.requestPictureInPicture)await player.requestPictureInPicture();
    }catch(err){if(typeof sfToast==='function')sfToast(err?.message||'Picture in Picture indisponível')}
  }
  function openAudio(){
    sfToggleSettings(true);
    requestAnimationFrame(()=>{
      const sections=[...document.querySelectorAll('#sf-player-settings-panel .sf-setting-section')];
      const audio=sections.find(s=>/^Áudio/i.test(s.querySelector('h3')?.textContent||''));
      audio?.scrollIntoView({block:'start'});
    });
  }
  function scrubPreview(e){
    if(!Number.isFinite(player.duration)||player.duration<=0)return;
    const rect=progress.getBoundingClientRect();
    const ratio=Math.max(0,Math.min(1,(e.clientX-rect.left)/rect.width));
    const preview=$v('sf-v4-scrub-preview');
    preview.textContent=typeof sfClock==='function'?sfClock(player.duration*ratio):Math.round(player.duration*ratio)+'s';
    preview.style.left=`${ratio*100}%`;
    preview.classList.add('show');
    scrubVisible=true;
  }
  function hideScrub(){
    if(!scrubVisible)return;
    $v('sf-v4-scrub-preview').classList.remove('show');
    scrubVisible=false;
  }

  $v('sf-v4-close').onclick=()=>closePlayer();
  $v('sf-play').onclick=()=>sfTogglePlay();
  $v('sf-center-play').onclick=()=>sfTogglePlay();
  $v('sf-back').onclick=$v('sf-back10').onclick=()=>sfSeekBy(-10);
  $v('sf-forward').onclick=$v('sf-forward10').onclick=()=>sfSeekBy(10);
  $v('sf-mute').onclick=()=>sfToggleMute();
  $v('sf-volume').oninput=e=>{player.volume=Number(e.target.value);player.muted=false;sfSyncVolume();updateVolumeIcon()};
  progress.oninput=e=>{if(Number.isFinite(player.duration)&&player.duration>0)player.currentTime=player.duration*(Number(e.target.value)/1000)};
  progress.addEventListener('pointermove',scrubPreview);
  progress.addEventListener('pointerleave',hideScrub);
  $v('sf-settings').onclick=()=>sfToggleSettings();
  $v('sf-v4-audio').onclick=openAudio;
  $v('sf-subtitle').onclick=()=>sfCycleSubtitles();
  $v('sf-fullscreen').onclick=()=>sfToggleFullscreen();
  $v('sf-v4-pip').onclick=togglePiP;
  $v('sf-v4-previous').onclick=()=>playNeighbor(neighbors.previous);
  $v('sf-v4-next').onclick=()=>playNeighbor(neighbors.next);

  player.addEventListener('play',updatePlayIcon);
  player.addEventListener('pause',updatePlayIcon);
  player.addEventListener('volumechange',updateVolumeIcon);
  player.addEventListener('timeupdate',updateProgressVisual);
  player.addEventListener('progress',updateProgressVisual);
  player.addEventListener('durationchange',updateProgressVisual);
  player.addEventListener('loadedmetadata',()=>{refreshMetadata();refreshResolution();updateProgressVisual()});
  player.addEventListener('dblclick',()=>sfToggleFullscreen());
  document.addEventListener('enterpictureinpicture',()=>modal.classList.add('sf-v4-pip-active'));
  document.addEventListener('leavepictureinpicture',()=>modal.classList.remove('sf-v4-pip-active'));

  const basePlayMedia=playMedia;
  playMedia=async function(item){
    neighbors={previous:null,next:null,series_title:''};
    refreshMetadata(item);
    const result=await basePlayMedia(item);
    refreshMetadata(item);
    loadNeighbors(item?.id);
    return result;
  };

  const planObserver=new MutationObserver(()=>refreshMetadata());
  const hiddenTitle=document.querySelector('#player-title');
  if(hiddenTitle)planObserver.observe(hiddenTitle,{childList:true,subtree:true,characterData:true});

  document.addEventListener('keydown',e=>{
    if(modal.classList.contains('hidden'))return;
    if((e.key==='p'||e.key==='P')&&!['INPUT','TEXTAREA','SELECT'].includes(e.target?.tagName)){
      e.preventDefault();togglePiP();
    }
  });

  const pipButton=$v('sf-v4-pip');
  pipButton.classList.toggle('hidden',!(document.pictureInPictureEnabled&&player.requestPictureInPicture));
  updatePlayIcon();
  updateVolumeIcon();
  updateProgressVisual();
  refreshMetadata();
  if(typeof sfShowControls==='function')sfShowControls();
})();
