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
  line-height: 1.4;
  height: 100vh;
  overflow: hidden;
}

.monitor-header {
  background: linear-gradient(180deg, #111820 0%, var(--bg) 100%);
  border-bottom: 1px solid var(--border);
  padding: 10px 24px;
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
  font-size: 15px;
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
  font-size: 13px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.main-grid {
  display: flex;
  flex-direction: column;
  gap: 1px;
  background: var(--border);
  margin: 0;
  height: calc(100vh - 52px);
  overflow: hidden;
}

.card {
  background: var(--bg-card);
  padding: 14px 16px;
  min-height: 0;
  overflow: hidden;
}

.card-full {
  grid-column: 1 / -1;
}

.card-title {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  margin-bottom: 6px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.card-title::before {
  content: '';
  width: 3px;
  height: 12px;
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
  padding: 4px 10px;
  font-weight: 600;
  color: var(--text-dim);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 1px;
  border-bottom: 1px solid var(--border);
}

.proc-table td {
  padding: 4px 10px;
  border-bottom: 1px solid var(--border);
  color: var(--text);
}

.proc-table tr:last-child td { border-bottom: none; }

.proc-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.proc-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--border);
  background: var(--bg);
  border-radius: 3px;
  padding: 3px 7px;
  font-family: var(--mono);
  font-size: 10px;
}

.proc-chip .pid {
  color: var(--text-dim);
}

.service-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.service-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 7px;
  border-radius: 3px;
  border: 1px solid var(--border);
  font-family: var(--mono);
  font-size: 10px;
  max-width: 100%;
}

.service-pill .svc-name {
  color: var(--text-bright);
  font-weight: 600;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.service-pill .svc-state {
  color: var(--text-dim);
  white-space: nowrap;
}

.service-pill.service-on {
  background: rgba(34, 197, 94, 0.12);
  border-color: rgba(34, 197, 94, 0.35);
}

.service-pill.service-warn {
  background: rgba(234, 179, 8, 0.1);
  border-color: rgba(234, 179, 8, 0.3);
}

.service-pill.service-off {
  background: rgba(239, 68, 68, 0.1);
  border-color: rgba(239, 68, 68, 0.3);
}

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
  padding: 5px 14px;
  border: 1px solid var(--border);
  border-radius: 3px;
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
  font-size: 11px;
  color: var(--text-dim);
  margin-top: 6px;
  min-height: 16px;
}

/* Operations split */
.ops-layout {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 8px;
}

.ops-col {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 6px 8px;
}

.ops-section {
  margin-bottom: 6px;
}

.ops-section:last-child {
  margin-bottom: 0;
}

.ops-section-title {
  font-family: var(--mono);
  font-size: 10px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-bottom: 6px;
}

.ops-section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.ops-section-head .ops-section-title {
  margin-bottom: 0;
}

.proc-head-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.proc-head-actions .btn {
  padding: 4px 9px;
  font-size: 11px;
}

/* Project list */
.project-item {
  padding: 3px 6px;
  border: 1px solid var(--border);
  border-radius: 3px;
  margin-bottom: 3px;
  background: var(--bg);
  transition: border-color 0.15s;
  display: flex;
  gap: 6px;
  align-items: center;
}

.project-item:hover { border-color: var(--border-active); }

.project-name {
  font-family: var(--mono);
  font-weight: 600;
  font-size: 12px;
  color: var(--text-bright);
  white-space: nowrap;
}

.project-meta {
  font-size: 11px;
  color: var(--text-dim);
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
  margin-left: auto;
}

.project-meta span { display: flex; align-items: center; gap: 4px; }

.project-path {
  font-family: var(--mono);
  font-size: 11px;
  color: var(--text-dim);
  flex: 1 1 auto;
  min-width: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Log viewer */
.log-controls {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
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
  border-radius: 3px;
  padding: 8px 10px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  font-family: var(--mono);
  font-size: 11px;
  line-height: 1.6;
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
  border-radius: 3px;
  padding: 10px;
  height: 100%;
  overflow: auto;
  font-family: var(--mono);
  font-size: 11px;
  line-height: 1.5;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-all;
}

/* Registry info */
.registry-info {
  font-family: var(--mono);
  font-size: 14px;
}

.reg-row {
  display: flex;
  padding: 3px 0;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
}

.reg-row:last-child { border-bottom: none; }
.reg-label { color: var(--text-dim); width: 100px; flex-shrink: 0; }
.reg-value { color: var(--text); word-break: break-all; }

/* Registry live table */
.reg-table { width: 100%; border-collapse: collapse; font-family: var(--mono); font-size: 12px; }
.reg-table th { text-align: left; padding: 4px 8px; color: var(--text-dim); border-bottom: 1px solid var(--border); font-weight: 500; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
.reg-table td { padding: 4px 8px; border-bottom: 1px solid var(--border); color: var(--text); }
.reg-table th, .reg-table td { white-space: nowrap; }
.reg-table tr:last-child td { border-bottom: none; }
.reg-table tr:hover td { background: var(--bg-card-hover); }
.status-symbol { font-size: 13px; margin-right: 5px; vertical-align: middle; }
.status-symbol.on { color: var(--green); }
.status-symbol.off { color: var(--red); }
.git-branch { color: var(--accent); }
.git-dirty { color: var(--yellow); font-weight: 600; }

/* Empty state */
.empty-state {
  color: var(--text-dim);
  font-family: var(--mono);
  font-size: 12px;
  padding: 12px 0;
  text-align: center;
}

/* Loading pulse */
@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
.loading { animation: pulse 1.5s ease-in-out infinite; }

/* Workspace tabs */
.viewer-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.ops-root {
  flex: 0 0 auto;
  overflow: visible;
}

.viewer-tabs {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.viewer-tab {
  font-family: var(--mono);
  font-size: 11px;
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: 3px;
  background: var(--bg);
  color: var(--text-dim);
  cursor: pointer;
}

.viewer-tab.active {
  border-color: var(--accent);
  color: var(--text-bright);
  background: var(--accent-dim);
}

.viewer-body {
  flex: 1;
  min-height: 0;
}

.viewer-panel {
  display: none;
  height: 100%;
}

.viewer-panel.active {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

/* Responsive */
@media (max-width: 860px) {
  body { height: auto; overflow: auto; }
  .main-grid { display: block; height: auto; overflow: visible; }
  .ops-layout { grid-template-columns: 1fr; }
  .viewer-body { min-height: 420px; }
  .log-container { min-height: 280px; }
  .monitor-header { padding: 8px 16px; }
  .monitor-header h1 { font-size: 14px; letter-spacing: 0; }
  .status-label { display: none; }
  .header-status { gap: 8px; }
  .project-item { display: grid; grid-template-columns: 1fr auto; align-items: center; gap: 2px 8px; }
  .project-name { grid-column: 1; font-size: 11px; }
  .project-path { grid-column: 1; width: 100%; min-width: 0; font-size: 10px; }
  .project-meta { grid-column: 2; grid-row: 1 / span 2; margin-left: 0; gap: 4px; justify-content: flex-end; }
  .badge { font-size: 10px; padding: 1px 6px; }
  .service-pill { font-size: 9px; padding: 2px 6px; gap: 4px; }
  .service-pill .svc-name { max-width: 92px; }
  .service-pill .svc-state { max-width: 88px; overflow: hidden; text-overflow: ellipsis; }
  .reg-table th, .reg-table td { padding: 3px 6px; font-size: 11px; }
  .reg-table td:nth-child(3), .reg-table td:nth-child(4) { max-width: 130px; overflow: hidden; text-overflow: ellipsis; }
  .card { padding: 10px 12px; }
}
</style>
</head>
<body>

<div class="monitor-header">
  <h1 id="brand-title">wheelmaker-monitor</h1>
  <div class="header-status">
    <div id="hdr-dot" class="status-dot"></div>
    <span id="hdr-label" class="status-label loading">checking...</span>
    <button class="btn btn-accent" onclick="refresh()" style="padding:5px 12px;font-size:11px;">refresh</button>
    <button class="btn" onclick="doAction('restart-monitor')" style="padding:5px 12px;font-size:11px;">restart monitor</button>
  </div>
</div>

<div class="main-grid">
  <!-- Operations -->
  <div class="card card-full ops-root">
    <div class="card-title">Operations</div>
    <div class="ops-layout">
      <div class="ops-col">
        <div class="ops-section">
          <div class="ops-section-head">
            <div class="ops-section-title">Services & Processes</div>
            <div class="proc-head-actions">
              <button class="btn btn-green" onclick="doAction('start')">Start</button>
              <button class="btn btn-accent" onclick="doAction('restart')">Restart</button>
              <button class="btn btn-danger" onclick="doAction('stop')">Stop</button>
            </div>
          </div>
          <div id="proc-list"></div>
          <div id="action-msg" class="action-msg"></div>
        </div>
        <div class="ops-section">
          <div class="ops-section-title">Hub</div>
          <div id="hub-info"></div>
          <div id="hub-projects" style="margin-top:4px;"></div>
        </div>
      </div>
      <div class="ops-col">
        <div class="ops-section">
          <div class="ops-section-title">Registry Config</div>
          <div id="registry-info"></div>
        </div>
        <div class="ops-section">
          <div class="ops-section-title">Registry Status <span id="reg-status-dot" class="status-dot" style="display:inline-block;margin-left:6px;vertical-align:middle"></span> <span id="reg-status-label" style="font-size:11px;color:var(--text-dim);font-weight:400;margin-left:4px"></span></div>
          <div id="registry-live"></div>
        </div>
      </div>
    </div>
  </div>

  <!-- Workspace -->
  <div class="card card-full viewer-card">
    <div class="card-title">Workspace</div>
    <div class="viewer-tabs">
      <button id="tab-logs" class="viewer-tab active" onclick="switchViewerTab('logs')">Logs</button>
      <button id="tab-state" class="viewer-tab" onclick="switchViewerTab('state')">State</button>
    </div>
    <div class="viewer-body">
      <div id="panel-logs" class="viewer-panel active">
        <div class="log-controls">
          <select id="log-file" class="log-select" onchange="loadLogs()">
            <option value="hub">hub.log</option>
            <option value="debug">hub.debug.log</option>
            <option value="registry">registry.log</option>
            <option value="registry-debug">registry.debug.log</option>
            <option value="updater">updater.log</option>
          </select>
          <select id="log-level" class="log-select" onchange="loadLogs()">
            <option value="">All Levels</option>
            <option value="error">Error+</option>
            <option value="warn">Warn+</option>
            <option value="info">Info+</option>
            <option value="debug">Debug+</option>
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
      <div id="panel-state" class="viewer-panel">
        <div id="state-view" class="json-view"><span class="loading">Loading...</span></div>
      </div>
    </div>
  </div>

  <!-- Config -->
  <div class="card" style="display:none;">
    <div class="card-title">Config (config.json)</div>
    <div id="config-view" class="json-view"><span class="loading">Loading...</span></div>
  </div>

</div>

<script>
const $ = id => document.getElementById(id);

function isNarrowScreen() {
  return window.matchMedia('(max-width: 860px)').matches;
}

function applyResponsiveLabels() {
  const title = $('brand-title');
  if (!title) return;
  title.textContent = isNarrowScreen() ? 'wheelmaker' : 'wheelmaker-monitor';
}

function switchViewerTab(tab) {
  const isLogs = tab === 'logs';
  $('tab-logs').classList.toggle('active', isLogs);
  $('tab-state').classList.toggle('active', !isLogs);
  $('panel-logs').classList.toggle('active', isLogs);
  $('panel-state').classList.toggle('active', !isLogs);
}

async function api(path) {
  const p = window.location.pathname || '/';
  const base = p.startsWith('/monitor') ? '/monitor/' : '/';
  const url = window.location.origin + base + 'api/' + path;
  const res = await fetch(url);
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
  loadRegistryStatus();
}

async function refreshStatusOnly() {
  try {
    const svc = await api('status');
    renderStatus(svc);
  } catch(e) {}
}

function renderStatus(svc) {
  const dot = $('hdr-dot');
  const label = $('hdr-label');
  label.className = 'status-label';
  const services = Array.isArray(svc && svc.services) ? svc.services : [];
  const processes = Array.isArray(svc && svc.processes) ? svc.processes : [];

  if (!svc || !svc.running) {
    dot.className = 'status-dot offline';
    label.textContent = isNarrowScreen() ? '' : 'offline';
    let offlineHtml = '<div class="empty-state">WheelMaker service is offline</div>';
    if (services.length > 0) {
      offlineHtml += '<div style="margin-top:8px;font-family:var(--mono);font-size:11px;color:var(--text-dim);">Services:</div>';
      offlineHtml += '<div class="service-list">';
      for (const s of services) {
        const status = String(s.status || 'Unknown');
        const tone = !s.installed ? 'service-off' :
                     status.toLowerCase() === 'running' ? 'service-on' : 'service-warn';
        offlineHtml += '<div class="service-pill ' + tone + '"><span class="svc-name">' + esc(s.name || '-') + '</span><span class="svc-state">' + esc(status) + '</span></div>';
      }
      offlineHtml += '</div>';
    }
    if (processes.length > 0) {
      offlineHtml += '<div style="margin-top:8px;font-family:var(--mono);font-size:11px;color:var(--yellow);">Residual wheelmaker processes detected:</div>';
      offlineHtml += '<div class="proc-chips" style="margin-top:6px;">';
      for (const p of processes) {
        const cls = p.role === 'guardian' ? 'badge-blue' :
                    p.role === 'hub-worker' ? 'badge-green' :
                    p.role === 'registry-worker' ? 'badge-yellow' : 'badge-red';
        const roleLabel = String(p.role || '').replace('-worker', '');
        offlineHtml += '<div class="proc-chip"><span class="pid">PID#' + esc(String(p.pid)) + '</span><span class="badge ' + cls + '">' + esc(roleLabel) + '</span></div>';
      }
      offlineHtml += '</div>';
    }
    $('proc-list').innerHTML = offlineHtml;
    return;
  }

  dot.className = 'status-dot online';
  label.textContent = isNarrowScreen() ? '' : 'online';

  let html = '';
  if (services.length > 0) {
    html += '<div style="margin-bottom:6px;font-family:var(--mono);font-size:11px;color:var(--text-dim);">Services</div>';
    html += '<div class="service-list" style="margin-bottom:10px;">';
    for (const s of services) {
      const status = String(s.status || 'Unknown');
      const tone = !s.installed ? 'service-off' :
                   status.toLowerCase() === 'running' ? 'service-on' : 'service-warn';
      const showStartType = !isNarrowScreen();
      const startType = showStartType && s.startType && s.startType !== '-' ? (' / ' + s.startType) : '';
      html += '<div class="service-pill ' + tone + '"><span class="svc-name">' + esc(s.name || '-') + '</span><span class="svc-state">' + esc(status + startType) + '</span></div>';
    }
    html += '</div>';
  }

  html += '<div style="margin-bottom:6px;font-family:var(--mono);font-size:11px;color:var(--text-dim);">WheelMaker Processes</div>';
  if (processes.length === 0) {
    html += '<div class="empty-state" style="text-align:left;padding:0;">No wheelmaker.exe process found</div>';
  } else {
    html += '<div class="proc-chips">';
    for (const p of processes) {
      const cls = p.role === 'guardian' ? 'badge-blue' :
                  p.role === 'hub-worker' ? 'badge-green' :
                  p.role === 'registry-worker' ? 'badge-yellow' : 'badge-red';
      const roleLabel = String(p.role || '').replace('-worker', '');
      html += '<div class="proc-chip"><span class="pid">PID#' + esc(String(p.pid)) + '</span><span class="badge ' + cls + '">' + esc(roleLabel) + '</span></div>';
    }
    html += '</div>';
  }
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
  const regEl = $('registry-info');
  const hubEl = $('hub-info');
  const hubProjectsEl = $('hub-projects');
  if (!cfg || !cfg.registry) {
    regEl.innerHTML = '<div class="empty-state">No registry configured</div>';
    hubEl.innerHTML = '<div class="empty-state">No hub info</div>';
    hubProjectsEl.innerHTML = '';
    return;
  }
  const r = cfg.registry;
  const projects = Array.isArray(cfg.projects) ? cfg.projects : [];
  let hubHtml = '<div class="registry-info">';
  hubHtml += row('Hub ID', r.hubId || '-');
  hubHtml += '</div>';
  hubEl.innerHTML = hubHtml;
  if (projects.length === 0) {
    hubProjectsEl.innerHTML = '<div class="empty-state">No projects configured</div>';
  } else {
    let html = '';
    for (const p of projects) {
      html += '<div class="project-item">';
      html += '<div class="project-name">' + esc(p.name) + '</div>';
      html += '<div class="project-path">' + esc(p.path || '-') + '</div>';
      html += '<div class="project-meta">';
      html += '<span><span class="badge badge-blue">' + esc(p.client?.agent || 'none') + '</span></span>';
      html += '<span><span class="badge badge-yellow">' + esc(p.im?.type || 'none') + '</span></span>';
      html += '<span><span class="badge ' + (p.yolo ? 'badge-green' : 'badge-red') + '">' + (p.yolo ? 'yolo' : 'safe') + '</span></span>';
      html += '</div></div>';
    }
    hubProjectsEl.innerHTML = html;
  }

  let regHtml = '<div class="registry-info">';
  regHtml += row('Mode', r.listen ? 'Local Server' : 'Remote Connect');
  regHtml += row('Endpoint', (r.server || '127.0.0.1') + ':' + String(r.port || 9630));
  regHtml += '</div>';
  regEl.innerHTML = regHtml;
}

async function loadRegistryStatus() {
  const el = $('registry-live');
  const dot = $('reg-status-dot');
  const label = $('reg-status-label');
  try {
    const data = await api('registry');
    if (!data.connected) {
      dot.className = 'status-dot offline';
      label.textContent = data.error || 'disconnected';
      el.innerHTML = '<div class="empty-state">' + esc(data.error || 'Registry not connected') + '</div>';
      return;
    }
    const projects = data.projects || [];
    const onlineCount = projects.filter(p => p.online).length;
    dot.className = 'status-dot online';
    label.textContent = onlineCount + '/' + projects.length + ' online';
    if (projects.length === 0) {
      el.innerHTML = '<div class="empty-state">No projects registered</div>';
      return;
    }
    let html = '<table class="reg-table"><thead><tr>';
    const compact = isNarrowScreen();
    html += '<th>' + (compact ? 'S' : 'Status') + '</th><th>Hub</th><th>Project</th><th>Branch</th><th>' + (compact ? 'D' : 'Dirty') + '</th>';
    html += '</tr></thead><tbody>';
    for (const p of projects) {
      const dotCls = p.online ? 'on' : 'off';
      const statusText = p.online ? 'online' : 'offline';
      const hubId = String(p.projectId || '').includes(':') ? String(p.projectId).split(':')[0] : '-';
      html += '<tr>';
      html += '<td><span class="status-symbol ' + dotCls + '">●</span>' + (compact ? '' : statusText) + '</td>';
      html += '<td>' + esc(hubId) + '</td>';
      html += '<td>' + esc(p.name || p.projectId) + '</td>';
      html += '<td>' + (p.git && p.git.branch ? '<span class="git-branch">' + esc(p.git.branch) + '</span>' : '-') + '</td>';
      html += '<td>' + (p.git && p.git.dirty ? '<span class="git-dirty">*</span>' : '-') + '</td>';
      html += '</tr>';
    }
    html += '</tbody></table>';
    el.innerHTML = html;
  } catch(e) {
    dot.className = 'status-dot offline';
    label.textContent = 'error';
    el.innerHTML = '<div class="empty-state">Failed to load registry status</div>';
  }
}

function row(label, value) {
  return '<div class="reg-row"><span class="reg-label">' + esc(label) + '</span><span class="reg-value">' + esc(value) + '</span></div>';
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
    const p = window.location.pathname || '/';
    const base = p.startsWith('/monitor') ? '/monitor/' : '/';
    const url = window.location.origin + base + 'api/action/' + action;
    const res = await fetch(url, { method: 'POST' });
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

// Initial load
(async function init() {
  applyResponsiveLabels();
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
  loadRegistryStatus();
})();

window.addEventListener('resize', applyResponsiveLabels);

// Auto-refresh service/process status every 5s while tab is visible.
setInterval(() => {
  if (document.hidden) return;
  refreshStatusOnly();
}, 5000);

window.addEventListener('visibilitychange', () => {
  if (!document.hidden) refreshStatusOnly();
});

// Auto-refresh logs every 15s
setInterval(loadLogs, 15000);

// Auto-refresh registry status every 15s
setInterval(loadRegistryStatus, 15000);
</script>
</body>
</html>
`
