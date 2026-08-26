/* StormFlix Admin Catalog: inspect and manually correct provider matches. */
(function(){
  let catalogItems=[];
  let activeMedia=null;
  let searchTimer=null;

  function init(){
    const nav=document.querySelector('nav [data-page="catalog"]');
    if(nav)nav.addEventListener('click',()=>setTimeout(loadCatalog,0));
  }

  async function loadCatalog(){
    const root=document.querySelector('#catalog');if(!root)return;
    root.innerHTML='<div class="catalog-admin-loading">Carregando catálogo…</div>';
    try{
      if(!Array.isArray(libs)||!libs.length)libs=await req('/admin/storage');
      root.innerHTML=`<div class="catalog-admin-head"><div><p class="kicker">Controle de correspondência</p><h2>Catálogo</h2><span>Corrija títulos, capas e metadados associados ao resultado errado.</span></div><div class="catalog-admin-tools"><input id="catalog-admin-search" placeholder="Buscar título, arquivo ou TMDB ID"><select id="catalog-admin-library"><option value="">Todas as bibliotecas</option>${libs.map(l=>`<option value="${l.id}">${esc(l.name)}</option>`).join('')}</select><button id="catalog-admin-refresh">Atualizar</button></div></div><div id="catalog-admin-results"></div>`;
      $('#catalog-admin-search').oninput=()=>{clearTimeout(searchTimer);searchTimer=setTimeout(fetchCatalog,300)};
      $('#catalog-admin-library').onchange=fetchCatalog;
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
      catalogItems=await req(`/admin/catalog?limit=160${q?`&q=${encodeURIComponent(q)}`:''}${lib?`&library_id=${encodeURIComponent(lib)}`:''}`);
      renderCatalog(target,catalogItems);
    }catch(err){target.innerHTML=`<p class="offline">${esc(err.message)}</p>`}
  }

  function renderCatalog(root,items){
    if(!items.length){root.innerHTML='<div class="catalog-admin-empty">Nenhum item encontrado.</div>';return}
    root.innerHTML=`<div class="catalog-admin-grid">${items.map(item=>{
      const poster=item.poster_url?`<img src="${esc(item.poster_url)}" alt="" loading="lazy">`:'<div class="catalog-admin-poster-fallback">SF</div>';
      const status=item.manual_match?'<span class="catalog-match-badge manual">MANUAL · PROTEGIDO</span>':item.metadata_status==='matched'?'<span class="catalog-match-badge ok">AUTOMÁTICO</span>':`<span class="catalog-match-badge error">${esc(item.metadata_status||'PENDENTE').toUpperCase()}</span>`;
      return `<article class="catalog-admin-card"><div class="catalog-admin-poster">${poster}</div><div class="catalog-admin-info"><div class="catalog-admin-title"><div><h3>${esc(item.title)}</h3><p>${esc(item.library_name)} · ${esc(item.media_type||'sem tipo')} ${item.year?`· ${item.year}`:''}</p></div>${status}</div><div class="catalog-admin-meta"><span>TMDB <b>${item.tmdb_id||'—'}</b></span><span>Classificação <b>${esc(item.content_rating||'—')}</b></span><span>Lançamento <b>${esc(item.release_date||'—')}</b></span></div><code title="${esc(item.path)}">${esc(item.path)}</code>${item.last_error?`<p class="catalog-admin-error">${esc(item.last_error)}</p>`:''}<div class="catalog-admin-actions"><button class="primary" data-fix-match="${item.id}">Corrigir correspondência</button>${item.manual_match?`<button data-auto-match="${item.id}">Voltar ao automático</button>`:''}</div></div></article>`}).join('')}</div>`;
    root.querySelectorAll('[data-fix-match]').forEach(b=>b.onclick=()=>openMatch(Number(b.dataset.fixMatch)));
    root.querySelectorAll('[data-auto-match]').forEach(b=>b.onclick=()=>resetAuto(Number(b.dataset.autoMatch)));
  }

  async function openMatch(id){
    activeMedia=catalogItems.find(x=>Number(x.id)===Number(id));if(!activeMedia)return;
    let overlay=$('#catalog-match-overlay');
    if(!overlay){overlay=document.createElement('div');overlay.id='catalog-match-overlay';overlay.className='catalog-match-overlay';document.body.appendChild(overlay)}
    overlay.innerHTML=`<div class="catalog-match-dialog"><header><div><p class="kicker">CORRESPONDÊNCIA MANUAL</p><h2>${esc(activeMedia.title)}</h2><span>${esc(activeMedia.path)}</span></div><button id="catalog-match-close">✕</button></header><div class="catalog-match-search"><input id="catalog-match-query" value="${esc(filenameHint(activeMedia.path))}" placeholder="Pesquisar no TMDB"><button id="catalog-match-search-button" class="primary">Pesquisar</button></div><label class="catalog-copy-option"><input type="checkbox" id="catalog-match-copies" checked><span>Aplicar também às cópias deste mesmo título nos outros servidores/origens desta biblioteca</span></label><div id="catalog-match-results" class="catalog-match-results"></div></div>`;
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
    if(!confirm('Usar este resultado? Metadados, capas e legendas ligados à correspondência anterior serão limpos.'))return;
    button.disabled=true;button.textContent='Aplicando…';
    try{
      const result=await req(`/admin/catalog/${activeMedia.id}/match`,{method:'POST',body:JSON.stringify({tmdb_id:tmdbID,media_type:mediaType,apply_copies:$('#catalog-match-copies')?.checked!==false})});
      notice(`Correspondência corrigida em ${result.updated||1} arquivo(s).`,true);
      $('#catalog-match-overlay')?.classList.add('hidden');
      await fetchCatalog();
    }catch(err){notice(err.message);button.disabled=false;button.textContent='Usar este resultado'}
  }

  async function resetAuto(id){
    if(!confirm('Remover a proteção manual e voltar a procurar metadados automaticamente para este item?'))return;
    try{await req(`/admin/catalog/${id}/auto`,{method:'POST'});notice('Item voltou ao modo automático.',true);await fetchCatalog()}catch(err){notice(err.message)}
  }

  function filenameHint(path){
    const file=String(path||'').split(/[\\/]/).pop()||'';
    return file.replace(/\.[^.]+$/,'').replace(/[._]+/g,' ').replace(/\b(2160p|1080p|720p|480p|4k|uhd|bluray|web[- ]?dl|webrip|x264|x265|hevc|h264|h265)\b/ig,' ').replace(/\s+/g,' ').trim();
  }
  window.loadCatalog=loadCatalog;
  document.addEventListener('DOMContentLoaded',init);
})();
