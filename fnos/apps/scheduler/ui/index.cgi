#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>计划任务</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; margin: 0; background: #f5f6fa; color: #1f2329; }
  header { background: #fff; border-bottom: 1px solid #e5e6eb; padding: 12px 24px; display: flex; align-items: center; gap: 24px; position: sticky; top: 0; z-index: 10; }
  header h1 { font-size: 18px; margin: 0; font-weight: 600; }
  nav { display: flex; gap: 4px; margin-left: 24px; }
  nav button { background: transparent; border: none; padding: 6px 14px; border-radius: 6px; cursor: pointer; color: #4e5969; font-size: 14px; }
  nav button.active { background: #165dff; color: #fff; }
  nav button:hover:not(.active) { background: #f2f3f5; }
  main { padding: 24px; max-width: 1400px; margin: 0 auto; }
  .card { background: #fff; border-radius: 8px; padding: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); margin-bottom: 16px; }
  .stat-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; }
  .stat { background: #fff; border-radius: 8px; padding: 18px 20px; border: 1px solid #f0f1f3; }
  .stat .label { color: #86909c; font-size: 12px; margin-bottom: 6px; }
  .stat .value { font-size: 26px; font-weight: 600; }
  .stat.success .value { color: #00b42a; }
  .stat.failed .value { color: #f53f3f; }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #f0f1f3; }
  th { color: #86909c; font-weight: 500; background: #fafbfc; font-size: 12px; text-transform: uppercase; letter-spacing: 0.5px; }
  tr:hover td { background: #fafbfc; }
  .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 500; }
  .badge.success { background: #e8ffea; color: #00b42a; }
  .badge.failed { background: #ffece8; color: #f53f3f; }
  .badge.running { background: #e8f3ff; color: #165dff; }
  .badge.timeout { background: #fff7e8; color: #ff7d00; }
  .badge.interrupted { background: #f5f5f5; color: #86909c; }
  .badge.disabled { background: #f5f5f5; color: #86909c; }
  .badge.enabled { background: #e8ffea; color: #00b42a; }
  .btn { background: #165dff; color: #fff; border: none; padding: 6px 14px; border-radius: 6px; cursor: pointer; font-size: 13px; }
  .btn:hover { background: #0e4cd9; }
  .btn.secondary { background: #f2f3f5; color: #1f2329; }
  .btn.secondary:hover { background: #e5e6eb; }
  .btn.danger { background: #f53f3f; }
  .btn.danger:hover { background: #d93030; }
  .btn.small { padding: 3px 10px; font-size: 12px; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; gap: 12px; flex-wrap: wrap; }
  .toolbar .left, .toolbar .right { display: flex; gap: 8px; align-items: center; }
  input[type="text"], input[type="number"], textarea, select { padding: 7px 12px; border: 1px solid #c9cdd4; border-radius: 6px; font-size: 14px; font-family: inherit; width: 100%; background: #fff; }
  input:focus, textarea:focus, select:focus { outline: none; border-color: #165dff; }
  textarea { min-height: 80px; resize: vertical; font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 13px; }
  label { display: block; font-size: 13px; color: #4e5969; margin-bottom: 6px; }
  .form-row { margin-bottom: 14px; }
  .form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .modal-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 100; display: flex; align-items: center; justify-content: center; padding: 20px; }
  .modal { background: #fff; border-radius: 8px; width: 100%; max-width: 600px; max-height: 90vh; overflow-y: auto; }
  .modal-header { padding: 16px 20px; border-bottom: 1px solid #f0f1f3; display: flex; justify-content: space-between; align-items: center; }
  .modal-header h2 { margin: 0; font-size: 16px; }
  .modal-body { padding: 20px; }
  .modal-footer { padding: 12px 20px; border-top: 1px solid #f0f1f3; display: flex; justify-content: flex-end; gap: 8px; }
  .close { background: none; border: none; font-size: 20px; cursor: pointer; color: #86909c; padding: 0; line-height: 1; }
  .empty { text-align: center; padding: 40px 20px; color: #86909c; font-size: 14px; }
  .log-box { background: #1f2329; color: #d4d4d4; padding: 16px; border-radius: 6px; font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 12px; max-height: 500px; overflow-y: auto; white-space: pre-wrap; word-break: break-all; line-height: 1.5; }
  .log-stdout { color: #d4d4d4; }
  .log-stderr { color: #ff7875; }
  .cron-hint { background: #f2f3f5; padding: 8px 12px; border-radius: 4px; font-size: 12px; color: #4e5969; margin-top: 6px; font-family: ui-monospace, monospace; }
  .toast { position: fixed; top: 24px; right: 24px; background: #1f2329; color: #fff; padding: 10px 18px; border-radius: 6px; z-index: 1000; font-size: 14px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); }
  .toast.error { background: #f53f3f; }
  .toast.success { background: #00b42a; }
  .status-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; vertical-align: middle; }
  .status-dot.running { background: #165dff; animation: pulse 1.5s ease-in-out infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
  .actions { display: flex; gap: 4px; }
  .help-icon { display: inline-block; width: 14px; height: 14px; border-radius: 50%; background: #c9cdd4; color: #fff; font-size: 10px; text-align: center; line-height: 14px; cursor: help; margin-left: 4px; }
  code { background: #f2f3f5; padding: 1px 6px; border-radius: 3px; font-family: ui-monospace, monospace; font-size: 12px; }
  .section { display: none; }
  .section.active { display: block; }
  .filter-bar { display: flex; gap: 8px; align-items: center; }
  .filter-bar select, .filter-bar input { width: auto; }
  .muted { color: #86909c; }
  .nowrap { white-space: nowrap; }
  .text-right { text-align: right; }
</style>
</head>
<body>
<header>
  <h1>📅 计划任务</h1>
  <nav>
    <button data-tab="dashboard" class="active">仪表盘</button>
    <button data-tab="jobs">任务列表</button>
    <button data-tab="runs">执行历史</button>
    <button data-tab="settings">设置</button>
  </nav>
  <div style="margin-left:auto; font-size:12px; color:#86909c;" id="health-info"></div>
</header>

<main>
  <!-- 仪表盘 -->
  <section id="sec-dashboard" class="section active">
    <div class="stat-grid" id="stat-grid"></div>
    <div class="card">
      <h3 style="margin-top:0;">最近执行</h3>
      <div id="recent-runs"></div>
    </div>
  </section>

  <!-- 任务列表 -->
  <section id="sec-jobs" class="section">
    <div class="toolbar">
      <div class="left">
        <button class="btn" onclick="openJobModal()">+ 新建任务</button>
      </div>
      <div class="right">
        <input type="text" id="job-search" placeholder="搜索任务名..." oninput="renderJobs()">
      </div>
    </div>
    <div class="card">
      <table>
        <thead>
          <tr>
            <th style="width:60px">ID</th>
            <th>名称</th>
            <th>cron 表达式</th>
            <th>命令</th>
            <th>状态</th>
            <th>下次执行</th>
            <th style="width:280px" class="text-right">操作</th>
          </tr>
        </thead>
        <tbody id="jobs-tbody"></tbody>
      </table>
      <div id="jobs-empty" class="empty" style="display:none">暂无任务，点击右上角"新建任务"开始</div>
    </div>
  </section>

  <!-- 执行历史 -->
  <section id="sec-runs" class="section">
    <div class="toolbar">
      <div class="left filter-bar">
        <label style="margin:0">任务:</label>
        <select id="runs-filter-job" onchange="loadRuns()">
          <option value="0">全部</option>
        </select>
        <label style="margin:0">状态:</label>
        <select id="runs-filter-status" onchange="loadRuns()">
          <option value="">全部</option>
          <option value="success">成功</option>
          <option value="failed">失败</option>
          <option value="running">运行中</option>
          <option value="timeout">超时</option>
          <option value="interrupted">中断</option>
        </select>
      </div>
      <div class="right">
        <button class="btn secondary" onclick="loadRuns()">刷新</button>
      </div>
    </div>
    <div class="card">
      <table>
        <thead>
          <tr>
            <th style="width:60px">ID</th>
            <th>任务</th>
            <th>触发方式</th>
            <th>开始时间</th>
            <th>耗时</th>
            <th>状态</th>
            <th>退出码</th>
            <th class="text-right" style="width:120px">操作</th>
          </tr>
        </thead>
        <tbody id="runs-tbody"></tbody>
      </table>
      <div id="runs-empty" class="empty" style="display:none">暂无执行记录</div>
    </div>
  </section>

  <!-- 设置 -->
  <section id="sec-settings" class="section">
    <div class="card" style="max-width:600px">
      <h3 style="margin-top:0">全局设置</h3>
      <div class="form-row">
        <label>时区</label>
        <select id="set-tz"></select>
        <div class="cron-hint">所有 cron 表达式将按所选时区解释。修改后即时生效（重建 cron 调度器）</div>
      </div>
      <div class="form-row">
        <label>默认超时（秒）</label>
        <input type="number" id="set-timeout" min="0" value="3600">
        <div class="cron-hint">任务单独配置的超时优先于此值</div>
      </div>
      <div class="form-row">
        <label>单次执行日志最大字节数</label>
        <input type="number" id="set-maxlog" min="1024" value="2097152">
      </div>
      <div class="form-row">
        <button class="btn" onclick="saveSettings()">保存设置</button>
      </div>
    </div>
    <div class="card" style="max-width:600px">
      <h3 style="margin-top:0">数据维护</h3>
      <div class="form-row" style="display:flex; gap:8px; align-items:center">
        <label style="margin:0">保留执行历史</label>
        <input type="number" id="cleanup-days" min="1" value="30" style="width:80px">
        <span class="muted">天</span>
        <button class="btn secondary" onclick="cleanupRuns()">清理历史</button>
      </div>
      <div class="cron-hint">删除早于指定天数的 runs 记录。runs.id 越大越新。</div>
    </div>
    <div class="card" style="max-width:900px">
      <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
        <h3 style="margin:0">服务端日志 <span class="muted" style="font-weight:normal; font-size:12px">（最近 200 行）</span></h3>
        <button class="btn small secondary" onclick="loadServerLog()">刷新</button>
      </div>
      <pre class="log-box" id="server-log" style="max-height:400px">加载中...</pre>
    </div>
  </section>
</main>

<!-- 任务编辑 Modal -->
<div class="modal-backdrop" id="job-modal" style="display:none">
  <div class="modal">
    <div class="modal-header">
      <h2 id="job-modal-title">新建任务</h2>
      <button class="close" onclick="closeJobModal()">×</button>
    </div>
    <div class="modal-body">
      <input type="hidden" id="job-id">
      <div class="form-row">
        <label>任务名称 *</label>
        <input type="text" id="job-name" placeholder="例如：每日清理日志">
      </div>
      <div class="form-row">
        <label>调度表达式 *</label>
        <input type="text" id="job-spec" placeholder="0 2 * * * 或 @hourly 或 @every 30s">
        <div class="cron-hint">
          5 段标准 cron：分 时 日 月 周<br>
          描述符：@yearly @monthly @weekly @daily @hourly @every 10m<br>
          示例：<code>0 2 * * *</code>（每天凌晨2点） / <code>*/5 * * * *</code>（每5分钟）
        </div>
      </div>
      <div class="form-row">
        <label>命令 *</label>
        <textarea id="job-cmd" placeholder="/usr/bin/sh /path/to/script.sh" style="min-height:100px"></textarea>
        <div class="cron-hint">通过 /bin/sh -c 执行，支持任意 shell 语法</div>
      </div>
      <div class="form-grid">
        <div class="form-row">
          <label>工作目录</label>
          <input type="text" id="job-workdir" placeholder="留空使用数据目录">
        </div>
        <div class="form-row">
          <label>超时（秒，0=使用默认）</label>
          <input type="number" id="job-timeout" min="0" value="0">
        </div>
      </div>
      <div class="form-grid">
        <div class="form-row">
          <label>触发通知</label>
          <select id="job-notify">
            <option value="failure">失败时</option>
            <option value="always">每次</option>
            <option value="none">从不</option>
          </select>
        </div>
        <div class="form-row">
          <label>启用</label>
          <select id="job-enabled">
            <option value="true">启用</option>
            <option value="false">禁用</option>
          </select>
        </div>
      </div>
      <div class="form-row">
        <label>备注</label>
        <input type="text" id="job-desc" placeholder="可选">
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn secondary" onclick="closeJobModal()">取消</button>
      <button class="btn" onclick="saveJob()">保存</button>
    </div>
  </div>
</div>

<!-- 日志 Modal -->
<div class="modal-backdrop" id="log-modal" style="display:none">
  <div class="modal" style="max-width:900px">
    <div class="modal-header">
      <h2>执行日志 #<span id="log-run-id"></span></h2>
      <button class="close" onclick="closeLogModal()">×</button>
    </div>
    <div class="modal-body">
      <div id="log-meta" style="margin-bottom:12px; font-size:13px; color:#4e5969;"></div>
      <div class="log-box" id="log-content"></div>
    </div>
    <div class="modal-footer">
      <span class="muted" id="log-status"></span>
      <button class="btn secondary" onclick="closeLogModal()">关闭</button>
    </div>
  </div>
</div>

<script>
const API = '/api';
let jobsCache = [];
let runsCache = [];
let logEventSource = null;
let refreshTimer = null;
const TIMEZONES = ['Asia/Shanghai', 'Asia/Tokyo', 'Asia/Singapore', 'Asia/Hong_Kong', 'Asia/Taipei', 'Asia/Seoul', 'Europe/London', 'Europe/Berlin', 'Europe/Paris', 'America/New_York', 'America/Los_Angeles', 'UTC'];

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('nav button').forEach(btn => {
    btn.addEventListener('click', () => switchTab(btn.dataset.tab));
  });
  loadSettings();
  loadStats();
  loadJobs();
  loadRuns();
  startRefresh();
});

function startRefresh() {
  stopRefresh();
  refreshTimer = setInterval(() => {
    const active = document.querySelector('nav button.active');
    if (!active) return;
    const tab = active.dataset.tab;
    if (tab === 'dashboard') loadStats();
    else if (tab === 'runs') loadRuns();
  }, 5000);
}

function stopRefresh() {
  if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
}

function switchTab(tab) {
  document.querySelectorAll('nav button').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
  document.querySelectorAll('.section').forEach(s => s.classList.toggle('active', s.id === 'sec-' + tab));
  if (tab === 'dashboard') loadStats();
  else if (tab === 'jobs') loadJobs();
  else if (tab === 'runs') loadRuns();
  else if (tab === 'settings') { loadSettings(); loadServerLog(); }
}

function refreshActive() {
  const active = document.querySelector('nav button.active').dataset.tab;
  if (active === 'dashboard') loadStats();
  else if (active === 'runs') loadRuns();
}

function toast(msg, type) {
  const el = document.createElement('div');
  el.className = 'toast' + (type ? ' ' + type : '');
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 3000);
}

async function api(path, opts) {
  try {
    const r = await fetch(API + path, opts);
    if (!r.ok) {
      const e = await r.json().catch(() => ({ error: r.statusText }));
      throw new Error(e.error || r.statusText);
    }
    return await r.json();
  } catch (e) {
    toast('请求失败: ' + e.message, 'error');
    throw e;
  }
}

async function loadStats() {
  try {
    const s = await api('/api/stats');
    document.getElementById('health-info').textContent = '🟢 运行中 · ' + new Date().toLocaleTimeString();
    document.getElementById('stat-grid').innerHTML = `
      <div class="stat"><div class="label">总任务数</div><div class="value">${s.total_jobs}</div></div>
      <div class="stat"><div class="label">启用中</div><div class="value" style="color:#165dff">${s.enabled_jobs}</div></div>
      <div class="stat success"><div class="label">成功执行</div><div class="value">${s.success_runs}</div></div>
      <div class="stat failed"><div class="label">失败执行</div><div class="value">${s.failed_runs}</div></div>
    `;
    const list = (s.recent_runs || []).slice(0, 8);
    if (list.length === 0) {
      document.getElementById('recent-runs').innerHTML = '<div class="empty">暂无执行记录</div>';
    } else {
      let html = '<table><thead><tr><th>ID</th><th>任务</th><th>开始时间</th><th>耗时</th><th>状态</th></tr></thead><tbody>';
      list.forEach(r => {
        html += `<tr style="cursor:pointer" onclick="viewRunLog(${r.id})">
          <td>${r.id}</td>
          <td>${escapeHtml(r.job_name || '#' + r.job_id)}</td>
          <td class="nowrap">${r.started_at}</td>
          <td>${formatDuration(r.duration_ms)}</td>
          <td>${statusBadge(r.status)}</td>
        </tr>`;
      });
      html += '</tbody></table>';
      document.getElementById('recent-runs').innerHTML = html;
    }
  } catch (e) { /* ignore */ }
}

async function loadJobs() {
  try {
    jobsCache = await api('/api/jobs') || [];
    renderJobs();
    const sel = document.getElementById('runs-filter-job');
    const current = sel.value;
    sel.innerHTML = '<option value="0">全部</option>' + jobsCache.map(j => `<option value="${j.id}">${escapeHtml(j.name)}</option>`).join('');
    sel.value = current;
  } catch (e) { jobsCache = []; renderJobs(); }
}

function renderJobs() {
  const q = (document.getElementById('job-search').value || '').toLowerCase();
  const list = jobsCache.filter(j => !q || j.name.toLowerCase().includes(q) || (j.description||'').toLowerCase().includes(q));
  const tbody = document.getElementById('jobs-tbody');
  document.getElementById('jobs-empty').style.display = list.length === 0 ? 'block' : 'none';
  tbody.innerHTML = list.map(j => `
    <tr>
      <td>${j.id}</td>
      <td>
        <div><strong>${escapeHtml(j.name)}</strong> ${!j.enabled ? '<span class="badge disabled">已禁用</span>' : '<span class="badge enabled">已启用</span>'}</div>
        <div class="muted" style="font-size:12px">${escapeHtml(j.description||'')}</div>
      </td>
      <td><code>${escapeHtml(j.spec)}</code></td>
      <td><code style="font-size:12px">${escapeHtml(truncate(j.command, 60))}</code></td>
      <td>${j.last_status ? statusBadge(j.last_status) : '<span class="muted">—</span>'}</td>
      <td class="nowrap">${j.next_run || '<span class="muted">—</span>'}</td>
      <td class="text-right">
        <div class="actions" style="justify-content:flex-end">
          <button class="btn small secondary" onclick="runJobNow(${j.id})" ${!j.enabled?'disabled':''}>运行</button>
          <button class="btn small secondary" onclick="toggleJob(${j.id})">${j.enabled?'禁用':'启用'}</button>
          <button class="btn small secondary" onclick="openJobModal(${j.id})">编辑</button>
          <button class="btn small danger" onclick="deleteJob(${j.id})">删除</button>
        </div>
      </td>
    </tr>
  `).join('');
}

async function loadRuns() {
  const jobId = document.getElementById('runs-filter-job').value;
  const status = document.getElementById('runs-filter-status').value;
  try {
    let url = '/api/runs?limit=200';
    if (jobId !== '0') url = '/api/runs?job_id=' + jobId + '&limit=200';
    runsCache = await api(url) || [];
    if (status) runsCache = runsCache.filter(r => r.status === status);
    const tbody = document.getElementById('runs-tbody');
    document.getElementById('runs-empty').style.display = runsCache.length === 0 ? 'block' : 'none';
    tbody.innerHTML = runsCache.map(r => `
      <tr>
        <td>${r.id}</td>
        <td>${escapeHtml(r.job_name || '#' + r.job_id)}</td>
        <td>${r.trigger === 'manual' ? '<span class="badge enabled">手动</span>' : '<span class="badge">定时</span>'}</td>
        <td class="nowrap">${r.started_at}</td>
        <td>${r.status === 'running' ? '<span class="status-dot running"></span>运行中' : formatDuration(r.duration_ms)}</td>
        <td>${statusBadge(r.status)}</td>
        <td>${r.status === 'running' ? '—' : r.exit_code}</td>
        <td class="text-right"><button class="btn small" onclick="viewRunLog(${r.id})">查看日志</button></td>
      </tr>
    `).join('');
  } catch (e) { runsCache = []; }
}

function statusBadge(s) {
  const map = { success:['success','成功'], failed:['failed','失败'], running:['running','运行中'], timeout:['timeout','超时'], interrupted:['interrupted','中断'] };
  const v = map[s] || ['disabled', s];
  return `<span class="badge ${v[0]}">${v[1]}</span>`;
}

function openJobModal(id) {
  document.getElementById('job-modal-title').textContent = id ? '编辑任务' : '新建任务';
  document.getElementById('job-id').value = id || '';
  if (id) {
    const j = jobsCache.find(x => x.id === id);
    if (!j) return;
    document.getElementById('job-name').value = j.name;
    document.getElementById('job-spec').value = j.spec;
    document.getElementById('job-cmd').value = j.command;
    document.getElementById('job-workdir').value = j.workdir || '';
    document.getElementById('job-timeout').value = j.timeout_sec || 0;
    document.getElementById('job-notify').value = j.notify_on || 'failure';
    document.getElementById('job-enabled').value = j.enabled ? 'true' : 'false';
    document.getElementById('job-desc').value = j.description || '';
  } else {
    ['job-name','job-spec','job-cmd','job-workdir','job-desc'].forEach(i => document.getElementById(i).value = '');
    document.getElementById('job-timeout').value = '0';
    document.getElementById('job-notify').value = 'failure';
    document.getElementById('job-enabled').value = 'true';
  }
  document.getElementById('job-modal').style.display = 'flex';
}

function closeJobModal() { document.getElementById('job-modal').style.display = 'none'; }

async function saveJob() {
  const id = document.getElementById('job-id').value;
  const data = {
    name: document.getElementById('job-name').value.trim(),
    spec: document.getElementById('job-spec').value.trim(),
    command: document.getElementById('job-cmd').value,
    workdir: document.getElementById('job-workdir').value.trim(),
    enabled: document.getElementById('job-enabled').value === 'true',
    notify_on: document.getElementById('job-notify').value,
    timeout_sec: parseInt(document.getElementById('job-timeout').value) || 0,
    description: document.getElementById('job-desc').value.trim(),
  };
  if (!data.name || !data.spec || !data.command) { toast('请填写必填项', 'error'); return; }
  try {
    if (id) {
      await api('/api/jobs/' + id, { method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data) });
      toast('已更新', 'success');
    } else {
      await api('/api/jobs', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data) });
      toast('已创建', 'success');
    }
    closeJobModal();
    loadJobs();
    loadStats();
  } catch (e) {}
}

async function deleteJob(id) {
  if (!confirm('确认删除该任务？执行历史也会一并删除。')) return;
  try {
    await api('/api/jobs/' + id, { method: 'DELETE' });
    toast('已删除', 'success');
    loadJobs();
    loadStats();
  } catch (e) {}
}

async function toggleJob(id) {
  try {
    const r = await api('/api/jobs/' + id + '/toggle', { method: 'POST' });
    toast(r.enabled ? '已启用' : '已禁用', 'success');
    loadJobs();
  } catch (e) {}
}

async function runJobNow(id) {
  try {
    const r = await api('/api/jobs/' + id + '/run', { method: 'POST' });
    toast(r.message || '已加入队列', 'success');
    setTimeout(loadRuns, 500);
  } catch (e) {}
}

function viewRunLog(id) {
  document.getElementById('log-run-id').textContent = id;
  document.getElementById('log-content').textContent = '加载中...';
  document.getElementById('log-meta').textContent = '';
  document.getElementById('log-status').textContent = '';
  document.getElementById('log-modal').style.display = 'flex';
  if (logEventSource) { logEventSource.close(); logEventSource = null; }
  logEventSource = new EventSource(API + '/runs/' + id + '/log');
  const content = document.getElementById('log-content');
  content.textContent = '';
  logEventSource.addEventListener('stdout', e => { appendLog(content, e.data, 'log-stdout'); });
  logEventSource.addEventListener('stderr', e => { appendLog(content, e.data, 'log-stderr'); });
  logEventSource.addEventListener('done', e => {
    document.getElementById('log-status').textContent = '已结束: ' + e.data;
    logEventSource.close(); logEventSource = null;
  });
  logEventSource.onerror = () => {
    document.getElementById('log-status').textContent = '连接已断开';
    if (logEventSource) { logEventSource.close(); logEventSource = null; }
  };
  api('/api/runs/' + id).then(r => {
    if (r) {
      document.getElementById('log-meta').innerHTML = `任务: <strong>${escapeHtml(r.job_name||'#'+r.job_id)}</strong> · 触发: ${r.trigger} · 开始: ${r.started_at} · 退出: ${r.exit_code}`;
    }
  });
}

function appendLog(el, line, cls) {
  const span = document.createElement('span');
  span.className = cls;
  span.textContent = line + '\n';
  el.appendChild(span);
  el.scrollTop = el.scrollHeight;
}

function closeLogModal() {
  if (logEventSource) { logEventSource.close(); logEventSource = null; }
  document.getElementById('log-modal').style.display = 'none';
}

async function loadSettings() {
  const sel = document.getElementById('set-tz');
  sel.innerHTML = TIMEZONES.map(t => `<option value="${t}">${t}</option>`).join('');
  try {
    const s = await api('/api/settings');
    sel.value = s.timezone || 'Asia/Shanghai';
    document.getElementById('set-timeout').value = s.default_timeout_sec || 3600;
    document.getElementById('set-maxlog').value = s.max_log_bytes || 2097152;
  } catch (e) {}
}

async function saveSettings() {
  const data = {
    timezone: document.getElementById('set-tz').value,
    default_timeout_sec: parseInt(document.getElementById('set-timeout').value) || 3600,
    max_log_bytes: parseInt(document.getElementById('set-maxlog').value) || 2097152,
  };
  try {
    await api('/api/settings', { method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data) });
    toast('设置已保存，时区变更已即时生效', 'success');
  } catch (e) {}
}

async function cleanupRuns() {
  const days = parseInt(document.getElementById('cleanup-days').value) || 30;
  if (!confirm(`确认删除早于 ${days} 天的执行历史？此操作不可恢复。`)) return;
  try {
    const r = await api('/api/cleanup?days=' + days, { method: 'POST' });
    toast(`已清理 ${r.deleted} 条历史记录`, 'success');
  } catch (e) {}
}

async function loadServerLog() {
  try {
    const r = await fetch(API + '/log?lines=200');
    const text = await r.text();
    document.getElementById('server-log').textContent = text || '（无日志）';
  } catch (e) {
    document.getElementById('server-log').textContent = '加载失败: ' + e.message;
  }
}

function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

function truncate(s, n) {
  s = String(s||'');
  return s.length > n ? s.slice(0, n) + '…' : s;
}

function formatDuration(ms) {
  if (!ms) return '—';
  if (ms < 1000) return ms + 'ms';
  if (ms < 60000) return (ms/1000).toFixed(1) + 's';
  if (ms < 3600000) return Math.floor(ms/60000) + 'm ' + Math.floor((ms%60000)/1000) + 's';
  return Math.floor(ms/3600000) + 'h ' + Math.floor((ms%3600000)/60000) + 'm';
}
</script>
</body>
</html>

HTM
