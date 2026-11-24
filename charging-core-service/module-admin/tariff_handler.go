package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

// tariffDTO uses string prices so no precision is lost over JSON.
type tariffDTO struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenantId"`
	StationID string          `json:"stationId"`
	Effective string          `json:"effective"` // RFC3339
	Slots     []tariffSlotDTO `json:"slots"`
}

type tariffSlotDTO struct {
	Slot  int    `json:"slot"`
	Price string `json:"price"` // Yuan/kWh with up to 5 decimals
}

type createTariffRequest struct {
	TenantID  string          `json:"tenantId"`
	StationID string          `json:"stationId"`
	Effective string          `json:"effective,omitempty"`
	Slots     []tariffSlotDTO `json:"slots"`
}

func toDTO(v storage.TariffVersion) tariffDTO {
	d := tariffDTO{
		ID: v.ID, TenantID: v.TenantID, StationID: v.StationID,
		Effective: v.Effective.Format(time.RFC3339),
		Slots:     make([]tariffSlotDTO, 0, len(v.Slots)),
	}
	for _, sl := range v.Slots {
		d.Slots = append(d.Slots, tariffSlotDTO{Slot: sl.Slot, Price: sl.Price.String()})
	}
	// Present slots in ascending order for the UI.
	sortSlots(d.Slots)
	return d
}

func sortSlots(s []tariffSlotDTO) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].Slot > s[j].Slot; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// RegisterTariffRoutes mounts tariff management endpoints.
func (a *API) RegisterTariffRoutes(mux *http.ServeMux) {
	if a.tariffs == nil {
		return
	}
	mux.HandleFunc("/api/admin/tariffs", a.handleTariffs)
	mux.HandleFunc("/api/admin/tariffs/", a.handleTariffByID)
}

func (a *API) handleTariffs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenant := r.URL.Query().Get("tenantId")
		station := r.URL.Query().Get("stationId")
		if tenant == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenantId required"})
			return
		}
		versions, err := a.tariffs.List(r.Context(), tenant, station)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]tariffDTO, 0, len(versions))
		for _, v := range versions {
			out = append(out, toDTO(v))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})

	case http.MethodPost:
		var req createTariffRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}
		v, err := buildTariff(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := storage.ValidateTariff(v); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := a.tariffs.Save(r.Context(), v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, toDTO(v))

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) handleTariffByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/tariffs/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	v, ok, err := a.tariffs.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tariff not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDTO(v))
}

func buildTariff(req createTariffRequest) (storage.TariffVersion, error) {
	if req.TenantID == "" || req.StationID == "" {
		return storage.TariffVersion{}, fmt.Errorf("tenantId and stationId required")
	}
	effective := time.Now()
	if req.Effective != "" {
		t, err := time.Parse(time.RFC3339, req.Effective)
		if err != nil {
			return storage.TariffVersion{}, fmt.Errorf("invalid effective time: %w", err)
		}
		effective = t
	}
	// Version ids are derived from station and effective time, so a snapshot is
	// immutable and addressable. Re-posting the same input is idempotent.
	id := fmt.Sprintf("%s-%s-%d", req.TenantID, req.StationID, effective.Unix())
	slots := make([]storage.TariffSlot, 0, len(req.Slots))
	for _, sl := range req.Slots {
		price, err := money.ParsePrice(sl.Price)
		if err != nil {
			return storage.TariffVersion{}, fmt.Errorf("slot %d: %w", sl.Slot, err)
		}
		slots = append(slots, storage.TariffSlot{Slot: sl.Slot, Price: price})
	}
	return storage.TariffVersion{
		ID: id, TenantID: req.TenantID, StationID: req.StationID,
		Effective: effective, Slots: slots,
	}, nil
}
