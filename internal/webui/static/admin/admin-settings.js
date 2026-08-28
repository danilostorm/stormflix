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

function settingBytes(value){
  const n=Number(value)||0;
  if(n<=0)return'0 B';
  const units=['B','KB','MB','GB','TB'];let i=0,x=n;
  while(x>=1024&&i<units.length-1){x/=1024;i++}
  return`${x.toFixed(i>=3?1:0)} ${units[i]}`;
}

function settingAge(value){
  if(!value)return'—';
  const ts=new Date(value).getTime();if(!Number.isFinite(ts))return'—';
  const seconds=Math.max(0,Math.round((Date.now()-ts)/1000));
  if(seconds<60)return'há poucos segundos';
  if(seconds<3600)return`há ${Math.round(seconds/60)} min`;
  if(seconds<86400)return`há ${Math.round(seconds/3600)} h`;
  return`há ${Math.round(seconds/86400)} dias`;
}

function cacheLimitOptions(current){
  const options=[[5,5*1024**3],[10,10*1024**3],[20,20*1024**3],[50,50*1024**3],[100,100*1024**3]];
  let html=options.map(([gb,bytes])=>`<option value="${bytes}">${gb} GB</option>`).join('');
  if(Number(current)===0)html+=`<option value="0">Ilimitado</option>`;
  else if(!options.some(([,bytes])=>bytes===Number(current)))html+=`<option value="${Number(current)}">Personalizado · ${settingBytes(current)}</option>`;
  else html+=`<option value="0">Ilimitado</option>`;
  return html;
}

async function loadSettings(){
  const [s,agents,cache]=await Promise.all([req('/admin/settings'),req('/admin/agents'),req('/admin/playback/cache')]);
  const secrets=s.secrets||{};
  const lastCleanup=cache.last_cleanup||{};
  const cacheLimit=Number(s.compat_cache_max_bytes??20*1024**3);
  const cacheTTL=Number(s.compat_cache_ttl_hours??48);
  const freeReserve=Number(s.compat_cache_min_free_bytes??10*1024**3);
  $('#settings').innerHTML=`
    <form id="settings-form" class="settings-shell">
      <div class="settings-hero"><div><p class="kicker">Configuração central</p><h2>Tudo pelo painel</h2><p>Chaves, agentes, CDN, idiomas, experiência do catálogo e cache de reprodução são aplicados sem editar Docker Compose ou .env.</p></div><button class="primary settings-save" type="submit">Salvar configurações</button></div>

      <div class="settings-grid">
        <section class="panel settings-section">
          <div class="section-title"><span>01</span><div><h2>Geral</h2><small>Identidade e comportamento da home.</small></div></div>
          <div class="setting-field"><label>Nome do servidor</label><input id="set-server-name" value="${esc(s.server_name||'StormFlix')}" placeholder="StormFlix"></div>
          <div class="setting-field"><label>Idioma dos metadados</label><select id="set-meta-language"><option value="pt-BR">Português (Brasil)</option><option value="pt-PT">Português (Portugal)</option><option value="en-US">English</option><option value="es-ES">Español</option></select></div>
          <div class="setting-field"><label>Destaque principal da home</label><select id="set-hero-mode"><option value="featured">Melhor destaque com backdrop</option><option value="top_rated">Mais bem avaliado</option><option value="recent">Mais recente</option><option value="random">Alternar automaticamente</option></select></div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>02</span><div><h2>Metadados & Capas</h2><small>Agentes usados no scanner de filmes, séries, desenhos e animes.</small></div></div>
          ${secretField('set-tmdb-token','TMDB · Read Access Token',secrets.tmdb_token,'Preferido. O API Key v3 pode ficar vazio se usar token.')}
          ${secretField('set-tmdb-key','TMDB · API Key v3',secrets.tmdb_api_key)}
          ${secretField('set-tvdb-key','TheTVDB v4 · API Key',secrets.tvdb_api_key,'Usado como fallback para séries/desenhos e para ordens de episódios. A chave é criada no painel TheTVDB.')}
          ${secretField('set-tvdb-pin','TheTVDB · Subscriber PIN',secrets.tvdb_pin,'Só é necessário quando a sua chave/projeto TheTVDB usa o modelo de assinatura do usuário.')}
          ${secretField('set-fanart-key','Fanart.tv · API Key',secrets.fanart_api_key)}
          ${secretField('set-fanart-client','Fanart.tv · Client Key',secrets.fanart_client_key,'Opcional conforme sua conta/aplicação.')}
          <div class="agent-mini-grid">${(agents.metadata||[]).map(a=>`<div><b>${esc(a.name)}</b><span class="${a.ready?'online':'offline'}">${a.ready?'PRONTO':'CONFIGURAR'}</span></div>`).join('')}</div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>03</span><div><h2>Música & identificação</h2><small>Fallbacks para coleções com tags incompletas ou nomes de pasta irregulares.</small></div></div>
          ${secretField('set-lastfm-key','Last.fm · API Key',secrets.lastfm_api_key,'Opcional, mas recomendado para sua coleção. Ajuda a corrigir artista, faixa, álbum e tags quando o arquivo não possui metadados confiáveis.')}
          <div class="storage-preview"><b>Ordem de identificação</b><span>Tags → nome do arquivo → Last.fm → MusicBrainz</span><small>O StormFlix reconhece padrões como “Artista - Título.mp3” antes de usar a estrutura de pastas.</small></div>
          <div class="agent-mini-grid">${(agents.music||[]).map(a=>`<div><b>${esc(a.name)}</b><span class="${a.ready?'online':'offline'}">${a.ready?'PRONTO':a.enabled?'INDISPONÍVEL':'OPCIONAL'}</span></div>`).join('')}</div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>04</span><div><h2>Legendas</h2><small>Busca automática opcional.</small></div></div>
          ${secretField('set-os-key','OpenSubtitles · API Key',secrets.opensubtitles_api_key)}
          <div class="setting-field"><label>OpenSubtitles · Usuário</label><input id="set-os-user" value="${esc(s.opensubtitles_username||'')}" autocomplete="off"></div>
          ${secretField('set-os-pass','OpenSubtitles · Senha',secrets.opensubtitles_password)}
          <div class="setting-field"><label>OpenSubtitles · User-Agent</label><input id="set-os-agent" value="${esc(s.opensubtitles_user_agent||'StormFlix/0.4')}"></div>
          ${secretField('set-subdl-key','SubDL · API Key',secrets.subdl_api_key)}
          <div class="setting-field"><label>Idiomas preferidos</label><input id="set-sub-langs" value="${esc(s.subtitle_languages||'pt-BR,pt,en')}" placeholder="pt-BR,pt,en"><small>Ordem de preferência, separados por vírgula.</small></div>
        </section>

        <section class="panel settings-section">
          <div class="section-title"><span>05</span><div><h2>CDN / Assets</h2><small>Onde ficam capas, fanart e legendas.</small></div></div>
          <div class="setting-field"><label>Diretório de assets</label><input id="set-asset-dir" value="${esc(s.asset_dir||'/data/assets')}" placeholder="/data/assets"><small>Pode ser local ou qualquer mount visível no container: Google Drive, FTP, S3, WebDAV via rclone.</small></div>
          <div class="setting-field"><label>URL pública / CDN</label><input id="set-asset-url" value="${esc(s.asset_public_base_url||'')}" placeholder="https://cdn.seudominio.com/stormflix"><small>Opcional. Deixe vazio para servir em /assets pelo próprio StormFlix.</small></div>
          <div class="storage-preview"><b>Modo atual</b><span>${esc((agents.assets||{}).mode||'local')}</span><small>${esc((agents.assets||{}).directory||s.asset_dir||'')}</small></div>
        </section>

        <section class="panel settings-section settings-wide">
          <div class="section-title"><span>06</span><div><h2>Experiência cinematográfica</h2><small>Comportamento da tela de detalhes.</small></div></div>
          <div class="experience-grid">
            <label class="switch-row"><div><b>Prévia de trilha sonora</b><small>Tenta localizar uma prévia curta associada ao título.</small></div><input id="set-theme-enabled" type="checkbox" ${s.theme_preview_enabled?'checked':''}></label>
            <label class="switch-row"><div><b>Tocar ao abrir detalhes</b><small>Depende da política de autoplay do navegador e sempre usa apenas a prévia.</small></div><input id="set-theme-autoplay" type="checkbox" ${s.theme_preview_autoplay?'checked':''}></label>
            <div class="setting-field"><label>País da busca de trilha</label><input id="set-theme-country" value="${esc(s.theme_preview_country||'BR')}" maxlength="2"></div>
            <div class="setting-field"><label>Volume da prévia <span id="volume-label">${Number(s.theme_preview_volume??24)}%</span></label><input id="set-theme-volume" type="range" min="0" max="100" value="${Number(s.theme_preview_volume??24)}"></div>
          </div>
        </section>

        <section class="panel settings-section settings-wide">
          <div class="section-title"><span>07</span><div><h2>Playback · Cache de compatibilidade</h2><small>Controle das versões MP4 seekable usadas quando o navegador/dispositivo precisa de remux ou áudio AAC.</small></div></div>
          <div class="agent-mini-grid">
            <div><b>Uso atual</b><span>${settingBytes(cache.usage_bytes)}</span></div>
            <div><b>Limite</b><span>${cache.max_bytes?settingBytes(cache.max_bytes):'ILIMITADO'}</span></div>
            <div><b>Arquivos</b><span>${Number(cache.files||0)}</span></div>
            <div><b>Em uso agora</b><span>${Number(cache.active_files||0)}</span></div>
            <div><b>Mais antigo</b><span>${settingAge(cache.oldest_last_used_at)}</span></div>
            <div><b>Espaço livre</b><span>${settingBytes(cache.free_bytes)}</span></div>
          </div>
          <div class="experience-grid">
            <div class="setting-field"><label>Limite máximo</label><select id="set-compat-cache-limit">${cacheLimitOptions(cacheLimit)}</select><small>O padrão é 20 GB. Ao ultrapassar, o StormFlix remove primeiro as versões menos usadas.</small></div>
            <div class="setting-field"><label>Expirar após</label><select id="set-compat-cache-ttl"><option value="24">24 horas</option><option value="48">48 horas</option><option value="72">72 horas</option><option value="168">7 dias</option><option value="0">Sem TTL</option></select><small>O limite de tamanho continua valendo mesmo sem TTL.</small></div>
            <div class="setting-field"><label>Reserva mínima livre</label><select id="set-compat-cache-free"><option value="5368709120">5 GB</option><option value="10737418240">10 GB</option><option value="21474836480">20 GB</option><option value="0">Somente percentual</option></select><small>Antes de materializar arquivos grandes, o cache tenta liberar espaço.</small></div>
            <div class="setting-field"><label>Reserva mínima do disco (%)</label><input id="set-compat-cache-free-percent" type="number" min="0" max="95" value="${Number(s.compat_cache_min_free_percent??5)}"><small>É usado o maior valor entre a reserva em GB e este percentual.</small></div>
            <label class="switch-row"><div><b>Limpeza automática</b><small>Executa limpeza no startup e periodicamente sem interromper reprodução ativa.</small></div><input id="set-compat-cache-auto" type="checkbox" ${s.compat_cache_auto_cleanup!==false?'checked':''}></label>
            <div class="storage-preview"><b>Última limpeza</b><span>${settingAge(lastCleanup.finished_at)}</span><small>${lastCleanup.finished_at?`${Number(lastCleanup.files_removed||0)} arquivo(s) · ${settingBytes(lastCleanup.bytes_removed)} liberados`:'Aguardando primeira limpeza registrada.'}</small></div>
          </div>
          <div class="storage-preview"><b>Arquivos maiores que o limite</b><span>Temporários / oversize</span><small>Uma mídia de 42 GB pode ser preparada para manter seek/Range, mas não fica permanentemente presa num cache de 20 GB: após ficar sem uso ela é descartada automaticamente.</small></div>
          <div style="margin-top:12px"><button id="compat-cache-clean" class="secondary" type="button">Limpar cache agora</button></div>
        </section>
      </div>
    </form>`;
  $('#set-meta-language').value=s.metadata_language||'pt-BR';
  $('#set-hero-mode').value=s.home_hero_mode||'featured';
  $('#set-theme-volume').oninput=e=>$('#volume-label').textContent=e.target.value+'%';
  $('#set-compat-cache-limit').value=String(cacheLimit);
  $('#set-compat-cache-ttl').value=String(cacheTTL);
  if(![24,48,72,168,0].includes(cacheTTL)){
    $('#set-compat-cache-ttl').insertAdjacentHTML('beforeend',`<option value="${cacheTTL}">${cacheTTL} horas · personalizado</option>`);
    $('#set-compat-cache-ttl').value=String(cacheTTL);
  }
  $('#set-compat-cache-free').value=String(freeReserve);
  if(![5*1024**3,10*1024**3,20*1024**3,0].includes(freeReserve)){
    $('#set-compat-cache-free').insertAdjacentHTML('beforeend',`<option value="${freeReserve}">${settingBytes(freeReserve)} · personalizado</option>`);
    $('#set-compat-cache-free').value=String(freeReserve);
  }
  $('#compat-cache-clean').onclick=async e=>{
    const button=e.currentTarget;button.disabled=true;button.textContent='Limpando…';
    try{
      const result=await req('/admin/playback/cache/cleanup',{method:'POST',body:'{}'});
      notice(`Cache limpo: ${Number(result.files_removed||0)} arquivo(s), ${settingBytes(result.bytes_removed)} liberados.`,true);
      await loadSettings();
    }catch(err){notice(err.message)}finally{button.disabled=false}
  };
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
    compat_cache_max_bytes:+$('#set-compat-cache-limit').value,
    compat_cache_ttl_hours:+$('#set-compat-cache-ttl').value,
    compat_cache_auto_cleanup:$('#set-compat-cache-auto').checked,
    compat_cache_min_free_bytes:+$('#set-compat-cache-free').value,
    compat_cache_min_free_percent:+$('#set-compat-cache-free-percent').value,
    opensubtitles_username:$('#set-os-user').value.trim(),
    opensubtitles_user_agent:$('#set-os-agent').value.trim(),
    tmdb_token:secretValue('set-tmdb-token'),
    tmdb_api_key:secretValue('set-tmdb-key'),
    tvdb_api_key:secretValue('set-tvdb-key'),
    tvdb_pin:secretValue('set-tvdb-pin'),
    fanart_api_key:secretValue('set-fanart-key'),
    fanart_client_key:secretValue('set-fanart-client'),
    lastfm_api_key:secretValue('set-lastfm-key'),
    opensubtitles_api_key:secretValue('set-os-key'),
    opensubtitles_password:secretValue('set-os-pass'),
    subdl_api_key:secretValue('set-subdl-key')
  };
  Object.keys(body).forEach(k=>body[k]===undefined&&delete body[k]);
  try{
    await req('/admin/settings',{method:'PUT',body:JSON.stringify(body)});
    notice('Configurações salvas e agentes/cache recarregados sem reiniciar o servidor.',true);
    await loadSettings();
  }catch(err){notice(err.message)}
}
