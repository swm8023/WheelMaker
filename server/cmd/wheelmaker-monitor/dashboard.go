package main

import (
	"net/http"
	"strings"
)

func handleDashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/monitor/" && r.URL.Path != "/monitor" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderDashboardHTML(r.URL.Path)))
	}
}

func renderDashboardHTML(path string) string {
	base := pwaBasePath(path)
	replacer := strings.NewReplacer(
		"__WM_MANIFEST__", pwaJoin(base, "manifest.webmanifest"),
		"__WM_ICON__", pwaJoin(base, "icons/icon.svg"),
	)
	return replacer.Replace(dashboardHTML)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="theme-color" content="#0e1520">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
<link id="wm-manifest" rel="manifest" href="__WM_MANIFEST__">
<link id="wm-icon" rel="icon" type="image/svg+xml" href="__WM_ICON__">
<link id="wm-apple-icon" rel="apple-touch-icon" href="__WM_ICON__">
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

.topbar-hub { display:flex; align-items:center; gap:6px; margin-left: 10px; }
.topbar-hub-label { font-size:10px; color:var(--text-dim); text-transform:uppercase; letter-spacing:1px; }
.topbar-hub-select { font-family: var(--mono); font-size: 11px; padding: 3px 8px; background: var(--bg); border: 1px solid var(--border); border-radius: 2px; color: var(--text); min-width: 140px; }
.topbar-hub-select:focus { border-color: var(--accent); outline: none; }

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
  flex-wrap: wrap;
  gap: 6px;
  padding: 3px 6px;
  border: 1px solid var(--border);
  border-radius: 2px;
  font-size: 11px;
}
.proc-chip .pid { color: var(--text-dim); font-size: 10px; }
.proc-chip .ptime { color: var(--text-dim); font-size: 10px; margin-left: auto; }

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

.db-view {
  flex: 1; min-height: 0; overflow: auto;
  padding: 10px 14px; background: #050810;
}
.db-toolbar {
  display: flex;
  justify-content: flex-end;
  padding: 10px 14px 0;
}
.db-section { margin-bottom: 14px; }
.db-table-title {
  font-size: 10px; font-weight: 700;
  text-transform: uppercase; letter-spacing: 1px;
  color: var(--accent); margin-bottom: 6px;
  display: flex; align-items: center; gap: 8px;
}
.db-count {
  font-size: 10px; font-weight: 400;
  color: var(--text-dim); letter-spacing: 0;
  text-transform: none;
}
.db-table-wrap { overflow-x: auto; }

/* JSON modal */
.hidden { display: none !important; }
.json-cell-btn {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 700;
  padding: 2px 8px;
  border: 1px solid rgba(59,130,246,.35);
  border-radius: 2px;
  background: var(--accent-bg);
  color: var(--accent);
  cursor: pointer;
  text-transform: uppercase;
  letter-spacing: .3px;
}
.json-cell-btn:hover { border-color: var(--accent); color: var(--text-hi); }
.tbl-muted { color: var(--text-lo); font-size: 11px; }

.json-modal {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: grid;
  place-items: center;
}
.json-modal-backdrop {
  position: absolute;
  inset: 0;
  background: rgba(3, 8, 15, .72);
}
.json-modal-panel {
  position: relative;
  width: min(960px, calc(100vw - 28px));
  max-height: calc(100vh - 28px);
  background: var(--surface);
  border: 1px solid var(--border-hi);
  border-radius: 4px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 20px 48px rgba(0, 0, 0, .45);
}
.json-modal-header {
  height: 40px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-2);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px 0 12px;
}
.json-modal-title {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: .6px;
  text-transform: uppercase;
  color: var(--text-hi);
}
.json-modal-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  margin-right: 10px;
}
.json-modal-tab {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 700;
  border: 1px solid var(--border-hi);
  border-radius: 2px;
  background: transparent;
  color: var(--text);
  padding: 3px 8px;
  text-transform: uppercase;
  letter-spacing: .4px;
  cursor: pointer;
}
.json-modal-tab:hover {
  border-color: var(--accent);
  color: var(--text-hi);
}
.json-modal-tab.active {
  border-color: var(--accent);
  color: var(--text-hi);
  background: var(--accent-bg);
}
.json-modal-close {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 700;
  border: 1px solid var(--border-hi);
  border-radius: 2px;
  background: transparent;
  color: var(--text);
  padding: 3px 8px;
  text-transform: uppercase;
  letter-spacing: .4px;
  cursor: pointer;
}
.json-modal-close:hover {
  border-color: var(--accent);
  color: var(--text-hi);
  background: var(--accent-bg);
}
.json-modal-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 12px;
  background: #050810;
}
.json-card {
  border: 1px solid var(--border);
  border-radius: 3px;
  background: var(--surface);
  margin-bottom: 10px;
}
.json-card-hd {
  padding: 7px 10px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.json-card-title {
  color: var(--text-hi);
  font-size: 11px;
  font-weight: 700;
}
.json-grid { padding: 8px 10px; display: grid; grid-template-columns: 130px 1fr; gap: 5px 8px; }
.json-k { color: var(--text-dim); font-size: 10px; text-transform: uppercase; letter-spacing: .4px; }
.json-v { color: var(--text); word-break: break-word; }
.json-subsection { border-top: 1px dashed var(--border); padding: 8px 10px; }
.json-subtitle {
  color: var(--accent);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: .5px;
  margin-bottom: 6px;
}
.json-list { display: flex; flex-wrap: wrap; gap: 5px; }
.json-pill {
  display: inline-block;
  border: 1px solid var(--border-hi);
  border-radius: 2px;
  padding: 1px 6px;
  font-size: 10px;
  color: var(--text);
  background: var(--surface-2);
}
.json-table-wrap { overflow-x: auto; }
.json-mini-table { width: 100%; border-collapse: collapse; font-size: 11px; }
.json-mini-table th, .json-mini-table td {
  text-align: left;
  border-bottom: 1px solid var(--border);
  padding: 3px 6px;
  white-space: nowrap;
}
.json-mini-table th {
  color: var(--text-dim);
  font-size: 9px;
  text-transform: uppercase;
  letter-spacing: .4px;
}
.json-mini-table td:last-child { color: var(--text-hi); }
.json-code {
  margin: 0;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 3px;
  background: #0b111a;
  color: var(--text);
  font-family: var(--mono);
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-word;
}
.turn-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.turn-item {
  border: 1px solid var(--border);
  border-radius: 3px;
  background: var(--surface);
  padding: 8px 10px;
}
.turn-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.turn-index {
  color: var(--text-dim);
  font-size: 10px;
}
.turn-method {
  color: var(--text-dim);
  font-size: 10px;
}
.turn-body {
  color: var(--text);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.empty-state { color: var(--text-dim); font-size: 11px; padding: 8px 0; text-align: center; }

@keyframes pulse { 0%,100%{ opacity:.4 } 50%{ opacity:1 } }
.loading { animation: pulse 1.5s ease-in-out infinite; }

/* Mobile portrait (<= 640px) */
@media (max-width: 640px) {
  html, body { overflow-x: hidden; overflow-y: auto; height: auto; }
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
  .db-view { min-height: 300px; }
  .topbar-label { display: none; }
  .topbar { height: auto; min-height: 40px; padding: 6px 10px; flex-wrap: wrap; row-gap: 6px; }
  .topbar-brand { min-width: 0; white-space: normal; overflow-wrap: anywhere; }
  .topbar-hub { margin-left: 0; flex: 1 1 100%; min-width: 0; }
  .topbar-hub-select { width: 100%; min-width: 0; }
  .topbar-sep { display: none; }
  .topbar-status { margin-left: auto; }
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
    <div class="topbar-hub">
      <span class="topbar-hub-label">Hub</span>
      <select id="hub-select" class="topbar-hub-select" onchange="onHubChanged()"></select>
    </div>
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
          <button class="abtn abtn-accent" onclick="doAction('update-publish')">Update+Publish</button>
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
            <button id="vtab-db" class="vtab"        onclick="switchTab('db')">Database</button>
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
          <div id="panel-db" class="log-panel">
            <div class="db-toolbar">
              <button class="tbtn" onclick="clearSessionHistory()">Clear Session History</button>
            </div>
            <div id="db-view" class="db-view"><span class="loading">Loading&#x2026;</span></div>
          </div>
        </div>

      </div>

    </div>

  </div>
</div>

<div id="json-modal" class="json-modal hidden" role="dialog" aria-modal="true" aria-labelledby="json-modal-title">
  <div class="json-modal-backdrop" onclick="closeJSONModal()"></div>
  <div class="json-modal-panel">
    <div class="json-modal-header">
      <div id="json-modal-title" class="json-modal-title">Session Agent Details</div>
      <div class="json-modal-actions">
        <button id="json-modal-tab-parsed" type="button" class="json-modal-tab active" onclick="switchJSONModalView('parsed')">Parsed</button>
        <button id="json-modal-tab-raw" type="button" class="json-modal-tab" onclick="switchJSONModalView('raw')">Raw JSON</button>
      </div>
      <button type="button" class="json-modal-close" onclick="closeJSONModal()">Close</button>
    </div>
    <div id="json-modal-body" class="json-modal-body"></div>
  </div>
</div>

<script>
const $ = id => document.getElementById(id);
const appBasePath = (() => {
  const p = window.location.pathname || '/';
  return p.startsWith('/monitor') ? '/monitor/' : '/';
})();
const jsonCellStore = {};
let jsonCellSeq = 0;
const jsonModalState = { raw: '', column: '', table: '', mode: 'parsed' };
let selectedHubId = "";
let appConfig = null;

function appURL(rel) {
  const clean = String(rel || '').replace(/^\/+/, '');
  return appBasePath + clean;
}

function initPWA() {
  const manifest = $('wm-manifest');
  if (manifest) manifest.setAttribute('href', appURL('manifest.webmanifest'));
  const icon = $('wm-icon');
  if (icon) icon.setAttribute('href', appURL('icons/icon.svg'));
  const apple = $('wm-apple-icon');
  if (apple) apple.setAttribute('href', appURL('icons/icon.svg'));

  if ('serviceWorker' in navigator) {
    window.addEventListener('load', () => {
      navigator.serviceWorker.register(appURL('service-worker.js'), { scope: appBasePath }).catch(() => {});
    });
  }
}

function switchTab(tab) {
  const isLogs = tab === 'logs';
  $('vtab-logs').classList.toggle('active', isLogs);
  $('vtab-db').classList.toggle('active', !isLogs);
  $('panel-logs').classList.toggle('active', isLogs);
  $('panel-db').classList.toggle('active', !isLogs);
}

async function api(path) {
  let fullPath = String(path || '');
  if (selectedHubId && fullPath !== 'hubs' && fullPath.indexOf('hubId=') < 0) {
    fullPath += (fullPath.includes('?') ? '&' : '?') + 'hubId=' + encodeURIComponent(selectedHubId);
  }
  const res = await fetch(window.location.origin + appURL('api/' + fullPath));
  return res.json();
}

function hubPath(path) {
  let fullPath = String(path || '');
  if (selectedHubId && fullPath.indexOf('hubId=') < 0) {
    fullPath += (fullPath.includes('?') ? '&' : '?') + 'hubId=' + encodeURIComponent(selectedHubId);
  }
  return fullPath;
}

async function apiHub(path) {
  return api(hubPath(path));
}


async function loadHubOptions() {
  const sel = $('hub-select');
  if (!sel) return;
  let hubs = [];
  try {
    const data = await api('hubs');
    hubs = Array.isArray(data.hubs) ? data.hubs : [];
  } catch (_) {}
  if (hubs.length === 0) {
    hubs = [{ hubId: 'local', online: false }];
  }
  if (!selectedHubId || !hubs.some(h => h.hubId === selectedHubId)) {
    selectedHubId = String(hubs[0].hubId || '');
  }
  sel.innerHTML = hubs.map(h => '<option value="' + esc(h.hubId) + '">' + esc(h.hubId) + (h.online ? '' : ' (offline)') + '</option>').join('');
  sel.value = selectedHubId;
}

function onHubChanged() {
  const sel = $('hub-select');
  selectedHubId = sel ? String(sel.value || '') : '';
  refresh();
}
function esc(s) {
  if (s == null) return '';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function stashJSONCellValue(raw) {
  jsonCellSeq += 1;
  const key = 'j' + String(jsonCellSeq);
  jsonCellStore[key] = raw == null ? '' : String(raw);
  return key;
}

function openJSONModal(key, columnName, tableName) {
  const body = $('json-modal-body');
  const modal = $('json-modal');
  const title = $('json-modal-title');
  if (!body || !modal) return;
  const raw = jsonCellStore[key];
  if (raw == null) return;
  const col = String(columnName || '').trim();
  const table = String(tableName || '').trim();
  jsonModalState.raw = String(raw);
  jsonModalState.column = col;
  jsonModalState.table = table;
  jsonModalState.mode = 'parsed';
  if (title) {
    title.textContent = table && col ? (table + '.' + col + ' JSON') : (col ? (col + ' JSON') : 'JSON Details');
  }
  switchJSONModalView('parsed');
  modal.classList.remove('hidden');
  document.body.style.overflow = 'hidden';
}

function switchJSONModalView(mode) {
  const body = $('json-modal-body');
  const parsedBtn = $('json-modal-tab-parsed');
  const rawBtn = $('json-modal-tab-raw');
  if (!body) return;
  const next = mode === 'raw' ? 'raw' : 'parsed';
  jsonModalState.mode = next;
  if (parsedBtn) parsedBtn.classList.toggle('active', next === 'parsed');
  if (rawBtn) rawBtn.classList.toggle('active', next === 'raw');
  if (next === 'raw') {
    body.innerHTML = renderGenericJSONContent(jsonModalState.raw);
    return;
  }
  body.innerHTML = renderParsedJSONContent(jsonModalState.raw, jsonModalState.column, jsonModalState.table);
}

function renderParsedJSONContent(raw, columnName, tableName) {
  const col = String(columnName || '').toLowerCase();
  const table = String(tableName || '').toLowerCase();
  if (col === 'agent_json') {
    return renderAgentJSONContent(raw);
  }
  if (table === 'session_prompts' && col === 'turns_json') {
    return renderSessionPromptsTurnsContent(raw);
  }
  return renderGenericJSONContent(raw);
}

function closeJSONModal() {
  const modal = $('json-modal');
  const body = $('json-modal-body');
  if (!modal || !body) return;
  modal.classList.add('hidden');
  body.innerHTML = '';
  jsonModalState.raw = '';
  jsonModalState.column = '';
  jsonModalState.table = '';
  jsonModalState.mode = 'parsed';
  document.body.style.overflow = '';
}

// Resolve a config option's current value to a human-readable string.
// Prefers a matching entry in opt.options; falls back to URI fragment / last
// path segment so URLs like https://example.com/modes#agent become "agent".
function resolveOptValue(opt) {
  if (!opt || opt.currentValue == null) return '-';
  const v = String(opt.currentValue);
  if (!v) return '-';
  const options = Array.isArray(opt.options) ? opt.options : [];
  for (const o of options) {
    if (o && o.value === v && o.name) return o.name;
  }
  if (v.includes('#')) {
    const frag = v.slice(v.lastIndexOf('#') + 1);
    if (frag) return frag;
  }
  if (v.includes('/')) {
    const seg = v.slice(v.lastIndexOf('/') + 1);
    if (seg) return seg;
  }
  return v;
}

function renderGenericJSONContent(raw) {
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (_) {
    return '<pre class="json-code">' + esc(raw) + '</pre>';
  }
  return '<pre class="json-code">' + esc(JSON.stringify(parsed, null, 2)) + '</pre>';
}

function normalizeConfigOptionName(opt) {
  const rawName = opt && opt.name ? String(opt.name).trim() : '';
  if (rawName) return rawName;
  if (opt && opt.id) return String(opt.id);
  return '-';
}

function renderSessionPromptsTurnsContent(raw) {
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (_) {
    return '<pre class="json-code">' + esc(raw) + '</pre>';
  }
  if (!Array.isArray(parsed)) {
    return '<pre class="json-code">' + esc(JSON.stringify(parsed, null, 2)) + '</pre>';
  }
  if (parsed.length === 0) {
    return '<div class="empty-state">No turns</div>';
  }
  let html = '<div class="turn-list">';
  for (let i = 0; i < parsed.length; i++) {
    const turn = parsePromptTurnEntry(parsed[i]);
    html += '<div class="turn-item">';
    html += '<div class="turn-head">' +
      '<span class="turn-index">#' + String(i + 1) + '</span>' +
      '<span class="badge badge-blue">' + esc(turn.role) + '</span>' +
      '<span class="turn-method">' + esc(turn.method) + '</span>' +
      '</div>';
    html += '<div class="turn-body">' + esc(turn.body) + '</div>';
    html += '</div>';
  }
  html += '</div>';
  return html;
}

function parsePromptTurnEntry(rawEntry) {
  const fallback = { role: 'system', method: 'unknown', body: '' };
  const text = typeof rawEntry === 'string' ? rawEntry : JSON.stringify(rawEntry || {});
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (_) {
    fallback.body = text;
    return fallback;
  }
  if (!parsed || typeof parsed !== 'object') {
    fallback.body = JSON.stringify(parsed);
    return fallback;
  }
  const method = parsed.method && String(parsed.method).trim() ? String(parsed.method).trim() : 'unknown';
  const param = parsed.param && typeof parsed.param === 'object' ? parsed.param : {};
  let role = 'assistant';
  let body = '';
  if (method === 'im.prompt.request') {
    role = 'user';
    const blocks = Array.isArray(param.contentBlocks) ? param.contentBlocks : [];
    const parts = [];
    for (const block of blocks) {
      if (block && String(block.type || '').trim() === 'text' && String(block.text || '').trim() !== '') {
        parts.push(String(block.text).trim());
      }
    }
    body = parts.join('\n');
  } else if (method === 'im.agent.message' || method === 'im.agent.thought') {
    role = 'assistant';
    body = String(param.text || '').trim();
  } else if (method === 'im.system') {
    role = 'system';
    body = String(param.text || '').trim();
  } else if (method === 'session.update') {
    const update = param.update && typeof param.update === 'object' ? param.update : {};
    const sessionUpdate = String(update.sessionUpdate || '').trim();
    const content = update.content;
    role = sessionUpdate === 'user_message_chunk' ? 'user' : 'assistant';
    if (typeof content === 'string') {
      body = content.trim();
    } else if (content && typeof content === 'object' && typeof content.text === 'string') {
      body = content.text.trim();
    }
    if (!body) {
      body = sessionUpdate || JSON.stringify(update);
    }
  } else if (method === 'im.prompt.done') {
    role = 'system';
    body = String(param.stopReason || '').trim();
  }
  if (!body) {
    body = JSON.stringify(param && Object.keys(param).length > 0 ? param : parsed, null, 2);
  }
  return { role, method, body };
}

function summarizeAgentJSON(raw) {
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return raw.trim() ? '?' : '-';
    }
    const looksLikeSingleAgentState = Array.isArray(parsed.configOptions) || Array.isArray(parsed.commands) || !!parsed.agentInfo;
    if (looksLikeSingleAgentState) {
      const agentInfo = parsed.agentInfo || {};
      return agentInfo.name || agentInfo.title || 'session agent';
    }
    const names = Object.keys(parsed || {});
    return names.length ? names.join(', ') : '-';
  } catch (_) {
    return raw.trim() ? '?' : '-';
  }
}

function renderAgentJSONContent(raw) {
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (_) {
    return '<pre class="json-code">' + esc(raw) + '</pre>';
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return '<pre class="json-code">' + esc(JSON.stringify(parsed, null, 2)) + '</pre>';
  }
  const looksLikeSingleAgentState = Array.isArray(parsed.configOptions) || Array.isArray(parsed.commands) || !!parsed.agentInfo;
  const agentEntries = looksLikeSingleAgentState
    ? [{ name: parsed.agentInfo && (parsed.agentInfo.name || parsed.agentInfo.title) ? (parsed.agentInfo.name || parsed.agentInfo.title) : 'session agent', info: parsed }]
    : Object.keys(parsed).map(name => ({ name, info: parsed[name] || {} }));
  if (agentEntries.length === 0) {
    return '<div class="empty-state">No agent data</div>';
  }
  let html = '';
  for (const entry of agentEntries) {
    const agent = entry.name;
    const info = entry.info || {};
    const agentInfo = info.agentInfo || {};
    const configOptions = Array.isArray(info.configOptions) ? info.configOptions : [];
    const commands = Array.isArray(info.commands) ? info.commands : [];
    const authMethods = Array.isArray(info.authMethods) ? info.authMethods : [];
    html += '<div class="json-card">';
    html += '<div class="json-card-hd"><div class="json-card-title">' + esc(agent) + '</div>' +
      '<span class="json-pill">' + esc(agentInfo.version || 'unknown') + '</span></div>';
    html += '<div class="json-grid">' +
      '<div class="json-k">Protocol</div><div class="json-v">' + esc(info.protocolVersion || '-') + '</div>' +
      '<div class="json-k">Agent Info</div><div class="json-v">' + esc((agentInfo.title || agentInfo.name || '-') + (agentInfo.name && agentInfo.title ? ' (' + agentInfo.name + ')' : '')) + '</div>' +
      '<div class="json-k">Config Options</div><div class="json-v">' + String(configOptions.length) + '</div>' +
      '<div class="json-k">Commands</div><div class="json-v">' + String(commands.length) + '</div>' +
      '</div>';
    if (configOptions.length > 0) {
      html += '<div class="json-subsection"><div class="json-subtitle">Config Options</div><div class="json-table-wrap"><table class="json-mini-table"><thead><tr><th>Name</th><th>Current</th></tr></thead><tbody>';
      for (const opt of configOptions) {
        html += '<tr><td>' + esc(normalizeConfigOptionName(opt)) + '</td><td>' + esc(resolveOptValue(opt)) + '</td></tr>';
      }
      html += '</tbody></table></div></div>';
    }
    if (commands.length > 0) {
      html += '<div class="json-subsection"><div class="json-subtitle">Commands</div><div class="json-list">';
      for (const cmd of commands) {
        html += '<span class="json-pill">' + esc(cmd && cmd.name ? cmd.name : '-') + '</span>';
      }
      html += '</div></div>';
    }
    if (authMethods.length > 0) {
      html += '<div class="json-subsection"><div class="json-subtitle">Auth Methods</div><div class="json-list">';
      for (const m of authMethods) {
        html += '<span class="json-pill">' + esc(m && (m.name || m.id) ? (m.name || m.id) : '-') + '</span>';
      }
      html += '</div></div>';
    }
    if (info.agentCapabilities) {
      html += '<div class="json-subsection"><div class="json-subtitle">Capabilities</div><pre class="json-code">' + esc(JSON.stringify(info.agentCapabilities, null, 2)) + '</pre></div>';
    }
    html += '</div>';
  }
  return html;
}

async function refresh() {
  await Promise.all([refreshStatusOnly(), loadLogs(), loadDBTables(), loadProjectsByHub(), loadRegistryStatus()]);
}

async function refreshStatusOnly() {
  try {
    const svc = await apiHub('status');
    renderStatus(svc);
  } catch(_) {}
}

async function loadDBTables() {
  try {
    const db = await apiHub('db');
    renderDBTables(db);
  } catch (_) {
    renderDBTables({ error: 'Failed to load DB' });
  }
}

async function loadHubList() {
  const sel = $('hub-select');
  if (!sel) return;
  let hubs = [];
  try {
    const data = await api('hubs');
    hubs = Array.isArray(data.hubs) ? data.hubs : [];
  } catch (_) {}
  if (hubs.length === 0) {
    hubs = [{ hubId: 'local', online: false }];
  }
  const wanted = selectedHubId && hubs.some(h => h.hubId === selectedHubId) ? selectedHubId : String(hubs[0].hubId || '');
  selectedHubId = wanted;
  sel.innerHTML = hubs.map(h => '<option value="' + esc(h.hubId) + '">' + esc(h.hubId) + (h.online ? '' : ' (offline)') + '</option>').join('');
  sel.value = selectedHubId;
  $('hub-id').innerHTML = 'ID: <span>' + esc(selectedHubId || '—') + '</span>';
}

function onHubChanged() {
  const sel = $('hub-select');
  selectedHubId = sel ? String(sel.value || '') : '';
  $('hub-id').innerHTML = 'ID: <span>' + esc(selectedHubId || '—') + '</span>';
  refresh();
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
      const startedAt = String(p.startedAt || '').trim() || '--';
      return '<div class="proc-chip">' +
             '<span class="badge ' + cls + '">' + esc(roleLabel) + '#' + esc(String(p.pid)) + '</span>' +
             '<span class="ptime">Started ' + esc(startedAt) + '</span></div>';
    }).join('');
  }
}

function renderSidebar(cfg, projects) {
  if (!cfg) return;
  const r = cfg.registry || {};

  $('hub-id').innerHTML = 'ID: <span>' + esc(selectedHubId || r.hubId || '—') + '</span>';

  const list = Array.isArray(projects) ? projects : [];
  if (list.length === 0) {
    $('proj-list').innerHTML = '<div class="empty-state">No projects</div>';
  } else {
    $('proj-list').innerHTML = list.map(p => {
      return '<div class="proj-item">' +
        '<div class="proj-name">' + esc(p.name || p.projectId || '-') + '</div>' +
        '<div class="proj-path">' + esc(p.path || '') + '</div>' +
        '<div class="proj-badges">' +
          '<span class="badge badge-blue">' + esc(p.agent || 'none') + '</span>' +
          '<span class="badge badge-yellow">' + esc(p.imType || 'none') + '</span>' +
          '<span class="badge ' + (p.online ? 'badge-green' : 'badge-red') + '">' + (p.online ? 'online' : 'offline') + '</span>' +
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

async function loadProjectsByHub() {
  try {
    const data = await apiHub('projects');
    const projects = Array.isArray(data.projects) ? data.projects : [];
    renderSidebar(appConfig || {}, projects);
  } catch (_) {
    renderSidebar(appConfig || {}, []);
  }
}

function cfgRow(label, value) {
  return '<div class="reg-row"><span class="rl">' + esc(label) + '</span><span class="rv">' + esc(value) + '</span></div>';
}

function renderDBTables(db) {
  const el = $('db-view');
  if (!db || db.error) {
    el.innerHTML = '<div class="empty-state">' + esc(db ? db.error : 'No data') + '</div>';
    return;
  }
  const tables = db.tables || [];
  if (tables.length === 0) {
    el.innerHTML = '<div class="empty-state">No tables</div>';
    return;
  }
  let html = '';
  for (const t of tables) {
    html += '<div class="db-section">';
    html += '<div class="db-table-title">' + esc(t.name) + '<span class="db-count">' + t.rows.length + ' rows</span></div>';
    if (t.rows.length === 0) {
      html += '<div class="empty-state">Empty</div>';
    } else {
      html += '<div class="db-table-wrap"><table class="reg-table"><thead><tr>';
      for (const col of t.columns) {
        html += '<th>' + esc(col) + '</th>';
      }
      html += '</tr></thead><tbody>';
      for (const row of t.rows) {
        html += '<tr>';
        for (let i = 0; i < row.length; i++) {
          const val = row[i];
          const col = i < t.columns.length ? String(t.columns[i] || '') : '';
          const tableName = String(t.name || '').toLowerCase();
          const isSessionsTable = tableName === 'sessions';
          const isSessionTurnsTable = tableName === 'session_turns';
          const colName = col.toLowerCase();
          const isAgentJSON = isSessionsTable && colName === 'agent_json';
          const isTurnUpdateJSON = isSessionTurnsTable && colName === 'update_json';
          const isJSONColumn = colName.endsWith('_json');
          const isStatus = isSessionsTable && colName === 'status';
          if (isJSONColumn) {
            const raw = val == null ? '' : String(val);
            const key = stashJSONCellValue(raw);
            let summary = '-';
            if (isAgentJSON) {
              summary = summarizeAgentJSON(raw);
            } else if (isTurnUpdateJSON && raw.trim()) {
              try {
                const parsed = JSON.parse(raw);
                const method = parsed && typeof parsed.method === 'string' ? parsed.method.trim() : '';
                const payload = parsed && typeof parsed.payload === 'object' ? parsed.payload : null;
                const params = parsed && typeof parsed.params === 'object' ? parsed.params : null;
                const update = params && typeof params.update === 'object' ? params.update : null;
                const updateMethod = update && typeof update.sessionUpdate === 'string' ? update.sessionUpdate.trim() : '';
                if (method === 'session.update') {
                  summary = updateMethod || method || '-';
                } else {
                  summary = method || '-';
                }
              } catch (_) {
                summary = 'invalid json';
              }
            } else if (raw.trim()) {
              try {
                const parsed = JSON.parse(raw);
                if (Array.isArray(parsed)) {
                  summary = 'array(' + String(parsed.length) + ')';
                } else if (parsed && typeof parsed === 'object') {
                  const keys = Object.keys(parsed);
                  summary = keys.length ? (keys.slice(0, 3).join(', ') + (keys.length > 3 ? ', ...' : '')) : '{}';
                } else {
                  summary = String(parsed);
                }
              } catch (_) {
                summary = 'invalid json';
              }
            }
            const safeCol = col.replace(/\\/g, '\\\\').replace(/'/g, "\\'");
            const safeTable = tableName.replace(/\\/g, '\\\\').replace(/'/g, "\\'");
            html += '<td><span class="tbl-muted">' + esc(summary) + '</span> <button type="button" class="json-cell-btn" onclick="openJSONModal(\'' + key + '\', \'' + safeCol + '\', \'' + safeTable + '\')">View JSON</button></td>';
            continue;
          }
          if (isStatus) {
            const statusMap = { '0': 'active', '1': 'suspended', '2': 'persisted' };
            const label = statusMap[String(val)] || String(val);
            const cls = val === 0 || val === '0' ? 'badge-green' : 'badge-red';
            html += '<td><span class="badge ' + cls + '">' + esc(label) + '</span></td>';
            continue;
          }
          const s = val == null ? '' : String(val);
          const truncated = s.length > 80 ? s.substring(0, 77) + '...' : s;
          html += '<td title="' + esc(s) + '">' + esc(truncated) + '</td>';
        }
        html += '</tr>';
      }
      html += '</tbody></table></div>';
    }
    html += '</div>';
  }
  el.innerHTML = html;
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
    const data = await apiHub('logs?file=' + file + '&level=' + level + '&tail=' + tail);
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


async function apiErrorFromResponse(res) {
  const raw = await res.text();
  const text = String(raw || '').trim();
  if (!text) {
    return { error: 'empty response', hint: '', code: '' };
  }
  try {
    const parsed = JSON.parse(text);
    if (parsed && typeof parsed === 'object') {
      return parsed;
    }
  } catch (_) {}
  return { error: text, hint: '', code: '' };
}

async function doAction(action) {
  const msg = $('action-msg');
  msg.textContent = action + '\u2026';
  msg.style.color = 'var(--text-dim)';
  try {
    const path = action === 'restart-monitor' ? 'api/action/' + action : hubPath('api/action/' + action);
    const res  = await fetch(window.location.origin + appURL(path), { method: 'POST' });
    const data = await apiErrorFromResponse(res);
    if (data.error) {
      msg.textContent = 'Error: ' + data.error + (data.hint ? ' | Hint: ' + data.hint : '');
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

async function clearSessionHistory() {
  if (!confirm('Clear persisted session turn files and reset session sync cursors?')) {
    return;
  }
  const msg = $('action-msg');
  msg.textContent = 'clear-session-history\u2026';
  msg.style.color = 'var(--text-dim)';
  try {
    const res = await fetch(window.location.origin + appURL(hubPath('api/action/clear-session-history')), { method: 'POST' });
    const data = await apiErrorFromResponse(res);
    if (data.error) {
      msg.textContent = 'Error: ' + data.error + (data.hint ? ' | Hint: ' + data.hint : '');
      msg.style.color = 'var(--red)';
      return;
    }
    msg.textContent = 'session history cleared';
    msg.style.color = 'var(--green)';
    await loadDBTables();
  } catch(e) {
    msg.textContent = 'Request failed: ' + e.message;
    msg.style.color = 'var(--red)';
  }
}

initPWA();

(async function init() {
  try {
    appConfig = await api('config');
  } catch (_) {
    appConfig = {};
  }
  await loadHubList();
  await refresh();
})();

setInterval(() => { if (!document.hidden) refreshStatusOnly(); }, 5000);
setInterval(() => { if (!document.hidden) { loadLogs(); loadDBTables(); } }, 15000);
setInterval(loadRegistryStatus, 15000);
window.addEventListener('visibilitychange', () => { if (!document.hidden) refreshStatusOnly(); });
window.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeJSONModal();
});
</script>
</body>
</html>
`
