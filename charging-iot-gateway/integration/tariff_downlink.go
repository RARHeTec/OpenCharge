package integration

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/RARHeTec/OpenCharge/charging-common/state"
	"github.com/RARHeTec/OpenCharge/charging-common/tariff"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
)

// TariffSender is the portion of the TCP server needed for downlink.
type TariffSender interface {
	SendTariffModel(deviceID string, body []byte) error
}

// TariffDownlink publishes a PricePlan as a 0x58 billing model to a pile.
type TariffDownlink struct {
	mu       sync.Mutex
	sender   TariffSender
	registry storage.RegistryStore
	plans    storage.PricePlanStore
	states   state.Store
	pending  map[string]string // deviceID -> planID awaiting 0x57
}

func NewTariffDownlink(sender TariffSender, registry storage.RegistryStore,
	plans storage.PricePlanStore, states state.Store) *TariffDownlink {
	return &TariffDownlink{
		sender: sender, registry: registry, plans: plans, states: states,
		pending: make(map[string]string),
	}
}

// DeliverResult reports how many piles were targeted and how many accepted.
type DeliverResult struct {
	PlanID   string   `json:"planId"`
	Targeted []string `json:"targeted"`
	Skipped  []string `json:"skipped"` // busy or offline piles
	Sent     int      `json:"sent"`
}

var (
	ErrPlanNotFound   = errors.New("price plan not found")
	ErrNoDeviceType   = errors.New("plan has no device type")
	ErrModelCodeLarge = errors.New("model code exceeds 2-byte BCD range")
)

// Deliver sends the plan to every online, idle pile of its device type.
func (d *TariffDownlink) Deliver(ctx context.Context, planID string) (tariff.DeliverResult, error) {
	res := tariff.DeliverResult{PlanID: planID}
	plan, ok, err := d.plans.Get(ctx, planID)
	if err != nil {
		return res, err
	}
	if !ok {
		return res, ErrPlanNotFound
	}
	if plan.ModelCode < 0 || plan.ModelCode > 9999 {
		return res, ErrModelCodeLarge
	}

	code, rates, slots, err := storage.PriceToProtocol(plan)
	if err != nil {
		return res, err
	}
	model := protocol.TariffModel{
		ModelCode: code,
		Rates: protocol.TariffRates{
			SharpElec: uint32(rates.SharpElec), SharpService: uint32(rates.SharpService),
			PeakElec: uint32(rates.PeakElec), PeakService: uint32(rates.PeakService),
			FlatElec: uint32(rates.FlatElec), FlatService: uint32(rates.FlatService),
			ValleyElec: uint32(rates.ValleyElec), ValleyService: uint32(rates.ValleyService),
		},
		Slots: slots,
	}

	devices, err := d.registry.ListDevices(ctx, plan.TenantID, plan.StationID)
	if err != nil {
		return res, err
	}
	for _, dev := range devices {
		if dev.DeviceType != plan.DeviceType {
			continue
		}
		// Only deliver during idle periods per spec section 6.6/9.5.
		if !d.isIdle(ctx, dev.DeviceID) {
			res.Skipped = append(res.Skipped, dev.DeviceID)
			continue
		}
		body, err := protocol.BuildTariffSetBody(protocol.DeviceBCD(dev.DeviceID), model)
		if err != nil {
			return res, err
		}
		if err := d.sender.SendTariffModel(dev.DeviceID, body); err != nil {
			slog.Warn("tariff downlink failed", "device", dev.DeviceID, "err", err)
			res.Skipped = append(res.Skipped, dev.DeviceID)
			continue
		}
		d.mu.Lock()
		d.pending[dev.DeviceID] = plan.ID
		d.mu.Unlock()
		res.Targeted = append(res.Targeted, dev.DeviceID)
		res.Sent++
	}
	plan.DeliveredCount += res.Sent
	if err := d.plans.Save(ctx, plan); err != nil {
		return res, err
	}
	return res, nil
}

func (d *TariffDownlink) isIdle(ctx context.Context, deviceID string) bool {
	if d.states == nil {
		return true // no live state store, assume idle in local demos
	}
	rows, err := d.states.List(ctx, deviceID)
	if err != nil || len(rows) == 0 {
		return true // never reported: treat as idle
	}
	for _, st := range rows {
		if st.Status == "charging" || st.Status == "plugged" {
			return false
		}
	}
	return true
}

// OnAck handles 0x57 from a pile and records success in the plan.
func (d *TariffDownlink) OnAck(ctx context.Context, deviceID string, success bool) {
	d.mu.Lock()
	planID := d.pending[deviceID]
	delete(d.pending, deviceID)
	d.mu.Unlock()
	if planID == "" {
		return
	}
	plan, ok, err := d.plans.Get(ctx, planID)
	if err != nil || !ok {
		return
	}
	if success {
		plan.SuccessCount++
	}
	if err := d.plans.Save(ctx, plan); err != nil {
		slog.Warn("failed to record tariff ack", "err", err)
	}
}

// OnEvent records 0x57 acknowledgements so delivery counts reflect reality.
func (d *TariffDownlink) OnEvent(e ykcserver.Event) {
	if e.Type != "TariffSetAck" {
		return
	}
	success, _ := e.Payload.(bool)
	d.OnAck(context.Background(), e.DeviceID, success)
}
