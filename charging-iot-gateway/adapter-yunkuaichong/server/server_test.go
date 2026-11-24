package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

// testClient is a minimal pile simulator used to validate the server end to end.
type testClient struct {
	conn net.Conn
	seq  uint16
}

func dial(t *testing.T, addr string) *testClient {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{conn: c}
}

func (c *testClient) send(t *testing.T, typ byte, body []byte) {
	t.Helper()
	c.seq++
	wire := protocol.Encode(c.seq, false, typ, body)
	if _, err := c.conn.Write(wire); err != nil {
		t.Fatal(err)
	}
}

func (c *testClient) recv(t *testing.T) protocol.Frame {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	s := protocol.NewSplitter(c.conn)
	wire, err := s.NextFrame()
	if err != nil {
		t.Fatal(err)
	}
	f, err := protocol.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func deviceBCDTest(id string) []byte {
	out := make([]byte, 7)
	copy(out, id)
	return out
}

func TestServerFullFlow(t *testing.T) {
	h := &collector{}
	srv := New("127.0.0.1:0", h)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	c := dial(t, srv.Addr())
	defer c.conn.Close()

	// 1. login -> ack
	loginBody := make([]byte, 30)
	copy(loginBody[:7], deviceBCDTest("5503141"))
	loginBody[7] = 0
	loginBody[8] = 2
	loginBody[9] = 0x0F
	c.send(t, protocol.TypeLogin, loginBody)
	ack := c.recv(t)
	if ack.Type != protocol.TypeLoginAck || ack.Body[7] != 0 {
		t.Fatalf("login ack = %x body=%x", ack.Type, ack.Body)
	}

	// 2. heartbeat -> ack
	hb := append(deviceBCDTest("5503141"), 1, 0)
	c.send(t, protocol.TypeHeartbeat, hb)
	hbAck := c.recv(t)
	if hbAck.Type != protocol.TypeHeartbeatAck {
		t.Fatalf("heartbeat ack type = %x", hbAck.Type)
	}

	// 3. realtime telemetry (69 bytes, no response expected)
	rt := make([]byte, 69)
	copy(rt[:16], []byte("0000000000000001"))
	copy(rt[16:23], deviceBCDTest("5503141"))
	rt[23] = 1  // gun
	rt[24] = 3  // charging
	c.send(t, protocol.TypeRealtime, rt)

	// 4. transaction record -> ack
	txn := make([]byte, 178)
	copy(txn[:16], []byte("0000000000000001"))
	copy(txn[16:23], deviceBCDTest("5503141"))
	txn[23] = 1
	c.send(t, protocol.TypeTransaction, txn)
	txnAck := c.recv(t)
	if txnAck.Type != protocol.TypeTransactionAck {
		t.Fatalf("transaction ack type = %x", txnAck.Type)
	}
	if txnAck.Body[16] != 0 {
		t.Fatalf("transaction ack result = %d", txnAck.Body[16])
	}

	// allow goroutine to finish ingestion
	time.Sleep(100 * time.Millisecond)
	st := srv.Stats()
	if st.LoginCount != 1 || st.HeartbeatCount != 1 || st.RealtimeCount != 1 || st.TransactionCount != 1 {
		t.Fatalf("stats = %+v", st)
	}
	if len(h.events) < 4 {
		t.Fatalf("handler events = %d", len(h.events))
	}
}

type collector struct {
	events []Event
}

func (c *collector) OnEvent(e Event) { c.events = append(c.events, e) }
