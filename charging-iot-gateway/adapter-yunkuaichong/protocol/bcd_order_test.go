package protocol

import (
	"encoding/hex"
	"testing"
)

func TestRealPileLowFirstHeartbeat(t *testing.T) {
	// The raw device-id bytes from the operator's real pile heartbeat:
	// 55 30 41 21 87 32 10 must decode to the registry id 55031412782301.
	if err := SetBCDOrder("low"); err != nil {
		t.Fatal(err)
	}
	defer SetBCDOrder("")

	got := DecodeBCD([]byte{0x55, 0x30, 0x41, 0x21, 0x87, 0x32, 0x10})
	if got != "55031412782301" {
		t.Fatalf("low-first decode = %s, want 55031412782301", got)
	}
	// Encoding must be symmetric so downlinks carry the same byte order.
	b := DeviceBCD(got)
	if hex.EncodeToString(b) != "55304121873210" {
		t.Errorf("low-first encode = %X", b)
	}
}

func TestSpecSampleHighFirst(t *testing.T) {
	// The protocol sample uses high-nibble-first; the built-in simulator does too.
	if err := SetBCDOrder("high"); err != nil {
		t.Fatal(err)
	}
	defer SetBCDOrder("")

	got := DecodeBCD([]byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05})
	if got != "55031412782305" {
		t.Fatalf("high-first decode = %s", got)
	}
	if hex.EncodeToString(DeviceBCD(got)) != "55031412782305" {
		t.Errorf("high-first encode mismatch")
	}
}

func TestSetBCDOrderValidation(t *testing.T) {
	if err := SetBCDOrder("bogus"); err == nil {
		t.Error("expected error for unknown order")
	}
	for _, name := range []string{"low", "high", "LOW-FIRST", "hi", ""} {
		if err := SetBCDOrder(name); err != nil {
			t.Errorf("%q rejected: %v", name, err)
		}
	}
	_ = SetBCDOrder("")
}

func TestParseHeartbeatUsesConfiguredOrder(t *testing.T) {
	_ = SetBCDOrder("low")
	defer SetBCDOrder("")

	// 0x2B heartbeat body: 7-byte device BCD + gun + status.
	body := append([]byte{0x55, 0x30, 0x41, 0x21, 0x87, 0x32, 0x10}, 1, 0)
	hb, err := ParseHeartbeat(body)
	if err != nil {
		t.Fatal(err)
	}
	if hb.DeviceID != "55031412782301" {
		t.Errorf("heartbeat device = %s", hb.DeviceID)
	}
}
