/* StormFlix Web Player v5.4 — presentation-only screen / zoom modes.
   These controls never rebuild the playback session or touch the HLS source. */
(function(){
  const modal=document.querySelector('#player-modal');
  const video=document.querySelector('#player');
  if(!modal||!video||modal.dataset.sfScreenV54==='1')return;
  modal.dataset.sfScreenV54='1';

  const STORAGE_KEY='stormflix.player.screen_mode';
  const modes=[
    ['fit','Ajustar / sem zoom','Preserva a proporção original e mostra toda a imagem'],
    ['16x9','16:9 · sem corte','Mantém uma janela 16:9 sem recortar o vídeo'],
    ['zoom','Preencher / Zoom','Preenche a tela e corta somente o excesso das bordas'],
    ['stretch','Esticar para tela','Preenche toda a tela sem barras, alterando a proporção']
  ];
  const valid=new Set(modes.map(x=>x[0]));
  let current=localStorage.getItem(STORAGE_KEY)||'fit';
  if(!valid.has(current))current='fit';

  const fullscreen=document.querySelector('#sf-fullscreen');
  const button=document.createElement('button');
  button.id='sf-v54-screen';
  button.type='button';
  button.className='sf-control-btn sf-v54-screen-btn';
  button.title='Tela / Zoom';
  button.setAttribute('aria-label','Tela / Zoom');
  button.innerHTML='<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 5h18v14H3V5zm2 2v10h14V7H5zm2.2 2H11v1.8H9v2.4h2V15H7.2V9zm5.8 0h3.8v6H13v-1.8h2v-2.4h-2V9z"/></svg><span class="sf-v4-control-label">Tela</span>';
  if(fullscreen?.parentElement)fullscreen.parentElement.insertBefore(button,fullscreen);

  const menu=document.createElement('div');
  menu.id='sf-v54-screen-menu';
  menu.className='sf-v5-popover sf-v54-screen-menu hidden';
  menu.innerHTML='<header><div><b>Tela / Zoom</b><small>Apenas muda a apresentação. O streaming continua sem reiniciar.</small></div><button type="button" data-v54-close>×</button></header><div class="sf-v54-screen-list"></div>';
  modal.appendChild(menu);

  function label(mode){return modes.find(x=>x[0]===mode)?.[1]||'Ajustar / sem zoom'}

  function apply(mode,announce){
    if(!valid.has(mode))mode='fit';
    current=mode;
    localStorage.setItem(STORAGE_KEY,mode);
    modal.dataset.sfScreenMode=mode;
    modal.classList.remove('sf-screen-fit','sf-screen-16x9','sf-screen-zoom','sf-screen-stretch');
    modal.classList.add(`sf-screen-${mode}`);
    render();
    if(announce&&typeof sfToast==='function')sfToast(label(mode));
    window.dispatchEvent(new CustomEvent('stormflix:screen-mode',{detail:{mode}}));
  }

  function render(){
    const root=menu.querySelector('.sf-v54-screen-list');
    if(!root)return;
    root.innerHTML=modes.map(([value,title,hint])=>`<button type="button" data-v54-screen="${value}" class="${value===current?'active':''}"><span><b>${title}</b><small>${hint}</small></span><i>${value===current?'✓':''}</i></button>`).join('');
  }

  function closeOtherMenus(){
    document.querySelector('#sf-v5-quality-menu')?.classList.add('hidden');
    document.querySelector('#sf-v5-diagnostics')?.classList.add('hidden');
    document.querySelector('#sf-v53-audio-menu')?.classList.add('hidden');
    document.querySelector('#sf-player-settings-panel')?.classList.add('hidden');
  }

  function toggle(force){
    const show=force===undefined?menu.classList.contains('hidden'):Boolean(force);
    if(show)closeOtherMenus();
    menu.classList.toggle('hidden',!show);
    if(show)render();
  }

  button.addEventListener('click',event=>{event.stopPropagation();toggle()});
  menu.querySelector('[data-v54-close]').onclick=()=>toggle(false);
  menu.addEventListener('click',event=>{
    const target=event.target.closest?.('[data-v54-screen]');
    if(!target)return;
    apply(target.dataset.v54Screen,true);
    toggle(false);
  });

  document.addEventListener('keydown',event=>{
    if(modal.classList.contains('hidden'))return;
    if(event.key==='Escape')toggle(false);
  });

  // Reapply after metadata/fullscreen transitions because browsers may rebuild
  // the rendering box while the media source itself remains untouched.
  video.addEventListener('loadedmetadata',()=>apply(current,false),{passive:true});
  document.addEventListener('fullscreenchange',()=>apply(current,false),{passive:true});

  apply(current,false);
  window.sfScreenMode={get:()=>current,set:mode=>apply(mode,true),modes:()=>modes.map(x=>x[0])};
})();