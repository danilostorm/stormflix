/* StormFlix Admin: Playback Engine v6 / adaptive decode diagnostics. */
(function(){
  let timer=null;
  let observer=null;

  function html(value){return String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function bytes(value){const n=Number(value||0);if(!n)return'0 B';const units=['B','KiB','MiB','GiB','TiB'];const i=Math.min(units.length-1,Math.floor(Math.log(n)/Math.log(1024)));return`${(n/Math.pow(1024,i)).toFixed(i?1:0)} ${units[i]}`}
  function rate(value){const n=Number(value||0);return n?`${(n/1000).toFixed(n>=10000?0:1)} Mb/s`:'—'}
  function engineName(engine){return engine?.hardware_summary||'CPU'}

  function localDecodeClientKind(){
    const ua=String(navigator.userAgent||'').toLowerCase();
    if(ua.includes('android')||ua.includes('; wv')||window.NativePlaybackAnywhere)return'android_webview';
    if(/tizen|web0s|webos|smart-tv|smarttv|hbbtv|netcast/.test(ua))return'tv';
    if(navigator.userAgentData?.mobile||/iphone|ipad|ipod|mobile/.test(ua)||(/macintosh/.test(ua)&&Number(navigator.maxTouchPoints||0)>1))return'mobile_web';
    return'web';
  }

  function localClientLabel(kind){
    return({web:'navegador desktop',android_webview:'Android / WebView',mobile_web:'navegador móvel',tv:'TV / navegador de TV'})[kind]||kind;
  }

  function ensureRoot(){
    const page=document.querySelector('#automation');
    if(!page||page.classList.contains('hidden'))return null;
    let root=document.querySelector('#automation-transcode-v5');
    if(root)return root;
    const section=document.createElement('section');
    section.className='panel';section.id='automation-transcode-v5';
    section.innerHTML='<div class="panel-head"><div><h2>Reprodução · Playback Engine v6</h2><p>Direct Play vem primeiro. HEVC incompatível pode ser decodificado localmente no navegador por WASM/WebCodecs; o servidor transcodifica vídeo somente quando as rotas anteriores não servem.</p></div><span class="mode-chip">carregando</span></div><div data-transcode-body><small>Lendo capacidades, FFmpeg, hardware e sessões…</small></div>';
    const playback=document.querySelector('#automation-playbacks')?.closest('section.panel');
    if(playback?.parentElement)playback.parentElement.insertBefore(section,playback);
    else page.appendChild(section);
    return section;
  }

  function browserLocalSummary(){
    const secure=Boolean(window.isSecureContext||location.hostname==='localhost'||location.hostname==='127.0.0.1');
    const webcodecs=typeof VideoEncoder==='function'&&typeof VideoDecoder==='function';
    const wasm=typeof WebAssembly==='object'&&typeof Worker==='function';
    const kind=localDecodeClientKind();
    const cores=Math.max(1,Number(navigator.hardwareConcurrency||2));
    const memory=Math.max(0,Number(navigator.deviceMemory||0));
    const maxHeight=cores>=12&&(memory===0||memory>=8)?2160:cores>=6?1080:720;
    return{kind,desktop:kind==='web',secure,webcodecs,wasm,cores,memory,maxHeight};
  }

  async function refresh(){
    const root=ensureRoot();if(!root)return stop();
    try{
      const [server,playbacks]=await Promise.all([req('/admin/server'),req('/admin/playbacks/diagnostics').catch(()=>[])]);
      const engine=server?.transcode_engine||{},cache=server?.transcode_cache||{},resources=server?.ffmpeg_resources||{};
      const sessions=Array.isArray(server?.transcode_session_details)?server.transcode_session_details:[];
      const webSessions=Array.isArray(server?.web_stream_session_details)?server.web_stream_session_details:[];
      const playbackBySession=new Map((playbacks||[]).map(p=>[String(p.playback_session_id||''),p]));
      const local=browserLocalSummary();
      const chip=root.querySelector('.mode-chip');
      if(chip)chip.textContent=`v6 · ${Number(resources.active||0)}/${Number(resources.total_limit||0)} FFmpeg`;
      const body=root.querySelector('[data-transcode-body]');if(!body)return;
      body.innerHTML=`
        <div class="technical-meter">
          <div><b>${local.desktop?'AUTOMÁTICO':'FALLBACK'}</b><span>Decisão local interna</span></div>
          <div><b>${local.desktop&&local.secure&&local.webcodecs&&local.wasm?'PRONTO':'FALLBACK'}</b><span>Desktop / WebCodecs / HTTPS</span></div>
          <div><b>${local.desktop?local.maxHeight+'p':'NATIVO'}</b><span>Limite local automático</span></div>
          <div><b>${html(engineName(engine))}</b><span>Aceleração do servidor</span></div>
        </div>
        <p class="phase2-hint">Cliente: ${html(localClientLabel(local.kind))} · ${local.cores||'—'} threads · ${local.memory?local.memory+' GB RAM estimada':'RAM não informada'} · HTTPS/contexto seguro: ${local.secure?'sim':'não'} · WebCodecs: ${local.webcodecs?'sim':'não'}. A rota é automática e não possui controle de usuário: Direct Play vem primeiro e um desktop capaz tenta HEVC local, inclusive até 3840×2160, antes do transcode de vídeo. HDR local permanece conservador/desativado; HDR incompatível continua no caminho seguro de tone mapping do servidor.</p>
        <div class="technical-meter">
          <div><b>${html(engine.preferred_h264||'—')}</b><span>Encoder H.264 servidor</span></div>
          <div><b>${html(engine.ffmpeg_version||'não detectado')}</b><span>FFmpeg</span></div>
          <div><b>${bytes(cache.usage_bytes)}</b><span>Cache / ${bytes(cache.max_bytes)}</span></div>
          <div><b>${engine.tone_map&&engine.zscale?'PRONTO':'LIMITADO'}</b><span>Tone mapping</span></div>
        </div>
        <div class="technical-meter">
          <div><b>${Number(resources.active||0)} / ${Number(resources.total_limit||0)}</b><span>FFmpeg ativos / limite</span></div>
          <div><b>${Number(resources.video_active||0)} / ${Number(resources.video_limit||0)}</b><span>Encodes de vídeo</span></div>
          <div><b>${Number(resources.cpu_thread_limit||0)}</b><span>Threads CPU por processo</span></div>
          <div><b>${Number(resources.waiters||0)}</b><span>Sessões aguardando recurso</span></div>
        </div>
        ${sessions.length||webSessions.length?`<div class="playback-diagnostic-grid">${sessions.map(s=>sessionCard(s,playbackBySession.get(String(s.id||'')))).join('')}${webSessions.map(s=>webSessionCard(s,playbackBySession.get(String(s.id||'')))).join('')}</div>`:'<p><small>Nenhuma sessão FFmpeg ativa. Direct Play nativo não cria processo; uma rota WASM local aparece como vídeo copiado no streaming contínuo.</small></p>'}
      `;
    }catch(err){const body=root.querySelector('[data-transcode-body]');if(body)body.innerHTML=`<p class="offline">${html(err.message||err)}</p>`}
  }

  function sessionCard(s,p){
    const title=p?.title||`Mídia #${s.media_id||'—'}`;
    const who=[p?.display_name,p?.device].filter(Boolean).join(' · ');
    const output=s.target_height?`${s.target_height}p`:s.quality||'Auto';
    return `<article><div class="playback-diagnostic-head"><div><b>${html(title)}</b><small>${html(who||s.quality||'Transcode')}</small></div><span class="mode-chip">VIDEO TRANSCODE</span></div><div class="playback-diagnostic-stats"><span><b>${html(String(s.source_video_codec||'—').toUpperCase())}</b> origem</span><span><b>${html(String(s.video_codec||'—').toUpperCase())}</b> saída</span><span><b>${html(output)}</b> qualidade</span><span><b>${rate(s.target_bitrate_kbps)}</b> alvo</span><span><b>${Number(s.fps||0).toFixed(1)}</b> FPS</span><span><b>${Number(s.speed||0).toFixed(2)}x</b> velocidade</span><span><b>${bytes(s.cache_bytes)}</b> cache</span><span><b>${Number(s.process_id||0)||'—'}</b> PID</span></div><small>${html(s.encoder||'encoder pendente')} · ${html(s.hardware||'auto')} · espera ${Number(s.resource_wait_ms||0)} ms${s.tone_map?' · tone mapping':''}${s.last_error?' · ERRO: '+html(s.last_error):''}</small></article>`;
  }

  function webSessionCard(s,p){
    const title=p?.title||`Mídia #${s.media_id||'—'}`;
    const labels={"server-video":'VIDEO TRANSCODE',"server-audio":'DIRECT STREAM · AAC',"server-remux":p?.mode==='local_decode'?'WASM LOCAL DECODE · VIDEO COPY':'DIRECT STREAM · REMUX'};
    const label=labels[String(s.route||'')]||String(s.route||'WEB STREAM').toUpperCase();
    return `<article><div class="playback-diagnostic-head"><div><b>${html(title)}</b><small>${html([p?.display_name,p?.device,s.playback_state].filter(Boolean).join(' · ')||'Streaming contínuo')}</small></div><span class="mode-chip">${html(label)}</span></div><div class="playback-diagnostic-stats"><span><b>${html(s.encoder||'pendente')}</b> encoder</span><span><b>${html(s.hardware||'auto')}</b> hardware</span><span><b>${Number(s.process_id||0)||'—'}</b> PID</span><span><b>${Number(s.ahead_segments||0)*2}s</b> buffer à frente</span><span><b>${bytes(s.cache_bytes)}</b> cache</span><span><b>${s.worker_paused?'SIM':'NÃO'}</b> worker pausado</span></div><small>Espera de recurso ${Number(s.resource_wait_ms||0)} ms${s.last_error?' · ERRO: '+html(s.last_error):''}</small></article>`;
  }

  function start(){stop();setTimeout(refresh,80);timer=setInterval(refresh,5000)}
  function stop(){if(timer){clearInterval(timer);timer=null}}

  document.querySelector('nav [data-page="automation"]')?.addEventListener('click',start);
  document.querySelectorAll('nav [data-page]').forEach(btn=>{if(btn.dataset.page!=='automation')btn.addEventListener('click',stop)});
  observer=new MutationObserver(()=>{const page=document.querySelector('#automation');if(page&&!page.classList.contains('hidden')){ensureRoot();if(!timer)start()}});
  const page=document.querySelector('#automation');if(page)observer.observe(page,{childList:true,subtree:false,attributes:true,attributeFilter:['class']});
})();
