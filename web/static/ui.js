async function api(path, opt){
  if(path === '/api/logs' && location.hash !== '#logs'){
    return {lines: [], path: '', truncated: false};
  }
  const r = await fetch(path, {credentials:'same-origin', headers:{'Content-Type':'application/json'}, ...opt});
  const isJSON = (r.headers.get('content-type')||'').includes('application/json');
  if(!r.ok){ const msg = isJSON? (await r.text()): (await r.text()); throw new Error(msg||('HTTP '+r.status)); }
  return isJSON? r.json(): r.text();
}
const toast = (msg, ok)=>{ const t = el('div',{class:'toast'}); t.style.background = ok? '#22c55e':'#ef4444'; t.style.color='#0b1220'; t.textContent = msg; document.body.appendChild(t); setTimeout(()=>t.remove(), 2000); };
const app = document.getElementById('app');

function el(tag, attrs={}, children=[]) {
  const e = document.createElement(tag);
  Object.entries(attrs).forEach(([k,v])=>{ if(k==='class') e.className=v; else e.setAttribute(k,v) });
  children.forEach(c=>{ if(typeof c==='string') e.appendChild(document.createTextNode(c)); else e.appendChild(c) });
  return e;
}

function section(title){
  const card = el('div',{class:'card'});
  card.append(el('h2',{},[title]));
  return card;
}

async function renderGlobal(){
  const g = await api('/api/global');
  const s = section('全局设置');
  const form = el('form');
  const warn = el('div',{class:'card'}); warn.style.background='#ef4444'; warn.style.color='#0b1220'; warn.style.display='none'; warn.append(el('strong',{},['警告：未设置下载目录，后台任务不会启动，请先设置并保存'])); s.append(warn);
  form.append(el('label',{},['默认间隔(分钟)']));
    const mins = el('input',{value: (g.default_interval_minutes? Math.max(5, parseInt(g.default_interval_minutes)):5) }); mins.type='number'; mins.min=String(5); mins.step='1';
  form.append(mins);
  form.append(el('label',{},['种子下载目录']));
  const dl = el('input',{value: g.download_dir||'', placeholder:'保存 .torrent 种子文件的目录(绝对路径或相对 ~/.pt-tools)'});
  const tip = el('div',{class:'muted'},['绝对路径将直接使用；相对路径会拼接为 ', (function(){ const span=document.createElement('code'); span.textContent='~/.pt-tools/<输入值>'; return span; })(), ' 并自动创建目录']);
  const tip2 = el('div',{class:'muted'},['此目录用于保存已下载的 ', (function(){ const code=document.createElement('code'); code.textContent='.torrent'; return code; })(), ' 种子文件；并非 qBittorrent 中文件数据的保存路径']);
  form.append(dl);
  form.append(tip);
  form.append(tip2);
  const row = el('div',{class:'row'});
  const colChk = el('div',{class:'card'});
  const chk = el('input',{type:'checkbox'}); chk.checked = !!g.download_limit_enabled;
  const sw1 = el('label',{class:'switch'}); sw1.append(chk, el('span',{class:'slider'}));
  colChk.append(el('label',{},['启用限速'])); colChk.append(sw1);
  const colSpeed = el('div',{class:'card'});
  colSpeed.append(el('label',{},['下载限速(MB/s)'])); const speed = el('input',{value:g.download_speed_limit||20}); colSpeed.append(speed);
  const colSize = el('div',{class:'card'});
  colSize.append(el('label',{},['最大种子大小(GB)'])); const size = el('input',{value:g.torrent_size_gb||200}); colSize.append(size);
  const colAuto = el('div',{class:'card'});
  const auto = el('input',{type:'checkbox'}); auto.checked = !!g.auto_start; const swAuto = el('label',{class:'switch'}); swAuto.append(auto, el('span',{class:'slider'})); colAuto.append(el('label',{},['自动启动任务'])); colAuto.append(swAuto);
  row.append(colChk,colSpeed,colSize);
  row.append(colAuto);
  form.append(row);
  const btn = el('button',{},['保存']); btn.onclick = async (e)=>{e.preventDefault(); btn.disabled=true;
    const payload = {
      default_interval_minutes: Math.max(5, parseInt(mins.value)||10),
      download_dir: dl.value,
      download_limit_enabled: chk.checked,
      download_speed_limit: parseInt(speed.value)||20,
      torrent_size_gb: parseInt(size.value)||500,
      auto_start: auto.checked
    };
    if(!payload.download_dir){ btn.disabled=false; toast('下载目录不能为空', false); warn.style.display=''; return; }
    try {
      await api('/api/global',{method:'POST',body:JSON.stringify(payload)}); toast('已保存', true);
      warn.style.display='none';
    } catch(err){ toast(err.message||'保存失败', false); }
    btn.disabled=false; };
  form.append(el('div',{},[btn])); s.append(form);
  if(!dl.value){ warn.style.display=''; }
  app.replaceChildren(s);
}

async function renderQbit(){
  const qb = await api('/api/qbit');
  const s = section('qBittorrent'); const f = el('form');
  const en = el('input',{type:'checkbox'}); en.checked=!!qb.enabled; const sw2 = el('label',{class:'switch'}); sw2.append(en, el('span',{class:'slider'})); f.append(el('label',{},['启用'])); f.append(sw2);
  const url = el('input',{value:qb.url}); f.append(el('label',{},['URL'])); f.append(url);
  const user = el('input',{value:qb.user}); f.append(el('label',{},['用户'])); f.append(user);
  const pwd = el('input',{type:'password', value:qb.password}); f.append(el('label',{},['密码'])); f.append(pwd);
  const btn = el('button',{},['保存']); btn.onclick = async (e)=>{e.preventDefault(); btn.disabled=true;
    const payload = {enabled:en.checked,url:url.value.trim(),user:user.value.trim(),password:pwd.value.trim()};
    if(!payload.url || !payload.user || !payload.password){ toast('URL、用户名、密码均为必填', false); btn.disabled=false; return; }
    try { await api('/api/qbit',{method:'POST',body:JSON.stringify(payload)}); toast('已保存', true); }
    catch(err){ toast(err.message||'保存失败', false); }
    btn.disabled=false; };
  f.append(btn); s.append(f); app.replaceChildren(s);
}

async function renderSites(){
  const sites = await api('/api/sites'); const s = section('站点与RSS');
  const table = el('table',{class:'list'});
  const thead = el('thead'); thead.append(el('tr',{},[el('th',{},['站点']),el('th',{},['启用']),el('th',{},['操作'])]));
  table.append(thead);
  const tbody = el('tbody');
  Object.keys(sites).forEach(name=>{
    const tr = el('tr');
    tr.append(el('td',{},[name]));
    const enabled = !!(sites[name].enabled||false);
    const toggle = el('input',{type:'checkbox'}); toggle.checked=enabled;
    const sw = el('label',{class:'switch'}); sw.append(toggle, el('span',{class:'slider'}));
    toggle.onchange = async ()=>{
      const sc = sites[name]; sc.enabled = toggle.checked;
      try { await api('/api/sites/'+name,{method:'POST',body:JSON.stringify(sc)}); toast('已保存', true); } catch(err){ toast(err.message||'保存失败', false); }
    };
    tr.append(el('td',{},[sw]));
    const manage = el('button',{class:'btn btn-secondary'},['管理']); manage.onclick = ()=> renderSite(name);
    const del = el('button',{class:'btn btn-danger'},['删除']); del.onclick = async ()=>{
      if(name==='cmct'||name==='hdsky'||name==='mteam'){ toast('预置站点不可删除', false); return; }
      try { await api('/api/sites?name='+encodeURIComponent(name),{method:'DELETE'}); toast('已删除', true); renderSites(); }
      catch(err){ toast(err.message||'删除失败', false); }
    };
    tr.append(el('td',{},[manage,' ',del])); tbody.append(tr);
  });
  table.append(tbody); s.append(table); app.replaceChildren(s);
  const add = el('button',{class:'btn btn-secondary'},['新增站点']); add.onclick = async ()=>{
    const name = prompt('站点标识(cmct/hdsky/mteam或自定义)'); if(!name) return;
    if(sites[name]){ toast('站点已存在', false); return; }
    const lower = name.toLowerCase();
    let payload = {enabled:false,rss:[],auth_method:'cookie',cookie:'',api_key:'',api_url:''};
    if(lower==='mteam'){ payload.auth_method='api_key'; payload.api_url='https://api.m-team.cc/api'; }
    try { await api('/api/sites/'+lower,{method:'POST',body:JSON.stringify(payload)}); toast('已新增站点', true); renderSites(); }
    catch(err){ toast(err.message||'新增失败', false); }
  };
  s.append(add);
}

async function renderSite(name){
  const sc = await api('/api/sites/'+name);
  const s = section('站点: '+name); const f = el('form');
  const en = el('input',{type:'checkbox'}); en.checked=!!(sc.enabled||false);
  const sw = el('label',{class:'switch'}); sw.append(en, el('span',{class:'slider'}));
  f.append(el('label',{},['启用'])); f.append(sw);
  const am = el('input',{value:sc.auth_method||'', readonly:''}); am.readOnly = true; f.append(el('label',{},['认证方式'])); f.append(am);
  const ck = el('input',{value:sc.cookie||''});
  const ak = el('input',{value:sc.api_key||''});
  const au = el('input',{value:sc.api_url||'', readonly:''}); au.readOnly = true;
  const rowAuth = el('div',{class:'row'});
  const colCookie = el('div',{class:'card'}); colCookie.append(el('label',{},['Cookie']), ck);
  const colApiKey = el('div',{class:'card'}); colApiKey.append(el('label',{},['API Key']), ak);
  const colApiUrl = el('div',{class:'card'}); colApiUrl.append(el('label',{},['API Url']), au);
  function applyAuthVisibility(){
    const method = (sc.auth_method||'').toLowerCase();
    // 按默认认证方式展示字段
    const isCookie = method==='cookie';
    colCookie.style.display = isCookie? '' : 'none';
    colApiKey.style.display = isCookie? 'none' : '';
    colApiUrl.style.display = isCookie? 'none' : '';
  }
  applyAuthVisibility();
  rowAuth.append(colCookie, colApiKey, colApiUrl);
  f.append(rowAuth);
  f.append(el('h3',{},['RSS 列表']));
  const rssTable = el('table',{class:'list'});
  const rthead = el('thead');
  const hrow = el('tr');
  hrow.append(
    el('th',{},['名称']),
    el('th',{},['链接']),
    el('th',{},['分类']),
    el('th',{},['标签']),
    el('th',{},['间隔(分钟)']),
    el('th',{},['操作'])
  );
  rthead.append(hrow); rssTable.append(rthead);
  const rtbody = el('tbody');
  const drawRow = (r, idx)=>{
    const tr = el('tr');
    tr.dataset.rssId = r.id||'';
    const nameI = el('input',{value:r.name||'', placeholder:'如：CMCT'});
    const urlI = el('input',{value:r.url||'', placeholder:'如：https://example/rss/xxx'});
    const tip = el('span',{class:'tip'},[el('span',{class:'tiptext'},[''])]);
    const catI = el('input',{value:r.category||'', placeholder:'如：Tv/Mv'});
    const tagI = el('input',{value:r.tag||'', placeholder:'如：CMCT/HDSKY'});
    const minI = el('input',{value: Math.max(5, r.interval_minutes||5), placeholder:'5-1440'}); minI.type='number'; minI.min=String(5); minI.max='1440'; minI.step='1';
    const urlCell = el('td',{class:'col-url'});
    urlCell.append(urlI, tip);
    tr.append(el('td',{class:'col-name'},[nameI]), urlCell, el('td',{class:'col-category'},[catI]), el('td',{class:'col-tag'},[tagI]), el('td',{class:'col-interval'},[minI]));
    const del = el('button',{class:'btn btn-danger'},['删除']);
    del.onclick = async (e)=>{
      e.preventDefault();
      if(!confirm('确定删除该 RSS 行？')) return;
      try { await api('/api/sites/'+name+'?id='+encodeURIComponent(tr.dataset.rssId||''), {method:'DELETE'}); toast('已删除', true); tr.remove(); }
      catch(err){ toast(err.message||'删除失败', false); }
    };
    // URL 正则校验与错误提示气泡
    function validateUrl(){
      const pattern = /^(https?:\/\/)[\w.-]+(\/.*)?$/i;
      const ok = pattern.test(urlI.value.trim());
      if(!ok){ urlI.classList.add('input-error'); tip.classList.add('error','show'); tip.querySelector('.tiptext').textContent = 'URL 格式不正确，需以 http/https 开头'; }
      else{ urlI.classList.remove('input-error'); tip.classList.remove('error','show'); tip.querySelector('.tiptext').textContent = '该目录为种子文件存放位置（非实际下载数据路径），推荐按 站点/分类 组织，如 cmct/mv 或 mteam/tvs'; }
      return ok;
    }
    urlI.addEventListener('blur', validateUrl);
    minI.addEventListener('input', ()=>{ if(parseInt(minI.value)<5||parseInt(minI.value)>1440){ minI.classList.add('input-error'); } else { minI.classList.remove('input-error'); } });
    tr.append(el('td',{class:'actions'},[del])); rtbody.append(tr);
  };
  sc.rss = sc.rss||[]; sc.rss.forEach((r,i)=>drawRow(r,i));
  const addRow = el('button',{class:'btn btn-secondary'},['新增 RSS']); addRow.onclick = (e)=>{ e.preventDefault(); sc.rss = sc.rss||[]; const idx = sc.rss.push({id:'',name:'',url:'',category:'',tag:'',interval_minutes:10}) - 1; drawRow(sc.rss[idx], idx); };
  rssTable.append(rtbody); f.append(rssTable); f.append(addRow);
  const btn = el('button',{},['保存']); btn.onclick = async (e)=>{e.preventDefault(); btn.disabled=true;
    const scPost = {enabled:en.checked, auth_method:sc.auth_method, cookie:ck.value, api_key:ak.value, api_url:sc.api_url, rss:[]};
    const rows = Array.from(rtbody.querySelectorAll('tr'));
    scPost.rss = rows.map(tr=>{
      const ins = tr.querySelectorAll('input');
      const name = (ins[0]?.value||'').trim();
      const url = (ins[1]?.value||'').trim();
      const category = ins[2]?.value||'';
      const tag = ins[3]?.value||'';
      const mins = parseInt(ins[4]?.value||'10')||10;
      return {name, url, category, tag, interval_minutes: mins};
    }).filter(r=> (r.name && r.url));
    if(scPost.enabled){
      const method = (sc.auth_method||'').toLowerCase();
      const needApi = method==='api_key';
      const hasAuth = needApi? (scPost.api_key && scPost.api_key.trim()) : (scPost.cookie && scPost.cookie.trim());
      if(!hasAuth){ toast(needApi? '启用站点时必须设置 API Key' : '启用站点时必须设置 Cookie', false); btn.disabled=false; return; }
      if(!scPost.rss || scPost.rss.length===0){ toast('启用站点时 RSS 列表不能为空', false); btn.disabled=false; return; }
    }
    try { await api('/api/sites/'+name,{method:'POST',body:JSON.stringify(scPost)}); toast('已保存', true); }
    catch(err){ toast(err.message||'保存失败', false); }
    btn.disabled=false; };
  f.append(btn); s.append(f); app.replaceChildren(s);
}

async function renderPassword(){
  const s = section('修改密码'); const f = el('form');
  const user = el('input',{value:'admin'}); f.append(el('label',{},['用户名'])); f.append(user);
  const old = el('input',{type:'password'}); f.append(el('label',{},['原密码'])); f.append(old);
  const nw = el('input',{type:'password'}); f.append(el('label',{},['新密码'])); f.append(nw);
  const btn = el('button',{},['保存']); btn.onclick = async (e)=>{e.preventDefault(); btn.disabled=true;
    try { await api('/api/password',{method:'POST',body:JSON.stringify({username:user.value,old:old.value,new:nw.value})}); toast('已保存', true); }
    catch(err){ toast(err.message||'保存失败', false); }
    btn.disabled=false; };
  f.append(btn); s.append(f); app.replaceChildren(s);
}

function route(){
  const h = location.hash.replace('#','');
  if(window.__logsTimer){ clearInterval(window.__logsTimer); window.__logsTimer=null }
  if(h==='global') return renderGlobal();
  if(h==='qbit') return renderQbit();
  if(h==='sites') return renderSites();
  if(h==='tasks') return renderTasks();
  if(h==='logs') return (async()=>{ const s = section('系统日志'); const ctrl = document.createElement('div'); ctrl.className='toolbar'; const kwWrap = document.createElement('div'); kwWrap.className='card'; const kw = document.createElement('input'); kw.placeholder='关键字过滤(大小写不敏感)'; kwWrap.append(kw); const levels = ['DEBUG','INFO','WARN','ERROR']; const levelBox = document.createElement('div'); levelBox.className='card'; levels.forEach((L)=>{ const c=document.createElement('input'); c.type='checkbox'; c.checked=true; c.dataset.level=L; const lbl=document.createElement('label'); lbl.textContent=L; levelBox.append(lbl, c) }); const pauseBtn = document.createElement('button'); pauseBtn.className='btn btn-secondary'; pauseBtn.textContent='暂停刷新'; const manualBtn = document.createElement('button'); manualBtn.className='btn btn-secondary'; manualBtn.textContent='手动刷新'; const stat = document.createElement('div'); stat.className='muted'; ctrl.append(kwWrap, levelBox, pauseBtn, manualBtn, stat); const box = document.createElement('pre'); box.className='card'; box.style.whiteSpace='pre'; box.style.maxHeight='70vh'; box.style.overflow='auto'; box.style.fontFamily='ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace'; box.style.wordBreak='break-word'; const tip = document.createElement('div'); tip.className='muted'; s.append(ctrl, tip, box); let timer=null; let pending=false; function applyFilter(lines){ const re = kw.value? new RegExp(kw.value,'i'):null; const checks = Array.from(levelBox.querySelectorAll('input[type=checkbox]')); const sel = new Set(checks.filter(c=>c.checked).map(c=>c.dataset.level)); const out=[]; for(const line of lines){ const has=levels.some(L=> line.includes(L)); let passLevel=true; if(has){ passLevel = [...sel].some(L=> line.includes(L)); } if(re && !re.test(line)) continue; if(passLevel) out.push(line); } stat.textContent = '匹配行数: '+out.length+(kw.value? (' | 关键字: '+kw.value):''); return out; } async function load(){ if(pending) return; pending=true; try{ const data = await api('/api/logs'); const lines = applyFilter(data.lines); box.textContent = lines.join('\n'); box.scrollTop = box.scrollHeight; tip.textContent = data.truncated? ('日志仅显示最近5000行，完整日志请查看：'+data.path):('日志路径：'+data.path); } catch(e){ box.textContent = '日志加载失败: '+e.message } finally { pending=false } } function start(){ if(timer) return; timer=setInterval(load,2000) } function stop(){ if(!timer) return; clearInterval(timer); timer=null } pauseBtn.onclick=()=>{ if(timer){ stop(); pauseBtn.textContent='继续刷新' } else { start(); pauseBtn.textContent='暂停刷新' } }; manualBtn.onclick=()=>load(); kw.oninput=()=>{ clearTimeout(kw._t); kw._t=setTimeout(load,200) }; levelBox.querySelectorAll('input[type=checkbox]').forEach(c=> c.onchange=load); await load(); start(); app.replaceChildren(s) })();
  if(h==='password') return renderPassword();
  renderGlobal();
}
window.addEventListener('hashchange', route);
route();

async function renderTasks(){
  const s = section('任务列表');
  const controls = el('div',{class:'row'});
  const siteSel = el('select'); siteSel.append(el('option',{value:''},['全部站点']));
  try{
    const sites = await api('/api/sites');
    Object.keys(sites).forEach(name=> siteSel.append(el('option',{value:name},[name.toUpperCase()])));
  }catch(e){}
  const q = el('input',{placeholder:'按标题/哈希搜索'});
  const sortSel = el('select'); sortSel.append(el('option',{value:'created_at_desc'},['按创建时间倒序']), el('option',{value:'created_at_asc'},['按创建时间正序']));
  const filters = el('div',{class:'card'});
  const fDownloaded = el('input',{type:'checkbox'}); const fPushed = el('input',{type:'checkbox'}); const fExpired = el('input',{type:'checkbox'});
  const queryBtn = el('button',{class:'btn'},['查询']); queryBtn.onclick=()=>load();
  filters.append(el('label',{},['已下载']), fDownloaded, el('label',{},['已推送']), fPushed, el('label',{},['已过期']), fExpired, queryBtn);
  controls.append(el('div',{class:'card'},[el('label',{},['站点']), siteSel]), el('div',{class:'card'},[el('label',{},['查询']), q]), el('div',{class:'card'},[el('label',{},['排序']), sortSel]), filters);
  const actions = el('div',{class:'card'});
  const exportBtn = el('button',{class:'btn btn-secondary'},['导出 CSV']);
  exportBtn.onclick = async ()=>{
    const params = buildParams();
    const data = await api('/api/tasks?'+params);
    const rows = [['站点','标题','分类','标签','哈希','已下载','已推送','创建时间','最后检查','是否过期','免费等级','免费结束','推送时间','重试次数','最后错误']].concat(
      data.items.map(it=>[
        it.siteName||'', it.title||'', it.category||'', it.tag||'', it.torrentHash||'', it.isDownloaded? '是':'否', (it.isPushed? '是':'否'), it.createdAt||'',
        it.lastCheckTime||'', it.isExpired? '是':'否', it.freeLevel||'', it.freeEndTime||'', it.pushTime||'', it.retryCount||0, it.lastError||''
      ])
    );
    const csv = rows.map(r=>r.map(x=>`"${String(x).replace(/"/g,'""')}"`).join(',')).join('\n');
    const blob = new Blob([csv], {type:'text/csv'}); const url = URL.createObjectURL(blob); const a=document.createElement('a'); a.href=url; a.download='tasks.csv'; a.click(); URL.revokeObjectURL(url);
  };
  actions.append(exportBtn); s.append(controls, actions);

  const table = el('table',{class:'list'}); const thead = el('thead');
  thead.append(el('tr',{},['站点','标题','分类','标签','哈希','是否免费','优惠级别','过期时间','已下载','已推送','创建时间','最后检查','是否过期'].map(t=>el('th',{},[t])))); table.append(thead);
  const tbody = el('tbody'); table.append(tbody); s.append(table);
  const pager = el('div',{class:'row'}); const pageSel = el('select'); [20,50,100].forEach(n=> pageSel.append(el('option',{value:String(n)},[String(n)]))); const pageInfo = el('div',{class:'card'}); pager.append(el('div',{class:'card'},[el('label',{},['每页']), pageSel]), pageInfo); s.append(pager);

  let curPage = 1;
  function buildParams(){
    const p = new URLSearchParams(); if(siteSel.value) p.set('site', siteSel.value); if(q.value) p.set('q', q.value); p.set('sort', sortSel.value);
    if(fDownloaded.checked) p.set('downloaded','1'); if(fPushed.checked) p.set('pushed','1'); if(fExpired.checked) p.set('expired','1');
    p.set('page', String(curPage)); p.set('page_size', pageSel.value||'20');
    return p.toString();
  }

  async function load(){
    const params = buildParams();
    const data = await api('/api/tasks?'+params);
    tbody.replaceChildren();
    data.items.forEach(it=>{
      const tr = el('tr');
      const lastCheck = it.lastCheckTime? new Date(it.lastCheckTime).toLocaleString(): '';
      tr.append(
        el('td',{},[it.siteName||'']),
        el('td',{class:'col-title',title:it.title||''},[it.title||''] ),
        el('td',{},[it.category||'']),
        el('td',{},[it.tag||'']),
        el('td',{class:'col-hash',title:it.torrentHash||''},[it.torrentHash||'']),
        el('td',{},[it.isFree? '是':'否']),
        el('td',{},[it.freeLevel||'']),
        el('td',{},[it.freeEndTime? new Date(it.freeEndTime).toLocaleString(): '' ]),
        el('td',{},[it.isDownloaded? '是':'否']),
        el('td',{},[(it.isPushed? '是':'否')]),
        el('td',{},[it.createdAt||'']),
        el('td',{class:'col-lastcheck',title:lastCheck},[lastCheck]),
        el('td',{class:'col-expired'},[it.isExpired? '是':'否'])
      );
      tbody.append(tr);
    });
    updatePageInfo(data.total||0, data.page||1, data.page_size||parseInt(pageSel.value||'20'));
  }
  siteSel.onchange=()=>{ curPage=1; load() }; q.oninput=()=>{ curPage=1; clearTimeout(q._t); q._t=setTimeout(load,300) }; sortSel.onchange=()=>{ curPage=1; load() }; fDownloaded.onchange=()=>{ curPage=1; load() }; fPushed.onchange=()=>{ curPage=1; load() }; fExpired.onchange=()=>{ curPage=1; load() };
  pageSel.onchange=()=>{ curPage=1; load() };
  function updatePageInfo(total, page, size){
    const pages = Math.max(1, Math.ceil((total||0)/(size||20)));
    const info = el('span',{},[`共 ${total||0} 条 | 第 ${page}/${pages} 页`]);
    const prevBtn = el('button',{class:'btn', title:'上一页'},['上一页']);
    const nextBtn = el('button',{class:'btn', title:'下一页'},['下一页']);
    prevBtn.onclick = ()=>{ if(curPage>1){ curPage--; load() } };
    nextBtn.onclick = ()=>{ if(curPage<pages){ curPage++; load() } };
    if (curPage<=1) prevBtn.setAttribute('disabled','true');
    if (curPage>=pages) nextBtn.setAttribute('disabled','true');
    pageInfo.replaceChildren(info, el('span',{},[' ']), prevBtn, el('span',{},[' ']), nextBtn);
  }
  await load();
  app.replaceChildren(s);
}