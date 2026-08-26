/* StormFlix mobile/bootstrap recovery.
 * Fresh devices often have no selected-profile cookie yet. The legacy boot path
 * loads /home before the profile picker; if that request requires a profile the
 * app can be left with both login and shell hidden. Recover that state and let
 * the profile module perform selection first.
 */
(function(){
  if(typeof authenticated==='function'){
    const profileAwareAuthenticated=authenticated;
    authenticated=async function(){
      try{
        return await profileAwareAuthenticated();
      }catch(err){
        const recoverable=err&&(Number(err.status)===400||Number(err.status)===403||/profile|perfil/i.test(String(err.message||'')));
        if(!recoverable||!window.sfProfiles?.reload)throw err;
        const login=document.querySelector('#login');
        const shell=document.querySelector('#shell');
        login?.classList.add('hidden');
        shell?.classList.remove('hidden');
        if(typeof me!=='undefined'&&me){
          const label=document.querySelector('#user-label');
          if(label)label.textContent=me.display_name||me.username||'StormFlix';
          if(me.role!=='user')document.querySelector('#admin-link')?.classList.remove('hidden');
        }
        await window.sfProfiles.reload(true);
      }
    };
  }

  // Never leave a working browser on an unexplained empty gray page. If boot
  // has not selected login, app shell or profile picker after a reasonable
  // amount of time, expose the login screen and a useful diagnostic message.
  window.addEventListener('DOMContentLoaded',()=>{
    setTimeout(()=>{
      const login=document.querySelector('#login');
      const shell=document.querySelector('#shell');
      const picker=document.querySelector('#profile-picker');
      const allHidden=[login,shell,picker].every(el=>!el||el.classList.contains('hidden'));
      if(!allHidden)return;
      login?.classList.remove('hidden');
      const error=document.querySelector('#login-error');
      if(error&&!error.textContent)error.textContent='O StormFlix não conseguiu concluir a inicialização. Atualize a página; se continuar, verifique a conexão com o servidor.';
    },8000);
  });

  // Mobile browsers can keep an overlay body-lock after page restoration from
  // the back/forward cache. Clear locks only when their corresponding UI is not
  // actually open.
  window.addEventListener('pageshow',()=>{
    if(document.querySelector('#profile-hub')?.classList.contains('hidden'))document.body.classList.remove('profile-hub-open');
    if(document.querySelector('#detail-modal')?.classList.contains('hidden'))document.body.classList.remove('detail-open');
    if(document.querySelector('#player-modal')?.classList.contains('hidden'))document.body.classList.remove('modal-open');
  });
})();