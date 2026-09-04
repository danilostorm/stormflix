/* StormFlix source: music-ui.js */
/* StormFlix Music: dedicated library UI and persistent Direct Play audio player. */
(function(){
  const root=document.querySelector('#music-view');
  const nav=document.querySelector('#music-nav');
  if(!root||!nav)return;

  let home=null,tracks=[],tab='home',query='',current=null,queue=[],queueIndex=-1;
  let audio=null,playerBar=null,lyricsPanel=null,startedSent=false,lastHeartbeatAt=0,heartbeatTimer=null,reloadTimer=null;

  function ensurePlayer(){
    if(audio)return;
    audio=document.createElement('audio');audio.id='music-audio';audio.preload='metadata';document.body.appendChild(audio);
    playerBar=document.createElement('section');playerBar.id='music-player';playerBar.className='music-player hidden';playerBar.setAttribute('aria-label','Player de música');
    playerBar.innerHTML=`<div class="music-now"><div class="music-now-cover" id="music-now-cover"><span>♪</span></div><div class="music-now-copy"><strong id="music-now-title">—</strong><small id="music-now-artist">—</small></div></div><div class="music-center"><div class="music-controls"><button class="music-control" id="music-prev" aria-label="Faixa anterior">◀</button><button class="music-control main" id="music-toggle" aria-label="Reproduzir">▶</button><button class="music-control" id="music-next" aria-label="Próxima faixa">▶</button></div><div class="music-progress-wrap"><span id="music-time">0:00</span><input id="music-progress" class="music-progress" type="range" min="0" max="1000" value="0" aria-label="Progresso"><span id="music-duration">0:00</span></div></div><div class="music-player-actions"><button class="music-control" id="music-like" aria-label="Favoritar">♡</button><button class="music-control" id="music-lyrics" aria-label="Letra">≡</button><button class="music-control" id="music-mute" aria-label="Silenciar">🔊</button><input id="music-volume" class="music-volume" type="range" min="0" max="1" step="0.02" value="1" aria-label="Volume"></div>`;
    document.body.appendChild(playerBar);
    lyricsPanel=document.createElement('aside');lyricsPanel.id='music-lyrics-panel';lyricsPanel.className='music-lyrics-panel hidden';document.body.appendChild(lyricsPanel);

    document.querySelector('#music-toggle').onclick=toggle;
    document.querySelector('#music-prev').onclick=()=>step(-1);
    document.querySelector('#music-next').onclick=()=>step(1);
    document.querySelector('#music-like').onclick=toggleFavorite;
    document.querySelector('#music-lyrics').onclick=showLyrics;
    document.querySelector('#music-mute').onclick=()=>{audio.muted=!audio.muted;syncVolume()};
    document.querySelector('#music-volume').oninput=e=>{audio.volume=Number(e.target.value);audio.muted=false;syncVolume()};
    document.querySelector('#music-progress').oninput=e=>{if(Number.isFinite(audio.duration)&&audio.duration>0)audio.currentTime=audio.duration*(Number(e.target.value)/1000)};
    audio.addEventListener('play',()=>{document.querySelector('#music-toggle').textContent='❚❚';if(window.player&&!player.paused)player.pause();else if(typeof player!=='undefined'&&!player.paused)player.pause();if(typeof stopTheme==='function')stopTheme();sendStarted();startHeartbeat();renderPlayingRows()});
    audio.addEventListener('pause',()=>{document.querySelector('#music-toggle').textContent='▶';stopHeartbeat();flushHeartbeat();renderPlayingRows()});
    audio.addEventListener('timeupdate',syncProgress);
    audio.addEventListener('durationchange',syncProgress);
    audio.addEventListener('volumechange',syncVolume);
    audio.addEventListener('ended',async()=>{await sendListening(0,false,true);step(1)});
    const video=document.querySelector('#player');if(video)video.addEventListener('play',()=>{if(audio&&!audio.paused)audio.pause()});
  }

  async function openMusic(){
    ensurePlayer();
    if(window.sfDiscardDetailPage)window.sfDiscardDetailPage();
    if(typeof stopTheme==='function')stopTheme();
    document.querySelector('#hero')?.classList.add('hidden');
    document.querySelector('#search-view')?.classList.add('hidden');
    document.querySelector('#catalog-view')?.classList.add('hidden');
    root.classList.remove('hidden');
    document.querySelectorAll('.main-nav button').forEach(b=>b.classList.toggle('active',b===nav));
    window.scrollTo({top:0,behavior:'auto'});
    await loadMusic();
  }

  function closeMusic(){
    root.classList.add('hidden');
    clearTimeout(reloadTimer);reloadTimer=null;
  }

  async function loadMusic(){
    root.innerHTML='<div class="music-shell"><div class="music-empty"><div class="music-empty-icon">♪</div><h2>Organizando sua música…</h2><p>Lendo a biblioteca e preparando artistas, álbuns e faixas.</p></div></div>';
    try{
      [home,tracks]=await Promise.all([request('/music/home'),request('/music/tracks?limit=5000')]);
      render();
      if(home?.indexing&&tracks.length===0){reloadTimer=setTimeout(async()=>{if(root.classList.contains('hidden'))return;try{[home,tracks]=await Promise.all([request('/music/home'),request('/music/tracks?limit=5000')]);render()}catch{}},3000)}
    }catch(err){root.innerHTML=`<div class="music-shell"><div class="music-empty"><div class="music-empty-icon">!</div><h2>Não foi possível abrir Música</h2><p>${escapeHTML(err.message)}</p></div></div>`}
  }

  function render(){
    const visible=filterTracks();
    root.innerHTML=`<div class="music-shell"><header class="music-hero"><div><p class="music-kicker">STORMFLIX MÚSICA</p><h1>Sua biblioteca sonora.</h1><p class="music-description">Álbuns, artistas e faixas do seu próprio Drive, organizados sem misturar com filmes e séries.</p>${home?.indexing?'<span class="music-indexing">Organizando metadados em segundo plano</span>':''}</div><label class="music-search"><input id="music-search-input" value="${escapeHTML(query)}" placeholder="Buscar música, artista ou álbum…" autocomplete="off"></label></header><nav class="music-tabs"><button class="music-tab ${tab==='home'?'active':''}" data-music-tab="home">Início</button><button class="music-tab ${tab==='albums'?'active':''}" data-music-tab="albums">Álbuns</button><button class="music-tab ${tab==='artists'?'active':''}" data-music-tab="artists">Artistas</button><button class="music-tab ${tab==='tracks'?'active':''}" data-music-tab="tracks">Músicas</button></nav><div id="music-content">${contentHTML(visible)}</div></div>`;
    root.querySelectorAll('[data-music-tab]').forEach(b=>b.onclick=()=>{tab=b.dataset.musicTab;render()});
    const search=root.querySelector('#music-search-input');if(search)search.oninput=e=>{query=e.target.value.trim();renderContentOnly()};
    bindContent();
  }

  function renderContentOnly(){
    const content=root.querySelector('#music-content');if(!content)return;content.innerHTML=contentHTML(filterTracks());bindContent();
  }

  function contentHTML(visible){
    if(!tracks.length)return `<div class="music-empty"><div class="music-empty-icon">♫</div><h2>Sua biblioteca Música está vazia</h2><p>No Admin, crie uma biblioteca com o tipo <b>Música</b>, selecione a pasta do seu Google Drive/rclone e execute o scan. MP3, FLAC, M4A, AAC, OGG, Opus, WAV, WMA, AIFF, APE e MKA podem ser catalogados.</p></div>`;
    if(query)return `<section class="music-section"><div class="music-section-head"><h2>Resultados</h2><span>${visible.length} faixas</span></div>${trackList(visible)}</section>`;
    if(tab==='albums')return albumGrid(buildAlbums(visible));
    if(tab==='artists')return artistGrid(buildArtists(visible));
    if(tab==='tracks')return `<section class="music-section"><div class="music-section-head"><h2>Todas as músicas</h2><span>${visible.length} faixas</span></div>${trackList(visible)}</section>`;
    const sections=[];
    if(home?.recently_played?.length)sections.push(musicSection('Tocadas recentemente','Continue de onde parou',trackRail(home.recently_played)));
    if(home?.most_played?.length)sections.push(musicSection('Mais ouvidas nesta semana','O que está tocando no StormFlix',trackRail(home.most_played)));
    const albums=(home?.recently_added_albums?.length?home.recently_added_albums:buildAlbums(visible).slice(0,24));
    if(albums.length)sections.push(musicSection('Álbuns adicionados recentemente',`${albums.length} álbuns`,albumRail(albums)));
    const artists=(home?.artists?.length?home.artists:buildArtists(visible).slice(0,24));
    if(artists.length)sections.push(musicSection('Artistas','Explore sua coleção',artistRail(artists)));
    const recent=home?.recently_added_tracks?.length?home.recently_added_tracks:visible.slice(0,20);
    if(recent.length)sections.push(musicSection('Faixas recentes',`${recent.length} músicas`,trackList(recent.slice(0,20))));
    return sections.join('');
  }

  function musicSection(title,sub,body){return `<section class="music-section"><div class="music-section-head"><h2>${escapeHTML(title)}</h2><span>${escapeHTML(sub)}</span></div>${body}</section>`}

  function albumRail(albums){return `<div class="music-album-rail">${albums.map(albumCard).join('')}</div>`}
  function albumGrid(albums){return `<section class="music-section"><div class="music-section-head"><h2>Álbuns</h2><span>${albums.length} álbuns</span></div><div class="music-album-grid">${albums.map(albumCard).join('')}</div></section>`}
  function albumCard(a){const key=a.key||albumKey(a.artist,a.title);return `<button class="music-album-card" data-album="${escapeAttr(key)}"><span class="music-cover">${cover(a.cover_url)}<i class="music-cover-play">▶</i></span><strong>${escapeHTML(a.title||'Singles')}</strong><small>${escapeHTML(a.artist||'Artista desconhecido')}${a.year?` · ${a.year}`:''}</small></button>`}

  function artistRail(artists){return `<div class="music-artist-rail">${artists.map(artistCard).join('')}</div>`}
  function artistGrid(artists){return `<section class="music-section"><div class="music-section-head"><h2>Artistas</h2><span>${artists.length} artistas</span></div><div class="music-artist-rail" style="flex-wrap:wrap;overflow:visible">${artists.map(artistCard).join('')}</div></section>`}
  function artistCard(a){return `<button class="music-artist-card" data-artist="${escapeAttr(a.name)}"><span class="music-artist-avatar">${escapeHTML((a.name||'?').trim().charAt(0).toUpperCase())}</span><strong>${escapeHTML(a.name)}</strong><small>${a.album_count||0} álbuns · ${a.track_count||0} faixas</small></button>`}

  function trackRail(items){return trackList(items.slice(0,14))}
  function trackList(items){return `<div class="music-track-list">${items.map((t,i)=>trackRow(t,i)).join('')}</div>`}
  function trackRow(t,i){const tech=[t.codec?String(t.codec).toUpperCase():'',t.bitrate?`${Math.round(t.bitrate/1000)} kbps`:''].filter(Boolean).join(' · ');return `<div class="music-track-row ${current&&Number(current.id)===Number(t.id)&&audio&&!audio.paused?'playing':''}" data-track-row="${t.id}" tabindex="0"><span class="music-track-index">${t.track_number||i+1}</span><span class="music-track-title"><strong>${escapeHTML(t.title)}</strong><small>${escapeHTML(t.artist||'Artista desconhecido')}</small></span><span class="music-track-album">${escapeHTML(t.album||'Singles')}</span><span class="music-track-tech">${escapeHTML(tech)}</span><span class="music-track-duration">${clock(t.duration_seconds)}</span><button class="music-favorite ${t.favorite?'on':''}" data-favorite="${t.id}" aria-label="Favoritar">${t.favorite?'♥':'♡'}</button></div>`}

  function bindContent(){
    bindCoverErrors(root);
    root.querySelectorAll('[data-track-row]').forEach(row=>{row.onclick=e=>{if(e.target.closest('[data-favorite]'))return;const t=trackByID(+row.dataset.trackRow);if(t)playTrack(t,filterTracks())};row.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();const t=trackByID(+row.dataset.trackRow);if(t)playTrack(t,filterTracks())}}});
    root.querySelectorAll('[data-favorite]').forEach(b=>b.onclick=async e=>{e.stopPropagation();const t=trackByID(+b.dataset.favorite);if(t)await favoriteTrack(t)});
    root.querySelectorAll('[data-album]').forEach(b=>b.onclick=e=>{const key=b.dataset.album;if(e.target.closest('.music-cover-play')){const list=tracksForAlbum(key);if(list[0])playTrack(list[0],list);return}showAlbum(key)});
    root.querySelectorAll('[data-artist]').forEach(b=>b.onclick=()=>showArtist(b.dataset.artist));
  }

  function showAlbum(key){tab='album-detail';const list=tracksForAlbum(key);if(!list.length)return;const a=buildAlbums(list)[0];const content=root.querySelector('#music-content');content.innerHTML=`${detailHero('ÁLBUM',a.title,a.artist,a.cover_url,`${a.year||''}${a.year?' · ':''}${a.track_count} faixas · ${clockLong(a.duration_seconds)}`,key,'album')}${trackList(list)}`;bindContent();content.scrollIntoView({behavior:'smooth',block:'start'})}
  function showArtist(name){tab='artist-detail';const list=tracks.filter(t=>same(t.artist,name)||same(t.album_artist,name));const albums=buildAlbums(list);const content=root.querySelector('#music-content');content.innerHTML=`${detailHero('ARTISTA',name,`${albums.length} álbuns`,albums[0]?.cover_url||'',`${list.length} faixas`,name,'artist')}${albumRail(albums)}<section class="music-section" style="margin-top:28px"><div class="music-section-head"><h2>Músicas</h2><span>${list.length} faixas</span></div>${trackList(list)}</section>`;bindContent();content.scrollIntoView({behavior:'smooth',block:'start'})}
  function detailHero(kind,title,subtitle,coverURL,meta,key,type){return `<section class="music-detail"><div class="music-cover">${cover(coverURL)}</div><div class="music-detail-content"><p>${kind}</p><h2>${escapeHTML(title)}</h2><h3>${escapeHTML(subtitle)}</h3><div class="music-detail-meta">${escapeHTML(meta)}</div><div class="music-detail-actions"><button class="music-round-play" data-detail-play="${escapeAttr(type+':'+key)}">▶</button><button class="music-back" data-music-back>← Voltar</button></div></div></section>`}

  root.addEventListener('click',e=>{const back=e.target.closest('[data-music-back]');if(back){tab='home';render();return}const play=e.target.closest('[data-detail-play]');if(play){const value=play.dataset.detailPlay;const split=value.indexOf(':');const type=value.slice(0,split),key=value.slice(split+1);const list=type==='album'?tracksForAlbum(key):tracks.filter(t=>same(t.artist,key)||same(t.album_artist,key));if(list[0])playTrack(list[0],list)}});

  function filterTracks(){if(!query)return tracks;const q=query.toLocaleLowerCase('pt-BR');return tracks.filter(t=>[t.title,t.artist,t.album_artist,t.album,t.genre].some(v=>String(v||'').toLocaleLowerCase('pt-BR').includes(q)))}
  function trackByID(id){return tracks.find(t=>Number(t.id)===Number(id))}
  function tracksForAlbum(key){return tracks.filter(t=>albumKey(t.album_artist||t.artist,t.album)===key).sort((a,b)=>(a.disc_number-b.disc_number)||(a.track_number-b.track_number)||(a.title||'').localeCompare(b.title||''))}

  function buildAlbums(list){const map=new Map();for(const t of list){const key=albumKey(t.album_artist||t.artist,t.album);let a=map.get(key);if(!a){a={key,title:t.album||'Singles',artist:t.album_artist||t.artist||'Artista desconhecido',year:t.year||0,track_count:0,duration_seconds:0,cover_url:t.cover_url||'',modified_unix:t.modified_unix||0,representative_track_id:t.id};map.set(key,a)}a.track_count++;a.duration_seconds+=Number(t.duration_seconds)||0;if(!a.cover_url&&t.cover_url)a.cover_url=t.cover_url;if((t.modified_unix||0)>a.modified_unix){a.modified_unix=t.modified_unix;a.representative_track_id=t.id}}return [...map.values()].sort((a,b)=>(b.modified_unix-a.modified_unix)||a.title.localeCompare(b.title))}
  function buildArtists(list){const map=new Map();for(const t of list){const name=t.album_artist||t.artist||'Artista desconhecido';let a=map.get(name.toLocaleLowerCase('pt-BR'));if(!a){a={name,albums:new Set(),track_count:0};map.set(name.toLocaleLowerCase('pt-BR'),a)}a.track_count++;a.albums.add(albumKey(name,t.album))}return [...map.values()].map(a=>({name:a.name,album_count:a.albums.size,track_count:a.track_count})).sort((a,b)=>(b.track_count-a.track_count)||a.name.localeCompare(b.name))}
  function albumKey(artist,album){return norm(artist)+'|'+norm(album)}function norm(v){return String(v||'').trim().replace(/\s+/g,' ').toLowerCase()}
  function same(a,b){return norm(a)===norm(b)}

  async function playTrack(track,list){
    ensurePlayer();current=track;queue=(list&&list.length?list:tracks).slice();queueIndex=queue.findIndex(t=>Number(t.id)===Number(track.id));if(queueIndex<0){queue=[track];queueIndex=0}
    startedSent=false;lastHeartbeatAt=performance.now();
    audio.src=`${api}/music/tracks/${track.id}/stream`;audio.load();
    syncCurrent();playerBar.classList.remove('hidden');
    await audio.play().catch(()=>{});
  }
  function toggle(){if(!current)return;if(audio.paused)audio.play().catch(()=>{});else audio.pause()}
  function step(dir){if(!queue.length)return;let next=queueIndex+dir;if(next<0)next=queue.length-1;if(next>=queue.length)next=0;playTrack(queue[next],queue)}
  function syncCurrent(){if(!current)return;document.querySelector('#music-now-title').textContent=current.title||'—';document.querySelector('#music-now-artist').textContent=[current.artist,current.album].filter(Boolean).join(' · ');const c=document.querySelector('#music-now-cover');c.innerHTML=current.cover_url?`<img src="${escapeHTML(current.cover_url)}" alt=""><span>♪</span>`:'<span>♪</span>';bindCoverErrors(c);document.querySelector('#music-like').textContent=current.favorite?'♥':'♡';document.querySelector('#music-like').classList.toggle('on',!!current.favorite);syncProgress()}
  function syncProgress(){if(!audio)return;const d=Number.isFinite(audio.duration)?audio.duration:Number(current?.duration_seconds)||0,p=Number.isFinite(audio.currentTime)?audio.currentTime:0;const range=document.querySelector('#music-progress');if(range)range.value=d?Math.round(p/d*1000):0;if(document.querySelector('#music-time'))document.querySelector('#music-time').textContent=clock(p);if(document.querySelector('#music-duration'))document.querySelector('#music-duration').textContent=clock(d)}
  function syncVolume(){if(!audio)return;document.querySelector('#music-volume').value=audio.muted?0:audio.volume;document.querySelector('#music-mute').textContent=audio.muted||audio.volume===0?'🔇':audio.volume<.5?'🔉':'🔊'}
  function renderPlayingRows(){root.querySelectorAll('[data-track-row]').forEach(r=>r.classList.toggle('playing',current&&!audio.paused&&Number(r.dataset.trackRow)===Number(current.id)))}

  async function favoriteTrack(track){try{const data=await request(`/music/tracks/${track.id}/favorite`,{method:'POST',body:'{}'});track.favorite=!!data.favorite;if(current&&Number(current.id)===Number(track.id))current.favorite=track.favorite;renderContentOnly();syncCurrent()}catch{}}
  async function toggleFavorite(){if(current)await favoriteTrack(current)}

  async function showLyrics(){if(!current)return;lyricsPanel.classList.remove('hidden');lyricsPanel.innerHTML=`<div class="music-lyrics-head"><div><p>LETRA</p><h3>${escapeHTML(current.title)}</h3></div><button id="music-lyrics-close">✕</button></div><div class="music-lyrics-body">Buscando letra…</div>`;lyricsPanel.querySelector('#music-lyrics-close').onclick=()=>lyricsPanel.classList.add('hidden');try{const data=await request(`/music/tracks/${current.id}/lyrics`);let text=data.plain_lyrics||stripLRC(data.synced_lyrics||'');if(data.instrumental)text='Faixa instrumental.';if(!text)text='Letra não encontrada para esta faixa.';lyricsPanel.querySelector('.music-lyrics-body').textContent=text;lyricsPanel.insertAdjacentHTML('beforeend',`<div class="music-lyrics-provider">${data.provider?'Fonte: '+escapeHTML(String(data.provider).toUpperCase()):'LRCLIB'}</div>`)}catch(err){lyricsPanel.querySelector('.music-lyrics-body').textContent='Não foi possível carregar a letra: '+err.message}}
  function stripLRC(value){return String(value||'').split('\n').map(line=>line.replace(/^(?:\[[^\]]+\])+\s*/,'')).join('\n').trim()}

  function sendStarted(){if(startedSent||!current)return;startedSent=true;sendListening(0,true,false)}
  function startHeartbeat(){stopHeartbeat();lastHeartbeatAt=performance.now();heartbeatTimer=setInterval(flushHeartbeat,30000)}
  function stopHeartbeat(){clearInterval(heartbeatTimer);heartbeatTimer=null}
  function flushHeartbeat(){if(!current||!audio||audio.paused)return;const now=performance.now();const delta=Math.max(0,Math.min(60,(now-lastHeartbeatAt)/1000));lastHeartbeatAt=now;if(delta>=1)sendListening(delta,false,false)}
  async function sendListening(delta,started,completed){if(!current)return;try{await request(`/music/tracks/${current.id}/listening`,{method:'POST',body:JSON.stringify({delta_seconds:delta,started,completed})})}catch{}}

  function cover(url){return `<span class="music-cover-fallback">♪</span>${url?`<img data-music-cover src="${escapeHTML(url)}" alt="" loading="lazy">`:''}`}
  function bindCoverErrors(scope){scope.querySelectorAll?.('img[data-music-cover],#music-now-cover img').forEach(img=>img.addEventListener('error',()=>img.remove(),{once:true}))}
  function clock(seconds){seconds=Math.max(0,Math.floor(Number(seconds)||0));const m=Math.floor(seconds/60),s=seconds%60;return`${m}:${String(s).padStart(2,'0')}`}
  function clockLong(seconds){seconds=Math.max(0,Math.floor(Number(seconds)||0));const h=Math.floor(seconds/3600),m=Math.floor((seconds%3600)/60);return h?`${h}h ${m}min`:`${m}min`}
  function escapeAttr(v){return escapeHTML(String(v||'')).replace(/`/g,'&#96;')}

  nav.onclick=openMusic;
  document.querySelector('.main-nav')?.addEventListener('click',e=>{const button=e.target.closest('button');if(button&&button!==nav)closeMusic()});
  document.querySelector('#brand-home')?.addEventListener('click',closeMusic,true);
  document.querySelector('#search-toggle')?.addEventListener('click',closeMusic,true);
  window.addEventListener('stormflix:profile',()=>{if(!root.classList.contains('hidden'))loadMusic()});
  window.sfMusic={open:openMusic,close:closeMusic,play:playTrack};
})();
