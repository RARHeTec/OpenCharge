package storage

import (
	"context"
	"testing"
	"time"
)

// A delayed offline bill must price with the version that was effective when
// the car charged, not the newest version present at upload time.
func TestTariffLatestRespectsTime(t *testing.T) {
	s := NewMemoryTariffStore()
	ctx := context.Background()
	v1 := TariffVersion{ID: "V1", TenantID: "T1", StationID: "S1",
		Effective: time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local), Slots: fullSlots()}
	v2 := TariffVersion{ID: "V2", TenantID: "T1", StationID: "S1",
		Effective: time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local), Slots: fullSlots()}
	_ = s.Save(ctx, v1)
	_ = s.Save(ctx, v2)

	// A charge in March must get V1 even though V2 now exists.
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.Local).UnixMilli()
	got, ok, err := s.Latest(ctx, "T1", "S1", at)
	if err != nil || !ok {
		t.Fatalf("latest: %v %v", ok, err)
	}
	if got.ID != "V1" {
		t.Errorf("march bill got %s, want V1", got.ID)
	}

	// A charge in July gets V2.
	at = time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local).UnixMilli()
	got, _, _ = s.Latest(ctx, "T1", "S1", at)
	if got.ID != "V2" {
		t.Errorf("july bill got %s, want V2", got.ID)
	}

	// A charge before any version existed falls back to the earliest.
	at = time.Date(2025, 12, 1, 12, 0, 0, 0, time.Local).UnixMilli()
	got, ok, _ = s.Latest(ctx, "T1", "S1", at)
	if !ok || got.ID != "V1" {
		t.Errorf("pre-launch bill got %s ok=%v, want fallback V1", got.ID, ok)
	}
}

func TestTariffLatestIsolatesTenants(t *testing.T) {
	s := NewMemoryTariffStore()
	ctx := context.Background()
	_ = s.Save(ctx, TariffVersion{ID: "A", TenantID: "T1", StationID: "S1",
		Effective: time.Unix(100, 0), Slots: fullSlots()})
	_ = s.Save(ctx, TariffVersion{ID: "B", TenantID: "T2", StationID: "S1",
		Effective: time.Unix(100, 0), Slots: fullSlots()})
	got, ok, _ := s.Latest(ctx, "T2", "S1", time.Unix(200, 0).UnixMilli())
	if !ok || got.ID != "B" {
		t.Errorf("got %s, want B (no cross-tenant leak)", got.ID)
	}
}
