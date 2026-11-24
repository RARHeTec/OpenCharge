package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

func registryAPI(t *testing.T) (*API, *storage.MemoryRegistryStore) {
	t.Helper()
	reg := storage.NewMemoryRegistryStore()
	api := NewAPI(storage.NewMemoryAuditStore(), storage.NewMemoryOrderStore()).
		WithRegistry(reg).
		WithKeys(NewStaticKeys(map[string]Principal{
			"key-t1": {KeyID: "key-t1", TenantID: "T1"},
			"key-t2": {KeyID: "key-t2", TenantID: "T2"},
		}))
	return api, reg
}

func doJSON(h http.Handler, method, path, key string, body any) *httptest.ResponseRecorder {
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateStationAndDevice(t *testing.T) {
	api, _ := registryAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	rec := doJSON(h, http.MethodPost, "/api/admin/stations", "key-t1",
		map[string]any{"id": "S100", "name": "中关村站", "timezone": "Asia/Shanghai"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("station create = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(h, http.MethodPost, "/api/admin/devices", "key-t1",
		map[string]any{"deviceId": "DEV100", "stationId": "S100", "gunCount": 2, "defaultUserId": "U1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("device create = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(h, http.MethodGet, "/api/admin/devices?stationId=S100", "key-t1", nil)
	var resp struct {
		Data []storage.Device `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].DeviceID != "DEV100" {
		t.Fatalf("devices = %+v", resp.Data)
	}
	if resp.Data[0].TenantID != "T1" {
		t.Errorf("tenant = %s, want T1 from credential", resp.Data[0].TenantID)
	}
}

func TestDeviceCannotAttachToForeignStation(t *testing.T) {
	api, _ := registryAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	// T2 owns the station.
	rec := doJSON(h, http.MethodPost, "/api/admin/stations", "key-t2",
		map[string]any{"id": "S-T2", "name": "T2 站"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("station create = %d", rec.Code)
	}
	// T1 must not be able to attach a device to it.
	rec = doJSON(h, http.MethodPost, "/api/admin/devices", "key-t1",
		map[string]any{"deviceId": "DEV-X", "stationId": "S-T2"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("attach to foreign station = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestStationListIsTenantScoped(t *testing.T) {
	api, _ := registryAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	doJSON(h, http.MethodPost, "/api/admin/stations", "key-t1", map[string]any{"id": "S1", "name": "T1 站"})
	doJSON(h, http.MethodPost, "/api/admin/stations", "key-t2", map[string]any{"id": "S2", "name": "T2 站"})

	rec := doJSON(h, http.MethodGet, "/api/admin/stations", "key-t1", nil)
	var resp struct {
		Data []storage.Station `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].TenantID != "T1" {
		t.Fatalf("T1 sees %+v", resp.Data)
	}
}

func TestStationCreateRequiresAuth(t *testing.T) {
	api, _ := registryAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	rec := doJSON(h, http.MethodPost, "/api/admin/stations", "", map[string]any{"id": "S9", "name": "x"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
