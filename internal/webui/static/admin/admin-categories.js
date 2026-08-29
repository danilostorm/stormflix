/* StormFlix Admin: Home menus + gallery sections. */
(function(){
  let categoryItems=[];

  const kindLabel=value=>({movie:'Filmes',series:'Séries / desenhos',anime:'Animes',mixed:'Misto',other:'Outros'})[value]||value||'Misto';
  const roots=()=>categoryItems.filter(c=>!c.parent_id).sort(sortCategory);
  const childrenOf=id=>categoryItems.filter(c=>Number(c.parent_id||0)===Number(id)).sort(sortCategory);
  const libraryByID=id=>(libs||[]).find(l=>Number(l.id)===Number(id));

  function init(){
    const nav=document.querySelector('nav [data-page="categories"]');
    if(nav)nav.addEventListener('click',()=>setTimeout(loadCategoryAdmin,0));
  }

  async function loadCategoryAdmin(){
    const root=document.querySelector('#categories');if(!root)return;
    root.innerHTML='<div class="catalog-admin-loading">Carregando menus da Home…</div>';
    try{
      if(!Array.isArray(libs)||!libs.length)libs=await req('/admin/storage');
      categoryItems=await req('/admin/categories');
      render(root);
    }catch(err){root.innerHTML=`<div class="panel"><p class="offline">${esc(err.message)}</p></div>`}
  }

  function render(root){
    const menus=roots();
    root.innerHTML=`
      <div class="home-menu-admin-shell">
        <section class="home-menu-admin-hero">
          <div>
            <p class="kicker">Organização da Home</p>
            <h2>Menus da Home e seções da galeria</h2>
            <p>Somente os <b>menus da Home</b> aparecem na navegação principal. As <b>seções</b> ficam dentro do menu escolhido e aparecem como fileiras separadas na galeria.</p>
          </div>
          <div class="home-menu-admin-actions">
            <button id="category-organize">Criar estrutura recomendada</button>
            <button class="primary" id="category-new-menu">+ Novo menu da Home</button>
          </div>
        </section>

        <div class="home-menu-flow-example">
          <span>HOME</span><b>→</b><span>ANIMES</span><b>→</b><span class="section-chip">Animes Dublados</span><span class="section-chip">Animes Legendados</span><span class="section-chip">Filmes de Anime</span>
        </div>

        <div class="phase2-hint home-menu-admin-hint">
          <b>Como funciona:</b> crie um menu como Filmes, Séries, Animes ou Desenhos. Depois use <b>+ Nova seção</b> dentro dele. Quando um menu possui seções, a Home mostra somente essas seções, na ordem configurada; elas não aparecem como botões no menu principal. Um menu sem seções mantém o agrupamento automático por gênero como compatibilidade.
        </div>

        <div id="category-form"></div>
        <div class="home-menu-list">
          ${menus.map(renderMenu).join('')||'<div class="home-menu-empty">Nenhum menu configurado.</div>'}
        </div>
      </div>`;

    $('#category-new-menu').onclick=()=>editMenu();
    $('#category-organize').onclick=organizeRecommended;
    root.querySelectorAll('[data-menu-edit]').forEach(b=>b.onclick=()=>editMenu(Number(b.dataset.menuEdit)));
    root.querySelectorAll('[data-section-new]').forEach(b=>b.onclick=()=>editSection(Number(b.dataset.sectionNew)));
    root.querySelectorAll('[data-section-edit]').forEach(b=>b.onclick=()=>editSection(Number(b.dataset.sectionParent),Number(b.dataset.sectionEdit)));
    root.querySelectorAll('[data-category-delete]').forEach(b=>b.onclick=()=>deleteCategory(Number(b.dataset.categoryDelete)));
  }

  function renderMenu(menu){
    const sections=childrenOf(menu.id);
    const assigned=Number((menu.library_ids||[]).length);
    const canDelete=!menu.system&&!sections.length;
    return `<article class="home-menu-card ${menu.active?'':'is-inactive'}">
      <header class="home-menu-card-head">
        <div class="home-menu-title-wrap">
          <span class="home-menu-order">${Number(menu.sort_order||0)}</span>
          <div><div class="home-menu-title-line"><h3>${esc(menu.name)}</h3><span class="home-menu-badge">MENU DA HOME</span>${menu.system?'<span class="home-menu-system">SISTEMA</span>':''}${menu.active?'':'<span class="home-menu-off">INATIVO</span>'}</div>
          <small>/${esc(menu.slug)} · ${esc(kindLabel(menu.kind))} · ${sections.length} seção(ões) · ${assigned} biblioteca(s) no ramo</small></div>
        </div>
        <div class="home-menu-card-actions">
          <button data-menu-edit="${menu.id}">Editar menu</button>
          <button class="primary" data-section-new="${menu.id}">+ Nova seção</button>
          ${canDelete?`<button class="danger" data-category-delete="${menu.id}">Excluir menu</button>`:''}
        </div>
      </header>
      <div class="home-menu-sections">
        ${sections.length?sections.map(section=>renderSection(menu,section)).join(''):`<div class="home-menu-no-sections"><b>Sem seções manuais</b><span>Enquanto não houver seções, ${esc(menu.name)} continua usando a organização automática por gênero.</span></div>`}
      </div>
    </article>`;
  }

  function renderSection(menu,section){
    const ids=section.library_ids||[];
    const names=ids.map(id=>libraryByID(id)?.name).filter(Boolean);
    return `<div class="home-section-row ${section.active?'':'is-inactive'}">
      <div class="home-section-order">${Number(section.sort_order||0)}</div>
      <div class="home-section-main">
        <div class="home-section-title"><b>${esc(section.name)}</b><span>SEÇÃO DA GALERIA</span>${section.active?'':'<em>INATIVA</em>'}</div>
        <small>/${esc(section.slug)} · ${esc(kindLabel(section.kind))}</small>
        <div class="home-section-libraries">${names.length?names.map(name=>`<span>${esc(name)}</span>`).join(''):'<span class="empty">Nenhuma biblioteca vinculada</span>'}</div>
      </div>
      <div class="home-section-actions">
        <button data-section-edit="${section.id}" data-section-parent="${menu.id}">Editar</button>
        <button class="danger" data-category-delete="${section.id}">Excluir</button>
      </div>
    </div>`;
  }

  function sortCategory(a,b){return Number(a.sort_order||0)-Number(b.sort_order||0)||Number(a.id)-Number(b.id)}

  async function organizeRecommended(){
    if(!confirm('Criar/atualizar menus e seções recomendadas com base nas bibliotecas atuais? Itens personalizados não serão apagados.'))return;
    try{
      const result=await req('/admin/categories/organize',{method:'POST',body:'{}'});
      notice(`Estrutura organizada · ${result.created||0} item(ns) criado(s) · ${result.assignments||0} associação(ões).`,true);
      await loadCategoryAdmin();
      if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
    }catch(err){notice(err.message)}
  }

  function editMenu(id=0){
    const existing=categoryItems.find(x=>Number(x.id)===Number(id));
    const c=existing||{id:0,name:'',slug:'',kind:'mixed',parent_id:null,sort_order:nextMenuOrder(),active:true,system:false,library_ids:[]};
    renderEditor(c,null);
  }

  function editSection(parentID,id=0){
    const menu=categoryItems.find(x=>Number(x.id)===Number(parentID)&&!x.parent_id);if(!menu)return;
    const existing=categoryItems.find(x=>Number(x.id)===Number(id));
    const c=existing||{id:0,name:'',slug:'',kind:menu.kind||'mixed',parent_id:menu.id,sort_order:nextSectionOrder(menu.id),active:true,system:false,library_ids:[]};
    renderEditor(c,menu);
  }

  function nextMenuOrder(){
    return Math.max(0,...roots().map(x=>Number(x.sort_order||0)))+10;
  }
  function nextSectionOrder(parentID){
    return Math.max(0,...childrenOf(parentID).map(x=>Number(x.sort_order||0)))+10;
  }

  function renderEditor(c,parentMenu){
    const host=$('#category-form');if(!host)return;
    const isSection=!!parentMenu;
    const selectedIDs=new Set((c.library_ids||[]).map(Number));
    host.innerHTML=`<section class="home-menu-editor-card">
      <div class="home-menu-editor-head"><div><p class="kicker">${isSection?'Seção da galeria':'Menu da Home'}</p><h3>${c.id?'Editar':'Criar'} ${isSection?'seção':'menu'}</h3><small>${isSection?`Esta seção ficará dentro de ${esc(parentMenu.name)} e não aparecerá na navegação principal.`:'Este item aparecerá na navegação principal da Home quando estiver ativo e tiver conteúdo/seções.'}</small></div><button type="button" id="category-cancel">Fechar</button></div>
      <form id="category-editor" class="home-menu-editor-form">
        <label><span>Nome</span><input id="category-name" value="${esc(c.name)}" placeholder="${isSection?'Ex.: Animes Dublados':'Ex.: Desenhos'}" required></label>
        <label><span>Slug</span><input id="category-slug" value="${esc(c.slug)}" placeholder="${isSection?'animes-dublados':'desenhos'}" ${c.system?'disabled':''} required></label>
        <label><span>Tipo de conteúdo</span><select id="category-kind">${['movie','series','anime','mixed','other'].map(k=>`<option value="${k}" ${k===c.kind?'selected':''}>${esc(kindLabel(k))}</option>`).join('')}</select></label>
        ${isSection?`<label><span>Menu da Home</span><select id="category-parent">${roots().map(root=>`<option value="${root.id}" ${Number(root.id)===Number(parentMenu.id)?'selected':''}>${esc(root.name)}</option>`).join('')}</select></label>`:''}
        <label><span>Ordem</span><input id="category-order" type="number" value="${Number(c.sort_order||0)}" step="1"></label>
        <label class="home-menu-toggle"><input id="category-active" type="checkbox" ${c.active?'checked':''}><span>Ativo</span></label>

        <div class="home-menu-library-picker">
          <div><b>${isSection?'Bibliotecas desta seção':'Bibliotecas diretas deste menu'}</b><small>${isSection?'Os títulos dessas bibliotecas formarão esta fileira da galeria.':'Usadas principalmente quando o menu ainda não possui seções manuais.'}</small></div>
          <div class="home-menu-library-grid">${(libs||[]).filter(l=>l.kind!=='music').map(l=>`<label><input type="checkbox" data-category-lib="${l.id}" ${selectedIDs.has(Number(l.id))?'checked':''}><span><b>${esc(l.name)}</b><small>${esc(l.kind||'')}</small></span></label>`).join('')||'<p>Nenhuma biblioteca de vídeo cadastrada.</p>'}</div>
        </div>
        <div class="home-menu-editor-buttons"><button class="primary" type="submit">Salvar ${isSection?'seção':'menu'}</button></div>
      </form>
    </section>`;

    $('#category-cancel').onclick=()=>{host.innerHTML=''};
    const nameInput=$('#category-name'),slugInput=$('#category-slug');
    let slugTouched=!!c.id;
    if(!c.system){
      slugInput.addEventListener('input',()=>{slugTouched=true});
      nameInput.addEventListener('input',()=>{if(!slugTouched)slugInput.value=slugify(nameInput.value)});
    }

    $('#category-editor').onsubmit=async e=>{
      e.preventDefault();
      const parentID=isSection?Number($('#category-parent').value):null;
      const body={
        name:nameInput.value.trim(),
        slug:c.system?c.slug:slugInput.value.trim().toLowerCase(),
        kind:$('#category-kind').value,
        parent_id:parentID||null,
        sort_order:Number($('#category-order').value||0),
        active:$('#category-active').checked,
        library_ids:$$('#category-editor [data-category-lib]:checked').map(x=>Number(x.dataset.categoryLib))
      };
      try{
        await req(c.id?`/admin/categories/${c.id}`:'/admin/categories',{method:c.id?'PUT':'POST',body:JSON.stringify(body)});
        notice(isSection?'Seção salva.':'Menu da Home salvo.',true);
        await loadCategoryAdmin();
        if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
      }catch(err){notice(err.message)}
    };
  }

  function slugify(value){
    return String(value||'').normalize('NFD').replace(/[\u0300-\u036f]/g,'').toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'').slice(0,40);
  }

  async function deleteCategory(id){
    const c=categoryItems.find(x=>Number(x.id)===Number(id));if(!c)return;
    const children=childrenOf(id);
    if(children.length){notice('Esse menu ainda possui seções. Exclua ou mova as seções antes de remover o menu.');return}
    const label=c.parent_id?'seção':'menu da Home';
    if(!confirm(`Excluir ${label} “${c.name}”? Nenhuma biblioteca ou mídia será apagada.`))return;
    try{
      await req(`/admin/categories/${id}`,{method:'DELETE'});
      notice(`${c.parent_id?'Seção':'Menu'} excluído. Conteúdo preservado.`,true);
      await loadCategoryAdmin();
      if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
    }catch(err){notice(err.message)}
  }

  window.loadCategoryAdmin=loadCategoryAdmin;
  document.addEventListener('DOMContentLoaded',init);
})();
