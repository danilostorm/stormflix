const baseAgentRenderer=renderAgent;
renderAgent=function(agent){
  return `<div class="agent-card"><div class="agent-status ${agent.ready?'ready':'missing'}">${agent.ready?'● CONFIGURADO':'● CONFIGURAR'}</div><h3>${esc(agent.name)}</h3><p>${esc(agent.description||'')}</p></div>`;
};

let baseMetadataLoader=loadMetadataPhase2;

loadMetadataPhase2=async function(){
  await baseMetadataLoader();
  await appendMetadataDiagnostics();
};

async function appendMetadataDiagnostics(){
  const root=$('#metadata');
  if(!root||root.classList.contains('hidden'))return;
  let errors=[];
  try{errors=await req('/admin/metadata/errors?limit=100')}catch(err){notice(err.message);return}
  const old=$('#metadata-diagnostics');
  if(old)old.remove();
  const panel=document.createElement('div');
  panel.id='metadata-diagnostics';
  panel.className='panel';
  const diagnosis=diagnoseMetadataErrors(errors);
  panel.innerHTML=`
    <div class="panel-head">
      <div>
        <h2>Diagnóstico de metadados</h2>
        <small>“Configurado” significa que existe uma credencial salva. Use o teste abaixo para confirmar autenticação e conectividade.</small>
      </div>
      <button id="test-tmdb-agent" class="primary">Testar TMDB</button>
    </div>
    <div id="tmdb-test-result" class="phase2-hint">${esc(diagnosis)}</div>
    <div class="table-wrap" style="margin-top:16px">
      <table>
        <thead><tr><th>Mídia</th><th>Erro real</th><th>Arquivo</th><th>Atualizado</th><th>Ação</th></tr></thead>
        <tbody>${errors.map(item=>`<tr>
          <td><b>${esc(item.title)}</b><br><small>${esc(item.library)}</small></td>
          <td class="job-error"><small>${esc(item.last_error)}</small></td>
          <td><small title="${esc(item.path)}">${esc(shortPath(item.path))}</small></td>
          <td><small>${esc(item.updated_at)}</small></td>
          <td class="actions"><button data-retry-metadata="${item.media_id}">Reprocessar</button></td>
        </tr>`).join('')||'<tr><td colspan="5"><small>Nenhum erro de metadados registrado.</small></td></tr>'}</tbody>
      </table>
    </div>`;
  root.appendChild(panel);

  $('#test-tmdb-agent').onclick=async()=>{
    const result=$('#tmdb-test-result');
    result.textContent='Testando autenticação e conexão com o TMDB...';
    try{
      const data=await req('/admin/agents/tmdb/test',{method:'POST'});
      result.textContent='✓ '+(data.message||'TMDB funcionando corretamente.');
      result.classList.add('diagnostic-ok');
      notice('TMDB conectado com sucesso.',true);
    }catch(err){
      result.textContent='✕ '+friendlyTMDBError(err.message);
      result.classList.remove('diagnostic-ok');
      notice(err.message);
    }
  };

  $$('[data-retry-metadata]').forEach(button=>button.onclick=async()=>{
    button.disabled=true;
    button.textContent='Processando...';
    try{
      await req(`/admin/media/${button.dataset.retryMetadata}/metadata`,{method:'POST'});
      notice('Mídia reprocessada.',true);
      await loadMetadataPhase2();
    }catch(err){
      notice(err.message);
      button.disabled=false;
      button.textContent='Reprocessar';
    }
  });
}

function diagnoseMetadataErrors(items){
  if(!items.length)return'Nenhum erro registrado. Use “Testar TMDB” para validar a conexão do agente.';
  const messages=items.map(x=>String(x.last_error||''));
  if(messages.some(x=>/HTTP 401|unauthor/i.test(x)))return'Provável causa: credencial TMDB inválida. Se você colou “Bearer ...”, a nova versão corrige esse formato automaticamente; teste o agente novamente.';
  if(messages.some(x=>/HTTP 403|forbidden/i.test(x)))return'Provável causa: a credencial foi reconhecida, mas o TMDB recusou o acesso (HTTP 403). Confira permissões/chave.';
  if(messages.some(x=>/HTTP 429|rate/i.test(x)))return'Provável causa: limite temporário da API TMDB (HTTP 429). Aguarde e tente novamente.';
  if(messages.every(x=>/no match|not found/i.test(x)))return'Os agentes responderam, mas não encontraram correspondências. O próximo ponto a revisar são os nomes dos arquivos e o parser de títulos.';
  if(messages.some(x=>/timeout|connection|network|dial|lookup/i.test(x)))return'Provável causa: falha de rede/DNS entre o container StormFlix e a API de metadados.';
  return `${items.length} mídia(s) com erro. A causa exata aparece na coluna “Erro real”.`;
}

function friendlyTMDBError(message){
  const text=String(message||'Falha desconhecida');
  if(/HTTP 401/.test(text))return'TMDB respondeu HTTP 401: token/API key inválido. Abra Configurações → Metadados & Capas e substitua a credencial.';
  if(/HTTP 403/.test(text))return'TMDB respondeu HTTP 403: acesso recusado para esta credencial.';
  if(/HTTP 429/.test(text))return'TMDB respondeu HTTP 429: limite temporário de requisições atingido.';
  return text;
}

function shortPath(path){
  const value=String(path||'');
  if(value.length<=58)return value;
  return '…'+value.slice(-57);
}
