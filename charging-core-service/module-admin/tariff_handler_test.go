package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

func newTariffAPI(t *testing.T) (*API, *storage.MemoryTariffStore) {
	t.Helper()
	tf := storage.NewMemoryTariffStore()
	api := NewAPI(storage.NewMemoryAuditStore(), storage.NewMemoryOrderStore()).WithTariffs(tf)
	return api, tf
}

func fullSlotPayload(price string) []map[string]any {
	out := make([]map[string]any, 48)
	for i := range out {
		out[i] = map[string]any{"slot": i, "price": price}
	}
	return out
}

func TestTariffCreateAndList(t *testing.T) {
	api, _ := newTariffAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	body, _ := json.Marshal(map[string]any{
		"tenantId": "T1", "stationId": "S1",
		"effective": "2026-11-25T00:00:00+08:00",
		"slots":     fullSlotPayload("0.62345"),
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/tariffs", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created tariffDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !strings.Contains(created.ID, "T1-S1") {
		t.Errorf("id = %s", created.ID)
	}
	if len(created.Slots) != 48 || created.Slots[0].Price != "0.62345" {
		t.Errorf("slots = %d first=%v", len(created.Slots), created.Slots[0])
	}

	// List
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/tariffs?tenantId=T1&stationId=S1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var listResp struct {
		Data []tariffDTO `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("list size = %d", len(listResp.Data))
	}

	// Get by id
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/tariffs/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
}

func TestTariffRejectsBadSlots(t *testing.T) {
	api, _ := newTariffAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	cases := []map[string]any{
		{"tenantId": "T1", "stationId": "S1", "slots": fullSlotPayload("0.6")[:47]},                 // 47 slots
		{"tenantId": "T1", "stationId": "S1", "slots": append(fullSlotPayload("0.6")[:47], map[string]any{"slot": 0, "price": "0.6"})}, // duplicate
		{"tenantId": "", "stationId": "S1", "slots": fullSlotPayload("0.6")},                       // no tenant
		{"tenantId": "T1", "stationId": "S1", "slots": fullSlotPayload("abc")},                     // bad price
	}
	for i, c := range cases {
		body, _ := json.Marshal(c)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/tariffs", bytes.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d, want 400 (body=%s)", i, rec.Code, rec.Body.String())
		}
	}
}

func TestTariffListRequiresTenant(t *testing.T) {
	api, _ := newTariffAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/tariffs", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	_ = context.Background()
}
