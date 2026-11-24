package integration

import (
	"context"
	"log/slog"
	"time"

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
	TenantID        string
	StationID       string
	UserID          string // In production this comes from the token/card mapping.
	TariffVersionID string // Fallback snapshot id when no timestamp is available.
	// TariffAt resolves the version effective at the charge start time. This is
	// what makes a delayed offline bill price with historical tariffs instead of
	// whatever version happens to be current when it finally uploads.
	TariffAt TariffAtResolver
	// Location interprets the device CP56 wall-clock time.
	Location *time.Location
	// Resolver looks up tenant/station/wallet for a device from master data.
	// When nil (or the device is unknown) the fields above are used.
	Resolver DeviceResolver
}

// DeviceResolver maps a device to its billing context from the registry.
type DeviceResolver interface {
	ResolveDevice(ctx context.Context, deviceID string) (DeviceBilling, bool)
}

// DeviceBilling is the per-device billing context.
type DeviceBilling struct {
	TenantID  string
	StationID string
	UserID    string
}

// TariffAtResolver returns the tariff version id effective at a moment.
type TariffAtResolver interface {
	// ResolveTariffVersion returns "" when nothing matches.
	ResolveTariffVersion(ctx context.Context, tenantID, stationID string, at time.Time) (string, error)
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
	cfg := b.cfgFor(txn.DeviceID)
	bill := ToBill(e, txn, cfg)
	ctx := context.Background()
	if vid, ok := b.resolveTariff(ctx, txn, cfg); ok {
		bill.TariffVersionID = vid
	}
	res, err := b.settlement.Settle(ctx, bill)
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

// cfgFor returns the billing config for a device, preferring master data over
// the static flags so one gateway process can serve many stations and tenants.
func (b *SettlementBridge) cfgFor(deviceID string) BillConfig {
	cfg := b.cfg
	if cfg.Resolver == nil {
		return cfg
	}
	db, ok := cfg.Resolver.ResolveDevice(context.Background(), deviceID)
	if !ok {
		slog.Warn("device not in registry, using default billing config", "device", deviceID)
		return cfg
	}
	cfg.TenantID = db.TenantID
	cfg.StationID = db.StationID
	if db.UserID != "" {
		cfg.UserID = db.UserID
	}
	return cfg
}

// resolveTariff picks the tariff version valid at the charge start time. It
// falls back to the configured snapshot when no resolver or timestamp exists.
func (b *SettlementBridge) resolveTariff(ctx context.Context, txn protocol.TransactionBody, cfg BillConfig) (string, bool) {
	if cfg.TariffAt == nil || len(txn.StartCP56) < 7 {
		return "", false
	}
	start, err := protocol.ParseCP56Time2a(txn.StartCP56, cfg.Location)
	if err != nil {
		slog.Warn("cannot parse charge start time, using configured tariff",
			"err", err, "txn", txn.TransactionID)
		return "", false
	}
	vid, err := cfg.TariffAt.ResolveTariffVersion(ctx, cfg.TenantID, cfg.StationID, start)
	if err != nil || vid == "" {
		return "", false
	}
	return vid, true
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
