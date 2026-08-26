/* StormFlix cast discovery: actor -> titles already available locally. */
(function(){
  let returnToDetail=false;
  let personOpen=false;

  function ensurePersonView(){
    let view=document.querySelector('#person-view');
    if(view)return view;
    view=document.createElement('section');
    view.id='person-view';
    view.className='person-view hidden';
    view.innerHTML=`<div class="person-view-head"><button type="button" id="person-back" class="person-back" aria-label="Voltar">←</button><div id="person-portrait" class="person-portrait"></div><div><p class="person-kicker">Elenco no StormFlix</p><h1 id="person-name"></h1><p id="person-summary"></p></div></div><div id="person-results" class="person-results"></div>`;
    document.querySelector('#main')?.appendChild(view);
    view.querySelector('#person-back').onclick=closePerson;
    return view;
  }

  const originalRenderCast=renderCast;
  renderCast=function(cast){
    const section=document.querySelector('#cast-section'),root=document.querySelector('#cast-row');
    if(!cast?.length){section?.classList.add('hidden');if(root)root.innerHTML='';return}
    section?.classList.remove('hidden');
    if(!root){originalRenderCast(cast);return}
    root.innerHTML=cast.map(person=>`<button type="button" class="cast-card cast-card-button" data-person-name="${escapeHTML(person.name)}" title="Ver títulos com ${escapeHTML(person.name)}">${person.profile_url?`<img src="${escapeHTML(person.profile_url)}" alt="${escapeHTML(person.name)}" loading="lazy">`:`<div class="cast-avatar">${escapeHTML((person.name||'?').charAt(0))}</div>`}<b>${escapeHTML(person.name)}</b><span>${escapeHTML(person.character||'')}</span><small>Ver títulos →</small></button>`).join('');
    root.querySelectorAll('[data-person-name]').forEach(button=>button.onclick=()=>openPerson(button.dataset.personName));
  };

  async function openPerson(name){
    name=String(name||'').trim();if(!name)return;
    const view=ensurePersonView();
    returnToDetail=!!currentDetail && !document.querySelector('#detail-modal')?.classList.contains('hidden');
    personOpen=true;
    stopTheme?.();
    document.querySelector('#hero')?.classList.add('hidden');
    document.querySelector('#search-view')?.classList.add('hidden');
    document.querySelector('#catalog-view')?.classList.add('hidden');
    document.querySelector('#detail-modal')?.classList.add('hidden');
    document.body.classList.remove('detail-open');
    view.classList.remove('hidden');
    view.querySelector('#person-name').textContent=name;
    view.querySelector('#person-summary').textContent='Buscando títulos disponíveis na sua biblioteca…';
    view.querySelector('#person-portrait').innerHTML='<span>…</span>';
    view.querySelector('#person-results').innerHTML='<div class="person-empty">Carregando filmografia local…</div>';
    window.scrollTo({top:0,behavior:'auto'});
    try{
      const data=await request(`/people?name=${encodeURIComponent(name)}`);
      const person=data.person||{name};
      view.querySelector('#person-name').textContent=person.name||name;
      view.querySelector('#person-summary').textContent=`${(data.items||[]).length} título(s) disponível(is) no StormFlix`;
      view.querySelector('#person-portrait').innerHTML=person.profile_url?`<img src="${escapeHTML(person.profile_url)}" alt="${escapeHTML(person.name||name)}">`:`<span>${escapeHTML((person.name||name).charAt(0).toUpperCase())}</span>`;
      const root=view.querySelector('#person-results');
      root.innerHTML=(data.items||[]).length?`<div class="person-grid">${data.items.map(cardHTML).join('')}</div>`:'<div class="person-empty">Nenhum outro título com esta pessoa está catalogado no StormFlix.</div>';
      bindCards(root);
    }catch(err){
      view.querySelector('#person-results').innerHTML=`<div class="person-empty error">${escapeHTML(err.message)}</div>`;
    }
  }

  function hidePersonOnly(){
    if(!personOpen)return;
    personOpen=false;
    document.querySelector('#person-view')?.classList.add('hidden');
  }

  function closePerson(){
    hidePersonOnly();
    if(returnToDetail&&currentDetail){
      document.querySelector('#detail-modal')?.classList.remove('hidden');
      document.querySelector('#detail-modal')?.setAttribute('aria-hidden','false');
      document.body.classList.add('detail-open');
      window.scrollTo({top:0,behavior:'auto'});
      return;
    }
    showHome();
  }

  const baseOpenDetail=openDetail;
  openDetail=async function(id){
    if(personOpen){hidePersonOnly();returnToDetail=false}
    return baseOpenDetail(id);
  };
  const baseShowHome=showHome;
  showHome=function(){hidePersonOnly();returnToDetail=false;return baseShowHome()};

  window.sfOpenPerson=openPerson;
})();
