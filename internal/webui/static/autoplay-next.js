/* StormFlix streaming-style next episode autoplay for Web. */
(function(){
  if(window.__sfAutoNextLoaded)return;
  window.__sfAutoNextLoaded=true;

  const video=document.querySelector('#player');
  const modal=document.querySelector('#player-modal');
  if(!video||!modal)return;

  const PREF='stormflix_autoplay_next';
  const COUNTDOWN_SECONDS=10;
  let timer=null;
  let nextItem=null;
  let remaining=COUNTDOWN_SECONDS;

  function enabled(){
    try{return localStorage.getItem(PREF)!=='0'}catch{return true}
  }
  function setEnabled(value){
    try{localStorage.setItem(PREF,value?'1':'0')}catch{}
    refreshPreferenceUi();
  }
  function current(){return typeof sfCurrentMedia!=='undefined'?sfCurrentMedia:null}

  const style=document.createElement('style');
  style.textContent=`
    .sf-autonext{position:absolute;right:max(24px,4vw);bottom:max(92px,12vh);z-index:40;width:min(430px,calc(100vw - 32px));padding:22px;border-radius:18px;background:rgba(10,12,17,.94);box-shadow:0 18px 60px rgba(0,0,0,.45);color:#fff;font-family:system-ui,-apple-system,sans-serif;backdrop-filter:blur(18px)}
    .sf-autonext.hidden{display:none}.sf-autonext-kicker{font-size:12px;text-transform:uppercase;letter-spacing:.13em;color:#aeb7c8}.sf-autonext h3{margin:7px 0 5px;font-size:22px}.sf-autonext p{margin:0 0 15px;color:#c8cfda;line-height:1.45}.sf-autonext-actions{display:flex;gap:10px;flex-wrap:wrap}.sf-autonext button{border:0;border-radius:10px;padding:10px 14px;font-weight:700;cursor:pointer}.sf-autonext-play{background:#fff;color:#080a0e}.sf-autonext-cancel{background:#2a2f38;color:#fff}.sf-autonext-pref{display:flex;align-items:center;gap:9px;margin-top:15px;color:#d7dce5;font-size:13px}.sf-autonext-pref input{width:17px;height:17px}.sf-autonext-settings{display:flex;align-items:center;justify-content:space-between;gap:14px;width:100%;margin-top:10px;padding:11px 12px;border:1px solid rgba(255,255,255,.12);border-radius:10px;background:rgba(255,255,255,.06);color:#fff;text-align:left;cursor:pointer}.sf-autonext-settings strong{font-size:13px}.sf-autonext-settings span{font-size:12px;color:#b8c0ce}
    @media(max-width:700px){.sf-autonext{left:16px;right:16px;bottom:82px;width:auto}}
  `;
  document.head.appendChild(style);

  const overlay=document.createElement('div');
  overlay.className='sf-autonext hidden';
  overlay.innerHTML=`
    <div class="sf-autonext-kicker">A seguir</div>
    <h3 id="sf-autonext-title">Próximo episódio</h3>
    <p id="sf-autonext-message"></p>
    <div class="sf-autonext-actions">
      <button class="sf-autonext-play" id="sf-autonext-play" type="button">Reproduzir agora</button>
      <button class="sf-autonext-cancel" id="sf-autonext-cancel" type="button">Cancelar</button>
    </div>
    <label class="sf-autonext-pref"><input id="sf-autonext-enabled" type="checkbox"> Reproduzir próximos episódios automaticamente</label>`;
  modal.appendChild(overlay);

  const titleEl=overlay.querySelector('#sf-autonext-title');
  const messageEl=overlay.querySelector('#sf-autonext-message');
  const enabledEl=overlay.querySelector('#sf-autonext-enabled');

  function stopTimer(){if(timer){clearInterval(timer);timer=null}}
  function hide(){stopTimer();overlay.classList.add('hidden');nextItem=null;remaining=COUNTDOWN_SECONDS}
  function refreshPreferenceUi(){
    enabledEl.checked=enabled();
    document.querySelectorAll('[data-sf-autonext-state]').forEach(el=>{el.textContent=enabled()?'Ligada':'Desligada'});
  }
  function updateMessage(){
    if(!nextItem)return;
    const label=String(nextItem.title||'Próximo episódio').trim();
    titleEl.textContent=label;
    messageEl.textContent=enabled()?`Reprodução automática em ${remaining}s.`:'A reprodução automática está desligada.';
  }
  async function playNext(){
    const item=nextItem;
    hide();
    if(!item?.id||typeof playMedia!=='function')return;
    try{await playMedia(item)}catch(e){console.warn('StormFlix auto-next failed',e)}
  }
  function begin(item){
    if(!item?.id)return;
    hide();
    nextItem=item;
    remaining=COUNTDOWN_SECONDS;
    overlay.classList.remove('hidden');
    refreshPreferenceUi();
    updateMessage();
    if(!enabled())return;
    timer=setInterval(()=>{
      remaining--;
      if(remaining<=0){playNext();return}
      updateMessage();
    },1000);
  }
  async function resolveAndBegin(){
    const item=current();
    if(!item?.id||typeof request!=='function')return;
    try{
      const result=await request(`/media/${Number(item.id)}/neighbors`);
      if(Number(current()?.id)!==Number(item.id))return;
      if(result?.next)begin(result.next);
    }catch(e){console.warn('StormFlix next-episode lookup failed',e)}
  }

  overlay.querySelector('#sf-autonext-play').addEventListener('click',playNext);
  overlay.querySelector('#sf-autonext-cancel').addEventListener('click',hide);
  enabledEl.addEventListener('change',()=>{
    const wasEnabled=enabled();
    setEnabled(enabledEl.checked);
    stopTimer();
    if(nextItem&&enabledEl.checked){
      if(!wasEnabled)remaining=COUNTDOWN_SECONDS;
      updateMessage();
      timer=setInterval(()=>{remaining--;if(remaining<=0){playNext();return}updateMessage()},1000);
    }else updateMessage();
  });

  function ensureSettingsToggle(){
    const panel=document.querySelector('#sf-player-settings-panel');
    if(!panel||panel.querySelector('#sf-autonext-settings'))return;
    const button=document.createElement('button');
    button.type='button';
    button.id='sf-autonext-settings';
    button.className='sf-autonext-settings';
    button.innerHTML='<strong>Próximo episódio automático</strong><span data-sf-autonext-state></span>';
    button.addEventListener('click',()=>setEnabled(!enabled()));
    panel.appendChild(button);
    refreshPreferenceUi();
  }
  const settingsPanel=document.querySelector('#sf-player-settings-panel');
  if(settingsPanel){
    new MutationObserver(ensureSettingsToggle).observe(settingsPanel,{childList:true,subtree:true});
    ensureSettingsToggle();
  }

  video.addEventListener('ended',resolveAndBegin);
  video.addEventListener('play',hide);
  video.addEventListener('loadedmetadata',()=>{if(!video.ended)hide()});
  document.addEventListener('keydown',event=>{if(event.key==='Escape'&&!overlay.classList.contains('hidden')){event.preventDefault();hide()}});
  refreshPreferenceUi();
})();
