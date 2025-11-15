async function api(path, opt){
  const r = await fetch(path, {credentials:'same-origin', headers:{'Content-Type':'application/json'}, ...opt});
  if(!r.ok) throw new Error(await r.text());
  return r.json();
}
const app = document.getElementById('app');
function el(tag, attrs={}, children=[]){ const e=document.createElement(tag); Object.entries(attrs).forEach(([k,v])=>{ if(k==='class') e.className=v; else e.setAttribute(k,v) }); children.forEach(c=> e.append(typeof c==='string'? document.createTextNode(c): c)); return e; }
function section(title){ const card=el('div',{class:'card'}); card.append(el('h2',{},[title])); return card }

async function renderTasks(){
  const s = section('任务列表');
  const controls = el('div',{class:'row'});
  const siteSel = el('select'); siteSel.append(el('option',{value:''},['全部站点']), el('option',{value:'cmct'},['CMCT']), el('option',{value:'hdsky'},['HDSKY']), el('option',{value:'mteam'},['MTEAM']));
  const q = el('input',{placeholder:'按标题/哈希搜索'});
  const sortSel = el('select'); sortSel.append(el('option',{value:'created_at_desc'},['按创建时间倒序']), el('option',{value:'created_at_asc'},['按创建时间正序']));
  controls.append(el('div',{class:'card'},[el('label',{},['站点']), siteSel]), el('div',{class:'card'},[el('label',{},['查询']), q]), el('div',{class:'card'},[el('label',{},['排序']), sortSel]));
  s.append(controls);
  const table = el('table',{class:'list'}); const thead = el('thead');
  thead.append(el('tr',{},['站点','标题','分类','标签','哈希','已下载','已推送','创建时间'].map(t=>el('th',{},[t])))); table.append(thead);
  const tbody = el('tbody'); table.append(tbody); s.append(table);

  async function load(){
    const params = new URLSearchParams(); if(siteSel.value) params.set('site', siteSel.value); if(q.value) params.set('q', q.value); params.set('sort', sortSel.value);
    const data = await api('/api/tasks?'+params.toString());
    tbody.replaceChildren();
    data.items.forEach(it=>{
      const tr = el('tr');
      tr.append(el('td',{},[it.site_name||'']), el('td',{},[it.title||'']), el('td',{},[it.category||'']), el('td',{},[it.tag||'']), el('td',{},[it.torrent_hash||'']), el('td',{},[it.is_downloaded? '是':'否']), el('td',{},[(it.is_pushed? '是':'否')]), el('td',{},[it.created_at||'']));
      tbody.append(tr);
    });
  }
  siteSel.onchange=load; q.oninput=()=>{ clearTimeout(q._t); q._t=setTimeout(load,300) }; sortSel.onchange=load;
  await load();
  app.replaceChildren(s);
}

renderTasks();