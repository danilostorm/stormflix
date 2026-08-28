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
    prepareCatalogView();
    showExplorer(false);
    const target=$('#rows');target.innerHTML='<div class="empty-state">Carregando seções…</div>';
    try{
      const childPromise=children.length
        ?Promise.all(children.map(async child=>({child,data:await request(`/categories/${encodeURIComponent(child.slug)}`)})))
        :Promise.resolve([]);
      const [rootData,results]=await Promise.all([
        request(`/categories/${encodeURIComponent(root.slug)}`),
        childPromise
      ]);

      // Explicit/configured children have first claim on a title. When sibling
      // categories overlap, the first configured category wins so the same
      // movie/series never appears twice on the root page.
      const assigned=new Set();
      const childRows=[];
      for(const {child,data} of results){
        const items=claimUniqueItems(categoryItems(data),assigned);
        if(items.length)childRows.push({id:`category-${child.slug}`,title:child.name,items});
      }

      // Metadata classification receives only titles not already claimed by a
      // configured category and assigns every remaining title to exactly one
      // Brazilian-Portuguese genre. Missing/unknown metadata falls into Outros.
      const genreRows=categoryGenreRows(root,rootData,assigned);
      const rows=mergeSectionRows([...childRows,...genreRows]);
      renderRows(rows);
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
      const items=categoryItems(data);
      if(aggregate){
        renderRows([{id:`category-${category.slug}`,title:`Tudo em ${root.name}`,items}]);
      }else{
        const rows=categoryGenreRows(category,data,new Set());
        renderRows(rows.length?rows:[{id:`category-${category.slug}`,title:category.name,items}]);
      }
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

  function itemCategoryKey(item){
    if(item?.entity_type==='series'||Number(item?.series_id||0)>0)return `s:${Number(item.series_id||item.id||0)}`;
    return `m:${Number(item?.id||0)}`;
  }

  function claimUniqueItems(items,assigned){
    const out=[];
    for(const item of items){
      const key=itemCategoryKey(item);
      if(assigned.has(key))continue;
      assigned.add(key);out.push(item);
    }
    return out;
  }

  function mergeSectionRows(rows){
    const merged=[];
    const byTitle=new Map();
    for(const row of rows){
      if(!row?.items?.length)continue;
      const sectionKey=normalizeSectionKey(row.title);
      const existing=byTitle.get(sectionKey);
      if(!existing){
        const next={id:row.id,title:row.title,items:[...row.items]};
        byTitle.set(sectionKey,next);merged.push(next);continue;
      }
      const seen=new Set(existing.items.map(itemCategoryKey));
      for(const item of row.items){
        const key=itemCategoryKey(item);if(seen.has(key))continue;
        seen.add(key);existing.items.push(item);
      }
    }
    return merged;
  }

  function normalizeSectionKey(value){
    return String(value||'').normalize('NFD').replace(/[\u0300-\u036f]/g,'').toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'');
  }

  function canonicalGenre(value){
    const key=normalizeSectionKey(value);
    if(!key)return null;
    const map={
      'acao':['action','acao'],
      'aventura':['adventure','aventura'],
      'animacao':['animation','animacao'],
      'comedia':['comedy','comedia'],
      'crime':['crime'],
      'documentarios':['documentary','documentario','documentarios'],
      'drama':['drama'],
      'familia':['family','familia'],
      'fantasia':['fantasy','fantasia'],
      'historia':['history','historia'],
      'terror':['horror','terror'],
      'musica':['music','musica'],
      'misterio':['mystery','misterio'],
      'romance':['romance'],
      'ficcao-cientifica':['science-fiction','sci-fi','ficcao-cientifica'],
      'suspense':['thriller','suspense'],
      'guerra':['war','guerra'],
      'faroeste':['western','faroeste'],
      'acao-e-aventura':['action-adventure','acao-e-aventura'],
      'infantil':['kids','infantil'],
      'reality-show':['reality','reality-show'],
      'ficcao-cientifica-e-fantasia':['sci-fi-fantasy','science-fiction-fantasy','ficcao-cientifica-e-fantasia','ficcao-cientifica-fantasia'],
      'novelas':['soap','novela','novelas'],
      'programas-de-entrevista':['talk','talk-show','talk-shows','programa-de-entrevista','programas-de-entrevista'],
      'guerra-e-politica':['war-politics','guerra-politica','guerra-e-politica'],
      'filmes-para-tv':['tv-movie','television-movie','filme-para-tv','filmes-para-tv'],
      'noticias':['news','noticia','noticias'],
      'psicologico':['psychological','psicologico'],
      'cotidiano':['slice-of-life','cotidiano'],
      'esportes':['sports','sport','esporte','esportes'],
      'sobrenatural':['supernatural','sobrenatural'],
      'garotas-magicas':['mahou-shoujo','magical-girl','magical-girls','garotas-magicas'],
      'mecha':['mecha'],
      'artes-marciais':['martial-arts','artes-marciais']
    };
    const titles={
      'acao':'Ação','aventura':'Aventura','animacao':'Animação','comedia':'Comédia','crime':'Crime',
      'documentarios':'Documentários','drama':'Drama','familia':'Família','fantasia':'Fantasia','historia':'História',
      'terror':'Terror','musica':'Música','misterio':'Mistério','romance':'Romance','ficcao-cientifica':'Ficção científica',
      'suspense':'Suspense','guerra':'Guerra','faroeste':'Faroeste','acao-e-aventura':'Ação e aventura',
      'infantil':'Infantil','reality-show':'Reality show','ficcao-cientifica-e-fantasia':'Ficção científica e fantasia',
      'novelas':'Novelas','programas-de-entrevista':'Programas de entrevista','guerra-e-politica':'Guerra e política',
      'filmes-para-tv':'Filmes para TV','noticias':'Notícias','psicologico':'Psicológico','cotidiano':'Cotidiano',
      'esportes':'Esportes','sobrenatural':'Sobrenatural','garotas-magicas':'Garotas mágicas','mecha':'Mecha','artes-marciais':'Artes marciais'
    };
    for(const [name,aliases] of Object.entries(map)){
      if(aliases.includes(key))return {key:name,title:titles[name]};
    }
    return null;
  }

  function preferredGenre(item){
    const genres=Array.isArray(item?.genres)?item.genres:[];
    for(const value of genres){
      const genre=canonicalGenre(value);
      if(genre)return genre;
    }
    return {key:'outros',title:'Outros'};
  }

  function categoryGenreRows(root,data,assigned){
    const groups=new Map();
    for(const item of categoryItems(data)){
      const itemKey=itemCategoryKey(item);
      if(assigned.has(itemKey))continue;
      const genre=preferredGenre(item);
      let group=groups.get(genre.key);
      if(!group){group={title:genre.title,items:[]};groups.set(genre.key,group)}
      group.items.push(item);
      assigned.add(itemKey);
    }
    return [...groups.entries()]
      .filter(([,group])=>group.items.length>0)
      .sort((a,b)=>{
        if(a[0]==='outros')return 1;
        if(b[0]==='outros')return -1;
        return b[1].items.length-a[1].items.length||a[1].title.localeCompare(b[1].title,'pt-BR');
      })
      .map(([key,group])=>({id:`category-${root.slug}-genre-${key}`,title:group.title,items:group.items}));
  }

  function seriesCard(s){return {id:s.representative_media_id,entity_type:'series',series_id:s.id,library_id:s.library_id,library_name:s.library_name,title:s.title,media_type:s.media_type,year:s.year,overview:s.overview,genres:s.genres,rating:s.rating,poster_url:s.poster_url,backdrop_url:s.backdrop_url,logo_url:s.logo_url,modified_unix:s.modified_unix,season_count:s.season_count,episode_count:s.episode_count}}

  const home=document.querySelector('[data-nav="home"]');
  if(home)home.onclick=()=>showHome();
  document.querySelector('#search-toggle')?.addEventListener('click',()=>showExplorer(false));
  document.querySelector('#search-close')?.addEventListener('click',()=>showExplorer(true));
  document.querySelector('#brand-home')?.addEventListener('click',()=>showExplorer(true));
  window.sfCategories={reload:loadCategories,list:()=>categories,openRoot};
})();
