package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func registerRoutes(mux *http.ServeMux, mon *Monitor) {
	registerRoutesAtPrefix(mux, mon, "")
	registerRoutesAtPrefix(mux, mon, "/monitor")
}

func registerRoutesAtPrefix(mux *http.ServeMux, mon *Monitor, prefix string) {
	// API endpoints
	mux.HandleFunc("GET "+prefix+"/api/overview", handleOverview(mon))
	mux.HandleFunc("GET "+prefix+"/api/status", handleStatus(mon))
	mux.HandleFunc("GET "+prefix+"/api/config", handleConfig(mon))
	mux.HandleFunc("GET "+prefix+"/api/db", handleDBTables(mon))
	mux.HandleFunc("GET "+prefix+"/api/sessions", handleSessions(mon))
	mux.HandleFunc("GET "+prefix+"/api/sessions/{sessionID}/messages", handleSessionMessages(mon))
	mux.HandleFunc("GET "+prefix+"/api/logs", handleLogs(mon))
	mux.HandleFunc("GET "+prefix+"/api/registry", handleRegistry(mon))
	mux.HandleFunc("POST "+prefix+"/api/action/restart", handleAction(mon, "restart"))
	mux.HandleFunc("POST "+prefix+"/api/action/restart-monitor", handleAction(mon, "restart-monitor"))
	mux.HandleFunc("POST "+prefix+"/api/action/stop", handleAction(mon, "stop"))
	mux.HandleFunc("POST "+prefix+"/api/action/start", handleAction(mon, "start"))
	mux.HandleFunc("POST "+prefix+"/api/action/update-publish", handleAction(mon, "update-publish"))

	// PWA resources
	mux.HandleFunc("GET "+prefix+"/manifest.webmanifest", handleManifest())
	mux.HandleFunc("GET "+prefix+"/service-worker.js", handleServiceWorker())
	mux.HandleFunc("GET "+prefix+"/icons/icon.svg", handleIcon())

	// Web dashboard
	root := prefix + "/"
	if prefix == "" {
		root = "/"
	}
	mux.HandleFunc("GET "+root, handleDashboard())
}

func handleOverview(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := mon.GetOverview()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, data)
	}
}

func handleStatus(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := mon.GetServiceStatus()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, data)
	}
}

func handleConfig(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := mon.GetConfig()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, data)
	}
}

func handleDBTables(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := mon.GetDBTables()
		writeJSON(w, data)
	}
}

func handleSessions(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := r.URL.Query().Get("project")
		limit := 200
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		sessions, err := mon.ListSessions(projectName, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"sessions": sessions})
	}
}

func handleSessionMessages(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionID")
		projectName := r.URL.Query().Get("project")
		limit := 200
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		var afterIndex int64
		if raw := r.URL.Query().Get("afterIndex"); raw != "" {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
				afterIndex = n
			}
		}
		var afterSubIndex int64
		if raw := r.URL.Query().Get("afterSubIndex"); raw != "" {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
				afterSubIndex = n
			}
		}
		messages, err := mon.GetSessionMessages(sessionID, projectName, afterIndex, afterSubIndex, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"messages": messages})
	}
}

func handleLogs(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file := r.URL.Query().Get("file")
		level := r.URL.Query().Get("level")
		tailStr := r.URL.Query().Get("tail")
		tail := 200
		if tailStr != "" {
			if n, err := strconv.Atoi(tailStr); err == nil && n > 0 {
				tail = n
			}
		}
		data, err := mon.GetLogs(file, level, tail)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, data)
	}
}

func handleRegistry(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := mon.GetRegistryStatus()
		writeJSON(w, data)
	}
}

func handleAction(mon *Monitor, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		switch action {
		case "restart":
			err = mon.RestartService()
		case "restart-monitor":
			err = mon.RestartMonitor()
		case "stop":
			err = mon.StopService()
		case "start":
			err = mon.StartService()
		case "update-publish":
			err = mon.TriggerUpdatePublish()
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "ok", "action": action})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
