package admin

import (
	"net/http"

)

// RegisterHeartbeatRoutes mounts the heartbeat statistics endpoint.
func (a *API) RegisterHeartbeatRoutes(mux *http.ServeMux) {
	if a.heartbeats == nil {
		return
	}
	mux.HandleFunc("/api/admin/heartbeats", a.handleHeartbeats)
}

func (a *API) handleHeartbeats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	_, ok := a.authorize(w, r)
	if !ok {
		return
	}
	rows, err := a.heartbeats.List(r.Context(), r.URL.Query().Get("deviceId"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}
