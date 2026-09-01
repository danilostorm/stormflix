/* StormFlix Games G3.2 input layer.
 * Virtual buttons call Nostalgist pressDown/pressUp directly. The directional
 * control uses a circular touch stick inspired by EmulatorJS' nipplejs zone:
 * the thumb follows the finger while SNES/NES-era cores still receive a
 * digital 8-way RetroPad D-pad.
 */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  const activePointers=new Map();
  const STICK_DEADZONE=.20;
  let pad=null;
  let stickState={pointerId:null,inputs:[],node:null};

  const layouts={
    nes:{name:'NES',face:[['b','B'],['a','A']]},
    gb:{name:'Game Boy',face:[['b','B'],['a','A']]},
    gbc:{name:'Game Boy Color',face:[['b','B'],['a','A']]},
    gba:{name:'Game Boy Advance',shoulders:[['l','L'],['r','R']],face:[['b','B'],['a','A']]},
    snes:{name:'Super Nintendo',shoulders:[['l','L'],['r','R']],face:[['y','Y'],['x','X'],['b','B'],['a','A']]},
    // Genesis Plus GX follows the libretro RetroPad layout. The six-button
    // presentation maps the console labels onto the RetroPad inputs.
    genesis:{name:'Mega Drive / Genesis',face:[['y','A'],['b','B'],['a','C'],['x','X'],['l','Y'],['r','Z']],genesis:true},
  };

  function player(){return window.StormFlixGamePlayer}
  function current(){return player()?.current?.()||null}
  function coarse(){return !!window.matchMedia?.('(pointer: coarse)').matches||innerWidth<=900}
  function enabled(){const mode=localStorage.getItem('stormflix.games.virtual-controls')||'auto';return mode==='on'||(mode==='auto'&&coarse())}
  function gameplayVisible(overlay){return !!overlay&&!$('[data-game-controls]',overlay)?.classList.contains('hidden')}

  function press(input,down){
    const api=player();if(!api)return false;
    return down?api.pressDown?.(input)!==false:api.pressUp?.(input)!==false;
  }

  function releasePointer(id){
    const inputs=activePointers.get(id);if(!inputs)return;
    activePointers.delete(id);for(const input of inputs)press(input,false);
    pad?.querySelectorAll(`[data-pointer-id="${id}"]`).forEach(node=>{node.classList.remove('pressed');node.removeAttribute('data-pointer-id')});
  }
  function setStickInputs(next){
    const previous=stickState.inputs;
    if(previous.join(',')===next.join(','))return;
    for(const input of previous)if(!next.includes(input))press(input,false);
    for(const input of next)if(!previous.includes(input))press(input,true);
    stickState.inputs=next;
  }
  function releaseStick(id){
    if(stickState.pointerId===null)return;
    if(id!==undefined&&id!==null&&id!==stickState.pointerId)return;
    setStickInputs([]);
    const node=stickState.node;
    if(node){
      const thumb=$('[data-sf-stick-thumb]',node);if(thumb)thumb.style.transform='translate(0px,0px)';
      node.classList.remove('active');node.removeAttribute('data-pointer-id');
      try{if(node.hasPointerCapture?.(stickState.pointerId))node.releasePointerCapture(stickState.pointerId)}catch{}
    }
    stickState={pointerId:null,inputs:[],node:null};
  }
  function releaseAll(){for(const id of [...activePointers.keys()])releasePointer(id);releaseStick()}
  function inputsFor(button){return String(button?.dataset.inputs||'').split(',').map(x=>x.trim()).filter(Boolean)}
  function buttonFromPoint(x,y){return document.elementFromPoint(x,y)?.closest?.('[data-sf-inputs]')||null}
  function setPointerButton(pointerId,button){
    const next=inputsFor(button),previous=activePointers.get(pointerId)||[];if(previous.join(',')===next.join(','))return;
    releasePointer(pointerId);if(!next.length)return;
    activePointers.set(pointerId,next);button.classList.add('pressed');button.dataset.pointerId=String(pointerId);for(const input of next)press(input,true);
  }

  function bindButton(button){
    button.addEventListener('pointerdown',event=>{event.preventDefault();event.stopPropagation();if(navigator.vibrate&&localStorage.getItem('stormflix.games.haptics')!=='off')navigator.vibrate(8);setPointerButton(event.pointerId,button)},{passive:false});
    button.addEventListener('pointermove',event=>{if(!activePointers.has(event.pointerId))return;event.preventDefault();setPointerButton(event.pointerId,buttonFromPoint(event.clientX,event.clientY))},{passive:false});
    button.addEventListener('contextmenu',event=>event.preventDefault());
  }

  function stickInputs(angle){
    const tau=Math.PI*2,sector=Math.round((((angle%tau)+tau)%tau)/(Math.PI/4))%8;
    return [
      ['right'],['down','right'],['down'],['down','left'],
      ['left'],['up','left'],['up'],['up','right'],
    ][sector];
  }
  function updateStick(node,event){
    const rect=node.getBoundingClientRect();
    const radius=Math.max(1,Math.min(rect.width,rect.height)/2);
    const dx=event.clientX-(rect.left+rect.width/2),dy=event.clientY-(rect.top+rect.height/2);
    const distance=Math.hypot(dx,dy),strength=Math.min(1,distance/radius);
    const travel=Math.max(1,radius*.48),scale=distance>travel?travel/distance:1;
    const thumb=$('[data-sf-stick-thumb]',node);if(thumb)thumb.style.transform=`translate(${(dx*scale).toFixed(1)}px,${(dy*scale).toFixed(1)}px)`;
    setStickInputs(strength<STICK_DEADZONE?[]:stickInputs(Math.atan2(dy,dx)));
  }
  function bindStick(node){
    node.addEventListener('pointerdown',event=>{
      if(stickState.pointerId!==null&&stickState.pointerId!==event.pointerId)return;
      event.preventDefault();event.stopPropagation();stickState={pointerId:event.pointerId,inputs:[],node};
      node.classList.add('active');node.dataset.pointerId=String(event.pointerId);try{node.setPointerCapture?.(event.pointerId)}catch{}
      if(navigator.vibrate&&localStorage.getItem('stormflix.games.haptics')!=='off')navigator.vibrate(8);updateStick(node,event);
    },{passive:false});
    node.addEventListener('pointermove',event=>{if(stickState.pointerId!==event.pointerId)return;event.preventDefault();event.stopPropagation();updateStick(node,event)},{passive:false});
    node.addEventListener('pointerup',event=>releaseStick(event.pointerId));node.addEventListener('pointercancel',event=>releaseStick(event.pointerId));
    node.addEventListener('contextmenu',event=>event.preventDefault());
  }

  function touchStick(){
    return `<div class="sf-pad-stick" data-sf-stick role="application" aria-label="Direcional touch">
      <span class="sf-pad-stick-orbit" aria-hidden="true"></span><span class="sf-pad-stick-thumb" data-sf-stick-thumb aria-hidden="true"></span><span class="sf-sr-only">Arraste a bolinha para mover em oito direções</span>
    </div>`;
  }
  function shoulders(layout){return layout.shoulders?.length?`<div class="sf-pad-shoulders">${layout.shoulders.map(([input,label])=>`<button data-sf-inputs data-inputs="${input}">${label}</button>`).join('')}</div>`:''}
  function faces(layout){
    if(layout.genesis){const top=layout.face.slice(3),bottom=layout.face.slice(0,3);return `<div class="sf-pad-face genesis"><div>${top.map(([input,label])=>`<button data-sf-inputs data-inputs="${input}">${label}</button>`).join('')}</div><div>${bottom.map(([input,label])=>`<button data-sf-inputs data-inputs="${input}">${label}</button>`).join('')}</div></div>`}
    if(layout.face.length===4){const map=Object.fromEntries(layout.face.map(([i,l])=>[i,l]));return `<div class="sf-pad-face diamond"><button data-sf-inputs data-inputs="x" class="x">${map.x||'X'}</button><button data-sf-inputs data-inputs="y" class="y">${map.y||'Y'}</button><button data-sf-inputs data-inputs="a" class="a">${map.a||'A'}</button><button data-sf-inputs data-inputs="b" class="b">${map.b||'B'}</button></div>`}
    return `<div class="sf-pad-face two">${layout.face.map(([input,label])=>`<button data-sf-inputs data-inputs="${input}" class="${input}">${label}</button>`).join('')}</div>`;
  }

  function install(){
    const overlay=$('#game-player-overlay');if(!overlay)return;
    releaseAll();$('.g3-virtual-controller',overlay)?.remove();
    if(!gameplayVisible(overlay)){overlay.classList.remove('sf-virtual-pad-on');return}
    const game=current();if(!game)return;const layout=layouts[game.platform]||layouts.nes;
    overlay.classList.toggle('sf-virtual-pad-on',enabled());overlay.dataset.gamePlatform=game.platform||'';
    pad?.remove();pad=null;if(!enabled())return;
    pad=document.createElement('div');pad.className='sf-virtual-pad';pad.dataset.sfVirtualPad='1';
    pad.innerHTML=`${shoulders(layout)}<div class="sf-pad-main">${touchStick()}<div class="sf-pad-center"><button data-sf-inputs data-inputs="select">SELECT</button><button data-sf-inputs data-inputs="start">START</button></div>${faces(layout)}</div><small>${layout.name}</small>`;
    $('.game-player-stage',overlay)?.appendChild(pad);$$('[data-sf-inputs]',pad).forEach(bindButton);const stick=$('[data-sf-stick]',pad);if(stick)bindStick(stick);
  }

  function refresh(){
    const overlay=$('#game-player-overlay');if(!overlay)return;
    if(!gameplayVisible(overlay)){releaseAll();$('[data-sf-virtual-pad]',overlay)?.remove();pad=null;overlay.classList.remove('sf-virtual-pad-on');return}
    overlay.classList.toggle('sf-virtual-pad-on',enabled());
    if(enabled()&&!$('[data-sf-virtual-pad]',overlay))install();
    if(!enabled()&&$('[data-sf-virtual-pad]',overlay)){releaseAll();$('[data-sf-virtual-pad]',overlay)?.remove();pad=null}
  }

  document.addEventListener('pointerup',event=>{releasePointer(event.pointerId);releaseStick(event.pointerId)},true);document.addEventListener('pointercancel',event=>{releasePointer(event.pointerId);releaseStick(event.pointerId)},true);
  addEventListener('blur',releaseAll);document.addEventListener('visibilitychange',()=>{if(document.hidden)releaseAll()});addEventListener('resize',refresh);addEventListener('orientationchange',()=>setTimeout(refresh,120));
  window.addEventListener('stormflix:game-started',()=>setTimeout(install,0));window.addEventListener('stormflix:game-closed',()=>{releaseAll();pad=null});

  const observer=new MutationObserver(()=>{
    const overlay=$('#game-player-overlay');if(!overlay)return;
    if(gameplayVisible(overlay)&&!pad&&current())install();
    const quick=$('[data-g3-quick-menu]',overlay);if(quick&&!quick.classList.contains('hidden'))releaseAll();
  });
  observer.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['class']});
})();
