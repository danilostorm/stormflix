/* StormFlix profile selector */
(function(){
  let profiles=[];
  let selected=null;
  const baseAuthenticated=authenticated;
  authenticated=async function(){await baseAuthenticated();await loadProfiles(true)};

  async function loadProfiles(afterLogin=false){
    const data=await request('/profiles');
    profiles=(data.profiles||[]).filter(p=>p.active);
    selected=profiles.find(p=>Number(p.id)===Number(data.selected_profile_id))||null;
    if(selected){applyProfile(selected);hidePicker();return}
    if(profiles.length===1){selected=await request(`/profiles/${profiles[0].id}/select`,{method:'POST'});applyProfile(selected);hidePicker();return}
    if(profiles.length>1||afterLogin)showPicker();
  }

  function avatar(p,small=false){
    const cls=small?'header-profile-avatar':'profile-avatar';
    const key=`avatar-${escapeHTML(p.avatar_key||'storm-red')}`;
    if(p.avatar_url)return `<span class="${cls} ${key}"><img src="${escapeHTML(p.avatar_url)}" alt=""></span>`;
    return `<span class="${cls} ${key}">${escapeHTML((p.name||'S').trim().charAt(0).toUpperCase()||'S')}</span>`;
  }

  function render(){
    const root=$('#profile-grid');if(!root)return;
    root.innerHTML=profiles.map(p=>`<button class="profile-choice" data-profile-id="${p.id}">${avatar(p)}<b>${escapeHTML(p.name)}</b>${p.is_kids?'<span class="profile-kids">INFANTIL</span>':''}</button>`).join('');
    root.querySelectorAll('[data-profile-id]').forEach(b=>b.onclick=()=>choose(Number(b.dataset.profileId)));
    $('#profile-add')?.classList.toggle('hidden',profiles.length>=8);
  }

  async function choose(id){
    try{selected=await request(`/profiles/${id}/select`,{method:'POST'});applyProfile(selected);hidePicker()}catch(err){message(err.message)}
  }
  function applyProfile(p){
    const old=$('#profile-initial');
    if(old)old.outerHTML=avatar(p,true).replace('<span ','<span id="profile-initial" ');
    $('#user-label').textContent=p.name||me.display_name;
  }
  function showPicker(){render();$('#profile-picker')?.classList.remove('hidden')}
  function hidePicker(){$('#profile-picker')?.classList.add('hidden')}
  function message(t){if($('#profile-message'))$('#profile-message').textContent=t||''}

  async function addProfile(){
    const name=window.prompt('Nome do novo perfil:');
    if(!name?.trim())return;
    try{await request('/profiles',{method:'POST',body:JSON.stringify({name:name.trim(),avatar_key:'ocean-blue',avatar_url:'',is_kids:false})});await loadProfiles(false);showPicker()}catch(err){message(err.message)}
  }

  window.sfProfiles={reload:loadProfiles,show:showPicker,current:()=>selected};
  $('#profile-btn').onclick=showPicker;
  $('#profile-add')?.addEventListener('click',addProfile);
})();
