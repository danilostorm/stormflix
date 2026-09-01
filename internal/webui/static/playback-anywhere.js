/* StormFlix Playback Anywhere v3 — native Cast, Web Cast, app chooser and casting-app handoff. */
(function(){
  'use strict';
  const modal=document.querySelector('#player-modal');
  const video=document.querySelector('#player');
  if(!modal||!video||modal.dataset.sfAnywhere==='3')return;
  modal.dataset.sfAnywhere='3';

  const style=document.createElement('style');
  style.textContent=`
    #sf-anywhere-toggle{min-width:44px;height:38px;border:1px solid rgba(255,255,255,.14);border-radius:10px;background:rgba(17,22,30,.88);color:#fff;font-size:18px;cursor:pointer}
    #sf-anywhere-toggle:hover,#sf-anywhere-toggle:focus-visible{outline:0;border-color:#59cdfb;box-shadow:0 0 0 2px rgba(89,205,251,.14)}
    .sf-anywhere{position:fixed;z-index:240;width:min(430px,calc(100vw - 28px));max-height:calc(100vh - 24px);overflow:auto;padding:0;border:1px solid #293542;border-radius:18px;background:rgba(9,13,19,.985);box-shadow:0 24px 70px rgba(0,0,0,.62);color:#edf3f8;backdrop-filter:blur(16px)}
    .sf-anywhere.hidden{display:none!important}.sf-anywhere header{display:flex;align-items:center;justify-content:space-between;gap:14px;padding:18px 18px 14px;border-bottom:1px solid #202a34}
    .sf-anywhere header p{margin:0 0 3px;color:#62d4ff;font-size:10px;font-weight:900;letter-spacing:.13em}.sf-anywhere header h2{margin:0;font-size:20px}.sf-anywhere header button{width:36px;height:36px;border:1px solid #2a3540;border-radius:10px;background:#151b23;color:#fff;cursor:pointer}
    .sf-anywhere-body{display:grid;gap:10px;padding:14px 18px 18px}.sf-anywhere-option{display:grid;grid-template-columns:44px minmax(0,1fr) auto;align-items:center;gap:12px;width:100%;padding:13px;border:1px solid #26313d;border-radius:14px;background:#10161e;color:#eaf1f7;text-align:left;cursor:pointer}
    .sf-anywhere-option:hover,.sf-anywhere-option:focus-visible{outline:0;border-color:#58cef9;background:#12202a}.sf-anywhere-option:disabled{opacity:.48;cursor:not-allowed}.sf-anywhere-option>i{display:grid;place-items:center;width:42px;height:42px;border-radius:12px;background:#142a38;color:#6ed7ff;font-style:normal;font-size:20px}.sf-anywhere-option b{display:block;font-size:13px}.sf-anywhere-option small{display:block;margin-top:3px;color:#8391a3;font-size:10px;line-height:1.35}.sf-anywhere-option em{color:#67daa0;font-size:9px;font-style:normal;font-weight:900;letter-spacing:.07em}
    .sf-anywhere-option[data-any-webcast]>i{background:#251d39;color:#c4a7ff}.sf-anywhere-option[data-any-webcast] em{color:#c8a9ff}
    .sf-anywhere-status{margin:2px 0 0;padding:11px 12px;border:1px solid #24313d;border-radius:12px;background:#0c1219;color:#91a1b3;font-size:11px;line-height:1.45}.sf-anywhere-status.ok{border-color:#22513c;color:#79dba8}.sf-anywhere-status.error{border-color:#5c3035;color:#ff9da6}
    .sf-anywhere-link{display:flex;gap:8px}.sf-anywhere-link input{min-width:0;flex:1;padding:10px 11px;border:1px solid #26313d;border-radius:10px;background:#090e14;color:#afbdca;font-size:10px}.sf-anywhere-link button{padding:0 12px;border:1px solid #315268;border-radius:10px;background:#102431;color:#bcecff;font-weight:800;cursor:pointer}
    @media(max-width:600px){.sf-anywhere{top:auto!important;bottom:10px!important;right:10px!important;left:10px!important;width:auto!important;max-height:78vh;border-radius:16px}.sf-anywhere-option{grid-template-columns:40px minmax(0,1fr)}.sf-anywhere-option em{display:none}}
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
      <button class="sf-anywhere-option" type="button" data-any-local><i>▶</i><span><b>Este dispositivo</b><small>Reproduzir no player atual do StormFlix.</small></span><em>LOCAL</em></button>
      <button class="sf-anywhere-option" type="button" data-any-cast><i>▣</i><span><b>Chromecast / Google TV</b><small>No APK usa descoberta Google Cast nativa; no Chrome usa o Web Sender.</small></span><em data-any-cast-state>PROCURAR</em></button>
      <button class="sf-anywhere-option" type="button" data-any-webcast><i>W</i><span><b>Web Video Cast / Roku / DLNA</b><small>No Android envia o stream direto ao Web Video Cast para escolher Roku, Fire TV, DLNA, webOS e outros receptores.</small></span><em data-any-wvc-state>ABRIR</em></button>
      <button class="sf-anywhere-option" type="button" data-any-external><i>↗</i><span><b>Abrir com outro player</b><small>No APK abre o seletor Android com VLC, MX Player, Web Video Cast e outros apps instalados.</small></span><em>ESCOLHER</em></button>
      <button class="sf-anywhere-option" type="button" data-any-tv-info><i>TV</i><span><b>Apps StormFlix TV</b><small>Samsung Tizen e LG webOS usam o mesmo PlaybackPlan, perfil e progresso.</small></span><em>PRONTO</em></button>
      <div class="sf-anywhere-status" data-any-status>Escolha onde deseja reproduzir.</div>
      <div class="sf-anywhere-link hidden" data-any-link><input readonly aria-label="Link temporário de reprodução"><button type="button">Copiar</button></div>
    </div>`;
  document.body.appendChild(panel);

  const status=panel.querySelector('[data-any-status]');
  const linkBox=panel.querySelector('[data-any-link]');
  const nativeBridge=window.NativePlaybackAnywhere;
  let castLoadPromise=null,castHeartbeat=0,castSequence=0,targetMedia=null,anchorElement=null;

  function hasNativeBridge(){try{return !!nativeBridge&&typeof nativeBridge.isAvailable==='function'&&!!nativeBridge.isAvailable()}catch{return false}}
  function livePlan(){return window.sfPlaybackCore?.currentPlan?.()||window.sfLastPlaybackPlan||{}}
  function currentPlayingID(){return Number(window.sfLastPlaybackPlan?.media_id||window.sfPlaybackCore?.currentPlan?.()?.media_id||0)}
  function activeDetail(){try{return typeof currentDetail!=='undefined'&&currentDetail?.id?currentDetail:null}catch{return null}}
  function detailMedia(){const current=activeDetail();const selected=window.sfSelectedDetailMedia;if(selected?.id&&(!current||Number(selected._logical_id||selected.id)===Number(current.id)))return selected;if(current?.id)return current;return selected?.id?selected:null}
  function mediaID(){return Number(targetMedia?.id||currentPlayingID()||0)}
  function currentPlan(){const plan=livePlan();return !targetMedia||Number(plan?.media_id||0)===mediaID()?plan:{}}
  function title(){return String(targetMedia?.title||document.querySelector('#player-title')?.textContent||'StormFlix').trim()||'StormFlix'}
  function position(){if(targetMedia&&Number(targetMedia.id)!==currentPlayingID())return 0;return Number.isFinite(video.currentTime)?Math.max(0,video.currentTime):0}
  function setStatus(message,type=''){status.textContent=message;status.className='sf-anywhere-status'+(type?' '+type:'')}
  function showLink(url){const input=linkBox.querySelector('input');input.value=url||'';linkBox.classList.toggle('hidden',!url)}
  function close(){panel.classList.add('hidden')}

  function placePanel(anchor){
    if(innerWidth<=600)return;
    const margin=12,topFloor=64;
    const rect=(anchor||toggle).getBoundingClientRect();
    const width=Math.min(430,innerWidth-28);
    panel.style.width=width+'px';
    panel.style.right='auto';panel.style.bottom='auto';
    let left=rect.right+12;
    if(left+width>innerWidth-margin)left=rect.left-width-12;
    if(left<margin)left=Math.min(Math.max(margin,rect.left),innerWidth-width-margin);
    const maxTop=Math.max(topFloor,innerHeight-Math.min(panel.scrollHeight||520,innerHeight-24)-margin);
    const top=Math.min(Math.max(topFloor,rect.top-12),maxTop);
    panel.style.left=Math.round(left)+'px';panel.style.top=Math.round(top)+'px';
  }

  function open(anchor){anchorElement=anchor||toggle;panel.classList.remove('hidden');showLink('');setStatus(targetMedia?.title?`Escolha onde reproduzir ${targetMedia.title}.`:'Escolha onde deseja continuar a reprodução.');requestAnimationFrame(()=>placePanel(anchorElement))}
  function openForMedia(media,anchor){if(!media?.id)return;targetMedia={...media,id:Number(media.id)};open(anchor)}
  function openCurrent(anchor){targetMedia=null;open(anchor)}

  toggle.addEventListener('click',e=>{e.stopPropagation();if(panel.classList.contains('hidden'))openCurrent(toggle);else close()});
  panel.querySelector('[data-any-close]').onclick=close;
  panel.querySelector('[data-any-local]').onclick=()=>{const selected=targetMedia;close();if(selected?.id&&typeof playMedia==='function'){targetMedia=null;playMedia(selected);return}video.play().catch(()=>{})};

  document.addEventListener('click',e=>{
    const button=e.target.closest?.('#detail-anywhere');
    if(button){e.preventDefault();e.stopPropagation();const selected=detailMedia();if(selected?.id)openForMedia(selected,button);else{targetMedia=null;open(button);setStatus('Não foi possível identificar este título para transmissão.','error')}return}
    if(e.target.closest?.('[data-close-detail],#player-close'))close();
  },true);
  window.addEventListener('resize',()=>{if(!panel.classList.contains('hidden'))placePanel(anchorElement)});

  async function json(url,options={}){const response=await fetch(url,{credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json',...(options.headers||{})},...options});const text=await response.text();let data={};try{data=JSON.parse(text)}catch{}if(!response.ok)throw new Error(data.error||`HTTP ${response.status}`);return data}
  function remoteCapabilities(){return {containers:['mp4'],video_codecs:['h264'],audio_codecs:['aac','mp3'],subtitle_formats:['vtt'],allow_remux:true,allow_audio_compatibility:true,allow_video_transcode:true,max_transcode_bitrate_kbps:18000,native_audio_track_selection:false,server_selects_audio:true,picture_in_picture:false,media_session:false}}

  async function prepareRemote(){
    const id=mediaID();if(!id)throw new Error('Nenhuma mídia selecionada para transmitir.');
    const base=currentPlan();
    const request={client_kind:'tv',client_name:'StormFlix Playback Anywhere',client_version:'3.0',quality:'auto',capabilities:remoteCapabilities(),start_position_seconds:position()};
    if(Number.isInteger(base.audio_stream)&&base.audio_stream>=0)request.audio_stream=base.audio_stream;
    const plan=await json(`/api/v1/media/${id}/playback/plan`,{method:'POST',body:JSON.stringify(request)});
    if(!plan?.available||!plan?.url)throw new Error(plan?.reason||'O receptor não recebeu uma rota de reprodução compatível.');
    const grant=await json(`/api/v1/media/${id}/playback/grant`,{method:'POST',body:JSON.stringify({url:plan.url})});
    return {id,plan,url:grant.url,mime:castContentType(plan,grant.url)};
  }

  function castContentType(plan,url){const value=String(url||'').toLowerCase();if(value.includes('.m3u8')||plan?.transport==='hls'||['video_transcode','audio_compatibility','remux'].includes(plan?.mode))return'application/x-mpegURL';return'video/mp4'}
  function ensureCastSDK(){if(window.cast?.framework&&window.chrome?.cast?.media)return Promise.resolve(true);if(location.protocol!=='https:'&&!['localhost','127.0.0.1'].includes(location.hostname))return Promise.reject(new Error('No navegador, Google Cast exige abrir o StormFlix por HTTPS. No APK Android a descoberta é nativa.'));if(castLoadPromise)return castLoadPromise;castLoadPromise=new Promise((resolve,reject)=>{const previous=window.__onGCastApiAvailable;window.__onGCastApiAvailable=function(available,error){try{previous?.(available,error)}catch{}if(available)resolve(true);else reject(new Error(error?.description||'Google Cast indisponível'))};const script=document.createElement('script');script.async=true;script.src='https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1';script.onload=()=>{if(window.cast?.framework)resolve(true)};script.onerror=()=>reject(new Error('Não foi possível carregar o Google Cast SDK.'));document.head.appendChild(script);setTimeout(()=>reject(new Error('Google Cast não respondeu neste navegador. Confirme HTTPS, Wi-Fi e permissões de rede local.')),12000)}).catch(err=>{castLoadPromise=null;throw err});return castLoadPromise}

  function stopCastHeartbeat(){if(castHeartbeat){clearInterval(castHeartbeat);castHeartbeat=0}}
  function castMediaPosition(session){const media=session?.getMediaSession?.();const p=Number(media?.currentTime);return Number.isFinite(p)?p:0}
  function sendCastProgress(id,plan,castPosition,duration){if(!Number.isFinite(castPosition)||castPosition<0)return;fetch(`/api/v1/media/${id}/playback`,{method:'POST',credentials:'same-origin',keepalive:true,headers:{'Content-Type':'application/json'},body:JSON.stringify({position_seconds:castPosition,duration_seconds:Number.isFinite(duration)?duration:0,state:'playing',mode:'cast',playback_session_id:String(plan?.playback_session_id||''),progress_sequence:++castSequence,progress_event_ms:Date.now(),progress_reason:'cast'})}).catch(()=>{})}
  function startWebCastHeartbeat(session,id,plan){stopCastHeartbeat();castSequence=0;const send=()=>sendCastProgress(id,plan,castMediaPosition(session),Number(session?.getMediaSession?.()?.media?.duration||video.duration||0));send();castHeartbeat=setInterval(send,10000)}

  window.sfPlaybackAnywhereNativeResult=(state,message)=>{
    const text=String(message||'');
    if(state==='cast_connected'){if(!modal.classList.contains('hidden'))video.pause();setStatus(text||'Transmitindo pelo Google Cast nativo.','ok');const badge=panel.querySelector('[data-any-cast-state]');if(badge)badge.textContent='CONECTADO';return}
    if(state==='cast_searching'){setStatus(text||'Procurando dispositivos Google Cast…');return}
    if(state==='external_opened'||state==='wvc_opened'){setStatus(text,'ok');return}
    if(state==='wvc_missing'||state==='error'){setStatus(text||'Não foi possível concluir a ação.','error');return}
    if(text)setStatus(text);
  };

  panel.querySelector('[data-any-cast]').onclick=async()=>{
    const button=panel.querySelector('[data-any-cast]');button.disabled=true;setStatus('Preparando uma rota compatível e procurando dispositivos…');
    try{
      const prepared=await prepareRemote();showLink(prepared.url);
      if(hasNativeBridge()&&typeof nativeBridge.openNativeCast==='function'){
        nativeBridge.openNativeCast(prepared.url,title(),prepared.mime,position());
        setStatus('Abrindo o seletor nativo do Google Cast…');
      }else{
        await ensureCastSDK();
        const context=cast.framework.CastContext.getInstance();context.setOptions({receiverApplicationId:chrome.cast.media.DEFAULT_MEDIA_RECEIVER_APP_ID,autoJoinPolicy:chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED});
        await context.requestSession();const session=context.getCurrentSession();if(!session)throw new Error('Nenhum Chromecast foi selecionado.');
        const info=new chrome.cast.media.MediaInfo(prepared.url,prepared.mime);const metadata=new chrome.cast.media.GenericMediaMetadata();metadata.title=title();metadata.subtitle='StormFlix';info.metadata=metadata;
        const request=new chrome.cast.media.LoadRequest(info);request.autoplay=true;request.currentTime=position();await session.loadMedia(request);if(!modal.classList.contains('hidden'))video.pause();startWebCastHeartbeat(session,prepared.id,prepared.plan);setStatus('Transmitindo para o Chromecast. O progresso continua vinculado ao perfil atual.','ok');const badge=panel.querySelector('[data-any-cast-state]');if(badge)badge.textContent='CONECTADO';
      }
    }catch(err){setStatus(err?.message||'Não foi possível transmitir para o Chromecast.','error')}
    finally{button.disabled=false}
  };

  panel.querySelector('[data-any-webcast]').onclick=async()=>{
    const button=panel.querySelector('[data-any-webcast]');button.disabled=true;setStatus('Preparando stream temporário para o Web Video Cast…');
    try{
      const prepared=await prepareRemote();showLink(prepared.url);
      if(hasNativeBridge()&&typeof nativeBridge.openWebVideoCast==='function'){
        nativeBridge.openWebVideoCast(prepared.url,title(),prepared.mime);
      }else if(navigator.share){
        await navigator.share({title:title(),text:'Reproduzir pelo StormFlix',url:prepared.url});setStatus('Link enviado ao menu de compartilhamento. Escolha o Web Video Cast se estiver instalado.','ok');
      }else{
        setStatus('No APK Android esta opção abre o Web Video Cast diretamente. Neste navegador, copie o link temporário abaixo e abra no app.','ok');
      }
    }catch(err){if(err?.name!=='AbortError')setStatus(err?.message||'Não foi possível abrir o Web Video Cast.','error')}
    finally{button.disabled=false}
  };

  panel.querySelector('[data-any-external]').onclick=async()=>{
    const button=panel.querySelector('[data-any-external]');button.disabled=true;setStatus('Preparando link temporário para os players instalados…');
    try{
      const prepared=await prepareRemote();showLink(prepared.url);
      if(hasNativeBridge()&&typeof nativeBridge.openExternalPlayer==='function'){
        nativeBridge.openExternalPlayer(prepared.url,title(),prepared.mime);
      }else if(navigator.share){
        await navigator.share({title:title(),text:'Abrir vídeo do StormFlix',url:prepared.url});setStatus('Menu do sistema aberto. Escolha um player compatível.','ok');
      }else{
        setStatus('O navegador não pode enumerar aplicativos instalados. Use o APK StormFlix para o seletor nativo ou copie o link temporário abaixo.','ok');
      }
    }catch(err){if(err?.name!=='AbortError')setStatus(err?.message||'Não foi possível preparar o player externo.','error')}
    finally{button.disabled=false}
  };

  panel.querySelector('[data-any-tv-info]').onclick=()=>setStatus('Samsung Tizen e LG webOS usam os apps StormFlix TV. Roku/DLNA/Fire TV podem ser alcançados agora pelo Web Video Cast no Android; DLNA nativo do StormFlix fica isolado para uma etapa posterior.','ok');
  linkBox.querySelector('button').onclick=async()=>{const value=linkBox.querySelector('input').value;if(!value)return;try{await navigator.clipboard.writeText(value);setStatus('Link temporário copiado. Ele expira automaticamente.','ok')}catch{linkBox.querySelector('input').select();document.execCommand('copy')}};

  window.StormFlixPlaybackAnywhere={openForMedia,openCurrent,close};
  window.addEventListener('stormflix:playback-plan',()=>showLink(''));
  document.addEventListener('keydown',e=>{if(e.key==='Escape'&&!panel.classList.contains('hidden')){e.stopPropagation();close()}},true);
  window.addEventListener('beforeunload',stopCastHeartbeat);
})();
