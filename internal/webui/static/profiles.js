/* StormFlix profiles: selector, PIN, avatars, preferences and management. */
(function(){
  let profiles=[];
  let selected=null;
  let manageMode=false;
  const avatarKeys=['storm-red','ocean-blue','anime-pink','matrix-green','sunset-orange','nebula-purple','midnight','kids-yellow'];
  const baseAuthenticated=authenticated;
  authenticated=async function(){await baseAuthenticated();await loadProfiles(true)};

  async function loadProfiles(afterLogin=false){
    const data=await request('/profiles');
    profiles=(data.profiles||[]).filter(p=>p.active);
    selected=profiles.find(p=>Number(p.id)===Number(data.selected_profile_id))||null;
    if(selected){applyProfile(selected);hidePicker();return}
    if(profiles.length===1&&!profiles[0].pin_enabled){
      selected=await request(`/profiles/${profiles[0].id}/select`,{method:'POST',body:'{}'});
      applyProfile(selected);hidePicker();await loadHome();return
    }
    if(profiles.length>0||afterLogin)showPicker();
  }

  function avatar(p,small=false){
    const cls=small?'header-profile-avatar':'profile-avatar';
    const key=`avatar-${escapeHTML(p.avatar_key||'storm-red')}`;
    if(p.avatar_url)return `<span class="${cls} ${key}"><img src="${escapeHTML(p.avatar_url)}" alt=""></span>`;
    return `<span class="${cls} ${key}">${escapeHTML((p.name||'S').trim().charAt(0).toUpperCase()||'S')}</span>`;
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
    root.innerHTML=profiles.map(p=>`<div class="profile-choice-wrap"><button class="profile-choice ${manageMode?'managing':''}" data-profile-id="${p.id}" type="button">${avatar(p)}<b>${escapeHTML(p.name)}</b><span class="profile-flags">${p.pin_enabled?'<i>PIN</i>':''}${p.is_kids?'<i>INFANTIL</i>':''}</span></button>${manageMode?`<button type="button" class="profile-edit" data-profile-edit="${p.id}">Editar</button>`:''}</div>`).join('');
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
    $('#user-label').textContent=p.name||me.display_name;
    document.documentElement.dataset.profileKids=p.is_kids?'1':'0';
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
  function closeEditor(){$('#profile-editor-overlay')?.classList.add('hidden')}

  function openEditor(profile){
    const isNew=!profile;
    profile=profile||{name:'',avatar_key:'storm-red',avatar_url:'',is_kids:false,pin_enabled:false,autoplay_next:true,autoplay_previews:true,preferred_audio:'pt-BR',preferred_subtitle:'pt-BR'};
    const overlay=ensureEditor();
    let avatarKey=profile.avatar_key||'storm-red';
    overlay.innerHTML=`<form class="profile-editor-card" id="profile-editor-form"><div class="profile-editor-head"><div><p>${isNew?'NOVO PERFIL':'EDITAR PERFIL'}</p><h2>${isNew?'Adicionar perfil':escapeHTML(profile.name)}</h2></div><button type="button" id="profile-editor-close">✕</button></div><div class="profile-editor-body"><label><span>Nome</span><input id="profile-edit-name" maxlength="40" value="${escapeHTML(profile.name)}" required></label><div class="profile-avatar-field"><span>Avatar</span><div class="profile-avatar-options">${avatarKeys.map(key=>`<button type="button" data-avatar-key="${key}" class="profile-avatar-swatch avatar-${key} ${key===avatarKey?'active':''}">${escapeHTML((profile.name||'S').charAt(0).toUpperCase())}</button>`).join('')}</div></div><div class="profile-editor-columns profile-wide"><label><span>Enviar foto do perfil</span><input id="profile-edit-avatar-file" type="file" accept="image/jpeg,image/png,image/webp,image/gif"><small>JPEG, PNG, WebP ou GIF · até 5 MiB</small></label><label><span>Ou usar imagem por URL</span><input id="profile-edit-avatar-url" value="${escapeHTML(profile.avatar_url||'')}" placeholder="https://..."><small>Ao escolher um avatar padrão, a foto personalizada é removida.</small></label></div><div class="profile-editor-columns"><label><span>${profile.pin_enabled?'Novo PIN (deixe vazio para manter)':'PIN opcional'}</span><input id="profile-edit-pin" type="password" inputmode="numeric" maxlength="6" pattern="[0-9]{4,6}" placeholder="4 a 6 números"></label><label class="profile-check"><input id="profile-clear-pin" type="checkbox"><span>Remover PIN atual</span></label></div><div class="profile-preferences"><label class="profile-check"><input id="profile-edit-kids" type="checkbox" ${profile.is_kids?'checked':''}><span>Perfil infantil</span></label><label class="profile-check"><input id="profile-edit-next" type="checkbox" ${profile.autoplay_next!==false?'checked':''}><span>Reproduzir próximo episódio automaticamente</span></label><label class="profile-check"><input id="profile-edit-previews" type="checkbox" ${profile.autoplay_previews!==false?'checked':''}><span>Permitir prévias automáticas</span></label></div><div class="profile-editor-columns"><label><span>Áudio preferido</span><select id="profile-edit-audio">${languageOptions(profile.preferred_audio)}</select></label><label><span>Legenda preferida</span><select id="profile-edit-subtitle">${languageOptions(profile.preferred_subtitle)}</select></label></div></div><div class="profile-editor-footer">${!isNew&&profiles.length>1?'<button type="button" class="profile-delete" id="profile-delete">Excluir perfil</button>':'<span></span>'}<div><button type="button" id="profile-cancel">Cancelar</button><button type="submit" class="profile-save">Salvar</button></div></div><p id="profile-editor-message"></p></form>`;
    overlay.classList.remove('hidden');
    const avatarURL=$('#profile-edit-avatar-url');
    const avatarFile=$('#profile-edit-avatar-file');
    overlay.querySelectorAll('[data-avatar-key]').forEach(b=>b.onclick=()=>{
      avatarKey=b.dataset.avatarKey;
      overlay.querySelectorAll('[data-avatar-key]').forEach(x=>x.classList.toggle('active',x===b));
      avatarURL.value='';
      avatarFile.value='';
    });
    avatarFile.onchange=()=>{if(avatarFile.files?.length)avatarURL.value=''};
    avatarURL.oninput=()=>{if(avatarURL.value.trim())avatarFile.value=''};
    $('#profile-editor-close').onclick=$('#profile-cancel').onclick=closeEditor;
    if($('#profile-delete'))$('#profile-delete').onclick=()=>deleteProfile(profile);
    $('#profile-editor-form').onsubmit=async e=>{
      e.preventDefault();
      const body={name:$('#profile-edit-name').value.trim(),avatar_key:avatarKey,avatar_url:avatarURL.value.trim(),is_kids:$('#profile-edit-kids').checked,pin:$('#profile-edit-pin').value.trim(),clear_pin:$('#profile-clear-pin').checked,autoplay_next:$('#profile-edit-next').checked,autoplay_previews:$('#profile-edit-previews').checked,preferred_audio:$('#profile-edit-audio').value,preferred_subtitle:$('#profile-edit-subtitle').value};
      try{
        const saved=isNew
          ?await request('/profiles',{method:'POST',body:JSON.stringify(body)})
          :await request(`/profiles/${profile.id}`,{method:'PUT',body:JSON.stringify(body)});
        if(avatarFile.files?.[0])await uploadAvatar(saved.id,avatarFile.files[0]);
        closeEditor();await loadProfiles(false);showPicker();
      }catch(err){$('#profile-editor-message').textContent=err.message}
    };
  }

  async function uploadAvatar(profileID,file){
    const form=new FormData();form.append('avatar',file);
    const response=await fetch(`${api}/profiles/${profileID}/avatar`,{method:'POST',body:form,credentials:'same-origin'});
    const data=await response.json().catch(()=>({}));
    if(!response.ok)throw Object.assign(new Error(data.error||`HTTP ${response.status}`),{status:response.status});
    return data;
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
