/* StormFlix Admin: Playback Engine v6 / adaptive decode diagnostics. */
(function(){
  let timer=null;
  let observer=null;

  function html(value){return String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function bytes(value){const n=Number(value||0);if(!n)return'0 B';const units=['B','KiB','MiB','GiB','TiB'];const i=Math.min(units.length-1,Math.floor(Math.log(n)/Math.log(1024)));return`${(n/Math.pow(1024,i)).toFixed(i?1:0)} ${units[i]}`}
  function rate(value){const n=Number(value||0);return n?`${(n/1000).toFixed(n>=10000?0:1)} Mb/s`:'—'}
  function engineName(engine){return engine?.hardware_summary||'CPU'}
  function localEnabled(){return localStorage.getItem('stormflix.player.local_decode')!=='off'}
  function local4K(){return localStorage.getItem('stormflix.player.local_decode_4k')!=='off'}

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
    section.addEventListener('click',event=>{
      const local=event.target.closest?.('[data-v6-local]');
      if(local){localStorage.setItem('stormflix.player.local_decode',localEnabled()?'off':'on');refresh();return}
      const four=event.target.closest?.('[data-v6-local-4k]');
      if(four){localStorage.setItem('stormflix.player.local_decode_4k',local4K()?'off':'on');refresh()}
    });
    return section;
  }

  function browserLocalSummary(){
    const secure=Boolean(window.isSecureContext||location.hostname==='localhost'||location.hostname==='127.0.0.1');
    const webcodecs=typeof VideoEncoder==='function'&&typeof VideoDecoder==='function';
    const wasm=typeof WebAssembly==='object'&&typeof Worker==='function';
    return{secure,webcodecs,wasm,cores:Number(navigator.hardwareConcurrency||0),memory:Number(navigator.deviceMemory||0)};
  }

  async function refresh(){
    const root=ensureRoot();if(!root)return stop();
    try{
      const [server,playbacks]=await Promise.all([req('/admin/server'),req('/admin/playbacks/diagnostics').catch(()=>[])]);
      const engine=server?.transcode_engine||{},cache=server?.transcode_cache||{},sessions=Array.isArray(server?.transcode_session_details)?server.transcode_session_details:[];
      const playbackBySession=new Map((playbacks||[]).map(p=>[String(p.playback_session_id||''),p]));
      const local=browserLocalSummary();
      const chip=root.querySelector('.mode-chip');
      if(chip)chip.textContent=`v6 · ${sessions.length} transcode(s)`;
      const body=root.querySelector('[data-transcode-body]');if(!body)return;
      body.innerHTML=`
        <div class="technical-meter">
          <div><b>${localEnabled()?'ATIVO':'DESLIGADO'}</b><span>WASM local neste navegador</span></div>
          <div><b>${local.secure&&local.webcodecs&&local.wasm?'PRONTO':'FALLBACK'}</b><span>WebCodecs / contexto seguro</span></div>
          <div><b>${html(engineName(engine))}</b><span>Aceleração do servidor</span></div>
          <div><b>${sessions.length}</b><span>Transcodes de vídeo</span></div>
        </div>
        <div class="games-admin-actions" style="margin:12px 0;display:flex;gap:8px;flex-wrap:wrap">
          <button type="button" data-v6-local>${localEnabled()?'Desativar':'Ativar'} decode local WASM</button>
          <button type="button" data-v6-local-4k>${local4K()?'Desativar':'Ativar'} 4K local automático</button>
        </div>
        <p class="phase2-hint">Cliente: ${local.cores||'—'} threads · ${local.memory?local.memory+' GB RAM estimada':'RAM não informada'} · HTTPS/contexto seguro: ${local.secure?'sim':'não'} · WebCodecs: ${local.webcodecs?'sim':'não'}. HDR local permanece conservador/desativado; HDR incompatível continua no caminho seguro de tone mapping do servidor.</p>
        <div class="technical-meter">
          <div><b>${html(engine.preferred_h264||'—')}</b><span>Encoder H.264 servidor</span></div>
          <div><b>${html(engine.ffmpeg_version||'não detectado')}</b><span>FFmpeg</span></div>
          <div><b>${bytes(cache.usage_bytes)}</b><span>Cache / ${bytes(cache.max_bytes)}</span></div>
          <div><b>${engine.tone_map&&engine.zscale?'PRONTO':'LIMITADO'}</b><span>Tone mapping</span></div>
        </div>
        ${sessions.length?`<div class="playback-diagnostic-grid">${sessions.map(s=>sessionCard(s,playbackBySession.get(String(s.id||'')))).join('')}</div>`:'<p><small>Nenhuma sessão está transcodificando vídeo agora. Direct Play, WASM local, Remux e áudio AAC não aparecem nesta lista de transcode de vídeo.</small></p>'}
      `;
    }catch(err){const body=root.querySelector('[data-transcode-body]');if(body)body.innerHTML=`<p class="offline">${html(err.message||err)}</p>`}
  }

  function sessionCard(s,p){
    const title=p?.title||`Mídia #${s.media_id||'—'}`;
    const who=[p?.display_name,p?.device].filter(Boolean).join(' · ');
    const output=s.target_height?`${s.target_height}p`:s.quality||'Auto';
    return `<article><div class="playback-diagnostic-head"><div><b>${html(title)}</b><small>${html(who||s.quality||'Transcode')}</small></div><span class="mode-chip">VIDEO TRANSCODE</span></div><div class="playback-diagnostic-stats"><span><b>${html(String(s.source_video_codec||'—').toUpperCase())}</b> origem</span><span><b>${html(String(s.video_codec||'—').toUpperCase())}</b> saída</span><span><b>${html(output)}</b> qualidade</span><span><b>${rate(s.target_bitrate_kbps)}</b> alvo</span><span><b>${Number(s.fps||0).toFixed(1)}</b> FPS</span><span><b>${Number(s.speed||0).toFixed(2)}x</b> velocidade</span><span><b>${bytes(s.cache_bytes)}</b> cache</span></div><small>${html(s.encoder||'encoder pendente')} · ${html(s.hardware||'auto')}${s.tone_map?' · tone mapping':''}${s.last_error?' · ERRO: '+html(s.last_error):''}</small></article>`;
  }

  function start(){stop();setTimeout(refresh,80);timer=setInterval(refresh,5000)}
  function stop(){if(timer){clearInterval(timer);timer=null}}

  document.querySelector('nav [data-page="automation"]')?.addEventListener('click',start);
  document.querySelectorAll('nav [data-page]').forEach(btn=>{if(btn.dataset.page!=='automation')btn.addEventListener('click',stop)});
  observer=new MutationObserver(()=>{const page=document.querySelector('#automation');if(page&&!page.classList.contains('hidden')){ensureRoot();if(!timer)start()}});
  const page=document.querySelector('#automation');if(page)observer.observe(page,{childList:true,subtree:false,attributes:true,attributeFilter:['class']});
})();
