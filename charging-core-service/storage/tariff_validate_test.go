package storage

import (
	"context"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

func fullSlots() []TariffSlot {
	slots := make([]TariffSlot, 48)
	for i := range slots {
		slots[i] = TariffSlot{Slot: i, Price: money.MustParsePrice("0.60000")}
	}
	return slots
}

func TestValidateTariffOK(t *testing.T) {
	v := TariffVersion{ID: "V1", TenantID: "T1", StationID: "S1", Slots: fullSlots()}
	if err := ValidateTariff(v); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTariffRejects(t *testing.T) {
	base := TariffVersion{ID: "V1", TenantID: "T1", StationID: "S1", Slots: fullSlots()}

	short := base
	short.Slots = base.Slots[:47]
	if err := ValidateTariff(short); err == nil {
		t.Error("expected error for 47 slots")
	}

	dup := base
	dup.Slots = fullSlots()
	dup.Slots[1] = dup.Slots[0]
	if err := ValidateTariff(dup); err == nil {
		t.Error("expected duplicate slot error")
	}

	neg := base
	neg.Slots = fullSlots()
	neg.Slots[0].Price = money.Price(-1)
	if err := ValidateTariff(neg); err == nil {
		t.Error("expected negative price error")
	}

	noTenant := base
	noTenant.TenantID = ""
	if err := ValidateTariff(noTenant); err == nil {
		t.Error("expected tenant error")
	}
}

func TestMemoryTariffList(t *testing.T) {
	s := NewMemoryTariffStore()
	ctx := context.Background()
	_ = s.Save(ctx, TariffVersion{ID: "V1", TenantID: "T1", StationID: "S1", Effective: time.Unix(100, 0), Slots: fullSlots()})
	_ = s.Save(ctx, TariffVersion{ID: "V2", TenantID: "T1", StationID: "S1", Effective: time.Unix(200, 0), Slots: fullSlots()})
	_ = s.Save(ctx, TariffVersion{ID: "V3", TenantID: "T2", StationID: "S9", Effective: time.Unix(300, 0), Slots: fullSlots()})

	got, err := s.List(ctx, "T1", "S1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("list = %d, want 2", len(got))
	}
	if got[0].ID != "V2" {
		t.Errorf("newest first: got %s", got[0].ID)
	}
}
