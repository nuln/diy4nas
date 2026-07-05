#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html><html lang="zh-CN"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>App Center</title><style>
*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0f1219;--bg2:#1a1f2e;--bg3:#242b3d;--fg:#e2e8f0;--fg2:#94a3b8;--accent:#3b82f6;--green:#22c55e;--red:#ef4444;--radius:12px}
body{font-family:-apple-system,sans-serif;background:var(--bg);color:var(--fg);padding:16px}
h1{font-size:1.3rem;margin-bottom:4px}.sub{font-size:.8rem;color:var(--fg2);margin-bottom:12px}
.tb{display:flex;gap:4px;margin-bottom:12px;flex-wrap:wrap}
.tb a{padding:6px 14px;border-radius:20px;font-size:.8rem;color:var(--fg2);text-decoration:none;cursor:pointer}
.tb a.act{background:var(--accent);color:#fff}.tb a:hover:not(.act){background:var(--bg3)}
.sb{width:100%;padding:10px 14px;background:var(--bg2);border:1px solid transparent;border-radius:var(--radius);color:var(--fg);font-size:.9rem;outline:none;margin-bottom:12px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:12px}
.card{background:var(--bg2);border-radius:var(--radius);overflow:hidden;cursor:pointer;transition:.2s}
.card:hover{transform:translateY(-2px);box-shadow:0 8px 24px rgba(0,0,0,.4)}
.card-body{padding:14px}.card-name{font-weight:600;font-size:.95rem;margin-bottom:4px}
.card-desc{color:var(--fg2);font-size:.75rem;line-height:1.4;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.card-meta{display:flex;gap:6px;margin-top:8px;font-size:.7rem;flex-wrap:wrap}
.card-meta span{padding:2px 8px;border-radius:10px;background:var(--bg3);color:var(--fg2)}
.card-footer{padding:10px 14px;border-top:1px solid var(--bg3);display:flex;justify-content:space-between;align-items:center}
.btn{display:inline-flex;align-items:center;gap:4px;padding:6px 14px;border:none;border-radius:8px;font-size:.75rem;font-weight:600;cursor:pointer;transition:.2s}
.btn.pri{background:var(--accent);color:#fff}.btn.dan{background:rgba(239,68,68,.15);color:var(--red)}.btn.out{background:var(--bg3);color:var(--fg)}
.bdg{display:inline-block;padding:2px 8px;border-radius:10px;font-size:.7rem}
.bdg.ins{background:rgba(34,197,94,.15);color:var(--green)}
.pg{display:none}.pg.act{display:block}
.modal{display:none;position:fixed;inset:0;background:rgba(0,0,0,.6);z-index:50;align-items:center;justify-content:center}
.modal.show{display:flex}
.modal-c{background:var(--bg2);border-radius:var(--radius);width:92%;max-width:500px;max-height:80vh;overflow-y:auto;padding:20px;animation:up .25s}
@keyframes up{from{opacity:0;transform:translateY(16px)}to{opacity:1;transform:translateY(0)}}
.modal-c h2{margin-bottom:8px;font-size:1.05rem}
.modal-c .desc{color:var(--fg2);font-size:.8rem;line-height:1.5;margin-bottom:12px}
.modal-c .info dt{color:var(--fg2);font-size:.7rem;margin-top:6px}.modal-c .info dd{font-size:.85rem}
.modal-c .acts{display:flex;gap:8px;margin-top:14px;flex-wrap:wrap}
input{width:100%;padding:8px 10px;background:var(--bg3);border:1px solid transparent;border-radius:8px;color:var(--fg);font-size:.85rem;outline:none;margin-bottom:6px}
input:focus{border-color:var(--accent)}
.src-row{display:flex;gap:6px;margin-bottom:6px;align-items:center}
.src-row input{flex:1;margin-bottom:0}
.src-row input[type=checkbox]{width:auto;flex:0}
.src-row button{flex:0;padding:6px 10px;background:var(--bg3);border:none;border-radius:6px;color:var(--red);cursor:pointer;font-size:1rem}
.toast{position:fixed;bottom:24px;left:50%;transform:translateX(-50%);background:var(--bg3);color:var(--fg);padding:8px 16px;border-radius:8px;font-size:.8rem;z-index:100;display:none}
</style></head><body>
<h1>App Center</h1>
<div class="sub" id="cnt"></div>
<div class="tb">
<a class="act" data-v="all">全部</a>
<a data-v="sources">源管理</a>
</div>
<input class="sb" id="s" placeholder="搜索应用..." oninput="f()">
<div id="main" class="pg act">
<div class="grid" id="g"></div>
</div>
<div id="sources" class="pg">
<div class="card"><div style="font-weight:600;margin-bottom:10px">第三方应用源</div>
<div id="src-list"></div>
<button class="btn out" onclick="addSource()" style="margin-top:6px">+ 添加源</button>
<button class="btn pri" onclick="saveSources()" style="margin-top:6px">保存</button>
</div></div>

<div class="modal" id="modal"><div class="modal-c">
<h2 id="md-n"></h2><p class="desc" id="md-d"></p>
<dl class="info"><dt>版本</dt><dd id="md-v"></dd><dt>端口</dt><dd id="md-p"></dd><dt>类型</dt><dd id="md-t"></dd></dl>
<div class="acts">
<button class="btn pri" id="md-btn" onclick="doInstall()">安装</button>
<button class="btn dan" id="md-unbtn" style="display:none" onclick="doUninstall()">卸载</button>
<button class="btn out" onclick="closeModal()">关闭</button>
</div></div></div>
<div class="toast" id="toast"></div>

<script>
let apps=[],cur='',sources=[];
const ts=(m,t=2000)=>{const e=document.getElementById('toast');e.textContent=m;e.style.display='block';clearTimeout(e._t);e._t=setTimeout(()=>e.style.display='none',t)};
const q=(a,cb)=>{const x=new XMLHttpRequest();x.open('GET','api.cgi?action='+a);x.onload=()=>{try{cb(JSON.parse(x.responseText))}catch(e){cb({})}};x.send()};
const p=(a,d,cb)=>{const x=new XMLHttpRequest();x.open('POST','api.cgi?action='+a);x.setRequestHeader('Content-Type','application/x-www-form-urlencoded');x.onload=()=>{try{cb(JSON.parse(x.responseText))}catch(e){cb({})}};x.send(d)};

document.querySelectorAll('.tb a').forEach(a=>a.onclick=()=>{
const v=a.dataset.v;document.querySelectorAll('.tb a').forEach(x=>x.classList.toggle('act',x.dataset.p===v));
document.getElementById('main').classList.toggle('act',v==='all');document.getElementById('sources').classList.toggle('act',v==='sources');
if(v==='sources')loadSources()});

async function load(){
document.getElementById('g').innerHTML='<div style=text-align:center;padding:40px;color:var(--fg2)>加载中...</div>';
const r=await fetch('api.cgi?action=catalog');const d=await r.json();apps=d.apps||[];
let html='';let cnt=0;
for(const a of apps){
const s=await(await fetch('api.cgi?action=status&slug='+a.slug)).json();
a.installed=s.installed;a.running=s.running;
html+=`<div class=card onclick=detail('${a.slug}')><div class=card-body><div class=card-name>${a.display_name||a.slug}</div><div class=card-desc>${(a.desc||'').slice(0,120)}</div><div class=card-meta><span>${a.app_type||'?'}</span><span>${a.category||''}</span></div></div><div class=card-footer><span style=font-size:.7rem;color:var(--fg2)>v${a.version||'-'}</span>${a.installed?'<span class="bdg ins">已安装</span>':'<span class="btn pri">安装</span>'}</div></div>`;
cnt++}
document.getElementById('g').innerHTML=html;document.getElementById('cnt').textContent=cnt+' 个应用';
}

function f(){const q=document.getElementById('s').value.toLowerCase();document.querySelectorAll('.card').forEach(c=>{const n=c.querySelector('.card-name').textContent.toLowerCase();c.style.display=n.includes(q)?'':'none'})}

function detail(slug){
const a=apps.find(x=>x.slug===slug);if(!a)return;cur=slug;
document.getElementById('md-n').textContent=a.display_name||slug;
document.getElementById('md-d').textContent=a.desc||'';
document.getElementById('md-v').textContent=a.version||'-';
document.getElementById('md-p').textContent=a.service_port||'-';
document.getElementById('md-t').textContent=a.app_type||'native';
const btn=document.getElementById('md-btn');const unbtn=document.getElementById('md-unbtn');
if(a.installed){btn.style.display='none';unbtn.style.display=''}else{btn.style.display='';unbtn.style.display='none'}
document.getElementById('modal').classList.add('show');}
function closeModal(){document.getElementById('modal').classList.remove('show')}
document.getElementById('modal').addEventListener('click',function(e){if(e.target===this)closeModal()});
async function doInstall(){if(!cur)return;p('install','slug='+cur,d=>{ts(d.status||'安装中');setTimeout(load,2000);closeModal()})}
async function doUninstall(){if(!confirm('确定卸载 '+cur+' 吗？'))return;p('uninstall','slug='+cur,d=>{ts(d.status||'卸载中');setTimeout(load,2000);closeModal()})}

// 源管理
async function loadSources(){
q('sources',d=>{sources=(Array.isArray(d)?d:[]);renderSources()})}
function renderSources(){
document.getElementById('src-list').innerHTML=sources.map((s,i)=>`
<div class=src-row><input type=checkbox ${s.enabled!==false?'checked':''} onchange="sources[${i}].enabled=this.checked">
<input value="${s.name||''}" placeholder="名称" oninput="sources[${i}].name=this.value">
<input value="${s.url||''}" placeholder="https://example.com/apps.json" oninput="sources[${i}].url=this.value">
<button onclick="sources.splice(${i},1);renderSources()">×</button></div>`).join('')||'<div style=color:var(--fg2);font-size:.8rem;margin-bottom:8px>暂无第三方源</div>'}
function addSource(){sources.push({name:'',url:'',enabled:true});renderSources()}
async function saveSources(){
const list=[];document.querySelectorAll('#src-list .src-row').forEach(r=>{
const chk=r.querySelector('input[type=checkbox]');const inp=r.querySelectorAll('input:not([type=checkbox])');
const name=inp[0]?.value.trim();const url=inp[1]?.value.trim();if(name||url)list.push({name,url,enabled:chk.checked})});
p('sources',JSON.stringify(list),d=>{ts('源配置已保存');load()})}

load();
</script></body></html>
HTM
