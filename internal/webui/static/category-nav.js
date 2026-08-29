/* StormFlix Home menus + gallery sections.
 * Root categories are the only category nodes shown in the main navigation.
 * Direct children are presentation sections/rails inside that Home menu and
 * are intentionally never rendered as navigation buttons.
 */
(function(){
  let categories=[];
  let activeRoot=null;
  const baseAuthenticated=authenticated;
  const baseShowHome=showHome;

  authenticated=async function(){
    await baseAuthenticated();
    await loadCategories();
  };

  showHome=function(){
    baseShowHome();
    activeRoot=null;
    hideLegacyCategoryChrome();
  };

  function hideLegacyCategoryChrome(){
    const explorer=document.querySelector('#category-explorer');
    if(explorer){explorer.innerHTML='';explorer.classList.add('hidden')}
    const sub=document.querySelector('.category-subnav');
    if(sub){sub.innerHTML='';sub.classList.add('hidden')}
  }

  async function loadCategories(){
    categories=await request('/categories').catch(()=>[]);
    const nav=document.querySelector('.main-nav');if(!nav)return;
    nav.querySelectorAll('[data-category-custom]').forEach(x=>x.remove());
    const roots=categories.filter(c=>!c.parent_id).sort(sortCategory);
    for(const root of roots){
      const systemButton=nav.querySelector(`[data-nav="${root.slug}"]`);
      if(systemButton){
        systemButton.textContent=root.name;
        bindRoot(systemButton,root);
        continue;
      }
      if(root.system)continue;
      const button=document.createElement('button');
      button.type='button';
      button.dataset.categoryCustom='1';
      button.dataset.homeMenu=String(root.id);
      button.textContent=root.name;
      bindRoot(button,root);
      const music=document.querySelector('#music-nav');
      if(music)nav.insertBefore(button,music);else nav.appendChild(button);
    }
    hideLegacyCategoryChrome();
  }

  function bindRoot(button,root){
    button.dataset.homeMenu=String(root.id);
    button.onclick=()=>openRoot(root,button);
  }

  function sortCategory(a,b){
    return Number(a.sort_order||0)-Number(b.sort_order||0)||Number(a.id)-Number(b.id);
  }

  function childrenOf(id){
    return categories.filter(c=>Number(c.parent_id||0)===Number(id)).sort(sortCategory);
  }

  function findRootButton(root){
    return document.querySelector(`.main-nav [data-home-menu="${root.id}"]`)
      ||document.querySelector(`.main-nav [data-nav="${root.slug}"]`)
      ||[...document.querySelectorAll('.main-nav [data-category-custom]')].find(x=>x.textContent===root.name);
  }

  async function openRoot(root,button=findRootButton(root)){
    activeRoot=root;
    hideLegacyCategoryChrome();
    document.querySelectorAll('.main-nav button').forEach(x=>x.classList.toggle('active',x===button));
    prepareCatalogView();
    const target=$('#rows');
    target.innerHTML='<div class="empty-state">Carregando seções…</div>';
    try{
      const sections=childrenOf(root.id);
      if(sections.length){
        // A configured section is authoritative for presentation. When a Home
        // menu has children, the page contains only those child sections in the
        // configured order. The children never become top/sub navigation.
        const results=await Promise.all(sections.map(async section=>({section,data:await request(`/categories/${encodeURIComponent(section.slug)}`)})));
        const claimed=new Set();
        const rows=[];
        for(const {section,data} of results){
          const items=claimUniqueItems(categoryItems(data),claimed);
          if(items.length)rows.push({id:`home-menu-${root.slug}-section-${section.slug}`,title:section.name,items});
        }
        if(rows.length)renderRows(rows);
        else target.innerHTML='<div class="empty-state">Este menu ainda não possui títulos nas seções configuradas.</div>';
      }else{
        // Compatibility fallback for existing installations: a root without
        // explicit sections continues to be organized automatically by primary
        // metadata genre. Once the admin creates a section, manual sections take
        // over presentation for that root.
        const data=await request(`/categories/${encodeURIComponent(root.slug)}`);
        const rows=categoryGenreRows(root,data,new Set());
        const items=categoryItems(data);
        renderRows(rows.length?rows:[{id:`home-menu-${root.slug}`,title:root.name,items}]);
      }
      window.scrollTo({top:0,behavior:'smooth'});
    }catch(err){
      target.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`;
    }
  }

  function prepareCatalogView(){
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    stopTheme();
    $('#search-view').classList.add('hidden');
    $('#catalog-view').classList.remove('hidden');
    $('#hero').classList.add('hidden');
  }

  function categoryItems(data){
    const series=(data.series||[]).map(seriesCard);
    const seen=new Set(series.map(x=>`s:${x.series_id}`));
    const mediaItems=(data.media||[]).filter(x=>{
      const key=x.entity_type==='series'?`s:${x.series_id}`:`m:${x.id}`;
      if(seen.has(key))return false;
      seen.add(key);
      return true;
    });
    return [...series,...mediaItems];
  }

  function itemCategoryKey(item){
    if(item?.entity_type==='series'||Number(item?.series_id||0)>0)return `s:${Number(item.series_id||item.id||0)}`;
    return `m:${Number(item?.id||0)}`;
  }

  function claimUniqueItems(items,claimed){
    const out=[];
    for(const item of items){
      const key=itemCategoryKey(item);
      if(claimed.has(key))continue;
      claimed.add(key);
      out.push(item);
    }
    return out;
  }

  function normalizeSectionKey(value){
    return String(value||'').normalize('NFD').replace(/[\u0300-\u036f]/g,'').toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'');
  }

  function canonicalGenre(value){
    const key=normalizeSectionKey(value);if(!key)return null;
    const aliases={
      'acao':['action','acao'],'aventura':['adventure','aventura'],'animacao':['animation','animacao'],
      'comedia':['comedy','comedia'],'crime':['crime'],'documentarios':['documentary','documentario','documentarios'],
      'drama':['drama'],'familia':['family','familia'],'fantasia':['fantasy','fantasia'],'historia':['history','historia'],
      'terror':['horror','terror'],'musica':['music','musica'],'misterio':['mystery','misterio'],'romance':['romance'],
      'ficcao-cientifica':['science-fiction','sci-fi','ficcao-cientifica'],'suspense':['thriller','suspense'],
      'guerra':['war','guerra'],'faroeste':['western','faroeste'],'acao-e-aventura':['action-adventure','acao-e-aventura'],
      'infantil':['kids','infantil'],'reality-show':['reality','reality-show'],
      'ficcao-cientifica-e-fantasia':['sci-fi-fantasy','science-fiction-fantasy','ficcao-cientifica-e-fantasia','ficcao-cientifica-fantasia'],
      'novelas':['soap','novela','novelas'],'programas-de-entrevista':['talk','talk-show','talk-shows','programa-de-entrevista','programas-de-entrevista'],
      'guerra-e-politica':['war-politics','guerra-politica','guerra-e-politica'],'filmes-para-tv':['tv-movie','television-movie','filme-para-tv','filmes-para-tv'],
      'noticias':['news','noticia','noticias'],'psicologico':['psychological','psicologico'],'cotidiano':['slice-of-life','cotidiano'],
      'esportes':['sports','sport','esporte','esportes'],'sobrenatural':['supernatural','sobrenatural'],
      'garotas-magicas':['mahou-shoujo','magical-girl','magical-girls','garotas-magicas'],'mecha':['mecha'],'artes-marciais':['martial-arts','artes-marciais']
    };
    const titles={
      'acao':'Ação','aventura':'Aventura','animacao':'Animação','comedia':'Comédia','crime':'Crime','documentarios':'Documentários',
      'drama':'Drama','familia':'Família','fantasia':'Fantasia','historia':'História','terror':'Terror','musica':'Música','misterio':'Mistério',
      'romance':'Romance','ficcao-cientifica':'Ficção científica','suspense':'Suspense','guerra':'Guerra','faroeste':'Faroeste',
      'acao-e-aventura':'Ação e aventura','infantil':'Infantil','reality-show':'Reality show','ficcao-cientifica-e-fantasia':'Ficção científica e fantasia',
      'novelas':'Novelas','programas-de-entrevista':'Programas de entrevista','guerra-e-politica':'Guerra e política','filmes-para-tv':'Filmes para TV',
      'noticias':'Notícias','psicologico':'Psicológico','cotidiano':'Cotidiano','esportes':'Esportes','sobrenatural':'Sobrenatural',
      'garotas-magicas':'Garotas mágicas','mecha':'Mecha','artes-marciais':'Artes marciais'
    };
    for(const [name,list] of Object.entries(aliases))if(list.includes(key))return {key:name,title:titles[name]};
    return null;
  }

  function preferredGenre(item){
    const genres=Array.isArray(item?.genres)?item.genres:[];
    for(const value of genres){const genre=canonicalGenre(value);if(genre)return genre}
    return {key:'outros',title:'Outros'};
  }

  function categoryGenreRows(root,data,claimed){
    const groups=new Map();
    for(const item of categoryItems(data)){
      const itemKey=itemCategoryKey(item);if(claimed.has(itemKey))continue;
      const genre=preferredGenre(item);
      if(!groups.has(genre.key))groups.set(genre.key,{title:genre.title,items:[]});
      groups.get(genre.key).items.push(item);claimed.add(itemKey);
    }
    return [...groups.entries()].filter(([,group])=>group.items.length).sort((a,b)=>{
      if(a[0]==='outros')return 1;if(b[0]==='outros')return -1;
      return b[1].items.length-a[1].items.length||a[1].title.localeCompare(b[1].title,'pt-BR');
    }).map(([key,group])=>({id:`home-menu-${root.slug}-genre-${key}`,title:group.title,items:group.items}));
  }

  function seriesCard(s){
    return {id:s.representative_media_id,entity_type:'series',series_id:s.id,library_id:s.library_id,library_name:s.library_name,title:s.title,media_type:s.media_type,year:s.year,overview:s.overview,genres:s.genres,rating:s.rating,poster_url:s.poster_url,backdrop_url:s.backdrop_url,logo_url:s.logo_url,modified_unix:s.modified_unix,season_count:s.season_count,episode_count:s.episode_count};
  }

  const home=document.querySelector('[data-nav="home"]');
  if(home)home.onclick=()=>showHome();
  window.sfCategories={reload:loadCategories,list:()=>categories,openRoot};
})();
