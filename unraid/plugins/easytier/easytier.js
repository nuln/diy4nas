const api = '/plugins/easytier/api.php';

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

// Overview page
function refresh() {
  var el = document.getElementById('et-node-info');
  if (!el) return;

  request('node_info').then(function(r) {
    var d = r.data || {};
    var h = '';
    var rows = [['Hostname',d.hostname||'-'],['Peer ID',String(d.peer_id||'-')],['Version',d.version||'-'],['Proxy CIDRs',(d.proxy_cidrs||[]).join(', ')||'None']];
    for (var i=0; i<rows.length; i++) {
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + escapeHtml(rows[i][0]) + '</td><td>' + escapeHtml(rows[i][1]) + '</td></tr>';
    }
    el.innerHTML = h;
  }).catch(function(e) {
    el.innerHTML = '<tr><td colspan="2">Error: ' + escapeHtml(e.message) + '</td></tr>';
  });
  request('peers').then(function(r) {
    var d = r.data || [];
    var el2 = document.getElementById('et-peers-content');
    if (!el2) return;
    if (d.length === 0) { el2.innerHTML = '<tr><td colspan="4">No peers</td></tr>'; return; }
    var h = '';
    for (var i=0; i<d.length; i++) {
      var p = d[i];
      var t = p.tunnel_proto || p.cost || '-';
      var lat = (p.lat_ms && p.lat_ms !== '-') ? p.lat_ms + 'ms' : '-';
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + escapeHtml(p.hostname||p.id||'-') + '</td><td>' + escapeHtml(t) + '</td><td>' + escapeHtml(lat) + '</td><td>' + escapeHtml(p.loss_rate||'-') + '</td></tr>';
    }
    el2.innerHTML = h;
  }).catch(function(e) {
    var el2 = document.getElementById('et-peers-content');
    if (el2) el2.innerHTML = '<tr><td colspan="4">Error: ' + escapeHtml(e.message) + '</td></tr>';
  });
  request('routes').then(function(r) {
    var d = r.data || [];
    var el3 = document.getElementById('et-routes-content');
    if (!el3) return;
    if (d.length === 0) { el3.innerHTML = '<tr><td colspan="4">No routes</td></tr>'; return; }
    var h = '';
    for (var i=0; i<d.length; i++) {
      var rt = d[i];
      var lat = (typeof rt.next_hop_lat === 'number') ? rt.next_hop_lat.toFixed(2) + 'ms' : '-';
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + escapeHtml(rt.hostname||'-') + '</td><td>' + escapeHtml(rt.proxy_cidrs||'-') + '</td><td>' + escapeHtml(rt.next_hop_hostname||'-') + '</td><td>' + escapeHtml(lat) + '</td></tr>';
    }
    el3.innerHTML = h;
  }).catch(function(e) {
    var el3 = document.getElementById('et-routes-content');
    if (el3) el3.innerHTML = '<tr><td colspan="4">Error: ' + escapeHtml(e.message) + '</td></tr>';
  });
}

function doAction(a) {
  var s = document.getElementById('et-status');
  if (!s) return;
  s.textContent = a + '...';
  request(a,'POST').then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output : 'OK';
    s.textContent = out.replace(/\n/g, ' ');
    setTimeout(refresh, 2000);
  }).catch(function(e) {
    s.textContent = 'Error: ' + e.message;
  });
}

// Settings page - load current config
function loadSettings() {
  request('status').then(function(r) {
    var d = r.data || {};
    var map = {
      'et-name': 'network_name',
      'et-secret': 'network_secret',
      'et-ipv4': 'virtual_ipv4',
      'et-host': 'hostname',
      'et-peers': 'peer_urls',
      'et-listeners': 'listener_urls',
      'et-proxy': 'proxy_cidrs'
    };
    for (var id in map) {
      var el = document.getElementById(id);
      if (el && d[map[id]] !== undefined && d[map[id]] !== null) el.value = d[map[id]];
    }
    var dhcp = document.getElementById('et-dhcp');
    if (dhcp && d.dhcp !== undefined && d.dhcp !== null) dhcp.value = d.dhcp;
    var autostart = document.getElementById('et-autostart');
    if (autostart && d.autostart !== undefined && d.autostart !== null) autostart.value = d.autostart;
  }).catch(function(e) {
    var m = document.getElementById('et-cfg-msg');
    if (m) { m.textContent = 'Failed to load config: ' + e.message; m.style.color = '#d9534f'; }
  });
}

function saveConfig() {
  var m = document.getElementById('et-cfg-msg');
  if (!m) return;
  request('save_config','POST',{
    network_name: document.getElementById('et-name').value.trim(),
    network_secret: document.getElementById('et-secret').value,
    dhcp: document.getElementById('et-dhcp').value,
    virtual_ipv4: document.getElementById('et-ipv4').value.trim(),
    hostname: document.getElementById('et-host').value.trim(),
    peer_urls: document.getElementById('et-peers').value.trim(),
    listener_urls: document.getElementById('et-listeners').value.trim(),
    proxy_cidrs: document.getElementById('et-proxy').value.trim(),
    autostart: document.getElementById('et-autostart').value
  }).then(function(){m.textContent='Saved.';m.style.color='#5cb85c';}).catch(function(e){m.textContent=e.message;m.style.color='#d9534f';});
}

// Management page
function addConnector() {
  var u = document.getElementById('et-conn-url');
  var msg = document.getElementById('et-conn-msg');
  if (!u || !msg) return;
  var url = u.value.trim();
  if (!url) { msg.textContent='Enter URL'; return; }
  request('connector_add','POST',{url:url}).then(function(){msg.textContent='Added'; u.value=''; renderConnectors();}).catch(function(e){msg.textContent=e.message;});
}

function removeConnector(url) {
  var msg = document.getElementById('et-conn-msg');
  if (!msg) return;
  request('connector_remove','POST',{url:url}).then(function(){msg.textContent='Removed'; renderConnectors();}).catch(function(e){msg.textContent=e.message;});
}

function renderConnectors() {
  var el = document.getElementById('et-conn-list');
  if (!el) return;
  request('connector_list').then(function(r) {
    var l = Array.isArray(r.data)?r.data:[];
    if (!l.length) { el.innerHTML = '<tr><td>No connectors</td></tr>'; return; }
    var h = '<table class="unraid statusTable" style="margin:0"><thead><tr><th>URL</th><th></th></tr></thead><tbody>';
    for (var i=0; i<l.length; i++) {
      var u = l[i].url || l[i];
      var safeUrl = escapeHtml(u);
      h += '<tr><td>' + safeUrl + '</td><td><input type="button" value="Remove" data-url="' + escapeHtml(u) + '"></td></tr>';
    }
    h += '</tbody></table>';
    el.innerHTML = h;
    el.querySelectorAll('input[data-url]').forEach(function(btn) {
      btn.addEventListener('click', function() { removeConnector(this.dataset.url); });
    });
  }).catch(function(e) {
    el.innerHTML = '<tr><td>Error: ' + escapeHtml(e.message) + '</td></tr>';
  });
}

function setLogLevel() {
  var el = document.getElementById('et-log-level');
  var msg = document.getElementById('et-logger-msg');
  if (!el || !msg) return;
  request('logger_set','POST',{level:el.value}).then(function(){msg.textContent='Set to '+el.value;}).catch(function(e){msg.textContent=e.message;});
}

$(function() {
  if (document.getElementById('et-node-info')) refresh();
  if (document.getElementById('et-save')) {
    loadSettings();
    document.getElementById('et-save').addEventListener('click', saveConfig);
  }
  if (document.getElementById('et-conn-list')) renderConnectors();
});
