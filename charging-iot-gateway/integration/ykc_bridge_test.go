package integration

import (
	"context"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-common/lock"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/internal/channels"
)

type recordingSink struct {
	fast int
	slow int
}

func (r *recordingSink) Publish(topic, key string, e ykcserver.Event) {
	if topic == channels.TopicTelemetry {
		r.fast++
	} else {
		r.slow++
	}
}

func TestBridgeRoutesChannels(t *testing.T) {
	sink := &recordingSink{}
	b := NewBridge(sink, lock.NewMemoryLocker())
	b.OnEvent(ykcserver.Event{Type: "MeterValues", DeviceID: "D1", GunNo: 1})
	b.OnEvent(ykcserver.Event{Type: "StopTransaction", DeviceID: "D1", GunNo: 1})
	if sink.fast != 1 || sink.slow != 1 {
		t.Fatalf("sink fast=%d slow=%d", sink.fast, sink.slow)
	}
	if got := RouteTopic(protocol.TypeRealtime); got != channels.TopicTelemetry {
		t.Errorf("realtime routed to %s", got)
	}
	if got := RouteTopic(protocol.TypeTransaction); got != channels.TopicState {
		t.Errorf("transaction routed to %s", got)
	}
}

func TestConnectorLockPreventsDuplicateStart(t *testing.T) {
	b := NewBridge(&recordingSink{}, lock.NewMemoryLocker())
	first, err := b.StartCharge(context.Background(), "DEV001", 1, "trace-1")
	if err != nil || !first {
		t.Fatalf("first start = %v %v", first, err)
	}
	second, _ := b.StartCharge(context.Background(), "DEV001", 1, "trace-2")
	if second {
		t.Fatal("second start should be rejected while lock held")
	}
	if err := b.ReleaseConnector("DEV001", 1, "trace-1"); err != nil {
		t.Fatal(err)
	}
	third, _ := b.StartCharge(context.Background(), "DEV001", 1, "trace-3")
	if !third {
		t.Fatal("third start should succeed after release")
	}
}
