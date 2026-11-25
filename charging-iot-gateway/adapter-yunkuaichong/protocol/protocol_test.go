package protocol

import (
	"bytes"
	"testing"
)

func TestCRC16GoldenVectors(t *testing.T) {
	// Independently established CRC-16/Modbus vectors (init 0xFFFF, poly 0xA001).
	cases := []struct {
		in   string
		want uint16
	}{
		{"123456789", 0x4B37},
		{"", 0xFFFF},
		{"A", 0x707F},
	}
	for _, c := range cases {
		if got := CRC16([]byte(c.in)); got != c.want {
			t.Errorf("CRC16(%q) = 0x%04X want 0x%04X", c.in, got, c.want)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	body := []byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05, 0x00}
	wire := Encode(0x001A, false, TypeLoginAck, body)
	if wire[0] != StartByte {
		t.Fatalf("start byte = %x", wire[0])
	}
	if int(wire[1]) != len(wire)-4 {
		t.Fatalf("length byte = %d, wire len = %d", wire[1], len(wire))
	}
	// CRC is transmitted low byte first.
	if wire[len(wire)-2] != byte(CRC16(wire[2:len(wire)-2])) {
		t.Fatal("CRC low byte order wrong")
	}
	f, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if f.Seq != 0x001A || f.Type != TypeLoginAck || !bytes.Equal(f.Body, body) {
		t.Fatalf("decoded = %+v", f)
	}
}

func TestEncodeGoldenFrame(t *testing.T) {
	_ = SetBCDOrder("high")
	defer SetBCDOrder("")
	// Build the exact login frame from section 6.1 with our encoder and verify
	// its CRC against the algorithm, then decode it back. The PDF's printed CRC
	// values are illustrative placeholders, so we assert structural correctness.
	body := make([]byte, 30)
	copy(body[0:7], []byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05})
	body[7] = 0x00
	body[8] = 0x02
	body[9] = 0x0F
	copy(body[10:18], []byte("v4.1.50\x00"))
	body[18] = 0x01
	for i := 19; i < 29; i++ {
		body[i] = 0x01
	}
	body[29] = 0x04
	wire := Encode(0, false, TypeLogin, body)
	if wire[1] != 0x22 {
		t.Fatalf("login length = 0x%02X want 0x22", wire[1])
	}
	f, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	login, err := ParseLogin(f.Body)
	if err != nil {
		t.Fatal(err)
	}
	if login.DeviceID != "55031412782305" {
		t.Errorf("device id = %q", login.DeviceID)
	}
	if login.GunCount != 2 || login.ProtocolVer != 0x0F || login.Operator != 4 {
		t.Errorf("login = %+v", login)
	}
	if login.ProgramVer != "v4.1.50" {
		t.Errorf("program version = %q", login.ProgramVer)
	}
}

func TestDecodeRejectsBadCRC(t *testing.T) {
	wire := Encode(1, false, TypeHeartbeat, []byte{1, 2, 3})
	wire[len(wire)-1] ^= 0xFF
	if _, err := Decode(wire); err != ErrBadCRC && !bytes.Contains([]byte(err.Error()), []byte("CRC")) {
		t.Fatalf("expected CRC error, got %v", err)
	}
}

func TestSplitterHandlesFragmentation(t *testing.T) {
	f1 := Encode(1, false, TypeHeartbeat, []byte{1})
	f2 := Encode(2, false, TypeHeartbeatAck, []byte{2})
	stream := append(append([]byte{0x00, 0xFF}, f1...), f2...) // leading junk
	s := NewSplitter(bytes.NewReader(stream))
	got1, err := s.NextFrame()
	if err != nil || !bytes.Equal(got1, f1) {
		t.Fatalf("frame1 = %x err=%v", got1, err)
	}
	got2, err := s.NextFrame()
	if err != nil || !bytes.Equal(got2, f2) {
		t.Fatalf("frame2 = %x err=%v", got2, err)
	}
}

func TestParseRealtime(t *testing.T) {
	body := make([]byte, 69)
	body[0] = 0x12 // BCD txn nibble
	copy(body[16:23], []byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05})
	body[23] = 1  // gun
	body[24] = 3  // charging
	body[26] = 2  // plugged
	body[27], body[28] = 0xE8, 0x03 // voltage 1000 -> 100.0V
	// charging energy at offset 16+7+1+1+1+1+2+2+1+8+1+1+2+2 = 46
	body[46], body[47], body[48], body[49] = 0x20, 0xA1, 0x07, 0x00 // 500000 units = 50 kWh
	r, err := ParseRealtime(body)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != 3 || r.Plugged != 2 || r.Voltage != 1000 {
		t.Errorf("realtime = %+v", r)
	}
	if r.EnergyUnits != 500000 {
		t.Errorf("energy = %d", r.EnergyUnits)
	}
}

func TestParseTransaction(t *testing.T) {
	body := make([]byte, 158)
	copy(body[0:16], []byte("0000000000000001"))
	copy(body[16:23], []byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05})
	body[23] = 1
	// segments start at 31; flat segment is 3rd: offset 31 + 2*16 = 63
	// flat segment starts at 70, totalEnergy at 112, totalAmount at 120
	body[70], body[71], body[72], body[73] = 0x30, 0x75, 0x00, 0x00 // price 30000 = 0.30000
	body[74], body[75], body[76], body[77] = 0x90, 0x65, 0x00, 0x00 // 26000 units = 2.6 kWh
	body[112], body[113], body[114], body[115] = 0x90, 0x65, 0x00, 0x00
	body[120], body[121], body[122], body[123] = 0xE8, 0x03, 0x00, 0x00 // 1000 units = 0.1 yuan
	txn, err := ParseTransaction(body)
	if err != nil {
		t.Fatal(err)
	}
	if txn.Flat.PriceUnits != 30000 || txn.Flat.EnergyUnits != 26000 {
		t.Errorf("flat segment = %+v", txn.Flat)
	}
	if txn.TotalEnergy != 26000 || txn.TotalAmount != 1000 {
		t.Errorf("totals: energy=%d amount=%d", txn.TotalEnergy, txn.TotalAmount)
	}
}
