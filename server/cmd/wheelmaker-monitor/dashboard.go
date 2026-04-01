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
@import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@300;400;500;600;700&display=swap');

:root {
  --bg: #080c11;
  --surface: #0e1520;
  --surface-2: #141e2c;
  --border: #1a2535;
  --border-hi: #243347;
  --text: #b0bcc9;
  --text-dim: #526070;
  --text-hi: #dde5ee;
  --accent: #3b82f6;
  --accent-bg: #0d2040;
  --green: #22c55e;
  --green-bg: #081f10;
  --red: #ef4444;
  --red-bg: #1a0808;
  --yellow: #eab308;
  --yellow-bg: #1a1500;
  --mono: 'JetBrains Mono', 'Consolas', monospace;
}

*, *::before, *::after { margin:0; padding:0; box-sizing:border-box; }

html, body {
  height: 100%;
  background: var(--bg);
  color: var(--text);
  font-family: var(--mono);
  font-size: 13px;
  line-height: 1.45;
  overflow: hidden;
}

::-webkit-scrollbar { width: 4px; height: 4px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--border-hi); border-radius: 2px; }

/* Shell: header + body */
.shell {
  display: grid;
  grid-template-rows: 40px 1fr;
  height: 100vh;
}

/* Top bar */
.topbar {
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 14px;
  z-index: 10;
  flex-shrink: 0;
}

.topbar-brand {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-hi);
  letter-spacing: .5px;
  white-space: nowrap;
}
.topbar-brand span { color: var(--accent); }

.topbar-sep { flex: 1; }

.topbar-status { display: flex; align-items: center; gap: 6px; }

.dot {
  width: 7px; height: 7px;
  border-radius: 50%;
  background: var(--text-dim);
  flex-shrink: 0;
  transition: background .3s, box-shadow .3s;
}
.dot.online  { background: var(--green); box-shadow: 0 0 6px rgba(34,197,94,.55); }
.dot.offline { background: var(--red);   box-shadow: 0 0 6px rgba(239,68,68,.55); }

.topbar-label {
  font-size: 11px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.tbtn {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 600;
  padding: 3px 10px;
  border: 1px solid var(--border-hi);
  border-radius: 2px;
  background: transparent;
  color: var(--text);
  cursor: pointer;
  text-transform: uppercase;
  letter-spacing: .5px;
  transition: all .12s;
  white-space: nowrap;
}
.tbtn:hover { background: var(--surface-2); color: var(--text-hi); border-color: var(--accent); }
.tbtn:active { transform: scale(.96); }

/* Body: sidebar | main */
.body {
  display: grid;
  grid-template-columns: 240px 1fr;
  min-height: 0;
  overflow: hidden;
}

/* Sidebar */
.sidebar {
  background: var(--surface);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  overflow-x: hidden;
}

.sbar-section {
  border-bottom: 1px solid var(--border);
  padding: 10px 12px;
  flex-shrink: 0;
}

.sbar-title {
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  margin-bottom: 8px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.sbar-title .ttl-accent {
  width: 2px; height: 10px;
  background: var(--accent);
  border-radius: 1px;
  flex-shrink: 0;
}

/* Service rows */
.svc-list { display: flex; flex-direction: column; gap: 3px; }

.svc-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 6px;
  border-radius: 2px;
  border: 1px solid transparent;
  font-size: 11px;
}
.svc-row.on   { background: var(--green-bg);  border-color: rgba(34,197,94,.15); }
.svc-row.off  { background: var(--red-bg);    border-color: rgba(239,68,68,.15); }
.svc-row.warn { background: var(--yellow-bg); border-color: rgba(234,179,8,.15); }

.svc-dot { width: 5px; height: 5px; border-radius: 50%; flex-shrink: 0; }
.svc-row.on   .svc-dot { background: var(--green); }
.svc-row.off  .svc-dot { background: var(--red); }
.svc-row.warn .svc-dot { background: var(--yellow); }

.svc-name {
  flex: 1;
  color: var(--text-hi);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
}
.svc-state { color: var(--text-dim); font-size: 10px; white-space: nowrap; }

/* Process chips */
.proc-chips { display: flex; flex-direction: column; gap: 3px; }

.proc-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 6px;
  border: 1px solid var(--border);
  border-radius: 2px;
  font-size: 11px;
}
.proc-chip .pid { color: var(--text-dim); font-size: 10px; }

/* Badges */
.badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 2px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: .2px;
  white-space: nowrap;
}
.badge-blue   { background: var(--accent-bg); color: var(--accent);  border: 1px solid rgba(59,130,246,.2); }
.badge-green  { background: var(--green-bg);  color: var(--green);   border: 1px solid rgba(34,197,94,.2); }
.badge-yellow { background: var(--yellow-bg); color: var(--yellow);  border: 1px solid rgba(234,179,8,.2); }
.badge-red    { background: var(--red-bg);    color: var(--red);     border: 1px solid rgba(239,68,68,.2); }

/* Action buttons */
.action-btns { display: flex; gap: 5px; flex-wrap: wrap; }

.abtn {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 700;
  padding: 4px 10px;
  border-radius: 2px;
  border: 1px solid var(--border-hi);
  background: transparent;
  color: var(--text);
  cursor: pointer;
  text-transform: uppercase;
  letter-spacing: .4px;
  transition: all .12s;
  flex: 1 1 auto;
}
.abtn:hover  { color: var(--text-hi); background: var(--surface-2); }
.abtn:active { transform: scale(.95); }
.abtn:disabled { opacity: .35; cursor: not-allowed; }
.abtn-green:hover  { background: var(--green-bg);  border-color: rgba(34,197,94,.5);  color: var(--green); }
.abtn-accent:hover { background: var(--accent-bg); border-color: rgba(59,130,246,.5); color: var(--accent); }
.abtn-red:hover    { background: var(--red-bg);    border-color: rgba(239,68,68,.5);  color: var(--red); }

.action-msg { font-size: 11px; color: var(--text-dim); margin-top: 6px; min-height: 14px; }

/* Hub / project list */
.hub-id-row { font-size: 11px; color: var(--text-dim); margin-bottom: 5px; }
.hub-id-row span { color: var(--text); font-weight: 600; }

.proj-list { display: flex; flex-direction: column; gap: 3px; }

.proj-item {
  padding: 4px 6px;
  border: 1px solid var(--border);
  border-radius: 2px;
  transition: border-color .12s;
}
.proj-item:hover { border-color: var(--border-hi); }
.proj-name {
  font-size: 12px; font-weight: 700; color: var(--text-hi);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.proj-path {
  font-size: 10px; color: var(--text-dim); margin-top: 1px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.proj-badges { display: flex; gap: 4px; margin-top: 3px; flex-wrap: wrap; }

/* Registry config rows */
.reg-cfg { display: flex; flex-direction: column; gap: 3px; }
.reg-row { display: flex; gap: 6px; font-size: 11px; line-height: 1.4; }
.reg-row .rl { color: var(--text-dim); flex-shrink: 0; min-width: 58px; }
.reg-row .rv { color: var(--text); word-break: break-all; }

/* Main panel */
.main {
  display: grid;
  grid-template-rows: auto 1fr;
  min-height: 0;
  overflow: hidden;
}

/* Registry live strip */
.strip {
  border-bottom: 1px solid var(--border);
  display: grid;
  grid-template-columns: 1fr;
  flex-shrink: 0;
  min-height: 0;
  max-height: 200px;
  overflow: hidden;
}

.strip-panel {
  padding: 8px 14px;
  overflow: auto;
  min-width: 0;
}
.strip-title {
  font-size: 9px; font-weight: 700;
  text-transform: uppercase; letter-spacing: 1.5px;
  color: var(--text-dim); margin-bottom: 6px;
  display: flex; align-items: center; gap: 6px;
  flex-shrink: 0;
}
.strip-title .ttl-accent { width: 2px; height: 10px; background: var(--accent); border-radius: 1px; flex-shrink: 0; }

.reg-hdr-extra { display: flex; align-items: center; gap: 5px; margin-left: auto; }
.reg-hdr-extra .dot { width: 6px; height: 6px; }

/* Registry table */
.reg-table { width: 100%; border-collapse: collapse; font-size: 11.5px; }
.reg-table th {
  text-align: left; padding: 2px 8px;
  font-size: 9px; font-weight: 700;
  text-transform: uppercase; letter-spacing: 1px;
  color: var(--text-dim); border-bottom: 1px solid var(--border);
  white-space: nowrap;
}
.reg-table td {
  padding: 3px 8px; border-bottom: 1px solid var(--border);
  color: var(--text); white-space: nowrap;
  overflow: hidden; text-overflow: ellipsis; max-width: 200px;
}
.reg-table tr:last-child td { border-bottom: none; }
.reg-table tr:hover td { background: var(--surface-2); }
.s-on  { color: var(--green); }
.s-off { color: var(--red); }
.git-branch { color: var(--accent); }
.git-dirty  { color: var(--yellow); font-weight: 700; }

/* Log area */
.log-area {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.log-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  flex-wrap: wrap;
  background: var(--surface);
}

.log-toolbar-tabs { display: flex; gap: 2px; }

.vtab {
  font-family: var(--mono);
  font-size: 11px; font-weight: 600;
  padding: 3px 10px;
  border: 1px solid transparent;
  border-radius: 2px;
  background: transparent;
  color: var(--text-dim);
  cursor: pointer;
  text-transform: uppercase;
  letter-spacing: .5px;
  transition: all .1s;
  white-space: nowrap;
}
.vtab.active { border-color: var(--accent); background: var(--accent-bg); color: var(--text-hi); }
.vtab:not(.active):hover { color: var(--text); border-color: var(--border-hi); }

.log-sep { width: 1px; height: 16px; background: var(--border); margin: 0 2px; flex-shrink: 0; }
.log-fill { flex: 1; }

.log-sel {
  font-family: var(--mono);
  font-size: 11px; padding: 3px 8px;
  background: var(--bg); border: 1px solid var(--border);
  border-radius: 2px; color: var(--text); cursor: pointer; outline: none;
}
.log-sel:focus { border-color: var(--accent); }

.log-body { flex: 1; min-height: 0; overflow: hidden; }

.log-panel { display: none; height: 100%; flex-direction: column; min-height: 0; }
.log-panel.active { display: flex; }

.log-scroll {
  flex: 1; min-height: 0; overflow-y: auto;
  padding: 8px 14px; background: #050810;
  font-size: 11.5px; line-height: 1.55;
}

.log-line { white-space: pre-wrap; word-break: break-all; }
.log-line .ts      { color: var(--text-dim); }
.log-line .lvl-info  { color: var(--accent); }
.log-line .lvl-warn  { color: var(--yellow); }
.log-line .lvl-error { color: var(--red); font-weight: 700; }
.log-line .lvl-debug { color: var(--text-dim); }
.log-line .msg { color: var(--text); }

.json-view {
  flex: 1; min-height: 0; overflow: auto;
  padding: 10px 14px; background: #050810;
  font-size: 11.5px; line-height: 1.5;
  white-space: pre-wrap; word-break: break-all; color: var(--text);
}

.empty-state { color: var(--text-dim); font-size: 11px; padding: 8px 0; text-align: center; }

@keyframes pulse { 0%,100%{ opacity:.4 } 50%{ opacity:1 } }
.loading { animation: pulse 1.5s ease-in-out infinite; }

/* Mobile portrait (<= 640px) */
@media (max-width: 640px) {
  html, body { overflow: auto; height: auto; }
  .shell { grid-template-rows: 40px auto; height: auto; min-height: 100dvh; }
  .body {
    grid-template-columns: 1fr;
    grid-template-rows: auto auto;
    overflow: visible; height: auto;
  }
  .sidebar {
    border-right: none; border-bottom: 1px solid var(--border);
    overflow: visible; height: auto;
    flex-direction: row; flex-wrap: wrap;
  }
  .sbar-section { flex: 1 1 50%; min-width: 0; }
  .main { grid-template-rows: auto auto; height: auto; overflow: visible; }
  .strip { grid-template-columns: 1fr; max-height: none; }
  .strip-panel + .strip-panel { border-left: none; border-top: 1px solid var(--border); }
  .log-area { height: 100dvh; min-height: 420px; }
  .log-scroll { min-height: 300px; }
  .json-view { min-height: 300px; }
  .topbar-label { display: none; }
  .tbtn { font-size: 10px; padding: 3px 8px; }
}

/* Tablet (641-1023) */
@media (min-width: 641px) and (max-width: 1023px) {
  .body { grid-template-columns: 200px 1fr; }
}
</style>
</head>
<body>

<div class="shell">

  <div class="topbar">
    <div class="topbar-brand">wheel<span>maker</span>&#x2011;monitor</div>
    <div class="topbar-sep"></div>
    <div class="topbar-status">
      <div id="hdr-dot" class="dot"></div>
      <span id="hdr-label" class="topbar-label loading">checking&#x2026;</span>
    </div>
    <button class="tbtn" onclick="refresh()">Refresh</button>
    <button class="tbtn" onclick="doAction('restart-monitor')">Restart Monitor</button>
  </div>

  <div class="body">

    <div class="sidebar">

      <div class="sbar-section">
        <div class="sbar-title"><span class="ttl-accent"></span>Services</div>
        <div id="svc-list" class="svc-list"><div class="empty-state loading">Loading&#x2026;</div></div>
      </div>

      <div class="sbar-section">
        <div class="sbar-title"><span class="ttl-accent"></span>Processes</div>
        <div id="proc-chips" class="proc-chips"><div class="empty-state loading">Loading&#x2026;</div></div>
      </div>

      <div class="sbar-section">
        <div class="sbar-title"><span class="ttl-accent"></span>Actions</div>
        <div class="action-btns">
          <button class="abtn abtn-green"  onclick="doAction('start')">Start</button>
          <button class="abtn abtn-accent" onclick="doAction('restart')">Restart</button>
          <button class="abtn abtn-red"    onclick="doAction('stop')">Stop</button>
        </div>
        <div id="action-msg" class="action-msg"></div>
      </div>

      <div class="sbar-section">
        <div class="sbar-title"><span class="ttl-accent"></span>Hub</div>
        <div id="hub-id" class="hub-id-row">ID: <span>&#x2014;</span></div>
        <div id="proj-list" class="proj-list"><div class="empty-state">No projects</div></div>
      </div>

      <div class="sbar-section">
        <div class="sbar-title"><span class="ttl-accent"></span>Registry Config</div>
        <div id="reg-cfg" class="reg-cfg"><div class="empty-state">No config</div></div>
      </div>

    </div>

    <div class="main">

      <div class="strip">
        <div class="strip-panel">
          <div class="strip-title">
            <span class="ttl-accent"></span>Registry Live
            <span class="reg-hdr-extra">
              <span id="reg-dot" class="dot"></span>
              <span id="reg-label" style="font-size:10px;color:var(--text-dim);"></span>
            </span>
          </div>
          <div id="reg-live" style="overflow-x:auto;"><div class="empty-state loading">Loading&#x2026;</div></div>
        </div>
      </div>

      <div class="log-area">

        <div class="log-toolbar">
          <div class="log-toolbar-tabs">
            <button id="vtab-logs"  class="vtab active" onclick="switchTab('logs')">Logs</button>
            <button id="vtab-state" class="vtab"        onclick="switchTab('state')">State JSON</button>
          </div>
          <div class="log-sep"></div>
          <select id="log-file"  class="log-sel" onchange="loadLogs()">
            <option value="hub">hub.log</option>
            <option value="debug">hub.debug.log</option>
            <option value="registry">registry.log</option>
            <option value="registry-debug">registry.debug.log</option>
            <option value="updater">updater.log</option>
          </select>
          <select id="log-level" class="log-sel" onchange="loadLogs()">
            <option value="">All</option>
            <option value="error">Error+</option>
            <option value="warn">Warn+</option>
            <option value="info">Info+</option>
            <option value="debug">Debug+</option>
          </select>
          <select id="log-tail" class="log-sel" onchange="loadLogs()">
            <option value="100">&#xd7;100</option>
            <option value="200" selected>&#xd7;200</option>
            <option value="500">&#xd7;500</option>
            <option value="1000">&#xd7;1000</option>
          </select>
          <div class="log-fill"></div>
          <button class="tbtn" onclick="loadLogs()">Reload</button>
        </div>

        <div class="log-body">
          <div id="panel-logs" class="log-panel active">
            <div id="log-scroll" class="log-scroll"><div class="empty-state loading">Loading logs&#x2026;</div></div>
          </div>
          <div id="panel-state" class="log-panel">
            <div id="state-view" class="json-view"><span class="loading">Loading&#x2026;</span></div>
          </div>
        </div>

      </div>

    </div>

  </div>
</div>

<script>
const $ = id => document.getElementById(id);

function switchTab(tab) {
  const isLogs = tab === 'logs';
  $('vtab-logs').classList.toggle('active', isLogs);
  $('vtab-state').classList.toggle('active', !isLogs);
  $('panel-logs').classList.toggle('active', isLogs);
  $('panel-state').classList.toggle('active', !isLogs);
}

async function api(path) {
  const p = window.location.pathname || '/';
  const base = p.startsWith('/monitor') ? '/monitor/' : '/';
  const res = await fetch(window.location.origin + base + 'api/' + path);
  return res.json();
}

function esc(s) {
  if (s == null) return '';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

async function refresh() {
  try {
    const ov = await api('overview');
    renderStatus(ov.service);
    renderSidebar(ov.config);
    renderStateJSON(ov.state);
  } catch(e) {
    $('hdr-dot').className = 'dot offline';
    $('hdr-label').textContent = 'error';
    $('hdr-label').className = 'topbar-label';
  }
  loadLogs();
  loadRegistryStatus();
}

async function refreshStatusOnly() {
  try {
    const svc = await api('status');
    renderStatus(svc);
  } catch(_) {}
}

function renderStatus(svc) {
  const dot = $('hdr-dot');
  const lbl = $('hdr-label');
  lbl.className = 'topbar-label';

  const services  = Array.isArray(svc && svc.services)  ? svc.services  : [];
  const processes = Array.isArray(svc && svc.processes) ? svc.processes : [];

  const running = svc && svc.running;
  dot.className = 'dot ' + (running ? 'online' : 'offline');
  lbl.textContent = running ? 'online' : 'offline';

  if (services.length === 0) {
    $('svc-list').innerHTML = '<div class="empty-state">No services</div>';
  } else {
    $('svc-list').innerHTML = services.map(s => {
      const status = String(s.status || 'Unknown');
      const cls = !s.installed ? 'off'
                : status.toLowerCase() === 'running' ? 'on' : 'warn';
      const stateText = status.toLowerCase() === 'running' ? '' : status;
      return '<div class="svc-row ' + cls + '">' +
        '<div class="svc-dot"></div>' +
        '<div class="svc-name">' + esc(s.name || '-') + '</div>' +
        (stateText ? '<div class="svc-state">' + esc(stateText) + '</div>' : '') +
        '</div>';
    }).join('');
  }

  if (processes.length === 0) {
    $('proc-chips').innerHTML = '<div class="empty-state">No wheelmaker.exe</div>';
  } else {
    $('proc-chips').innerHTML = processes.map(p => {
      const cls = p.role === 'guardian'       ? 'badge-blue'
                : p.role === 'hub-worker'      ? 'badge-green'
                : p.role === 'registry-worker' ? 'badge-yellow' : 'badge-red';
      const roleLabel = String(p.role || '').replace('-worker', '');
      return '<div class="proc-chip"><span class="pid">PID#' + esc(String(p.pid)) + '</span>' +
             '<span class="badge ' + cls + '">' + esc(roleLabel) + '</span></div>';
    }).join('');
  }
}

function renderSidebar(cfg) {
  if (!cfg) return;
  const r = cfg.registry || {};

  $('hub-id').innerHTML = 'ID: <span>' + esc(r.hubId || '&#x2014;') + '</span>';

  const projects = Array.isArray(cfg.projects) ? cfg.projects : [];
  if (projects.length === 0) {
    $('proj-list').innerHTML = '<div class="empty-state">No projects</div>';
  } else {
    $('proj-list').innerHTML = projects.map(p => {
      const yoloCls = p.yolo ? 'badge-green' : 'badge-red';
      return '<div class="proj-item">' +
        '<div class="proj-name">' + esc(p.name) + '</div>' +
        '<div class="proj-path">' + esc(p.path || '') + '</div>' +
        '<div class="proj-badges">' +
          '<span class="badge badge-blue">' + esc(p.client && p.client.agent ? p.client.agent : 'none') + '</span>' +
          '<span class="badge badge-yellow">' + esc(p.im && p.im.type ? p.im.type : 'none') + '</span>' +
          '<span class="badge ' + yoloCls + '">' + (p.yolo ? 'yolo' : 'safe') + '</span>' +
        '</div>' +
        '</div>';
    }).join('');
  }

  let regHtml = '';
  if (r.hubId || r.server || r.port) {
    regHtml += cfgRow('Mode',     r.listen ? 'Local Server' : 'Remote Connect');
    regHtml += cfgRow('Endpoint', (r.server || '127.0.0.1') + ':' + String(r.port || 9630));
  } else {
    regHtml = '<div class="empty-state">No registry configured</div>';
  }
  $('reg-cfg').innerHTML = regHtml;
}

function cfgRow(label, value) {
  return '<div class="reg-row"><span class="rl">' + esc(label) + '</span><span class="rv">' + esc(value) + '</span></div>';
}

function renderStateJSON(state) {
  $('state-view').textContent = state ? JSON.stringify(state, null, 2) : 'null';
}

async function loadRegistryStatus() {
  const el  = $('reg-live');
  const dot = $('reg-dot');
  const lbl = $('reg-label');
  try {
    const data = await api('registry');
    if (!data.connected) {
      dot.className = 'dot offline';
      lbl.textContent = data.error || 'disconnected';
      el.innerHTML = '<div class="empty-state">' + esc(data.error || 'Registry not connected') + '</div>';
      return;
    }
    const projects = data.projects || [];
    const onlineN  = projects.filter(p => p.online).length;
    dot.className = 'dot online';
    lbl.textContent = onlineN + '/' + projects.length + ' online';
    if (projects.length === 0) {
      el.innerHTML = '<div class="empty-state">No projects registered</div>';
      return;
    }
    let html = '<table class="reg-table"><thead><tr>' +
      '<th>&#x25CF;</th><th>Hub</th><th>Project</th><th>Branch</th><th>Dirty</th>' +
      '</tr></thead><tbody>';
    for (const p of projects) {
      const hubId = String(p.projectId || '').includes(':') ? String(p.projectId).split(':')[0] : '-';
      html += '<tr>' +
        '<td><span class="' + (p.online ? 's-on' : 's-off') + '">&#x25CF;</span></td>' +
        '<td>' + esc(hubId) + '</td>' +
        '<td>' + esc(p.name || p.projectId) + '</td>' +
        '<td>' + (p.git && p.git.branch ? '<span class="git-branch">' + esc(p.git.branch) + '</span>' : '&#x2014;') + '</td>' +
        '<td>' + (p.git && p.git.dirty  ? '<span class="git-dirty">&#x2731;</span>' : '&#x2014;') + '</td>' +
        '</tr>';
    }
    html += '</tbody></table>';
    el.innerHTML = html;
  } catch(e) {
    dot.className = 'dot offline';
    lbl.textContent = 'error';
    el.innerHTML = '<div class="empty-state">Failed to load registry status</div>';
  }
}

async function loadLogs() {
  const file  = $('log-file').value;
  const level = $('log-level').value;
  const tail  = $('log-tail').value;
  const el    = $('log-scroll');
  try {
    const data = await api('logs?file=' + file + '&level=' + level + '&tail=' + tail);
    if (!data.entries || data.entries.length === 0) {
      el.innerHTML = '<div class="empty-state">No log entries</div>';
      return;
    }
    el.innerHTML = data.entries.map(e => {
      const lvlCls = e.level ? 'lvl-' + e.level.toLowerCase() : '';
      return '<div class="log-line">' +
        (e.time  ? '<span class="ts">' + esc(e.time) + '</span> ' : '') +
        (e.level ? '<span class="' + lvlCls + '">' + esc(e.level.padEnd(5)) + '</span> ' : '') +
        '<span class="msg">' + esc(e.message) + '</span>' +
        '</div>';
    }).join('');
    el.scrollTop = el.scrollHeight;
  } catch(e) {
    el.innerHTML = '<div class="empty-state">Failed to load logs</div>';
  }
}

async function doAction(action) {
  const msg = $('action-msg');
  msg.textContent = action + '\u2026';
  msg.style.color = 'var(--text-dim)';
  try {
    const p = window.location.pathname || '/';
    const base = p.startsWith('/monitor') ? '/monitor/' : '/';
    const res  = await fetch(window.location.origin + base + 'api/action/' + action, { method: 'POST' });
    const data = await res.json();
    if (data.error) {
      msg.textContent = 'Error: ' + data.error;
      msg.style.color = 'var(--red)';
    } else {
      msg.textContent = action + ' triggered';
      msg.style.color = 'var(--green)';
      setTimeout(refresh, 3000);
    }
  } catch(e) {
    msg.textContent = 'Request failed: ' + e.message;
    msg.style.color = 'var(--red)';
  }
}

(async function init() {
  try {
    const ov = await api('overview');
    renderStatus(ov.service);
    renderSidebar(ov.config);
    renderStateJSON(ov.state);
  } catch(e) {
    $('hdr-dot').className = 'dot offline';
    $('hdr-label').textContent = 'error';
    $('hdr-label').className = 'topbar-label';
  }
  loadLogs();
  loadRegistryStatus();
})();

setInterval(() => { if (!document.hidden) refreshStatusOnly(); }, 5000);
setInterval(loadLogs, 15000);
setInterval(loadRegistryStatus, 15000);
window.addEventListener('visibilitychange', () => { if (!document.hidden) refreshStatusOnly(); });
</script>
</body>
</html>
`
