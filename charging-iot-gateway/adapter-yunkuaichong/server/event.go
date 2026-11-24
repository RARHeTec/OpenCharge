package server

import "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"

// Event is what the TCP server emits upward for business integration.
// It deliberately mirrors the unified thing-model event names.
type Event struct {
	TraceID       string
	Type          string // BootNotification, Heartbeat, StatusNotification, MeterValues, StopTransaction
	DeviceID      string
	GunNo         byte
	TransactionID string
	Payload       any
}

// Handler receives decoded uplink events. Injectable for tests and integration.
type Handler interface {
	OnEvent(e Event)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(Event)

func (f HandlerFunc) OnEvent(e Event) { f(e) }

// NopHandler discards events.
type NopHandler struct{}

func (NopHandler) OnEvent(Event) {}

// Stats is an in-memory view of what the server has seen, used by tests and
// for the /stats endpoint. No external middleware required.
type Stats struct {
	LoginCount       int
	HeartbeatCount   int
	RealtimeCount    int
	StartRequestCount int
	TransactionCount int
	LastRealtime     *protocol.RealtimeBody
	LastTransaction  *protocol.TransactionBody
}
