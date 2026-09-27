/* StormFlix Player v7 — cinematic UX over original-file Playback Core. */
(function(){
  const modal=document.querySelector('#player-modal');
  const video=document.querySelector('#player');
  if(!modal||!video||modal.dataset.sfV5==='1')return;
  modal.dataset.sfV5='1';
  modal.classList.add('sf-player-v5');

  const diagnosticsButton=document.createElement('button');
  diagnosticsButton.id='sf-v5-diagnostics-toggle';diagnosticsButton.type='button';diagnosticsButton.className='sf-control-btn sf-v5-diagnostics-btn';diagnosticsButton.title='Diagnóstico';diagnosticsButton.textContent='i';
  const fullscreen=document.querySelector('#sf-fullscreen');
  if(fullscreen?.parentElement)fullscreen.parentElement.insertBefore(diagnosticsButton,fullscreen);

  const diagnostics=document.createElement('aside');
  diagnostics.id='sf-v5-diagnostics';diagnostics.className='sf-v5-diagnostics hidden';
  diagnostics.innerHTML='<header><div><b>Diagnóstico de reprodução</b><small>Playback Engine v7</small></div><button type="button" data-v5-diag-close>×</button></header><div id="sf-v5-diag-body"></div>';
  modal.appendChild(diagnostics);

  const ambient=document.createElement('div');ambient.className='sf-v5-ambient';modal.insertBefore(ambient,modal.firstChild);
  const vignette=document.createElement('div');vignette.className='sf-v5-vignette';modal.insertBefore(vignette,modal.firstChild);

  function qualityLabel(){return 'Original'}
  function plan(){return window.sfLastPlaybackPlan||window.sfPlaybackCore?.currentPlan?.()||{}}
  function modeLabel(mode,p=plan()){
    if(p?.local_origin)return'DECODE LOCAL · ORIGINAL';
    if(p?.local_decode)return'WASM LOCAL DECODE';
    return({direct_play:'DIRECT PLAY',local_decode:'WASM LOCAL DECODE',remux:'DIRECT STREAM · REMUX',audio_compatibility:'DIRECT STREAM · AAC',video_transcode:'VIDEO TRANSCODE',unsupported:'SEM ROTA'})[mode]||String(mode||'STORMFLIX').replaceAll('_',' ').toUpperCase();
  }
  function transportLabel(value){return({hls:'HLS sob demanda',progressive_mp4:'MP4 seekable',original_range:'Arquivo original · HTTP Range'})[String(value||'')]||'Automático'}
  function escapeHtml(value){return String(value??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]))}
  function formatRate(kbps){const n=Number(kbps||0);return n?`${(n/1000).toFixed(n>=10000?0:1)} Mb/s`:'—'}
  function yesNo(v){return v?'Sim':'Não'}

  function refreshPlan(){
    const p=plan();
    const chip=document.querySelector('#sf-v4-plan');
    if(chip){chip.textContent=modeLabel(p.mode,p);chip.dataset.mode=p.local_decode?'local_decode':p.mode||''}
    const detail=document.querySelector('#sf-v4-playback-detail');
    if(detail){
      const source=String(p.source_video_codec||p.video_codec||'').toUpperCase();
      const target=p.video_transcode&&p.video_codec?` → ${String(p.video_codec).toUpperCase()}`:'';
      const resolution=p.target_video_height?`${p.target_video_height}p`:(p.video_height?`${p.video_height}p`:'');
      const transport=p.transport?transportLabel(p.transport):'';
      detail.textContent=[modeLabel(p.mode,p),transport,source+target,resolution,String(p.audio_codec||'').toUpperCase()].filter(Boolean).join(' · ');
    }
    renderDiagnostics();
  }

  function renderDiagnostics(){
    const p=plan(),root=document.querySelector('#sf-v5-diag-body');if(!root)return;
    const reasons=Array.isArray(p.transcode_reasons)?p.transcode_reasons:[];
    const activeQuality=window.sfPlaybackCore?.currentQuality?.()||p.quality||'auto';
    const local=window.sfLocalDecodeStats||{};
    root.innerHTML=`
      <div class="sf-v5-diag-hero"><span class="sf-v5-mode ${escapeHtml(p.local_decode?'local_decode':p.mode||'')}">${escapeHtml(modeLabel(p.mode,p))}</span><b>${escapeHtml(p.reason||'Aguardando PlaybackPlan…')}</b></div>
      <div class="sf-v5-diag-grid">
        ${diag('Origem',`${escapeHtml(String(p.source_video_codec||p.video_codec||'—').toUpperCase())} · ${p.video_width||'—'}×${p.video_height||'—'}`)}
        ${diag('Saída',p.local_decode?'Decodificação no dispositivo':`${escapeHtml(String(p.video_codec||'—').toUpperCase())} · ${p.target_video_width||p.video_width||'—'}×${p.target_video_height||p.video_height||'—'}`)}
        ${diag('Transporte',escapeHtml(transportLabel(p.transport)))}
        ${diag('Bitrate origem',formatRate(p.source_bitrate_kbps))}
        ${diag('Bitrate alvo',p.local_decode?'Original':formatRate(p.target_bitrate_kbps))}
        ${diag('Áudio',`${escapeHtml(String(p.source_audio_codec||p.audio_codec||'—').toUpperCase())}${p.audio_transcode?' → '+escapeHtml(String(p.audio_codec||'AAC').toUpperCase()):''}`)}
        ${diag('Decoder',p.local_decode?escapeHtml(p.local_decode_engine||'stormflix-v6-wasm'):escapeHtml(p.encoder||'Nativo / copy'))}
        ${diag('Buffer local',p.local_origin?`${Number(local.buffer_seconds||0).toFixed(1)} s`:p.local_decode&&Number(local.speed_x)>0?`${Number(local.speed_x).toFixed(2)}x`:'—')}
        ${diag('Frames perdidos',p.local_origin?String(Number(local.dropped_frames||0)):'—')}
        ${diag('CPU/GPU servidor',p.local_decode?'Não usados para decodificar vídeo':escapeHtml(p.hardware_acceleration||'Auto'))}
        ${diag('Tone mapping',yesNo(p.tone_map))}
        ${diag('Qualidade',escapeHtml(qualityLabel(activeQuality)))}
        ${diag('Sessão',escapeHtml(p.playback_session_id||'—'))}
      </div>
      ${reasons.length?`<div class="sf-v5-reasons"><b>Motivos da compatibilidade</b>${reasons.map(x=>`<span>${escapeHtml(String(x).replaceAll('_',' '))}</span>`).join('')}</div>`:''}`;
  }
  function diag(label,value){return`<div><span>${label}</span><b>${value}</b></div>`}

  function toggleDiagnostics(show){
    const shouldShow=show===undefined?diagnostics.classList.contains('hidden'):show;
    diagnostics.classList.toggle('hidden',!shouldShow);
    if(shouldShow)renderDiagnostics();
  }

  diagnosticsButton.addEventListener('click',e=>{e.stopPropagation();toggleDiagnostics()});
  diagnostics.querySelector('[data-v5-diag-close]').onclick=()=>toggleDiagnostics(false);

  window.addEventListener('stormflix:playback-plan',refreshPlan);
  window.addEventListener('stormflix:local-decode-stat',renderDiagnostics);
  video.addEventListener('loadedmetadata',refreshPlan,{passive:true});
  video.addEventListener('playing',refreshPlan,{passive:true});

  document.addEventListener('keydown',e=>{
    if(modal.classList.contains('hidden')||e.ctrlKey||e.metaKey||e.altKey)return;
    const tag=String(e.target?.tagName||'').toLowerCase();if(tag==='input'||tag==='textarea'||tag==='select')return;
    switch(e.key.toLowerCase()){
      case ' ':
      case 'k':e.preventDefault();video.paused?video.play().catch(()=>{}):video.pause();break;
      case 'arrowleft':e.preventDefault();video.currentTime=Math.max(0,(video.currentTime||0)-10);break;
      case 'arrowright':e.preventDefault();video.currentTime=Math.min(video.duration||Infinity,(video.currentTime||0)+10);break;
      case 'j':video.currentTime=Math.max(0,(video.currentTime||0)-10);break;
      case 'l':video.currentTime=Math.min(video.duration||Infinity,(video.currentTime||0)+10);break;
      case 'm':video.muted=!video.muted;break;
      case 'f':document.querySelector('#sf-fullscreen')?.click();break;
      case 'i':toggleDiagnostics();break;
      case 'escape':toggleDiagnostics(false);break;
    }
  });

  modal.addEventListener('keydown',e=>{
    if(!['ArrowLeft','ArrowRight'].includes(e.key))return;
    const active=document.activeElement;if(!(active instanceof HTMLButtonElement))return;
    const buttons=[...modal.querySelectorAll('button:not(.hidden):not([disabled])')].filter(b=>b.offsetParent!==null);
    const idx=buttons.indexOf(active);if(idx<0)return;
    e.preventDefault();buttons[(idx+(e.key==='ArrowRight'?1:-1)+buttons.length)%buttons.length]?.focus();
  });

  refreshPlan();
})();
