/* StormFlix Games G4.1 video filters: curated RetroArch GLSL shader bundles. */
(function(){
  const SHADER_REV='235448f244bf676d135f7b25ea6b8e1eae41c4e4';
  const CDN=`https://cdn.jsdelivr.net/gh/libretro/glsl-shaders@${SHADER_REV}`;
  const bundleCache=new Map();
  const catalog=[
    {id:'pixel',name:'Pixel perfeito',short:'Pixel',kind:'native',cost:'Leve',description:'Nearest-neighbor nítido, sem suavização. Ideal para preservar cada pixel.'},
    {id:'bilinear',name:'Bilinear',short:'Bilinear',kind:'native',cost:'Leve',description:'Suavização simples do RetroArch. Reduz blocos sem alterar os sprites.'},
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
  function makeFile(name,content){
    if(typeof File==='function')return new File([content],name,{type:'application/octet-stream'});
    return {fileName:name,fileContent:content};
  }
  async function fetchAsset(path){
    const response=await fetch(`${CDN}/${path}`,{cache:'force-cache',mode:'cors'});
    if(!response.ok)throw new Error(`Filtro gráfico indisponível (${response.status})`);
    return response.blob();
  }
  function rewritePreset(text,rewrites){
    let out=text;
    for(const [from,to] of Object.entries(rewrites||{}))out=out.split(from).join(to);
    return out;
  }
  async function buildBundle(id){
    if(bundleCache.has(id))return bundleCache.get(id);
    const spec=byId.get(id);
    if(!spec||spec.kind!=='shader')return [];
    const promise=(async()=>{
      const [presetBlob,...assetBlobs]=await Promise.all([fetchAsset(spec.preset),...spec.assets.map(fetchAsset)]);
      const presetText=rewritePreset(await presetBlob.text(),spec.rewrites);
      const files=[makeFile(`stormflix-${id}.glslp`,presetText)];
      spec.assets.forEach((path,index)=>files.push(makeFile(fileName(path),assetBlobs[index])));
      return files;
    })().catch(error=>{bundleCache.delete(id);throw error});
    bundleCache.set(id,promise);
    return promise;
  }
  function decorateOptions(options,video){
    const id=selected(video),spec=byId.get(id)||byId.get('pixel');
    options.retroarchConfig={...(options.retroarchConfig||{}),video_smooth:id==='bilinear'};
    options.cache={...(options.cache||{}),shader:true};
    if(spec.kind==='shader'){
      options.shader=id;
      options.resolveShader=()=>buildBundle(id);
    }else{
      delete options.shader;
      delete options.resolveShader;
    }
    return options;
  }
  function expose(){
    const api=player();if(!api)return;
    api.videoFilters=()=>catalog.map(item=>({...item,assets:undefined,rewrites:undefined,preset:undefined}));
    api.videoFilter=()=>selected(api.preferences?.().video||{});
    api.preloadVideoFilter=id=>buildBundle(id);
  }
  function esc(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function costClass(cost){return String(cost||'').toLowerCase().replace(/[^a-z]/g,'')}
  function filterGridHTML(active){
    return `<div class="sf-g41-filter-grid">${catalog.map(item=>`<button type="button" class="sf-g41-filter-card ${active===item.id?'active':''}" data-g41-filter="${item.id}"><span class="sf-g41-filter-head"><b>${esc(item.name)}</b><i class="${costClass(item.cost)}">${esc(item.cost)}</i></span><small>${esc(item.description)}</small></button>`).join('')}</div>`;
  }
  function enhanceVideoPanel(){
    const panel=document.querySelector('[data-g4-panel]:not(.hidden)');if(!panel)return;
    const smooth=panel.querySelector('[data-video-smooth]');if(!smooth)return;
    const row=smooth.closest('.sf-g4-setting');if(!row||row.dataset.g41Filters==='1')return;
    const active=selected(player()?.preferences?.().video||{});
    row.dataset.g41Filters='1';row.classList.add('sf-g41-filter-setting');
    row.innerHTML=`<span class="sf-g41-filter-title"><b>Filtro gráfico</b><small>Shaders reais do RetroArch. Pixel/Bilinear são nativos; os demais usam GLSL.</small></span>${filterGridHTML(active)}`;
    row.querySelectorAll('[data-g41-filter]').forEach(button=>button.addEventListener('click',async()=>{
      const id=button.dataset.g41Filter,spec=byId.get(id);if(!spec)return;
      row.querySelectorAll('[data-g41-filter]').forEach(b=>b.disabled=true);button.classList.add('loading');
      try{
        if(spec.kind==='shader')await buildBundle(id);
        player()?.patchPreferences?.({video:{filter:id,smooth:id==='bilinear'}});
        row.querySelectorAll('[data-g41-filter]').forEach(b=>{
          b.disabled=false;b.classList.remove('loading');b.classList.toggle('active',b.dataset.g41Filter===id);
        });
      }catch(error){
        button.classList.remove('loading');row.querySelectorAll('[data-g41-filter]').forEach(b=>b.disabled=false);
        button.title=error?.message||'Não foi possível preparar o filtro';
      }
    }));
    const note=[...panel.querySelectorAll('.sf-g4-note')].find(node=>/Filtro|escala inteira|aspecto/i.test(node.textContent||''));
    if(note)note.textContent='Filtro gráfico, escala inteira e aspecto são aplicados com segurança após “Aplicar e reiniciar o jogo”. O save state e a SRAM são preservados.';
  }

  window.StormFlixGameVideo={catalog,selected,decorateOptions,preload:buildBundle,shaderRevision:SHADER_REV};
  expose();
  const observer=new MutationObserver(()=>{expose();enhanceVideoPanel()});
  observer.observe(document.documentElement,{childList:true,subtree:true});
  window.addEventListener('stormflix:game-menu-request',()=>setTimeout(enhanceVideoPanel,0));
  window.addEventListener('stormflix:game-preferences-changed',enhanceVideoPanel);
})();
