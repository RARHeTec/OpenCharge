package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/state"
)

// WithStateHub attaches the connector-state broadcaster used by SSE.
func (a *API) WithStateHub(hub *state.Hub) *API {
	a.stateHub = hub
	return a
}

// RegisterSSE mounts the connector event stream.
func (a *API) RegisterSSE(mux *http.ServeMux) {
	if a.stateHub == nil {
		return
	}
	mux.HandleFunc("/api/admin/connector-events", a.handleConnectorEvents)
}

func (a *API) handleConnectorEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	_, ok := a.authorize(w, r)
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := a.stateHub.Subscribe()
	defer a.stateHub.Unsubscribe(ch)

	// Immediate comment line makes browsers fire onopen and confirms the
	// stream is live even before the first connector event arrives.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, alive := <-ch:
			if !alive {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
