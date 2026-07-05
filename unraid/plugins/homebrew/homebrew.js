const api = '/plugins/homebrew/api.php';

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

function escapeHtml(s) {
  if (!s) return '';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

function msg(text, color) {
  var el = document.getElementById('hb-msg');
  if (!el) return;
  el.textContent = text;
  el.style.color = color || '#888';
}

function pkgMsg(text, color) {
  var el = document.getElementById('hb-pkg-msg');
  if (!el) return;
  el.textContent = text;
  el.style.color = color || '#888';
}

// Overview page
function refreshStatus() {
  var el = document.getElementById('hb-status-content');
  if (!el) return;

  request('status').then(function(r) {
    var d = r.data || {};
    var h = '';
    var rows = [
      ['Installed', d.installed === '1' ? 'Yes' : 'No'],
      ['Version', d.version || '-'],
      ['Brew Storage', d.brew_storage || '-'],
      ['Bind Mounted', d.bind_mounted === '1' ? 'Yes' : 'No'],
      ['linuxbrew User', d.linuxbrew_user === '1' ? 'Yes' : 'No'],
      ['Auto Setup', d.autostart === 'yes' ? 'Enabled' : 'Disabled'],
      ['Shell Integration', d.shell_integration || '-'],
      ['GCC Installed', d.gcc_installed === '1' ? 'Yes' : 'No'],
      ['GCC Auto Install', d.gcc_autoinstall === 'yes' ? 'Enabled' : 'Disabled'],
    ];
    for (var i=0; i<rows.length; i++) {
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + rows[i][0] + '</td><td>' + rows[i][1] + '</td></tr>';
    }
    el.innerHTML = h;

    var btn = document.getElementById('hb-install-btn');
    if (btn) btn.style.display = d.installed === '1' ? 'none' : 'inline-block';
  }).catch(function(e) {
    el.innerHTML = '<tr><td colspan="2">Error: ' + escapeHtml(e.message) + '</td></tr>';
  });

  refreshPackages();
  refreshOutdated();
}

function refreshPackages() {
  var el = document.getElementById('hb-packages');
  if (!el) return;

  request('list_packages').then(function(r) {
    var pkgs = r.data && r.data.packages ? r.data.packages : [];
    if (pkgs.length === 0) {
      el.innerHTML = '<tr><td>No formulae installed</td><td></td></tr>';
      return;
    }
    var h = '';
    for (var i=0; i<pkgs.length; i++) {
      var n = escapeHtml(pkgs[i]);
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + n + '</td>';
      h += '<td><input type="button" value="Uninstall" data-action="uninstall" data-name="' + n + '">';
      h += ' <input type="button" value="Info" data-action="info" data-name="' + n + '"></td></tr>';
    }
    el.innerHTML = h;
    el.querySelectorAll('input[data-action="uninstall"]').forEach(function(btn) {
      btn.addEventListener('click', function() { uninstallPkg(this.dataset.name); });
    });
    el.querySelectorAll('input[data-action="info"]').forEach(function(btn) {
      btn.addEventListener('click', function() { showInfo(this.dataset.name); });
    });
  }).catch(function() {
    el.innerHTML = '<tr><td colspan="2">Error loading packages</td></tr>';
  });
}

function refreshOutdated() {
  var el = document.getElementById('hb-outdated');
  if (!el) return;

  request('outdated').then(function(r) {
    var out = r.data && r.data.outdated ? r.data.outdated : '';
    var pkgs = out.trim() ? out.trim().split('\n') : [];
    if (pkgs.length === 0 || (pkgs.length === 1 && pkgs[0] === '')) {
      el.innerHTML = '<tr><td>All packages up to date</td><td></td></tr>';
      return;
    }
    var h = '';
    for (var i=0; i<pkgs.length; i++) {
      var n = escapeHtml(pkgs[i].trim());
      if (!n) continue;
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + n + '</td>';
      h += '<td><input type="button" value="Upgrade" data-action="upgrade" data-name="' + n + '"></td></tr>';
    }
    if (!h) h = '<tr><td>All packages up to date</td><td></td></tr>';
    el.innerHTML = h;
    el.querySelectorAll('input[data-action="upgrade"]').forEach(function(btn) {
      btn.addEventListener('click', function() { upgradePkg(this.dataset.name); });
    });
  }).catch(function() {
    el.innerHTML = '<tr><td colspan="2">Error loading outdated</td></tr>';
  });
}

function doAction(a) {
  msg(a === 'install' ? 'Installing Homebrew (this may take a while)...' : a + '...');

  request(a, 'POST').then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output : 'OK';
    msg(out.replace(/\n/g, ' '), '#5cb85c');
    setTimeout(refreshStatus, 2000);
  }).catch(function(e) {
    msg('Error: ' + e.message, '#d9534f');
  });
}

function confirmRemove() {
  if (!confirm('Remove Homebrew completely? This will delete all packages.')) return;
  msg('Removing...');
  request('remove', 'POST').then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output : 'OK';
    msg(out.replace(/\n/g, ' '), '#5cb85c');
    setTimeout(refreshStatus, 2000);
  }).catch(function(e) {
    msg('Error: ' + e.message, '#d9534f');
  });
}

function uninstallPkg(name) {
  if (!confirm('Uninstall ' + name + '?')) return;
  pkgMsg('Uninstalling ' + name + '...');
  request('package_uninstall', 'POST', {formula: name}).then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output : 'OK';
    pkgMsg(out.replace(/\n/g, ' '), '#5cb85c');
    setTimeout(refreshPackages, 2000);
    setTimeout(refreshOutdated, 2000);
  }).catch(function(e) {
    pkgMsg('Error: ' + e.message, '#d9534f');
  });
}

function showInfo(name) {
  pkgMsg('Loading info for ' + name + '...');
  request('package_info', 'GET', {formula: name}).then(function(r) {
    var info = (r.data && r.data.info) ? r.data.info : 'No info';
    pkgMsg('','#888');
    var overlay = document.getElementById('hb-info-overlay');
    if (!overlay) {
      overlay = document.createElement('div');
      overlay.id = 'hb-info-overlay';
      overlay.innerHTML = '<div style="position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.5);z-index:9999;display:flex;align-items:center;justify-content:center;"><div style="background:#fff;padding:16px;border-radius:4px;max-width:80%;max-height:80%;overflow:auto;"><pre id="hb-info-content" style="white-space:pre-wrap;word-break:break-all;margin:0 0 8px;"></pre><input type="button" value="Close" id="hb-info-close"></div></div>';
      document.body.appendChild(overlay);
      document.getElementById('hb-info-close').addEventListener('click', function() { overlay.style.display='none'; });
      overlay.addEventListener('click', function(e) { if (e.target === overlay) overlay.style.display='none'; });
    }
    overlay.style.display = '';
    document.getElementById('hb-info-content').textContent = info;
  }).catch(function(e) {
    pkgMsg('Error: ' + e.message, '#d9534f');
  });
}

function upgradePkg(name) {
  pkgMsg('Upgrading ' + name + '...');
  request('package_install', 'POST', {formula: name}).then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output : 'OK';
    pkgMsg(out.replace(/\n/g, ' '), '#5cb85c');
    setTimeout(refreshOutdated, 2000);
  }).catch(function(e) {
    pkgMsg('Error: ' + e.message, '#d9534f');
  });
}

// Settings page
function loadSettings() {
  request('status').then(function(r) {
    var d = r.data || {};
    var el = document.getElementById('hb-storage');
    if (el && d.brew_storage) el.value = d.brew_storage;
    var el2 = document.getElementById('hb-autostart');
    if (el2 && d.autostart) el2.value = d.autostart;
    var el3 = document.getElementById('hb-shell');
    if (el3 && d.shell_integration) el3.value = d.shell_integration;
    var el4 = document.getElementById('hb-gcc');
    if (el4 && d.gcc_autoinstall) el4.value = d.gcc_autoinstall;
  }).catch(function(e) {
    var m = document.getElementById('hb-cfg-msg');
    if (m) { m.textContent = 'Failed to load config: ' + e.message; m.style.color = '#d9534f'; }
  });
}

function saveConfig() {
  var m = document.getElementById('hb-cfg-msg');
  if (!m) return;

  var payload = {
    brew_storage: document.getElementById('hb-storage').value.trim(),
    autostart: document.getElementById('hb-autostart').value,
    shell_integration: document.getElementById('hb-shell').value,
    gcc_autoinstall: document.getElementById('hb-gcc').value,
  };

  request('save_config', 'POST', payload).then(function() {
    m.textContent = 'Saved.';
    m.style.color = '#5cb85c';
  }).catch(function(e) {
    m.textContent = e.message;
    m.style.color = '#d9534f';
  });
}

// Packages page
function searchPackages() {
  var q = document.getElementById('hb-search-q');
  var results = document.getElementById('hb-search-results');
  if (!q || !results) return;

  var query = q.value.trim();
  if (!query) { results.innerHTML = ''; return; }

  results.innerHTML = '<div class="spinner">Searching...</div>';

  // Fetch installed list first, then search
  Promise.all([
    request('list_packages').then(function(r) { return (r.data && r.data.packages) || []; }).catch(function() { return []; }),
    request('search', 'GET', {q: query}).then(function(r) { return (r.data && r.data.output) || ''; }).catch(function() { return ''; })
  ]).then(function(resultsArr) {
    var installed = resultsArr[0];
    var out = resultsArr[1];
    var lines = out.split('\n').filter(function(l) { return l.trim(); });

    var h = '<table class="unraid statusTable" style="margin:0"><thead><tr><th>Formula</th><th></th></tr></thead><tbody>';
    var count = 0;
    for (var i=0; i<lines.length; i++) {
      var name = lines[i].trim();
      if (!name || name === '=' || name.startsWith('Warning') || name.startsWith('Error')) continue;
      if (name.startsWith('==>')) {
        h += '<tr><td colspan="2" style="font-weight:bold;padding:8px 4px 2px">' + escapeHtml(name) + '</td></tr>';
        continue;
      }
      var isInstalled = installed.indexOf(name) !== -1;
      var sn = escapeHtml(name);
      h += '<tr class="' + (count%2===0?'normal-row':'alt-row') + '">';
      h += '<td>' + sn + (isInstalled ? ' <span style="color:#5cb85c;font-size:11px;">(installed)</span>' : '') + '</td>';
      if (isInstalled) {
        h += '<td><input type="button" value="Uninstall" data-action="pkg-uninstall" data-name="' + sn + '"></td>';
      } else {
        h += '<td><input type="button" value="Install" data-action="pkg-install" data-name="' + sn + '"></td>';
      }
      h += '</tr>';
      count++;
    }
    if (count === 0) {
      h += '<tr><td colspan="2">No results found</td></tr>';
    }
    h += '</tbody></table>';
    results.innerHTML = h;
    results.querySelectorAll('input[data-action="pkg-uninstall"]').forEach(function(btn) {
      btn.addEventListener('click', function() { uninstallPkg(this.dataset.name); });
    });
    results.querySelectorAll('input[data-action="pkg-install"]').forEach(function(btn) {
      btn.addEventListener('click', function() { installPkg(this.dataset.name); });
    });
  }).catch(function(e) {
    results.innerHTML = '<span style="color:#d9534f;">Error: ' + escapeHtml(e.message) + '</span>';
  });
}

function installPkg(name) {
  pkgMsg('Installing ' + name + ' (this may take a while)...');
  request('package_install', 'POST', {formula: name}).then(function(r) {
    var out = (r.data && r.data.output) ? r.data.output : 'OK';
    pkgMsg(out.replace(/\n/g, ' '), '#5cb85c');
    setTimeout(function() {
      searchPackages();
      refreshPackages();
    }, 2000);
  }).catch(function(e) {
    pkgMsg('Error: ' + e.message, '#d9534f');
  });
}

// Casks on Packages page
function refreshCasks() {
  var el = document.getElementById('hb-casks');
  if (!el) return;

  request('list_casks').then(function(r) {
    var casks = r.data && r.data.casks ? r.data.casks : [];
    if (casks.length === 0) {
      el.innerHTML = '<tr><td>No casks installed</td><td></td></tr>';
      return;
    }
    var h = '';
    for (var i=0; i<casks.length; i++) {
      var n = escapeHtml(casks[i]);
      h += '<tr class="' + (i%2===0?'normal-row':'alt-row') + '"><td>' + n + '</td>';
      h += '<td><input type="button" value="Uninstall" data-action="cask-uninstall" data-name="' + n + '"></td></tr>';
    }
    el.innerHTML = h;
    el.querySelectorAll('input[data-action="cask-uninstall"]').forEach(function(btn) {
      btn.addEventListener('click', function() { uninstallPkg(this.dataset.name); });
    });
  }).catch(function() {
    el.innerHTML = '<tr><td colspan="2">Error loading casks</td></tr>';
  });
}

$(function() {
  if (document.getElementById('hb-status-content')) {
    refreshStatus();
    setInterval(refreshStatus, 30000);
  }
  if (document.getElementById('hb-cfg-msg')) {
    loadSettings();
    document.getElementById('hb-save').addEventListener('click', saveConfig);
  }
  if (document.getElementById('hb-casks')) {
    refreshCasks();
  }
  if (document.getElementById('hb-search-q')) {
    document.getElementById('hb-search-q').addEventListener('keypress', function(e) {
      if (e.key === 'Enter') searchPackages();
    });
  }
});
