#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no">
<title>Mihomo</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{--bg:#0b0f17;--bg2:#131822;--bg3:#1c2333;--card:#1a2130;--fg:#e8edf5;--fg2:#8892b0;--blue:#3b82f6;--green:#22c55e;--red:#ef4444;--radius:12px}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--fg);padding:0;padding-bottom:64px;min-height:100vh}
.hd{display:flex;align-items:center;justify-content:space-between;padding:14px 18px 6px;max-width:960px;margin:0 auto}
.hd h1{font-size:1.15rem;font-weight:700;display:flex;align-items:center;gap:8px}
.st{font-size:.72rem;padding:4px 12px;border-radius:20px;font-weight:500}
.st.on{background:rgba(34,197,94,.12);color:var(--green)}.st.off{background:rgba(239,68,68,.12);color:var(--red)}
.ct{padding:8px 14px;max-width:960px;margin:0 auto}
.pt{display:none}.pt.act{display:block;animation:fade .2s}
@keyframes fade{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:translateY(0)}}
.cd{background:var(--card);border-radius:var(--radius);padding:16px;margin-bottom:12px;border:1px solid rgba(255,255,255,.04)}
.cd h3{font-size:.82rem;font-weight:600;margin-bottom:10px;color:var(--fg2);letter-spacing:.3px}
.sg{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:4px}
.si{background:var(--bg3);border-radius:10px;padding:12px;text-align:center}
.si .l{font-size:.65rem;color:var(--fg2);margin-bottom:2px}.si .v{font-size:.95rem;font-weight:600}
#ch{width:100%;height:100px;border-radius:8px;background:var(--bg3);display:block;margin:6px 0 2px}
.lb{background:var(--bg3);border-radius:8px;padding:10px;font:13px/1.5 monospace;color:var(--fg2);white-space:pre-wrap;max-height:300px;overflow-y:auto;margin-top:6px}
.bn{position:fixed;bottom:0;left:0;right:0;background:var(--bg2);display:flex;border-top:1px solid rgba(255,255,255,.05);padding:4px 0 env(safe-area-inset-bottom)}
.bn a{flex:1;text-align:center;padding:5px 2px;font-size:.58rem;color:var(--fg2);text-decoration:none;cursor:pointer}
.bn a.act{color:var(--blue)}
.bn a svg{display:block;margin:0 auto 2px;width:20px;height:20px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}
.btn{display:inline-flex;align-items:center;gap:4px;padding:6px 12px;border:none;border-radius:6px;font-size:.75rem;font-weight:500;cursor:pointer}
.btn.p{background:var(--blue);color:#fff}.btn.d{background:rgba(239,68,68,.15);color:var(--red)}.btn.s{background:var(--bg3);color:var(--fg2)}
.tb{display:flex;gap:6px;margin-bottom:8px;flex-wrap:wrap}
.flex{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.p-box{background:var(--bg3);border-radius:8px;padding:10px;margin-bottom:4px;display:flex;justify-content:space-between;align-items:center}
.p-box .pn{font-weight:500;font-size:.8rem}.p-box .pt{font-size:.68rem;color:var(--fg2)}.p-box .pl{font-size:.72rem;color:var(--green)}
</style>
</head>
<body>
<div class="hd"><h1><span id="dot" style="display:inline-block;width:8px;height:8px;border-radius:50%;background:var(--red)"></span> Mihomo</h1><span id="st" class="st off">停止</span></div>
<div class="ct">
<div id="pt-ov" class="pt act">
<div class="cd"><h3>运行状态</h3><div class="sg" id="sg"></div></div>
<div class="cd"><h3>实时流量</h3><canvas id="ch"></canvas></div>
<div class="cd"><h3>代理组</h3><div id="proxy-groups"></div></div>
</div>
<div id="pt-lg" class="pt">
<div class="cd"><h3 style="display:flex;justify-content:space-between"><span>运行日志</span><button class="btn s" onclick="lg()">刷新</button></h3><div class="lb" id="lbox">点击刷新加载...</div></div>
</div>
</div>
<div class="bn">
<a class="act" data-p="ov"><svg viewBox="0 0 24 24"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>概览</a>
<a data-p="lg"><svg viewBox="0 0 24 24"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>日志</a>
</div>
<script>
const $=x=>document.getElementById(x);
const F=b=>{if(!b)return'0 B';const u=['B','KB','MB','GB'];let i=0,v=b;while(v>=1024&&i<3){v/=1024;i++}return v.toFixed(1)+' '+u[i]};
const api=async(a,m,b)=>{try{let u='api.cgi?action='+a;let o={method:m||'GET'};if(b){o.headers={'Content-Type':'application/x-www-form-urlencoded'};o.body=new URLSearchParams(b).toString()}const r=await fetch(u,o);return await r.json()}catch(e){return{}}};
document.querySelectorAll('.bn a').forEach(a=>a.onclick=()=>{document.querySelectorAll('.bn a').forEach(x=>x.classList.toggle('act',x.dataset.p===a.dataset.p));document.querySelectorAll('.pt').forEach(x=>x.classList.toggle('act',x.id==='pt-'+a.dataset.p));if(a.dataset.p==='lg')lg()});

const st=async()=>{
const d=await api('status');
const r=!!d.running;
$('dot').style.background=r?'var(--green)':'var(--red)';
$('st').textContent=r?'运行中':'停止';$('st').className='st '+(r?'on':'off');
$('sg').innerHTML=r?`<div class=si><div class=l>版本</div><div class=v style=font-size:.82rem>${d.version||d.api?'v'+d.version:'连接中'}</div></div><div class=si><div class=l>模式</div><div class=v>${d.mode||'Rule'}</div></div>`:`<div class=si><div class=l>状态</div><div class=v style=color:var(--red)>未运行</div></div><div class=si><div class=l>版本</div><div class=v>-</div></div>`};
st();setInterval(st,5000);

let td=[];
const dc=async()=>{
const d=await api('traffic');
if(d.rx!==undefined){td.push({rx:d.rx||0,tx:d.tx||0});if(td.length>60)td.shift();drawChart()}};
const drawChart=()=>{
const c=$('ch');if(!c||td.length<2)return;
const w=c.parentElement.clientWidth;c.width=w*2;c.height=100*2;const ctx=c.getContext('2d');ctx.scale(2,2);ctx.clearRect(0,0,w,100);
const mx=Math.max(...td.flatMap(x=>[x.rx,x.tx]),1);
[['rx','#3b82f6'],['tx','#22c55e']].forEach(([k,col])=>{
ctx.beginPath();ctx.strokeStyle=col;ctx.lineWidth=1.5;ctx.lineJoin='round';
td.forEach((d,i)=>{const x=6+(i/(td.length-1))*(w-12),y=8+84-(d[k]/mx)*84;i===0?ctx.moveTo(x,y):ctx.lineTo(x,y)});ctx.stroke()})};
setInterval(dc,3000);

const lp=async()=>{
const p=await api('proxies');
if(!p||p.error){$('proxy-groups').innerHTML='<div style="color:var(--fg2);font-size:.8rem">代理数据不可用</div>';return}
let h='';Object.entries(p).forEach(([k,v])=>{if(typeof v==='object'&&v.type){h+=`<div class=p-box><div><div class=pn>${k}</div><div class=pt>${v.type}</div></div><div class=pl>${v.now||'-'}</div></div>`}});
$('proxy-groups').innerHTML=h||'<div style="color:var(--fg2);font-size:.8rem">无代理数据</div>'};
setInterval(lp,10000);

const lg=async()=>{$('lbox').textContent='加载中...';const d=await api('log');$('lbox').textContent=d.log||'无日志'};
</script>
</body></html>
HTM
