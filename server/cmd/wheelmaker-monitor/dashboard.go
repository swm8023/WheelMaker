package main

import "net/http"

func handleDashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dashboardHTML))
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>WheelMaker Monitor</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;600;700&family=Inter:wght@400;500;600&display=swap');

:root {
  --bg: #0a0e14;
  --bg-card: #111820;
  --bg-card-hover: #161e28;
  --border: #1e2a38;
  --border-active: #2a3a4e;
  --text: #c5cdd8;
  --text-dim: #6b7a8d;
  --text-bright: #e8edf3;
  --accent: #3b82f6;
  --accent-dim: #1e3a5f;
  --green: #22c55e;
  --green-dim: #0a3d1e;
  --red: #ef4444;
  --red-dim: #3d0a0a;
  --yellow: #eab308;
  --yellow-dim: #3d3508;
  --orange: #f97316;
  --mono: 'JetBrains Mono', 'Consolas', monospace;
  --sans: 'Inter', -apple-system, sans-serif;
}

* { margin:0; padding:0; box-sizing:border-box; }

body {
  background: var(--bg);
  color: var(--text);
  font-family: var(--sans);
  font-size: 14px;
  line-height: 1.5;
  min-height: 100vh;
}

.monitor-header {
  background: linear-gradient(180deg, #111820 0%, var(--bg) 100%);
  border-bottom: 1px solid var(--border);
  padding: 20px 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: sticky;
  top: 0;
  z-index: 100;
  backdrop-filter: blur(8px);
}

.monitor-header h1 {
  font-family: var(--mono);
  font-size: 18px;
  font-weight: 700;
  color: var(--text-bright);
  letter-spacing: -0.5px;
}

.monitor-header h1 span {
  color: var(--accent);
}

.header-status {
  display: flex;
  align-items: center;
  gap: 12px;
}

.status-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--text-dim);
  transition: background 0.3s;
}

.status-dot.online { background: var(--green); box-shadow: 0 0 8px rgba(34,197,94,0.4); }
.status-dot.offline { background: var(--red); box-shadow: 0 0 8px rgba(239,68,68,0.4); }

.status-label {
  font-family: var(--mono);
  font-size: 12px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.main-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1px;
  background: var(--border);
  margin: 0;
}

.card {
  background: var(--bg-card);
  padding: 24px;
  min-height: 180px;
}

.card-full {
  grid-column: 1 / -1;
}

.card-title {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-title::before {
  content: '';
  width: 3px;
  height: 14px;
  background: var(--accent);
  border-radius: 1px;
}

/* Process Table */
.proc-table {
  width: 100%;
  border-collapse: collapse;
  font-family: var(--mono);
  font-size: 13px;
}

.proc-table th {
  text-align: left;
  padding: 8px 12px;
  font-weight: 600;
  color: var(--text-dim);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 1px;
  border-bottom: 1px solid var(--border);
}

.proc-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  color: var(--text);
}

.proc-table tr:last-child td { border-bottom: none; }

.badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 600;
  font-family: var(--mono);
  letter-spacing: 0.5px;
}

.badge-green { background: var(--green-dim); color: var(--green); border: 1px solid rgba(34,197,94,0.2); }
.badge-red { background: var(--red-dim); color: var(--red); border: 1px solid rgba(239,68,68,0.2); }
.badge-yellow { background: var(--yellow-dim); color: var(--yellow); border: 1px solid rgba(234,179,8,0.2); }
.badge-blue { background: var(--accent-dim); color: var(--accent); border: 1px solid rgba(59,130,246,0.2); }

/* Actions */
.actions-row {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.btn {
  font-family: var(--mono);
  font-size: 12px;
  font-weight: 600;
  padding: 8px 18px;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--bg);
  color: var(--text);
  cursor: pointer;
  transition: all 0.15s;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.btn:hover { background: var(--bg-card-hover); border-color: var(--border-active); color: var(--text-bright); }
.btn:active { transform: scale(0.97); }
.btn:disabled { opacity: 0.4; cursor: not-allowed; }

.btn-danger { border-color: rgba(239,68,68,0.3); color: var(--red); }
.btn-danger:hover { background: var(--red-dim); border-color: rgba(239,68,68,0.5); }

.btn-green { border-color: rgba(34,197,94,0.3); color: var(--green); }
.btn-green:hover { background: var(--green-dim); border-color: rgba(34,197,94,0.5); }

.btn-accent { border-color: rgba(59,130,246,0.3); color: var(--accent); }
.btn-accent:hover { background: var(--accent-dim); border-color: rgba(59,130,246,0.5); }

.action-msg {
  font-family: var(--mono);
  font-size: 12px;
  color: var(--text-dim);
  margin-top: 10px;
  min-height: 18px;
}

/* Project list */
.project-item {
  padding: 12px 16px;
  border: 1px solid var(--border);
  border-radius: 4px;
  margin-bottom: 8px;
  background: var(--bg);
  transition: border-color 0.15s;
}

.project-item:hover { border-color: var(--border-active); }

.project-name {
  font-family: var(--mono);
  font-weight: 600;
  font-size: 14px;
  color: var(--text-bright);
  margin-bottom: 4px;
}

.project-meta {
  font-size: 12px;
  color: var(--text-dim);
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}

.project-meta span { display: flex; align-items: center; gap: 4px; }

/* Log viewer */
.log-controls {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.log-select {
  font-family: var(--mono);
  font-size: 12px;
  padding: 6px 10px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  color: var(--text);
  cursor: pointer;
}

.log-select:focus { outline: none; border-color: var(--accent); }

.log-container {
  background: #060a10;
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 12px;
  max-height: 420px;
  overflow-y: auto;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.7;
  scroll-behavior: smooth;
}

.log-container::-webkit-scrollbar { width: 6px; }
.log-container::-webkit-scrollbar-track { background: transparent; }
.log-container::-webkit-scrollbar-thumb { background: var(--border-active); border-radius: 3px; }

.log-line { white-space: pre-wrap; word-break: break-all; }
.log-line .ts { color: var(--text-dim); }
.log-line .lvl-info { color: var(--accent); }
.log-line .lvl-warn { color: var(--yellow); }
.log-line .lvl-error { color: var(--red); font-weight: 600; }
.log-line .lvl-debug { color: var(--text-dim); }
.log-line .msg { color: var(--text); }

/* JSON viewer */
.json-view {
  background: #060a10;
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 14px;
  max-height: 360px;
  overflow: auto;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.6;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-all;
}

/* Registry info */
.registry-info {
  font-family: var(--mono);
  font-size: 13px;
}

.reg-row {
  display: flex;
  padding: 6px 0;
  border-bottom: 1px solid var(--border);
}

.reg-row:last-child { border-bottom: none; }
.reg-label { color: var(--text-dim); width: 140px; flex-shrink: 0; }
.reg-value { color: var(--text); word-break: break-all; }

/* Empty state */
.empty-state {
  color: var(--text-dim);
  font-family: var(--mono);
  font-size: 13px;
  padding: 24px 0;
  text-align: center;
}

/* Loading pulse */
@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
.loading { animation: pulse 1.5s ease-in-out infinite; }

/* Responsive */
@media (max-width: 860px) {
  .main-grid { grid-template-columns: 1fr; }
  .monitor-header { padding: 16px 20px; }
  .card { padding: 18px; }
}
</style>
</head>
<body>

<div class="monitor-header">
  <h1><span>&gt;</span> wheelmaker<span>-</span>monitor</h1>
  <div class="header-status">
    <div id="hdr-dot" class="status-dot"></div>
    <span id="hdr-label" class="status-label loading">checking...</span>
    <button class="btn btn-accent" onclick="refresh()" style="padding:5px 12px;font-size:11px;">refresh</button>
  </div>
</div>

<div class="main-grid">
  <!-- Process Status -->
  <div class="card">
    <div class="card-title">Processes</div>
    <div id="proc-list"></div>
  </div>

  <!-- Actions -->
  <div class="card">
    <div class="card-title">Service Control</div>
    <div class="actions-row">
      <button class="btn btn-green" onclick="doAction('start')">Start</button>
      <button class="btn btn-accent" onclick="doAction('restart')">Restart</button>
      <button class="btn btn-danger" onclick="doAction('stop')">Stop</button>
    </div>
    <div id="action-msg" class="action-msg"></div>
    <div style="margin-top:20px">
      <div class="card-title" style="margin-bottom:12px">Registry</div>
      <div id="registry-info"></div>
    </div>
  </div>

  <!-- Projects -->
  <div class="card card-full">
    <div class="card-title">Projects</div>
    <div id="project-list"></div>
  </div>

  <!-- Log Viewer -->
  <div class="card card-full">
    <div class="card-title">Logs</div>
    <div class="log-controls">
      <select id="log-file" class="log-select" onchange="loadLogs()">
        <option value="hub">hub.log</option>
        <option value="debug">hub.debug.log</option>
      </select>
      <select id="log-level" class="log-select" onchange="loadLogs()">
        <option value="">All Levels</option>
        <option value="error">Error</option>
        <option value="warn">Warning</option>
        <option value="info">Info</option>
        <option value="debug">Debug</option>
      </select>
      <select id="log-tail" class="log-select" onchange="loadLogs()">
        <option value="100">Last 100</option>
        <option value="200" selected>Last 200</option>
        <option value="500">Last 500</option>
        <option value="1000">Last 1000</option>
      </select>
      <button class="btn" onclick="loadLogs()" style="padding:5px 12px;font-size:11px;">reload</button>
    </div>
    <div id="log-container" class="log-container">
      <div class="empty-state loading">Loading logs...</div>
    </div>
  </div>

  <!-- Config -->
  <div class="card">
    <div class="card-title">Config (config.json)</div>
    <div id="config-view" class="json-view"><span class="loading">Loading...</span></div>
  </div>

  <!-- State -->
  <div class="card">
    <div class="card-title">State (state.json)</div>
    <div id="state-view" class="json-view"><span class="loading">Loading...</span></div>
  </div>
</div>

<script>
const $ = id => document.getElementById(id);

async function api(path) {
  const res = await fetch('/api/' + path);
  return res.json();
}

async function refresh() {
  try {
    const ov = await api('overview');
    renderStatus(ov.service);
    renderConfig(ov.config);
    renderState(ov.state);
    renderRegistry(ov.config);
  } catch(e) {
    $('hdr-dot').className = 'status-dot offline';
    $('hdr-label').textContent = 'error';
    $('hdr-label').className = 'status-label';
  }
  loadLogs();
}

function renderStatus(svc) {
  const dot = $('hdr-dot');
  const label = $('hdr-label');
  label.className = 'status-label';

  if (!svc || !svc.running) {
    dot.className = 'status-dot offline';
    label.textContent = 'offline';
    $('proc-list').innerHTML = '<div class="empty-state">No wheelmaker processes running</div>';
    return;
  }

  dot.className = 'status-dot online';
  label.textContent = svc.processes.length + ' process' + (svc.processes.length !== 1 ? 'es' : '');

  let html = '<table class="proc-table"><thead><tr><th>PID</th><th>Role</th></tr></thead><tbody>';
  for (const p of svc.processes) {
    const cls = p.role === 'guardian' ? 'badge-blue' :
                p.role === 'hub-worker' ? 'badge-green' :
                p.role === 'registry-worker' ? 'badge-yellow' : 'badge-red';
    html += '<tr><td>' + esc(String(p.pid)) + '</td><td><span class="badge ' + cls + '">' + esc(p.role) + '</span></td></tr>';
  }
  html += '</tbody></table>';
  $('proc-list').innerHTML = html;
}

function renderConfig(cfg) {
  $('config-view').textContent = cfg ? JSON.stringify(sanitizeConfig(cfg), null, 2) : 'null';
}

function sanitizeConfig(cfg) {
  if (!cfg || typeof cfg !== 'object') return cfg;
  const copy = JSON.parse(JSON.stringify(cfg));
  if (Array.isArray(copy.projects)) {
    for (const p of copy.projects) {
      if (p.im && p.im.appSecret) p.im.appSecret = '***';
    }
  }
  if (copy.registry && copy.registry.token) copy.registry.token = '***';
  return copy;
}

function renderState(state) {
  $('state-view').textContent = state ? JSON.stringify(state, null, 2) : 'null';
}

function renderRegistry(cfg) {
  const el = $('registry-info');
  if (!cfg || !cfg.registry) {
    el.innerHTML = '<div class="empty-state">No registry configured</div>';
    return;
  }
  const r = cfg.registry;
  let html = '<div class="registry-info">';
  html += row('Mode', r.listen ? 'Local Server' : 'Remote Connect');
  html += row('Server', r.server || '127.0.0.1');
  html += row('Port', String(r.port || 9630));
  html += row('Hub ID', r.hubId || '-');
  html += '</div>';
  el.innerHTML = html;
}

function row(label, value) {
  return '<div class="reg-row"><span class="reg-label">' + esc(label) + '</span><span class="reg-value">' + esc(value) + '</span></div>';
}

function renderProjects(cfg) {
  const el = $('project-list');
  if (!cfg || !cfg.projects || cfg.projects.length === 0) {
    el.innerHTML = '<div class="empty-state">No projects configured</div>';
    return;
  }
  let html = '';
  for (const p of cfg.projects) {
    html += '<div class="project-item">';
    html += '<div class="project-name">' + esc(p.name) + '</div>';
    html += '<div class="project-meta">';
    html += '<span><span class="badge badge-blue">' + esc(p.client?.agent || 'none') + '</span></span>';
    html += '<span><span class="badge badge-yellow">' + esc(p.im?.type || 'none') + '</span></span>';
    html += '<span>' + esc(p.path || '-') + '</span>';
    if (p.yolo) html += '<span><span class="badge badge-red">YOLO</span></span>';
    if (p.debug) html += '<span><span class="badge badge-green">DEBUG</span></span>';
    html += '</div></div>';
  }
  el.innerHTML = html;
}

async function loadLogs() {
  const file = $('log-file').value;
  const level = $('log-level').value;
  const tail = $('log-tail').value;
  const el = $('log-container');

  try {
    const data = await api('logs?file=' + file + '&level=' + level + '&tail=' + tail);
    if (!data.entries || data.entries.length === 0) {
      el.innerHTML = '<div class="empty-state">No log entries</div>';
      return;
    }
    let html = '';
    for (const e of data.entries) {
      const lvlCls = e.level ? 'lvl-' + e.level.toLowerCase() : '';
      html += '<div class="log-line">';
      if (e.time) html += '<span class="ts">' + esc(e.time) + '</span> ';
      if (e.level) html += '<span class="' + lvlCls + '">' + esc(e.level.padEnd(5)) + '</span> ';
      html += '<span class="msg">' + esc(e.message) + '</span>';
      html += '</div>';
    }
    el.innerHTML = html;
    el.scrollTop = el.scrollHeight;
  } catch(e) {
    el.innerHTML = '<div class="empty-state">Failed to load logs</div>';
  }
}

async function doAction(action) {
  const msg = $('action-msg');
  msg.textContent = action + '...';
  msg.style.color = 'var(--text-dim)';

  try {
    const res = await fetch('/api/action/' + action, { method: 'POST' });
    const data = await res.json();
    if (data.error) {
      msg.textContent = 'Error: ' + data.error;
      msg.style.color = 'var(--red)';
    } else {
      msg.textContent = action + ' triggered successfully';
      msg.style.color = 'var(--green)';
      setTimeout(refresh, 3000);
    }
  } catch(e) {
    msg.textContent = 'Request failed: ' + e.message;
    msg.style.color = 'var(--red)';
  }
}

function esc(s) {
  if (!s) return '';
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// Initial load + render projects from overview
(async function init() {
  try {
    const ov = await api('overview');
    renderStatus(ov.service);
    renderConfig(ov.config);
    renderState(ov.state);
    renderRegistry(ov.config);
    renderProjects(ov.config);
  } catch(e) {
    $('hdr-dot').className = 'status-dot offline';
    $('hdr-label').textContent = 'error';
    $('hdr-label').className = 'status-label';
  }
  loadLogs();
})();

// Auto-refresh every 10s
setInterval(async () => {
  try {
    const svc = await api('status');
    renderStatus(svc);
  } catch(e) {}
}, 10000);

// Auto-refresh logs every 15s
setInterval(loadLogs, 15000);
</script>
</body>
</html>
`
