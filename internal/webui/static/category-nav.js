/* StormFlix library categories: one category can contain many libraries. */
(function(){
  let categories=[];
  const baseAuthenticated=authenticated;
  authenticated=async function(){await baseAuthenticated();await loadCategories()};

  async function loadCategories(){
    categories=await request('/categories').catch(()=>[]);
    const nav=document.querySelector('.main-nav');if(!nav)return;
    nav.querySelectorAll('[data-category-custom]').forEach(x=>x.remove());
    for(const category of categories){
      const systemButton=nav.querySelector(`[data-nav="${category.slug}"]`);
      if(systemButton){systemButton.textContent=category.name;bind(systemButton,category);continue}
      if(category.system)continue;
      const button=document.createElement('button');
      button.dataset.categoryCustom='1';button.textContent=category.name;bind(button,category);nav.appendChild(button);
    }
  }

  function bind(button,category){
    button.onclick=()=>openCategory(category,button);
  }

  async function openCategory(category,button){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    stopTheme();
    document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
    $('#search-view').classList.add('hidden');$('#catalog-view').classList.remove('hidden');$('#hero').classList.add('hidden');
    const root=$('#rows');root.innerHTML='<div class="empty-state">Carregando categoria…</div>';
    try{
      const data=await request(`/categories/${encodeURIComponent(category.slug)}`);
      const series=(data.series||[]).map(seriesCard);
      const seen=new Set(series.map(x=>`s:${x.series_id}`));
      const mediaItems=(data.media||[]).filter(x=>{
        const key=x.entity_type==='series'?`s:${x.series_id}`:`m:${x.id}`;
        if(seen.has(key))return false;seen.add(key);return true;
      });
      const items=[...series,...mediaItems];
      renderRows([{id:`category-${category.slug}`,title:category.name,items}]);
      window.scrollTo({top:0,behavior:'smooth'});
    }catch(err){root.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`}
  }

  function seriesCard(s){return {id:s.representative_media_id,entity_type:'series',series_id:s.id,library_id:s.library_id,library_name:s.library_name,title:s.title,media_type:s.media_type,year:s.year,overview:s.overview,genres:s.genres,rating:s.rating,poster_url:s.poster_url,backdrop_url:s.backdrop_url,logo_url:s.logo_url,modified_unix:s.modified_unix,season_count:s.season_count,episode_count:s.episode_count}}

  const home=document.querySelector('[data-nav="home"]');
  if(home)home.onclick=()=>{document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===home));showHome()};
  window.sfCategories={reload:loadCategories,list:()=>categories};
})();
