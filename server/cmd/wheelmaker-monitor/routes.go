package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func registerRoutes(mux *http.ServeMux, mon *Monitor) {
	// API endpoints
	mux.HandleFunc("GET /api/overview", handleOverview(mon))
	mux.HandleFunc("GET /api/status", handleStatus(mon))
	mux.HandleFunc("GET /api/config", handleConfig(mon))
	mux.HandleFunc("GET /api/state", handleState(mon))
	mux.HandleFunc("GET /api/logs", handleLogs(mon))
	mux.HandleFunc("GET /api/registry", handleRegistry(mon))
	mux.HandleFunc("POST /api/action/restart", handleAction(mon, "restart"))
	mux.HandleFunc("POST /api/action/stop", handleAction(mon, "stop"))
	mux.HandleFunc("POST /api/action/start", handleAction(mon, "start"))

	// Web dashboard (embedded HTML)
	mux.HandleFunc("GET /", handleDashboard())
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

func handleState(mon *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := mon.GetState()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, data)
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
		case "stop":
			err = mon.StopService()
		case "start":
			err = mon.StartService()
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
