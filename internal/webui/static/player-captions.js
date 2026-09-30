/* Explicit subtitle selection; report success only after the decoder accepts it. */
(()=>{
  const modal=document.querySelector('#player-modal'),video=document.querySelector('#player'),button=document.querySelector('#sf-subtitle');if(!modal||!video||!button)return;
  const menu=document.createElement('div');menu.id='sf-caption-menu';menu.className='sf-v5-popover hidden';menu.setAttribute('role','dialog');menu.setAttribute('aria-label','Legendas');modal.appendChild(menu);
  let selected='0',busy=false,generation=0;
  const local=()=>window.sfLocalOrigin?.isActive?.();
  const rows=()=>local()?window.sfLocalOrigin.subtitleTracks():[...video.querySelectorAll('track[data-subtitle-id]')].map(t=>({id:t.dataset.subtitleId,label:t.label||t.srclang||'Legenda'}));
  function render(){
    menu.replaceChildren();const head=document.createElement('header'),title=document.createElement('b'),close=document.createElement('button');title.textContent='Legendas';close.textContent='×';close.setAttribute('aria-label','Fechar legendas');close.onclick=()=>menu.classList.add('hidden');head.append(title,close);menu.append(head);
    const tracks=[{id:'0',label:'Desativadas'},...rows()];
    for(const row of tracks){const b=document.createElement('button');b.type='button';b.className='sf-caption-option';b.dataset.captionId=row.id;b.textContent=(String(row.id)===selected?'✓ ':'')+row.label;b.disabled=busy;b.setAttribute('aria-pressed',String(String(row.id)===selected));b.onclick=()=>choose(String(row.id));menu.append(b)}
    if(tracks.length===1){const p=document.createElement('p');p.textContent='Nenhuma legenda disponível para este arquivo.';menu.append(p)}
  }
  async function choose(id){
    if(busy)return;busy=true;const token=generation;render();
    try{
      if(local()){if(await window.sfLocalOrigin.selectSubtitle(id)===false)throw new Error('A seleção de legenda foi cancelada')}
      else{
        const track=[...video.querySelectorAll('track[data-subtitle-id]')].find(t=>t.dataset.subtitleId===id);
        if(id!=='0'&&!track)throw new Error('Legenda indisponível');
        if(track&&track.readyState!==2){
          await new Promise((resolve,reject)=>{
            const cleanup=()=>{clearTimeout(timer);track.removeEventListener('load',ok);track.removeEventListener('error',fail)};
            const ok=()=>{cleanup();resolve()},fail=()=>{cleanup();reject(new Error('Não foi possível carregar a legenda'))};
            const timer=setTimeout(fail,10000);track.addEventListener('load',ok);track.addEventListener('error',fail);track.track.mode='hidden';
          });
        }
        if(token!==generation)return;
        for(const t of video.textTracks)t.mode='disabled';if(track)track.track.mode='showing';
      }
      if(token!==generation)return;selected=id;button.setAttribute('aria-pressed',String(id!=='0'));menu.classList.add('hidden');window.sfToast?.(id==='0'?'Legendas desativadas':'Legendas ativadas');
    }catch(error){window.sfToast?.(error.message||'Falha ao ativar legenda')}
    finally{if(token===generation){busy=false;render()}}
  }
  window.sfSelectSubtitle=id=>choose(String(id||0));
  window.sfCycleSubtitles=()=>{const ids=['0',...rows().map(r=>String(r.id))];return choose(ids[(Math.max(0,ids.indexOf(selected))+1)%ids.length])};
  button.onclick=e=>{e.stopPropagation();menu.classList.toggle('hidden');render();menu.querySelector('button[data-caption-id]')?.focus()};
  window.addEventListener('stormflix:local-tracks',render);
  window.addEventListener('stormflix:playback-plan',()=>{generation++;selected='0';busy=false;menu.classList.add('hidden');button.setAttribute('aria-pressed','false')});
  document.addEventListener('keydown',e=>{if(e.key==='Escape')menu.classList.add('hidden')});
  // A decoder can render video while Web Audio is blocked by autoplay policy.
  const sound=document.createElement('button');sound.id='sf-unlock-audio';sound.type='button';sound.textContent='Ativar som';sound.className='hidden';modal.appendChild(sound);
  const sync=()=>sound.classList.toggle('hidden',!local()||!window.sfLocalOrigin.isAudioSuspended());
  sound.onclick=()=>{const result=window.sfLocalOrigin.unlockAudio();Promise.resolve(result).then(sync)};
  window.addEventListener('stormflix:local-decode-stat',sync);video.addEventListener('playing',sync);
})();
