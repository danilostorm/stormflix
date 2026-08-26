/* StormFlix Rail Navigation v2: desktop arrows + end-of-row guarantees for video, music and detail rails. */
(function(){
  const managed=new WeakMap();
  const desktop=()=>window.matchMedia('(min-width:801px)').matches;

  function railEntries(root=document){
    const entries=[];
    root.querySelectorAll('.content-row').forEach(section=>{
      const track=section.querySelector(':scope > .row-track');
      if(track)entries.push({host:section,track,type:'video'});
    });
    root.querySelectorAll('.music-section').forEach(section=>{
      section.querySelectorAll(':scope > .music-album-rail,:scope > .music-artist-rail').forEach(track=>entries.push({host:section,track,type:'music'}));
    });
    root.querySelectorAll('.detail-section').forEach(section=>{
      section.querySelectorAll(':scope > .related-row').forEach(track=>entries.push({host:section,track,type:'detail'}));
    });
    return entries;
  }

  function makeButton(side){
    const button=document.createElement('button');
    button.type='button';
    button.className=`sf-rail-arrow ${side}`;
    button.setAttribute('aria-label',side==='left'?'Ver itens anteriores':'Ver mais itens');
    button.innerHTML=`<span aria-hidden="true">${side==='left'?'‹':'›'}</span>`;
    return button;
  }

  function ensure(entry){
    const {host,track,type}=entry;
    let state=managed.get(track);
    if(state){sync(state);return state}

    host.classList.add('sf-rail-host',`sf-rail-${type}`);
    track.classList.add('sf-scroll-rail');

    let left=host.querySelector(':scope > .sf-rail-arrow.left');
    let right=host.querySelector(':scope > .sf-rail-arrow.right');
    if(!left){left=makeButton('left');host.appendChild(left)}
    if(!right){right=makeButton('right');host.appendChild(right)}

    const move=dir=>{
      const amount=Math.max(360,Math.floor(track.clientWidth*.86));
      track.scrollBy({left:dir*amount,behavior:'smooth'});
      window.setTimeout(()=>sync(state),360);
    };
    left.addEventListener('click',e=>{e.preventDefault();e.stopPropagation();move(-1)});
    right.addEventListener('click',e=>{e.preventDefault();e.stopPropagation();move(1)});

    track.addEventListener('scroll',()=>sync(state),{passive:true});
    track.addEventListener('wheel',e=>{
      if(!desktop())return;
      if(Math.abs(e.deltaX)>Math.abs(e.deltaY))return;
      if(!e.shiftKey)return;
      e.preventDefault();
      track.scrollBy({left:e.deltaY,behavior:'auto'});
    },{passive:false});

    state={host,track,left,right};
    managed.set(track,state);
    requestAnimationFrame(()=>sync(state));
    setTimeout(()=>sync(state),100);
    setTimeout(()=>sync(state),600);
    return state;
  }

  function sync(state){
    if(!state||!state.track.isConnected)return;
    const {host,track,left,right}=state;
    const overflow=track.scrollWidth-track.clientWidth>8;
    const atStart=track.scrollLeft<=6;
    const atEnd=track.scrollLeft+track.clientWidth>=track.scrollWidth-8;
    host.classList.toggle('sf-has-overflow',overflow&&desktop());
    host.classList.toggle('sf-at-start',atStart);
    host.classList.toggle('sf-at-end',atEnd);
    left.disabled=!overflow||atStart;
    right.disabled=!overflow||atEnd;

    if(overflow&&desktop()){
      const hostRect=host.getBoundingClientRect();
      const railRect=track.getBoundingClientRect();
      const top=Math.max(0,railRect.top-hostRect.top);
      const height=Math.max(72,railRect.height);
      left.style.top=right.style.top=`${top}px`;
      left.style.height=right.style.height=`${height}px`;
    }
  }

  function scan(root=document){
    railEntries(root).forEach(ensure);
  }

  const observer=new MutationObserver(records=>{
    let needed=false;
    for(const record of records){
      if(record.addedNodes.length||record.removedNodes.length){needed=true;break}
    }
    if(needed)setTimeout(()=>scan(document),20);
  });
  observer.observe(document.body,{childList:true,subtree:true});
  window.addEventListener('resize',()=>{scan(document);managedForEachSync()},{passive:true});

  function managedForEachSync(){
    document.querySelectorAll('.sf-scroll-rail').forEach(track=>sync(managed.get(track)));
  }

  document.addEventListener('keydown',e=>{
    if(!desktop()||!['ArrowLeft','ArrowRight'].includes(e.key))return;
    const active=document.activeElement;
    const rail=active?.closest?.('.sf-scroll-rail');
    if(!rail)return;
    const state=managed.get(rail);if(!state)return;
    setTimeout(()=>{
      active?.scrollIntoView?.({behavior:'smooth',block:'nearest',inline:'center'});
      sync(state);
    },20);
  },true);

  scan(document);
  setTimeout(()=>scan(document),250);
  window.sfRefreshRails=()=>scan(document);
})();
