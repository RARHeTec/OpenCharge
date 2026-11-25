package protocol

import (
	"encoding/binary"
	"fmt"
)

// TariffRates are the 8 four-byte price fields in a 0x58/0x0A body.
// Prices are 1e-5 Yuan units, little endian (verified against the spec sample:
// 40 0D 03 00 = 200000 = 2.00000 Yuan/kWh).
type TariffRates struct {
	SharpElec, SharpService uint32
	PeakElec, PeakService   uint32
	FlatElec, FlatService   uint32
	ValleyElec, ValleyService uint32
}

// TariffModel is the platform's billing model for a pile.
type TariffModel struct {
	ModelCode uint16     // BCD encoded in the body (e.g. 12 -> 0x12)
	Rates     TariffRates
	LossRatio byte
	// Slots holds 48 half-hour band codes: 0=尖 1=峰 2=平 3=谷.
	Slots [48]byte
}

const TariffBodyLen = 7 + 2 + 8*4 + 1 + 48 // = 90

// BuildTariffSetBody serializes the 0x58 body. deviceBCD must be 7 bytes.
func BuildTariffSetBody(deviceBCD []byte, m TariffModel) ([]byte, error) {
	if len(deviceBCD) != 7 {
		return nil, fmt.Errorf("ykc: device BCD must be 7 bytes, got %d", len(deviceBCD))
	}
	out := make([]byte, 0, TariffBodyLen)
	out = append(out, deviceBCD...)
	// Model code is BCD: 12 decimal -> bytes 0x12, 0x00.
	code := bcdUint16(m.ModelCode)
	out = append(out, code...)
	for _, v := range []uint32{
		m.Rates.SharpElec, m.Rates.SharpService,
		m.Rates.PeakElec, m.Rates.PeakService,
		m.Rates.FlatElec, m.Rates.FlatService,
		m.Rates.ValleyElec, m.Rates.ValleyService,
	} {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		out = append(out, b[:]...)
	}
	out = append(out, m.LossRatio)
	out = append(out, m.Slots[:]...)
	if len(out) != TariffBodyLen {
		return nil, fmt.Errorf("ykc: tariff body length %d, want %d", len(out), TariffBodyLen)
	}
	return out, nil
}

// ParseTariffModel decodes a 0x58/0x0A body (device id is returned raw BCD).
func ParseTariffModel(body []byte) (TariffModel, error) {
	var m TariffModel
	if len(body) < TariffBodyLen {
		return m, fmt.Errorf("ykc: tariff body %d bytes, want %d", len(body), TariffBodyLen)
	}
	o := 7
	m.ModelCode = fromBCDUint16(body[o], body[o+1])
	o += 2
	rates := []*uint32{
		&m.Rates.SharpElec, &m.Rates.SharpService,
		&m.Rates.PeakElec, &m.Rates.PeakService,
		&m.Rates.FlatElec, &m.Rates.FlatService,
		&m.Rates.ValleyElec, &m.Rates.ValleyService,
	}
	for _, p := range rates {
		*p = binary.LittleEndian.Uint32(body[o : o+4])
		o += 4
	}
	m.LossRatio = body[o]
	o++
	copy(m.Slots[:], body[o:o+48])
	return m, nil
}

// BuildTariffSetAckBody is the 0x57 response: 7-byte device BCD + result.
func BuildTariffSetAckBody(deviceBCD []byte, success bool) []byte {
	out := make([]byte, 0, 8)
	out = append(out, deviceBCD...)
	if success {
		out = append(out, 0x01)
	} else {
		out = append(out, 0x00)
	}
	return out
}

// ParseTariffSetAck reads a 0x57 body.
func ParseTariffSetAck(body []byte) (device string, success bool, err error) {
	if len(body) < 8 {
		return "", false, fmt.Errorf("ykc: tariff ack %d bytes, want 8", len(body))
	}
	return DecodeBCD(body[:7]), body[7] == 0x01, nil
}

// bcdUint16 packs a decimal code into two BCD bytes, low byte first on wire.
// Model 1 -> {0x01, 0x00}, model 12 -> {0x12, 0x00}.
func bcdUint16(v uint16) []byte {
	// Model codes use the spec's own layout (low decimal digit in the low
	// nibble), independent of the device-id BCD order setting.
	return []byte{
		byte((v % 10) | ((v / 10 % 10) << 4)),
		byte((v / 100 % 10) | ((v / 1000 % 10) << 4)),
	}
}

func FromBCDUint16(lo, hi byte) uint16 {
	return fromBCDUint16(lo, hi)
}

func fromBCDUint16(b0, b1 byte) uint16 {
	return uint16(b0&0x0F) + uint16((b0>>4)&0x0F)*10 +
		uint16(b1&0x0F)*100 + uint16((b1>>4)&0x0F)*1000
}
