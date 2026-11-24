// Package admin exposes read-only operational APIs for the management UI:
// orders, billing audits, exception orders and basic stats.
package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RARHeTec/OpenCharge/charging-common/trace"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

type API struct {
	store  storage.AuditStore
	orders storage.OrderStore
}

func NewAPI(store storage.AuditStore, orders storage.OrderStore) *API {
	return &API{store: store, orders: orders}
}

// Register mounts admin routes on the given mux under /api/admin.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/orders", a.handleOrders)
	mux.HandleFunc("/api/admin/exceptions", a.handleExceptions)
	mux.HandleFunc("/api/admin/exceptions/resolve", a.handleResolve)
	mux.HandleFunc("/api/admin/audits", a.handleAudits)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func tenantFrom(r *http.Request) string {
	return r.URL.Query().Get("tenantId")
}

func (a *API) handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := 50
	orders, err := a.orders.List(r.Context(), tenantFrom(r), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": orders, "traceId": trace.FromContext(r.Context())})
}

func (a *API) handleExceptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	rows, err := a.store.ListExceptions(r.Context(), tenantFrom(r), 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (a *API) handleResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	operator := strings.TrimSpace(r.FormValue("operator"))
	if id == "" || operator == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and operator required"})
		return
	}
	if err := a.store.ResolveException(r.Context(), id, operator); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (a *API) handleAudits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// MemoryAuditStore exposes Audits(); PG implementation is queried by txn.
	type auditLister interface{ Audits() []storage.AuditRecord }
	rows := []storage.AuditRecord{}
	if l, ok := a.store.(auditLister); ok {
		rows = l.Audits()
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}
