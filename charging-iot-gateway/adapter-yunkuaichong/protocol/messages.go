package protocol

import "encoding/binary"

// BCD decoding: each byte holds two decimal digits (low nibble first per spec
// convention for BCD-encoded device IDs; sample 55 03 14 12 78 23 05 -> "55031412782305").
func DecodeBCD(b []byte) string {
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		hi, lo := (v>>4)&0x0F, v&0x0F
		if hi <= 9 {
			out = append(out, '0'+hi)
		}
		if lo <= 9 {
			out = append(out, '0'+lo)
		}
	}
	return string(out)
}

// LoginBody parses the 0x01 login frame body.
type LoginBody struct {
	DeviceID     string
	PileType     byte // 0 DC, 1 AC
	GunCount     byte
	ProtocolVer  byte
	ProgramVer   string
	NetworkType  byte
	SIM          string
	Operator     byte
}

func ParseLogin(body []byte) (LoginBody, error) {
	if len(body) < 30 {
		return LoginBody{}, ErrBadLength
	}
	return LoginBody{
		DeviceID:    DecodeBCD(body[0:7]),
		PileType:    body[7],
		GunCount:    body[8],
		ProtocolVer: body[9],
		ProgramVer:  string(trimZero(body[10:18])),
		NetworkType: body[18],
		SIM:         DecodeBCD(body[19:29]),
		Operator:    body[29],
	}, nil
}

func trimZero(b []byte) []byte {
	n := len(b)
	for n > 0 && b[n-1] == 0 {
		n--
	}
	return b[:n]
}

// LoginAckBody builds the 0x02 response body.
func LoginAckBody(deviceBCD []byte, success bool) []byte {
	result := byte(1)
	if success {
		result = 0
	}
	out := make([]byte, 0, 8)
	out = append(out, deviceBCD...)
	out = append(out, result)
	return out
}

// HeartbeatBody parses 0x03 body: 7-byte device BCD + gun + status.
type HeartbeatBody struct {
	DeviceID string
	GunNo    byte
	Status   byte // 0 normal, 1 fault
}

func ParseHeartbeat(body []byte) (HeartbeatBody, error) {
	if len(body) < 9 {
		return HeartbeatBody{}, ErrBadLength
	}
	return HeartbeatBody{DeviceID: DecodeBCD(body[0:7]), GunNo: body[7], Status: body[8]}, nil
}

func HeartbeatAckBody(deviceBCD []byte, gun byte) []byte {
	out := make([]byte, 0, 9)
	out = append(out, deviceBCD...)
	out = append(out, gun, 0)
	return out
}

// RealtimeBody parses 0x13 telemetry body (69 bytes).
// Field offsets follow section 7.2; all numerics little-endian BIN.
type RealtimeBody struct {
	TransactionID string
	DeviceID      string
	GunNo         byte
	State         byte // 0 offline 1 fault 2 idle 3 charging
	Returned      byte
	Plugged       byte
	Voltage       uint16 // 0.1 V
	Current       uint16 // 0.1 A
	GunTemp       byte   // offset -50
	ChargeTimeMin uint16
	RemainMin     uint16
	EnergyUnits   uint32 // 0.0001 kWh
	LossUnits     uint32
	AmountUnits   uint32 // 0.0001 yuan
	Fault         uint16
}

func ParseRealtime(body []byte) (RealtimeBody, error) {
	// txn(16) + device(7) + gun(1) + state(1) + returned(1) + plugged(1)
	// + volt(2) + curr(2) + temp(1) + gunCode(8) + soc(1) + battTemp(1)
	// + chargeMin(2) + remainMin(2) + energy(4) + loss(4) + amount(4) + fault(2)
	const need = 16 + 7 + 1 + 1 + 1 + 1 + 2 + 2 + 1 + 8 + 1 + 1 + 2 + 2 + 4 + 4 + 4 + 2
	if len(body) < need {
		return RealtimeBody{}, ErrBadLength
	}
	o := 0
	r := RealtimeBody{}
	r.TransactionID = DecodeBCD(body[o : o+16]); o += 16
	r.DeviceID = DecodeBCD(body[o : o+7]); o += 7
	r.GunNo = body[o]; o++
	r.State = body[o]; o++
	r.Returned = body[o]; o++
	r.Plugged = body[o]; o++
	r.Voltage = binary.LittleEndian.Uint16(body[o : o+2]); o += 2
	r.Current = binary.LittleEndian.Uint16(body[o : o+2]); o += 2
	r.GunTemp = body[o]; o++
	o += 8 // gun code
	o += 2 // soc + battery temp
	r.ChargeTimeMin = binary.LittleEndian.Uint16(body[o : o+2]); o += 2
	r.RemainMin = binary.LittleEndian.Uint16(body[o : o+2]); o += 2
	r.EnergyUnits = binary.LittleEndian.Uint32(body[o : o+4]); o += 4
	r.LossUnits = binary.LittleEndian.Uint32(body[o : o+4]); o += 4
	r.AmountUnits = binary.LittleEndian.Uint32(body[o : o+4]); o += 4
	r.Fault = binary.LittleEndian.Uint16(body[o : o+2])
	return r, nil
}

// TransactionBody parses the 0x3B settlement record (section 8.7).
type PriceSegment struct {
	PriceUnits   uint32 // 0.00001 yuan
	EnergyUnits  uint32 // 0.0001 kWh
	LossUnits    uint32
	AmountUnits  uint32 // 0.0001 yuan
}

type TransactionBody struct {
	TransactionID string
	DeviceID      string
	GunNo         byte
	StartCP56     []byte
	EndCP56       []byte
	Sharp         PriceSegment
	Peak          PriceSegment
	Flat          PriceSegment
	Valley        PriceSegment
	TotalEnergy   uint32
	TotalLoss     uint32
	TotalAmount   uint32
	TradeFlag     byte
	StopReason    byte
}

func ParseTransaction(body []byte) (TransactionBody, error) {
	const need = 158 // txn16 + dev7 + gun1 + 2xCP56(14) + 4segsx16(64) + meter10 + totals12 + VIN17 + flag1 + time7 + reason1 + card8
	if len(body) < need {
		return TransactionBody{}, ErrBadLength
	}
	o := 0
	t := TransactionBody{}
	t.TransactionID = DecodeBCD(body[o : o+16]); o += 16
	t.DeviceID = DecodeBCD(body[o : o+7]); o += 7
	t.GunNo = body[o]; o++
	t.StartCP56 = append([]byte(nil), body[o:o+7]...); o += 7
	t.EndCP56 = append([]byte(nil), body[o:o+7]...); o += 7
	readSeg := func() PriceSegment {
		s := PriceSegment{
			PriceUnits:  binary.LittleEndian.Uint32(body[o : o+4]),
			EnergyUnits: binary.LittleEndian.Uint32(body[o+4 : o+8]),
			LossUnits:   binary.LittleEndian.Uint32(body[o+8 : o+12]),
			AmountUnits: binary.LittleEndian.Uint32(body[o+12 : o+16]),
		}
		o += 16
		return s
	}
	t.Sharp = readSeg()
	t.Peak = readSeg()
	t.Flat = readSeg()
	t.Valley = readSeg()
	o += 10 // meter start+end (5+5)
	t.TotalEnergy = binary.LittleEndian.Uint32(body[o : o+4]); o += 4
	t.TotalLoss = binary.LittleEndian.Uint32(body[o : o+4]); o += 4
	t.TotalAmount = binary.LittleEndian.Uint32(body[o : o+4]); o += 4
	o += 17 // VIN
	t.TradeFlag = body[o]; o++
	o += 7 // trade time
	t.StopReason = body[o]
	return t, nil
}

func TransactionAckBody(txnBCD []byte, ok bool) []byte {
	result := byte(1)
	if ok {
		result = 0
	}
	out := make([]byte, 0, 17)
	out = append(out, txnBCD...)
	return append(out, result)
}
