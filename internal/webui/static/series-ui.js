/* StormFlix series -> seasons -> episodes UI */
(function(){
  let currentSeries=null;
  const baseCardHTML=cardHTML;
  const baseBindCards=bindCards;
  const baseCloseDetail=closeDetail;

  cardHTML=function(item){
    if(item?.entity_type!=='series'||!item.series_id)return baseCardHTML(item);
    const poster=item.poster_url?`<img src="${escapeHTML(item.poster_url)}" alt="${escapeHTML(item.title)}" loading="lazy">`:`<div class="poster-fallback"><span>STORM<span>FLIX</span></span></div>`;
    const badge=item.rating?`<span class="rating">★ ${Number(item.rating).toFixed(1)}</span>`:'';
    const seasons=item.season_count?`${item.season_count} temp.`:'Série';
    const episodes=item.episode_count?`${item.episode_count} eps.`:'';
    return `<article class="media-tile series-tile" data-series="${escapeHTML(item.series_id)}" tabindex="0"><div class="tile-poster">${poster}<div class="tile-shade"></div><button class="tile-play" data-open-series="${escapeHTML(item.series_id)}" aria-label="Abrir série">▶</button></div><div class="tile-info"><strong>${escapeHTML(item.title)}</strong><div><span>${item.year||''}</span>${badge}<span>${seasons}${episodes?` · ${episodes}`:''}</span></div></div></article>`;
  };

  bindCards=function(root=document){
    baseBindCards(root);
    root.querySelectorAll('[data-series]').forEach(card=>{
      const open=()=>openSeries(card.dataset.series);
      card.onclick=e=>{if(e.target.closest('[data-open-series]'))return;open()};
      card.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();open()}};
    });
    root.querySelectorAll('[data-open-series]').forEach(button=>button.onclick=e=>{e.stopPropagation();openSeries(button.dataset.openSeries)});
  };

  // Replace the file-oriented nav filter from app.js. Series are top-level
  // entities here; raw episode cards remain available only in "Novos episódios".
  $$('[data-nav]').forEach(button=>button.onclick=()=>{
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    const mode=button.dataset.nav;
    $$('[data-nav]').forEach(x=>x.classList.toggle('active',x===button));
    if(mode==='home'){showHome();return}
    stopTheme();
    $('#search-view').classList.add('hidden');
    $('#catalog-view').classList.remove('hidden');
    $('#hero').classList.add('hidden');
    const labels={movie:'Filmes',series:'Séries',anime:'Animes'};
    const filtered=allFeedItems().filter(item=>{
      if(mode==='series')return item.entity_type==='series'&&item.media_type==='series';
      if(mode==='anime')return item.media_type==='anime'&&(item.entity_type==='series'||!item.episode_number);
      return item.media_type==='movie'&&!item.episode_number;
    });
    renderRows([{id:mode,title:labels[mode],items:filtered}]);
    window.scrollTo({top:0,behavior:'smooth'});
  });

  searchMedia=async function(query){
    const root=$('#search-results');
    if(!query){root.innerHTML='<div class="empty-state">Digite para buscar na sua biblioteca.</div>';return}
    root.innerHTML='<div class="empty-state">Buscando…</div>';
    try{
      const [mediaItems,seriesItems]=await Promise.all([
        request(`/media?limit=200&q=${encodeURIComponent(query)}`),
        request(`/series?q=${encodeURIComponent(query)}`).catch(()=>[])
      ]);
      const topMedia=(mediaItems||[]).filter(x=>!x.episode_number&&x.media_type!=='series');
      const cards=[...(seriesItems||[]).map(seriesCardItem),...topMedia];
      root.innerHTML=cards.length?cards.map(cardHTML).join(''):'<div class="empty-state">Nenhum título encontrado.</div>';
      bindCards(root);
    }catch(err){root.innerHTML=`<div class="empty-state error">${escapeHTML(err.message)}</div>`}
  };

  async function openSeries(id){
    stopTheme();
    try{
      const data=await request(`/series/${encodeURIComponent(id)}`);
      currentSeries=data;
      let representative=null;
      if(data.representative_media_id){representative=await request(`/media/${data.representative_media_id}`).catch(()=>null)}
      currentDetail=representative||null;
      $('#detail-title').textContent=data.title;
      $('#detail-tagline').textContent=representative?.tagline||'';
      $('#detail-meta').innerHTML=seriesMetaHTML(data);
      $('#detail-overview').textContent=data.overview||representative?.overview||'Sem sinopse disponível.';
      $('#detail-directors').textContent=representative?.directors?.length?representative.directors.join(', '):'—';
      $('#detail-genres').textContent=data.genres?.length?data.genres.join(', '):representative?.genres?.join(', ')||'—';
      $('#detail-library').textContent=data.library_name||'—';
      $('#detail-format').textContent=`${data.season_count||0} temporadas · ${data.episode_count||0} episódios`;
      const backdrop=$('#detail-backdrop');
      const image=data.backdrop_url||representative?.backdrop_url||data.poster_url||representative?.poster_url||'';
      backdrop.style.backgroundImage=image?`url("${cssURL(image)}")`:'none';
      const logo=$('#detail-logo');
      const logoURL=data.logo_url||representative?.logo_url||'';
      if(logoURL){logo.src=logoURL;logo.classList.remove('hidden');$('#detail-title').classList.add('title-with-logo')}
      else{logo.classList.add('hidden');$('#detail-title').classList.remove('title-with-logo')}
      const first=firstEpisode(data);
      const play=$('#detail-play');
      play.classList.toggle('hidden',!first);
      play.textContent=first?'▶ Reproduzir':'Sem episódios';
      play.onclick=()=>first&&playMedia(first);
      const trailer=$('#detail-trailer');
      if(representative?.trailer_url){trailer.href=representative.trailer_url;trailer.classList.remove('hidden')}else trailer.classList.add('hidden');
      renderCast(representative?.cast||[]);
      $('#related-section').classList.add('hidden');
      renderSeriesSeasons(data);
      if(representative)setupTheme(representative);else stopThemeButton();
      if(window.sfEnterDetailPage)window.sfEnterDetailPage();
    }catch(err){console.error(err);if(typeof sfToast==='function')sfToast(err.message)}
  }
  window.openSeries=openSeries;

  function seriesCardItem(s){
    return {id:s.representative_media_id,entity_type:'series',series_id:s.id,library_id:s.library_id,library_name:s.library_name,title:s.title,media_type:s.media_type,year:s.year,overview:s.overview,genres:s.genres,rating:s.rating,poster_url:s.poster_url,backdrop_url:s.backdrop_url,logo_url:s.logo_url,modified_unix:s.modified_unix,season_count:s.season_count,episode_count:s.episode_count};
  }

  function seriesMetaHTML(data){
    const parts=[];
    if(data.year)parts.push(`<span>${data.year}</span>`);
    if(data.rating)parts.push(`<span class="match">★ ${Number(data.rating).toFixed(1)}</span>`);
    parts.push(`<span>${data.season_count||0} temporadas</span>`);
    parts.push(`<span>${data.episode_count||0} episódios</span>`);
    return parts.join('');
  }

  function ensureSeriesSection(){
    let section=$('#series-seasons-section');
    if(section)return section;
    section=document.createElement('section');section.id='series-seasons-section';section.className='detail-section series-seasons-section';
    const main=$('.detail-main');main.insertBefore(section,$('#related-section'));
    return section;
  }

  function renderSeriesSeasons(data){
    const section=ensureSeriesSection();
    const seasons=data.seasons||[];
    if(!seasons.length){section.innerHTML='<h2>Temporadas</h2><div class="empty-state">Nenhum episódio identificado.</div>';return}
    section.innerHTML=`<div class="series-season-head"><h2>Episódios</h2><div class="season-tabs">${seasons.map((s,i)=>`<button data-season-index="${i}" class="${i===0?'active':''}">${escapeHTML(s.title||`Temporada ${s.number}`)}</button>`).join('')}</div></div><div id="series-episode-list" class="series-episode-list"></div>`;
    const render=index=>{
      section.querySelectorAll('[data-season-index]').forEach((b,i)=>b.classList.toggle('active',i===index));
      const season=seasons[index];
      $('#series-episode-list').innerHTML=(season.episodes||[]).map(episodeHTML).join('')||'<div class="empty-state">Temporada vazia.</div>';
      $('#series-episode-list').querySelectorAll('[data-episode-play]').forEach(button=>button.onclick=()=>{
        const ep=(season.episodes||[]).find(x=>Number(x.id)===Number(button.dataset.episodePlay));if(ep)playMedia(ep);
      });
    };
    section.querySelectorAll('[data-season-index]').forEach(button=>button.onclick=()=>render(Number(button.dataset.seasonIndex)));
    render(0);
  }

  function episodeHTML(ep){
    const image=ep.backdrop_url||ep.poster_url||'';
    const number=`E${String(ep.episode_number||0).padStart(2,'0')}`;
    const title=episodeDisplayTitle(ep.title,number);
    return `<article class="series-episode"><button class="episode-image" data-episode-play="${ep.id}" aria-label="Reproduzir ${escapeHTML(title)}">${image?`<img src="${escapeHTML(image)}" loading="lazy" alt="">`:'<span>▶</span>'}<i>▶</i></button><div class="episode-copy"><div><b>${number} · ${escapeHTML(title)}</b>${ep.runtime_minutes?`<span>${formatRuntime(ep.runtime_minutes)}</span>`:''}</div><p>${escapeHTML(ep.overview||'')}</p></div></article>`;
  }
  function episodeDisplayTitle(value,fallback){
    const text=String(value||'').replace(/^.*?S\d{1,2}E\d{1,3}\s*[·:\-]?\s*/i,'').trim();return text||fallback;
  }
  function firstEpisode(data){for(const season of data.seasons||[])if(season.episodes?.length)return season.episodes[0];return null}
  function stopThemeButton(){stopTheme();$('#theme-toggle').classList.add('hidden');$('#theme-info-wrap').classList.add('hidden')}

  closeDetail=function(){
    currentSeries=null;
    const section=$('#series-seasons-section');if(section)section.innerHTML='';
    baseCloseDetail();
  };
  $$('[data-close-detail]').forEach(x=>x.onclick=closeDetail);
})();
