/* StormFlix Admin: Home menus + smart gallery sections. */
(function(){
  let categoryItems=[];
  let ruleMap=new Map();
  let previewTimer=null;

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
      const [cats,rules]=await Promise.all([req('/admin/categories'),req('/admin/category-rules').catch(()=>[])]);
      categoryItems=cats;
      ruleMap=new Map((rules||[]).map(x=>[Number(x.category_id),x]));
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
            <h2>Menus da Home e seções inteligentes</h2>
            <p>Menus aparecem na navegação principal. Seções ficam dentro deles como fileiras e podem usar bibliotecas, regras automáticas ou as duas coisas.</p>
          </div>
          <div class="home-menu-admin-actions">
            <button id="category-organize">Criar estrutura recomendada</button>
            <button class="primary" id="category-new-menu">+ Novo menu da Home</button>
          </div>
        </section>

        <div class="home-menu-flow-example">
          <span>HOME</span><b>→</b><span>ANIMES</span><b>→</b><span class="section-chip">Dublados automaticamente</span><span class="section-chip">Legendados</span><span class="section-chip">4K / HDR</span>
        </div>

        <div class="phase2-hint home-menu-admin-hint">
          <b>Automação:</b> arraste menus e seções para mudar a ordem. Uma seção inteligente pode, por exemplo, herdar todo o conteúdo de Animes e filtrar apenas áudio Português (Brasil), 4K, HDR, gênero, ano, nota ou recém-adicionados. A prévia mostra o resultado antes de salvar.
        </div>

        <div id="category-form"></div>
        <div class="home-menu-list" id="home-menu-sortable">
          ${menus.map(renderMenu).join('')||'<div class="home-menu-empty">Nenhum menu configurado.</div>'}
        </div>
      </div>`;

    $('#category-new-menu').onclick=()=>editMenu();
    $('#category-organize').onclick=organizeRecommended;
    root.querySelectorAll('[data-menu-edit]').forEach(b=>b.onclick=()=>editMenu(Number(b.dataset.menuEdit)));
    root.querySelectorAll('[data-section-new]').forEach(b=>b.onclick=()=>editSection(Number(b.dataset.sectionNew)));
    root.querySelectorAll('[data-section-edit]').forEach(b=>b.onclick=()=>editSection(Number(b.dataset.sectionParent),Number(b.dataset.sectionEdit)));
    root.querySelectorAll('[data-category-delete]').forEach(b=>b.onclick=()=>deleteCategory(Number(b.dataset.categoryDelete)));
    enableSortable($('#home-menu-sortable'),'.home-menu-card',null);
    menus.forEach(menu=>enableSortable(root.querySelector(`[data-sections-for="${menu.id}"]`),'.home-section-row',menu.id));
  }

  function renderMenu(menu){
    const sections=childrenOf(menu.id);
    const assigned=Number((menu.library_ids||[]).length);
    const canDelete=!menu.system&&!sections.length;
    return `<article class="home-menu-card ${menu.active?'':'is-inactive'}" draggable="true" data-sort-id="${menu.id}">
      <header class="home-menu-card-head">
        <div class="home-menu-title-wrap">
          <span class="home-drag-handle" title="Arraste para ordenar">⋮⋮</span><span class="home-menu-order">${Number(menu.sort_order||0)}</span>
          <div><div class="home-menu-title-line"><h3>${esc(menu.name)}</h3><span class="home-menu-badge">MENU DA HOME</span>${menu.system?'<span class="home-menu-system">SISTEMA</span>':''}${menu.active?'':'<span class="home-menu-off">INATIVO</span>'}</div>
          <small>/${esc(menu.slug)} · ${esc(kindLabel(menu.kind))} · ${sections.length} seção(ões) · ${assigned} biblioteca(s) no ramo</small></div>
        </div>
        <div class="home-menu-card-actions">
          <button data-menu-edit="${menu.id}">Editar menu</button>
          <button class="primary" data-section-new="${menu.id}">+ Nova seção</button>
          ${canDelete?`<button class="danger" data-category-delete="${menu.id}">Excluir menu</button>`:''}
        </div>
      </header>
      <div class="home-menu-sections" data-sections-for="${menu.id}">
        ${sections.length?sections.map(section=>renderSection(menu,section)).join(''):`<div class="home-menu-no-sections"><b>Sem seções manuais</b><span>Enquanto não houver seções, ${esc(menu.name)} continua usando a organização automática por gênero.</span></div>`}
      </div>
    </article>`;
  }

  function renderSection(menu,section){
    const ids=section.library_ids||[];
    const names=ids.map(id=>libraryByID(id)?.name).filter(Boolean);
    const config=ruleMap.get(Number(section.id))||{rule_mode:'libraries',rules:{}};
    const ruleSummary=sectionRuleSummary(config);
    return `<div class="home-section-row ${section.active?'':'is-inactive'}" draggable="true" data-sort-id="${section.id}">
      <span class="home-drag-handle" title="Arraste para ordenar">⋮⋮</span><div class="home-section-order">${Number(section.sort_order||0)}</div>
      <div class="home-section-main">
        <div class="home-section-title"><b>${esc(section.name)}</b><span>SEÇÃO DA GALERIA</span>${config.rule_mode!=='libraries'?'<span class="smart-rule-badge">INTELIGENTE</span>':''}${section.active?'':'<em>INATIVA</em>'}</div>
        <small>/${esc(section.slug)} · ${esc(kindLabel(section.kind))} · ${esc(ruleSummary)}</small>
        <div class="home-section-libraries">${names.length?names.map(name=>`<span>${esc(name)}</span>`).join(''):'<span class="empty">Escopo herdado do menu / nenhuma biblioteca fixa</span>'}</div>
      </div>
      <div class="home-section-actions">
        <button data-section-edit="${section.id}" data-section-parent="${menu.id}">Editar</button>
        <button class="danger" data-category-delete="${section.id}">Excluir</button>
      </div>
    </div>`;
  }

  function sectionRuleSummary(config){
    if(config.rule_mode==='libraries')return'Bibliotecas escolhidas';
    const r=config.rules||{},parts=[];
    if(r.dub_status)parts.push(r.dub_status==='dublado'?'Dublado':r.dub_status==='legendado'?'Legendado':r.dub_status);
    if(r.audio_pt_br)parts.push('áudio pt-BR');
    if(r.subtitle_pt_br)parts.push('legenda pt-BR');
    if(r.min_height>=2000)parts.push('4K+');else if(r.min_height>=900)parts.push('1080p+');
    if(r.hdr)parts.push(r.hdr.toUpperCase());
    if(r.genres?.length)parts.push(r.genres.join(', '));
    if(r.year_from||r.year_to)parts.push(`${r.year_from||'…'}–${r.year_to||'…'}`);
    if(r.min_rating)parts.push(`nota ≥ ${r.min_rating}`);
    if(r.recent_days)parts.push(`últimos ${r.recent_days} dias`);
    return `${config.rule_mode==='both'?'Bibliotecas + regras':'Regras no menu'}${parts.length?' · '+parts.join(' · '):''}`;
  }

  function sortCategory(a,b){return Number(a.sort_order||0)-Number(b.sort_order||0)||Number(a.id)-Number(b.id)}

  function enableSortable(container,selector,parentID){
    if(!container)return;
    let dragged=null;
    container.querySelectorAll(`:scope > ${selector}`).forEach(el=>{
      el.addEventListener('dragstart',e=>{dragged=el;el.classList.add('is-dragging');e.dataTransfer.effectAllowed='move'});
      el.addEventListener('dragend',async()=>{
        if(!dragged)return;
        dragged.classList.remove('is-dragging');dragged=null;
        const ids=[...container.querySelectorAll(`:scope > ${selector}`)].map(x=>Number(x.dataset.sortId)).filter(Boolean);
        try{
          await req('/admin/categories/order',{method:'PUT',body:JSON.stringify({parent_id:parentID,ids})});
          await loadCategoryAdmin();
          if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
        }catch(err){notice(err.message);await loadCategoryAdmin()}
      });
      el.addEventListener('dragover',e=>{
        if(!dragged||dragged===el)return;
        e.preventDefault();
        const rect=el.getBoundingClientRect();
        container.insertBefore(dragged,e.clientY<rect.top+rect.height/2?el:el.nextSibling);
      });
    });
  }

  async function organizeRecommended(){
    if(!confirm('Criar/atualizar menus e seções recomendadas com base nas bibliotecas atuais? Um backup automático será criado antes. Itens personalizados não serão apagados.'))return;
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

  function nextMenuOrder(){return Math.max(0,...roots().map(x=>Number(x.sort_order||0)))+10}
  function nextSectionOrder(parentID){return Math.max(0,...childrenOf(parentID).map(x=>Number(x.sort_order||0)))+10}

  function renderEditor(c,parentMenu){
    const host=$('#category-form');if(!host)return;
    const isSection=!!parentMenu;
    const selectedIDs=new Set((c.library_ids||[]).map(Number));
    const stored=ruleMap.get(Number(c.id))||{rule_mode:'libraries',rules:{}};
    const rules=stored.rules||{};
    host.innerHTML=`<section class="home-menu-editor-card">
      <div class="home-menu-editor-head"><div><p class="kicker">${isSection?'Seção inteligente da galeria':'Menu da Home'}</p><h3>${c.id?'Editar':'Criar'} ${isSection?'seção':'menu'}</h3><small>${isSection?`Esta seção ficará dentro de ${esc(parentMenu.name)} e não aparecerá na navegação principal.`:'Este item aparecerá na navegação principal da Home quando estiver ativo e tiver conteúdo/seções.'}</small></div><button type="button" id="category-cancel">Fechar</button></div>
      <form id="category-editor" class="home-menu-editor-form">
        <label><span>Nome</span><input id="category-name" value="${esc(c.name)}" placeholder="${isSection?'Ex.: Animes Dublados':'Ex.: Desenhos'}" required></label>
        <label><span>Slug</span><input id="category-slug" value="${esc(c.slug)}" placeholder="${isSection?'animes-dublados':'desenhos'}" ${c.system?'disabled':''} required></label>
        <label><span>Tipo de conteúdo</span><select id="category-kind">${['movie','series','anime','mixed','other'].map(k=>`<option value="${k}" ${k===c.kind?'selected':''}>${esc(kindLabel(k))}</option>`).join('')}</select></label>
        ${isSection?`<label><span>Menu da Home</span><select id="category-parent">${roots().map(root=>`<option value="${root.id}" ${Number(root.id)===Number(parentMenu.id)?'selected':''}>${esc(root.name)}</option>`).join('')}</select></label>`:''}
        <label><span>Ordem</span><input id="category-order" type="number" value="${Number(c.sort_order||0)}" step="1"><small>Você também pode arrastar depois de salvar.</small></label>
        <label class="home-menu-toggle"><input id="category-active" type="checkbox" ${c.active?'checked':''}><span>Ativo</span></label>

        ${isSection?smartRuleEditor(stored.rule_mode||'libraries',rules):''}

        <div class="home-menu-library-picker">
          <div><b>${isSection?'Bibliotecas desta seção':'Bibliotecas diretas deste menu'}</b><small>${isSection?'No modo Regras automáticas, deixar vazio faz a seção herdar todas as bibliotecas do menu.':'Usadas principalmente quando o menu ainda não possui seções.'}</small></div>
          <div class="home-menu-library-grid">${(libs||[]).filter(l=>l.kind!=='music').map(l=>`<label><input type="checkbox" data-category-lib="${l.id}" ${selectedIDs.has(Number(l.id))?'checked':''}><span><b>${esc(l.name)}</b><small>${esc(l.kind||'')}</small></span></label>`).join('')||'<p>Nenhuma biblioteca de vídeo cadastrada.</p>'}</div>
        </div>
        ${isSection?`<div class="smart-preview"><div><b>Prévia da seção</b><small id="category-preview-state">Altere os filtros ou clique em atualizar.</small></div><button type="button" id="category-preview-button">Atualizar prévia</button><div id="category-preview-results"></div></div>`:''}
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
    if(isSection){
      $('#category-preview-button').onclick=()=>updatePreview(c);
      $('#category-editor').addEventListener('input',()=>schedulePreview(c));
      $('#category-editor').addEventListener('change',()=>schedulePreview(c));
      updatePreview(c);
    }

    $('#category-editor').onsubmit=async e=>{
      e.preventDefault();
      const parentID=isSection?Number($('#category-parent').value):null;
      const body={name:nameInput.value.trim(),slug:c.system?c.slug:slugInput.value.trim().toLowerCase(),kind:$('#category-kind').value,parent_id:parentID||null,sort_order:Number($('#category-order').value||0),active:$('#category-active').checked,library_ids:$$('#category-editor [data-category-lib]:checked').map(x=>Number(x.dataset.categoryLib))};
      try{
        const saved=await req(c.id?`/admin/categories/${c.id}`:'/admin/categories',{method:c.id?'PUT':'POST',body:JSON.stringify(body)});
        const savedID=Number(c.id||saved.id||0);
        if(isSection&&savedID){
          const smart=collectSmartRules();
          await req(`/admin/categories/${savedID}/rules`,{method:'PUT',body:JSON.stringify(smart)});
        }
        notice(isSection?'Seção salva.':'Menu da Home salvo.',true);
        await loadCategoryAdmin();
        if(window.sfCategories?.reload)window.sfCategories.reload().catch(()=>{});
      }catch(err){notice(err.message)}
    };
  }

  function smartRuleEditor(mode,r){
    const resolution=r.min_height>=4000?'8k':r.min_height>=2000?'4k':r.min_height>=1300?'1440':r.min_height>=900?'1080':r.min_height>=650?'720':'any';
    return `<div class="smart-rules-card">
      <div class="smart-rules-head"><div><b>Regras inteligentes</b><small>O StormFlix usa metadados e análise real dos streams para manter a seção sozinho.</small></div><span>100% pt-BR</span></div>
      <div class="smart-rules-grid">
        <label><span>Como escolher títulos</span><select id="rule-mode"><option value="libraries" ${mode==='libraries'?'selected':''}>Somente bibliotecas</option><option value="rules" ${mode==='rules'?'selected':''}>Regras automáticas em todo o menu</option><option value="both" ${mode==='both'?'selected':''}>Bibliotecas + regras</option></select></label>
        <label><span>Áudio / legenda</span><select id="rule-dub"><option value="">Qualquer</option><option value="dublado" ${r.dub_status==='dublado'?'selected':''}>Dublado (áudio pt-BR)</option><option value="legendado" ${r.dub_status==='legendado'?'selected':''}>Legendado em pt-BR</option><option value="original" ${r.dub_status==='original'?'selected':''}>Sem faixa pt-BR detectada</option></select></label>
        <label><span>Resolução mínima</span><select id="rule-resolution"><option value="any">Qualquer</option><option value="720" ${resolution==='720'?'selected':''}>720p+</option><option value="1080" ${resolution==='1080'?'selected':''}>1080p+</option><option value="1440" ${resolution==='1440'?'selected':''}>1440p+</option><option value="4k" ${resolution==='4k'?'selected':''}>4K / UHD</option><option value="8k" ${resolution==='8k'?'selected':''}>8K</option></select></label>
        <label><span>HDR</span><select id="rule-hdr"><option value="">Qualquer</option><option value="hdr" ${r.hdr==='hdr'?'selected':''}>Qualquer HDR</option><option value="hdr10" ${r.hdr==='hdr10'?'selected':''}>HDR10</option><option value="hlg" ${r.hdr==='hlg'?'selected':''}>HLG</option><option value="sdr" ${r.hdr==='sdr'?'selected':''}>Somente SDR</option></select></label>
        <label><span>Gêneros (separe por vírgula)</span><input id="rule-genres" value="${esc((r.genres||[]).join(', '))}" placeholder="Ação, Comédia, Romance"></label>
        <label><span>Tipos (separe por vírgula)</span><input id="rule-types" value="${esc((r.media_types||[]).join(', '))}" placeholder="movie, anime, series"></label>
        <label><span>Ano inicial</span><input id="rule-year-from" type="number" min="1900" max="2200" value="${Number(r.year_from||0)||''}"></label>
        <label><span>Ano final</span><input id="rule-year-to" type="number" min="1900" max="2200" value="${Number(r.year_to||0)||''}"></label>
        <label><span>Nota mínima</span><input id="rule-rating" type="number" min="0" max="10" step="0.1" value="${Number(r.min_rating||0)||''}"></label>
        <label><span>Adicionados nos últimos dias</span><input id="rule-recent" type="number" min="0" max="3650" value="${Number(r.recent_days||0)||''}" placeholder="Ex.: 30"></label>
        <label class="home-menu-toggle"><input id="rule-audio-pt" type="checkbox" ${r.audio_pt_br?'checked':''}><span>Exigir áudio Português (Brasil)</span></label>
        <label class="home-menu-toggle"><input id="rule-subtitle-pt" type="checkbox" ${r.subtitle_pt_br?'checked':''}><span>Exigir legenda Português (Brasil)</span></label>
        <label class="home-menu-toggle"><input id="rule-metadata" type="checkbox" ${r.require_metadata?'checked':''}><span>Exigir metadados identificados</span></label>
      </div>
    </div>`;
  }

  function resolutionHeight(value){return {720:650,1080:900,1440:1300,'4k':2000,'8k':4000}[value]||0}
  function splitRules(value){return String(value||'').split(',').map(x=>x.trim()).filter(Boolean)}
  function collectSmartRules(){
    return {rule_mode:$('#rule-mode')?.value||'libraries',rules:{genres:splitRules($('#rule-genres')?.value),media_types:splitRules($('#rule-types')?.value),year_from:Number($('#rule-year-from')?.value||0),year_to:Number($('#rule-year-to')?.value||0),min_rating:Number($('#rule-rating')?.value||0),min_height:resolutionHeight($('#rule-resolution')?.value),hdr:$('#rule-hdr')?.value||'',dub_status:$('#rule-dub')?.value||'',audio_pt_br:Boolean($('#rule-audio-pt')?.checked),subtitle_pt_br:Boolean($('#rule-subtitle-pt')?.checked),recent_days:Number($('#rule-recent')?.value||0),require_metadata:Boolean($('#rule-metadata')?.checked)}};
  }

  function schedulePreview(c){clearTimeout(previewTimer);previewTimer=setTimeout(()=>updatePreview(c),450)}
  async function updatePreview(c){
    const state=$('#category-preview-state'),root=$('#category-preview-results');if(!state||!root)return;
    state.textContent='Calculando…';
    const smart=collectSmartRules();
    const parentID=Number($('#category-parent')?.value||c.parent_id||0);
    const libraryIDs=$$('#category-editor [data-category-lib]:checked').map(x=>Number(x.dataset.categoryLib));
    try{
      const result=await req('/admin/categories/preview',{method:'POST',body:JSON.stringify({category_id:Number(c.id||0),parent_id:parentID,library_ids:libraryIDs,...smart})});
      state.textContent=`${result.count||0} título(s)${result.technical_pending?' · análise técnica ainda processando em segundo plano':''}`;
      root.innerHTML=(result.samples||[]).length?`<div class="smart-preview-covers">${result.samples.map(item=>item.poster_url?`<img src="${esc(item.poster_url)}" alt="${esc(item.title)}" loading="lazy" decoding="async" title="${esc(item.title)}">`:`<span title="${esc(item.title)}">${esc((item.title||'?').charAt(0))}</span>`).join('')}</div>`:'<small>Nenhum título corresponde às regras atuais.</small>';
    }catch(err){state.textContent=err.message;root.innerHTML=''}
  }

  function slugify(value){return String(value||'').normalize('NFD').replace(/[\u0300-\u036f]/g,'').toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'').slice(0,40)}

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
