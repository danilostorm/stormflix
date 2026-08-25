const settingsButton=document.querySelector('nav [data-page="settings"]');
if(settingsButton)settingsButton.onclick=()=>showSettings();

async function showSettings(){
  if(phase2Timer){clearTimeout(phase2Timer);phase2Timer=null}
  $$('nav [data-page]').forEach(x=>x.classList.toggle('active',x.dataset.page==='settings'));
  $$('.page').forEach(x=>x.classList.add('hidden'));
  $('#settings').classList.remove('hidden');
  $('#page-title').textContent='Configurações';
  await loadSettings();
}

function secretField(id,label,configured,help=''){
  return `<div class="setting-field secret-field"><label for="${id}">${label}</label><div class="secret-input"><input id="${id}" type="password" autocomplete="new-password" placeholder="${configured?'Configurado — deixe vazio para manter':'Não configurado'}"><span class="secret-state ${configured?'ready':'missing'}">${configured?'CONFIGURADO':'VAZIO'}</span></div>${help?`<small>${help}</small>`:''}<label class="clear-secret"><input type="checkbox" data-clear-secret="${id}"> Limpar valor salvo</label></div>`;
}

async function loadSettings(){
  const [s,agents]=await Promise.all([req('/admin/settings'),req('/admin/agents')]);
  const secrets=s.secrets||{};
  $('#settings').innerHTML=`
    <form id="settings-form" class="settings-shell">
      <div class="settings-hero"><div><p class="kicker">Configuração central</p><h2>Tudo pelo painel</h2><p>Chaves, agentes, CDN, idiomas e experiência do catálogo são aplicados sem editar Docker Compose ou .env.</p></div><button class="primary settings-save" type="submit">Salvar configurações</button></div>

      <div class="settings-grid">
        <section class="panel settings-section">
          <div class="section-title"><span>01</span><div><h2>Geral</h2><small>Identidade e comportamento da home.</small></div></div>
          <div class="setting-field"><label>Nome do servidor</label><input id="set-server-name" value="${esc(s.server_name||'StormFlix')}" placeholder="StormFlix"></div>
          <div class="setting-field"><label>Idioma dos metadados</label><select id="set-meta-language"><option value="pt-BR">Português (Brasil)</option><option value="pt-PT">Português (Portugal)</option><option value="en-US">English</option><option value="es-ES">Español</option></select></div>
          <div class="setting-field"><label>Destaque principal da home</label><select id="set-hero-mode"><option value="featured">Melhor destaque com backdrop</option><option value="top_rated">Mais bem avaliado</option><option value="recent">Mais recente</option><option value="random">Alternar automaticamente</option></select></div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>02</span><div><h2>Metadados & Capas</h2><small>Agentes usados no scanner de filmes, séries e animes.</small></div></div>
          ${secretField('set-tmdb-token','TMDB · Read Access Token',secrets.tmdb_token,'Preferido. O API Key v3 pode ficar vazio se usar token.')}
          ${secretField('set-tmdb-key','TMDB · API Key v3',secrets.tmdb_api_key)}
          ${secretField('set-fanart-key','Fanart.tv · API Key',secrets.fanart_api_key)}
          ${secretField('set-fanart-client','Fanart.tv · Client Key',secrets.fanart_client_key,'Opcional conforme sua conta/aplicação.')}
          <div class="agent-mini-grid">${(agents.metadata||[]).map(a=>`<div><b>${esc(a.name)}</b><span class="${a.ready?'online':'offline'}">${a.ready?'PRONTO':'CONFIGURAR'}</span></div>`).join('')}</div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>03</span><div><h2>Legendas</h2><small>Busca automática opcional.</small></div></div>
          ${secretField('set-os-key','OpenSubtitles · API Key',secrets.opensubtitles_api_key)}
          <div class="setting-field"><label>OpenSubtitles · Usuário</label><input id="set-os-user" value="${esc(s.opensubtitles_username||'')}" autocomplete="off"></div>
          ${secretField('set-os-pass','OpenSubtitles · Senha',secrets.opensubtitles_password)}
          <div class="setting-field"><label>OpenSubtitles · User-Agent</label><input id="set-os-agent" value="${esc(s.opensubtitles_user_agent||'StormFlix/0.4')}"></div>
          ${secretField('set-subdl-key','SubDL · API Key',secrets.subdl_api_key)}
          <div class="setting-field"><label>Idiomas preferidos</label><input id="set-sub-langs" value="${esc(s.subtitle_languages||'pt-BR,pt,en')}" placeholder="pt-BR,pt,en"><small>Ordem de preferência, separados por vírgula.</small></div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>04</span><div><h2>CDN / Assets</h2><small>Onde ficam capas, fanart e legendas.</small></div></div>
          <div class="setting-field"><label>Diretório de assets</label><input id="set-asset-dir" value="${esc(s.asset_dir||'/data/assets')}" placeholder="/data/assets"><small>Pode ser local ou qualquer mount visível no container: Google Drive, FTP, S3, WebDAV via rclone.</small></div>
          <div class="setting-field"><label>URL pública / CDN</label><input id="set-asset-url" value="${esc(s.asset_public_base_url||'')}" placeholder="https://cdn.seudominio.com/stormflix"><small>Opcional. Deixe vazio para servir em /assets pelo próprio StormFlix.</small></div>
          <div class="storage-preview"><b>Modo atual</b><span>${esc((agents.assets||{}).mode||'local')}</span><small>${esc((agents.assets||{}).directory||s.asset_dir||'')}</small></div>
        </section>

        <section class="panel settings-section settings-wide">
          <div class="section-title"><span>05</span><div><h2>Experiência cinematográfica</h2><small>Comportamento da tela de detalhes.</small></div></div>
          <div class="experience-grid">
            <label class="switch-row"><div><b>Prévia de trilha sonora</b><small>Tenta localizar uma prévia curta associada ao título.</small></div><input id="set-theme-enabled" type="checkbox" ${s.theme_preview_enabled?'checked':''}></label>
            <label class="switch-row"><div><b>Tocar ao abrir detalhes</b><small>Depende da política de autoplay do navegador e sempre usa apenas a prévia.</small></div><input id="set-theme-autoplay" type="checkbox" ${s.theme_preview_autoplay?'checked':''}></label>
            <div class="setting-field"><label>País da busca de trilha</label><input id="set-theme-country" value="${esc(s.theme_preview_country||'BR')}" maxlength="2"></div>
            <div class="setting-field"><label>Volume da prévia <span id="volume-label">${Number(s.theme_preview_volume??24)}%</span></label><input id="set-theme-volume" type="range" min="0" max="100" value="${Number(s.theme_preview_volume??24)}"></div>
          </div>
        </section>
      </div>
    </form>`;
  $('#set-meta-language').value=s.metadata_language||'pt-BR';
  $('#set-hero-mode').value=s.home_hero_mode||'featured';
  $('#set-theme-volume').oninput=e=>$('#volume-label').textContent=e.target.value+'%';
  $('#settings-form').onsubmit=saveSettings;
}

function secretValue(id){
  const clear=$(`[data-clear-secret="${id}"]`)?.checked;
  if(clear)return'__clear__';
  const value=$('#'+id)?.value.trim()||'';
  return value||undefined;
}

async function saveSettings(e){
  e.preventDefault();
  const body={
    server_name:$('#set-server-name').value.trim(),
    metadata_language:$('#set-meta-language').value,
    home_hero_mode:$('#set-hero-mode').value,
    subtitle_languages:$('#set-sub-langs').value.trim(),
    asset_dir:$('#set-asset-dir').value.trim(),
    asset_public_base_url:$('#set-asset-url').value.trim(),
    theme_preview_enabled:$('#set-theme-enabled').checked,
    theme_preview_autoplay:$('#set-theme-autoplay').checked,
    theme_preview_country:$('#set-theme-country').value.trim().toUpperCase(),
    theme_preview_volume:+$('#set-theme-volume').value,
    opensubtitles_username:$('#set-os-user').value.trim(),
    opensubtitles_user_agent:$('#set-os-agent').value.trim(),
    tmdb_token:secretValue('set-tmdb-token'),
    tmdb_api_key:secretValue('set-tmdb-key'),
    fanart_api_key:secretValue('set-fanart-key'),
    fanart_client_key:secretValue('set-fanart-client'),
    opensubtitles_api_key:secretValue('set-os-key'),
    opensubtitles_password:secretValue('set-os-pass'),
    subdl_api_key:secretValue('set-subdl-key')
  };
  Object.keys(body).forEach(k=>body[k]===undefined&&delete body[k]);
  try{
    await req('/admin/settings',{method:'PUT',body:JSON.stringify(body)});
    notice('Configurações salvas e agentes recarregados sem reiniciar o servidor.',true);
    await loadSettings();
  }catch(err){notice(err.message)}
}
