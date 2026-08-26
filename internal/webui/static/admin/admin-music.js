/* StormFlix Admin: music library support without mixing it with the video catalog. */
(function(){
  const baseEdit=window.editLibrary;
  if(typeof baseEdit==='function'){
    window.editLibrary=function(id){
      const kind=id?(libs.find(x=>Number(x.id)===Number(id))?.kind||'movies'):'movies';
      baseEdit(id);
      const form=document.querySelector('#library-editor');
      const select=form?.querySelector('select');
      if(!select)return;
      if(![...select.options].some(o=>o.value==='music')){
        const option=document.createElement('option');option.value='music';option.textContent='Música';select.appendChild(option);
      }
      select.value=kind;
      const path=form.querySelector('input.wide');
      if(path&&kind==='music')path.placeholder='/media/Musicas';
      select.addEventListener('change',()=>{if(path)path.placeholder=select.value==='music'?'/media/Musicas':'/media/Filmes'});
    };
  }

  const baseLoadLibraries=window.loadLibraries;
  if(typeof baseLoadLibraries==='function'){
    window.loadLibraries=async function(){await baseLoadLibraries();decorateMusicLibraries()};
  }

  function decorateMusicLibraries(){
    const page=document.querySelector('#libraries');
    if(!page)return;
    page.querySelectorAll('[data-music-admin]').forEach(x=>x.remove());
    const music=(libs||[]).filter(l=>l.kind==='music');
    if(!music.length)return;
    const panel=document.createElement('div');panel.className='panel';panel.dataset.musicAdmin='1';
    panel.innerHTML=`<div class="panel-head"><div><h2>StormFlix Música</h2><p style="color:#8791a1;margin:5px 0 0">${music.length} biblioteca(s) de música · tags locais + MusicBrainz + Cover Art Archive + LRCLIB</p></div><button class="primary" id="music-index-now">Organizar metadados</button></div><p style="color:#8d97a6;line-height:1.6">O scan encontra os arquivos no Drive. Depois, a organização lê artista, álbum, faixa, gênero, ano, duração, codec e qualidade sem alterar os arquivos originais. Capas externas são associadas ao catálogo; letras são buscadas somente quando o usuário abre a letra.</p>`;
    page.appendChild(panel);
    panel.querySelector('#music-index-now').onclick=async()=>{try{const r=await req('/admin/music/index',{method:'POST',body:'{}'});notice(r.started?'Organização da biblioteca de música iniciada.':'A biblioteca de música já está sendo organizada.',true)}catch(err){notice(err.message)}};
  }

  const observer=new MutationObserver(()=>decorateMusicLibraries());
  const libraries=document.querySelector('#libraries');if(libraries)observer.observe(libraries,{childList:true});
  document.addEventListener('click',e=>{const button=e.target.closest('button[data-page="libraries"]');if(button)setTimeout(decorateMusicLibraries,100)});
  setTimeout(decorateMusicLibraries,800);
})();
