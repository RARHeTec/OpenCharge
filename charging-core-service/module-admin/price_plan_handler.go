package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

// RegisterPricePlanRoutes mounts the adjustable pricing endpoints.
func (a *API) RegisterPricePlanRoutes(mux *http.ServeMux) {
	if a.pricePlans == nil {
		return
	}
	mux.HandleFunc("/api/admin/price-plans", a.handlePricePlans)
	mux.HandleFunc("/api/admin/price-plans/", a.handlePricePlanDetail)
	mux.HandleFunc("/api/admin/device-types", a.handleDeviceTypes)
}

type segmentDTO struct {
	Seq          int    `json:"seq"`
	StartMin     int    `json:"startMin"`
	EndMin       int    `json:"endMin"`
	Period       string `json:"period"`
	ElecPrice    string `json:"elecPrice"`
	ServicePrice string `json:"servicePrice"`
}

type createPricePlanRequest struct {
	TenantID     string        `json:"tenantId"`
	StationID    string        `json:"stationId"`
	DeviceTypes  []string      `json:"deviceTypes"`
	EffectiveNow bool          `json:"effectiveNow"`
	EffectiveAt  string        `json:"effectiveAt"`
	Source       string        `json:"source"`
	Note         string        `json:"note"`
	Operator     string        `json:"operator"`
	Segments     []segmentDTO  `json:"segments"`
}

// deviceTypes derives the list shown as selectable cards on the admin UI.
// Registered devices win; seeded fallbacks keep the demo usable before any
// devices are registered.
func (a *API) handleDeviceTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	tenant, ok := a.authorize(w, r)
	if !ok {
		return
	}
	seen := make(map[string]int)
	if a.registry != nil {
		devices, err := a.registry.ListDevices(r.Context(), tenant, "")
		if err == nil {
			for _, d := range devices {
				t := d.DeviceType
				if t == "" {
					t = d.Protocol
				}
				if t != "" {
					seen[t]++
				}
			}
		}
	}
	if len(seen) == 0 {
		seen["直流160kW-浙江艾充ZJAC-DC-160KW"] = 1
		seen["直流240kW-浙江艾充ZJAC-DC-240KW"] = 1
	}
	type item struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	out := make([]item, 0, len(seen))
	for name, count := range seen {
		out = append(out, item{Name: name, Count: count})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (a *API) handlePricePlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenant, ok := a.authorize(w, r)
		if !ok {
			return
		}
		if tenant == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenantId required"})
			return
		}
		history := r.URL.Query().Get("view") == "history"
		plans, err := a.pricePlans.List(r.Context(), tenant,
			r.URL.Query().Get("stationId"), r.URL.Query().Get("deviceType"), history)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": plans})

	case http.MethodPost:
		var req createPricePlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}
		tenant, ok := a.resolveTenant(r, req.TenantID)
		if !ok || tenant == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "tenant mismatch or missing"})
			return
		}
		if req.StationID == "" || len(req.DeviceTypes) == 0 || len(req.Segments) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "stationId, deviceTypes and segments required"})
			return
		}
		if req.Operator == "" {
			req.Operator = "admin"
		}
		effAt := time.Now()
		if !req.EffectiveNow && req.EffectiveAt != "" {
			parsed, err := time.Parse(time.RFC3339, req.EffectiveAt)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid effectiveAt: " + err.Error()})
				return
			}
			effAt = parsed
		}
		segments := make([]storage.PriceSegment, 0, len(req.Segments))
		for _, d := range req.Segments {
			elec, err := money.ParsePrice(d.ElecPrice)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid elecPrice: " + err.Error()})
				return
			}
			svc, err := money.ParsePrice(d.ServicePrice)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid servicePrice: " + err.Error()})
				return
			}
			segments = append(segments, storage.PriceSegment{
				Seq: d.Seq, StartMin: d.StartMin, EndMin: d.EndMin,
				Period: storage.Period(d.Period), ElecPrice: elec, ServicePrice: svc,
			})
		}

		created := make([]storage.PricePlan, 0, len(req.DeviceTypes))
		for _, dt := range req.DeviceTypes {
			// Publish atomically per device type: invalidate old, save new.
			if err := a.pricePlans.InvalidateCurrent(r.Context(), tenant, req.StationID, dt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			plan := storage.PricePlan{
				ID: fmt.Sprintf("pp-%d", time.Now().UnixNano()), TenantID: tenant,
				StationID: req.StationID, DeviceType: dt, EffectiveAt: effAt,
				EffectiveNow: req.EffectiveNow, Status: "valid", Source: req.Source,
				Operator: req.Operator, Note: req.Note, Segments: segments,
				CreatedAt: time.Now(),
			}
			if plan.Source == "" {
				plan.Source = "manual"
			}
			if err := storage.ValidatePlan(plan); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if err := a.pricePlans.Save(r.Context(), plan); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			created = append(created, plan)
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": created})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) handlePricePlanDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, ok := a.authorize(w, r); !ok {
		return
	}
	id := r.URL.Path[len("/api/admin/price-plans/"):]
	plan, ok, err := a.pricePlans.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "price plan not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": plan})
}
