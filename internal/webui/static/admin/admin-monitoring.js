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
      const s=data.stats||{},active=dedupeActive(data.active||[]),history=data.history||[],analytics=data.analytics||{};
      $('#playbacks').innerHTML=`
        <div class="monitor-cards">
          ${monitorCard('Streams ativos',active.length,'agora')}
          ${monitorCard('Direct Play',s.direct_play_streams||0,'sem conversão')}
          ${monitorCard('Áudio AAC',s.audio_aac_streams||0,'vídeo original')}
          ${monitorCard('Web Remux',s.web_remux_streams||0,'-c copy')}
          ${monitorCard('Música',s.music_streams||0,'tocando agora')}
          ${monitorCard('Banda estimada',monitorBitrate(s.bandwidth_kbps||0),'ativos')}
          ${monitorCard('Reproduções hoje',s.plays_today||0,'histórico')}
          ${monitorCard('Tempo assistido 7d',monitorDuration(s.watch_seconds_7_days||0),`${s.unique_users_7_days||0} usuários`)}
        </div>
        <div class="panel monitor-panel">
          <div class="panel-head"><div><h2>Atividade agora</h2><small>Atualização automática a cada 8 segundos · sessões deduplicadas · dados técnicos em tempo real</small></div><span class="monitor-live">● AO VIVO</span></div>
          <div class="monitor-active">${active.length?active.map(activePlaybackHTML).join(''):'<div class="monitor-empty">Ninguém está assistindo ou ouvindo agora.</div>'}</div>
        </div>
        ${analyticsHTML(analytics)}
        <div class="panel">
          <div class="panel-head"><div><h2>Histórico recente</h2><small>Usuário, dispositivo, IP, modo de reprodução, progresso e tempo assistido.</small></div></div>
          <div class="table-wrap"><table><thead><tr><th>Quando</th><th>Usuário</th><th>Mídia</th><th>Dispositivo / IP</th><th>Modo</th><th>Progresso</th><th>Assistido</th></tr></thead><tbody>
          ${history.length?history.map(historyHTML).join(''):'<tr><td colspan="7"><small>Nenhum histórico concluído ainda.</small></td></tr>'}
          </tbody></table></div>
        </div>`;
      if(!$('#playbacks').classList.contains('hidden'))monitorTimer=setTimeout(loadMonitoring,8000);
    }catch(err){notice(err.message)}
  }
  window.loadMonitoring=loadMonitoring;

  function dedupeActive(items){
    const seen=new Set(),out=[];
    for(const p of items){
      const family=deviceFamily(p.device);
      const key=[p.user_id,p.media_id,p.ip||'',family].join('|');
      if(seen.has(key))continue;
      seen.add(key);out.push(p);
    }
    return out;
  }

  function deviceFamily(value){
    const raw=String(value||'').trim(),v=raw.toLowerCase().replace(/[-_/]+/g,' ');
    if(v.includes('stormflix android'))return'stormflix android';
    if(v.includes('stormflix desktop'))return'stormflix desktop';
    return v;
  }

  function activePlaybackHTML(p){
    const isMusic=String(p.media_kind||'').toLowerCase()==='music'||p.mode==='music';
    const poster=p.poster_url?`<img src="${esc(p.poster_url)}" alt="">`:`<div class="monitor-poster-fallback">${isMusic?'♪':'S<span>F</span>'}</div>`;
    const progress=Math.max(0,Math.min(100,Number(p.progress_percent)||0));
    const state=p.state==='paused'?'PAUSADO':isMusic?'OUVINDO':'REPRODUZINDO';
    const subtitle=isMusic?[p.artist,p.album].filter(Boolean).join(' · '):(p.library_name||'');
    const mode=modeInfo(p.mode,isMusic);
    const sourceAudio=String(p.source_audio_codec||p.audio_codec||'').toUpperCase();
    const outputAudio=String(p.audio_codec||'').toUpperCase();
    const audioText=sourceAudio&&outputAudio&&sourceAudio!==outputAudio?`${sourceAudio} → ${outputAudio}`:(outputAudio||sourceAudio);
    const tech=[
      p.resolution?techChip('Tela',p.resolution):'',
      p.video_codec?techChip('Vídeo',String(p.video_codec).toUpperCase()):'',
      audioText?techChip('Áudio',audioText):'',
      p.audio_language?techChip('Idioma',p.audio_language):'',
      p.subtitle_language?techChip('Legenda',p.subtitle_language):'',
      p.bitrate_kbps?techChip('Bitrate',monitorBitrate(p.bitrate_kbps)):''
    ].filter(Boolean).join('');
    return `<article class="monitor-session ${isMusic?'music':''}">
      <div class="monitor-poster">${poster}</div>
      <div class="monitor-session-main">
        <div class="monitor-session-top"><div><span class="monitor-state ${p.state==='paused'?'paused':''}">${state}</span><h3>${esc(p.title)}</h3><p>${esc(subtitle)}</p></div><div class="monitor-mode ${mode.className}">${mode.label}</div></div>
        <div class="monitor-progress"><span style="width:${progress}%"></span></div>
        <div class="monitor-time"><b>${monitorClock(p.position_seconds)}</b><span>${progress.toFixed(0)}%</span><b>${monitorClock(p.duration_seconds)}</b></div>
        <div class="monitor-meta"><span>👤 ${esc(p.display_name)}</span><span>▣ ${esc(p.device)}</span><span>⌁ ${esc(p.ip)}</span></div>
        <div class="monitor-tech">${tech||'<span class="monitor-tech-wait">Informações técnicas chegando pelo player…</span>'}</div>
      </div>
    </article>`;
  }

  function techChip(label,value){return `<span class="monitor-tech-chip"><b>${esc(label)}</b>${esc(value)}</span>`}

  function modeInfo(value,isMusic=false){
    if(isMusic||value==='music')return{label:'MÚSICA',className:'music'};
    if(value==='audio_aac'||String(value||'').includes('aac'))return{label:'ÁUDIO AAC',className:'aac'};
    if(value==='web_remux')return{label:'WEB REMUX',className:'remux'};
    return{label:'DIRECT PLAY',className:''};
  }

  function analyticsHTML(a){
    const daily=a.daily||[],maxPlays=Math.max(1,...daily.map(d=>Number(d.plays)||0));
    return `<div class="monitor-analytics-grid">
      <div class="panel monitor-ranking"><div class="panel-head"><h2>Top usuários · 7 dias</h2></div>${rankHTML(a.top_users||[])}</div>
      <div class="panel monitor-ranking"><div class="panel-head"><h2>Top títulos · 7 dias</h2></div>${rankHTML(a.top_media||[])}</div>
      <div class="panel monitor-ranking"><div class="panel-head"><h2>Top bibliotecas · 7 dias</h2></div>${rankHTML(a.top_libraries||[])}</div>
      <div class="panel monitor-week"><div class="panel-head"><h2>Atividade · 7 dias</h2></div><div class="monitor-bars">${daily.map(d=>`<div class="monitor-day" title="${d.plays} reproduções · ${monitorDuration(d.watch_seconds)}"><div class="monitor-bar-wrap"><span style="height:${Math.max(4,(Number(d.plays)||0)/maxPlays*100)}%"></span></div><b>${Number(d.plays)||0}</b><small>${shortDate(d.date)}</small></div>`).join('')||'<div class="monitor-empty">Sem atividade.</div>'}</div></div>
    </div>`;
  }

  function rankHTML(items){
    if(!items.length)return'<div class="monitor-empty compact">Sem dados ainda.</div>';
    const max=Math.max(1,...items.map(x=>Number(x.watch_seconds)||0));
    return `<div class="monitor-rank-list">${items.map((x,i)=>`<div class="monitor-rank"><span class="monitor-rank-number">${i+1}</span><div><b>${esc(x.label)}</b><div class="monitor-rank-bar"><span style="width:${Math.max(3,(Number(x.watch_seconds)||0)/max*100)}%"></span></div><small>${x.plays} plays · ${monitorDuration(x.watch_seconds)}</small></div></div>`).join('')}</div>`;
  }

  function historyHTML(h){
    const mode=modeInfo(h.mode,false);
    return `<tr><td><small>${esc(h.stopped_at)}</small></td><td><b>${esc(h.display_name)}</b></td><td><b>${esc(h.title)}</b><br><small>${esc(h.library_name)}</small></td><td><small>${esc(h.device)}<br>${esc(h.ip)}</small></td><td><span class="monitor-mode mini ${mode.className}">${mode.label}</span></td><td>${Number(h.progress_percent||0).toFixed(0)}%</td><td>${monitorDuration(h.watch_seconds||0)}</td></tr>`;
  }

  function monitorCard(label,value,sub){return `<div class="monitor-card"><strong>${value}</strong><span>${label}</span><small>${esc(sub||'')}</small></div>`}
  function monitorBitrate(kbps){kbps=Number(kbps)||0;return kbps>=1000?`${(kbps/1000).toFixed(1)} Mbps`:`${Math.round(kbps)} Kbps`}
  function monitorDuration(seconds){seconds=Math.max(0,Math.floor(Number(seconds)||0));const d=Math.floor(seconds/86400),h=Math.floor(seconds%86400/3600),m=Math.floor(seconds%3600/60);return d?`${d}d ${h}h`:h?`${h}h ${m}m`:`${m} min`}
  function monitorClock(seconds){seconds=Math.max(0,Math.floor(Number(seconds)||0));const h=Math.floor(seconds/3600),m=Math.floor(seconds%3600/60),s=seconds%60;return h?`${h}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`:`${m}:${String(s).padStart(2,'0')}`}
  function shortDate(value){const parts=String(value||'').split('-');return parts.length===3?`${parts[2]}/${parts[1]}`:value}
})();
