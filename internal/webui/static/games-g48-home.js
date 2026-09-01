/* StormFlix Games G4.8: personalized Games Home dashboard.
 * Keeps the latest session as the only hero, moves the remaining active games
 * into a compact activity panel and progressively de-duplicates every rail.
 */
(function(){
  const $=(s,r=document)=>r.querySelector(s);
  const $$=(s,r=document)=>[...r.querySelectorAll(s)];
  let observer=null,timer=0;

  function sectionByTitle(home,title){
    return $$(':scope > .gx-section',home).find(section=>$('h2',section)?.textContent.trim()===title)||null;
  }
  function gameID(node){
    const button=node?.matches?.('[data-game-open]')?node:node?.querySelector?.('[data-game-open]');
    const id=Number(button?.dataset.gameOpen||0);
    return Number.isFinite(id)&&id>0?id:0;
  }
  function removeDuplicateCards(section,seen){
    if(!section)return;
    for(const card of $$('.gx-card',section)){
      const id=gameID(card);
      if(!id||seen.has(id)){card.remove();continue}
      seen.add(id);
    }
    if(!section.querySelector('.gx-card'))section.remove();
  }
  function quickTile(screen,icon,title,copy){
    const button=document.createElement('button');
    button.type='button';button.className='g48-quick-card';button.dataset.g48Screen=screen;
    button.innerHTML=`<span>${icon}</span><strong>${title}</strong><small>${copy}</small>`;
    button.onclick=()=>document.querySelector(`#games-view [data-gx-screen="${screen}"]`)?.click();
    return button;
  }
  function buildActivityPanel(activeCards){
    const aside=document.createElement('aside');aside.className='g48-activity';
    const head=document.createElement('div');head.className='g48-activity-head';
    head.innerHTML=`<div><p>SUA ATIVIDADE</p><h2>${activeCards.length?'Continue seus outros jogos':'Acesso rápido'}</h2></div><small>${activeCards.length?'Retome outra partida sem repetir o destaque.':'Sua biblioteca e seus saves em um toque.'}</small>`;
    const grid=document.createElement('div');grid.className='g48-activity-grid';
    activeCards.forEach(card=>{card.classList.add('g48-continue-card');grid.appendChild(card)});
    const shortcuts=[
      ['library','▦','Biblioteca','Ver todos os jogos'],
      ['saves','▤','Saves','Continuar de outro save'],
      ['collections','▣','Coleções','Explorar séries e franquias']
    ];
    let i=0;
    while(grid.children.length<4&&i<shortcuts.length){const q=shortcuts[i++];grid.appendChild(quickTile(...q))}
    aside.append(head,grid);return aside;
  }
  function enhanceHome(){
    const home=$('#games-view .gx-home');
    if(!home||home.dataset.g48Enhanced==='1')return;
    const hero=$(':scope > .gx-hero',home);
    if(!hero)return;
    home.dataset.g48Enhanced='1';

    const continued=sectionByTitle(home,'Continuar jogando');
    const heroId=gameID(hero);
    const continuedCards=continued?$$('.gx-card',continued):[];
    const hasProgress=continuedCards.some(card=>gameID(card)===heroId)||/CONTINUAR/i.test($('.gx-hero-copy>p:first-child',hero)?.textContent||'');
    const otherActive=[];
    for(const card of continuedCards){
      if(gameID(card)===heroId)card.remove();
      else otherActive.push(card);
    }
    const shownActive=otherActive.slice(0,4);
    continued?.remove();

    const eyebrow=$('.gx-hero-copy>p:first-child',hero);
    if(eyebrow)eyebrow.textContent=hasProgress?'RETOMAR ÚLTIMA PARTIDA':'DESTAQUE DA SUA BIBLIOTECA';
    const hint=$('.gx-hero-copy>small',hero);
    if(hint){hint.classList.add('g48-hero-action');hint.textContent=hasProgress?'▶ Continuar partida':'▶ Abrir jogo'}

    const dashboard=document.createElement('section');dashboard.className='g48-dashboard';
    hero.before(dashboard);dashboard.append(hero,buildActivityPanel(shownActive));

    const seen=new Set();if(heroId)seen.add(heroId);shownActive.forEach(card=>{const id=gameID(card);if(id)seen.add(id)});
    const favorites=sectionByTitle(home,'Favoritos');
    const recent=sectionByTitle(home,'Adicionados recentemente');
    const ready=sectionByTitle(home,'Prontos para jogar');
    removeDuplicateCards(favorites,seen);
    removeDuplicateCards(recent,seen);
    removeDuplicateCards(ready,seen);

    const readyAlive=ready?.isConnected?ready:null;
    if(readyAlive){const title=$('h2',readyAlive);if(title)title.textContent='Explore sua biblioteca'}

    /* Priority is intentional: active session -> favorites -> recent -> discovery.
       This prevents the catch-all library rail from consuming every game before
       the curated rows get a chance to show something useful. */
    for(const section of [favorites,recent,readyAlive])if(section?.isConnected)home.appendChild(section);
  }
  function schedule(){clearTimeout(timer);timer=setTimeout(enhanceHome,30)}
  function boot(){
    const root=$('#games-view');if(!root)return;
    observer=new MutationObserver(schedule);observer.observe(root,{childList:true,subtree:true});
    document.addEventListener('click',e=>{if(e.target.closest?.('#games-nav,[data-gx-screen="home"]'))setTimeout(schedule,60)},true);
    window.addEventListener('stormflix:profile',()=>setTimeout(schedule,80));
    schedule();
  }
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot,{once:true});else boot();
})();

/* G4.9 is loaded from G4.8 so older cached shells keep receiving the new
 * Collections behavior after the normal hard refresh used for deployments. */
(function(){
  if(document.querySelector('script[data-games-g49]'))return;
  const script=document.createElement('script');
  script.src='/games-g49-collections.js?v=g49';script.defer=true;script.dataset.gamesG49='1';
  document.head.appendChild(script);
})();
