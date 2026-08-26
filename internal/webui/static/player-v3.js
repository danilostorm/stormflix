/* StormFlix desktop player settings v3 */
(function(){
  let screenMode='fit';

  function mode(){return String(window.sfPlaybackMode||'direct_play')}
  function modeLabel(value){
    const labels={
      direct_play:'Direct Play',
      web_remux:'Remux · vídeo original',
      direct_stream_audio_aac:'Direct Stream · vídeo original + áudio AAC',
      unsupported:'Formato não suportado'
    };
    return labels[value]||value||'Direct Play';
  }
  function codec(value){return value?String(value).toUpperCase():'—'}

  function setScreenMode(next){
    screenMode=next||'fit';
    player.classList.remove('sf-screen-fit','sf-screen-169','sf-screen-zoom','sf-screen-fill');
    const map={fit:'sf-screen-fit','16:9':'sf-screen-169',zoom:'sf-screen-zoom',fill:'sf-screen-fill'};
    player.classList.add(map[screenMode]||map.fit);
    if(typeof sfToast==='function'){
      const labels={fit:'Ajustar / sem zoom','16:9':'16:9 · sem corte',zoom:'Preencher / Zoom',fill:'Esticar para tela'};
      sfToast(labels[screenMode]||labels.fit);
    }
    if(typeof sfRenderSettings==='function'&&typeof sfSettingsOpen!=='undefined'&&sfSettingsOpen)sfRenderSettings();
  }
  window.sfSetScreenMode=setScreenMode;

  const baseRenderSettings=typeof sfRenderSettings==='function'?sfRenderSettings:null;
  if(baseRenderSettings){
    sfRenderSettings=function(){
      baseRenderSettings();
      const panel=document.querySelector('#sf-player-settings-panel');
      if(!panel)return;

      const sections=[...panel.querySelectorAll('.sf-setting-section')];
      const sourceHead=sections[0]?.querySelector('h3');
      if(sourceHead)sourceHead.textContent='Fonte / Qualidade';

      const audioSection=sections.find(s=>s.querySelector('h3')?.textContent.trim()==='Áudio');
      if(audioSection){
        const row=document.createElement('div');
        row.className='sf-v3-compat-row';
        const current=mode();
        row.innerHTML=`
          <button class="sf-setting-option ${current==='direct_play'?'active':''}" data-sf-audio-mode="original"><span>Automático / Original</span><small>Usa o áudio do arquivo quando o navegador suporta</small></button>
          <button class="sf-setting-option ${current==='direct_stream_audio_aac'?'active':''}" data-sf-audio-mode="aac"><span>Compatibilidade AAC</span><small>Mantém o vídeo original e converte somente o áudio</small></button>`;
        audioSection.insertBefore(row,audioSection.children[1]||null);
        row.querySelector('[data-sf-audio-mode="original"]').onclick=()=>{
          if(typeof window.sfUseOriginalStream==='function')window.sfUseOriginalStream();
          if(typeof sfToast==='function')sfToast('Áudio original / automático');
          sfToggleSettings(false);
        };
        row.querySelector('[data-sf-audio-mode="aac"]').onclick=async e=>{
          const button=e.currentTarget;button.disabled=true;
          try{
            if(typeof window.sfUseAACCompatibility!=='function')throw new Error('Fallback AAC indisponível');
            await window.sfUseAACCompatibility();
            sfToggleSettings(false);
          }catch(err){
            if(typeof sfToast==='function')sfToast(err.message||'Não foi possível ativar AAC');
          }finally{button.disabled=false}
        };
      }

      const screen=document.createElement('section');
      screen.className='sf-setting-section';
      screen.innerHTML=`<h3>Tela / Zoom</h3>
        <button class="sf-setting-option ${screenMode==='fit'?'active':''}" data-sf-screen="fit"><span>Ajustar / sem zoom</span></button>
        <button class="sf-setting-option ${screenMode==='16:9'?'active':''}" data-sf-screen="16:9"><span>16:9 · sem corte</span></button>
        <button class="sf-setting-option ${screenMode==='zoom'?'active':''}" data-sf-screen="zoom"><span>Preencher / Zoom</span></button>
        <button class="sf-setting-option ${screenMode==='fill'?'active':''}" data-sf-screen="fill"><span>Esticar para tela</span></button>`;
      panel.appendChild(screen);
      screen.querySelectorAll('[data-sf-screen]').forEach(b=>b.onclick=()=>setScreenMode(b.dataset.sfScreen));

      const plan=window.sfLastCompatibilityPlan||{};
      const info=document.createElement('section');
      info.className='sf-setting-section';
      const resolution=player.videoWidth&&player.videoHeight?`${player.videoWidth} × ${player.videoHeight}`:'—';
      const sourceAudio=plan.source_audio_codec&&plan.source_audio_codec!==plan.audio_codec?codec(plan.source_audio_codec):codec(plan.audio_codec);
      info.innerHTML=`<h3>Informações</h3><div class="sf-v3-info">
        <div><span>Modo</span><b>${escapeHTML(modeLabel(mode()))}</b></div>
        <div><span>Vídeo</span><b>${escapeHTML(codec(plan.video_codec))}</b></div>
        <div><span>Áudio de origem</span><b>${escapeHTML(sourceAudio)}</b></div>
        <div><span>Áudio enviado</span><b>${escapeHTML(codec(plan.audio_codec))}</b></div>
        <div><span>Resolução</span><b>${escapeHTML(resolution)}</b></div>
      </div>`;
      panel.appendChild(info);
    };
  }

  const badge=document.querySelector('#player-modal .direct-badge');
  if(badge)badge.remove();
  const topGear=document.querySelector('#player-settings-top');
  if(topGear)topGear.onclick=()=>{
    if(typeof sfToggleSettings==='function')sfToggleSettings();
    if(typeof sfShowControls==='function')sfShowControls();
  };

  const baseSelectVersion=typeof sfSelectVersion==='function'?sfSelectVersion:null;
  if(baseSelectVersion){
    sfSelectVersion=async function(id){
      window.sfPlaybackMode='direct_play';window.sfLastCompatibilityPlan=null;
      return baseSelectVersion(id);
    };
  }

  setScreenMode('fit');
})();
