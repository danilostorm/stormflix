/* StormFlix UX v2 */
let sfCurrentMedia=null,sfVersions=[],sfSubtitles=[],sfHideTimer=null,sfToastTimer=null,sfSettingsOpen=false;
const sfModal=document.querySelector('#player-modal');

function sfDecorateRows(root=document){
  root.querySelectorAll('.content-row').forEach(section=>{
    if(section.dataset.sfRail==='1')return;
    section.dataset.sfRail='1';
    const track=section.querySelector('.row-track');
    if(!track)return;
    const left=document.createElement('button'),right=document.createElement('button');
    left.className='row-nav left';right.className='row-nav right';
    left.setAttribute('aria-label','Ver títulos anteriores');right.setAttribute('aria-label','Ver mais títulos');
    left.innerHTML='<span>‹</span>';right.innerHTML='<span>›</span>';
    section.append(left,right);
    const move=dir=>track.scrollBy({left:dir*Math.max(320,track.clientWidth*.82),behavior:'smooth'});
    left.onclick=()=>move(-1);right.onclick=()=>move(1);
    const sync=()=>{
      left.disabled=track.scrollLeft<8;
      right.disabled=track.scrollLeft+track.clientWidth>=track.scrollWidth-8;
    };
    track.addEventListener('scroll',sync,{passive:true});
    window.addEventListener('resize',sync,{passive:true});
    setTimeout(sync,40);
  });
}

const sfRowsObserver=new MutationObserver(()=>sfDecorateRows(document));
const sfRowsRoot=document.querySelector('#rows');
if(sfRowsRoot)sfRowsObserver.observe(sfRowsRoot,{childList:true,subtree:true});
sfDecorateRows(document);

function sfBuildPlayer(){
  if(!sfModal||sfModal.dataset.sfReady==='1')return;
  sfModal.dataset.sfReady='1';
  sfModal.classList.add('sf-player-ready');
  player.controls=false;
  const ui=document.createElement('div');
  ui.className='sf-player-ui';
  ui.innerHTML=`
    <div class="sf-player-center">
      <button class="sf-skip" id="sf-back10" aria-label="Voltar 10 segundos">↶10</button>
      <button class="sf-center-play" id="sf-center-play" aria-label="Reproduzir ou pausar">▶</button>
      <button class="sf-skip" id="sf-forward10" aria-label="Avançar 10 segundos">10↷</button>
    </div>
    <div class="sf-player-controls">
      <div class="sf-progress-wrap"><span id="sf-time">0:00</span><input id="sf-progress" class="sf-progress" type="range" min="0" max="1000" value="0"><span id="sf-duration">0:00</span></div>
      <div class="sf-controls-row">
        <button class="sf-control-btn" id="sf-play" aria-label="Reproduzir ou pausar">▶</button>
        <button class="sf-control-btn" id="sf-back" aria-label="Voltar 10 segundos">↶10</button>
        <button class="sf-control-btn" id="sf-forward" aria-label="Avançar 10 segundos">10↷</button>
        <button class="sf-control-btn" id="sf-mute" aria-label="Mudo">🔊</button>
        <input id="sf-volume" class="sf-volume" type="range" min="0" max="1" step="0.02" value="1" aria-label="Volume">
        <div class="sf-control-spacer"></div>
        <button class="sf-control-btn" id="sf-subtitle" aria-label="Legendas">CC</button>
        <button class="sf-control-btn" id="sf-settings" aria-label="Qualidade áudio e legendas">⚙</button>
        <button class="sf-control-btn" id="sf-fullscreen" aria-label="Tela cheia">⛶</button>
      </div>
    </div>
    <div id="sf-player-settings-panel" class="sf-player-settings hidden"></div>
    <div id="sf-player-toast" class="sf-player-toast"></div>`;
  sfModal.appendChild(ui);

  $('#sf-play').onclick=sfTogglePlay;$('#sf-center-play').onclick=sfTogglePlay;
  $('#sf-back').onclick=$('#sf-back10').onclick=()=>sfSeekBy(-10);
  $('#sf-forward').onclick=$('#sf-forward10').onclick=()=>sfSeekBy(10);
  $('#sf-mute').onclick=sfToggleMute;
  $('#sf-volume').oninput=e=>{player.volume=Number(e.target.value);player.muted=false;sfSyncVolume()};
  $('#sf-progress').oninput=e=>{if(Number.isFinite(player.duration)&&player.duration>0)player.currentTime=player.duration*(Number(e.target.value)/1000)};
  $('#sf-settings').onclick=()=>sfToggleSettings();
  $('#sf-subtitle').onclick=sfCycleSubtitles;
  $('#sf-fullscreen').onclick=sfToggleFullscreen;

  player.addEventListener('play',sfSyncPlay);player.addEventListener('pause',sfSyncPlay);
  player.addEventListener('timeupdate',sfSyncProgress);player.addEventListener('durationchange',sfSyncProgress);
  player.addEventListener('volumechange',sfSyncVolume);
  player.addEventListener('loadedmetadata',()=>{sfSyncProgress();sfRenderSettings()});
  player.addEventListener('click',sfTogglePlay);
  sfModal.addEventListener('mousemove',sfShowControls,{passive:true});
  sfModal.addEventListener('touchstart',sfShowControls,{passive:true});
  sfShowControls();
}

function sfTogglePlay(){if(player.paused)player.play().catch(()=>{});else player.pause()}
function sfSyncPlay(){
  const icon=player.paused?'▶':'❚❚';
  $('#sf-play').textContent=icon;$('#sf-center-play').textContent=icon;
  sfShowControls();
}
function sfSeekBy(seconds){
  if(!Number.isFinite(player.duration))return;
  player.currentTime=Math.max(0,Math.min(player.duration,player.currentTime+seconds));
  sfToast(`${seconds>0?'+':''}${seconds}s`);
}
function sfSyncProgress(){
  const duration=Number.isFinite(player.duration)?player.duration:0,current=Number.isFinite(player.currentTime)?player.currentTime:0;
  $('#sf-progress').value=duration?Math.round(current/duration*1000):0;
  $('#sf-time').textContent=sfClock(current);$('#sf-duration').textContent=sfClock(duration);
}
function sfToggleMute(){player.muted=!player.muted;sfSyncVolume()}
function sfSyncVolume(){
  $('#sf-volume').value=player.muted?0:player.volume;
  $('#sf-mute').textContent=player.muted||player.volume===0?'🔇':player.volume<.5?'🔉':'🔊';
}
function sfClock(seconds){
  seconds=Math.max(0,Math.floor(seconds||0));const h=Math.floor(seconds/3600),m=Math.floor(seconds%3600/60),s=seconds%60;
  return h?`${h}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`:`${m}:${String(s).padStart(2,'0')}`;
}
function sfShowControls(){
  sfModal.classList.remove('sf-controls-hidden');clearTimeout(sfHideTimer);
  if(!player.paused&&!sfSettingsOpen)sfHideTimer=setTimeout(()=>sfModal.classList.add('sf-controls-hidden'),2600);
}
function sfToast(text){
  const el=$('#sf-player-toast');el.textContent=text;el.classList.add('show');clearTimeout(sfToastTimer);sfToastTimer=setTimeout(()=>el.classList.remove('show'),900);
}
async function sfToggleFullscreen(){
  try{
    if(document.fullscreenElement)await document.exitFullscreen();
    else if(sfModal.requestFullscreen)await sfModal.requestFullscreen();
    else if(player.webkitEnterFullscreen)player.webkitEnterFullscreen();
  }catch{}
}

function sfToggleSettings(force){
  const panel=$('#sf-player-settings-panel');
  sfSettingsOpen=typeof force==='boolean'?force:panel.classList.contains('hidden');
  panel.classList.toggle('hidden',!sfSettingsOpen);
  sfRenderSettings();sfShowControls();
}

function sfRenderSettings(){
  const panel=$('#sf-player-settings-panel');if(!panel)return;
  const currentID=sfCurrentMedia?.id;
  const versionHTML=(sfVersions.length?sfVersions:[{id:currentID,label:'Original',extension:sfCurrentMedia?.extension||'',size_bytes:sfCurrentMedia?.size_bytes||0}]).map(v=>`<button class="sf-setting-option ${Number(v.id)===Number(currentID)?'active':''}" data-sf-version="${v.id}"><span>${escapeHTML(v.label||'Original')} · ${escapeHTML(String(v.extension||'').replace('.','').toUpperCase())}</span><small>${sfFormatBytes(v.size_bytes)}</small></button>`).join('');
  const subtitleHTML=['<button class="sf-setting-option '+(sfActiveSubtitleID()===0?'active':'')+'" data-sf-sub="0"><span>Desativada</span></button>'].concat(sfSubtitles.map(s=>`<button class="sf-setting-option ${sfActiveSubtitleID()===Number(s.id)?'active':''}" data-sf-sub="${s.id}"><span>${escapeHTML(sfLanguage(s.language))}${s.hearing_impaired?' · SDH':''}</span><small>${escapeHTML(s.provider||'')}</small></button>`)).join('');
  const audio=sfAudioTracks();
  const audioHTML=audio.length?audio.map((t,i)=>`<button class="sf-setting-option ${t.enabled?'active':''}" data-sf-audio="${i}"><span>${escapeHTML(t.label||sfLanguage(t.language)||`Faixa ${i+1}`)}</span><small>${escapeHTML(t.language||'')}</small></button>`).join(''):`<div class="sf-setting-note">O navegador não expôs as faixas de áudio deste arquivo. Em MKV dual-audio isso depende do navegador; os apps Desktop/Android poderão selecionar todas as trilhas nativamente.</div>`;
  panel.innerHTML=`<section class="sf-setting-section"><h3>Qualidade · Direct Play</h3>${versionHTML}<div class="sf-setting-note">Só aparecem versões reais existentes no catálogo. StormFlix não cria 1080p/720p por transcodificação.</div></section><section class="sf-setting-section"><h3>Áudio</h3>${audioHTML}</section><section class="sf-setting-section"><h3>Legendas</h3>${subtitleHTML}</section>`;
  panel.querySelectorAll('[data-sf-version]').forEach(b=>b.onclick=()=>sfSelectVersion(+b.dataset.sfVersion));
  panel.querySelectorAll('[data-sf-sub]').forEach(b=>b.onclick=()=>sfSelectSubtitle(+b.dataset.sfSub));
  panel.querySelectorAll('[data-sf-audio]').forEach(b=>b.onclick=()=>sfSelectAudio(+b.dataset.sfAudio));
}

async function sfSelectVersion(id){
  if(!id||Number(id)===Number(sfCurrentMedia?.id))return;
  const version=sfVersions.find(v=>Number(v.id)===Number(id));
  const oldTime=player.currentTime||0,wasPlaying=!player.paused;
  sfCurrentMedia={...sfCurrentMedia,...version,id};
  player.src=`${api}/media/${id}/stream`;player.load();
  await sfLoadPlayerOptions(id);
  player.addEventListener('loadedmetadata',function restore(){player.removeEventListener('loadedmetadata',restore);if(oldTime&&oldTime<player.duration)player.currentTime=oldTime;if(wasPlaying)player.play().catch(()=>{})},{once:false});
  sfToast(`${version?.label||'Versão'} · Direct Play`);sfRenderSettings();
}

function sfAudioTracks(){try{return player.audioTracks?[...player.audioTracks]:[]}catch{return[]}}
function sfSelectAudio(index){const tracks=sfAudioTracks();tracks.forEach((track,i)=>track.enabled=i===index);sfRenderSettings();sfToast(tracks[index]?.label||`Áudio ${index+1}`)}

function sfActiveSubtitleID(){
  if(window.sfLocalOrigin?.isActive?.())return Number(window.sfLocalSubtitleID||0);
  const tracks=[...player.querySelectorAll('track[data-subtitle-id]')];
  const active=tracks.find(t=>t.track?.mode==='showing');return active?Number(active.dataset.subtitleId):0;
}
function sfSelectSubtitle(id){
  if(window.sfLocalOrigin?.isActive?.()){window.sfLocalOrigin.selectSubtitle(id).catch(()=>sfToast('Não foi possível trocar a legenda'));sfRenderSettings();sfToast(id?'Legendas ativadas':'Legendas desativadas');return}
  [...player.textTracks].forEach(t=>t.mode='disabled');
  if(id){const el=player.querySelector(`track[data-subtitle-id="${id}"]`);if(el?.track)el.track.mode='showing'}
  sfRenderSettings();sfToast(id?'Legendas ativadas':'Legendas desativadas');
}
function sfCycleSubtitles(){
  const ids=[0,...sfSubtitles.map(s=>Number(s.id))];const current=sfActiveSubtitleID();const next=ids[(Math.max(0,ids.indexOf(current))+1)%ids.length];sfSelectSubtitle(next);
}
function sfLanguage(code){
  const c=String(code||'').toLowerCase();const names={'pt-br':'Português (Brasil)','pt':'Português','en':'English','es':'Español','fr':'Français','de':'Deutsch','it':'Italiano','ja':'日本語'};return names[c]||String(code||'Legenda');
}

async function sfLoadPlayerOptions(id){
  try{sfVersions=await request(`/media/${id}/versions`)}catch{sfVersions=[]}
  try{sfSubtitles=await request(`/media/${id}/subtitles`)}catch{sfSubtitles=[]}
  player.querySelectorAll('track[data-subtitle-id]').forEach(t=>t.remove());
  sfSubtitles.forEach((sub,index)=>{
    const track=document.createElement('track');track.kind='subtitles';track.label=sfLanguage(sub.language);track.srclang=String(sub.language||'').slice(0,2)||'pt';track.src=`${api}/media/${id}/subtitles/${sub.id}/vtt`;track.dataset.subtitleId=sub.id;if(index===0)track.default=false;player.appendChild(track);
  });
  sfRenderSettings();
}

function sfFormatBytes(bytes){
  if(!bytes)return'';const units=['B','KB','MB','GB','TB'];let i=0,n=Number(bytes);while(n>=1024&&i<units.length-1){n/=1024;i++}return`${n.toFixed(i>=3?1:0)} ${units[i]}`;
}

const sfOriginalPlayMedia=playMedia;
playMedia=async function(item){
  stopTheme();sfBuildPlayer();sfCurrentMedia={...item};
  $('#player-title').textContent=item.title||'StormFlix';$('#player-help').classList.add('hidden');sfModal.classList.remove('hidden');sfModal.classList.remove('sf-controls-hidden');
  player.src=`${api}/media/${item.id}/stream`;player.load();
  await sfLoadPlayerOptions(item.id);
  player.play().catch(()=>{});sfShowControls();
};

const sfOriginalClosePlayer=closePlayer;
closePlayer=function(){
  clearTimeout(sfHideTimer);sfToggleSettings(false);sfCurrentMedia=null;sfVersions=[];sfSubtitles=[];
  player.pause();player.querySelectorAll('track[data-subtitle-id]').forEach(t=>t.remove());player.removeAttribute('src');player.load();sfModal.classList.add('hidden');sfModal.classList.remove('sf-controls-hidden');
};
$('#player-close').onclick=closePlayer;
sfBuildPlayer();

function sfFocusableCards(){return [...document.querySelectorAll('.media-tile:not(.hidden)')].filter(el=>el.offsetParent!==null)}
function sfMoveCardFocus(current,key){
  const cards=sfFocusableCards(),index=cards.indexOf(current);if(index<0)return false;
  if(key==='ArrowRight'||key==='ArrowLeft'){
    const next=cards[index+(key==='ArrowRight'?1:-1)];if(next){next.focus({preventScroll:true});next.scrollIntoView({behavior:'smooth',block:'nearest',inline:'center'});return true}
  }
  if(key==='ArrowDown'||key==='ArrowUp'){
    const row=current.closest('.content-row'),rows=[...document.querySelectorAll('.content-row')].filter(x=>x.offsetParent!==null),ri=rows.indexOf(row),targetRow=rows[ri+(key==='ArrowDown'?1:-1)];
    if(targetRow){const rowCards=[...row.querySelectorAll('.media-tile')],ci=Math.max(0,rowCards.indexOf(current)),targets=[...targetRow.querySelectorAll('.media-tile')];const target=targets[Math.min(ci,targets.length-1)];if(target){target.focus({preventScroll:true});target.scrollIntoView({behavior:'smooth',block:'center',inline:'center'});return true}}
  }
  return false;
}

document.addEventListener('keydown',e=>{
  const playerOpen=!sfModal.classList.contains('hidden');
  if(playerOpen){
    if(['INPUT','SELECT','TEXTAREA'].includes(e.target.tagName))return;
    const keys=[' ','k','K','ArrowLeft','ArrowRight','ArrowUp','ArrowDown','m','M','f','F','c','C','Escape'];if(keys.includes(e.key))e.preventDefault();
    if(e.key===' '||e.key.toLowerCase()==='k')sfTogglePlay();
    else if(e.key==='ArrowLeft')sfSeekBy(-10);else if(e.key==='ArrowRight')sfSeekBy(10);
    else if(e.key==='ArrowUp'){player.volume=Math.min(1,player.volume+.05);player.muted=false;sfToast(`Volume ${Math.round(player.volume*100)}%`)}
    else if(e.key==='ArrowDown'){player.volume=Math.max(0,player.volume-.05);sfToast(`Volume ${Math.round(player.volume*100)}%`)}
    else if(e.key.toLowerCase()==='m')sfToggleMute();else if(e.key.toLowerCase()==='f')sfToggleFullscreen();else if(e.key.toLowerCase()==='c')sfCycleSubtitles();
    return;
  }
  const card=e.target.closest?.('.media-tile');
  if(card&&['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key)){if(sfMoveCardFocus(card,e.key))e.preventDefault()}
  if(card&&e.key===' '){e.preventDefault();openDetail(+card.dataset.media)}
},true);

window.addEventListener('gamepadconnected',()=>console.info('StormFlix: gamepad/TV remote detected; keyboard navigation remains active.'));
