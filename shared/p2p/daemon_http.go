package p2p

import (
	"encoding/json"
	"net/http"
)

// DaemonHTTPHandler serves health checks and status telemetry for the BridgeDaemon.
type DaemonHTTPHandler struct {
	daemon *BridgeDaemon
	mux    *http.ServeMux
}

// NewDaemonHTTPHandler constructs a new HTTP handler exposing /healthz and /status.
func NewDaemonHTTPHandler(daemon *BridgeDaemon) *DaemonHTTPHandler {
	h := &DaemonHTTPHandler{
		daemon: daemon,
		mux:    http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

func (h *DaemonHTTPHandler) registerRoutes() {
	h.mux.HandleFunc("/healthz", h.handleHealthz)
	h.mux.HandleFunc("/status", h.handleStatus)
}

func (h *DaemonHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *DaemonHTTPHandler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify both DBs are alive
	if err := h.daemon.ONDCDB().PingContext(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  "ondc db ping failed: " + err.Error(),
		})
		return
	}

	if err := h.daemon.CRMDB().PingContext(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  "crm db ping failed: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (h *DaemonHTTPHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.daemon.Stats()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}
