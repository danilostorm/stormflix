/* StormFlix automatic movie collections.
 * TMDB membership is indexed server-side; this layer only presents collections
 * that already pass the authenticated library/profile filters.
 */
(function(){
  const minimumSize=2;
  let collections=[];
  let refreshTimer=null;
  const baseAuthenticated=authenticated;
  const baseShowHome=showHome;

  authenticated=async function(){
    await baseAuthenticated();
    await refreshCollectionsMenu();
    scheduleCollectionRefresh();
  };

  showHome=function(){
    baseShowHome();
    const button=document.querySelector('#collections-nav');
    if(button)button.classList.remove('active');
  };

  async function fetchCollections(){
    const data=await request(`/media?group=collections&minimum_size=${minimumSize}`);
    return Array.isArray(data)?data:[];
  }

  function displayCollectionName(value){
    const original=String(value||'').trim();
    const cleaned=original.replace(/^Coleção\s+/i,'').replace(/\s+Collection$/i,'').trim();
    return cleaned||original||'Coleção';
  }

  async function refreshCollectionsMenu(){
    try{collections=await fetchCollections()}catch{collections=[]}
    syncButton();
    return collections;
  }

  function syncButton(){
    const nav=document.querySelector('.main-nav');
    if(!nav)return;
    let button=document.querySelector('#collections-nav');
    if(!collections.length){
      if(button)button.remove();
      return;
    }
    if(!button){
      button=document.createElement('button');
      button.id='collections-nav';
      button.type='button';
      button.textContent='Coleções';
      const music=document.querySelector('#music-nav');
      if(music)nav.insertBefore(button,music);else nav.appendChild(button);
    }
    button.onclick=()=>openCollections(button);
  }

  function scheduleCollectionRefresh(){
    if(refreshTimer)clearTimeout(refreshTimer);
    let attempts=0;
    const tick=async()=>{
      attempts++;
      await refreshCollectionsMenu();
      // Existing catalogs are backfilled slowly in the background so playback
      // and remote scans remain more important than TMDB enrichment.
      if(attempts<30)refreshTimer=setTimeout(tick,10000);
    };
    refreshTimer=setTimeout(tick,10000);
  }

  async function openCollections(button){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    if(typeof stopTheme==='function')stopTheme();
    document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
    $('#search-view').classList.add('hidden');
    $('#music-view')?.classList.add('hidden');
    $('#category-explorer')?.classList.add('hidden');
    $('#catalog-view').classList.remove('hidden');
    $('#hero').classList.add('hidden');
    const root=$('#rows');
    root.innerHTML='<div class="empty-state">Carregando coleções…</div>';
    try{
      collections=await fetchCollections();
      syncButton();
      const rows=collections.map(collection=>({
        id:`tmdb-collection-${collection.tmdb_id}`,
        title:displayCollectionName(collection.name),
        items:collection.items||[]
      })).filter(row=>row.items.length>=minimumSize);
      if(rows.length)renderRows(rows);
      else root.innerHTML='<div class="empty-state">As coleções estão sendo identificadas automaticamente pelo TMDB.</div>';
      window.scrollTo({top:0,behavior:'smooth'});
    }catch(err){
      root.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`;
    }
  }

  window.sfMovieCollections={reload:refreshCollectionsMenu,list:()=>collections,open:openCollections};
})();
