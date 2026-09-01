/* StormFlix Games G4.2 video filters: live RetroArch GLSL switching without restarting. */
(function(){
  const SHADER_REV='235448f244bf676d135f7b25ea6b8e1eae41c4e4';
  const CDN=`https://cdn.jsdelivr.net/gh/libretro/glsl-shaders@${SHADER_REV}`;
  const packageCache=new Map();
  let liveFilterId=null,liveApplyPromise=null;
  const catalog=[
    {id:'pixel',name:'Pixel perfeito',short:'Pixel',kind:'native',cost:'Leve',description:'Nearest-neighbor nítido, sem suavização. Ideal para preservar cada pixel.'},
    {id:'bilinear',name:'Bilinear',short:'Bilinear',kind:'native',cost:'Leve',description:'Suavização simples no canvas. Reduz blocos sem alterar os sprites.'},
    {id:'scanlines',name:'Scanlines',short:'Scanlines',kind:'shader',cost:'Leve',description:'Linhas de varredura leves para lembrar uma tela CRT sem pesar no navegador.',preset:'scanlines/res-independent-scanlines.glslp',assets:['scanlines/shaders/res-independent-scanlines.glsl']},
    {id:'crt',name:'CRT EasyMode',short:'CRT',kind:'shader',cost:'Médio',description:'Curvatura, máscara e scanlines de CRT com bom equilíbrio entre aparência e desempenho.',preset:'crt/crt-easymode.glslp',assets:['crt/shaders/crt-easymode.glsl']},
    {id:'ntsc',name:'NTSC S-Video',short:'NTSC',kind:'shader',cost:'Médio',description:'Mistura de cor e sinal analógico no estilo de uma TV antiga ligada por S-Video.',preset:'ntsc/ntsc-320px-svideo.glslp',assets:['ntsc/shaders/ntsc-pass1-svideo-2phase.glsl','ntsc/shaders/ntsc-pass2-2phase-gamma.glsl']},
    {id:'hq2x',name:'HQ2x',short:'HQ2x',kind:'shader',cost:'Médio',description:'Suaviza diagonais e contornos em 2x mantendo a leitura do pixel art.',preset:'hqx/hq2x.glslp',assets:['hqx/shader-files/hqx-pass1.glsl','hqx/shader-files/hqx-pass2.glsl','hqx/resources/hq2x.png'],rewrites:{'shader-files/hqx-pass1.glsl':'shaders/hqx-pass1.glsl','shader-files/hqx-pass2.glsl':'shaders/hqx-pass2.glsl','resources/hq2x.png':'shaders/hq2x.png'}},
    {id:'hq4x',name:'HQ4x',short:'HQ4x',kind:'shader',cost:'Pesado',description:'Versão 4x do HQx para telas grandes. Mais lisa e mais exigente na GPU.',preset:'hqx/hq4x.glslp',assets:['hqx/shader-files/hqx-pass1.glsl','hqx/shader-files/hqx-pass2.glsl','hqx/resources/hq4x.png'],rewrites:{'shader-files/hqx-pass1.glsl':'shaders/hqx-pass1.glsl','shader-files/hqx-pass2.glsl':'shaders/hqx-pass2.glsl','resources/hq4x.png':'shaders/hq4x.png'}},
    {id:'xbrz',name:'xBRZ Adaptativo',short:'xBRZ',kind:'shader',cost:'Médio',description:'Escala xBRZ livre, limpa curvas e diagonais conforme o tamanho do viewport.',preset:'xbrz/xbrz-freescale.glslp',assets:['xbrz/shaders/xbrz-freescale.glsl']},
    {id:'xbrz4',name:'xBRZ 4x',short:'xBRZ 4x',kind:'shader',cost:'Pesado',description:'xBRZ em 4x com acabamento HD para sprites, recomendado para desktop.',preset:'xbrz/4xbrz-linear.glslp',assets:['xbrz/shaders/4xbrz.glsl','stock.glsl'],rewrites:{'../stock.glsl':'shaders/stock.glsl'}},
  ];
  const byId=new Map(catalog.map(item=>[item.id,item]));

  function player(){return window.StormFlixGamePlayer}
  function selected(video){const id=String(video?.filter||'').trim();if(byId.has(id))return id;return video?.smooth?'bilinear':'pixel'}
  function fileName(path){return path.split('/').pop()||path}
  function esc(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function costClass(cost){return String(cost||'').toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/[^a-z]/g,'')}
  function makeFile(name,content){if(typeof File==='function')return new File([content],name,{type:'application/octet-stream'});return {fileName:name,fileContent:content}}

  async function fetchAsset(path){
    const response=await fetch(`${CDN}/${path}`,{cache:'force-cache',mode:'cors'});
    if(!response.ok)throw new Error(`Filtro gráfico indisponível (${response.status})`);
    return response.blob();
  }
  function rewritePreset(text,rewrites){let out=text;for(const [from,to] of Object.entries(rewrites||{}))out=out.split(from).join(to);return out}
  function assetDestination(spec,path){
    const parts=path.split('/'),relative=parts.length>1?parts.slice(1).join('/'):path,base=fileName(path);
    if(spec.rewrites?.[relative])return spec.rewrites[relative];
    const match=Object.entries(spec.rewrites||{}).find(([from])=>from===base||from.endsWith(`/${base}`));
    if(match)return match[1];
    if(relative.startsWith('shaders/'))return relative;
    return `shaders/${base}`;
  }
  async function loadPackage(id){
    if(packageCache.has(id))return packageCache.get(id);
    const spec=byId.get(id);if(!spec||spec.kind!=='shader')return null;
    const promise=(async()=>{
      const [presetBlob,...assetBlobs]=await Promise.all([fetchAsset(spec.preset),...spec.assets.map(fetchAsset)]);
      const presetText=rewritePreset(await presetBlob.text(),spec.rewrites);
      const assets=await Promise.all(spec.assets.map(async(path,index)=>({name:assetDestination(spec,path),data:new Uint8Array(await assetBlobs[index].arrayBuffer())})));
      return {id,presetName:`stormflix-${id}.glslp`,presetText,assets};
    })().catch(error=>{packageCache.delete(id);throw error});
    packageCache.set(id,promise);return promise;
  }
  async function buildBundle(id){
    const pkg=await loadPackage(id);if(!pkg)return [];
    return [makeFile(pkg.presetName,pkg.presetText),...pkg.assets.map(asset=>makeFile(fileName(asset.name),asset.data))];
  }

  function runtimeContext(){
    const runtime=player()?.runtime?.();if(!runtime||runtime.getStatus?.()==='terminated')throw new Error('Emulador ainda não está pronto');
    const ctx=runtime.getEmscripten?.();if(!ctx)throw new Error('Runtime RetroArch indisponível');return ctx;
  }
  function ensureDir(FS,path){
    if(FS.mkdirTree){FS.mkdirTree(path);return}
    let current='';for(const part of path.split('/').filter(Boolean)){current+=`/${part}`;try{FS.mkdir(current)}catch{}}
  }
  function callSetShader(ctx,path){
    const Module=ctx.Module||ctx,fn=Module._cmd_set_shader||ctx._cmd_set_shader,alloc=Module.stringToNewUTF8||ctx.stringToNewUTF8,free=Module._free||ctx._free;
    if(typeof fn!=='function'){
      if(!path)return true;
      throw new Error('Este core Web não expõe troca de shader em tempo real');
    }
    if(typeof alloc==='function'){
      const ptr=alloc(path);try{const result=fn(ptr);if(path&&result===0)throw new Error('RetroArch recusou o shader selecionado');return result!==0}finally{if(ptr&&typeof free==='function')free(ptr)}
    }
    if(typeof Module.ccall==='function'){
      const result=Module.ccall('cmd_set_shader','number',['string'],[path]);if(path&&result===0)throw new Error('RetroArch recusou o shader selecionado');return result!==0;
    }
    throw new Error('Runtime Web sem alocador de string para shader');
  }
  function writeRuntimePackage(ctx,pkg){
    const Module=ctx.Module||ctx,FS=Module.FS||ctx.FS;if(!FS?.writeFile)throw new Error('Filesystem do RetroArch indisponível');
    const root=`/stormflix-shaders/${pkg.id}`;ensureDir(FS,root);ensureDir(FS,`${root}/shaders`);
    FS.writeFile(`${root}/${pkg.presetName}`,pkg.presetText);
    for(const asset of pkg.assets){const slash=asset.name.lastIndexOf('/');if(slash>0)ensureDir(FS,`${root}/${asset.name.slice(0,slash)}`);FS.writeFile(`${root}/${asset.name}`,asset.data)}
    return `${root}/${pkg.presetName}`;
  }
  async function applyLive(id){
    if(liveApplyPromise)await liveApplyPromise;
    const spec=byId.get(id);if(!spec)throw new Error('Filtro desconhecido');
    liveApplyPromise=(async()=>{
      const ctx=runtimeContext();
      if(spec.kind==='native'){
        callSetShader(ctx,'');liveFilterId=id;window.dispatchEvent(new CustomEvent('stormflix:game-filter-applied',{detail:{id,live:true}}));return true;
      }
      const pkg=await loadPackage(id),path=writeRuntimePackage(ctx,pkg);callSetShader(ctx,path);liveFilterId=id;window.dispatchEvent(new CustomEvent('stormflix:game-filter-applied',{detail:{id,live:true,path}}));return true;
    })();
    try{return await liveApplyPromise}finally{liveApplyPromise=null}
  }

  function decorateOptions(options,video){
    const id=selected(video);
    options.retroarchConfig={...(options.retroarchConfig||{}),video_smooth:false};
    options.cache={...(options.cache||{}),shader:true};
    delete options.shader;delete options.resolveShader;
    options.__stormflixVideoFilter=id;
    return options;
  }
  function expose(){
    const api=player();if(!api)return;
    api.videoFilters=()=>catalog.map(item=>({...item,assets:undefined,rewrites:undefined,preset:undefined}));
    api.videoFilter=()=>selected(api.preferences?.().video||{});
    api.preloadVideoFilter=id=>loadPackage(id);
    api.applyVideoFilter=id=>applyLive(id);
  }
  function filterGridHTML(active){
    return `<div class="sf-g41-filter-grid">${catalog.map(item=>`<button type="button" class="sf-g41-filter-card ${active===item.id?'active':''}" data-g41-filter="${item.id}" data-live="${liveFilterId===item.id?'1':'0'}"><span class="sf-g41-filter-head"><b>${esc(item.name)}</b><i class="${costClass(item.cost)}">${esc(item.cost)}</i></span><small>${esc(item.description)}</small></button>`).join('')}</div>`;
  }
  function markLive(row,id){row.querySelectorAll('[data-g41-filter]').forEach(b=>{b.classList.toggle('active',b.dataset.g41Filter===id);b.dataset.live=b.dataset.g41Filter===id?'1':'0'})}
  function enhanceVideoPanel(){
    const panel=document.querySelector('[data-g4-panel]:not(.hidden)');if(!panel)return;
    const smooth=panel.querySelector('[data-video-smooth]');if(!smooth)return;
    const row=smooth.closest('.sf-g4-setting');if(!row||row.dataset.g41Filters==='1')return;
    const active=selected(player()?.preferences?.().video||{});
    row.dataset.g41Filters='1';row.classList.add('sf-g41-filter-setting');
    row.innerHTML=`<span class="sf-g41-filter-title"><b>Filtro gráfico em tempo real</b><small>Mesmo princípio do EmulatorJS/RomM: o preset é escrito no RetroArch já aberto e trocado sem reiniciar o jogo.</small></span>${filterGridHTML(active)}`;
    row.querySelectorAll('[data-g41-filter]').forEach(button=>button.addEventListener('click',async()=>{
      const id=button.dataset.g41Filter,spec=byId.get(id);if(!spec)return;
      row.querySelectorAll('[data-g41-filter]').forEach(b=>b.disabled=true);button.classList.add('loading');
      try{
        await applyLive(id);
        player()?.patchPreferences?.({video:{filter:id,smooth:id==='bilinear'}});
        markLive(row,id);player()?.toast?.(`${spec.name} aplicado em tempo real ✓`);
      }catch(error){
        button.title=error?.message||'Não foi possível aplicar o filtro';player()?.toast?.(button.title,true);
      }finally{row.querySelectorAll('[data-g41-filter]').forEach(b=>{b.disabled=false;b.classList.remove('loading')})}
    }));
    const note=[...panel.querySelectorAll('.sf-g4-note')].find(node=>/Filtro|escala inteira|aspecto/i.test(node.textContent||''));
    if(note){note.classList.add('sf-g41-live-note');note.textContent='Filtros, Fit/Esticar e saturação agora mudam na hora. Apenas “Escala inteira” continua exigindo reinício do core.'}
    const apply=panel.querySelector('[data-g4-apply]');if(apply){apply.textContent='Reiniciar para aplicar Escala inteira';apply.title='O reinício só é necessário para opções internas do core, não para os filtros gráficos.'}
  }

  async function applySavedFilter(){
    expose();const api=player();if(!api?.active?.())return;const id=selected(api.preferences?.().video||{});
    try{await applyLive(id);api.patchPreferences?.({video:{filter:id,smooth:id==='bilinear'}});api.resize?.()}catch(error){api.toast?.(`Filtro ${byId.get(id)?.name||id}: ${error?.message||error}`,true)}
  }

  window.StormFlixGameVideo={catalog,selected,decorateOptions,preload:loadPackage,buildBundle,apply:applyLive,shaderRevision:SHADER_REV};
  expose();
  const observer=new MutationObserver(()=>{expose();enhanceVideoPanel()});observer.observe(document.documentElement,{childList:true,subtree:true});
  window.addEventListener('stormflix:game-menu-request',()=>setTimeout(enhanceVideoPanel,0));
  window.addEventListener('stormflix:game-preferences-changed',enhanceVideoPanel);
  window.addEventListener('stormflix:game-started',()=>{liveFilterId=null;setTimeout(applySavedFilter,120)});
  window.addEventListener('stormflix:game-closed',()=>{liveFilterId=null;liveApplyPromise=null});
})();
