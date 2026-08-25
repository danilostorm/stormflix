/* StormFlix UX v3: TV keyboard focus + stronger rails + episodic-only theme previews */
(function(){
  const modal=document.querySelector('#player-modal');

  const originalSetupTheme=setupTheme;
  setupTheme=function(d){
    const episodic=d && (d.media_type==='series'||d.media_type==='anime');
    if(!episodic){
      stopTheme();
      const button=document.querySelector('#theme-toggle');
      const wrap=document.querySelector('#theme-info-wrap');
      if(button)button.classList.add('hidden');
      if(wrap)wrap.classList.add('hidden');
      return;
    }
    originalSetupTheme(d);
  };

  function visible(el){
    if(!el||el.disabled||el.classList.contains('hidden'))return false;
    const style=getComputedStyle(el);
    if(style.display==='none'||style.visibility==='hidden')return false;
    const r=el.getBoundingClientRect();
    return r.width>1&&r.height>1;
  }

  function focusables(){
    const selectors=[
      '.main-nav button',
      '#hero-play','#hero-more','#search-toggle','#profile-btn','#admin-link','#logout',
      '.media-tile',
      '.detail-card button:not(.hidden)', '.detail-card a:not(.hidden)',
      '.row-nav:not(:disabled)',
      '#search-close','#search'
    ];
    return [...document.querySelectorAll(selectors.join(','))].filter(visible);
  }

  function firstMediaCard(){
    return [...document.querySelectorAll('.media-tile')].find(visible)||null;
  }

  function activateFirst(direction){
    let target=null;
    const detailOpen=!document.querySelector('#detail-modal')?.classList.contains('hidden');
    if(detailOpen)target=document.querySelector('#detail-play');
    if(!target&&direction==='ArrowUp')target=document.querySelector('.main-nav button.active');
    if(!target)target=firstMediaCard()||document.querySelector('#hero-play')||document.querySelector('.main-nav button.active');
    if(target&&visible(target)){
      target.focus({preventScroll:true});
      target.scrollIntoView({behavior:'smooth',block:'nearest',inline:'center'});
      return true;
    }
    return false;
  }

  function sameRailCard(current,dir){
    const track=current.closest('.row-track,.related-row');
    if(!track)return null;
    const cards=[...track.querySelectorAll('.media-tile')].filter(visible);
    const index=cards.indexOf(current);
    if(index<0)return null;
    return cards[index+(dir==='ArrowRight'?1:-1)]||null;
  }

  function adjacentRailCard(current,dir){
    const rows=[...document.querySelectorAll('.content-row,.detail-section')].filter(visible);
    const row=current.closest('.content-row,.detail-section');
    const rowIndex=rows.indexOf(row);
    if(rowIndex<0)return null;
    const step=dir==='ArrowDown'?1:-1;
    for(let i=rowIndex+step;i>=0&&i<rows.length;i+=step){
      const cards=[...rows[i].querySelectorAll('.media-tile')].filter(visible);
      if(!cards.length)continue;
      const cr=current.getBoundingClientRect();
      const cx=cr.left+cr.width/2;
      return cards.reduce((best,card)=>{
        const r=card.getBoundingClientRect();
        const dist=Math.abs((r.left+r.width/2)-cx);
        return !best||dist<best.dist?{card,dist}:best;
      },null)?.card||null;
    }
    return null;
  }

  function spatial(current,dir){
    if(current?.classList?.contains('media-tile')){
      if(dir==='ArrowLeft'||dir==='ArrowRight'){
        const target=sameRailCard(current,dir);
        if(target)return target;
      }
      if(dir==='ArrowUp'||dir==='ArrowDown'){
        const target=adjacentRailCard(current,dir);
        if(target)return target;
      }
    }

    const candidates=focusables().filter(el=>el!==current);
    if(!current||!visible(current))return candidates[0]||null;
    const a=current.getBoundingClientRect();
    const ax=a.left+a.width/2,ay=a.top+a.height/2;
    let best=null;
    for(const el of candidates){
      const b=el.getBoundingClientRect();
      const bx=b.left+b.width/2,by=b.top+b.height/2;
      const dx=bx-ax,dy=by-ay;
      if(dir==='ArrowRight'&&dx<=8)continue;
      if(dir==='ArrowLeft'&&dx>=-8)continue;
      if(dir==='ArrowDown'&&dy<=8)continue;
      if(dir==='ArrowUp'&&dy>=-8)continue;
      const primary=(dir==='ArrowLeft'||dir==='ArrowRight')?Math.abs(dx):Math.abs(dy);
      const cross=(dir==='ArrowLeft'||dir==='ArrowRight')?Math.abs(dy):Math.abs(dx);
      const score=primary+(cross*2.4);
      if(!best||score<best.score)best={el,score};
    }
    return best?.el||null;
  }

  function moveFocus(dir){
    const active=document.activeElement;
    if(!active||active===document.body||active===document.documentElement)return activateFirst(dir);
    const target=spatial(active,dir);
    if(!target)return false;
    target.focus({preventScroll:true});
    target.scrollIntoView({behavior:'smooth',block:'nearest',inline:'center'});
    return true;
  }

  document.addEventListener('keydown',e=>{
    if(!modal?.classList.contains('hidden'))return;
    if(['INPUT','TEXTAREA','SELECT'].includes(e.target?.tagName))return;
    if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;
    if(moveFocus(e.key)){
      document.body.classList.add('sf-keyboard-nav');
      e.preventDefault();
      e.stopImmediatePropagation();
    }
  },true);

  document.addEventListener('pointerdown',()=>document.body.classList.remove('sf-keyboard-nav'),{passive:true});

  function improveRailButtons(){
    document.querySelectorAll('.content-row').forEach(section=>{
      const track=section.querySelector('.row-track');
      const left=section.querySelector('.row-nav.left');
      const right=section.querySelector('.row-nav.right');
      if(!track||!left||!right)return;
      const sync=()=>{
        const overflow=track.scrollWidth-track.clientWidth>12;
        section.classList.toggle('has-overflow',overflow);
        left.disabled=!overflow||track.scrollLeft<8;
        right.disabled=!overflow||track.scrollLeft+track.clientWidth>=track.scrollWidth-8;
      };
      track.addEventListener('scroll',sync,{passive:true});
      setTimeout(sync,60);
    });
  }

  const observer=new MutationObserver(()=>setTimeout(improveRailButtons,20));
  const rows=document.querySelector('#rows');
  if(rows)observer.observe(rows,{childList:true,subtree:true});
  window.addEventListener('resize',improveRailButtons,{passive:true});
  setTimeout(improveRailButtons,120);
})();
