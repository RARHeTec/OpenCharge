// Package spi defines the Protocol Adapter SPI. Every physical protocol
// implements this interface and is registered at startup. Business code only
// ever sees unified thing-model events.
package spi

import (
	"context"

	model "github.com/RARHeTec/OpenCharge/charging-thing-model/event"
)

// Adapter converts between native protocol frames and unified events.
type Adapter interface {
	// Code is the protocol identifier, e.g. "yunkuaichong-1.6" or "ocpp1.6j".
	Code() string

	// Decode turns one native frame into zero or more unified uplink events.
	Decode(ctx context.Context, deviceID string, frame []byte) ([]model.Envelope, error)

	// Encode turns one unified downlink command into a native frame.
	Encode(ctx context.Context, cmd model.Envelope) ([]byte, error)

	// Heartbeat returns a native keepalive frame, if the protocol has one.
	Heartbeat(deviceID string) []byte
}

// Registry holds all known adapters keyed by Code().
type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

func (r *Registry) Register(a Adapter) {
	r.adapters[a.Code()] = a
}

func (r *Registry) Get(code string) (Adapter, bool) {
	a, ok := r.adapters[code]
	return a, ok
}

func (r *Registry) Codes() []string {
	out := make([]string, 0, len(r.adapters))
	for k := range r.adapters {
		out = append(out, k)
	}
	return out
}
