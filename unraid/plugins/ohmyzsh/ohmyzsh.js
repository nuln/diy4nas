const api = '/plugins/ohmyzsh/api.php';

function escapeHtml(s) {
  if (!s) return '';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

function request(a, m, p, timeout) {
  timeout = timeout || 30000;
  return new Promise(function(r, j) {
    var opts = {
      url: api, dataType: 'json', timeout: timeout,
      data: $.extend({action: a}, p || {}),
      success: function(d) { if (d.success) r(d); else j(new Error(d.error || 'Failed')); },
      error: function(x, t) { j(new Error(t === 'timeout' ? 'Timeout' : 'HTTP ' + x.status)); }
    };
    if (m === 'POST') opts.type = 'POST';
    $.ajax(opts);
  });
}

function loadStatus() {
  var el = document.getElementById('oz-status');
  if (!el) return;
  request('status').then(function(r) {
    var d = r.data || {};
    var installed = d.installed === '1' ? 'Yes' : 'No';
    var h = '';
    var rows = [
      ['Installed', installed],
      ['Version', d.version || '-'],
      ['ZSH Theme', d.theme || '-'],
      ['ZSH Plugins', d.plugins || '-'],
      ['Shell', d.shell || '-'],
      ['Config File', d.config || '-']
    ];
    for (var i=0; i<rows.length; i++) {
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + escapeHtml(rows[i][0]) + '</td><td>' + escapeHtml(rows[i][1]) + '</td></tr>';
    }
    el.innerHTML = h;
  }).catch(function(e) {
    el.innerHTML = '<tr><td colspan="2">Error: ' + e.message + '</td></tr>';
  });

  request('get_config').then(function(r) {
    var d = r.data || {};
    if (document.getElementById('oz-autostart')) document.getElementById('oz-autostart').value = d.autostart || 'yes';
    if (document.getElementById('oz-theme')) document.getElementById('oz-theme').value = d.zsh_theme || 'robbyrussell';
    if (document.getElementById('oz-plugins')) document.getElementById('oz-plugins').value = d.zsh_plugins || 'git';
    if (document.getElementById('oz-aliases')) document.getElementById('oz-aliases').value = d.custom_aliases || '';
  }).catch(function(e) {
    console.error('Failed to load config:', e.message);
  });
}

function doAction(a) {
  var msg = document.getElementById('oz-msg');
  if (!msg) return;
  msg.textContent = a + '...';
  request(a, 'POST').then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output.replace(/\n/g, ' ') : 'Done';
    msg.textContent = out;
    setTimeout(loadStatus, 1000);
  }).catch(function(e) {
    msg.textContent = 'Error: ' + e.message;
  });
}

function saveConfig() {
  var msg = document.getElementById('oz-cfg-msg');
  if (!msg) return;
  request('save_config', 'POST', {
    autostart: document.getElementById('oz-autostart').value,
    zsh_theme: document.getElementById('oz-theme').value.trim(),
    zsh_plugins: document.getElementById('oz-plugins').value.trim(),
    custom_aliases: document.getElementById('oz-aliases').value
  }).then(function() {
    msg.textContent = 'Configuration saved';
    msg.style.color = '#5cb85c';
  }).catch(function(e) {
    msg.textContent = e.message;
    msg.style.color = '#d9534f';
  });
}

$(function() {
  loadStatus();
});
