package integration

import (
	"context"
	"log/slog"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	billing "github.com/RARHeTec/OpenCharge/charging-core-service/module-order-billing"
)

// SettlementPort is the settlement service as seen by the gateway. Defining
// it here keeps the gateway independent of storage details.
type SettlementPort interface {
	Settle(ctx context.Context, bill billing.Bill) (billing.Result, error)
}

// SettlementBridge extends Bridge: on a StopTransaction (0x3B) it converts the
// device bill and calls the settlement service. All other events route as before.
type SettlementBridge struct {
	*Bridge
	settlement SettlementPort
	cfg        BillConfig
}

type BillConfig struct {
	TenantID  string
	StationID string
	UserID          string // In production this comes from the token/card mapping.
	TariffVersionID string // Platform snapshot id, never derived from pile data.
}

func NewSettlementBridge(base *Bridge, settlement SettlementPort, cfg BillConfig) *SettlementBridge {
	return &SettlementBridge{Bridge: base, settlement: settlement, cfg: cfg}
}

func (b *SettlementBridge) OnEvent(e ykcserver.Event) {
	// Keep topic routing and connector lock behavior from the base bridge.
	b.Bridge.OnEvent(e)

	if e.Type != "StopTransaction" || b.settlement == nil {
		return
	}
	txn, ok := e.Payload.(protocol.TransactionBody)
	if !ok {
		slog.Warn("settlement: unexpected transaction payload", "traceId", e.TraceID)
		return
	}
	bill := ToBill(e, txn, b.cfg)
	res, err := b.settlement.Settle(context.Background(), bill)
	if err != nil {
		slog.Error("settlement failed", "traceId", e.TraceID, "txn", txn.TransactionID, "err", err)
		return
	}
	slog.Info("settlement result",
		"traceId", e.TraceID,
		"txn", txn.TransactionID,
		"status", res.Status,
		"reason", res.ReasonCode,
		"pile", res.PileAmount.String(),
		"platform", res.PlatformAmount.String(),
		"charged", res.ChargeAmount.String(),
	)
}

// ToBill converts a parsed 0x3B record into the settlement input.
// All amounts are decoded from the protocol's integer fields - no floats.
func ToBill(e ykcserver.Event, t protocol.TransactionBody, cfg BillConfig) billing.Bill {
	segSum := t.Sharp.AmountUnits + t.Peak.AmountUnits + t.Flat.AmountUnits + t.Valley.AmountUnits
	segs := make([]billing.BillSegment, 0, 4)
	add := func(period string, slot int, seg protocol.PriceSegment) {
		if seg.EnergyUnits > 0 {
			segs = append(segs, billing.BillSegment{
				Period: period, Slot: slot, EnergyUnits: int64(seg.EnergyUnits),
			})
		}
	}
	// Protocol periods map to tariff slots; in production the slot should be
	// derived from the CP56 start/end timestamps and the station timezone.
	add("sharp", 2, t.Sharp)
	add("peak", 1, t.Peak)
	add("flat", 3, t.Flat)
	add("valley", 0, t.Valley)

	return billing.Bill{
		TransactionID:   t.TransactionID,
		TenantID:        cfg.TenantID,
		StationID:       cfg.StationID,
		UserID:          cfg.UserID,
		DeviceID:        t.DeviceID,
		GunNo:           int(t.GunNo),
		TariffVersionID: cfg.TariffVersionID,
		EnergyUnits:     int64(t.TotalEnergy),
		PileAmount:      money.FromUnits(int64(t.TotalAmount)),
		PileSegmentSum:  money.FromUnits(int64(segSum)),
		Segments:        segs,
		TraceID:         e.TraceID,
	}
}

// Ensure the settlement bridge is still a valid server handler.
var _ ykcserver.Handler = (*SettlementBridge)(nil)
