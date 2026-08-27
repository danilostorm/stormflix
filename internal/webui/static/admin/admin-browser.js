let folderTarget=null;

window.editLibrary=id=>{
  const l=libs.find(x=>Number(x.id)===Number(id))||{name:'',kind:'movies',path:'',paths:[],enabled:true};
  const initialPaths=(Array.isArray(l.paths)&&l.paths.length?l.paths:[l.path||'']).filter((v,i,a)=>v||a.length===1);
  $('#lib-form').innerHTML=`<form id="library-editor" class="library-editor-card">
    <div class="library-editor-head">
      <div><p class="kicker">${id?'Editar biblioteca':'Nova biblioteca'}</p><h2>${id?esc(l.name||'Biblioteca'):'Criar biblioteca'}</h2><p>Uma biblioteca pode reunir várias pastas/remotes como uma única coleção.</p></div>
      <button type="button" class="ghost library-editor-close" id="library-editor-close">Fechar</button>
    </div>
    <div class="library-main-fields">
      <label class="field"><span>Nome da biblioteca</span><input id="lib-name" value="${esc(l.name)}" placeholder="Ex.: Filmes Animação" required></label>
      <label class="field"><span>Tipo de conteúdo</span><select id="lib-kind"><option value="movies">Filmes</option><option value="series">Séries</option><option value="anime">Animes</option><option value="anime_series">Séries + Anime (temporadas)</option><option value="mixed">Filmes + Anime misto</option><option value="shows">Shows</option><option value="other">Outros</option><option value="music">Música</option></select></label>
      <label class="toggle-field"><input id="lib-enabled" type="checkbox" ${l.enabled?'checked':''}><span><b>Biblioteca ativa</b><small>Permite scan e exibição no catálogo.</small></span></label>
    </div>
    <div class="library-sources-block">
      <div class="library-sources-head"><div><h3>Origens da biblioteca</h3><p>Adicione um caminho para cada Drive/remote que pertence a esta mesma biblioteca.</p></div><button type="button" id="add-library-source">+ Adicionar origem</button></div>
      <div id="library-source-list" class="library-source-list"></div>
    </div>
    <div id="library-kind-hint" class="library-kind-hint"></div>
    <div class="library-editor-footer"><span class="muted">Os arquivos nos remotes nunca são movidos nem apagados ao salvar.</span><button class="primary" type="submit">Salvar biblioteca</button></div>
  </form>`;
  $('#lib-kind').value=l.kind||'movies';
  const sourceList=$('#library-source-list');
  const renderSources=paths=>{
    const values=paths.length?paths:[''];
    sourceList.innerHTML=values.map((path,index)=>`<div class="library-source-row" data-source-row>
      <div class="source-index"><b>${index+1}</b><span>Origem ${index+1}</span></div>
      <div class="source-path-wrap"><input data-source-path value="${esc(path)}" placeholder="/media/remote/Filmes Animação" readonly required><button type="button" data-browse-source>Selecionar pasta</button></div>
      <span class="source-state ${path&&libs.some(lib=>(lib.paths||[lib.path]).includes(path)&&lib.online)?'online':''}">${path?'Drive / pasta':'Não selecionada'}</span>
      <button type="button" class="danger source-remove" data-remove-source ${values.length===1?'disabled':''}>Remover</button>
    </div>`).join('');
    sourceList.querySelectorAll('[data-browse-source]').forEach(button=>button.onclick=()=>openFolderBrowser(button.closest('[data-source-row]').querySelector('[data-source-path]')));
    sourceList.querySelectorAll('[data-remove-source]').forEach(button=>button.onclick=()=>{
      const all=[...sourceList.querySelectorAll('[data-source-path]')].map(input=>input.value);
      const index=[...sourceList.querySelectorAll('[data-source-row]')].indexOf(button.closest('[data-source-row]'));
      all.splice(index,1);renderSources(all);
    });
  };
  renderSources(initialPaths);
  $('#add-library-source').onclick=()=>{
    const paths=[...sourceList.querySelectorAll('[data-source-path]')].map(input=>input.value);
    paths.push('');renderSources(paths);
    const last=[...sourceList.querySelectorAll('[data-source-path]')].at(-1);if(last)openFolderBrowser(last);
  };
  $('#library-editor-close').onclick=()=>{$('#lib-form').innerHTML=''};
  const updateHint=()=>{
    const kind=$('#lib-kind').value;
    const messages={mixed:'TMDB identifica filmes; AniList/AniDB/MyAnimeList complementam animes.',anime:'AniList é o agente principal; AniDB/MyAnimeList e AnimeAPI ajudam na identificação.',anime_series:'Para animes com temporadas: TMDB procura como série primeiro; AniDB, MyAnimeList, AnimeAPI e Fanart recuperam e enriquecem os títulos que faltarem.',series:'TMDB organiza séries, temporadas e episódios.',movies:'TMDB + Fanart.tv para filmes.',music:'FFprobe + tags locais + MusicBrainz + Cover Art Archive.'};
    $('#library-kind-hint').innerHTML=`<b>Estratégia:</b> ${messages[kind]||'Metadados conforme o tipo selecionado.'}`;
  };
  $('#lib-kind').addEventListener('change',updateHint);updateHint();
  const f=$('#library-editor');
  f.onsubmit=async e=>{
    e.preventDefault();
    const paths=[...sourceList.querySelectorAll('[data-source-path]')].map(input=>input.value.trim()).filter(Boolean);
    if(!paths.length){notice('Adicione pelo menos uma origem para a biblioteca.');return}
    const body={name:$('#lib-name').value.trim(),kind:$('#lib-kind').value,paths,path:paths[0],enabled:$('#lib-enabled').checked};
    try{
      await req(id?`/libraries/${id}`:'/libraries',{method:id?'PUT':'POST',body:JSON.stringify(body)});
      notice('Biblioteca salva com '+paths.length+' origem(ns).',true);
      $('#lib-form').innerHTML='';
      await loadLibraries();
    }catch(err){notice(err.message)}
  };
};

async function openFolderBrowser(target){
  folderTarget=target;
  let modal=$('#folder-browser');
  if(!modal){
    modal=document.createElement('div');
    modal.id='folder-browser';
    modal.className='folder-modal hidden';
    modal.innerHTML=`<div class="folder-card">
      <div class="folder-head"><div><p class="kicker">Armazenamento do servidor</p><h2>Selecionar pasta</h2></div><button type="button" id="folder-close">✕</button></div>
      <div class="folder-path" id="folder-current"></div>
      <div class="folder-toolbar"><button type="button" id="folder-up">← Voltar</button><button type="button" class="primary" id="folder-use">Usar esta pasta</button></div>
      <div class="folder-list" id="folder-list"></div>
    </div>`;
    document.body.appendChild(modal);
    $('#folder-close').onclick=closeFolderBrowser;
    modal.onclick=e=>{if(e.target===modal)closeFolderBrowser()};
  }
  modal.classList.remove('hidden');
  await browseFolder(target.value||'');
}

function closeFolderBrowser(){
  $('#folder-browser')?.classList.add('hidden');
  folderTarget=null;
}

async function browseFolder(path){
  const list=$('#folder-list');
  list.innerHTML='<div class="folder-empty">Carregando...</div>';
  try{
    const d=await req('/admin/filesystem'+(path?`?path=${encodeURIComponent(path)}`:''));
    $('#folder-current').textContent=d.current;
    $('#folder-up').disabled=!d.parent;
    $('#folder-up').onclick=()=>d.parent&&browseFolder(d.parent);
    $('#folder-use').onclick=()=>{
      if(folderTarget)folderTarget.value=d.current;
      closeFolderBrowser();
    };
    list.innerHTML=d.directories.length?d.directories.map(dir=>`<button type="button" class="folder-row" data-path="${esc(dir.path)}"><span class="folder-icon">📁</span><span>${esc(dir.name)}</span><span class="folder-arrow">›</span></button>`).join(''):'<div class="folder-empty">Nenhuma subpasta aqui.</div>';
    $$('#folder-list [data-path]').forEach(b=>b.onclick=()=>browseFolder(b.dataset.path));
  }catch(err){
    list.innerHTML=`<div class="folder-empty offline">${esc(err.message)}</div>`;
  }
}
