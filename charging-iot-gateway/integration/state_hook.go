package integration

import (
	"context"
	"log/slog"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/state"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/telemetry"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
)

// StateHook updates live connector state from device events. It composes with
// SettlementBridge so both concerns run on the same event stream.
type StateHook struct {
	store     state.Store
	telemetry *telemetry.Buffer
}

func NewStateHook(store state.Store, buffers ...*telemetry.Buffer) *StateHook {
	h := &StateHook{store: store}
	if len(buffers) > 0 {
		h.telemetry = buffers[0]
	}
	return h
}

func (h *StateHook) OnEvent(e ykcserver.Event) {
	if h == nil || h.store == nil {
		return
	}
	ctx := context.Background()
	now := time.Now()

	switch e.Type {
	case "BootNotification":
		if b, ok := e.Payload.(protocol.LoginBody); ok {
			for gun := 1; gun <= int(b.GunCount); gun++ {
				_ = h.store.Update(ctx, state.ConnectorState{
					DeviceID: e.DeviceID, GunNo: gun, Status: "idle", LastSeen: now,
				})
			}
		}

	case "Heartbeat":
		if b, ok := e.Payload.(protocol.HeartbeatBody); ok {
			st := "idle"
			if b.Status == 1 {
				st = "faulted"
			}
			_ = h.store.Update(ctx, state.ConnectorState{
				DeviceID: e.DeviceID, GunNo: int(b.GunNo), Status: st, LastSeen: now,
			})
		}

	case "MeterValues":
		if b, ok := e.Payload.(protocol.RealtimeBody); ok {
			if h.telemetry != nil {
				h.telemetry.Push(telemetry.Sample{
					Timestamp: now, DeviceID: e.DeviceID, ConnectorID: int(b.GunNo),
					VoltageMV: int64(b.Voltage), CurrentMA: int64(b.Current),
					EnergyWh: int64(b.EnergyUnits),
				})
			}
			status := map[byte]string{0: "offline", 1: "faulted", 2: "idle", 3: "charging"}[b.State]
			_ = h.store.Update(ctx, state.ConnectorState{
				DeviceID: e.DeviceID, GunNo: int(b.GunNo), Status: status,
				TransactionID: b.TransactionID, EnergyWh: int64(b.EnergyUnits),
				AmountUnits: int64(b.AmountUnits), LastSeen: now,
				FaultCode: int(b.Fault),
			})
		}

	case "StopTransaction":
		// Mark the gun idle once the bill is accepted, but carry forward the
		// last meter values so the admin view still shows the completed session.
		prev, ok, _ := h.store.Get(ctx, e.DeviceID, int(e.GunNo))
		if !ok {
			prev = state.ConnectorState{DeviceID: e.DeviceID, GunNo: int(e.GunNo)}
		}
		prev.Status = "idle"
		prev.TransactionID = e.TransactionID
		prev.LastSeen = now
		_ = h.store.Update(ctx, prev)
	}

	slog.Debug("connector state updated", "traceId", e.TraceID, "type", e.Type)
}
