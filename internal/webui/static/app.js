const $=s=>document.querySelector(s),api='/api/v1';let me=null;
async function request(path,opt={}){const r=await fetch(api+path,{...opt,headers:{'Content-Type':'application/json',...(opt.headers||{})}});const d=await r.json().catch(()=>({}));if(!r.ok)throw Object.assign(new Error(d.error||`HTTP ${r.status}`),{status:r.status});return d}
async function boot(){const setup=await request('/setup/status');if(setup.needs_setup){location.href='/admin/';return}try{me=await request('/auth/me');authenticated()}catch{showLogin()}}
function showLogin(){$('#login').classList.remove('hidden')}
function authenticated(){$('#login').classList.add('hidden');$('#user-label').textContent=me.display_name;$('#logout').classList.remove('hidden');if(me.role!=='user')$('#admin-link').classList.remove('hidden');Promise.all([loadLibraries(),loadMedia()]).catch(e=>message(e.message,true))}
$('#login-form').onsubmit=async e=>{e.preventDefault();try{me=await request('/auth/login',{method:'POST',body:JSON.stringify({username:$('#login-user').value,password:$('#login-pass').value})});authenticated()}catch(err){$('#login-error').textContent=err.message}}
$('#logout').onclick=async()=>{await request('/auth/logout',{method:'POST'}).catch(()=>{});location.reload()}
async function loadLibraries(){const a=await request('/libraries');const s=$('#library-filter');s.innerHTML='<option value="">Todas as bibliotecas</option>'+a.map(x=>`<option value="${x.id}">${escapeHTML(x.name)}</option>`).join('')}
async function loadMedia(){const q=$('#search').value.trim(),lib=$('#library-filter').value;const items=await request(`/media?limit=200&q=${encodeURIComponent(q)}&library_id=${encodeURIComponent(lib)}`);const root=$('#media');root.innerHTML='';if(!items.length){root.innerHTML='<div class="empty">Nenhuma mídia disponível para este perfil.</div>';return}for(const item of items){const f=$('#media-template').content.cloneNode(true);f.querySelector('.media-title').textContent=item.title;f.querySelector('.media-meta').textContent=`${item.extension.replace('.','').toUpperCase()} · ${formatBytes(item.size_bytes)}`;f.querySelector('.play').href=`${api}/media/${item.id}/stream`;root.appendChild(f)}}
let timer;$('#search').oninput=()=>{clearTimeout(timer);timer=setTimeout(loadMedia,250)};$('#library-filter').onchange=loadMedia;
function message(t,e=false){$('#message').textContent=t;$('#message').className=`message ${e?'error':'success'}`}
function formatBytes(b){if(!b)return'0 B';const u=['B','KB','MB','GB','TB'],i=Math.min(Math.floor(Math.log(b)/Math.log(1024)),u.length-1);return`${(b/1024**i).toFixed(i>2?2:1)} ${u[i]}`}
function escapeHTML(v){return String(v).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
boot().catch(e=>message(e.message,true));
