package protocol

import (
	"encoding/binary"
	"fmt"
	"time"
)

// CP56Time2a is the 7-byte IEC 60870-5-4 timestamp used by the YunKuaiChong
// transaction record for charge start and end times.
//
//	byte 0-1: milliseconds (little endian, 0..59999)
//	byte 2  : minute   (bit7 = IV, invalid)
//	byte 3  : hour     (bit7 = IV, bit6 = SU summer time)
//	byte 4  : day of month (bits 0-4) + day of week (bits 5-7)
//	byte 5  : month    (bits 0-3)
//	byte 6  : year     (0..99, offset from 2000)
//
// The device reports wall-clock local time, so the result is built in loc.
func ParseCP56Time2a(b []byte, loc *time.Location) (time.Time, error) {
	if len(b) < 7 {
		return time.Time{}, fmt.Errorf("cp56: need 7 bytes, got %d", len(b))
	}
	ms := int(binary.LittleEndian.Uint16(b[0:2]))
	minute := int(b[2] & 0x3F)
	hour := int(b[3] & 0x1F)
	day := int(b[4] & 0x1F)
	month := int(b[5] & 0x0F)
	year := 2000 + int(b[6]&0x7F)

	if month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("cp56: invalid month %d", month)
	}
	if day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("cp56: invalid day %d", day)
	}
	if hour > 23 || minute > 59 {
		return time.Time{}, fmt.Errorf("cp56: invalid time %02d:%02d", hour, minute)
	}
	if ms > 59999 {
		ms = ms % 60000
	}
	if loc == nil {
		loc = time.Local
	}
	return time.Date(year, time.Month(month), day, hour, minute, ms/1000,
		(ms%1000)*int(time.Millisecond), loc), nil
}

// EncodeCP56Time2a is the inverse, used by the simulator and tests.
func EncodeCP56Time2a(t time.Time) []byte {
	out := make([]byte, 7)
	ms := t.Second()*1000 + t.Nanosecond()/int(time.Millisecond)
	binary.LittleEndian.PutUint16(out[0:2], uint16(ms))
	out[2] = byte(t.Minute())
	out[3] = byte(t.Hour())
	out[4] = byte(t.Day()) | byte(int(t.Weekday())<<5)
	out[5] = byte(int(t.Month()))
	out[6] = byte(t.Year() - 2000)
	return out
}
