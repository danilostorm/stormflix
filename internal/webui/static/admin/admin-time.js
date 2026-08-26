/* StormFlix stores SQLite timestamps in UTC. Render them in the administrator browser timezone. */
(function(){
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
    $('#users').innerHTML=`<div class="panel"><div class="panel-head"><h2>Usuários</h2><button class="primary" onclick="editUser()">+ Criar</button></div><div id="user-form"></div><div class="table-wrap"><table><thead><tr><th>Usuário</th><th>Perfil</th><th>Status</th><th>Bibliotecas</th><th>Último login</th><th>Ações</th></tr></thead><tbody>${users.map(u=>`<tr><td><b>${esc(u.display_name)}</b><br><small>@${esc(u.username)}</small></td><td>${esc(u.role)}</td><td>${u.active?'<span class="online">ATIVO</span>':'<span class="offline">BLOQUEADO</span>'}</td><td>${u.library_ids?.length||0}</td><td><small>${esc(stormLocalDate(u.last_login_at))}</small></td><td class="actions"><button onclick="editUser(${u.id})">Editar</button>${u.id!==me.id?`<button class="danger" onclick="delUser(${u.id})">Excluir</button>`:''}</td></tr>`).join('')}</tbody></table></div><div class="phase2-hint">Horários são armazenados em UTC e exibidos no fuso do navegador administrador.</div></div>`;
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
