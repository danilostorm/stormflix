/* StormFlix profiles: selector, PIN, avatars, preferences, Trakt and management. */
(function(){
  let profiles=[];
  let selected=null;
  let manageMode=false;
  let traktPollTimer=null;
  const avatarKeys=['storm-red','ocean-blue','anime-pink','matrix-green','sunset-orange','nebula-purple','midnight','kids-yellow'];
  const baseAuthenticated=authenticated;

  // Resolve the selected profile before the expensive Home request. Older
  // builds loaded Home once as the account and then again after auto-selecting
  // the only profile; multi-profile accounts also loaded a hidden Home behind
  // "Quem está assistindo?". The profile cookie is now ready first, so Home is
  // loaded exactly once for the profile that will actually see it.
  authenticated=async function(){
    const ready=await loadProfiles(true,true);
    if(ready){
      await baseAuthenticated();
      if(selected)applyProfile(selected);
      hidePicker();
      return;
    }
    revealShellWithoutHome();
    showPicker();
  };

  function revealShellWithoutHome(){
    $('#login')?.classList.add('hidden');
    $('#shell')?.classList.remove('hidden');
    if($('#user-label'))$('#user-label').textContent=me?.display_name||me?.username||'StormFlix';
    const initial=$('#profile-initial');
    if(initial)initial.textContent=(me?.display_name||me?.username||'S').trim().charAt(0).toUpperCase()||'S';
    if(me?.role==='admin')$('#admin-link')?.classList.remove('hidden');
  }

  async function loadProfiles(afterLogin=false,bootstrap=false){
    const data=await request('/profiles');
    profiles=(data.profiles||[]).filter(p=>p.active);
    selected=profiles.find(p=>Number(p.id)===Number(data.selected_profile_id))||null;
    if(selected){
      if(!bootstrap)applyProfile(selected);
      hidePicker();
      return true;
    }
    if(profiles.length===1&&!profiles[0].pin_enabled){
      selected=await request(`/profiles/${profiles[0].id}/select`,{method:'POST',body:'{}'});
      if(!bootstrap){
        applyProfile(selected);hidePicker();await loadHome();
      }else hidePicker();
      return true;
    }
    if(!bootstrap&&(profiles.length>0||afterLogin))showPicker();
    return false;
  }

  function avatar(p,small=false){
    const cls=small?'header-profile-avatar':'profile-avatar';
    const key=`avatar-${escapeHTML(p.avatar_key||'storm-red')}`;
    const fallback=escapeHTML((p.name||'S').trim().charAt(0).toUpperCase()||'S');
    if(p.avatar_url)return `<span class="${cls} ${key}"><img data-profile-avatar-img data-avatar-fallback="${fallback}" src="${escapeHTML(p.avatar_url)}" alt=""></span>`;
    return `<span class="${cls} ${key}">${fallback}</span>`;
  }

  function bindAvatarFallbacks(root=document){
    root.querySelectorAll?.('[data-profile-avatar-img]').forEach(img=>{
      img.onerror=()=>{
        const parent=img.parentElement;
        if(!parent)return;
        parent.textContent=img.dataset.avatarFallback||'S';
      };
    });
  }

  function ensureToolbar(){
    const add=$('#profile-add');if(!add)return;
    add.textContent='+ Adicionar perfil';
    let manage=$('#profile-manage');
    if(!manage){
      manage=document.createElement('button');manage.id='profile-manage';manage.type='button';manage.className='profile-manage-link';
      add.before(manage);
    }
    manage.textContent=manageMode?'Concluir':'Gerenciar perfis';
    manage.onclick=()=>{manageMode=!manageMode;render()};
    add.onclick=()=>openEditor(null);
  }

  function render(){
    const root=$('#profile-grid');if(!root)return;
    root.innerHTML=profiles.map(p=>`<div class="profile-choice-wrap"><button class="profile-choice ${manageMode?'managing':''}" data-profile-id="${p.id}" type="button">${avatar(p)}<b>${escapeHTML(p.name)}</b><span class="profile-flags">${p.pin_enabled?'<i>PIN</i>':''}${p.is_kids?'<i>INFANTIL</i>':''}${Number(p.content_rating_limit)<18?`<i>ATÉ ${ratingLabel(p.content_rating_limit)}</i>`:''}</span></button>${manageMode?`<button type="button" class="profile-edit" data-profile-edit="${p.id}">Editar</button>`:''}</div>`).join('');
    bindAvatarFallbacks(root);
    root.querySelectorAll('[data-profile-id]').forEach(b=>b.onclick=()=>manageMode?openEditor(profiles.find(p=>Number(p.id)===Number(b.dataset.profileId))):choose(Number(b.dataset.profileId)));
    root.querySelectorAll('[data-profile-edit]').forEach(b=>b.onclick=()=>openEditor(profiles.find(p=>Number(p.id)===Number(b.dataset.profileEdit))));
    $('#profile-add')?.classList.toggle('hidden',profiles.length>=8);
    ensureToolbar();
  }

  async function choose(id){
    const profile=profiles.find(p=>Number(p.id)===Number(id));if(!profile)return;
    try{
      let pin='';
      if(profile.pin_enabled){
        pin=await requestPIN(profile.name);
        if(pin===null)return;
      }
      selected=await request(`/profiles/${id}/select`,{method:'POST',body:JSON.stringify({pin})});
      applyProfile(selected);hidePicker();await loadHome();
    }catch(err){message(err.message);if(err.status===401)setTimeout(()=>choose(id),250)}
  }

  function applyProfile(p){
    selected=p;
    const old=$('#profile-initial');
    if(old)old.outerHTML=avatar(p,true).replace('<span ','<span id="profile-initial" ');
    bindAvatarFallbacks(document);
    $('#user-label').textContent=p.name||me.display_name;
    document.documentElement.dataset.profileKids=p.is_kids?'1':'0';
    document.documentElement.dataset.profileRating=String(p.content_rating_limit??18);
    window.dispatchEvent(new CustomEvent('stormflix:profile',{detail:p}));
  }
  function showPicker(){manageMode=false;render();$('#profile-picker')?.classList.remove('hidden')}
  function hidePicker(){$('#profile-picker')?.classList.add('hidden');message('')}
  function message(t){if($('#profile-message'))$('#profile-message').textContent=t||''}

  function ensureEditor(){
    let overlay=$('#profile-editor-overlay');if(overlay)return overlay;
    overlay=document.createElement('div');overlay.id='profile-editor-overlay';overlay.className='profile-editor-overlay hidden';document.body.appendChild(overlay);
    overlay.onclick=e=>{if(e.target===overlay)closeEditor()};
    return overlay;
  }
  function stopTraktPolling(){if(traktPollTimer){clearTimeout(traktPollTimer);traktPollTimer=null}}
  function closeEditor(){stopTraktPolling();$('#profile-editor-overlay')?.classList.add('hidden')}

  function openEditor(profile){
    stopTraktPolling();
    const isNew=!profile;
    profile=profile||{name:'',avatar_key:'storm-red',avatar_url:'',is_kids:false,content_rating_limit:18,pin_enabled:false,autoplay_next:true,autoplay_previews:true,preferred_audio:'pt-BR',preferred_subtitle:'pt-BR'};
    const overlay=ensureEditor();
    let avatarKey=profile.avatar_key||'storm-red';
    const rating=Number(profile.content_rating_limit??(profile.is_kids?10:18));
    overlay.innerHTML=`<form class="profile-editor-card" id="profile-editor-form"><div class="profile-editor-head"><div><p>${isNew?'NOVO PERFIL':'EDITAR PERFIL'}</p><h2>${isNew?'Adicionar perfil':escapeHTML(profile.name)}</h2></div><button type="button" id="profile-editor-close">✕</button></div><div class="profile-editor-body"><label><span>Nome</span><input id="profile-edit-name" maxlength="40" value="${escapeHTML(profile.name)}" required></label><div class="profile-avatar-field"><span>Avatar</span><div class="profile-avatar-options">${avatarKeys.map(key=>`<button type="button" data-avatar-key="${key}" class="profile-avatar-swatch avatar-${key} ${key===avatarKey?'active':''}">${escapeHTML((profile.name||'S').charAt(0).toUpperCase())}</button>`).join('')}</div></div><div class="profile-editor-columns profile-wide"><label><span>Enviar foto do perfil</span><input id="profile-edit-avatar-file" type="file" accept="image/jpeg,image/png,image/webp,image/gif"><small>JPEG, PNG, WebP ou GIF · até 5 MiB</small></label><label><span>Ou usar imagem por URL</span><input id="profile-edit-avatar-url" value="${escapeHTML(profile.avatar_url||'')}" placeholder="https://..."><small>Ao escolher um avatar padrão, a foto personalizada é removida.</small></label></div><div class="profile-editor-columns"><label><span>${profile.pin_enabled?'Novo PIN (deixe vazio para manter)':'PIN opcional'}</span><input id="profile-edit-pin" type="password" inputmode="numeric" maxlength="6" pattern="[0-9]{4,6}" placeholder="4 a 6 números"></label><label class="profile-check"><input id="profile-clear-pin" type="checkbox"><span>Remover PIN atual</span></label></div><div class="profile-editor-columns"><label class="profile-check"><input id="profile-edit-kids" type="checkbox" ${profile.is_kids?'checked':''}><span>Perfil infantil</span></label><label><span>Classificação máxima</span><select id="profile-edit-rating">${ratingOptions(rating)}</select><small>Conteúdo acima deste limite fica oculto e bloqueado.</small></label></div><div class="profile-preferences"><label class="profile-check"><input id="profile-edit-next" type="checkbox" ${profile.autoplay_next!==false?'checked':''}><span>Reproduzir próximo episódio automaticamente</span></label><label class="profile-check"><input id="profile-edit-previews" type="checkbox" ${profile.autoplay_previews!==false?'checked':''}><span>Permitir prévias automáticas</span></label></div><div class="profile-editor-columns"><label><span>Áudio preferido</span><select id="profile-edit-audio">${languageOptions(profile.preferred_audio)}</select></label><label><span>Legenda preferida</span><select id="profile-edit-subtitle">${languageOptions(profile.preferred_subtitle)}</select></label></div>${!isNew?'<section id="profile-trakt-panel" class="profile-trakt-panel"><div><b>Trakt</b><small>Carregando integração deste perfil…</small></div></section>':''}</div><div class="profile-editor-footer">${!isNew&&profiles.length>1?'<button type="button" class="profile-delete" id="profile-delete">Excluir perfil</button>':'<span></span>'}<div><button type="button" id="profile-cancel">Cancelar</button><button type="submit" class="profile-save">Salvar</button></div></div><p id="profile-editor-message"></p></form>`;
    overlay.classList.remove('hidden');
    const avatarURL=$('#profile-edit-avatar-url');
    const avatarFile=$('#profile-edit-avatar-file');
    const kids=$('#profile-edit-kids');
    const ratingSelect=$('#profile-edit-rating');
    overlay.querySelectorAll('[data-avatar-key]').forEach(b=>b.onclick=()=>{
      avatarKey=b.dataset.avatarKey;
      overlay.querySelectorAll('[data-avatar-key]').forEach(x=>x.classList.toggle('active',x===b));
      avatarURL.value='';
      avatarFile.value='';
    });
    avatarFile.onchange=()=>{if(avatarFile.files?.length)avatarURL.value=''};
    avatarURL.oninput=()=>{if(avatarURL.value.trim())avatarFile.value=''};
    kids.onchange=()=>{
      if(kids.checked&&Number(ratingSelect.value)>10)ratingSelect.value='10';
      if(!kids.checked&&Number(ratingSelect.value)<=10)ratingSelect.value='18';
    };
    $('#profile-editor-close').onclick=$('#profile-cancel').onclick=closeEditor;
    if($('#profile-delete'))$('#profile-delete').onclick=()=>deleteProfile(profile);
    if(!isNew)loadTraktPanel(profile.id);
    $('#profile-editor-form').onsubmit=async e=>{
      e.preventDefault();
      const body={name:$('#profile-edit-name').value.trim(),avatar_key:avatarKey,avatar_url:avatarURL.value.trim(),is_kids:kids.checked,content_rating_limit:Number(ratingSelect.value),pin:$('#profile-edit-pin').value.trim(),clear_pin:$('#profile-clear-pin').checked,autoplay_next:$('#profile-edit-next').checked,autoplay_previews:$('#profile-edit-previews').checked,preferred_audio:$('#profile-edit-audio').value,preferred_subtitle:$('#profile-edit-subtitle').value};
      try{
        const saved=isNew
          ?await request('/profiles',{method:'POST',body:JSON.stringify(body)})
          :await request(`/profiles/${profile.id}`,{method:'PUT',body:JSON.stringify(body)});
        if(avatarFile.files?.[0])await uploadAvatar(saved.id,avatarFile.files[0]);
        closeEditor();await loadProfiles(false);showPicker();
      }catch(err){$('#profile-editor-message').textContent=err.message}
    };
  }

  async function loadTraktPanel(profileID){
    const panel=$('#profile-trakt-panel');if(!panel)return;
    try{
      const status=await request(`/profiles/${profileID}/trakt`);
      renderTraktPanel(profileID,status);
    }catch(err){panel.innerHTML=`<div><b>Trakt</b><small>${escapeHTML(err.message)}</small></div>`}
  }

  function renderTraktPanel(profileID,status){
    const panel=$('#profile-trakt-panel');if(!panel)return;
    stopTraktPolling();
    if(!status.configured){
      panel.innerHTML='<div><b>Trakt</b><small>Integração disponível, mas o administrador ainda precisa configurar o Client ID/Secret do aplicativo Trakt no servidor.</small></div>';
      return;
    }
    if(status.connected){
      const label=status.username||status.user_slug||'Conta conectada';
      panel.innerHTML=`<div><b>Trakt conectado</b><small>@${escapeHTML(label)} · histórico/scrobble deste perfil fica independente dos outros perfis.</small></div><button type="button" class="secondary" id="profile-trakt-disconnect">Desconectar</button>`;
      $('#profile-trakt-disconnect').onclick=async()=>{
        if(!confirm('Desconectar o Trakt somente deste perfil?'))return;
        try{await request(`/profiles/${profileID}/trakt`,{method:'DELETE'});await loadTraktPanel(profileID)}catch(err){$('#profile-editor-message').textContent=err.message}
      };
      return;
    }
    if(status.authorization_pending){
      const url=escapeHTML(status.verification_url||'https://trakt.tv/activate');
      panel.innerHTML=`<div class="profile-trakt-code"><div><b>Autorize o Trakt</b><small>Abra o endereço abaixo em qualquer celular/PC e informe o código.</small></div><strong>${escapeHTML(status.user_code||'')}</strong><a href="${url}" target="_blank" rel="noopener">${url}</a></div><button type="button" class="secondary" id="profile-trakt-check">Já autorizei</button>`;
      $('#profile-trakt-check').onclick=()=>pollTrakt(profileID,true);
      scheduleTraktPoll(profileID,status);
      return;
    }
    panel.innerHTML='<div><b>Trakt</b><small>Vincule uma conta diferente para cada perfil. O StormFlix usa Device OAuth, inclusive em TV/Fire TV, e nunca compartilha o token entre perfis.</small></div><button type="button" class="secondary" id="profile-trakt-connect">Conectar Trakt</button>';
    $('#profile-trakt-connect').onclick=async()=>{
      const button=$('#profile-trakt-connect');button.disabled=true;button.textContent='Gerando código…';
      try{const status=await request(`/profiles/${profileID}/trakt/device`,{method:'POST',body:'{}'});renderTraktPanel(profileID,status)}catch(err){$('#profile-editor-message').textContent=err.message;button.disabled=false;button.textContent='Conectar Trakt'}
    };
  }

  function scheduleTraktPoll(profileID,status){
    stopTraktPolling();
    const seconds=Math.max(5,Number(status.interval_seconds||5));
    traktPollTimer=setTimeout(()=>pollTrakt(profileID,false),seconds*1000);
  }

  async function pollTrakt(profileID,manual){
    try{
      const status=await request(`/profiles/${profileID}/trakt/device/poll`,{method:'POST',body:'{}'});
      renderTraktPanel(profileID,status);
    }catch(err){
      if(manual)$('#profile-editor-message').textContent=err.message;
      else setTimeout(()=>loadTraktPanel(profileID),1500);
    }
  }

  async function uploadAvatar(profileID,file){
    const form=new FormData();form.append('avatar',file);
    const response=await fetch(`${api}/profiles/${profileID}/avatar`,{method:'POST',body:form,credentials:'same-origin'});
    const data=await response.json().catch(()=>({}));
    if(!response.ok)throw Object.assign(new Error(data.error||`HTTP ${response.status}`),{status:response.status});
    return data;
  }

  function ratingLabel(value){return Number(value)===0?'Livre':String(value)}
  function ratingOptions(current){
    return [[0,'Livre'],[10,'Até 10 anos'],[12,'Até 12 anos'],[14,'Até 14 anos'],[16,'Até 16 anos'],[18,'Até 18 anos / sem restrição']].map(([value,label])=>`<option value="${value}" ${Number(current)===value?'selected':''}>${label}</option>`).join('');
  }
  function languageOptions(current){
    const options=[['pt-BR','Português (Brasil)'],['en','English'],['es','Español'],['ja','日本語'],['','Automático']];
    return options.map(([value,label])=>`<option value="${value}" ${String(current||'pt-BR')===value?'selected':''}>${label}</option>`).join('');
  }

  async function deleteProfile(profile){
    if(!confirm(`Excluir o perfil ${profile.name}? O progresso deste perfil também será removido.`))return;
    try{await request(`/profiles/${profile.id}`,{method:'DELETE'});closeEditor();if(Number(selected?.id)===Number(profile.id))selected=null;await loadProfiles(false);showPicker()}catch(err){$('#profile-editor-message').textContent=err.message}
  }

  function requestPIN(name){
    return new Promise(resolve=>{
      let overlay=$('#profile-pin-overlay');
      if(!overlay){overlay=document.createElement('div');overlay.id='profile-pin-overlay';overlay.className='profile-pin-overlay';document.body.appendChild(overlay)}
      overlay.innerHTML=`<form class="profile-pin-card"><p>PERFIL PROTEGIDO</p><h2>${escapeHTML(name)}</h2><span>Digite o PIN para continuar</span><input id="profile-pin-input" type="password" inputmode="numeric" maxlength="6" pattern="[0-9]{4,6}" autocomplete="off" autofocus><div><button type="button" id="profile-pin-cancel">Cancelar</button><button type="submit">Entrar</button></div></form>`;
      overlay.classList.remove('hidden');
      const finish=value=>{overlay.classList.add('hidden');resolve(value)};
      $('#profile-pin-cancel').onclick=()=>finish(null);
      overlay.querySelector('form').onsubmit=e=>{e.preventDefault();finish($('#profile-pin-input').value.trim())};
      setTimeout(()=>$('#profile-pin-input')?.focus(),20);
    });
  }

  window.sfProfiles={reload:loadProfiles,show:showPicker,current:()=>selected,edit:openEditor};
  $('#profile-btn').onclick=showPicker;
  $('#profile-add')?.addEventListener('click',()=>openEditor(null));
})();
