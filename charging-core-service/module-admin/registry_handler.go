package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

// RegisterRegistryRoutes mounts station and device master-data endpoints.
func (a *API) RegisterRegistryRoutes(mux *http.ServeMux) {
	if a.registry == nil {
		return
	}
	mux.HandleFunc("/api/admin/stations", a.handleStations)
	mux.HandleFunc("/api/admin/devices", a.handleDevices)
}

type createStationRequest struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

type createDeviceRequest struct {
	DeviceID      string `json:"deviceId"`
	TenantID      string `json:"tenantId"`
	StationID     string `json:"stationId"`
	Protocol      string `json:"protocol"`
	DefaultUserID string `json:"defaultUserId"`
	GunCount      int    `json:"gunCount"`
}

func (a *API) handleStations(w http.ResponseWriter, r *http.Request) {
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
		rows, err := a.registry.ListStations(r.Context(), tenant)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rows})

	case http.MethodPost:
		var req createStationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}
		tenant, ok := a.resolveTenant(r, req.TenantID)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "tenant mismatch with credential"})
			return
		}
		if req.ID == "" || req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and name required"})
			return
		}
		tz := req.Timezone
		if tz == "" {
			tz = "Asia/Shanghai"
		}
		st := storage.Station{
			ID: req.ID, TenantID: tenant, Name: req.Name,
			Timezone: tz, CreatedAt: time.Now(),
		}
		if err := a.registry.SaveStation(r.Context(), st); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, st)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) handleDevices(w http.ResponseWriter, r *http.Request) {
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
		rows, err := a.registry.ListDevices(r.Context(), tenant, r.URL.Query().Get("stationId"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rows})

	case http.MethodPost:
		var req createDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}
		tenant, ok := a.resolveTenant(r, req.TenantID)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "tenant mismatch with credential"})
			return
		}
		if req.DeviceID == "" || req.StationID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deviceId and stationId required"})
			return
		}
		// A device may only attach to a station owned by the same tenant.
		if _, ok, err := a.registry.GetStation(r.Context(), tenant, req.StationID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		} else if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "station " + req.StationID + " not found for this tenant"})
			return
		}
		proto := req.Protocol
		if proto == "" {
			proto = "yunkuaichong-v1.6"
		}
		if req.GunCount <= 0 {
			req.GunCount = 1
		}
		d := storage.Device{
			DeviceID: req.DeviceID, TenantID: tenant, StationID: req.StationID,
			Protocol: proto, DefaultUserID: req.DefaultUserID, GunCount: req.GunCount,
		}
		if err := a.registry.SaveDevice(r.Context(), d); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, d)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// tenantFromPath is a small helper for future nested routes.
func tenantFromPath(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}
