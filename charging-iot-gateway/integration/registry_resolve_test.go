package integration

import (
	"context"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-common/lock"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	billing "github.com/RARHeTec/OpenCharge/charging-core-service/module-order-billing"
)

type stubResolver struct{ devices map[string]DeviceBilling }

func (s stubResolver) ResolveDevice(_ context.Context, id string) (DeviceBilling, bool) {
	d, ok := s.devices[id]
	return d, ok
}

// A registered device must bill under its own tenant/station/wallet, so one
// gateway process can serve multiple sites without restarting with new flags.
func TestRegisteredDeviceOverridesDefaults(t *testing.T) {
	base := NewBridge(&recordingSink{}, lock.NewMemoryLocker())
	cap := &capturingSettlement{}
	bridge := NewSettlementBridge(base, cap, BillConfig{
		TenantID: "T-DEFAULT", StationID: "S-DEFAULT", UserID: "U-DEFAULT",
		TariffVersionID: "V1",
		Resolver: stubResolver{devices: map[string]DeviceBilling{
			"DEV-REG": {TenantID: "T9", StationID: "S9", UserID: "U9"},
		}},
	})

	txn := protocol.TransactionBody{
		TransactionID: "TXN-REG", DeviceID: "DEV-REG", GunNo: 1,
		TotalEnergy: 20000, TotalAmount: 7500,
		Valley: protocol.PriceSegment{EnergyUnits: 15000, AmountUnits: 4500},
		Peak:   protocol.PriceSegment{EnergyUnits: 5000, AmountUnits: 3000},
	}
	bridge.OnEvent(ykcserver.Event{Type: "StopTransaction", DeviceID: "DEV-REG", GunNo: 1, Payload: txn})

	if len(cap.bills) != 1 {
		t.Fatalf("bills = %d", len(cap.bills))
	}
	got := cap.bills[0]
	if got.TenantID != "T9" || got.StationID != "S9" || got.UserID != "U9" {
		t.Errorf("bill context = %s/%s/%s, want T9/S9/U9", got.TenantID, got.StationID, got.UserID)
	}
}

// An unknown device must not lose its bill; it falls back to configured defaults.
func TestUnknownDeviceFallsBackToDefaults(t *testing.T) {
	base := NewBridge(&recordingSink{}, lock.NewMemoryLocker())
	cap := &capturingSettlement{}
	bridge := NewSettlementBridge(base, cap, BillConfig{
		TenantID: "T-DEFAULT", StationID: "S-DEFAULT", UserID: "U-DEFAULT",
		Resolver: stubResolver{devices: map[string]DeviceBilling{}},
	})

	txn := protocol.TransactionBody{TransactionID: "TXN-UNK", DeviceID: "DEV-UNKNOWN", GunNo: 1,
		TotalEnergy: 20000, TotalAmount: 7500}
	bridge.OnEvent(ykcserver.Event{Type: "StopTransaction", DeviceID: "DEV-UNKNOWN", GunNo: 1, Payload: txn})

	if len(cap.bills) != 1 {
		t.Fatalf("bills = %d", len(cap.bills))
	}
	if cap.bills[0].TenantID != "T-DEFAULT" {
		t.Errorf("tenant = %s, want fallback T-DEFAULT", cap.bills[0].TenantID)
	}
}

var _ = billing.Result{}
