package bus

import (
	"context"
	"testing"

	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/internal/channels"
)

func TestRouteSeparatesChannels(t *testing.T) {
	telemetry, tk := Route(ykcserver.Event{Type: "MeterValues", DeviceID: "D1", GunNo: 1})
	if telemetry != channels.TopicTelemetry || tk != "D1" {
		t.Errorf("telemetry -> %s / %s", telemetry, tk)
	}
	state, sk := Route(ykcserver.Event{Type: "StopTransaction", DeviceID: "D1", GunNo: 2})
	if state != channels.TopicState || sk != "D1:2" {
		t.Errorf("state -> %s / %s", state, sk)
	}
}

func TestMemoryBusRecordsByTopic(t *testing.T) {
	b := NewMemoryBus()
	ctx := context.Background()
	topic, key := Route(ykcserver.Event{Type: "MeterValues", DeviceID: "D1"})
	_ = b.Publish(ctx, topic, key, ykcserver.Event{Type: "MeterValues"})
	topic, key = Route(ykcserver.Event{Type: "StopTransaction", DeviceID: "D1", GunNo: 1})
	_ = b.Publish(ctx, topic, key, ykcserver.Event{Type: "StopTransaction"})
	if got := len(b.Events(channels.TopicTelemetry)); got != 1 {
		t.Errorf("telemetry events = %d", got)
	}
	if got := len(b.Events(channels.TopicState)); got != 1 {
		t.Errorf("state events = %d", got)
	}
}
