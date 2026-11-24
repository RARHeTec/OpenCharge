// Package bus provides the unified event bus with physically separated
// fast (telemetry) and slow (state/settlement) channels.
package bus

import (
	"context"
	"sync"

	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/internal/channels"
)

// Bus publishes events to one of the two channels.
type Bus interface {
	Publish(ctx context.Context, topic, key string, e ykcserver.Event) error
	Close() error
}

// Route decides the channel for a device event.
func Route(e ykcserver.Event) (topic, key string) {
	key = channels.StateKey(e.DeviceID, int(e.GunNo))
	if e.Type == "MeterValues" {
		return channels.TopicTelemetry, e.DeviceID
	}
	return channels.TopicState, key
}

// MemoryBus is the default in-process implementation. It records events so
// tests and the admin view can assert routing without middleware.
type MemoryBus struct {
	mu     sync.Mutex
	events map[string][]ykcserver.Event
}

func NewMemoryBus() *MemoryBus {
	return &MemoryBus{events: make(map[string][]ykcserver.Event)}
}

func (b *MemoryBus) Publish(_ context.Context, topic, _ string, e ykcserver.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events[topic] = append(b.events[topic], e)
	return nil
}
func (b *MemoryBus) Close() error { return nil }

func (b *MemoryBus) Events(topic string) []ykcserver.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]ykcserver.Event(nil), b.events[topic]...)
}
