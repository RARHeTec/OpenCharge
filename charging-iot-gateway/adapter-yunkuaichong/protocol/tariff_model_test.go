package protocol

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestBuildTariffSetBodyMatchesSpecSample(t *testing.T) {
	_ = SetBCDOrder("high")
	defer SetBCDOrder("")
	// Spec 9.5 sample: device 55031412782305, model 0100, rates and 48 slots.
	m := TariffModel{
		ModelCode: 1,
		Rates: TariffRates{
			SharpElec: 200000, SharpService: 16540,
			PeakElec: 300000, PeakService: 16540,
			FlatElec: 400000, FlatService: 16540,
			ValleyElec: 500000, ValleyService: 16540,
		},
		LossRatio: 0,
	}
	// Sample body uses 0x00 for every slot.
	body, err := BuildTariffSetBody([]byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05}, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 90 {
		t.Fatalf("body length = %d, want 90", len(body))
	}
	got := hex.EncodeToString(body)
	// Verify the field boundaries from the spec sample.
	checks := []struct{ at int; want string; label string }{
		{0, "55031412782305", "device BCD"},
		{7, "0100", "model code"},
		{9, "400d0300", "sharp elec 2.00000"},
		{13, "9c400000", "sharp service 0.16540"},
		{17, "e0930400", "peak elec 3.00000"},
		{33, "20a10700", "valley elec 5.00000"},
		{37, "9c400000", "valley service"},
		{41, "00", "loss ratio"},
	}
	for _, c := range checks {
		part := got[c.at*2 : (c.at+len(c.want)/2)*2]
		if part != c.want {
			t.Errorf("%s = %s, want %s", c.label, part, c.want)
		}
	}
	if !strings.HasSuffix(got, strings.Repeat("00", 48)) {
		t.Errorf("48 slots should all be zero in this sample, tail=%s", got[len(got)-24:])
	}
}

func TestTariffModelRoundTrip(t *testing.T) {
	_ = SetBCDOrder("high")
	defer SetBCDOrder("")
	m := TariffModel{ModelCode: 12}
	m.Rates = TariffRates{60000, 18000, 88000, 15000, 119500, 9500, 60000, 18000}
	m.LossRatio = 2
	for i := range m.Slots {
		m.Slots[i] = byte(i % 4)
	}
	body, err := BuildTariffSetBody([]byte{1, 2, 3, 4, 5, 6, 7}, m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseTariffModel(body)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelCode != 12 {
		t.Errorf("model code = %d", got.ModelCode)
	}
	if got.Rates != m.Rates {
		t.Errorf("rates = %+v", got.Rates)
	}
	if got.LossRatio != 2 {
		t.Errorf("loss = %d", got.LossRatio)
	}
	if got.Slots != m.Slots {
		t.Errorf("slots mismatch")
	}
}

func TestTariffSetAckCodec(t *testing.T) {
	_ = SetBCDOrder("high")
	defer SetBCDOrder("")
	dev := []byte{0x55, 0x03, 0x14, 0x12, 0x78, 0x23, 0x05}
	body := BuildTariffSetAckBody(dev, true)
	if len(body) != 8 || body[7] != 0x01 {
		t.Fatalf("ack body = %X", body)
	}
	id, ok, err := ParseTariffSetAck(body)
	if err != nil || !ok || id != "55031412782305" {
		t.Fatalf("parse ack = %s %v %v", id, ok, err)
	}
	fail := BuildTariffSetAckBody(dev, false)
	_, ok, _ = ParseTariffSetAck(fail)
	if ok {
		t.Error("failure ack parsed as success")
	}
}

func TestTariffBodyRejectsShortDevice(t *testing.T) {
	_ = SetBCDOrder("high")
	defer SetBCDOrder("")
	if _, err := BuildTariffSetBody([]byte{1, 2}, TariffModel{}); err == nil {
		t.Error("expected error for short device BCD")
	}
}

func TestDeviceBCDRoundTrip(t *testing.T) {
	_ = SetBCDOrder("high")
	defer SetBCDOrder("")
	const id = "55031412782305"
	b := DeviceBCD(id)
	if got := DecodeBCD(b); got != id {
		t.Fatalf("DecodeBCD(DeviceBCD(%s)) = %s", id, got)
	}
	want := "55031412782305"
	if hex.EncodeToString(b) != want {
		t.Errorf("BCD = %X, want %s", b, want)
	}
}
