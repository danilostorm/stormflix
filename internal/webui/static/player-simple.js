/* StormFlix simple player: browser-native controls inspired by Just Player.
 * Original-file engines stay unchanged; transforms work for HTML video and WASM canvas. */
(()=>{
  const modal=document.querySelector('#player-modal'),video=document.querySelector('#player');
  if(!modal||!video)return;
  modal.classList.add('sf-simple-player');
  const row=modal.querySelector('.sf-controls-row');if(!row)return;
  function button(id,label,action){const b=document.createElement('button');b.id=id;b.type='button';b.className='sf-control-btn';b.textContent=label;b.setAttribute('aria-label',label);b.onclick=action;row.insertBefore(b,document.querySelector('#sf-fullscreen'));return b}
  const speed=button('sf-simple-speed','1×',()=>{const values=[.5,.75,1,1.25,1.5,1.75,2];video.playbackRate=values[(values.indexOf(video.playbackRate)+1)%values.length]});
  video.addEventListener('ratechange',()=>speed.textContent=`${video.playbackRate}×`);
  let locked=false;
  const lock=button('sf-simple-lock','Bloquear',()=>setLocked(!locked));
  function setLocked(value){locked=value;modal.classList.toggle('sf-touch-locked',locked);lock.textContent=locked?'Desbloquear':'Bloquear';lock.setAttribute('aria-pressed',String(locked));if(!locked)window.sfShowControls?.()}
  const menu=document.querySelector('#sf-v54-screen-menu');
  const custom=document.createElement('div');custom.className='sf-simple-framing';
  custom.innerHTML='<label>Zoom manual <input aria-label="Zoom manual" type="range" min="1" max="2" step=".05" value="1" data-framing="zoom"></label><label>Posição horizontal <input aria-label="Posição horizontal" type="range" min="-100" max="100" value="0" data-framing="x"></label><label>Posição vertical <input aria-label="Posição vertical" type="range" min="-100" max="100" value="0" data-framing="y"></label><button type="button" data-framing-reset>Restaurar enquadramento</button>';
  menu?.appendChild(custom);
  let zoom=1,panX=0,panY=0;
  function framing(){
    const w=modal.clientWidth,h=modal.clientHeight,vw=video.videoWidth||16,vh=video.videoHeight||9;
    if(!w||!h)return;
    const fit=Math.min(w/vw,h/vh),iw=vw*fit,ih=vh*fit,mode=window.sfScreenMode?.get?.()||'fit';
    let sx=1,sy=1;
    if(mode==='zoom')sx=sy=Math.max(w/iw,h/ih);
    if(mode==='stretch'){sx=w/iw;sy=h/ih}
    if(mode==='16x9'){const bw=Math.min(w,h*16/9),bh=bw*9/16;sx=sy=Math.min(bw/vw,bh/vh)/fit}
    sx*=zoom;sy*=zoom;
    modal.style.setProperty('--sf-frame-x',String(sx));modal.style.setProperty('--sf-frame-y',String(sy));
    modal.style.setProperty('--sf-pan-x',`${panX/100*Math.max(0,iw*sx-w)/2}px`);
    modal.style.setProperty('--sf-pan-y',`${panY/100*Math.max(0,ih*sy-h)/2}px`);
  }
  custom.addEventListener('input',e=>{const k=e.target.dataset.framing;if(k==='zoom')zoom=+e.target.value;if(k==='x')panX=+e.target.value;if(k==='y')panY=+e.target.value;framing()});
  function reset(){zoom=1;panX=panY=0;for(const el of custom.querySelectorAll('input'))el.value=el.dataset.framing==='zoom'?1:0;framing()}
  custom.querySelector('[data-framing-reset]').onclick=()=>{window.sfScreenMode?.set?.('fit');reset()};
  new ResizeObserver(framing).observe(modal);
  window.addEventListener('stormflix:screen-mode',framing);video.addEventListener('loadedmetadata',framing);
  window.addEventListener('stormflix:playback-plan',()=>{setLocked(false);reset()});
  // One tap reveals controls, double tap seeks. Swipes change position or volume.
  const interactive=t=>!!t.closest?.('button,input,select,.sf-v5-popover,.sf-player-settings');
  let start=null,lastTap=0,tapTimer=0;
  modal.addEventListener('pointerdown',e=>{
    if(interactive(e.target))return;if(locked){e.preventDefault();e.stopImmediatePropagation();return}
    start={x:e.clientX,y:e.clientY,time:video.currentTime,volume:video.volume,id:e.pointerId};
  },true);
  modal.addEventListener('pointerup',e=>{
    if(!start||start.id!==e.pointerId)return;const s=start;start=null;
    const dx=e.clientX-s.x,dy=e.clientY-s.y;
    if(locked)return;
    if(Math.abs(dx)>40&&Math.abs(dx)>Math.abs(dy)){
      if(Number.isFinite(video.duration))video.currentTime=Math.max(0,Math.min(video.duration,s.time+dx/modal.clientWidth*120));
      window.sfToast?.('Posição ajustada');lastTap=0;return;
    }
    if(Math.abs(dy)>40){video.volume=Math.max(0,Math.min(1,s.volume-dy/modal.clientHeight));video.muted=false;window.sfToast?.(`Volume ${Math.round(video.volume*100)}%`);lastTap=0;return}
    const now=performance.now();clearTimeout(tapTimer);
    if(now-lastTap<300){const delta=e.clientX<modal.clientWidth/2?-10:10;if(Number.isFinite(video.duration))video.currentTime=Math.max(0,Math.min(video.duration,video.currentTime+delta));window.sfToast?.(`${delta>0?'+':''}${delta}s`);lastTap=0}
    else{lastTap=now;tapTimer=setTimeout(()=>window.sfShowControls?.(),300)}
  },true);
  modal.addEventListener('pointercancel',()=>{start=null});
  modal.addEventListener('click',e=>{if(!interactive(e.target)){e.preventDefault();e.stopImmediatePropagation()}},true);
  window.addEventListener('keydown',e=>{if(modal.classList.contains('hidden'))return;if(locked){if(e.key==='Escape')setLocked(false);e.preventDefault();e.stopImmediatePropagation()}},true);
  new MutationObserver(()=>{if(modal.classList.contains('hidden')){if(locked)setLocked(false);start=null;clearTimeout(tapTimer)}}).observe(modal,{attributes:true,attributeFilter:['class']});
  framing();
})();
