/* StormFlix Tautulli-inspired activity monitoring */
(function(){
  let monitorTimer=null;
  const baseShow=show;
  show=async function(name){
    if(monitorTimer){clearTimeout(monitorTimer);monitorTimer=null}
    await baseShow(name);
    if(name==='playbacks')await loadMonitoring();
  };

  loadPlaybacks=async function(){await loadMonitoring()};

  async function loadMonitoring(){
    try{
      const data=await req('/admin/monitoring');
      const s=data.stats||{},active=data.active||[],history=data.history||[];
      $('#playbacks').innerHTML=`
        <div class="monitor-cards">
          ${monitorCard('Streams ativos',s.active_streams||0,'agora')}
          ${monitorCard('Direct Play',s.direct_play_streams||0,'sem transcodificação')}
          ${monitorCard('Web Remux',s.web_remux_streams||0,'-c copy')}
          ${monitorCard('Banda estimada',monitorBitrate(s.bandwidth_kbps||0),'ativos')}
          ${monitorCard('Reproduções hoje',s.plays_today||0,'histórico')}
          ${monitorCard('Tempo assistido 7d',monitorDuration(s.watch_seconds_7_days||0),`${s.unique_users_7_days||0} usuários`)}
        </div>
        <div class="panel monitor-panel">
          <div class="panel-head"><div><h2>Atividade agora</h2><small>Atualização automática a cada 8 segundos · estilo Tautulli</small></div><span class="monitor-live">● AO VIVO</span></div>
          <div class="monitor-active">${active.length?active.map(activePlaybackHTML).join(''):'<div class="monitor-empty">Ninguém está assistindo agora.</div>'}</div>
        </div>
        <div class="panel">
          <div class="panel-head"><div><h2>Histórico recente</h2><small>Base para estatísticas, usuários, dispositivos e relatórios.</small></div></div>
          <div class="table-wrap"><table><thead><tr><th>Quando</th><th>Usuário</th><th>Mídia</th><th>Dispositivo / IP</th><th>Modo</th><th>Progresso</th><th>Assistido</th></tr></thead><tbody>
          ${history.length?history.map(historyHTML).join(''):'<tr><td colspan="7"><small>Nenhum histórico concluído ainda.</small></td></tr>'}
          </tbody></table></div>
        </div>`;
      if(!$('#playbacks').classList.contains('hidden'))monitorTimer=setTimeout(loadMonitoring,8000);
    }catch(err){notice(err.message)}
  }
  window.loadMonitoring=loadMonitoring;

  function activePlaybackHTML(p){
    const poster=p.poster_url?`<img src="${esc(p.poster_url)}" alt="">`:'<div class="monitor-poster-fallback">S<span>F</span></div>';
    const progress=Math.max(0,Math.min(100,Number(p.progress_percent)||0));
    const state=p.state==='paused'?'PAUSADO':'REPRODUZINDO';
    const mode=p.mode==='web_remux'?'WEB REMUX':'DIRECT PLAY';
    const technical=[p.resolution,String(p.video_codec||'').toUpperCase(),String(p.audio_codec||'').toUpperCase(),p.audio_language,p.subtitle_language?`CC ${p.subtitle_language}`:''].filter(Boolean).join(' · ');
    return `<article class="monitor-session">
      <div class="monitor-poster">${poster}</div>
      <div class="monitor-session-main">
        <div class="monitor-session-top"><div><span class="monitor-state ${p.state==='paused'?'paused':''}">${state}</span><h3>${esc(p.title)}</h3><p>${esc(p.library_name||'')}</p></div><div class="monitor-mode ${p.mode==='web_remux'?'remux':''}">${mode}</div></div>
        <div class="monitor-progress"><span style="width:${progress}%"></span></div>
        <div class="monitor-time"><b>${monitorClock(p.position_seconds)}</b><span>${progress.toFixed(0)}%</span><b>${monitorClock(p.duration_seconds)}</b></div>
        <div class="monitor-meta"><span>👤 ${esc(p.display_name)}</span><span>▣ ${esc(p.device)}</span><span>⌁ ${esc(p.ip)}</span><span>⇅ ${monitorBitrate(p.bitrate_kbps)}</span></div>
        <div class="monitor-tech">${esc(technical||'Informações técnicas chegando pelo player…')}</div>
      </div>
    </article>`;
  }

  function historyHTML(h){
    return `<tr><td><small>${esc(h.stopped_at)}</small></td><td><b>${esc(h.display_name)}</b></td><td><b>${esc(h.title)}</b><br><small>${esc(h.library_name)}</small></td><td><small>${esc(h.device)}<br>${esc(h.ip)}</small></td><td><span class="monitor-mode mini ${h.mode==='web_remux'?'remux':''}">${h.mode==='web_remux'?'WEB REMUX':'DIRECT PLAY'}</span></td><td>${Number(h.progress_percent||0).toFixed(0)}%</td><td>${monitorDuration(h.watch_seconds||0)}</td></tr>`;
  }

  function monitorCard(label,value,sub){return `<div class="monitor-card"><strong>${value}</strong><span>${label}</span><small>${esc(sub||'')}</small></div>`}
  function monitorBitrate(kbps){kbps=Number(kbps)||0;return kbps>=1000?`${(kbps/1000).toFixed(1)} Mbps`:`${Math.round(kbps)} Kbps`}
  function monitorDuration(seconds){seconds=Math.max(0,Math.floor(Number(seconds)||0));const d=Math.floor(seconds/86400),h=Math.floor(seconds%86400/3600),m=Math.floor(seconds%3600/60);return d?`${d}d ${h}h`:h?`${h}h ${m}m`:`${m} min`}
  function monitorClock(seconds){seconds=Math.max(0,Math.floor(Number(seconds)||0));const h=Math.floor(seconds/3600),m=Math.floor(seconds%3600/60),s=seconds%60;return h?`${h}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`:`${m}:${String(s).padStart(2,'0')}`}
})();
