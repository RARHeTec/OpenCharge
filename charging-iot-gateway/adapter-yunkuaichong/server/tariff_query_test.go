package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

// A pile that verifies with an outdated model must be told "mismatch" (0x01)
// so it requests a fresh model, and the 0x09 request must return the full
// 90-byte model instead of the old empty placeholder.
func TestTariffVerifyAndRequest(t *testing.T) {
	const device = "55031412782305"
	model := protocol.TariffModel{ModelCode: 7}
	model.Rates = protocol.TariffRates{SharpElec: 60000, SharpService: 18000, PeakElec: 88000, PeakService: 15000, FlatElec: 119500, FlatService: 9500, ValleyElec: 60000, ValleyService: 18000}
	model.Slots[0] = 3

	srv := New("127.0.0.1:0", NopHandler{})
	srv.TariffModelFor = func(id string) (protocol.TariffModel, bool) {
		if id == device {
			return model, true
		}
		return protocol.TariffModel{}, false
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Log in so the session knows its device id, using the real wire format.
	login := make([]byte, 30)
	copy(login[:7], protocol.DeviceBCD(device))
	login[7], login[8], login[9] = 0, 2, 0x10
	copy(login[10:18], []byte("v1.6.0\x00\x00"))
	login[18], login[29] = 1, 4
	sendFrame(t, conn, 1, protocol.TypeLogin, login)
	recvFrame(t, conn) // login ack

	// 0x05 with pile model 99 -> platform model 7, expect result 0x01.
	verify := append(append([]byte(nil), protocol.DeviceBCD(device)...), 0x63, 0x00)
	sendFrame(t, conn, 2, protocol.TypeTariffVerify, verify)
	ack := recvFrame(t, conn)
	if ack.Type != protocol.TypeTariffVerifyAck || len(ack.Body) < 10 || ack.Body[9] != 0x01 {
		t.Fatalf("verify ack = type %X body %X, want result 0x01", ack.Type, ack.Body)
	}

	// 0x09 -> complete model.
	sendFrame(t, conn, 3, protocol.TypeTariffRequest, protocol.DeviceBCD(device))
	resp := recvFrame(t, conn)
	if resp.Type != protocol.TypeTariffRequestAck {
		t.Fatalf("model response type = %X", resp.Type)
	}
	if len(resp.Body) != protocol.TariffBodyLen {
		t.Fatalf("model body = %d bytes, want %d", len(resp.Body), protocol.TariffBodyLen)
	}
	got, err := protocol.ParseTariffModel(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelCode != 7 || got.Rates.ValleyElec != 60000 || got.Slots[0] != 3 {
		t.Errorf("decoded model = %+v", got)
	}
}

func sendFrame(t *testing.T, conn net.Conn, seq uint16, typ byte, body []byte) {
	t.Helper()
	if _, err := conn.Write(protocol.Encode(seq, false, typ, body)); err != nil {
		t.Fatal(err)
	}
}

func recvFrame(t *testing.T, conn net.Conn) protocol.Frame {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	f, err := protocol.NewSplitter(conn).NextFrame()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := protocol.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
