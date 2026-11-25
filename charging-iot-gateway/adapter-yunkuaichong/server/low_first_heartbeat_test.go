package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

// A real pile sends device id 55 30 41 21 87 32 10 (low-nibble-first BCD),
// which is the registry id 55031412782301. Verify the whole wire path decodes
// login and heartbeat frames to that id under the "low" setting.
func TestLowFirstPileHeartbeatDecodesToRegistryId(t *testing.T) {
	if err := protocol.SetBCDOrder("low"); err != nil {
		t.Fatal(err)
	}
	defer protocol.SetBCDOrder("")

	const wantID = "55031412782301"
	var gotLogin, gotHeartbeat string
	done := make(chan struct{})

	handler := handlerFunc(func(e Event) {
		switch e.Type {
		case "BootNotification":
			gotLogin = e.DeviceID
		case "Heartbeat":
			gotHeartbeat = e.DeviceID
			close(done)
		}
	})
	srv := New("127.0.0.1:0", handler)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	dev := protocol.DeviceBCD(wantID)
	if got := dev; len(got) != 7 {
		t.Fatalf("device bcd len = %d", len(got))
	}
	login := make([]byte, 30)
	copy(login[:7], dev)
	login[7], login[8], login[9] = 0, 2, 0x10
	copy(login[10:18], []byte("v1.6.0\x00\x00"))
	login[18], login[29] = 1, 4

	sendFrame(t, conn, 1, protocol.TypeLogin, login)
	recvFrame(t, conn) // login ack
	sendFrame(t, conn, 2, protocol.TypeHeartbeat, append(append([]byte(nil), dev...), 1, 0))
	recvFrame(t, conn) // heartbeat ack

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat event received")
	}
	if gotLogin != wantID {
		t.Errorf("login device = %s, want %s", gotLogin, wantID)
	}
	if gotHeartbeat != wantID {
		t.Errorf("heartbeat device = %s, want %s", gotHeartbeat, wantID)
	}
}

type handlerFunc func(Event)

func (f handlerFunc) OnEvent(e Event) { f(e) }
