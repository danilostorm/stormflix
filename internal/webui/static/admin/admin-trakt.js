/* StormFlix Admin: Trakt application credentials.
 * The application is configured once by an administrator; each StormFlix
 * profile then authorizes its own Trakt account through Device OAuth.
 */
(function(){
  if(typeof loadSettings!=='function')return;
  const baseLoadSettings=loadSettings;

  loadSettings=async function(){
    await baseLoadSettings();
    await renderAdminTrakt();
  };

  async function renderAdminTrakt(){
    const grid=document.querySelector('#settings .settings-grid');
    if(!grid||document.querySelector('#settings-trakt-section'))return;
    let s;
    try{s=await req('/admin/settings')}catch{return}
    const section=document.createElement('section');
    section.id='settings-trakt-section';
    section.className='panel settings-section settings-wide';
    section.innerHTML=`
      <div class="section-title"><span>08</span><div><h2>Trakt · Contas por perfil</h2><small>Configure o aplicativo uma vez. Cada perfil vincula a própria conta sem compartilhar histórico ou token.</small></div></div>
      <div class="agent-mini-grid">
        <div><b>Aplicativo</b><span class="${s.trakt_configured?'online':'offline'}">${s.trakt_configured?'PRONTO':'CONFIGURAR'}</span></div>
        <div><b>Client ID</b><span>${s.trakt_client_id_configured?'CONFIGURADO':'VAZIO'}</span></div>
        <div><b>Client Secret</b><span>${s.trakt_client_secret_configured?'CONFIGURADO':'VAZIO'}</span></div>
      </div>
      <div class="experience-grid">
        <div class="setting-field"><label>Trakt · Client ID</label><input id="set-trakt-client-id" type="password" autocomplete="new-password" placeholder="${s.trakt_client_id_configured?'Configurado — deixe vazio para manter':'Cole o Client ID do aplicativo'}"><small>Fica criptografado no servidor. Não é o token pessoal de nenhum usuário.</small><label class="clear-secret"><input id="clear-trakt-client-id" type="checkbox"> Limpar valor salvo</label></div>
        <div class="setting-field"><label>Trakt · Client Secret</label><input id="set-trakt-client-secret" type="password" autocomplete="new-password" placeholder="${s.trakt_client_secret_configured?'Configurado — deixe vazio para manter':'Cole o Client Secret do aplicativo'}"><small>Armazenado com AES-GCM usando a chave local do StormFlix.</small><label class="clear-secret"><input id="clear-trakt-client-secret" type="checkbox"> Limpar valor salvo</label></div>
        <div class="setting-field"><label>Redirect URI</label><input id="set-trakt-redirect" value="${esc(s.trakt_redirect_uri||'urn:ietf:wg:oauth:2.0:oob')}"><small>Para Device OAuth, o padrão recomendado no StormFlix é <code>urn:ietf:wg:oauth:2.0:oob</code>.</small></div>
        <div class="storage-preview"><b>Como funciona</b><span>Admin configura app → perfil recebe código → usuário autoriza no Trakt</span><small>Scrobble é assíncrono: indisponibilidade do Trakt nunca deve bloquear Direct Play, progresso local ou a Home.</small></div>
      </div>
      <div style="margin-top:12px;display:flex;gap:10px;align-items:center;flex-wrap:wrap"><button id="save-trakt-settings" class="secondary" type="button">Salvar configuração Trakt</button><small>Depois disso, abra/edite um perfil no site e use “Conectar Trakt”.</small></div>`;
    grid.appendChild(section);
    document.querySelector('#save-trakt-settings').onclick=saveAdminTrakt;
  }

  async function saveAdminTrakt(e){
    const button=e.currentTarget;
    const clientID=document.querySelector('#set-trakt-client-id')?.value.trim()||'';
    const clientSecret=document.querySelector('#set-trakt-client-secret')?.value.trim()||'';
    const body={
      trakt_redirect_uri:document.querySelector('#set-trakt-redirect')?.value.trim()||'urn:ietf:wg:oauth:2.0:oob'
    };
    if(document.querySelector('#clear-trakt-client-id')?.checked)body.trakt_client_id='__clear__';
    else if(clientID)body.trakt_client_id=clientID;
    if(document.querySelector('#clear-trakt-client-secret')?.checked)body.trakt_client_secret='__clear__';
    else if(clientSecret)body.trakt_client_secret=clientSecret;
    button.disabled=true;button.textContent='Salvando…';
    try{
      await req('/admin/settings',{method:'PUT',body:JSON.stringify(body)});
      notice('Trakt configurado. Cada perfil já pode vincular sua própria conta.',true);
      await loadSettings();
    }catch(err){notice(err.message)}finally{button.disabled=false;button.textContent='Salvar configuração Trakt'}
  }
})();
