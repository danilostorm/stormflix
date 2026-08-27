/* StormFlix hierarchical category navigation + visible Home explorer. */
(function(){
  let categories=[];
  let activeRoot=null;
  const baseAuthenticated=authenticated;
  const baseShowHome=showHome;

  authenticated=async function(){
    await baseAuthenticated();
    await loadCategories();
    showExplorer(true);
  };

  showHome=function(){
    baseShowHome();
    activeRoot=null;
    const sub=ensureSubnav();if(sub)sub.classList.add('hidden');
    showExplorer(true);
  };

  function ensureSubnav(){
    let sub=document.querySelector('.category-subnav');
    if(sub)return sub;
    const explorer=document.querySelector('#category-explorer');
    const topbar=document.querySelector('#topbar');
    if(!explorer&&!topbar)return null;
    sub=document.createElement('div');sub.className='category-subnav hidden';
    if(explorer)explorer.insertAdjacentElement('afterend',sub);else topbar.insertAdjacentElement('afterend',sub);
    return sub;
  }

  function showExplorer(show){
    const explorer=document.querySelector('#category-explorer');if(!explorer)return;
    explorer.classList.toggle('hidden',!show||!categories.some(c=>!c.parent_id&&childrenOf(c.id).length));
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
      button.dataset.categoryCustom='1';button.textContent=category.name;bindRoot(button,category);nav.insertBefore(button,document.querySelector('#music-nav'));
    }
    renderExplorer(roots);
    const sub=ensureSubnav();if(sub){sub.innerHTML='';sub.classList.add('hidden')}
  }

  function renderExplorer(roots){
    const explorer=document.querySelector('#category-explorer');if(!explorer)return;
    const useful=roots.filter(root=>childrenOf(root.id).length);
    if(!useful.length){explorer.innerHTML='';explorer.classList.add('hidden');return}
    explorer.innerHTML=`<div class="category-explorer-head"><div><h2>Explorar por categoria</h2><small>Conteúdo separado em categorias e subcategorias para não misturar todas as bibliotecas na mesma fileira.</small></div></div><div class="category-explorer-groups">${useful.map(root=>`<div class="category-explorer-group"><button data-explorer-root="${root.id}">${escapeHTML(root.name)} →</button><div class="category-explorer-chips">${childrenOf(root.id).map(child=>`<button data-explorer-child="${child.id}" data-root="${root.id}">${escapeHTML(child.name)}</button>`).join('')}</div></div>`).join('')}</div>`;
    explorer.querySelectorAll('[data-explorer-root]').forEach(button=>button.onclick=()=>{
      const root=byID(button.dataset.explorerRoot);if(root)openRoot(root,findRootButton(root));
    });
    explorer.querySelectorAll('[data-explorer-child]').forEach(button=>button.onclick=()=>{
      const child=byID(button.dataset.explorerChild),root=byID(button.dataset.root);if(child&&root)openCategory(child,findRootButton(root),root);
    });
  }

  function sortCategory(a,b){return Number(a.sort_order||0)-Number(b.sort_order||0)||Number(a.id)-Number(b.id)}
  function childrenOf(id){return categories.filter(c=>Number(c.parent_id||0)===Number(id)).sort(sortCategory)}
  function byID(id){return categories.find(c=>Number(c.id)===Number(id))}
  function findRootButton(root){return document.querySelector(`.main-nav [data-nav="${root.slug}"]`)||[...document.querySelectorAll('.main-nav [data-category-custom]')].find(x=>x.textContent===root.name)}

  function bindRoot(button,category){
    button.onclick=()=>openRoot(category,button);
  }

  async function openRoot(root,button){
    activeRoot=root;
    document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
    renderSubnav(root,null,button);
    const children=childrenOf(root.id);
    if(!children.length){await openCategory(root,button,root);return}
    prepareCatalogView();
    showExplorer(false);
    const target=$('#rows');target.innerHTML='<div class="empty-state">Carregando subcategorias…</div>';
    try{
      const results=await Promise.all(children.map(async child=>({child,data:await request(`/categories/${encodeURIComponent(child.slug)}`)})));
      const rows=results.map(({child,data})=>({id:`category-${child.slug}`,title:child.name,items:categoryItems(data)})).filter(row=>row.items.length);
      if(!rows.length){
        const fallback=await request(`/categories/${encodeURIComponent(root.slug)}`);
        renderRows([{id:`category-${root.slug}`,title:`Todos em ${root.name}`,items:categoryItems(fallback)}]);
      }else{
        renderRows(rows);
      }
      window.scrollTo({top:0,behavior:'smooth'});
    }catch(err){target.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`}
  }

  function renderSubnav(root,current,rootButton){
    const sub=ensureSubnav();if(!sub)return;
    const children=childrenOf(root.id);
    if(!children.length){sub.innerHTML='';sub.classList.add('hidden');return}
    sub.classList.remove('hidden');sub.innerHTML=`<span class="category-root-label">${escapeHTML(root.name)}</span>`;
    const all=document.createElement('button');all.textContent=`Visão por subcategorias`;all.className=!current?'active category-back':'category-back';
    all.onclick=()=>openRoot(root,rootButton);sub.appendChild(all);
    const aggregate=document.createElement('button');aggregate.textContent=`Tudo em ${root.name}`;
    aggregate.onclick=()=>openCategory(root,rootButton,root,true);sub.appendChild(aggregate);
    for(const child of children){
      const b=document.createElement('button');b.textContent=child.name;b.classList.toggle('active',Number(child.id)===Number(current?.id));
      b.onclick=()=>openCategory(child,rootButton,root);sub.appendChild(b);
    }
  }

  async function openCategory(category,button,root=category,aggregate=false){
    activeRoot=root;
    document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
    renderSubnav(root,aggregate?null:category,button);
    prepareCatalogView();
    showExplorer(false);
    const target=$('#rows');target.innerHTML='<div class="empty-state">Carregando categoria…</div>';
    try{
      const data=await request(`/categories/${encodeURIComponent(category.slug)}`);
      renderRows([{id:`category-${category.slug}`,title:aggregate?`Tudo em ${root.name}`:category.name,items:categoryItems(data)}]);
      window.scrollTo({top:0,behavior:'smooth'});
    }catch(err){target.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`}
  }

  function prepareCatalogView(){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    stopTheme();
    $('#search-view').classList.add('hidden');$('#catalog-view').classList.remove('hidden');$('#hero').classList.add('hidden');
  }

  function categoryItems(data){
    const series=(data.series||[]).map(seriesCard);
    const seen=new Set(series.map(x=>`s:${x.series_id}`));
    const mediaItems=(data.media||[]).filter(x=>{
      const key=x.entity_type==='series'?`s:${x.series_id}`:`m:${x.id}`;
      if(seen.has(key))return false;seen.add(key);return true;
    });
    return [...series,...mediaItems];
  }

  function seriesCard(s){return {id:s.representative_media_id,entity_type:'series',series_id:s.id,library_id:s.library_id,library_name:s.library_name,title:s.title,media_type:s.media_type,year:s.year,overview:s.overview,genres:s.genres,rating:s.rating,poster_url:s.poster_url,backdrop_url:s.backdrop_url,logo_url:s.logo_url,modified_unix:s.modified_unix,season_count:s.season_count,episode_count:s.episode_count}}

  const home=document.querySelector('[data-nav="home"]');
  if(home)home.onclick=()=>showHome();
  document.querySelector('#search-toggle')?.addEventListener('click',()=>showExplorer(false));
  document.querySelector('#search-close')?.addEventListener('click',()=>showExplorer(true));
  document.querySelector('#brand-home')?.addEventListener('click',()=>showExplorer(true));
  window.sfCategories={reload:loadCategories,list:()=>categories,openRoot};
})();
