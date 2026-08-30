/* StormFlix large-catalog rendering: keep DOM/image work bounded per rail. */
(function(){
  if(typeof renderRows!=='function'||typeof cardHTML!=='function'||typeof bindCards!=='function')return;
  const baseCardHTML=cardHTML;
  const CHUNK=28;

  cardHTML=function(item){
    let html=baseCardHTML(item);
    html=html.replace('loading="lazy"','loading="lazy" decoding="async" fetchpriority="low"');
    return html;
  };

  renderRows=function(rows){
    const root=$('#rows');
    root.innerHTML='';
    const observers=[];
    for(const row of rows||[]){
      if(!row.items?.length)continue;
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
        track.insertAdjacentHTML('beforeend',next.map(cardHTML).join(''));
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
})();
