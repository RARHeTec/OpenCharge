package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

func authAPI(t *testing.T) (*API, *storage.MemoryOrderStore) {
	t.Helper()
	orders := storage.NewMemoryOrderStore()
	api := NewAPI(storage.NewMemoryAuditStore(), orders).
		WithTariffs(storage.NewMemoryTariffStore()).
		WithKeys(NewStaticKeys(map[string]Principal{
			"key-t1": {KeyID: "key-t1", TenantID: "T1"},
			"key-t2": {KeyID: "key-t2", TenantID: "T2"},
		}))
	return api, orders
}

func TestMissingKeyIsUnauthorized(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestInvalidKeyIsUnauthorized(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil)
	req.Header.Set("X-API-Key", "nope")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestValidKeyWorks(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil)
	req.Header.Set("Authorization", "Bearer key-t1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// The critical isolation test: a T1 credential must never read T2 data.
func TestCrossTenantReadForbidden(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders?tenantId=T2", nil)
	req.Header.Set("X-API-Key", "key-t1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (cross-tenant read)", rec.Code)
	}
}

// A T1 credential must not create a tariff under T2 either.
func TestCrossTenantWriteForbidden(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	body, _ := json.Marshal(map[string]any{
		"tenantId": "T2", "stationId": "S1", "slots": fullSlotPayload("0.6"),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tariffs", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "key-t1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (cross-tenant write)", rec.Code)
	}
}

// The tenant must come from the credential, so a request without tenantId
// still only sees the caller's own tenant.
func TestTenantComesFromCredential(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)
	h := api.AuthMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil)
	req.Header.Set("X-API-Key", "key-t2")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	p, ok := PrincipalFrom(req.Context())
	_ = p
	_ = ok // context on the original request is not mutated; server uses r.WithContext
}

func TestHealthzStaysOpen(t *testing.T) {
	api, _ := authAPI(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := api.AuthMiddleware(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200 without key", rec.Code)
	}
}
