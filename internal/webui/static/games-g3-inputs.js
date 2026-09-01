/* StormFlix Games G3.1 input layer.
 * Input model follows the robust pattern used by RetroAssembly: virtual buttons
 * call Nostalgist pressDown/pressUp directly instead of synthesizing DOM keys.
 */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  const activePointers=new Map();
  let pad=null;

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
  function releaseAll(){for(const id of [...activePointers.keys()])releasePointer(id)}
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

  function dpad(){
    return `<div class="sf-pad-dpad" aria-label="Direcional">
      <button data-sf-inputs data-inputs="up,left" class="diag ul" aria-label="Cima e esquerda">↖</button><button data-sf-inputs data-inputs="up" class="up" aria-label="Cima">▲</button><button data-sf-inputs data-inputs="up,right" class="diag ur" aria-label="Cima e direita">↗</button>
      <button data-sf-inputs data-inputs="left" class="left" aria-label="Esquerda">◀</button><span class="center"></span><button data-sf-inputs data-inputs="right" class="right" aria-label="Direita">▶</button>
      <button data-sf-inputs data-inputs="down,left" class="diag dl" aria-label="Baixo e esquerda">↙</button><button data-sf-inputs data-inputs="down" class="down" aria-label="Baixo">▼</button><button data-sf-inputs data-inputs="down,right" class="diag dr" aria-label="Baixo e direita">↘</button>
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
    $('.g3-virtual-controller',overlay)?.remove();
    if(!gameplayVisible(overlay)){overlay.classList.remove('sf-virtual-pad-on');return}
    const game=current();if(!game)return;const layout=layouts[game.platform]||layouts.nes;
    overlay.classList.toggle('sf-virtual-pad-on',enabled());overlay.dataset.gamePlatform=game.platform||'';
    pad?.remove();pad=null;if(!enabled())return;
    pad=document.createElement('div');pad.className='sf-virtual-pad';pad.dataset.sfVirtualPad='1';
    pad.innerHTML=`${shoulders(layout)}<div class="sf-pad-main">${dpad()}<div class="sf-pad-center"><button data-sf-inputs data-inputs="select">SELECT</button><button data-sf-inputs data-inputs="start">START</button></div>${faces(layout)}</div><small>${layout.name}</small>`;
    $('.game-player-stage',overlay)?.appendChild(pad);$$('[data-sf-inputs]',pad).forEach(bindButton);
  }

  function refresh(){
    const overlay=$('#game-player-overlay');if(!overlay)return;
    if(!gameplayVisible(overlay)){releaseAll();$('[data-sf-virtual-pad]',overlay)?.remove();pad=null;overlay.classList.remove('sf-virtual-pad-on');return}
    overlay.classList.toggle('sf-virtual-pad-on',enabled());
    if(enabled()&&!$('[data-sf-virtual-pad]',overlay))install();
    if(!enabled()&&$('[data-sf-virtual-pad]',overlay)){releaseAll();$('[data-sf-virtual-pad]',overlay)?.remove();pad=null}
  }

  document.addEventListener('pointerup',event=>releasePointer(event.pointerId),true);document.addEventListener('pointercancel',event=>releasePointer(event.pointerId),true);
  addEventListener('blur',releaseAll);document.addEventListener('visibilitychange',()=>{if(document.hidden)releaseAll()});addEventListener('resize',refresh);addEventListener('orientationchange',()=>setTimeout(refresh,120));
  window.addEventListener('stormflix:game-started',()=>setTimeout(install,0));window.addEventListener('stormflix:game-closed',()=>{releaseAll();pad=null});

  const observer=new MutationObserver(()=>{
    const overlay=$('#game-player-overlay');if(!overlay)return;
    if(gameplayVisible(overlay)&&!pad&&current())install();
    const quick=$('[data-g3-quick-menu]',overlay);if(quick&&!quick.classList.contains('hidden'))releaseAll();
  });
  observer.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['class']});
})();
