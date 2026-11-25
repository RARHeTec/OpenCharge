// Package admin exposes read-only operational APIs for the management UI:
// orders, billing audits, exception orders and basic stats.
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RARHeTec/OpenCharge/charging-common/heartbeat"
	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-common/state"
	"github.com/RARHeTec/OpenCharge/charging-common/trace"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
	billing "github.com/RARHeTec/OpenCharge/charging-core-service/module-order-billing"
)

type API struct {
	store   storage.AuditStore
	orders  storage.OrderStore
	states  state.Store
	tariffs  storage.TariffStore
	resolver Resolver
	keys     KeyRegistry
	registry  storage.RegistryStore
	heartbeats heartbeat.Store
	pricePlans storage.PricePlanStore
}

func NewAPI(store storage.AuditStore, orders storage.OrderStore, states ...state.Store) *API {
	var st state.Store
	if len(states) > 0 {
		st = states[0]
	}
	return &API{store: store, orders: orders, states: st}
}

// WithTariffs attaches a tariff store so tariff management routes are mounted.
func (a *API) WithTariffs(t storage.TariffStore) *API {
	a.tariffs = t
	return a
}

// Register mounts admin routes on the given mux under /api/admin.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/orders", a.handleOrders)
	mux.HandleFunc("/api/admin/orders/", a.handleOrderDetail)
	mux.HandleFunc("/api/admin/exceptions", a.handleExceptions)
	mux.HandleFunc("/api/admin/exceptions/resolve", a.handleResolve)
	mux.HandleFunc("/api/admin/audits", a.handleAudits)
	mux.HandleFunc("/api/admin/connectors", a.handleConnectors)
	a.RegisterTariffRoutes(mux)
	a.RegisterRegistryRoutes(mux)
	a.RegisterHeartbeatRoutes(mux)
	a.RegisterPricePlanRoutes(mux)
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
	tenant, ok := a.authorize(w, r)
	if !ok {
		return
	}
	limit := 50
	orders, err := a.orders.List(r.Context(), tenant, limit)
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
	tenant, ok := a.authorize(w, r)
	if !ok {
		return
	}
	rows, err := a.store.ListExceptions(r.Context(), tenant, 100)
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
	// Accept both JSON and form posts so the UI and curl both work.
	var req resolveRequest
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}
	} else {
		req.TransactionID = strings.TrimSpace(r.FormValue("transactionId"))
		if req.TransactionID == "" {
			req.TransactionID = strings.TrimSpace(r.FormValue("id"))
		}
		req.Decision = strings.TrimSpace(r.FormValue("decision"))
		req.Amount = strings.TrimSpace(r.FormValue("amount"))
		req.Operator = strings.TrimSpace(r.FormValue("operator"))
		req.Note = strings.TrimSpace(r.FormValue("note"))
	}
	if req.TransactionID == "" || req.Decision == "" || req.Operator == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "transactionId, decision (platform|pile|custom) and operator required"})
		return
	}
	if a.resolver == nil {
		// Fallback for deployments without the settlement engine wired in:
		// only flip the flag, never pretend money moved.
		if err := a.store.ResolveException(r.Context(), req.TransactionID, req.Operator); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "warning": "settlement engine not wired; no charge applied"})
		return
	}

	var custom money.Money
	if req.Decision == "custom" {
		if req.Amount == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "custom decision requires amount"})
			return
		}
		parsed, err := money.ParseYuan(req.Amount)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid amount: " + err.Error()})
			return
		}
		custom = parsed
	}

	res, err := a.resolver.ResolveException(r.Context(), billing.ResolveRequest{
		TransactionID: req.TransactionID,
		Decision:      billing.Resolution(req.Decision),
		CustomAmount:  custom,
		Operator:      req.Operator,
		Note:          req.Note,
	})
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case billing.ErrOrderNotFound:
			status = http.StatusNotFound
		case billing.ErrNotException, billing.ErrBadResolution, billing.ErrCustomRequired:
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orderId":       res.OrderID,
		"decision":      string(res.Decision),
		"chargedAmount": res.ChargedAmount.String(),
		"walletBalance": res.WalletBalance.String(),
	})
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

func (a *API) handleConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if a.states == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	rows, err := a.states.List(r.Context(), r.URL.Query().Get("deviceId"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// resolveRequest is the admin disposition payload.
type resolveRequest struct {
	TransactionID string `json:"transactionId"`
	Decision      string `json:"decision"`
	Amount        string `json:"amount"`
	Operator      string `json:"operator"`
	Note          string `json:"note"`
}

// Resolver is the settlement engine surface needed to close an exception.
type Resolver interface {
	ResolveException(ctx context.Context, req billing.ResolveRequest) (billing.ResolveResult, error)
}

// WithPricePlans attaches the adjustable pricing plan store.
func (a *API) WithPricePlans(p storage.PricePlanStore) *API {
	a.pricePlans = p
	return a
}

// WithHeartbeats attaches cumulative heartbeat counters for the admin view.
func (a *API) WithHeartbeats(h heartbeat.Store) *API {
	a.heartbeats = h
	return a
}

// WithRegistry attaches the station/device master-data store.
func (a *API) WithRegistry(r storage.RegistryStore) *API {
	a.registry = r
	return a
}

// WithKeys enables API key authentication and tenant scoping.
func (a *API) WithKeys(k KeyRegistry) *API {
	a.keys = k
	return a
}

// WithResolver attaches the settlement engine so exception disposition moves money.
func (a *API) WithResolver(r Resolver) *API {
	a.resolver = r
	return a
}

// handleOrderDetail returns one order plus its time-of-use segments.
func (a *API) handleOrderDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	txnID := strings.TrimPrefix(r.URL.Path, "/api/admin/orders/")
	if txnID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "transaction id required"})
		return
	}
	order, ok, err := a.orders.GetByTransaction(r.Context(), txnID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
		return
	}
	segs, err := a.orders.GetSegments(r.Context(), order.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if segs == nil {
		segs = []storage.OrderSegment{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": order, "segments": segs})
}
