/* StormFlix Web v5.3 — real server-side audio track selection. */
(function(){
  let mediaID=0;
  let tracks=[];
  let loading=false;
  const modal=document.querySelector('#player-modal');
  if(!modal)return;

  const menu=document.createElement('div');
  menu.id='sf-v53-audio-menu';
  menu.className='sf-v53-audio-menu hidden';
  menu.setAttribute('role','dialog');
  menu.setAttribute('aria-label','Faixas de áudio');
  modal.appendChild(menu);

  function languageName(value){
    const raw=String(value||'').trim();
    if(!raw)return'Áudio';
    const normalized={por:'Português',pt:'Português',pob:'Português (Brasil)',eng:'Inglês',en:'Inglês',spa:'Espanhol',es:'Espanhol',jpn:'Japonês',ja:'Japonês',fra:'Francês',fre:'Francês',fr:'Francês',deu:'Alemão',ger:'Alemão',de:'Alemão',ita:'Italiano',it:'Italiano'}[raw.toLowerCase()];
    return normalized||raw.toUpperCase();
  }

  function label(track){
    const title=String(track?.title||'').trim();
    const lang=languageName(track?.language);
    if(title&&title.toLowerCase()!==String(track?.language||'').toLowerCase())return`${lang} · ${title}`;
    return lang;
  }

  function detail(track){
    const values=[String(track?.codec||'').toUpperCase()];
    if(track?.default)values.push('Padrão');
    return values.filter(Boolean).join(' · ');
  }

  function activeIndex(){
    const plan=window.sfPlaybackCore?.currentPlan?.()||window.sfLastPlaybackPlan||{};
    return Number.isInteger(plan.audio_stream)?Number(plan.audio_stream):Number(window.sfPlaybackCore?.currentAudioStream?.());
  }

  async function load(id){
    id=Number(id||0);mediaID=id;tracks=[];
    if(!id){render();return}
    try{
      const data=await request(`/media/${id}/playback/streams`);
      if(id!==mediaID)return;
      tracks=Array.isArray(data?.audio)?data.audio:[];
    }catch{tracks=[]}
    render();
    renderSettingsAudio();
  }

  async function select(index){
    if(loading)return;
    index=Number(index);if(!Number.isInteger(index)||index<0)return;
    if(activeIndex()===index){close();return}
    loading=true;menu.classList.add('sf-v53-audio-loading');
    try{
      await window.sfPlaybackCore?.setAudioStream?.(index);
      render();renderSettingsAudio();close();
      const chosen=tracks.find(t=>Number(t.index)===index);
      if(typeof sfToast==='function')sfToast(`Áudio: ${label(chosen)}`);
    }catch{
      if(typeof sfToast==='function')sfToast('Não foi possível trocar a faixa de áudio');
    }finally{loading=false;menu.classList.remove('sf-v53-audio-loading')}
  }

  function makeTrackButton(track){
    const button=document.createElement('button');
    button.type='button';button.className='sf-v53-audio-track';button.dataset.audioStream=String(track.index);
    if(Number(track.index)===activeIndex())button.classList.add('active');
    const check=document.createElement('span');check.className='sf-v53-audio-check';check.textContent=Number(track.index)===activeIndex()?'✓':'';
    const copy=document.createElement('span');copy.className='sf-v53-audio-copy';
    const strong=document.createElement('b');strong.textContent=label(track);
    const small=document.createElement('small');small.textContent=detail(track);
    copy.append(strong,small);button.append(check,copy);button.onclick=()=>select(track.index);return button;
  }

  function render(){
    menu.replaceChildren();
    const head=document.createElement('div');head.className='sf-v53-audio-head';
    const title=document.createElement('strong');title.textContent='Áudio';
    const closeButton=document.createElement('button');closeButton.type='button';closeButton.className='sf-v53-audio-close';closeButton.setAttribute('aria-label','Fechar');closeButton.textContent='×';closeButton.onclick=close;
    head.append(title,closeButton);menu.appendChild(head);
    if(!tracks.length){const empty=document.createElement('div');empty.className='sf-v53-audio-empty';empty.textContent='Este arquivo possui apenas a faixa de áudio selecionada ou não informou outras faixas.';menu.appendChild(empty);return}
    tracks.forEach(track=>menu.appendChild(makeTrackButton(track)));
  }

  function renderSettingsAudio(){
    const panel=document.querySelector('#sf-player-settings-panel');if(!panel)return;
    const section=[...panel.querySelectorAll('.sf-setting-section')].find(s=>/^Áudio/i.test(s.querySelector('h3')?.textContent||''));
    if(!section)return;
    const heading=section.querySelector('h3')?.cloneNode(true)||document.createElement('h3');heading.textContent='Áudio';section.replaceChildren(heading);
    if(!tracks.length){const p=document.createElement('p');p.className='sf-setting-note';p.textContent='Nenhuma faixa adicional informada pelo arquivo.';section.appendChild(p);return}
    tracks.forEach(track=>section.appendChild(makeTrackButton(track)));
  }

  function open(){
    if(typeof sfToggleSettings==='function')sfToggleSettings(false);
    render();menu.classList.remove('hidden');
  }
  function close(){menu.classList.add('hidden')}

  if(typeof sfLoadPlayerOptions==='function'){
    const baseLoad=sfLoadPlayerOptions;
    sfLoadPlayerOptions=async function(id){const result=await baseLoad(id);await load(id);return result};
  }
  if(typeof sfRenderSettings==='function'){
    const baseRender=sfRenderSettings;
    sfRenderSettings=function(){const result=baseRender();renderSettingsAudio();return result};
  }

  const audioButton=document.querySelector('#sf-v4-audio');if(audioButton)audioButton.onclick=open;
  document.querySelector('#sf-settings')?.addEventListener('click',close,{capture:true});
  document.querySelector('#sf-subtitle')?.addEventListener('click',close,{capture:true});
  document.addEventListener('keydown',event=>{if(event.key==='Escape')close()});
  window.addEventListener('stormflix:playback-plan',()=>{render();renderSettingsAudio()});
  window.addEventListener('stormflix:player-built',()=>{const b=document.querySelector('#sf-v4-audio');if(b)b.onclick=open});
  window.sfWebAudioTracks=()=>tracks.slice();
})();
