// Package integration bridges the raw YunKuaiChong TCP server to the unified
// thing-model event flow. It enforces the connector-level lock before any
// start command is dispatched and routes telemetry vs state to separate topics.
package integration

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/lock"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/internal/channels"
)

// Sink receives unified events, one per topic. Tests can assert routing.
type Sink interface {
	Publish(topic string, key string, event ykcserver.Event)
}

// Bridge implements ykcserver.Handler.
type Bridge struct {
	sink    Sink
	locker  lock.Locker
	mu      sync.Mutex
	started map[string]time.Time // key: device:gun -> when lock acquired
}

func NewBridge(sink Sink, locker lock.Locker) *Bridge {
	return &Bridge{sink: sink, locker: locker, started: make(map[string]time.Time)}
}

func (b *Bridge) OnEvent(e ykcserver.Event) {
	topic := channels.TopicState
	key := channels.StateKey(e.DeviceID, int(e.GunNo))
	switch e.Type {
	case "MeterValues":
		topic = channels.TopicTelemetry
		key = e.DeviceID
	case "BootNotification", "Heartbeat":
		key = e.DeviceID
	}
	b.sink.Publish(topic, key, e)
}

// StartCharge enforces the connector-level distributed lock. Returns false if
// another scan already holds the gun, preventing duplicate start commands.
func (b *Bridge) StartCharge(ctx context.Context, deviceID string, gun int, token string) (bool, error) {
	ok, err := b.locker.Acquire(ctx, deviceID, gun, token, 30*time.Second)
	if !ok || err != nil {
		slog.Warn("connector lock rejected start", "device", deviceID, "gun", gun, "err", err)
		return false, err
	}
	b.mu.Lock()
	b.started[lock.Key(deviceID, gun)] = time.Now()
	b.mu.Unlock()
	return true, nil
}

func (b *Bridge) ReleaseConnector(deviceID string, gun int, token string) error {
	_, err := b.locker.Release(context.Background(), deviceID, gun, token)
	return err
}

// HoldsLock is a test/inspection helper.
func (b *Bridge) HoldsLock(deviceID string, gun int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.started[lock.Key(deviceID, gun)]
	return ok
}

// RouteTopic exposes the fast/slow channel decision for tests.
func RouteTopic(frameType byte) string {
	if channels.IsHighPriority(frameTypeName(frameType)) {
		return channels.TopicState
	}
	return channels.TopicTelemetry
}

func frameTypeName(t byte) string {
	if t == protocol.TypeRealtime {
		return "MeterValues"
	}
	return "StateChange"
}
