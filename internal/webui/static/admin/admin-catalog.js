/* StormFlix Admin Catalog: Plex-style principal-work matching. */
(function(){
  let catalogItems=[];
  let activeMedia=null;
  let searchTimer=null;
  let catalogView='works';

  function init(){
    const nav=document.querySelector('nav [data-page="catalog"]');
    if(nav)nav.addEventListener('click',()=>setTimeout(loadCatalog,0));
  }

  async function loadCatalog(){
    const root=document.querySelector('#catalog');if(!root)return;
    root.innerHTML='<div class="catalog-admin-loading">Carregando catálogo…</div>';
    try{
      if(!Array.isArray(libs)||!libs.length)libs=await req('/admin/storage');
      root.innerHTML=`<div class="catalog-admin-head"><div><p class="kicker">Controle de correspondência</p><h2>Catálogo</h2><span>No modo Obras principais, séries, animes com temporadas e desenhos aparecem uma única vez. Corrija a obra principal e o scanner reorganiza os episódios dela.</span></div><div class="catalog-admin-tools"><input id="catalog-admin-search" placeholder="Buscar obra principal, título ou TMDB ID"><select id="catalog-admin-library"><option value="">Todas as bibliotecas</option>${libs.map(l=>`<option value="${l.id}">${esc(l.name)}</option>`).join('')}</select><select id="catalog-admin-view"><option value="works">Obras principais</option><option value="files">Arquivos / diagnóstico</option></select><button id="catalog-admin-refresh">Atualizar</button></div></div><div id="catalog-admin-results"></div>`;
      $('#catalog-admin-view').value=catalogView;
      $('#catalog-admin-search').oninput=()=>{clearTimeout(searchTimer);searchTimer=setTimeout(fetchCatalog,300)};
      $('#catalog-admin-library').onchange=fetchCatalog;
      $('#catalog-admin-view').onchange=e=>{catalogView=e.target.value;fetchCatalog()};
      $('#catalog-admin-refresh').onclick=fetchCatalog;
      await fetchCatalog();
    }catch(err){root.innerHTML=`<div class="panel"><p class="offline">${esc(err.message)}</p></div>`}
  }

  async function fetchCatalog(){
    const target=$('#catalog-admin-results');if(!target)return;
    target.innerHTML='<div class="catalog-admin-loading">Consultando…</div>';
    const q=$('#catalog-admin-search')?.value.trim()||'';
    const lib=$('#catalog-admin-library')?.value||'';
    try{
      const endpoint=catalogView==='works'?'/admin/catalog/works':'/admin/catalog';
      catalogItems=await req(`${endpoint}?limit=160${q?`&q=${encodeURIComponent(q)}`:''}${lib?`&library_id=${encodeURIComponent(lib)}`:''}`);
      renderCatalog(target,catalogItems);
    }catch(err){target.innerHTML=`<p class="offline">${esc(err.message)}</p>`}
  }

  function renderCatalog(root,items){
    if(!items.length){root.innerHTML='<div class="catalog-admin-empty">Nenhum item encontrado.</div>';return}
    root.innerHTML=`<div class="catalog-admin-grid">${items.map(item=>{
      const isSeries=item.entity_type==='series';
      const poster=item.poster_url?`<img src="${esc(item.poster_url)}" alt="" loading="lazy">`:'<div class="catalog-admin-poster-fallback">SF</div>';
      const status=item.manual_series?'<span class="catalog-match-badge manual">OBRA MANUAL · PROTEGIDA</span>':item.manual_match?'<span class="catalog-match-badge manual">ITEM MANUAL · PROTEGIDO</span>':item.metadata_status==='matched'?'<span class="catalog-match-badge ok">AUTOMÁTICO</span>':`<span class="catalog-match-badge error">${esc(item.metadata_status||'PENDENTE').toUpperCase()}</span>`;
      const scopeLine=isSeries?`<p><b>Obra principal</b> · ${Number(item.season_count||0)} temporada(s) · ${Number(item.episode_count||0)} episódio(s)</p>`:item.series_key?`<p><b>${esc(item.series_title||'Série detectada')}</b> · arquivo gerenciado pela obra principal</p>`:'';
      const meta=isSeries?`<span>TMDB <b>${item.tmdb_id||'—'}</b></span><span>Temporadas <b>${Number(item.season_count||0)}</b></span><span>Episódios <b>${Number(item.episode_count||0)}</b></span>`:`<span>TMDB <b>${item.tmdb_id||'—'}</b></span><span>Classificação <b>${esc(item.content_rating||'—')}</b></span><span>Lançamento <b>${esc(item.release_date||'—')}</b></span>`;
      let actions='';
      if(isSeries){
        actions=`<button class="primary" data-fix-match="${item.id}">Corrigir obra principal</button>${item.manual_series?`<button data-auto-series="${item.id}">Obra → automático</button>`:''}`;
      }else if(catalogView==='files'&&item.series_key){
        actions=`<button data-open-series="${item.id}">Abrir obra principal</button>`;
      }else{
        actions=`<button class="primary" data-fix-match="${item.id}">Corrigir correspondência</button>${item.manual_match?`<button data-auto-match="${item.id}">Item → automático</button>`:''}`;
      }
      const pathLine=isSeries?`<code title="${esc(item.path||'')}">${esc(item.series_title||item.title)}</code>`:`<code title="${esc(item.path)}">${esc(item.path)}</code>`;
      return `<article class="catalog-admin-card"><div class="catalog-admin-poster">${poster}</div><div class="catalog-admin-info"><div class="catalog-admin-title"><div><h3>${esc(item.title)}</h3><p>${esc(item.library_name)} · ${isSeries?'obra episódica':esc(item.media_type||'sem tipo')} ${item.year?`· ${item.year}`:''}</p>${scopeLine}</div>${status}</div><div class="catalog-admin-meta">${meta}</div>${pathLine}${item.last_error?`<p class="catalog-admin-error">${esc(item.last_error)}</p>`:''}<div class="catalog-admin-actions">${actions}</div></div></article>`}).join('')}</div>`;
    root.querySelectorAll('[data-fix-match]').forEach(b=>b.onclick=()=>openMatch(Number(b.dataset.fixMatch)));
    root.querySelectorAll('[data-auto-match]').forEach(b=>b.onclick=()=>resetAuto(Number(b.dataset.autoMatch),false));
    root.querySelectorAll('[data-auto-series]').forEach(b=>b.onclick=()=>resetAuto(Number(b.dataset.autoSeries),true));
    root.querySelectorAll('[data-open-series]').forEach(b=>b.onclick=()=>openSeriesFromFile(Number(b.dataset.openSeries)));
  }

  function openSeriesFromFile(id){
    const item=catalogItems.find(x=>Number(x.id)===Number(id));if(!item)return;
    catalogView='works';
    const view=$('#catalog-admin-view');if(view)view.value='works';
    const search=$('#catalog-admin-search');if(search)search.value=(item.series_title||'').trim();
    fetchCatalog();
  }

  async function openMatch(id){
    activeMedia=catalogItems.find(x=>Number(x.id)===Number(id));if(!activeMedia)return;
    const hasSeries=activeMedia.entity_type==='series';
    const hint=(activeMedia.series_title||activeMedia.title||'').trim()||filenameHint(activeMedia.path);
    let overlay=$('#catalog-match-overlay');
    if(!overlay){overlay=document.createElement('div');overlay.id='catalog-match-overlay';overlay.className='catalog-match-overlay';document.body.appendChild(overlay)}
    overlay.innerHTML=`<div class="catalog-match-dialog"><header><div><p class="kicker">${hasSeries?'CORRESPONDÊNCIA DA OBRA PRINCIPAL':'CORRESPONDÊNCIA MANUAL'}</p><h2>${esc(hasSeries?(activeMedia.series_title||activeMedia.title):activeMedia.title)}</h2><span>${hasSeries?'Esta escolha pertence à série principal. O StormFlix preserva a identidade da pasta, reescaneia a série e aplica metadados aos episódios atuais e futuros.':esc(activeMedia.path)}</span></div><button id="catalog-match-close">✕</button></header><div class="catalog-match-search"><input id="catalog-match-query" value="${esc(hint)}" placeholder="Pesquisar no TMDB"><button id="catalog-match-search-button" class="primary">Pesquisar</button></div>${hasSeries?`<div class="phase2-hint"><b>Escopo: obra inteira.</b> Não existe correspondência manual por episódio neste fluxo. Temporada e episódio continuam pertencendo ao scanner.</div>`:`<label class="catalog-copy-option"><input type="checkbox" id="catalog-match-copies" checked><span>Aplicar também às cópias deste mesmo título nos outros servidores/origens desta biblioteca</span></label>`}<div id="catalog-match-results" class="catalog-match-results"></div></div>`;
    overlay.classList.remove('hidden');
    $('#catalog-match-close').onclick=()=>overlay.classList.add('hidden');
    overlay.onclick=e=>{if(e.target===overlay)overlay.classList.add('hidden')};
    $('#catalog-match-search-button').onclick=searchMatches;
    $('#catalog-match-query').onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();searchMatches()}};
    await searchMatches();
  }

  async function searchMatches(){
    if(!activeMedia)return;
    const root=$('#catalog-match-results');
    const q=$('#catalog-match-query').value.trim();
    root.innerHTML='<div class="catalog-admin-loading">Buscando no TMDB…</div>';
    try{
      const items=await req(`/admin/catalog/${activeMedia.id}/matches${q?`?q=${encodeURIComponent(q)}`:''}`);
      root.innerHTML=items.length?items.map(candidate=>`<article class="catalog-candidate"><div class="catalog-candidate-poster">${candidate.poster_url?`<img src="${esc(candidate.poster_url)}" alt="">`:'<span>SEM CAPA</span>'}</div><div><span>${candidate.media_type==='movie'?'FILME':'SÉRIE'} · TMDB ${candidate.tmdb_id}</span><h3>${esc(candidate.title)}</h3><p>${candidate.year||'Ano desconhecido'}${candidate.original_title&&candidate.original_title!==candidate.title?` · ${esc(candidate.original_title)}`:''}</p><small>${esc(candidate.overview||'Sem sinopse disponível.')}</small><button class="primary" data-use-tmdb="${candidate.tmdb_id}" data-use-type="${candidate.media_type}">Usar este resultado</button></div></article>`).join(''):'<div class="catalog-admin-empty">Nenhum resultado. Tente pesquisar pelo título original ou ano.</div>';
      root.querySelectorAll('[data-use-tmdb]').forEach(b=>b.onclick=()=>applyMatch(Number(b.dataset.useTmdb),b.dataset.useType,b));
    }catch(err){root.innerHTML=`<p class="offline">${esc(err.message)}</p>`}
  }

  async function applyMatch(tmdbID,mediaType,button){
    if(!activeMedia)return;
    const seriesScope=activeMedia.entity_type==='series';
    if(seriesScope&&mediaType!=='tv'){notice('Para uma obra episódica escolha um resultado do tipo SÉRIE.');return}
    const confirmation=seriesScope?'Usar este resultado para a OBRA PRINCIPAL? O StormFlix salvará a série, reconstruirá a identidade episódica e atualizará os episódios atuais em segundo plano.':'Usar este resultado? Metadados, capas e legendas ligados à correspondência anterior serão limpos.';
    if(!confirm(confirmation))return;
    button.disabled=true;button.textContent='Aplicando…';
    try{
      const result=await req(`/admin/catalog/${activeMedia.id}/match`,{method:'POST',body:JSON.stringify({tmdb_id:tmdbID,media_type:mediaType,scope:seriesScope?'series':'item',apply_copies:!seriesScope&&($('#catalog-match-copies')?.checked!==false)})});
      if(seriesScope)notice(`Obra principal corrigida. ${result.updated||1} episódio(s) serão reorganizados/atualizados em segundo plano.`,true);
      else notice(`Correspondência corrigida em ${result.updated||1} arquivo(s).`,true);
      $('#catalog-match-overlay')?.classList.add('hidden');
      await fetchCatalog();
    }catch(err){notice(err.message);button.disabled=false;button.textContent='Usar este resultado'}
  }

  async function resetAuto(id,series){
    const label=series?'a obra principal':'este item';
    if(!confirm(`Remover a proteção manual e voltar ao modo automático para ${label}?`))return;
    try{const r=await req(`/admin/catalog/${id}/auto${series?'?scope=series':''}`,{method:'POST'});notice(series?`${r.updated||0} episódio(s) voltaram a seguir a identificação automática da obra.`:'Item voltou ao modo automático.',true);await fetchCatalog()}catch(err){notice(err.message)}
  }

  function filenameHint(path){
    const file=String(path||'').split(/[\\/]/).pop()||'';
    return file.replace(/\.[^.]+$/,'').replace(/[._]+/g,' ').replace(/\b(2160p|1080p|720p|480p|4k|uhd|bluray|web[- ]?dl|webrip|x264|x265|hevc|h264|h265)\b/ig,' ').replace(/\s+/g,' ').trim();
  }
  window.loadCatalog=loadCatalog;
  document.addEventListener('DOMContentLoaded',init);
})();
