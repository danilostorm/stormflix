/* StormFlix Admin: Playback Engine v5 / transcoding diagnostics. */
(function(){
  let timer=null;
  let observer=null;

  function html(value){return String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function bytes(value){const n=Number(value||0);if(!n)return'0 B';const units=['B','KiB','MiB','GiB','TiB'];const i=Math.min(units.length-1,Math.floor(Math.log(n)/Math.log(1024)));return`${(n/Math.pow(1024,i)).toFixed(i?1:0)} ${units[i]}`}
  function rate(value){const n=Number(value||0);return n?`${(n/1000).toFixed(n>=10000?0:1)} Mb/s`:'—'}
  function engineName(engine){return engine?.hardware_summary||'CPU'}

  function ensureRoot(){
    const page=document.querySelector('#automation');
    if(!page||page.classList.contains('hidden'))return null;
    let root=document.querySelector('#automation-transcode-v5');
    if(root)return root;
    const section=document.createElement('section');
    section.className='panel';section.id='automation-transcode-v5';
    section.innerHTML='<div class="panel-head"><div><h2>Transcodificação · Playback Engine v5</h2><p>Direct Play continua prioritário. Esta área mostra apenas sessões que realmente precisam de recodificação de vídeo.</p></div><span class="mode-chip">carregando</span></div><div data-transcode-body><small>Lendo FFmpeg, hardware e sessões…</small></div>';
    const playback=document.querySelector('#automation-playbacks')?.closest('section.panel');
    if(playback?.parentElement)playback.parentElement.insertBefore(section,playback);
    else page.appendChild(section);
    return section;
  }

  async function refresh(){
    const root=ensureRoot();if(!root)return stop();
    try{
      const [server,playbacks]=await Promise.all([req('/admin/server'),req('/admin/playbacks/diagnostics').catch(()=>[])]);
      const engine=server?.transcode_engine||{},cache=server?.transcode_cache||{},sessions=Array.isArray(server?.transcode_session_details)?server.transcode_session_details:[];
      const playbackBySession=new Map((playbacks||[]).map(p=>[String(p.playback_session_id||''),p]));
      const chip=root.querySelector('.mode-chip');
      if(chip)chip.textContent=server?.transcoding_enabled?`${sessions.length} ativa(s)`:'FFmpeg indisponível';
      const body=root.querySelector('[data-transcode-body]');if(!body)return;
      body.innerHTML=`
        <div class="technical-meter">
          <div><b>${html(engineName(engine))}</b><span>Aceleração detectada</span></div>
          <div><b>${html(engine.preferred_h264||'—')}</b><span>Encoder H.264</span></div>
          <div><b>${sessions.length}</b><span>Sessões de transcode</span></div>
          <div><b>${bytes(cache.usage_bytes)}</b><span>Cache / ${bytes(cache.max_bytes)}</span></div>
        </div>
        <p class="phase2-hint">FFmpeg: ${html(engine.ffmpeg_version||'não detectado')} · Tone mapping: ${engine.tone_map&&engine.zscale?'pronto':'limitado'} · Reserva livre: ${bytes(cache.min_free_bytes)} / ${Number(cache.min_free_percent||0)}%</p>
        ${sessions.length?`<div class="playback-diagnostic-grid">${sessions.map(s=>sessionCard(s,playbackBySession.get(String(s.id||'')))).join('')}</div>`:'<p><small>Nenhuma sessão está transcodificando vídeo agora. Direct Play, Remux e áudio AAC não aparecem nesta lista.</small></p>'}
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