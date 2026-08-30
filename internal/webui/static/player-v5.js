/* StormFlix Player v5.1 — cinematic UX over the unified Playback Core. */
(function(){
  const modal=document.querySelector('#player-modal');
  const video=document.querySelector('#player');
  if(!modal||!video||modal.dataset.sfV5==='1')return;
  modal.dataset.sfV5='1';
  modal.classList.add('sf-player-v5');

  const qualityValues=[
    ['auto','Auto'],['original','Original'],['2160p','4K'],['1440p','1440p'],['1080p','1080p'],['720p','720p'],['480p','480p']
  ];
  const qualityLabels=Object.fromEntries(qualityValues);
  const qualityButton=document.createElement('button');
  qualityButton.id='sf-v5-quality';qualityButton.type='button';qualityButton.className='sf-control-btn sf-v5-quality-btn';qualityButton.title='Qualidade';
  qualityButton.innerHTML='<span class="sf-v5-quality-value">AUTO</span><small>Qualidade</small>';
  const settings=document.querySelector('#sf-settings');
  if(settings?.parentElement)settings.parentElement.insertBefore(qualityButton,settings);

  const diagnosticsButton=document.createElement('button');
  diagnosticsButton.id='sf-v5-diagnostics-toggle';diagnosticsButton.type='button';diagnosticsButton.className='sf-control-btn sf-v5-diagnostics-btn';diagnosticsButton.title='Diagnóstico';diagnosticsButton.textContent='i';
  const fullscreen=document.querySelector('#sf-fullscreen');
  if(fullscreen?.parentElement)fullscreen.parentElement.insertBefore(diagnosticsButton,fullscreen);

  const qualityMenu=document.createElement('div');
  qualityMenu.id='sf-v5-quality-menu';qualityMenu.className='sf-v5-popover hidden';
  qualityMenu.innerHTML='<header><div><b>Qualidade</b><small>Mostramos somente resoluções compatíveis com a fonte.</small></div><button type="button" data-v5-close>×</button></header><div class="sf-v5-quality-list"></div>';
  modal.appendChild(qualityMenu);

  const diagnostics=document.createElement('aside');
  diagnostics.id='sf-v5-diagnostics';diagnostics.className='sf-v5-diagnostics hidden';
  diagnostics.innerHTML='<header><div><b>Diagnóstico de reprodução</b><small>PlaybackPlan v5.1</small></div><button type="button" data-v5-diag-close>×</button></header><div id="sf-v5-diag-body"></div>';
  modal.appendChild(diagnostics);

  const ambient=document.createElement('div');ambient.className='sf-v5-ambient';modal.insertBefore(ambient,modal.firstChild);
  const vignette=document.createElement('div');vignette.className='sf-v5-vignette';modal.insertBefore(vignette,modal.firstChild);

  function qualityHint(value){
    return ({auto:'Melhor rota automática',original:'Preservar a fonte quando possível','2160p':'Até 2160p','1440p':'Até 1440p','1080p':'Até 1080p','720p':'Até 720p','480p':'Economia de dados'})[value]||'';
  }
  function qualityLabel(value){return qualityLabels[String(value||'auto')]||'Auto'}
  function plan(){return window.sfLastPlaybackPlan||window.sfPlaybackCore?.currentPlan?.()||{}}
  function modeLabel(mode){return({direct_play:'DIRECT PLAY',remux:'DIRECT STREAM · REMUX',audio_compatibility:'DIRECT STREAM · AAC',video_transcode:'VIDEO TRANSCODE',unsupported:'SEM ROTA'})[mode]||String(mode||'STORMFLIX').replaceAll('_',' ').toUpperCase()}
  function transportLabel(value){return({hls:'HLS sob demanda',progressive_mp4:'MP4 seekable'})[String(value||'')]||'Automático'}
  function escapeHtml(value){return String(value??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]))}
  function formatRate(kbps){const n=Number(kbps||0);return n?`${(n/1000).toFixed(n>=10000?0:1)} Mb/s`:'—'}
  function yesNo(v){return v?'Sim':'Não'}

  function fallbackAvailableQualities(){
    const p=plan(),height=Number(p.video_height||video.videoHeight||0),values=['auto','original'];
    for(const [minimum,value] of [[2160,'2160p'],[1440,'1440p'],[1080,'1080p'],[720,'720p'],[480,'480p']])if(height>=minimum)values.push(value);
    return values;
  }

  function availableQualities(){
    const fromCore=window.sfPlaybackCore?.availableQualities?.();
    if(Array.isArray(fromCore)&&fromCore.length)return fromCore;
    const supplied=plan().available_qualities;
    if(Array.isArray(supplied)&&supplied.length)return supplied;
    return fallbackAvailableQualities();
  }

  function renderQualityOptions(){
    const list=qualityMenu.querySelector('.sf-v5-quality-list');if(!list)return;
    const allowed=new Set(availableQualities().map(String));
    list.innerHTML=qualityValues.filter(([value])=>allowed.has(value)).map(([value,label])=>`<button type="button" data-v5-quality="${value}"><span>${label}</span><small>${qualityHint(value)}</small></button>`).join('');
  }

  function refreshQuality(){
    renderQualityOptions();
    const current=window.sfPlaybackCore?.currentQuality?.()||'auto';
    const value=qualityButton.querySelector('.sf-v5-quality-value');if(value)value.textContent=qualityLabel(current).toUpperCase();
    qualityMenu.querySelectorAll('[data-v5-quality]').forEach(btn=>btn.classList.toggle('active',btn.dataset.v5Quality===current));
  }

  function refreshPlan(){
    const p=plan();
    const chip=document.querySelector('#sf-v4-plan');
    if(chip){chip.textContent=modeLabel(p.mode);chip.dataset.mode=p.mode||''}
    const detail=document.querySelector('#sf-v4-playback-detail');
    if(detail){
      const source=String(p.source_video_codec||p.video_codec||'').toUpperCase();
      const target=p.video_transcode&&p.video_codec?` → ${String(p.video_codec).toUpperCase()}`:'';
      const resolution=p.target_video_height?`${p.target_video_height}p`:(p.video_height?`${p.video_height}p`:'');
      const transport=p.transport?transportLabel(p.transport):'';
      detail.textContent=[modeLabel(p.mode),transport,source+target,resolution,String(p.audio_codec||'').toUpperCase()].filter(Boolean).join(' · ');
    }
    renderDiagnostics();refreshQuality();
  }

  function renderDiagnostics(){
    const p=plan(),root=document.querySelector('#sf-v5-diag-body');if(!root)return;
    const reasons=Array.isArray(p.transcode_reasons)?p.transcode_reasons:[];
    const activeQuality=window.sfPlaybackCore?.currentQuality?.()||p.quality||'auto';
    root.innerHTML=`
      <div class="sf-v5-diag-hero"><span class="sf-v5-mode ${escapeHtml(p.mode||'')}">${escapeHtml(modeLabel(p.mode))}</span><b>${escapeHtml(p.reason||'Aguardando PlaybackPlan…')}</b></div>
      <div class="sf-v5-diag-grid">
        ${diag('Origem',`${escapeHtml(String(p.source_video_codec||p.video_codec||'—').toUpperCase())} · ${p.video_width||'—'}×${p.video_height||'—'}`)}
        ${diag('Saída',`${escapeHtml(String(p.video_codec||'—').toUpperCase())} · ${p.target_video_width||p.video_width||'—'}×${p.target_video_height||p.video_height||'—'}`)}
        ${diag('Transporte',escapeHtml(transportLabel(p.transport)))}
        ${diag('Bitrate origem',formatRate(p.source_bitrate_kbps))}
        ${diag('Bitrate alvo',formatRate(p.target_bitrate_kbps))}
        ${diag('Áudio',`${escapeHtml(String(p.source_audio_codec||p.audio_codec||'—').toUpperCase())}${p.audio_transcode?' → '+escapeHtml(String(p.audio_codec||'AAC').toUpperCase()):''}`)}
        ${diag('Encoder',escapeHtml(p.encoder||'Automático / copy'))}
        ${diag('Hardware',escapeHtml(p.hardware_acceleration||'Auto'))}
        ${diag('Tone mapping',yesNo(p.tone_map))}
        ${diag('Qualidade',escapeHtml(qualityLabel(activeQuality)))}
        ${diag('Sessão',escapeHtml(p.playback_session_id||'—'))}
      </div>
      ${reasons.length?`<div class="sf-v5-reasons"><b>Motivos da compatibilidade</b>${reasons.map(x=>`<span>${escapeHtml(String(x).replaceAll('_',' '))}</span>`).join('')}</div>`:''}`;
  }
  function diag(label,value){return`<div><span>${label}</span><b>${value}</b></div>`}

  function toggleQuality(show){
    const shouldShow=show===undefined?qualityMenu.classList.contains('hidden'):show;
    qualityMenu.classList.toggle('hidden',!shouldShow);
    if(shouldShow)diagnostics.classList.add('hidden');
    refreshQuality();
  }
  function toggleDiagnostics(show){
    const shouldShow=show===undefined?diagnostics.classList.contains('hidden'):show;
    diagnostics.classList.toggle('hidden',!shouldShow);
    if(shouldShow)qualityMenu.classList.add('hidden');
    if(shouldShow)renderDiagnostics();
  }

  qualityButton.addEventListener('click',e=>{e.stopPropagation();toggleQuality()});
  diagnosticsButton.addEventListener('click',e=>{e.stopPropagation();toggleDiagnostics()});
  qualityMenu.querySelector('[data-v5-close]').onclick=()=>toggleQuality(false);
  diagnostics.querySelector('[data-v5-diag-close]').onclick=()=>toggleDiagnostics(false);
  qualityMenu.addEventListener('click',async event=>{
    const button=event.target.closest?.('[data-v5-quality]');if(!button)return;
    const value=button.dataset.v5Quality;
    if(!availableQualities().includes(value))return;
    toggleQuality(false);
    if(window.sfPlaybackCore?.setQuality){
      if(typeof sfToast==='function')sfToast(`Qualidade: ${qualityLabel(value)}`);
      await window.sfPlaybackCore.setQuality(value);
      refreshPlan();
    }
  });

  window.addEventListener('stormflix:playback-plan',refreshPlan);
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
      case 'escape':toggleQuality(false);toggleDiagnostics(false);break;
    }
  });

  // Mouse/remote focus movement keeps the UI responsive without replacing the
  // native Android TV focus engine. WebView/desktop users can navigate buttons
  // with arrows when a control has focus.
  modal.addEventListener('keydown',e=>{
    if(!['ArrowLeft','ArrowRight'].includes(e.key))return;
    const active=document.activeElement;if(!(active instanceof HTMLButtonElement))return;
    const buttons=[...modal.querySelectorAll('button:not(.hidden):not([disabled])')].filter(b=>b.offsetParent!==null);
    const idx=buttons.indexOf(active);if(idx<0)return;
    e.preventDefault();buttons[(idx+(e.key==='ArrowRight'?1:-1)+buttons.length)%buttons.length]?.focus();
  });

  refreshQuality();refreshPlan();
})();