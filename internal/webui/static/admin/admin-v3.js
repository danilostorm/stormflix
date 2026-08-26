/* StormFlix Admin v3: categories, Netflix-style profiles and cleanup. */
(function(){
  let categories=[];
  let profileCache=[];
  const avatarKeys=['storm-red','ocean-blue','anime-pink','matrix-green','sunset-orange','nebula-purple','midnight','kids-yellow'];
  const baseTitle=title;
  title=function(name){return ({categories:'Categorias',cleanup:'Limpeza',users:'Contas & Perfis'})[name]||baseTitle(name)};

  document.querySelector('[data-page="categories"]')?.addEventListener('click',()=>loadCategoriesAdmin());
  document.querySelector('[data-page="cleanup"]')?.addEventListener('click',()=>loadCleanup());

  async function loadCategoriesAdmin(){
    categories=await req('/admin/categories');
    const libName=id=>libs.find(l=>Number(l.id)===Number(id))?.name||`Biblioteca #${id}`;
    $('#categories').innerHTML=`<div class="section-intro"><div><h2>Categorias de navegação</h2><p>Uma categoria pode reunir várias bibliotecas, e a mesma biblioteca pode participar de mais de uma categoria.</p></div><button class="primary" onclick="v3EditCategory()">+ Nova categoria</button></div><div id="category-form"></div><div class="category-grid">${categories.map(c=>`<article class="category-card"><div class="account-head"><h3>${esc(c.name)}</h3><span class="account-role">${c.system?'SISTEMA':esc(c.kind)}</span></div><p>/${esc(c.slug)} · ${c.active?'Ativa':'Oculta'}</p><div class="category-libraries">${(c.library_ids||[]).map(id=>`<span class="mini-chip">${esc(libName(id))}</span>`).join('')||'<span class="muted">Nenhuma biblioteca</span>'}</div><div class="category-actions"><button onclick="v3EditCategory(${c.id})">Editar</button>${c.system?'':`<button class="danger" onclick="v3DeleteCategory(${c.id})">Excluir</button>`}</div></article>`).join('')||'<div class="v3-empty">Nenhuma categoria.</div>'}</div>`;
  }

  window.v3EditCategory=id=>{
    const c=categories.find(x=>x.id===id)||{name:'',slug:'',kind:'mixed',sort_order:50,active:true,system:false,library_ids:[]};
    $('#category-form').innerHTML=`<form id="category-editor" class="form"><input value="${esc(c.name)}" placeholder="Nome da categoria" required><input value="${esc(c.slug)}" placeholder="slug-exemplo" ${c.system?'disabled':''} required><select><option value="movie">Filmes</option><option value="series">Séries</option><option value="anime">Animes</option><option value="mixed">Mista</option><option value="other">Outros</option></select><input type="number" value="${Number(c.sort_order||0)}" placeholder="Ordem"><label><input type="checkbox" ${c.active?'checked':''}> Ativa</label><button class="primary">Salvar</button><div class="wide checklist">${libs.map(l=>`<label><input type="checkbox" data-cat-lib="${l.id}" ${(c.library_ids||[]).includes(l.id)?'checked':''}> ${esc(l.name)}</label>`).join('')}</div></form>`;
    const f=$('#category-editor');f.elements[2].value=c.kind||'mixed';
    f.onsubmit=async e=>{e.preventDefault();const slug=c.system?c.slug:f.elements[1].value.trim().toLowerCase();const body={name:f.elements[0].value.trim(),slug,kind:f.elements[2].value,sort_order:Number(f.elements[3].value||0),active:f.elements[4].checked,library_ids:$$('#category-editor [data-cat-lib]:checked').map(x=>Number(x.dataset.catLib))};try{await req(id?`/admin/categories/${id}`:'/admin/categories',{method:id?'PUT':'POST',body:JSON.stringify(body)});notice('Categoria salva.',true);loadCategoriesAdmin()}catch(err){notice(err.message)}};
  };
  window.v3DeleteCategory=async id=>{if(!confirm('Excluir esta categoria? As bibliotecas e arquivos não serão apagados.'))return;try{await req(`/admin/categories/${id}`,{method:'DELETE'});notice('Categoria removida.',true);loadCategoriesAdmin()}catch(err){notice(err.message)}};

  loadUsers=async function(){
    users=await req('/admin/users');
    $('#users').innerHTML=`<div class="section-intro"><div><h2>Contas & Perfis</h2><p>A conta controla login e permissões. Cada conta pode ter até 8 perfis com avatar e progresso independentes.</p></div><button class="primary" onclick="editUser()">+ Criar conta</button></div><div id="user-form"></div><div id="profile-manager"></div><div class="account-grid">${users.map(u=>`<article class="account-card"><div class="account-head"><div><h3>${esc(u.display_name)}</h3><p>@${esc(u.username)}</p></div><span class="account-role">${esc(u.role)}</span></div><p>${u.active?'● Conta ativa':'○ Conta bloqueada'} · ${u.library_ids?.length||0} bibliotecas</p><p>Último login: ${esc(u.last_login_at||'Nunca')}</p><div class="account-actions"><button onclick="editUser(${u.id})">Conta e acesso</button><button onclick="v3ManageProfiles(${u.id})">Perfis</button>${u.id!==me.id?`<button class="danger" onclick="delUser(${u.id})">Excluir</button>`:''}</div></article>`).join('')}</div>`;
  };

  window.v3ManageProfiles=async userID=>{
    const user=users.find(x=>Number(x.id)===Number(userID));
    profileCache=await req(`/admin/users/${userID}/profiles`);
    $('#profile-manager').innerHTML=`<div class="panel"><div class="panel-head"><div><h2>Perfis de ${esc(user?.display_name||'usuário')}</h2><p class="muted">Avatar e histórico ficam separados por perfil.</p></div><button class="primary" onclick="v3EditProfile(${userID})">+ Perfil</button></div><div id="profile-editor-wrap"></div><div class="profile-admin-grid">${profileCache.map(p=>profileCard(p,userID)).join('')}</div></div>`;
    $('#profile-manager').scrollIntoView({behavior:'smooth',block:'start'});
  };

  function profileCard(p,userID){const av=p.avatar_url?`<span class="admin-avatar"><img src="${esc(p.avatar_url)}" alt=""></span>`:`<span class="admin-avatar avatar-${esc(p.avatar_key)}">${esc((p.name||'S').charAt(0).toUpperCase())}</span>`;return `<article class="profile-admin-card">${av}<div><h3>${esc(p.name)}</h3><p>${p.is_kids?'Perfil infantil':'Perfil padrão'} · ${p.active?'ativo':'oculto'}</p><div class="account-actions"><button onclick="v3EditProfile(${userID},${p.id})">Editar</button>${profileCache.length>1?`<button class="danger" onclick="v3DeleteProfile(${userID},${p.id})">Excluir</button>`:''}</div></div></article>`}

  window.v3EditProfile=(userID,id)=>{
    const p=profileCache.find(x=>x.id===id)||{name:'',avatar_key:'storm-red',avatar_url:'',is_kids:false,active:true};
    $('#profile-editor-wrap').innerHTML=`<form id="profile-editor" class="form profile-editor"><input value="${esc(p.name)}" placeholder="Nome do perfil" required><select>${avatarKeys.map(k=>`<option value="${k}">${k}</option>`).join('')}</select><input class="wide" value="${esc(p.avatar_url||'')}" placeholder="URL personalizada do avatar (opcional)"><label><input type="checkbox" ${p.is_kids?'checked':''}> Infantil</label><label><input type="checkbox" ${p.active?'checked':''}> Ativo</label><button class="primary">Salvar perfil</button></form>`;
    const f=$('#profile-editor');f.elements[1].value=p.avatar_key||'storm-red';
    f.onsubmit=async e=>{e.preventDefault();const body={name:f.elements[0].value.trim(),avatar_key:f.elements[1].value,avatar_url:f.elements[2].value.trim(),is_kids:f.elements[3].checked,active:f.elements[4].checked};try{await req(id?`/admin/profiles/${id}`:`/admin/users/${userID}/profiles`,{method:id?'PUT':'POST',body:JSON.stringify(body)});notice('Perfil salvo.',true);v3ManageProfiles(userID)}catch(err){notice(err.message)}};
  };
  window.v3DeleteProfile=async(userID,id)=>{if(!confirm('Excluir este perfil e seu progresso?'))return;try{await req(`/admin/profiles/${id}`,{method:'DELETE'});notice('Perfil removido.',true);v3ManageProfiles(userID)}catch(err){notice(err.message)}};

  async function loadCleanup(){
    const d=await req('/admin/cleanup');
    $('#cleanup').innerHTML=`<div class="section-intro"><div><h2>Limpeza e manutenção</h2><p>Remove apenas arquivos e registros locais do StormFlix. Nada é apagado dos seus remotes/rclone.</p></div><button onclick="loadCleanup()">Atualizar análise</button></div><div class="cleanup-grid"><article class="cleanup-card cleanup-safe"><h3>Assets órfãos</h3><span class="cleanup-number">${d.orphan_asset_files}</span><p>${bytes(d.orphan_asset_bytes)} sem referência no catálogo.</p></article><article class="cleanup-card"><h3>Temporários</h3><span class="cleanup-number">${d.temp_files}</span><p>${bytes(d.temp_bytes)} em arquivos .tmp.</p></article><article class="cleanup-card"><h3>Sessões expiradas</h3><span class="cleanup-number">${d.expired_sessions}</span><p>Tokens de login que já venceram.</p></article><article class="cleanup-card"><h3>Catálogo indisponível</h3><span class="cleanup-number">${d.unavailable_media}</span><p>Itens marcados como removidos/offline no catálogo.</p></article><article class="cleanup-card"><h3>Assets totais</h3><span class="cleanup-number">${bytes(d.asset_bytes)}</span><p>${d.asset_files} arquivos locais.</p></article><article class="cleanup-card"><h3>Banco SQLite</h3><span class="cleanup-number">${bytes(d.database_bytes)}</span><p>${d.old_logs} logs com mais de 90 dias.</p></article></div><div class="panel"><h2>Ações</h2><p class="muted">A limpeza segura remove órfãos, temporários, sessões vencidas e logs com mais de 90 dias.</p><div class="cleanup-actions"><button class="primary" onclick="v3SafeCleanup()">Limpeza segura</button><button onclick="v3Vacuum()">Compactar banco</button><button class="danger" onclick="v3PurgeUnavailable()">Remover itens indisponíveis do catálogo</button></div></div>`;
  }
  window.loadCleanup=loadCleanup;
  async function doCleanup(body){try{const r=await req('/admin/cleanup',{method:'POST',body:JSON.stringify(body)});notice(`Limpeza concluída: ${r.removed_files||0} arquivos, ${bytes(r.freed_bytes||0)} liberados.`,true);loadCleanup()}catch(err){notice(err.message)}}
  window.v3SafeCleanup=()=>doCleanup({orphan_assets:true,temp_files:true,expired_sessions:true,logs_older_than_days:90});
  window.v3Vacuum=()=>doCleanup({vacuum:true});
  window.v3PurgeUnavailable=()=>{if(confirm('Remover definitivamente do catálogo os itens já marcados como indisponíveis? Isso NÃO apaga arquivos dos remotes.'))doCleanup({unavailable_media:true,orphan_assets:true})};
})();
