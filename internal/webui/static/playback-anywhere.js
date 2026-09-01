/* StormFlix Playback Anywhere v1 — Chromecast, external players and TV handoff. */
(function(){
  'use strict';
  const modal=document.querySelector('#player-modal');
  const video=document.querySelector('#player');
  if(!modal||!video||modal.dataset.sfAnywhere==='1')return;
  modal.dataset.sfAnywhere='1';

  const style=document.createElement('style');
  style.textContent=`
    #sf-anywhere-toggle{min-width:44px;height:38px;border:1px solid rgba(255,255,255,.14);border-radius:10px;background:rgba(17,22,30,.88);color:#fff;font-size:18px;cursor:pointer}
    #sf-anywhere-toggle:hover,#sf-anywhere-toggle:focus-visible{outline:0;border-color:#59cdfb;box-shadow:0 0 0 2px rgba(89,205,251,.14)}
    .sf-anywhere{position:absolute;z-index:80;top:58px;right:14px;width:min(430px,calc(100vw - 28px));max-height:calc(100vh - 82px);overflow:auto;padding:0;border:1px solid #293542;border-radius:18px;background:rgba(9,13,19,.98);box-shadow:0 24px 70px rgba(0,0,0,.55);color:#edf3f8}
    .sf-anywhere.hidden{display:none!important}.sf-anywhere header{display:flex;align-items:center;justify-content:space-between;gap:14px;padding:18px 18px 14px;border-bottom:1px solid #202a34}
    .sf-anywhere header p{margin:0 0 3px;color:#62d4ff;font-size:10px;font-weight:900;letter-spacing:.13em}.sf-anywhere header h2{margin:0;font-size:20px}.sf-anywhere header button{width:36px;height:36px;border:1px solid #2a3540;border-radius:10px;background:#151b23;color:#fff;cursor:pointer}
    .sf-anywhere-body{display:grid;gap:10px;padding:14px 18px 18px}.sf-anywhere-option{display:grid;grid-template-columns:44px minmax(0,1fr) auto;align-items:center;gap:12px;width:100%;padding:13px;border:1px solid #26313d;border-radius:14px;background:#10161e;color:#eaf1f7;text-align:left;cursor:pointer}
    .sf-anywhere-option:hover,.sf-anywhere-option:focus-visible{outline:0;border-color:#58cef9;background:#12202a}.sf-anywhere-option:disabled{opacity:.48;cursor:not-allowed}.sf-anywhere-option>i{display:grid;place-items:center;width:42px;height:42px;border-radius:12px;background:#142a38;color:#6ed7ff;font-style:normal;font-size:20px}.sf-anywhere-option b{display:block;font-size:13px}.sf-anywhere-option small{display:block;margin-top:3px;color:#8391a3;font-size:10px;line-height:1.35}.sf-anywhere-option em{color:#67daa0;font-size:9px;font-style:normal;font-weight:900;letter-spacing:.07em}
    .sf-anywhere-status{margin:2px 0 0;padding:11px 12px;border:1px solid #24313d;border-radius:12px;background:#0c1219;color:#91a1b3;font-size:11px;line-height:1.45}.sf-anywhere-status.ok{border-color:#22513c;color:#79dba8}.sf-anywhere-status.error{border-color:#5c3035;color:#ff9da6}
    .sf-anywhere-link{display:flex;gap:8px}.sf-anywhere-link input{min-width:0;flex:1;padding:10px 11px;border:1px solid #26313d;border-radius:10px;background:#090e14;color:#afbdca;font-size:10px}.sf-anywhere-link button{padding:0 12px;border:1px solid #315268;border-radius:10px;background:#102431;color:#bcecff;font-weight:800;cursor:pointer}
    @media(max-width:600px){.sf-anywhere{top:auto;bottom:10px;right:10px;left:10px;width:auto;max-height:78vh;border-radius:16px}.sf-anywhere-option{grid-template-columns:40px minmax(0,1fr)}.sf-anywhere-option em{display:none}}
  `;
  document.head.appendChild(style);

  const toggle=document.createElement('button');
  toggle.id='sf-anywhere-toggle';toggle.type='button';toggle.title='Reproduzir em…';toggle.setAttribute('aria-label','Reproduzir em outro dispositivo');toggle.textContent='📺';
  const topActions=modal.querySelector('.player-top-actions');
  if(topActions)topActions.insertBefore(toggle,topActions.firstChild);else modal.appendChild(toggle);

  const panel=document.createElement('aside');
  panel.id='sf-anywhere';panel.className='sf-anywhere hidden';panel.setAttribute('aria-label','Reproduzir em outro dispositivo');
  panel.innerHTML=`<header><div><p>PLAYBACK ANYWHERE</p><h2>Reproduzir em…</h2></div><button type="button" data-any-close aria-label="Fechar">×</button></header>
    <div class="sf-anywhere-body">
      <button class="sf-anywhere-option" type="button" data-any-local><i>▶</i><span><b>Este dispositivo</b><small>Continuar no player atual do StormFlix.</small></span><em>ATUAL</em></button>
      <button class="sf-anywhere-option" type="button" data-any-cast><i>▣</i><span><b>Chromecast / Google TV</b><small>Enviar para uma TV compatível usando Google Cast.</small></span><em data-any-cast-state>PROCURAR</em></button>
      <button class="sf-anywhere-option" type="button" data-any-external><i>↗</i><span><b>Player externo</b><small>Abrir um stream temporário no VLC, mpv ou outro player do sistema.</small></span><em>ABRIR</em></button>
      <button class="sf-anywhere-option" type="button" data-any-tv-info><i>TV</i><span><b>Samsung Tizen / LG webOS</b><small>Os apps StormFlix TV usam o mesmo PlaybackPlan, progresso e perfil.</small></span><em>PRONTO</em></button>
      <div class="sf-anywhere-status" data-any-status>Escolha onde deseja continuar a reprodução.</div>
      <div class="sf-anywhere-link hidden" data-any-link><input readonly aria-label="Link temporário de reprodução"><button type="button">Copiar</button></div>
    </div>`;
  modal.appendChild(panel);

  const status=panel.querySelector('[data-any-status]');
  const linkBox=panel.querySelector('[data-any-link]');
  let castLoadPromise=null,remotePlan=null,remoteGrant=null,castHeartbeat=0,castSequence=0;

  function mediaID(){return Number(window.sfLastPlaybackPlan?.media_id||window.sfPlaybackCore?.currentPlan?.()?.media_id||0)}
  function currentPlan(){return window.sfPlaybackCore?.currentPlan?.()||window.sfLastPlaybackPlan||{}}
  function title(){return document.querySelector('#player-title')?.textContent?.trim()||'StormFlix'}
  function setStatus(message,type=''){status.textContent=message;status.className='sf-anywhere-status'+(type?' '+type:'')}
  function showLink(url){const input=linkBox.querySelector('input');input.value=url||'';linkBox.classList.toggle('hidden',!url)}
  function close(){panel.classList.add('hidden')}
  function open(){panel.classList.remove('hidden');showLink('');setStatus('Escolha onde deseja continuar a reprodução.')}
  toggle.addEventListener('click',e=>{e.stopPropagation();panel.classList.contains('hidden')?open():close()});
  panel.querySelector('[data-any-close]').onclick=close;
  panel.querySelector('[data-any-local]').onclick=()=>{close();video.play().catch(()=>{})};

  async function json(url,options={}){
    const response=await fetch(url,{credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json',...(options.headers||{})},...options});
    const text=await response.text();let data={};try{data=JSON.parse(text)}catch{}
    if(!response.ok)throw new Error(data.error||`HTTP ${response.status}`);
    return data;
  }

  function remoteCapabilities(){
    return {containers:['mp4'],video_codecs:['h264'],audio_codecs:['aac','mp3'],subtitle_formats:['vtt'],allow_remux:true,allow_audio_compatibility:true,allow_video_transcode:true,max_transcode_bitrate_kbps:18000,native_audio_track_selection:false,server_selects_audio:true,picture_in_picture:false,media_session:false};
  }

  async function prepareRemote(){
    const id=mediaID();if(!id)throw new Error('Nenhuma mídia ativa para transmitir.');
    const base=currentPlan();
    const request={client_kind:'tv',client_name:'StormFlix Playback Anywhere',client_version:'1.0',quality:'auto',capabilities:remoteCapabilities(),start_position_seconds:Number.isFinite(video.currentTime)?Math.max(0,video.currentTime):0};
    if(Number.isInteger(base.audio_stream)&&base.audio_stream>=0)request.audio_stream=base.audio_stream;
    const plan=await json(`/api/v1/media/${id}/playback/plan`,{method:'POST',body:JSON.stringify(request)});
    if(!plan?.available||!plan?.url)throw new Error(plan?.reason||'A TV não recebeu uma rota de reprodução compatível.');
    const grant=await json(`/api/v1/media/${id}/playback/grant`,{method:'POST',body:JSON.stringify({url:plan.url})});
    remotePlan=plan;remoteGrant=grant;return {id,plan,url:grant.url};
  }

  function castContentType(plan,url){
    const value=String(url||'').toLowerCase();
    if(value.includes('.m3u8')||plan?.transport==='hls'||['video_transcode','audio_compatibility','remux'].includes(plan?.mode))return'application/x-mpegURL';
    return'video/mp4';
  }

  function ensureCastSDK(){
    if(window.cast?.framework&&window.chrome?.cast?.media)return Promise.resolve(true);
    if(castLoadPromise)return castLoadPromise;
    castLoadPromise=new Promise((resolve,reject)=>{
      const previous=window.__onGCastApiAvailable;
      window.__onGCastApiAvailable=function(available,error){try{previous?.(available,error)}catch{}if(available)resolve(true);else reject(new Error(error?.description||'Google Cast indisponível'))};
      const script=document.createElement('script');script.async=true;script.src='https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1';script.onload=()=>{if(window.cast?.framework)resolve(true)};script.onerror=()=>reject(new Error('Não foi possível carregar o Google Cast SDK.'));document.head.appendChild(script);
      setTimeout(()=>reject(new Error('Google Cast não respondeu neste navegador.')),12000);
    }).catch(err=>{castLoadPromise=null;throw err});
    return castLoadPromise;
  }

  function stopCastHeartbeat(){if(castHeartbeat){clearInterval(castHeartbeat);castHeartbeat=0}}
  function castMediaPosition(session){const media=session?.getMediaSession?.();const p=Number(media?.currentTime);return Number.isFinite(p)?p:0}
  function startCastHeartbeat(session,id,plan){
    stopCastHeartbeat();castSequence=0;
    const send=()=>{
      const position=castMediaPosition(session),duration=Number(session?.getMediaSession?.()?.media?.duration||video.duration||0);if(!Number.isFinite(position)||position<0)return;
      fetch(`/api/v1/media/${id}/playback`,{method:'POST',credentials:'same-origin',keepalive:true,headers:{'Content-Type':'application/json'},body:JSON.stringify({position_seconds:position,duration_seconds:Number.isFinite(duration)?duration:0,state:'playing',mode:'cast',playback_session_id:String(plan?.playback_session_id||''),progress_sequence:++castSequence,progress_event_ms:Date.now(),progress_reason:'cast'})}).catch(()=>{});
    };
    send();castHeartbeat=setInterval(send,10000);
  }

  panel.querySelector('[data-any-cast]').onclick=async()=>{
    const button=panel.querySelector('[data-any-cast]');button.disabled=true;setStatus('Preparando uma rota compatível e procurando dispositivos…');
    try{
      const prepared=await prepareRemote();await ensureCastSDK();
      const context=cast.framework.CastContext.getInstance();
      context.setOptions({receiverApplicationId:chrome.cast.media.DEFAULT_MEDIA_RECEIVER_APP_ID,autoJoinPolicy:chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED});
      await context.requestSession();const session=context.getCurrentSession();if(!session)throw new Error('Nenhum Chromecast foi selecionado.');
      const info=new chrome.cast.media.MediaInfo(prepared.url,castContentType(prepared.plan,prepared.url));
      const metadata=new chrome.cast.media.GenericMediaMetadata();metadata.title=title();metadata.subtitle='StormFlix';info.metadata=metadata;
      const request=new chrome.cast.media.LoadRequest(info);request.autoplay=true;request.currentTime=Number.isFinite(video.currentTime)?Math.max(0,video.currentTime):0;
      await session.loadMedia(request);video.pause();startCastHeartbeat(session,prepared.id,prepared.plan);showLink(prepared.url);setStatus('Transmitindo para o Chromecast. O progresso continua vinculado ao perfil atual.','ok');
      const state=panel.querySelector('[data-any-cast-state]');if(state)state.textContent='CONECTADO';
    }catch(err){setStatus(err?.message||'Não foi possível transmitir para o Chromecast.','error')}
    finally{button.disabled=false}
  };

  function openExternalURL(url){
    const bridge=window.ExternalPlayer||window.NativePlayer;
    try{
      if(bridge&&typeof bridge.open==='function'){bridge.open(url,title());return true}
      if(bridge&&typeof bridge.play==='function'){bridge.play(url,title());return true}
      if(window.NativeInterface&&typeof window.NativeInterface.openExternalPlayer==='function'){window.NativeInterface.openExternalPlayer(url,title());return true}
    }catch{}
    const a=document.createElement('a');a.href=url;a.target='_blank';a.rel='noopener';document.body.appendChild(a);a.click();a.remove();return true;
  }

  panel.querySelector('[data-any-external]').onclick=async()=>{
    const button=panel.querySelector('[data-any-external]');button.disabled=true;setStatus('Preparando link temporário para o player externo…');
    try{const prepared=await prepareRemote();showLink(prepared.url);openExternalURL(prepared.url);setStatus('Link temporário aberto. Se o sistema perguntar, escolha VLC, mpv ou seu player preferido.','ok')}
    catch(err){setStatus(err?.message||'Não foi possível preparar o player externo.','error')}
    finally{button.disabled=false}
  };

  panel.querySelector('[data-any-tv-info]').onclick=()=>setStatus('Samsung Tizen e LG webOS usam os apps StormFlix TV. Eles abrem a interface completa, usam o PlaybackPlan do servidor e preservam perfil, progresso, áudio e qualidade.','ok');
  linkBox.querySelector('button').onclick=async()=>{const value=linkBox.querySelector('input').value;if(!value)return;try{await navigator.clipboard.writeText(value);setStatus('Link temporário copiado. Ele expira automaticamente.','ok')}catch{linkBox.querySelector('input').select();document.execCommand('copy')}};

  window.addEventListener('stormflix:playback-plan',()=>{remotePlan=null;remoteGrant=null;showLink('')});
  document.addEventListener('keydown',e=>{if(e.key==='Escape'&&!panel.classList.contains('hidden')){e.stopPropagation();close()}},true);
  window.addEventListener('beforeunload',stopCastHeartbeat);
})();
