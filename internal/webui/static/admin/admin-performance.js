/* StormFlix storage/performance maintenance controls. */
(function(){
  if(typeof req!=='function')return;
  let renderGeneration=0;

  function cleanupCard(title,value,note){
    return `<article class="metric-card"><div><strong>${value}</strong><span>${esc(title)}</span></div><small>${esc(note||'')}</small></article>`;
  }

  async function loadPerformanceCleanup(){
    const generation=++renderGeneration;
    const c=await req('/admin/cleanup');
    // Ignore a response from an older refresh if the user clicked Atualizar
    // again while the first request was still in flight.
    if(generation!==renderGeneration)return;
    const root=$('#cleanup');
    if(!root)return;
    const physical=Number(c.asset_physical_bytes||c.asset_bytes||0);
    const logical=Number(c.asset_bytes||0);
    const saved=Number(c.asset_dedup_savings_bytes||0);
    root.innerHTML=`
      <div class="section-intro"><div><p class="kicker">Armazenamento local</p><h2>Limpeza e otimização</h2><p>Remove somente cache/registros locais do StormFlix. Seus remotes e arquivos de mídia nunca são apagados.</p></div><div class="v3-toolbar"><button onclick="sfRefreshCleanup()">Atualizar análise</button></div></div>
      <div class="metric-grid">
        ${cleanupCard('Assets no disco',bytes(physical),`${c.asset_files||0} caminhos · ${bytes(logical)} lógicos`)}
        ${cleanupCard('Economia por dedupe',bytes(saved),saved>0?'Arquivos idênticos já compartilham espaço físico.':'Nenhuma economia por hardlinks aplicada ainda.')}
        ${cleanupCard('Assets órfãos',c.orphan_asset_files||0,`${bytes(c.orphan_asset_bytes||0)} sem referência no catálogo`)}
        ${cleanupCard('Temporários',c.temp_files||0,`${bytes(c.temp_bytes||0)} em arquivos temporários`)}
        ${cleanupCard('Sessões expiradas',c.expired_sessions||0,'Tokens de login que já venceram')}
        ${cleanupCard('Catálogo indisponível',c.unavailable_media||0,'Itens marcados como removidos/offline')}
        ${cleanupCard('Banco SQLite',bytes(c.database_bytes||0),`${c.old_logs||0} logs com mais de 90 dias`)}
      </div>
      <div class="panel">
        <div class="panel-head"><div><h2>Otimização segura de assets</h2><p>Procura posters, backdrops, logos e outros assets byte-a-byte idênticos. Duplicados continuam com os mesmos caminhos/URLs, mas passam a compartilhar o mesmo arquivo físico por hardlink. Não há recompressão nem perda de qualidade.</p></div></div>
        <div class="actions">
          <button class="primary" onclick="sfOptimizeAssets()">Otimizar assets sem perda</button>
          <button onclick="sfSafeCleanup()">Limpeza segura</button>
          <button onclick="sfVacuumDatabase()">Compactar banco</button>
          <button class="danger" onclick="sfRemoveUnavailable()">Remover itens indisponíveis do catálogo</button>
        </div>
        <p><small>A primeira otimização pode levar alguns minutos porque arquivos do mesmo tamanho são comparados por SHA-256. Passagens seguintes são idempotentes.</small></p>
      </div>`;
  }

  // Core admin navigation owns page loading. Exposing one loader here avoids
  // stacking wrappers around the global show() function, which could let stale
  // cleanup renderers race and replace this panel after it appeared.
  window.loadPerformanceCleanup=loadPerformanceCleanup;
  window.sfRefreshCleanup=async()=>{try{await loadPerformanceCleanup()}catch(err){notice(err.message)}};
  window.sfOptimizeAssets=async()=>{
    if(!confirm('Otimizar assets idênticos sem alterar qualidade, nomes ou URLs?'))return;
    notice('Analisando e consolidando assets idênticos…');
    try{
      const r=await req('/admin/cleanup',{method:'POST',body:JSON.stringify({deduplicate_assets:true})});
      const o=r.asset_optimization||{};
      notice(`Otimização concluída: ${o.linked_files||0} duplicados consolidados · ${bytes(o.saved_bytes||0)} liberados.`,true);
      await loadPerformanceCleanup();
    }catch(err){notice(err.message)}
  };
  window.sfSafeCleanup=async()=>{
    try{
      const r=await req('/admin/cleanup',{method:'POST',body:JSON.stringify({orphan_assets:true,temp_files:true,expired_sessions:true,logs_older_than_days:90})});
      notice(`Limpeza concluída · ${bytes(r.freed_bytes||0)} removidos.`,true);await loadPerformanceCleanup();
    }catch(err){notice(err.message)}
  };
  window.sfVacuumDatabase=async()=>{
    try{await req('/admin/cleanup',{method:'POST',body:JSON.stringify({vacuum:true})});notice('Banco compactado.',true);await loadPerformanceCleanup()}catch(err){notice(err.message)}
  };
  window.sfRemoveUnavailable=async()=>{
    if(!confirm('Remover do catálogo os itens atualmente marcados como indisponíveis? Os arquivos de mídia não serão apagados.'))return;
    try{await req('/admin/cleanup',{method:'POST',body:JSON.stringify({unavailable_media:true})});notice('Itens indisponíveis removidos do catálogo.',true);await loadPerformanceCleanup()}catch(err){notice(err.message)}
  };
})();
