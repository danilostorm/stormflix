/* StormFlix library categories: hierarchical navigation over many libraries. */
(function(){
  let categories=[];
  const baseAuthenticated=authenticated;
  authenticated=async function(){await baseAuthenticated();await loadCategories()};

  function ensureSubnav(){
    let sub=document.querySelector('.category-subnav');
    if(sub)return sub;
    const nav=document.querySelector('.main-nav');if(!nav)return null;
    sub=document.createElement('div');sub.className='category-subnav hidden';
    nav.insertAdjacentElement('afterend',sub);
    if(!document.querySelector('#sf-category-subnav-style')){
      const style=document.createElement('style');style.id='sf-category-subnav-style';style.textContent='.category-subnav{display:flex;gap:8px;overflow-x:auto;padding:10px 3vw 6px;background:rgba(5,7,11,.96);position:sticky;top:64px;z-index:25;scrollbar-width:none}.category-subnav.hidden{display:none}.category-subnav button{white-space:nowrap;border:1px solid rgba(255,255,255,.14);background:rgba(255,255,255,.05);color:#ddd;border-radius:999px;padding:8px 14px;cursor:pointer}.category-subnav button.active{background:#fff;color:#111;border-color:#fff}.category-subnav .category-back{opacity:.72}';document.head.appendChild(style);
    }
    return sub;
  }

  async function loadCategories(){
    categories=await request('/categories').catch(()=>[]);
    const nav=document.querySelector('.main-nav');if(!nav)return;
    nav.querySelectorAll('[data-category-custom]').forEach(x=>x.remove());
    const roots=categories.filter(c=>!c.parent_id).sort(sortCategory);
    for(const category of roots){
      const systemButton=nav.querySelector(`[data-nav="${category.slug}"]`);
      if(systemButton){systemButton.textContent=category.name;bindRoot(systemButton,category);continue}
      if(category.system)continue;
      const button=document.createElement('button');
      button.dataset.categoryCustom='1';button.textContent=category.name;bindRoot(button,category);nav.appendChild(button);
    }
    const sub=ensureSubnav();if(sub){sub.innerHTML='';sub.classList.add('hidden')}
  }

  function sortCategory(a,b){return Number(a.sort_order||0)-Number(b.sort_order||0)||Number(a.id)-Number(b.id)}
  function childrenOf(id){return categories.filter(c=>Number(c.parent_id||0)===Number(id)).sort(sortCategory)}

  function bindRoot(button,category){
    button.onclick=()=>{
      document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
      renderSubnav(category,category,button);
      openCategory(category,button,false);
    };
  }

  function renderSubnav(root,current,rootButton){
    const sub=ensureSubnav();if(!sub)return;
    const children=childrenOf(current.id);
    const siblings=children.length?children:childrenOf(current.parent_id||root.id);
    if(!siblings.length&&Number(current.id)===Number(root.id)){sub.innerHTML='';sub.classList.add('hidden');return}
    sub.classList.remove('hidden');sub.innerHTML='';
    const all=document.createElement('button');all.textContent=Number(current.id)===Number(root.id)?`Todos em ${root.name}`:`← ${root.name}`;all.className='category-back';
    all.onclick=()=>{renderSubnav(root,root,rootButton);openCategory(root,rootButton,false)};sub.appendChild(all);
    const list=children.length?children:siblings;
    for(const child of list){
      const b=document.createElement('button');b.textContent=child.name;b.classList.toggle('active',Number(child.id)===Number(current.id));
      b.onclick=()=>{openCategory(child,rootButton,true);renderSubnav(root,child,rootButton)};sub.appendChild(b);
    }
  }

  async function openCategory(category,button,keepRoot){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    stopTheme();
    if(!keepRoot)document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
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
  if(home)home.onclick=()=>{document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===home));const sub=ensureSubnav();if(sub)sub.classList.add('hidden');showHome()};
  window.sfCategories={reload:loadCategories,list:()=>categories};
})();
