package integration

import (
	"context"
	"testing"
	"github.com/RARHeTec/OpenCharge/charging-common/state"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
)

// End-to-end: a real-format 0x13 frame with plugged=1 must surface as
// "occupied", and the unplugged frame must return it to "idle".
func TestRealPilePlugUnplugUpdatesAdminState(t *testing.T) {
	if err := protocol.SetBCDOrder("low"); err != nil {
		t.Fatal(err)
	}
	defer protocol.SetBCDOrder("")

	const deviceID = "55031412782301"
	store := state.NewMemoryStore()
	hook := NewStateHook(store)
	var h *StateHook = hook
	srv := ykcserver.New("127.0.0.1:0", h)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	// Drive the hook directly with the decoded realtime body, matching what
	// the server emits for the operator's 0x13 frames.
	push := func(plugged byte, want string) {
		t.Helper()
		h.OnEvent(ykcserver.Event{
			Type: "MeterValues", DeviceID: deviceID, GunNo: 1,
			Payload: protocol.RealtimeBody{
				DeviceID: deviceID, GunNo: 1, State: 2, Plugged: plugged,
			},
		})
		got, ok, err := store.Get(context.Background(), deviceID, 1)
		if err != nil || !ok {
			t.Fatalf("state not found: ok=%v err=%v", ok, err)
		}
		if got.Status != want {
			t.Errorf("plugged=%d status = %s, want %s", plugged, got.Status, want)
		}
	}

	push(1, "occupied") // 已插枪
	push(0, "idle")     // 未插枪(归位)

	// Charging must win even if the connector is still plugged.
	h.OnEvent(ykcserver.Event{Type: "MeterValues", DeviceID: deviceID, GunNo: 1,
		Payload: protocol.RealtimeBody{DeviceID: deviceID, GunNo: 1, State: 3, Plugged: 1}})
	got, _, _ := store.Get(context.Background(), deviceID, 1)
	if got.Status != "charging" {
		t.Errorf("charging+plugged = %s, want charging", got.Status)
	}
}
