/* StormFlix hierarchical categories admin. */
(function(){
  let categoryItems=[];

  function init(){
    const nav=document.querySelector('nav [data-page="categories"]');
    if(nav)nav.addEventListener('click',()=>setTimeout(loadCategoryAdmin,0));
  }

  async function loadCategoryAdmin(){
    const root=document.querySelector('#categories');if(!root)return;
    root.innerHTML='<div class="catalog-admin-loading">Carregando categorias…</div>';
    try{
      if(!Array.isArray(libs)||!libs.length)libs=await req('/admin/storage');
      categoryItems=await req('/admin/categories');
      render(root);
    }catch(err){root.innerHTML=`<div class="panel"><p class="offline">${esc(err.message)}</p></div>`}
  }

  function render(root){
    const roots=categoryItems.filter(c=>!c.parent_id).sort(sortCategory);
    root.innerHTML=`<div class="panel"><div class="panel-head"><div><h2>Categorias e subcategorias</h2><small>O menu principal fica só com as categorias raiz; as subcategorias organizam o conteúdo abaixo delas no site.</small></div><div class="actions"><button id="category-organize">Organizar estrutura recomendada</button><button class="primary" id="category-new">+ Nova categoria</button></div></div><div class="phase2-hint"><b>Estrutura recomendada:</b> Filmes → 4K / UHD, Animação, Outros · Séries → Séries de TV, Desenhos, Animes com temporadas · Animes → Dublados, Séries, Filmes. O organizador usa os tipos e nomes das bibliotecas atuais e não apaga categorias personalizadas.</div><div id="category-form"></div><div class="category-tree-admin">${roots.map(c=>renderNode(c,0)).join('')||'<p>Nenhuma categoria.</p>'}</div></div>`;
    $('#category-new').onclick=()=>editCategory();
    $('#category-organize').onclick=organizeRecommended;
    root.querySelectorAll('[data-category-edit]').forEach(b=>b.onclick=()=>editCategory(Number(b.dataset.categoryEdit)));
    root.querySelectorAll('[data-category-child]').forEach(b=>b.onclick=()=>editCategory(0,Number(b.dataset.categoryChild)));
    root.querySelectorAll('[data-category-delete]').forEach(b=>b.onclick=()=>deleteCategory(Number(b.dataset.categoryDelete)));
  }

  function renderNode(c,depth){
    const children=categoryItems.filter(x=>Number(x.parent_id||0)===Number(c.id)).sort(sortCategory);
    const assigned=(c.library_ids||[]).length;
    return `<div class="category-admin-node" style="margin-left:${Math.min(depth,3)*24}px"><div class="category-admin-row"><div><b>${depth?'↳ ':''}${esc(c.name)}</b><small>${esc(c.kind)} · ${assigned} biblioteca(s)${c.active?'':' · INATIVA'}${c.system?' · SISTEMA':''}</small></div><div class="actions"><button data-category-edit="${c.id}">Editar</button><button data-category-child="${c.id}">+ Subcategoria</button>${c.system?'':`<button class="danger" data-category-delete="${c.id}">Excluir</button>`}</div></div>${children.map(x=>renderNode(x,depth+1)).join('')}</div>`;
  }

  function sortCategory(a,b){return Number(a.sort_order||0)-Number(b.sort_order||0)||Number(a.id)-Number(b.id)}

  async function organizeRecommended(){
    if(!confirm('Criar/atualizar a estrutura recomendada de subcategorias com base nas bibliotecas atuais? Categorias personalizadas não serão apagadas.'))return;
    try{
      const result=await req('/admin/categories/organize',{method:'POST',body:'{}'});
      notice(`Categorias organizadas · ${result.created||0} criada(s) · ${result.assignments||0} associação(ões).`,true);
      await loadCategoryAdmin();
      if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
    }catch(err){notice(err.message)}
  }

  function editCategory(id=0,parentPreset=0){
    const root=$('#category-form');if(!root)return;
    const c=categoryItems.find(x=>Number(x.id)===Number(id))||{id:0,name:'',slug:'',kind:'mixed',parent_id:parentPreset||null,sort_order:0,active:true,system:false,library_ids:[]};
    const possibleParents=categoryItems.filter(x=>Number(x.id)!==Number(c.id));
    root.innerHTML=`<form id="category-editor" class="form"><input value="${esc(c.name)}" placeholder="Nome (ex.: Clássicos, Dublados)" required><input value="${esc(c.slug)}" placeholder="slug-sem-espacos" ${c.system?'disabled':''} required><select>${['movie','series','anime','mixed','other'].map(k=>`<option value="${k}">${k}</option>`).join('')}</select><select><option value="">Sem categoria principal</option>${possibleParents.map(p=>`<option value="${p.id}">${esc(parentLabel(p))}</option>`).join('')}</select><input type="number" value="${Number(c.sort_order||0)}" placeholder="Ordem"><label><input type="checkbox" ${c.active?'checked':''}> Ativa</label><button class="primary">Salvar</button><button type="button" id="category-cancel">Cancelar</button><div class="wide checklist"><b>Bibliotecas diretamente nesta categoria</b>${libs.map(l=>`<label><input type="checkbox" data-category-lib="${l.id}" ${(c.library_ids||[]).includes(l.id)?'checked':''}> ${esc(l.name)}</label>`).join('')}</div><div class="wide phase2-hint">Uma categoria principal mostra automaticamente o conteúdo das subcategorias. No site, cada subcategoria vira uma fileira própria em vez de misturar tudo.</div></form>`;
    const f=$('#category-editor');
    f.elements[2].value=c.kind||'mixed';
    f.elements[3].value=c.parent_id||'';
    $('#category-cancel').onclick=()=>{root.innerHTML=''};
    f.onsubmit=async e=>{
      e.preventDefault();
      const parentValue=f.elements[3].value;
      const body={name:f.elements[0].value.trim(),slug:c.system?c.slug:f.elements[1].value.trim().toLowerCase(),kind:f.elements[2].value,parent_id:parentValue?Number(parentValue):null,sort_order:Number(f.elements[4].value||0),active:f.elements[5].checked,library_ids:$$('#category-editor [data-category-lib]:checked').map(x=>Number(x.dataset.categoryLib))};
      try{
        await req(c.id?`/admin/categories/${c.id}`:'/admin/categories',{method:c.id?'PUT':'POST',body:JSON.stringify(body)});
        notice('Categoria salva.',true);await loadCategoryAdmin();
        if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
      }catch(err){notice(err.message)}
    };
  }

  function parentLabel(c){
    const parent=categoryItems.find(x=>Number(x.id)===Number(c.parent_id||0));
    return parent?`${parent.name} → ${c.name}`:c.name;
  }

  async function deleteCategory(id){
    const c=categoryItems.find(x=>Number(x.id)===Number(id));if(!c)return;
    if(!confirm(`Excluir a categoria “${c.name}”? Subcategorias existentes subirão um nível; nenhuma biblioteca ou mídia será apagada.`))return;
    try{await req(`/admin/categories/${id}`,{method:'DELETE'});notice('Categoria excluída. Conteúdo preservado.',true);await loadCategoryAdmin()}catch(err){notice(err.message)}
  }

  window.loadCategoryAdmin=loadCategoryAdmin;
  document.addEventListener('DOMContentLoaded',init);
})();
