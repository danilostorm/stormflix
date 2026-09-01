/* StormFlix Games G3.4 input layer.
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
  const ALL_DIGITAL_INPUTS=['up','down','left','right','a','b','x','y','l','r','l2','r2','select','start'];
  let pad=null;
  let stickState={pointerId:null,inputs:[],node:null};

  const layouts={
    nes:{name:'NES',face:[['b','B'],['a','A']]},
    gb:{name:'Game Boy',face:[['b','B'],['a','A']]},
    gbc:{name:'Game Boy Color',face:[['b','B'],['a','A']]},
    gba:{name:'Game Boy Advance',shoulders:[['l','L'],['r','R']],face:[['b','B'],['a','A']]},
    snes:{name:'Super Nintendo',shoulders:[['l','L'],['r','R']],face:[['y','Y'],['x','X'],['b','B'],['a','A']]},
    genesis:{name:'Mega Drive / Genesis',face:[['y','A'],['b','B'],['a','C'],['x','X'],['l','Y'],['r','Z']],genesis:true},
  };

  function player(){return window.StormFlixGamePlayer}
  function current(){return player()?.current?.()||null}
  function coarse(){return !!window.matchMedia?.('(pointer: coarse)').matches||innerWidth<=900}
  function enabled(){const mode=localStorage.getItem('stormflix.games.virtual-controls')||'auto';return mode==='on'||(mode==='auto'&&coarse())}
  function gameplayVisible(overlay){return !!overlay&&!$('[data-game-controls]',overlay)?.classList.contains('hidden')}
  function requestPlayerResize(delay=0){setTimeout(()=>player()?.resize?.(),Math.max(0,delay))}

  function press(input,down){
    if(!ALL_DIGITAL_INPUTS.includes(input))return false;
    const api=player();if(!api)return false;
    return down?api.pressDown?.(input)!==false:api.pressUp?.(input)!==false;
  }

  function releasePointer(id){
    const entry=activePointers.get(id);if(!entry)return;
    activePointers.delete(id);
    for(const input of entry.inputs)press(input,false);
    const node=entry.node;
    if(node){node.classList.remove('pressed');node.removeAttribute('data-pointer-id')}
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
  function resetVirtualInputs(){for(const input of ALL_DIGITAL_INPUTS)press(input,false)}
  function releaseAll(){for(const id of [...activePointers.keys()])releasePointer(id);releaseStick();resetVirtualInputs()}
  function inputsFor(button){return String(button?.dataset.inputs||'').split(',').map(x=>x.trim()).filter(input=>ALL_DIGITAL_INPUTS.includes(input))}

  function pressPointerButton(pointerId,button){
    releasePointer(pointerId);
    const inputs=inputsFor(button);if(!inputs.length)return;
    activePointers.set(pointerId,{inputs,node:button});button.classList.add('pressed');button.dataset.pointerId=String(pointerId);
    for(const input of inputs)press(input,true);
  }

  function bindButton(button){
    button.addEventListener('pointerdown',event=>{
      event.preventDefault();event.stopPropagation();
      // Touch browsers may apply implicit pointer capture. RetroAssembly releases
      // that capture so the global pointerup/pointercancel owner always sees the
      // end of the gesture instead of leaving a logical button held.
      try{if(button.hasPointerCapture?.(event.pointerId))button.releasePointerCapture(event.pointerId)}catch{}
      if(navigator.vibrate&&localStorage.getItem('stormflix.games.haptics')!=='off')navigator.vibrate(8);
      pressPointerButton(event.pointerId,button);
    },{passive:false});
    button.addEventListener('pointermove',event=>{
      if(!activePointers.has(event.pointerId))return;
      event.preventDefault();
      if(event.buttons===0)releasePointer(event.pointerId);
    },{passive:false});
    button.addEventListener('pointerup',event=>{event.preventDefault();releasePointer(event.pointerId)},{passive:false});
    button.addEventListener('pointercancel',event=>releasePointer(event.pointerId));
    button.addEventListener('click',event=>{event.preventDefault();event.stopPropagation()});
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
    node.addEventListener('lostpointercapture',event=>releaseStick(event.pointerId));
    node.addEventListener('contextmenu',event=>event.preventDefault());
  }

  function touchStick(){
    return `<div class="sf-pad-stick" data-sf-stick role="application" aria-label="Direcional touch">
      <span class="sf-pad-stick-orbit" aria-hidden="true"></span><span class="sf-pad-stick-thumb" data-sf-stick-thumb aria-hidden="true"></span><span class="sf-sr-only">Arraste a bolinha para mover em oito direções</span>
    </div>`;
  }
  function inputButton(input,label,extra=''){return `<button type="button" data-sf-inputs data-inputs="${input}"${extra}>${label}</button>`}
  function shoulders(layout){return layout.shoulders?.length?`<div class="sf-pad-shoulders">${layout.shoulders.map(([input,label])=>inputButton(input,label)).join('')}</div>`:''}
  function faces(layout){
    if(layout.genesis){const top=layout.face.slice(3),bottom=layout.face.slice(0,3);return `<div class="sf-pad-face genesis"><div>${top.map(([input,label])=>inputButton(input,label)).join('')}</div><div>${bottom.map(([input,label])=>inputButton(input,label)).join('')}</div></div>`}
    if(layout.face.length===4){const map=Object.fromEntries(layout.face.map(([i,l])=>[i,l]));return `<div class="sf-pad-face diamond">${inputButton('x',map.x||'X',' class="x"')}${inputButton('y',map.y||'Y',' class="y"')}${inputButton('a',map.a||'A',' class="a"')}${inputButton('b',map.b||'B',' class="b"')}</div>`}
    return `<div class="sf-pad-face two">${layout.face.map(([input,label])=>inputButton(input,label,` class="${input}"`)).join('')}</div>`;
  }

  function removeLegacyController(overlay){$('.g3-virtual-controller',overlay)?.remove()}

  function install(){
    const overlay=$('#game-player-overlay');if(!overlay)return;
    releaseAll();removeLegacyController(overlay);
    if(!gameplayVisible(overlay)){overlay.classList.remove('sf-virtual-pad-on');requestPlayerResize(30);return}
    const game=current();if(!game)return;const layout=layouts[game.platform]||layouts.nes;
    overlay.classList.toggle('sf-virtual-pad-on',enabled());overlay.dataset.gamePlatform=game.platform||'';
    pad?.remove();pad=null;if(!enabled()){requestPlayerResize(30);return}
    pad=document.createElement('div');pad.className='sf-virtual-pad';pad.dataset.sfVirtualPad='1';
    pad.innerHTML=`${shoulders(layout)}<div class="sf-pad-main">${touchStick()}<div class="sf-pad-center">${inputButton('select','SELECT')}${inputButton('start','START')}</div>${faces(layout)}</div><small>${layout.name}</small>`;
    $('.game-player-stage',overlay)?.appendChild(pad);$$('[data-sf-inputs]',pad).forEach(bindButton);const stick=$('[data-sf-stick]',pad);if(stick)bindStick(stick);
    requestPlayerResize(40);requestPlayerResize(160);
  }

  function refresh(){
    const overlay=$('#game-player-overlay');if(!overlay)return;
    removeLegacyController(overlay);
    if(!gameplayVisible(overlay)){releaseAll();$('[data-sf-virtual-pad]',overlay)?.remove();pad=null;overlay.classList.remove('sf-virtual-pad-on');requestPlayerResize(40);return}
    overlay.classList.toggle('sf-virtual-pad-on',enabled());
    if(enabled()&&!$('[data-sf-virtual-pad]',overlay))install();
    if(!enabled()&&$('[data-sf-virtual-pad]',overlay)){releaseAll();$('[data-sf-virtual-pad]',overlay)?.remove();pad=null;requestPlayerResize(40)}
    else requestPlayerResize(60);
  }

  document.addEventListener('pointerup',event=>{releasePointer(event.pointerId);releaseStick(event.pointerId)},true);document.addEventListener('pointercancel',event=>{releasePointer(event.pointerId);releaseStick(event.pointerId)},true);
  addEventListener('blur',releaseAll);document.addEventListener('visibilitychange',()=>{if(document.hidden)releaseAll()});addEventListener('resize',refresh);addEventListener('orientationchange',()=>setTimeout(refresh,120));
  window.addEventListener('stormflix:virtual-controls-changed',refresh);
  window.addEventListener('stormflix:game-started',()=>setTimeout(install,0));window.addEventListener('stormflix:game-closed',()=>{releaseAll();pad=null});

  const observer=new MutationObserver(()=>{
    const overlay=$('#game-player-overlay');if(!overlay)return;
    removeLegacyController(overlay);
    if(gameplayVisible(overlay)&&!pad&&current())install();
    const quick=$('[data-g3-quick-menu]',overlay);if(quick&&!quick.classList.contains('hidden'))releaseAll();
  });
  observer.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['class']});
})();
