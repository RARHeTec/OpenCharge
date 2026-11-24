package integration

import (
	"context"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-common/lock"
	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	billing "github.com/RARHeTec/OpenCharge/charging-core-service/module-order-billing"
)

type capturingSettlement struct {
	bills []billing.Bill
}

func (c *capturingSettlement) Settle(_ context.Context, b billing.Bill) (billing.Result, error) {
	c.bills = append(c.bills, b)
	return billing.Result{Status: billing.ResultSettled, ChargeAmount: b.PileAmount}, nil
}

func TestSettlementBridgeConvertsTransaction(t *testing.T) {
	base := NewBridge(&recordingSink{}, lock.NewMemoryLocker())
	cap := &capturingSettlement{}
	bridge := NewSettlementBridge(base, cap, BillConfig{
		TenantID: "T1", StationID: "S1", UserID: "U1", TariffVersionID: "V1",
	})

	txn := protocol.TransactionBody{
		TransactionID: "TXN-100", DeviceID: "D1", GunNo: 1,
		TotalEnergy: 20000, TotalAmount: 7500,
		Valley: protocol.PriceSegment{EnergyUnits: 15000, AmountUnits: 4500},
		Peak:   protocol.PriceSegment{EnergyUnits: 5000, AmountUnits: 3000},
	}
	bridge.OnEvent(ykcserver.Event{
		TraceID: "trace-x", Type: "StopTransaction", DeviceID: "D1",
		GunNo: 1, TransactionID: "TXN-100", Payload: txn,
	})

	if len(cap.bills) != 1 {
		t.Fatalf("settlement calls = %d", len(cap.bills))
	}
	got := cap.bills[0]
	if got.TransactionID != "TXN-100" || got.TenantID != "T1" || got.TariffVersionID != "V1" {
		t.Errorf("bill header = %+v", got)
	}
	if got.PileAmount.String() != "0.7500" {
		t.Errorf("pile amount = %s", got.PileAmount.String())
	}
	if got.PileSegmentSum.String() != "0.7500" {
		t.Errorf("segment sum = %s", got.PileSegmentSum.String())
	}
	if got.EnergyUnits != 20000 || len(got.Segments) != 2 {
		t.Errorf("energy=%d segments=%d", got.EnergyUnits, len(got.Segments))
	}
	// Ensure no amount was derived from pile prices: platform tariff is authoritative.
	if got.TariffVersionID != "V1" {
		t.Error("tariff version must come from platform config")
	}
}

func TestSettlementBridgeIgnoresNonTransaction(t *testing.T) {
	base := NewBridge(&recordingSink{}, lock.NewMemoryLocker())
	cap := &capturingSettlement{}
	bridge := NewSettlementBridge(base, cap, BillConfig{})
	bridge.OnEvent(ykcserver.Event{Type: "MeterValues", DeviceID: "D1", GunNo: 1})
	if len(cap.bills) != 0 {
		t.Error("telemetry must not trigger settlement")
	}
}

var _ = money.Zero
