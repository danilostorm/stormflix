/* StormFlix screen bundle loader: navigation stays usable while optional UI is fetched. */
(function(){
  const loaded=new Set(),loading=new Map();
  let manifestPromise=null;
  function manifest(){
    if(!manifestPromise)manifestPromise=fetch('/bundles/manifest.json',{cache:'no-cache'}).then(response=>{
      if(!response.ok)throw new Error(`manifesto web ${response.status}`);
      return response.json();
    }).catch(error=>{manifestPromise=null;throw error});
    return manifestPromise;
  }
  function style(url){
    return new Promise((resolve,reject)=>{
      if(document.querySelector(`link[data-screen-bundle="${url}"]`))return resolve();
      const link=document.createElement('link');link.rel='stylesheet';link.href=url;link.dataset.screenBundle=url;
      link.onload=resolve;link.onerror=()=>reject(new Error(`falha ao carregar ${url}`));document.head.appendChild(link);
    });
  }
  function script(url){
    return new Promise((resolve,reject)=>{
      const node=document.createElement('script');node.src=url;node.async=true;node.dataset.screenBundle=url;
      node.onload=resolve;node.onerror=()=>reject(new Error(`falha ao carregar ${url}`));document.head.appendChild(node);
    });
  }
  async function load(name){
    if(loaded.has(name))return;
    if(loading.has(name))return loading.get(name);
    const pending=manifest().then(data=>{
      const entry=data?.screens?.[name];if(!entry)throw new Error(`tela ${name} ausente`);
      return Promise.all([style(entry.css),script(entry.js)]);
    }).then(()=>{loaded.add(name);window.dispatchEvent(new CustomEvent('stormflix:screen-loaded',{detail:{name}}))}).finally(()=>loading.delete(name));
    loading.set(name,pending);return pending;
  }
  window.sfLoadScreenBundle=load;
  document.addEventListener('click',event=>{
    const button=event.target.closest?.('#games-nav,#music-nav');if(!button)return;
    const name=button.id==='games-nav'?'games':'music';if(loaded.has(name))return;
    event.preventDefault();event.stopImmediatePropagation();
    load(name).then(()=>button.click()).catch(error=>{console.error(error);if(typeof sfToast==='function')sfToast('Não foi possível abrir esta área')});
  },true);
  const idle=window.requestIdleCallback||((callback)=>setTimeout(callback,1800));
  idle(()=>{load('games').catch(()=>{});load('music').catch(()=>{})},{timeout:3500});
})();
