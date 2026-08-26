/* StormFlix profile hub: history, stats and monthly league. */
(function(){
  function ensureHub(){
    let hub=document.querySelector('#profile-hub');
    if(hub)return hub;
    hub=document.createElement('section');
    hub.id='profile-hub';
    hub.className='profile-hub hidden';
    hub.innerHTML=`<div class="profile-hub-shell"><header class="profile-hub-head"><div><p>SEU ESPAÇO</p><h1 id="profile-hub-title">Meu perfil</h1></div><button id="profile-hub-close" type="button" aria-label="Fechar">✕</button></header><nav class="profile-hub-tabs"><button class="active" data-profile-tab="overview">Visão geral</button><button data-profile-tab="history">Histórico</button><button data-profile-tab="league">Liga StormFlix</button></nav><div id="profile-hub-content" class="profile-hub-content"></div></div>`;
    document.body.appendChild(hub);
    hub.querySelector('#profile-hub-close').onclick=close;
    hub.addEventListener('click',e=>{if(e.target===hub)close()});
    hub.querySelectorAll('[data-profile-tab]').forEach(button=>button.onclick=()=>selectTab(button.dataset.profileTab));
    return hub;
  }

  async function open(tab='overview'){
    const profile=window.sfProfiles?.current?.();
    if(!profile)return window.sfProfiles?.show?.();
    const hub=ensureHub();
    hub.classList.remove('hidden');
    document.body.classList.add('profile-hub-open');
    document.querySelector('#profile-hub-title').textContent=profile.name||'Meu perfil';
    await selectTab(tab);
  }
  function close(){
    document.querySelector('#profile-hub')?.classList.add('hidden');
    document.body.classList.remove('profile-hub-open');
  }

  async function selectTab(tab){
    const hub=ensureHub();
    hub.querySelectorAll('[data-profile-tab]').forEach(b=>b.classList.toggle('active',b.dataset.profileTab===tab));
    const root=hub.querySelector('#profile-hub-content');
    root.innerHTML='<div class="profile-hub-loading">Carregando…</div>';
    try{
      if(tab==='history')return renderHistory(root,await request('/profiles/history?limit=120'));
      if(tab==='league')return renderLeague(root,await request('/community/ranking'));
      const [stats,history,league]=await Promise.all([request('/profiles/stats'),request('/profiles/history?limit=8'),request('/community/ranking')]);
      renderOverview(root,stats,history,league);
    }catch(err){root.innerHTML=`<p class="profile-hub-error">${escapeHTML(err.message)}</p>`}
  }

  function renderOverview(root,stats,history,league){
    const profile=window.sfProfiles?.current?.()||{};
    const own=(league.ranking||[]).find(x=>Number(x.profile_id)===Number(profile.id));
    root.innerHTML=`<div class="profile-summary"><div class="profile-summary-avatar">${profile.avatar_url?`<img src="${escapeHTML(profile.avatar_url)}" alt="">`:escapeHTML((profile.name||'S').charAt(0).toUpperCase())}</div><div><span>PERFIL ATUAL</span><h2>${escapeHTML(profile.name||'Perfil')}</h2><p>${profile.is_kids?'Perfil infantil · ':''}Classificação máxima: ${ratingText(profile.content_rating_limit)}</p></div></div><div class="profile-stat-grid"><article><span>Tempo registrado</span><strong>${duration(stats.watch_seconds)}</strong><small>desde a ativação das estatísticas</small></article><article><span>Este mês</span><strong>${duration(stats.month_watch_seconds)}</strong><small>tempo assistido</small></article><article><span>Concluídos</span><strong>${Number(stats.completed_titles||0)}</strong><small>títulos finalizados</small></article><article><span>Sequência</span><strong>${Number(stats.current_streak||0)} dias</strong><small>dias consecutivos</small></article><article><span>Liga StormFlix</span><strong>${own?`#${own.rank}`:'—'}</strong><small>${escapeHTML(league.period||'Este mês')}</small></article></div><section class="profile-hub-section"><div class="profile-hub-section-head"><h2>Assistidos recentemente</h2><button data-open-history>Ver histórico</button></div>${historyCards(history)}</section><section class="profile-hub-section"><div class="profile-hub-section-head"><h2>Liga StormFlix</h2><button data-open-league>Ver classificação</button></div>${leaguePreview(league.ranking||[])}</section>`;
    root.querySelector('[data-open-history]')?.addEventListener('click',()=>selectTab('history'));
    root.querySelector('[data-open-league]')?.addEventListener('click',()=>selectTab('league'));
    bindHistory(root);
  }

  function renderHistory(root,items){
    root.innerHTML=`<section class="profile-hub-section full"><div class="profile-hub-section-head"><div><p>ATIVIDADE DO PERFIL</p><h2>Histórico de reprodução</h2></div><span>${items.length} títulos</span></div>${items.length?`<div class="profile-history-grid">${items.map(historyCard).join('')}</div>`:'<div class="profile-empty">Seu histórico aparecerá aqui quando você começar a assistir.</div>'}</section>`;
    bindHistory(root);
  }

  function historyCards(items){return items?.length?`<div class="profile-history-strip">${items.map(historyCard).join('')}</div>`:'<div class="profile-empty">Nada assistido ainda.</div>'}
  function historyCard(item){
    const pct=Math.max(0,Math.min(100,Number(item.progress_percent||0)));
    const poster=item.poster_url?`<img src="${escapeHTML(item.poster_url)}" alt="" loading="lazy">`:'<div class="profile-history-fallback">SF</div>';
    return `<button class="profile-history-card" data-history-media="${item.id}" type="button"><span class="profile-history-poster">${poster}${item.completed?'<i class="history-complete">✓ Concluído</i>':''}<b style="--history-progress:${pct}%"></b></span><strong>${escapeHTML(item.title)}</strong><small>${item.year||''}${item.completed?' · Assistido':pct?` · ${Math.round(pct)}%`:''}</small></button>`;
  }
  function bindHistory(root){root.querySelectorAll('[data-history-media]').forEach(card=>card.onclick=()=>{close();openDetail(Number(card.dataset.historyMedia))})}

  function renderLeague(root,data){
    const ranking=data.ranking||[];
    root.innerHTML=`<section class="profile-hub-section full"><div class="league-hero"><p>COMPETIÇÃO SAUDÁVEL DA COMUNIDADE</p><h2>${escapeHTML(data.title||'Liga StormFlix')}</h2><span>${escapeHTML(data.period||'Este mês')} · baseada em tempo realmente reproduzido</span></div>${ranking.length?`<div class="league-table">${ranking.map(leagueRow).join('')}</div>`:'<div class="profile-empty">A Liga começa a aparecer conforme os perfis assistem conteúdo.</div>'}<p class="league-note">Pulos na linha do tempo e retomadas não contam como minutos extras. A posição é calculada somente com reprodução registrada.</p></section>`;
  }
  function leaguePreview(ranking){return ranking.length?`<div class="league-preview">${ranking.slice(0,5).map(leagueRow).join('')}</div>`:'<div class="profile-empty">A Liga ainda está aquecendo.</div>'}
  function leagueRow(item){
    const medal=item.rank===1?'🥇':item.rank===2?'🥈':item.rank===3?'🥉':`#${item.rank}`;
    const avatar=item.avatar_url?`<img src="${escapeHTML(item.avatar_url)}" alt="">`:escapeHTML((item.name||'S').charAt(0).toUpperCase());
    return `<article class="league-row ${item.rank<=3?'podium':''}"><strong class="league-rank">${medal}</strong><span class="league-avatar avatar-${escapeHTML(item.avatar_key||'storm-red')}">${avatar}</span><div><b>${escapeHTML(item.name)}</b><small>${duration(item.watch_seconds)} · ${item.completed_titles||0} concluídos · ${item.active_days||0} dias ativos</small></div></article>`;
  }

  function duration(seconds){
    const total=Math.max(0,Math.round(Number(seconds||0)));
    const hours=Math.floor(total/3600),minutes=Math.floor((total%3600)/60);
    if(hours>0)return `${hours}h ${minutes}min`;
    return `${minutes}min`;
  }
  function ratingText(value){return Number(value)===0?'Livre':Number(value)>=18?'18 / sem restrição':`${Number(value)||18} anos`}

  window.sfProfileHub={open,close};
  document.addEventListener('DOMContentLoaded',()=>{
    const button=document.querySelector('#profile-hub-btn');
    if(button)button.onclick=()=>open('overview');
  });
})();
