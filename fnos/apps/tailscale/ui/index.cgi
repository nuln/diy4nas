#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no">
<title>Tailscale</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0f1219;--bg2:#1a1f2e;--bg3:#242b3d;--fg:#e2e8f0;--fg2:#94a3b8;--accent:#3b82f6;--green:#22c55e;--red:#ef4444;--yellow:#eab308;--radius:10px}
body{font-family:-apple-system,sans-serif;background:var(--bg);color:var(--fg);padding:0;padding-bottom:64px;min-height:100vh}
.hdr{display:flex;align-items:center;justify-content:space-between;padding:14px 16px 8px;background:var(--bg);max-width:100%;margin:0 auto}
.hdr h1{font-size:1.2rem;display:flex;align-items:center;gap:6px}
.bdg{display:inline-block;padding:2px 10px;border-radius:20px;font-size:.7rem;font-weight:600}
.bdg.on{background:rgba(34,197,94,.15);color:var(--green)}.bdg.off{background:rgba(239,68,68,.15);color:var(--red)}
.tb{display:flex;gap:2px;padding:0 16px 10px;overflow-x:auto;background:var(--bg);position:sticky;top:0;z-index:10;max-width:100%;margin:0 auto}
.tb a{padding:6px 14px;border-radius:20px;font-size:.8rem;color:var(--fg2);text-decoration:none;cursor:pointer;white-space:nowrap}
.tb a.act{background:var(--accent);color:#fff}
.pg{display:none;padding:0 16px;max-width:100%;margin:0 auto;animation:f .2s}.pg.act{display:block}
@keyframes f{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:translateY(0)}}
.cd{background:var(--bg2);border-radius:var(--radius);padding:14px;margin-bottom:10px}
.cd-t{font-size:.85rem;font-weight:600;margin-bottom:8px;display:flex;align-items:center;gap:6px}
.g2{display:grid;grid-template-columns:1fr 1fr;gap:8px}
.sc{background:var(--bg3);border-radius:8px;padding:10px;text-align:center}
.sc .l{font-size:.65rem;color:var(--fg2)}.sc .v{font-size:1rem;font-weight:700;margin-top:2px}
canvas#tr{width:100%;height:150px;border-radius:8px;background:var(--bg3)}
.lb{background:var(--bg3);border-radius:8px;padding:10px;font-family:monospace;font-size:.7rem;color:var(--fg2);white-space:pre-wrap;max-height:350px;overflow-y:auto}
input,select{width:100%;padding:8px 10px;background:var(--bg3);border:1px solid transparent;border-radius:8px;color:var(--fg);font-size:.85rem;outline:none;margin-bottom:6px}
input:focus{border-color:var(--accent)}
.btn{display:inline-flex;align-items:center;gap:4px;padding:7px 14px;border:none;border-radius:8px;font-size:.8rem;font-weight:600;cursor:pointer;transition:.2s;margin-right:4px;margin-bottom:4px}
.btn.pri{background:var(--accent);color:#fff}.btn.dan{background:rgba(239,68,68,.15);color:var(--red)}.btn.out{background:var(--bg3);color:var(--fg)}.btn.sm{padding:4px 10px;font-size:.7rem}
.dg{display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:6px}
.dv{background:var(--bg3);border-radius:8px;padding:10px;cursor:pointer}.dv:active{transform:scale(.97)}
.dv .dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:4px}.dot.on{background:var(--green)}.dot.off{background:var(--red)}
.dv .n{font-weight:600;font-size:.85rem}.dv .ip{font-size:.75rem;color:var(--fg2);font-family:monospace}.dv .m{font-size:.65rem;color:var(--fg2);margin-top:4px}
.btm{position:fixed;bottom:0;left:0;right:0;background:var(--bg2);display:flex;border-top:1px solid var(--bg3);z-index:50;padding-bottom:env(safe-area-inset-bottom)}
.btm a{flex:1;text-align:center;padding:7px 2px;font-size:.6rem;color:var(--fg2);text-decoration:none;cursor:pointer}
.btm a.act{color:var(--accent)}.btm a svg{display:block;margin:0 auto 2px;width:18px;height:18px}
.ts{position:fixed;bottom:72px;left:50%;transform:translateX(-50%);background:var(--bg3);color:var(--fg);padding:8px 16px;border-radius:8px;font-size:.8rem;z-index:100;display:none}
label.cb{display:flex;align-items:center;gap:6px;font-size:.8rem;cursor:pointer;margin:4px 0}
</style>
</head>
<body>

<div class="hdr"><h1>Tailscale</h1><span id="bdg" class="bdg off">离线</span></div>
<div class="tb" id="tb">
<a class="act" data-p="ov">概览</a><a data-p="dv">设备</a><a data-p="tp">拓扑</a><a data-p="pi">Ping</a><a data-p="st">设置</a><a data-p="lg">日志</a>
</div>

<!-- 概览 -->
<div id="pg-ov" class="pg act">
<div class="cd"><div class="cd-t">运行状态</div><div class="g2" id="stats"></div></div>
<div class="cd"><div class="cd-t">实时流量</div><canvas id="tr"></canvas></div>
<div class="cd"><div class="cd-t">操作</div><button class="btn pri" id="btn-up" onclick="action('up')">连接</button><button class="btn dan" id="btn-down" onclick="action('down');st()">断开</button></div>
</div>

<!-- 设备 -->
<div id="pg-dv" class="pg">
<div class="cd"><div class="cd-t">设备列表 <span id="dev-cnt" style="font-size:.7rem;color:var(--fg2)"></span></div><div class="dg" id="devs"></div></div>
</div>

<!-- 拓扑 -->
<div id="pg-tp" class="pg">
<div class="cd">
<div class="cd-t">网络拓扑图</div>
<div id="topo-container" style="width:100%;height:calc(100vh - 240px);min-height:350px;background:var(--bg3);border-radius:8px;position:relative;overflow:hidden;margin-top:6px"></div>
</div>
<div class="cd" style="margin-top:12px">
<div class="cd-t">批量测速</div>
<button class="btn pri" id="btn-batch-ping" onclick="batchPing()" style="width:100%">开始批量 Ping 测速</button>
<div id="batch-ping-progress-container" style="display:none;margin-top:10px">
<div style="display:flex;justify-content:space-between;font-size:.7rem;color:var(--fg2);margin-bottom:4px">
<span id="batch-ping-status">正在准备测速...</span>
<span id="batch-ping-percent">0%</span>
</div>
<div style="width:100%;height:6px;background:var(--bg3);border-radius:3px;overflow:hidden">
<div id="batch-ping-progress-bar" style="width:0%;height:100%;background:var(--accent);transition:width 0.2s"></div>
</div>
</div>
<div class="lb" id="bpo" style="display:none;margin-top:8px;max-height:200px"></div>
</div>
</div>

<!-- Ping -->
<div id="pg-pi" class="pg">
<div class="cd"><div class="cd-t">Ping 测试</div><input id="pt" placeholder="目标 IP 或主机名"><button class="btn pri" onclick="doPing()">发送 Ping</button><div class="lb" id="po" style="margin-top:6px">等待测试...</div></div>
</div>

<!-- 设置 -->
<div id="pg-st" class="pg">
<div class="cd"><div class="cd-t">认证</div><button class="btn pri" onclick="auth()">Web 认证</button><button class="btn dan" onclick="action('down');st()">断开</button><button class="btn dan" style="background:rgba(239,68,68,0.15);color:var(--red)" onclick="if(confirm('确定要退出登录（清除所有认证状态）吗？')){action('logout');st()}">退出登录</button></div>
<div class="cd"><div class="cd-t">配置</div>
<input id="s-host" placeholder="主机名"><input id="s-routes" placeholder="子网路由 (192.168.1.0/24)">
<input id="s-login" placeholder="Login Server (Headscale 地址)"><input id="s-exit" placeholder="出口节点 IP">
<label class="cb"><input type="checkbox" id="s-dns" checked>接受 DNS</label>
<label class="cb"><input type="checkbox" id="s-sh">Shields Up</label>
<label class="cb"><input type="checkbox" id="s-ar">接受子网路由</label>
<button class="btn pri" onclick="saveSet()">应用配置</button></div>
</div>

<!-- 日志 -->
<div id="pg-lg" class="pg">
<div class="cd"><div class="cd-t" style="display:flex;justify-content:space-between"><span>运行日志</span><button class="btn sm out" onclick="lg()">刷新</button></div><div class="lb" id="lbox" style="height:calc(100vh - 180px);min-height:350px;max-height:none">点击刷新加载...</div></div>
</div>

<div class="btm">
<a class="act" data-p="ov"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/></svg>概览</a>
<a data-p="dv"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>设备</a>
<a data-p="tp"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><line x1="12" y1="8" x2="6.5" y2="16.5"/><line x1="12" y1="8" x2="17.5" y2="16.5"/><line x1="8" y1="19" x2="16" y2="19"/></svg>拓扑</a>
<a data-p="pi"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>Ping</a>
<a data-p="st"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 1v2m0 18v2M4.22 4.22l1.42 1.42m12.72 12.72l1.42 1.42M1 12h2m18 0h2"/></svg>设置</a>
<a data-p="lg"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>日志</a>
</div>

<div class="ts" id="ts"></div>

<script>
const TS=(m,t=2000)=>{const e=document.getElementById('ts');e.textContent=m;e.style.display='block';clearTimeout(e._t);e._t=setTimeout(()=>e.style.display='none',t)};
const F=b=>{if(!b)return'0B';const u=['B','KB','MB','GB','TB'];let i=0,v=b;while(v>=1024&&i<4){v/=1024;i++}return v.toFixed(1)+' '+u[i]};
document.querySelectorAll('#tb a,.btm a').forEach(a=>a.onclick=()=>{const p=a.dataset.p;document.querySelectorAll('#tb a,.btm a').forEach(x=>x.classList.toggle('act',x.dataset.p===p));document.querySelectorAll('.pg').forEach(x=>x.classList.toggle('act',x.id==='pg-'+p));if(p==='lg')lg();if(p==='tp')drawTopo();if(p==='ov')tr()});

let td=[];
function q(a,m,b){
const x=new XMLHttpRequest();x.open(m||'GET','api.cgi?action='+a);
x.setRequestHeader('Content-Type','application/x-www-form-urlencoded');
let bd='';if(b){const f=new FormData();Object.keys(b).forEach(k=>f.append(k,b[k]));bd=new URLSearchParams(b).toString()}
x.onload=()=>{try{window._ld=JSON.parse(x.responseText)}catch(e){}};
x.onerror=()=>{window._ld=null};
x.send(bd||null);
setTimeout(()=>{if(window._ld)try{window['_cb'](window._ld)}catch(e){}},600)
}

function st(){
window._cb=function(d){const s=d.Self||{};const p=d.Peer||{};
const oc=Object.values(p).filter(x=>x.Online).length;
document.getElementById('bdg').textContent=d.online?'在线':'离线';document.getElementById('bdg').className='bdg '+(d.online?'on':'off');
document.getElementById('stats').innerHTML=
`<div class=sc><div class=l>节点</div><div class=v>${s.HostName||'-'}</div></div>
<div class=sc><div class=l>IP</div><div class=v style=font-size:.8rem;font-family:monospace>${s.TailAddr||'-'}</div></div>
<div class=sc><div class=l>在线设备</div><div class=v>${oc}/${Object.keys(p).length}</div></div>
<div class=sc><div class=l>流量</div><div class=v style=font-size:.8rem>↓${F(d.totalRx)}↑${F(d.totalTx)}</div></div>`;
// Device list
let h='';Object.entries(p).forEach(([k,v])=>{
h+=`<div class=dv onclick="navigator.clipboard.writeText('${v.TailAddr||''}');TS('已复制 IP')">
<span class="dot ${v.Online?'on':'off'}"></span><span class=n>${v.HostName||k.slice(0,8)}</span>
<div class=ip>${v.TailAddr||'-'}</div><div class=m>${v.OS||''} ${v.Online?'在线':'离线'}</div></div>`});
document.getElementById('devs').innerHTML=h;document.getElementById('dev-cnt').textContent=`${Object.keys(p).length} 台`};
q('status','GET');}
setInterval(()=>q('status','GET'),8000);

function tr(){
window._cb=function(d){if(d.rx===undefined)return;if(td.length===0){td.push(d)}td.push(d);if(td.length>60)td.shift();
const c=document.getElementById('tr');if(!c)return;
const w=c.parentElement.clientWidth;c.width=w*2;c.height=150*2;const ctx=c.getContext('2d');ctx.scale(2,2);
ctx.clearRect(0,0,w,150);if(td.length<2)return;
const mx=Math.max(...td.flatMap(x=>[x.rx,x.tx]),1);
[['rx','#3b82f6'],['tx','#22c55e']].forEach(([k,col])=>{
ctx.beginPath();ctx.strokeStyle=col;ctx.lineWidth=2;
td.forEach((d,i)=>{const px=10+(i/(td.length-1))*(w-20),py=10+120-(d[k]/mx)*120;i===0?ctx.moveTo(px,py):ctx.lineTo(px,py)});ctx.stroke();
ctx.fillStyle=col;ctx.font='9px sans-serif';ctx.fillText(k.toUpperCase()+':'+F(td[td.length-1][k]),10+k==='rx'?0:80,140)})}};
q('traffic','GET');}
setInterval(()=>q('traffic','GET'),3000);

function action(a){
const b={};const s=document.getElementById('s-host');if(s&&s.value)b.hostname=s.value;
q(a,'POST',b);
setTimeout(()=>{TS(a==='up'?'已连接':(a==='logout'?'已退出登录':'已断开'));st()},1000);
}

function auth(){q('up','POST',{});setTimeout(()=>{TS('认证页面已打开');st()},1000)}

function doPing(){
const t=document.getElementById('pt').value.trim();if(!t){TS('输入目标');return}
document.getElementById('po').textContent='Ping '+t+'...\n';
window._cb=function(d){document.getElementById('po').textContent+=d.output||'无响应'};
q('ping','POST',{target:t,count:10});
}

function saveSet(){
q('up','POST',{hostname:document.getElementById('s-host').value,routes:document.getElementById('s-routes').value,loginServer:document.getElementById('s-login').value,exitNode:document.getElementById('s-exit').value,acceptDns:document.getElementById('s-dns').checked?1:0,shieldsUp:document.getElementById('s-sh').checked?1:0,acceptRoutes:document.getElementById('s-ar').checked?1:0});
setTimeout(()=>TS('配置已应用'),500);
}

function lg(){
document.getElementById('lbox').textContent='加载中...';
window._cb=function(d){document.getElementById('lbox').textContent=d.log||'无日志'};
q('log','GET');}

const apiCGI = async (action, method, body) => {
    try {
        let url = 'api.cgi?action=' + action;
        let options = { method: method || 'GET' };
        if (body) {
            options.headers = { 'Content-Type': 'application/x-www-form-urlencoded' };
            options.body = new URLSearchParams(body).toString();
        }
        const r = await fetch(url, options);
        return await r.json();
    } catch (e) {
        console.error(e);
        return {};
    }
};

let visLoaded = false;
const loadVisNetwork = () => {
    return new Promise((resolve, reject) => {
        if (window.vis) {
            resolve();
            return;
        }
        const oldDefine = window.define;
        window.define = undefined;
        const script = document.createElement('script');
        script.src = 'vis-network.min.js';
        script.onload = () => {
            window.define = oldDefine;
            if (window.vis) {
                visLoaded = true;
                resolve();
            } else {
                reject(new Error('拓扑图库对象未定义'));
            }
        };
        script.onerror = () => {
            window.define = oldDefine;
            reject(new Error('无法加载拓扑图库'));
        };
        document.head.appendChild(script);
    });
};

const drawTopo = async () => {
    const container = document.getElementById('topo-container');
    container.innerHTML = '<div style="color:var(--fg2);font-size:.8rem;padding:20px;text-align:center;">正在加载...</div>';
    try {
        await loadVisNetwork();
        container.innerHTML = '<div style="color:var(--fg2);font-size:.8rem;padding:20px;text-align:center;">正在获取设备列表...</div>';
        
        const d = await apiCGI('status');
        let retries = 0;
        while ((container.clientWidth === 0 || container.clientHeight === 0) && retries < 10) {
            await new Promise(r => setTimeout(r, 50));
            retries++;
        }
        const self = d.Self || {};
        const peers = Object.values(d.Peer || {});
        container.innerHTML = '';
        
        const nodes = [];
        const edges = [];
        
        nodes.push({
            id: 'self',
            label: (self.HostName || '当前NAS') + '\n(' + (self.TailAddr || '-') + ')',
            color: {
                background: '#1d4ed8',
                border: '#3b82f6',
                highlight: { background: '#2563eb', border: '#60a5fa' }
            },
            shape: 'database',
            size: 25,
            font: { color: '#ffffff', size: 12 }
        });
        
        peers.forEach(p => {
            const label = (p.HostName || p.ID.slice(0, 8)) + '\n(' + (p.TailAddr || '-') + ')';
            nodes.push({
                id: p.ID,
                label: label,
                color: p.Online ? {
                    background: '#047857',
                    border: '#10b981',
                    highlight: { background: '#059669', border: '#34d399' }
                } : {
                    background: '#b91c1c',
                    border: '#ef4444',
                    highlight: { background: '#dc2626', border: '#f87171' }
                },
                shape: 'dot',
                size: 18,
                font: { color: '#8892b0', size: 11 }
            });
            
            edges.push({
                from: 'self',
                to: p.ID,
                color: p.Online ? '#10b981' : '#475569',
                width: p.Online ? 2 : 1,
                length: 160
            });
        });
        
        const data = { nodes: new vis.DataSet(nodes), edges: new vis.DataSet(edges) };
        const options = {
            physics: {
                solver: 'forceAtlas2Based',
                forceAtlas2Based: {
                    gravitationalConstant: -50,
                    centralGravity: 0.01,
                    springLength: 100,
                    springConstant: 0.08
                }
            },
            interaction: {
                dragNodes: true,
                zoomView: true,
                dragView: true
            }
        };
        new vis.Network(container, data, options);
    } catch (e) {
        container.innerHTML = `<div style="color:var(--red);font-size:.8rem;padding:20px;text-align:center;">加载失败: ${e.message}</div>`;
    }
};

const batchPing = async () => {
    const btn = document.getElementById('btn-batch-ping');
    const pContainer = document.getElementById('batch-ping-progress-container');
    const pBar = document.getElementById('batch-ping-progress-bar');
    const pStatus = document.getElementById('batch-ping-status');
    const pPercent = document.getElementById('batch-ping-percent');
    const output = document.getElementById('bpo');
    
    btn.disabled = true;
    pContainer.style.display = 'block';
    pBar.style.width = '0%';
    pPercent.textContent = '0%';
    pStatus.textContent = '正在获取设备列表...';
    output.style.display = 'block';
    output.textContent = '开始批量测速...\n';
    
    try {
        const d = await apiCGI('status');
        const peers = Object.values(d.Peer || {}).filter(p => p.Online);
        if (peers.length === 0) {
            output.textContent += '没有在线的设备可以测试。\n';
            btn.disabled = false;
            pStatus.textContent = '测速结束';
            return;
        }
        
        let completed = 0;
        const total = peers.length;
        pStatus.textContent = `准备测试 ${total} 台设备...`;
        
        const runPing = async p => {
            const ip = p.TailAddr;
            const hostname = p.HostName || p.ID.slice(0, 8);
            output.textContent += `[测试中] ${hostname} (${ip})...\n`;
            
            const r = await apiCGI('ping', 'POST', { target: ip, count: 3 });
            completed++;
            
            const pct = Math.round((completed / total) * 100);
            pBar.style.width = pct + '%';
            pPercent.textContent = pct + '%';
            pStatus.textContent = `已完成 ${completed}/${total}`;
            
            let latency = '超时';
            if (r && r.output) {
                const m = r.output.match(/avg[^=]*=\s*([0-9.]+)/i) || r.output.match(/([0-9.]+)ms/i);
                if (m) {
                    latency = m[1] + ' ms';
                } else if (r.output.indexOf('pong') !== -1) {
                    latency = '在线';
                }
            }
            output.textContent += `[结果] ${hostname}: ${latency}\n`;
        };
        
        const limitConcurrency = async (tasks, limit, fn) => {
            const results = [];
            const executing = new Set();
            for (const item of tasks) {
                const p = Promise.resolve().then(() => fn(item));
                results.push(p);
                executing.add(p);
                const clean = () => executing.delete(p);
                p.then(clean, clean);
                if (executing.size >= limit) {
                    await Promise.race(executing);
                }
            }
            return Promise.all(results);
        };
        
        await limitConcurrency(peers, 3, runPing);
        pStatus.textContent = '测速完成';
        output.textContent += '所有设备测速完毕。\n';
    } catch (e) {
        output.textContent += `发生错误: ${e.message}\n`;
        pStatus.textContent = '测速出错';
    } finally {
        btn.disabled = false;
    }
};

// 移动端手势切换
let touchStartX = 0;
let touchEndX = 0;

document.addEventListener('touchstart', e => {
    touchStartX = e.changedTouches[0].screenX;
}, { passive: true });

document.addEventListener('touchend', e => {
    touchEndX = e.changedTouches[0].screenX;
    handleSwipe();
}, { passive: true });

const handleSwipe = () => {
    const tabs = ['ov', 'dv', 'tp', 'pi', 'st', 'lg'];
    const activeLink = document.querySelector('.btm a.act');
    if (!activeLink) return;
    
    const currentTab = activeLink.dataset.p;
    const currentIndex = tabs.indexOf(currentTab);
    if (currentIndex === -1) return;
    
    const diff = touchEndX - touchStartX;
    if (Math.abs(diff) > 80) {
        let newIndex = currentIndex;
        if (diff > 0 && currentIndex > 0) {
            newIndex = currentIndex - 1;
        } else if (diff < 0 && currentIndex < tabs.length - 1) {
            newIndex = currentIndex + 1;
        }
        
        if (newIndex !== currentIndex) {
            const nextTabId = tabs[newIndex];
            const nextLink = document.querySelector(`.btm a[data-p="${nextTabId}"]`);
            if (nextLink) nextLink.click();
        }
    }
};

// Init
st();tr();
setInterval(()=>q('traffic','GET'),3000);
</script>
</body></html>
HTM
