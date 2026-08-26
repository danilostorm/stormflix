/* StormFlix stores SQLite timestamps in UTC. Render them in the administrator browser timezone.
   This module also owns the compact Accounts & Profiles workspace because it is loaded after
   the legacy admin.js and can safely replace the old library-checkbox editor. */
(function(){
  let openUserID=0;

  window.stormLocalDate=function(value,fallback='Nunca'){
    if(value===null||value===undefined||String(value).trim()==='')return fallback;
    const raw=String(value).trim();
    let iso=raw;
    if(!/[zZ]$|[+-]\d{2}:?\d{2}$/.test(iso))iso=iso.replace(' ','T')+'Z';
    const date=new Date(iso);
    if(Number.isNaN(date.getTime()))return raw;
    try{return new Intl.DateTimeFormat('pt-BR',{dateStyle:'short',timeStyle:'medium'}).format(date)}catch{return date.toLocaleString('pt-BR')}
  };

  loadUsers=async function(){
    users=await req('/admin/users');
    const profileCounts={};
    await Promise.all(users.map(async u=>{
      try{const p=await req(`/admin/users/${u.id}/profiles`);profileCounts[u.id]=Array.isArray(p)?p.length:0}catch{profileCounts[u.id]=0}
    }));
    $('#users').innerHTML=`
      <div class="accounts-intro">
        <div><p class="kicker">Acesso simplificado</p><h2>Contas & Perfis</h2><p>Novas contas recebem acesso automático ao catálogo disponível. Perfis continuam separados por usuário para manter progresso, preferências, PIN e modo infantil.</p></div>
        <button class="primary accounts-create" onclick="editUser()">+ Criar usuário</button>
      </div>
      <div id="user-form"></div>
      <div class="accounts-grid">
        ${users.map(u=>accountCard(u,profileCounts[u.id]||0)).join('')||'<div class="v3-empty">Nenhum usuário cadastrado.</div>'}
      </div>
      <div id="profile-workspace"></div>`;
    if(openUserID&&users.some(u=>Number(u.id)===Number(openUserID)))openProfiles(openUserID,true);
  };

  function accountCard(u,profileCount){
    const own=Number(u.id)===Number(me?.id);
    return `<article class="account-card">
      <div class="account-card-head"><div class="account-avatar">${esc((u.display_name||u.username||'?').trim().charAt(0).toUpperCase())}</div><div><h3>${esc(u.display_name)}</h3><p>@${esc(u.username)}</p></div><span class="account-status ${u.active?'active':'blocked'}">${u.active?'ATIVO':'BLOQUEADO'}</span></div>
      <div class="account-facts">
        <div><span>Permissão</span><b>${esc(roleLabel(u.role))}</b></div>
        <div><span>Catálogo</span><b>Completo</b></div>
        <div><span>Perfis</span><b>${profileCount}</b></div>
        <div><span>Último login</span><b>${esc(stormLocalDate(u.last_login_at))}</b></div>
      </div>
      <div class="account-actions">
        <button onclick="openProfiles(${u.id})">Perfis</button>
        <button onclick="editUser(${u.id})">Editar conta</button>
        ${own?'':`<button class="danger" onclick="delUser(${u.id})">Excluir</button>`}
      </div>
    </article>`;
  }

  function roleLabel(role){return({user:'Usuário',operator:'Operador',manager:'Gerente',admin:'Administrador'})[role]||role||'Usuário'}

  window.editUser=id=>{
    const u=users.find(x=>Number(x.id)===Number(id))||{display_name:'',username:'',role:'user',active:true};
    $('#user-form').innerHTML=`<section class="account-editor-card">
      <div class="account-editor-head"><div><p class="kicker">${id?'Editar conta':'Nova conta'}</p><h3>${id?esc(u.display_name):'Criar usuário'}</h3><small>Acesso ao catálogo é automático; não é necessário escolher bibliotecas.</small></div><button type="button" class="ghost" data-close-user>✕</button></div>
      <form id="user-editor" class="account-editor-form">
        <label><span>Nome</span><input value="${esc(u.display_name)}" placeholder="Nome de exibição" required></label>
        <label><span>Usuário</span><input value="${esc(u.username)}" placeholder="usuario" ${id?'disabled':'required'}></label>
        <label><span>${id?'Nova senha':'Senha'}</span><input type="password" placeholder="${id?'Deixe vazio para manter':'Mínimo 8 caracteres'}" ${id?'':'required'}></label>
        <label><span>Permissão</span><select><option value="user">Usuário</option><option value="operator">Operador</option><option value="manager">Gerente</option><option value="admin">Administrador</option></select></label>
        <label class="account-toggle"><input type="checkbox" ${u.active?'checked':''}><span>Conta ativa</span></label>
        <div class="account-access-note"><b>✓ Catálogo completo</b><span>Filmes, séries, animes e música disponíveis para a conta conforme as regras do perfil.</span></div>
        <div class="account-editor-actions"><button type="button" data-cancel-user>Cancelar</button><button class="primary">Salvar conta</button></div>
      </form>
    </section>`;
    const f=$('#user-editor');f.elements[3].value=u.role;
    const close=()=>{$('#user-form').innerHTML=''};
    $('[data-close-user]').onclick=close;$('[data-cancel-user]').onclick=close;
    f.onsubmit=async e=>{
      e.preventDefault();
      const body={display_name:f.elements[0].value,password:f.elements[2].value,role:f.elements[3].value,active:f.elements[4].checked,library_ids:[]};
      if(!id)body.username=f.elements[1].value;
      try{await req(id?`/admin/users/${id}`:'/admin/users',{method:id?'PUT':'POST',body:JSON.stringify(body)});notice('Usuário salvo com acesso ao catálogo.',true);close();await loadUsers()}catch(err){notice(err.message)}
    };
  };

  window.openProfiles=async function(userID,silent=false){
    userID=Number(userID);openUserID=userID;
    const u=users.find(x=>Number(x.id)===userID);if(!u)return;
    const root=$('#profile-workspace');
    if(!silent){root.innerHTML='<div class="panel"><small>Carregando perfis…</small></div>';root.scrollIntoView({behavior:'smooth',block:'start'})}
    try{
      const profiles=await req(`/admin/users/${userID}/profiles`);
      root.innerHTML=`<section class="profiles-admin-card">
        <div class="profiles-admin-head"><div><p class="kicker">${esc(u.display_name)}</p><h3>Perfis</h3><small>Progresso, PIN e preferências ficam independentes em cada perfil.</small></div><div><button class="primary" onclick="editAdminProfile(${userID})">+ Novo perfil</button><button onclick="closeProfiles()">Fechar</button></div></div>
        <div id="profile-editor"></div>
        <div class="profiles-admin-grid">${profiles.map(p=>profileCard(userID,p)).join('')||'<div class="profiles-empty">Nenhum perfil. Crie o primeiro para começar.</div>'}</div>
      </section>`;
      if(!silent)root.scrollIntoView({behavior:'smooth',block:'start'});
    }catch(err){notice(err.message)}
  };

  window.closeProfiles=function(){openUserID=0;$('#profile-workspace').innerHTML=''};

  function profileCard(userID,p){
    const initial=(p.name||'?').trim().charAt(0).toUpperCase();
    return `<article class="profile-admin-item">
      <div class="profile-admin-avatar">${p.avatar_url?`<img src="${esc(p.avatar_url)}" alt="">`:`<span>${esc(initial)}</span>`}</div>
      <div class="profile-admin-copy"><h4>${esc(p.name)}</h4><p>${p.is_kids?'Infantil':'Padrão'} · ${p.pin_enabled?'PIN ativo':'Sem PIN'}</p><small>Áudio ${esc(p.preferred_audio||'pt-BR')} · Legenda ${esc(p.preferred_subtitle||'pt-BR')}</small></div>
      <div class="profile-admin-actions"><button onclick='editAdminProfile(${userID},${JSON.stringify(p).replace(/'/g,"&#39;")})'>Editar</button><button class="danger" onclick="deleteAdminProfile(${userID},${p.id})">Excluir</button></div>
    </article>`;
  }

  window.editAdminProfile=function(userID,p=null){
    const editing=!!p;
    const root=$('#profile-editor');
    root.innerHTML=`<form id="admin-profile-form" class="profile-editor-form">
      <div class="profile-editor-title"><b>${editing?'Editar perfil':'Novo perfil'}</b><button type="button" data-close-profile>✕</button></div>
      <label><span>Nome</span><input value="${esc(p?.name||'')}" required></label>
      <label><span>Avatar URL</span><input value="${esc(p?.avatar_url||'')}" placeholder="Opcional"></label>
      <label><span>Áudio preferido</span><select><option value="pt-BR">Português (Brasil)</option><option value="en">English</option><option value="es">Español</option></select></label>
      <label><span>Legenda preferida</span><select><option value="pt-BR">Português (Brasil)</option><option value="en">English</option><option value="es">Español</option></select></label>
      <label><span>${editing?'Novo PIN':'PIN'}</span><input type="password" inputmode="numeric" maxlength="8" placeholder="Opcional"></label>
      <label class="account-toggle"><input type="checkbox" ${p?.is_kids?'checked':''}><span>Perfil infantil</span></label>
      <label class="account-toggle"><input type="checkbox" ${p?.autoplay_next!==false?'checked':''}><span>Reproduzir próximo episódio</span></label>
      <div class="profile-editor-actions"><button type="button" data-close-profile>Cancelar</button><button class="primary">Salvar perfil</button></div>
    </form>`;
    const f=$('#admin-profile-form');f.elements[2].value=p?.preferred_audio||'pt-BR';f.elements[3].value=p?.preferred_subtitle||'pt-BR';
    root.querySelectorAll('[data-close-profile]').forEach(b=>b.onclick=()=>root.innerHTML='');
    f.onsubmit=async e=>{
      e.preventDefault();
      const body={name:f.elements[0].value,avatar_url:f.elements[1].value,avatar_key:p?.avatar_key||'',preferred_audio:f.elements[2].value,preferred_subtitle:f.elements[3].value,pin:f.elements[4].value,is_kids:f.elements[5].checked,autoplay_next:f.elements[6].checked};
      if(editing)body.active=p.active!==false;
      try{await req(editing?`/admin/profiles/${p.id}`:`/admin/users/${userID}/profiles`,{method:editing?'PUT':'POST',body:JSON.stringify(body)});notice('Perfil salvo.',true);await openProfiles(userID,true)}catch(err){notice(err.message)}
    };
  };

  window.deleteAdminProfile=async function(userID,profileID){
    if(!confirm('Excluir este perfil e seus dados associados?'))return;
    try{await req(`/admin/profiles/${profileID}`,{method:'DELETE'});notice('Perfil excluído.',true);await openProfiles(userID,true)}catch(err){notice(err.message)}
  };

  loadSessions=async function(){
    const a=await req('/admin/sessions');
    $('#sessions').innerHTML=table('Sessões',a.map(s=>`<tr><td><b>${esc(s.display_name)}</b><br><small>@${esc(s.username)}</small></td><td>${esc(s.ip)}</td><td><small>${esc(s.user_agent)}</small></td><td>${esc(stormLocalDate(s.created_at,'—'))}</td><td class="actions"><button class="danger" onclick="revoke(${s.id})">Encerrar</button></td></tr>`).join(''),'Usuário|IP|Dispositivo|Criada|Ações');
  };

  loadLogs=async function(){
    const a=await req('/admin/logs?limit=200');
    $('#logs').innerHTML=table('Logs',a.map(x=>`<tr><td>${esc(stormLocalDate(x.created_at,'—'))}</td><td><span class="pill">${esc(x.level)}</span></td><td>${esc(x.category)}</td><td>${esc(x.message)}<br><small>${esc(x.details)}</small></td></tr>`).join(''),'Data|Nível|Categoria|Evento');
  };
})();
