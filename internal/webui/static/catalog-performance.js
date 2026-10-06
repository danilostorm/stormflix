/* StormFlix large-catalog rendering: keep DOM/image work bounded per rail. */
(function(){
  if(typeof renderRows!=='function'||typeof cardHTML!=='function'||typeof bindCards!=='function')return;
  const baseCardHTML=cardHTML;
  const baseRequest=typeof request==='function'?request:null;
  const baseLoadHome=typeof loadHome==='function'?loadHome:null;
  const baseAllFeedItems=typeof allFeedItems==='function'?allFeedItems:null;
  const baseFindItem=typeof findItem==='function'?findItem:null;
  const baseShowHome=typeof showHome==='function'?showHome:null;
  const CHUNK=12;
  const SNAPSHOT_PREFIX='stormflix.home.snapshot.v4:';
  const SNAPSHOT_TTL=10*60*1000;
  let instantFeed=null;
  let homePending=null,homePendingKey="";
  const pageStarted=performance.now();
  let homeRequestStarted=0,homeResponseMS=0,homeMetricSent=false;

  function reportFirstContent(renderStarted){
    if(homeMetricSent)return;homeMetricSent=true;
    requestAnimationFrame(()=>{
      const firstContent=Math.max(0,performance.now()-pageStarted),render=Math.max(0,performance.now()-renderStarted);
      if(!baseRequest)return;
      baseRequest('/telemetry/home',{method:'POST',body:JSON.stringify({first_content_ms:firstContent,response_ms:homeResponseMS,render_ms:render})}).catch(()=>{});
    });
  }

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
      section.innerHTML=`<div class="row-head"><h2>${escapeHTML(row.title)}</h2><span>${row.items.length} títulos</span></div><div class="row-track"></div>`;
      root.appendChild(section);
      const track=section.querySelector('.row-track');
      let rendered=0;
      const append=()=>{
        if(rendered>=row.items.length)return;
        const next=row.items.slice(rendered,rendered+CHUNK);
        const start=rendered;
        track.insertAdjacentHTML('beforeend',next.map((item,index)=>cardHTML(item,urgentRow&&start+index<4)).join(''));
        rendered+=next.length;
        bindCards(track);
        // Fill wide screens too; lazy rows remain bounded to the visible rail.
        requestAnimationFrame(()=>{
          if(track.isConnected&&track.clientWidth>0&&track.scrollWidth<=track.clientWidth+1)append();
        });
      };
      track.addEventListener('focusin',event=>{
        const cards=[...track.children];
        if(cards.slice(-3).some(card=>card.contains(event.target)))append();
      });
      if('ResizeObserver'in window){
        const resize=new ResizeObserver(()=>{
          if(rendered>0&&track.clientWidth>0&&track.scrollWidth<=track.clientWidth+1)append();
        });
        resize.observe(track);observers.push(resize);
      }
      // Only visible rails create posters. Horizontal paging uses the actual
      // track edge, not a vertical sentinel that eagerly drains every row.
      track.style.minHeight='260px';
      const initialize=()=>{if(rendered===0)append()};
      if(urgentRow||!('IntersectionObserver'in window))initialize();
      else{
        const observer=new IntersectionObserver(entries=>{
          if(entries.some(entry=>entry.isIntersecting)){initialize();observer.disconnect()}
        },{rootMargin:'350px 0px'});
        observer.observe(section);observers.push(observer);
      }
      let paging=false;
      track.addEventListener('scroll',()=>{
        if(paging)return;paging=true;
        requestAnimationFrame(()=>{
          paging=false;
          if(track.isConnected&&track.scrollWidth-track.clientWidth-track.scrollLeft<400)append();
        });
      },{passive:true});

    }
    window.sfCatalogObservers?.forEach(observer=>observer.disconnect());
    window.sfCatalogObservers=observers;
  };

  function profileKey(){
    const profile=window.sfProfiles?.current?.();
    if(!profile?.id)return'';
    const libraries=me?.role==='user'?(Array.isArray(me?.library_ids)?me.library_ids.map(Number).sort((a,b)=>a-b).join(','):'none'):'all';
    const account=[Number(me?.id||0),String(me?.role||''),String(me?.updated_at||''),libraries].join('|');
    const restrictions=[Number(profile.id),String(profile.updated_at||''),profile.is_kids?'kids':'standard',Number(profile.content_rating_limit??18)].join('|');
    const device=JSON.stringify(window.sfDeviceCapabilitySnapshot?.()||{});
    return SNAPSHOT_PREFIX+encodeURIComponent(account+'|'+restrictions+'|'+device);
  }

  function readSnapshot(){
    const key=profileKey();if(!key)return null;
    try{
      const cached=JSON.parse(sessionStorage.getItem(key)||'null');
      if(!cached?.feed||!cached.at||Date.now()-Number(cached.at)>SNAPSHOT_TTL){if(cached)sessionStorage.removeItem(key);return null}
      return cached.feed;
    }catch{return null}
  }

  function storeSnapshot(value,key=profileKey()){
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
    const renderStarted=performance.now();
    instantFeed=value;
    document.title=value.server_name||document.title||'StormFlix';
    renderHero(value.hero);
    renderRows(value.rows||[]);
    window.dispatchEvent(new CustomEvent('stormflix:home-snapshot-painted'));
    reportFirstContent(renderStarted);
    return true;
  }

  if(baseRequest){
    request=async function(path,opt={}){
      const started=path==='/home'?performance.now():0;
      const scope=profileKey();
      const value=await baseRequest(path,opt);
      const method=String(opt.method||'GET').toUpperCase();
      if(path==='/home'&&method==='GET'&&scope===profileKey()){
        homeResponseMS=Math.max(0,performance.now()-started);
        instantFeed=value;
        storeSnapshot(value,scope);
        window.dispatchEvent(new CustomEvent('stormflix:home-fresh',{detail:{rows:value?.rows?.length||0}}));
      }
      return value;
    };
  }

  if(baseLoadHome){
    loadHome=async function(){
      await window.sfCatalogCapabilityQuery?.();
      const scope=profileKey();
      if(homePending&&homePendingKey===scope)return homePending;
      homeRequestStarted=performance.now();
      const cached=readSnapshot();
      if(cached)paintSnapshot(cached);
      const pending=(async()=>{
        try{
          const value=await request('/home');
          if(scope!==profileKey())return;
          feed=value;
          paintSnapshot(value);
          return value;
        }catch(err){
          // Permission/authentication failures must never resurrect cached rows.
          if(cached&&scope===profileKey()&&![401,403].includes(err?.status))return cached;
          throw err;
        }
      })();
      homePending=pending;homePendingKey=scope;
      try{return await pending}finally{if(homePending===pending){homePending=null;homePendingKey=''}}
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

  window.addEventListener('stormflix:profile',()=>{instantFeed=null;homePending=null;homePendingKey=''});
  document.querySelector('#logout')?.addEventListener('click',()=>{
    try{for(let i=sessionStorage.length-1;i>=0;i--){const key=sessionStorage.key(i);if(key?.startsWith(SNAPSHOT_PREFIX))sessionStorage.removeItem(key)}}catch{}
  },true);
})();
