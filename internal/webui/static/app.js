const $=s=>document.querySelector(s),$$=s=>[...document.querySelectorAll(s)],api='/api/v1';
let me=null,feed=null,currentDetail=null,searchTimer=null,detailReturnState=null;
const themeAudio=$('#theme-audio'),player=$('#player');

async function request(path,opt={}){
  const r=await fetch(api+path,{...opt,headers:{'Content-Type':'application/json',...(opt.headers||{})}});
  const d=await r.json().catch(()=>({}));
  if(!r.ok)throw Object.assign(new Error(d.error||`HTTP ${r.status}`),{status:r.status});
  return d;
}

async function boot(){
  const setup=await request('/setup/status');
  if(setup.needs_setup){location.href='/admin/';return}
  try{me=await request('/auth/me');await authenticated()}catch{showLogin()}
}

function showLogin(){
  $('#shell').classList.add('hidden');
  $('#login').classList.remove('hidden');
}

async function authenticated(){
  $('#login').classList.add('hidden');
  $('#shell').classList.remove('hidden');
  $('#user-label').textContent=me.display_name;
  $('#profile-initial').textContent=(me.display_name||me.username||'S').trim().charAt(0).toUpperCase();
  if(me.role!=='user')$('#admin-link').classList.remove('hidden');
  await loadHome();
}

$('#login-form').onsubmit=async e=>{
  e.preventDefault();
  $('#login-error').textContent='';
  try{
    me=await request('/auth/login',{method:'POST',body:JSON.stringify({username:$('#login-user').value,password:$('#login-pass').value})});
    await authenticated();
  }catch(err){$('#login-error').textContent=err.message}
};

$('#logout').onclick=async()=>{await request('/auth/logout',{method:'POST'}).catch(()=>{});location.reload()};
$('#brand-home').onclick=e=>{e.preventDefault();showHome()};
window.addEventListener('scroll',()=>$('#topbar').classList.toggle('scrolled',window.scrollY>24),{passive:true});

async function loadHome(){
  feed=await request('/home');
  document.title=feed.server_name||'StormFlix';
  renderHero(feed.hero);
  renderRows(feed.rows||[]);
}

function responsiveImageURL(value,width){
  value=String(value||'');if(!value||!value.startsWith('/assets/'))return value;
  try{const url=new URL(value,location.origin);url.searchParams.set('w',String(width));url.searchParams.set('format','auto');return url.pathname+url.search}catch{return value}
}

function responsiveImageSet(value){
  if(!String(value||'').startsWith('/assets/'))return'';
  return [240,360,500,780].map(width=>`${responsiveImageURL(value,width)} ${width}w`).join(',');
}

function renderHero(item){
  const hero=$('#hero');
  if(!item){hero.classList.add('hidden');return}
  hero.classList.remove('hidden');
  const bg=$('#hero-backdrop');
  bg.style.backgroundImage=item.backdrop_url?`url("${cssURL(responsiveImageURL(item.backdrop_url,1280))}")`:item.poster_url?`url("${cssURL(responsiveImageURL(item.poster_url,780))}")`:'none';
  const logo=$('#hero-logo');
  if(item.logo_url){logo.src=responsiveImageURL(item.logo_url,500);logo.classList.remove('hidden');$('#hero-title').classList.add('title-with-logo')}
  else{logo.classList.add('hidden');$('#hero-title').classList.remove('title-with-logo')}
  $('#hero-title').textContent=item.title;
  $('#hero-eyebrow').textContent=item.library_name||'Destaque StormFlix';
  $('#hero-meta').innerHTML=metaHTML(item);
  $('#hero-overview').textContent=item.overview||'Abra para ver informações e iniciar a reprodução direta.';
  $('#hero-play').onclick=()=>playMedia(item);
  $('#hero-more').onclick=()=>openDetail(item.id);
}

function renderRows(rows){
  const root=$('#rows');
  root.innerHTML='';
  for(const row of rows){
    if(!row.items?.length)continue;
    const section=document.createElement('section');
    section.className='content-row';
    section.innerHTML=`<div class="row-head"><h2>${escapeHTML(row.title)}</h2><span>${row.items.length} títulos</span></div><div class="row-track">${row.items.map(cardHTML).join('')}</div>`;
    root.appendChild(section);
  }
  bindCards(root);
}

function cardHTML(item){
  const posterURL=responsiveImageURL(item.poster_url,360),posterSet=responsiveImageSet(item.poster_url);
  const poster=item.poster_url?`<img src="${escapeHTML(posterURL)}"${posterSet?` srcset="${escapeHTML(posterSet)}" sizes="(max-width: 700px) 38vw, 180px"`:''} alt="${escapeHTML(item.title)}" loading="lazy">`:`<div class="poster-fallback"><span>STORM<span>FLIX</span></span></div>`;
  const badge=item.rating?`<span class="rating">★ ${Number(item.rating).toFixed(1)}</span>`:'';
  return `<article class="media-tile" data-media="${item.id}" tabindex="0"><div class="tile-poster">${poster}<div class="tile-shade"></div><button class="tile-play" data-play="${item.id}" aria-label="Assistir">▶</button></div><div class="tile-info"><strong>${escapeHTML(item.title)}</strong><div><span>${item.year||''}</span>${badge}<span>${escapeHTML(typeLabel(item))}</span></div></div></article>`;
}

function bindCards(root=document){
  root.querySelectorAll('[data-media]').forEach(card=>{
    card.onclick=e=>{if(e.target.closest('[data-play]'))return;openDetail(+card.dataset.media)};
    card.onkeydown=e=>{if(e.key==='Enter')openDetail(+card.dataset.media)};
  });
  root.querySelectorAll('[data-play]').forEach(button=>button.onclick=async e=>{
    e.stopPropagation();
    const id=+button.dataset.play;
    const item=findItem(id)||await request(`/media/${id}`);
    playMedia(item);
  });
}

function allFeedItems(){
  const map=new Map();
  if(feed?.hero)map.set(feed.hero.id,feed.hero);
  for(const row of feed?.rows||[])for(const item of row.items||[])map.set(item.id,item);
  return [...map.values()];
}
function findItem(id){return allFeedItems().find(x=>x.id===id)}

function detailOpen(){return !$('#detail-modal').classList.contains('hidden')}
function enterDetailPage(){
  if(!detailOpen()){
    detailReturnState={
      heroHidden:$('#hero').classList.contains('hidden'),
      searchHidden:$('#search-view').classList.contains('hidden'),
      catalogHidden:$('#catalog-view').classList.contains('hidden'),
      scrollY:window.scrollY
    };
  }
  $('#hero').classList.add('hidden');
  $('#search-view').classList.add('hidden');
  $('#catalog-view').classList.add('hidden');
  $('#detail-modal').classList.remove('hidden');
  $('#detail-modal').setAttribute('aria-hidden','false');
  document.body.classList.add('detail-open');
  window.scrollTo({top:0,behavior:'auto'});
}
function discardDetailPage(){
  if(!detailOpen())return;
  stopTheme();
  $('#detail-modal').classList.add('hidden');
  $('#detail-modal').setAttribute('aria-hidden','true');
  document.body.classList.remove('detail-open');
  currentDetail=null;
  detailReturnState=null;
}
function restoreFromDetailPage(){
  stopTheme();
  currentDetail=null;
  $('#detail-modal').classList.add('hidden');
  $('#detail-modal').setAttribute('aria-hidden','true');
  document.body.classList.remove('detail-open');
  const state=detailReturnState;
  detailReturnState=null;
  if(!state){
    renderHero(feed?.hero);$('#catalog-view').classList.remove('hidden');return;
  }
  $('#hero').classList.toggle('hidden',state.heroHidden);
  $('#search-view').classList.toggle('hidden',state.searchHidden);
  $('#catalog-view').classList.toggle('hidden',state.catalogHidden);
  requestAnimationFrame(()=>window.scrollTo({top:state.scrollY||0,behavior:'auto'}));
}
window.sfEnterDetailPage=enterDetailPage;
window.sfDiscardDetailPage=discardDetailPage;

$$('[data-nav]').forEach(button=>button.onclick=()=>{
  discardDetailPage();
  const mode=button.dataset.nav;
  $$('[data-nav]').forEach(x=>x.classList.toggle('active',x===button));
  if(mode==='home'){showHome();return}
  stopTheme();
  $('#search-view').classList.add('hidden');
  $('#catalog-view').classList.remove('hidden');
  $('#hero').classList.add('hidden');
  const labels={movie:'Filmes',series:'Séries',anime:'Animes'};
  const filtered=allFeedItems().filter(item=>mode==='movie'?item.media_type==='movie':mode==='anime'?item.media_type==='anime':item.media_type==='series');
  renderRows([{id:mode,title:labels[mode],items:filtered}]);
  window.scrollTo({top:0,behavior:'smooth'});
});

function showHome(){
  discardDetailPage();
  $$('[data-nav]').forEach(x=>x.classList.toggle('active',x.dataset.nav==='home'));
  $('#search-view').classList.add('hidden');
  $('#catalog-view').classList.remove('hidden');
  renderHero(feed?.hero);
  renderRows(feed?.rows||[]);
  window.scrollTo({top:0,behavior:'smooth'});
}

$('#search-toggle').onclick=()=>{
  discardDetailPage();
  stopTheme();
  $('#hero').classList.add('hidden');
  $('#catalog-view').classList.add('hidden');
  $('#search-view').classList.remove('hidden');
  $('#search').focus();
};
$('#search-close').onclick=showHome;
$('#search').oninput=e=>{
  clearTimeout(searchTimer);
  searchTimer=setTimeout(()=>searchMedia(e.target.value.trim()),220);
};
async function searchMedia(query){
  const root=$('#search-results');
  if(!query){root.innerHTML='<div class="empty-state">Digite para buscar na sua biblioteca.</div>';return}
  root.innerHTML='<div class="empty-state">Buscando…</div>';
  try{
    const items=await request(`/media?limit=200&q=${encodeURIComponent(query)}`);
    root.innerHTML=items.length?items.map(cardHTML).join(''):'<div class="empty-state">Nenhum título encontrado.</div>';
    bindCards(root);
  }catch(err){root.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`}
}

async function openDetail(id){
  stopTheme();
  try{
    const d=await request(`/media/${id}`);
    currentDetail=d;
    $('#detail-title').textContent=d.title;
    $('#detail-tagline').textContent=d.tagline||'';
    $('#detail-meta').innerHTML=metaHTML(d,true);
    $('#detail-overview').textContent=d.overview||'Sem sinopse disponível.';
    $('#detail-directors').textContent=d.directors?.length?d.directors.join(', '):'—';
    $('#detail-genres').textContent=d.genres?.length?d.genres.join(', '):'—';
    $('#detail-library').textContent=d.library_name||'—';
    $('#detail-format').textContent=`${String(d.extension||'').replace('.','').toUpperCase()} · DIRECT PLAY`;
    const backdrop=$('#detail-backdrop');
    backdrop.style.backgroundImage=d.backdrop_url?`url("${cssURL(d.backdrop_url)}")`:d.poster_url?`url("${cssURL(d.poster_url)}")`:'none';
    const logo=$('#detail-logo');
    if(d.logo_url){logo.src=d.logo_url;logo.classList.remove('hidden');$('#detail-title').classList.add('title-with-logo')}
    else{logo.classList.add('hidden');$('#detail-title').classList.remove('title-with-logo')}
    $('#detail-play').onclick=()=>playMedia(d);
    const trailer=$('#detail-trailer');
    if(d.trailer_url){trailer.href=d.trailer_url;trailer.classList.remove('hidden')}else trailer.classList.add('hidden');
    renderCast(d.cast||[]);
    renderRelated(d.related||[]);
    setupTheme(d);
    enterDetailPage();
  }catch(err){console.error(err)}
}

function renderCast(cast){
  const section=$('#cast-section'),root=$('#cast-row');
  if(!cast.length){section.classList.add('hidden');root.innerHTML='';return}
  section.classList.remove('hidden');
  root.innerHTML=cast.map(person=>`<article class="cast-card">${person.profile_url?`<img src="${escapeHTML(person.profile_url)}" alt="${escapeHTML(person.name)}" loading="lazy">`:`<div class="cast-avatar">${escapeHTML(person.name.charAt(0))}</div>`}<b>${escapeHTML(person.name)}</b><span>${escapeHTML(person.character||'')}</span></article>`).join('');
}
function renderRelated(items){
  const section=$('#related-section'),root=$('#related-row');
  if(!items.length){section.classList.add('hidden');root.innerHTML='';return}
  section.classList.remove('hidden');
  root.innerHTML=items.map(cardHTML).join('');
  bindCards(root);
}

function setupTheme(d){
  const button=$('#theme-toggle'),wrap=$('#theme-info-wrap');
  if(!feed?.theme_preview_enabled||!d.theme_preview_url){button.classList.add('hidden');wrap.classList.add('hidden');return}
  themeAudio.src=d.theme_preview_url;
  themeAudio.volume=Math.max(0,Math.min(1,(feed.theme_preview_volume??24)/100));
  $('#theme-info').textContent=d.theme_preview_title||'Prévia de trilha sonora';
  wrap.classList.remove('hidden');button.classList.remove('hidden');button.textContent='♫ Prévia da trilha';
  button.onclick=()=>{
    if(themeAudio.paused){themeAudio.play().then(()=>button.textContent='❚❚ Pausar trilha').catch(()=>{})}
    else{themeAudio.pause();button.textContent='♫ Prévia da trilha'}
  };
  if(feed.theme_preview_autoplay){themeAudio.play().then(()=>button.textContent='❚❚ Pausar trilha').catch(()=>{})}
}

$$('[data-close-detail]').forEach(x=>x.onclick=closeDetail);
function closeDetail(){restoreFromDetailPage()}
function stopTheme(){
  if(!themeAudio)return;
  themeAudio.pause();themeAudio.currentTime=0;themeAudio.removeAttribute('src');themeAudio.load();
}

function playMedia(item){
  stopTheme();
  const id=item.id;
  $('#player-title').textContent=item.title||'StormFlix';
  $('#player-help').classList.add('hidden');
  $('#player-modal').classList.remove('hidden');
  player.src=`${api}/media/${id}/stream`;
  player.load();
  player.play().catch(()=>{});
}
$('#player-close').onclick=closePlayer;
function closePlayer(){player.pause();player.removeAttribute('src');player.load();$('#player-modal').classList.add('hidden')}
player.addEventListener('error',()=>$('#player-help').classList.remove('hidden'));

function metaHTML(item,detail=false){
  const parts=[];
  if(item.year)parts.push(`<span>${item.year}</span>`);
  if(item.rating)parts.push(`<span class="match">★ ${Number(item.rating).toFixed(1)}</span>`);
  if(item.runtime_minutes)parts.push(`<span>${formatRuntime(item.runtime_minutes)}</span>`);
  if(detail&&item.media_type)parts.push(`<span>${escapeHTML(typeLabel(item))}</span>`);
  parts.push('<span class="direct-badge small">DIRECT PLAY</span>');
  return parts.join('');
}
function typeLabel(item){
  if(item.media_type==='movie')return'Filme';
  if(item.media_type==='anime')return'Anime';
  if(item.media_type==='series')return item.episode_number?`S${String(item.season_number).padStart(2,'0')}E${String(item.episode_number).padStart(2,'0')}`:'Série';
  return String(item.extension||'').replace('.','').toUpperCase();
}
function formatRuntime(minutes){const h=Math.floor(minutes/60),m=minutes%60;return h?`${h}h ${m}min`:`${m}min`}
function escapeHTML(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function cssURL(v){return String(v??'').replace(/["\\\n\r]/g,'')}

document.addEventListener('keydown',e=>{
  if(e.key!=='Escape')return;
  if(!$('#player-modal').classList.contains('hidden'))closePlayer();
  else if(detailOpen())closeDetail();
  else if(!$('#search-view').classList.contains('hidden'))showHome();
});

boot().catch(err=>{console.error(err);showLogin()});
