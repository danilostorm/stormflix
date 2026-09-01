/* StormFlix large-catalog rendering: keep DOM/image work bounded per rail. */
(function(){
  if(typeof renderRows!=='function'||typeof cardHTML!=='function'||typeof bindCards!=='function')return;
  const baseCardHTML=cardHTML;
  const baseRequest=typeof request==='function'?request:null;
  const baseLoadHome=typeof loadHome==='function'?loadHome:null;
  const baseAllFeedItems=typeof allFeedItems==='function'?allFeedItems:null;
  const baseFindItem=typeof findItem==='function'?findItem:null;
  const baseShowHome=typeof showHome==='function'?showHome:null;
  const CHUNK=28;
  const SNAPSHOT_PREFIX='stormflix.home.snapshot.v2:';
  const SNAPSHOT_TTL=12*60*60*1000;
  let instantFeed=null;

  cardHTML=function(item,urgent=false){
    let html=baseCardHTML(item);
    html=html.replace('loading="lazy"',urgent?'loading="eager" decoding="async" fetchpriority="high"':'loading="lazy" decoding="async" fetchpriority="low"');
    return html;
  };

  renderRows=function(rows){
    const root=$('#rows');
    root.innerHTML='';
    const observers=[];
    let rowOrdinal=0;
    for(const row of rows||[]){
      if(!row.items?.length)continue;
      const urgentRow=rowOrdinal++<2;
      const section=document.createElement('section');
      section.className='content-row';
      section.dataset.virtualRow=String(row.id||row.title||'row');
      section.innerHTML=`<div class="row-head"><h2>${escapeHTML(row.title)}</h2><span>${row.items.length} títulos</span></div><div class="row-track"></div><button type="button" class="catalog-load-more hidden" aria-label="Carregar mais títulos">Carregar mais</button><div class="catalog-load-sentinel" aria-hidden="true"></div>`;
      root.appendChild(section);
      const track=section.querySelector('.row-track');
      const button=section.querySelector('.catalog-load-more');
      const sentinel=section.querySelector('.catalog-load-sentinel');
      let rendered=0;
      const append=()=>{
        if(rendered>=row.items.length)return;
        const next=row.items.slice(rendered,rendered+CHUNK);
        const start=rendered;
        track.insertAdjacentHTML('beforeend',next.map((item,index)=>cardHTML(item,urgentRow&&start+index<12)).join(''));
        rendered+=next.length;
        bindCards(track);
        const complete=rendered>=row.items.length;
        button.classList.toggle('hidden',complete);
        sentinel.classList.toggle('hidden',complete);
      };
      button.onclick=append;
      append();
      if(rendered<row.items.length&&'IntersectionObserver'in window){
        const observer=new IntersectionObserver(entries=>{
          if(entries.some(entry=>entry.isIntersecting))append();
        },{rootMargin:'1200px 800px'});
        observer.observe(sentinel);
        observers.push(observer);
      }else if(rendered<row.items.length){
        button.classList.remove('hidden');
      }
    }
    window.sfCatalogObservers?.forEach(observer=>observer.disconnect());
    window.sfCatalogObservers=observers;
  };

  function profileKey(){
    const profile=window.sfProfiles?.current?.();
    if(!profile?.id)return'';
    const user=(document.querySelector('#user-label')?.textContent||'conta').trim().toLocaleLowerCase('pt-BR');
    return SNAPSHOT_PREFIX+encodeURIComponent(user)+'|'+Number(profile.id)+'|'+encodeURIComponent(String(profile.name||''));
  }

  function readSnapshot(){
    const key=profileKey();if(!key)return null;
    try{
      const cached=JSON.parse(sessionStorage.getItem(key)||'null');
      if(!cached?.feed||!cached.at||Date.now()-Number(cached.at)>SNAPSHOT_TTL){if(cached)sessionStorage.removeItem(key);return null}
      return cached.feed;
    }catch{return null}
  }

  function storeSnapshot(value){
    const key=profileKey();
    if(!key||!value||!Array.isArray(value.rows))return;
    try{sessionStorage.setItem(key,JSON.stringify({at:Date.now(),feed:value}))}catch{}
  }

  function snapshotItems(value){
    const map=new Map();
    if(value?.hero?.id)map.set(Number(value.hero.id),value.hero);
    for(const row of value?.rows||[])for(const item of row.items||[])if(item?.id)map.set(Number(item.id),item);
    return [...map.values()];
  }

  function paintSnapshot(value){
    if(!value)return false;
    instantFeed=value;
    document.title=value.server_name||document.title||'StormFlix';
    renderHero(value.hero);
    renderRows(value.rows||[]);
    window.dispatchEvent(new CustomEvent('stormflix:home-snapshot-painted'));
    return true;
  }

  if(baseRequest){
    request=async function(path,opt={}){
      const value=await baseRequest(path,opt);
      const method=String(opt.method||'GET').toUpperCase();
      if(path==='/home'&&method==='GET'){
        instantFeed=value;
        storeSnapshot(value);
        window.dispatchEvent(new CustomEvent('stormflix:home-fresh',{detail:{rows:value?.rows?.length||0}}));
      }
      return value;
    };
  }

  if(baseLoadHome){
    loadHome=async function(){
      const cached=readSnapshot();
      if(cached)paintSnapshot(cached);
      try{return await baseLoadHome()}
      catch(err){if(cached)return cached;throw err}
    };
  }

  if(baseAllFeedItems){
    allFeedItems=function(){
      const items=baseAllFeedItems();
      return items?.length?items:snapshotItems(instantFeed);
    };
  }

  if(baseFindItem){
    findItem=function(id){
      return baseFindItem(id)||snapshotItems(instantFeed).find(item=>Number(item.id)===Number(id));
    };
  }

  if(baseShowHome){
    showHome=function(){
      baseShowHome();
      if(baseAllFeedItems&&baseAllFeedItems().length===0){
        const cached=instantFeed||readSnapshot();
        if(cached)paintSnapshot(cached);
      }
    };
  }

  window.addEventListener('stormflix:profile',()=>{instantFeed=null});
  document.querySelector('#logout')?.addEventListener('click',()=>{
    try{for(let i=sessionStorage.length-1;i>=0;i--){const key=sessionStorage.key(i);if(key?.startsWith(SNAPSHOT_PREFIX))sessionStorage.removeItem(key)}}catch{}
  },true);
})();
