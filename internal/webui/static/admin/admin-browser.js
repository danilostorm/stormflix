let folderTarget=null;

window.editLibrary=id=>{
  const l=libs.find(x=>x.id===id)||{name:'',kind:'movies',path:'',enabled:true};
  $('#lib-form').innerHTML=`<form class="form" id="library-editor">
    <input id="lib-name" value="${esc(l.name)}" placeholder="Nome" required>
    <select id="lib-kind"><option value="movies">Filmes</option><option value="series">Séries</option><option value="anime">Animes</option><option value="shows">Shows</option><option value="other">Outros</option></select>
    <div class="wide path-picker"><input id="lib-path" value="${esc(l.path)}" placeholder="/media/Filmes" required readonly><button type="button" id="browse-path">Selecionar pasta</button></div>
    <label><input id="lib-enabled" type="checkbox" ${l.enabled?'checked':''}> Ativa</label>
    <button class="primary">Salvar</button>
  </form>`;
  $('#lib-kind').value=l.kind;
  $('#browse-path').onclick=()=>openFolderBrowser($('#lib-path'));
  const f=$('#library-editor');
  f.onsubmit=async e=>{
    e.preventDefault();
    const body={name:$('#lib-name').value,kind:$('#lib-kind').value,path:$('#lib-path').value,enabled:$('#lib-enabled').checked};
    try{
      await req(id?`/libraries/${id}`:'/libraries',{method:id?'PUT':'POST',body:JSON.stringify(body)});
      notice('Biblioteca salva.',true);
      loadLibraries();
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
