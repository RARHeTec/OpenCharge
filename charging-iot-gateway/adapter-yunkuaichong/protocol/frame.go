// Package protocol implements the YunKuaiChong V1.6 application-layer frame.
//
// Frame layout (per section 3.1 of the protocol spec):
//   +--------+--------+--------+--------+--------+---------+--------+
//   | start  | length |  seq   |  enc   |  type  |  body   |  crc   |
//   | 0x68   | 1 byte | 2 byte | 1 byte | 1 byte | N bytes | 2 byte |
//   +--------+--------+--------+--------+--------+---------+--------+
// length = bytes of (seq + enc + type + body), max 200.
// CRC is standard CRC-16/Modbus (init 0xFFFF, poly 0xA001), low byte first.
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	StartByte  = 0x68
	MaxBodyLen = 200
)

var (
	ErrBadStart   = errors.New("ykc: invalid start byte")
	ErrBadCRC     = errors.New("ykc: CRC mismatch")
	ErrBadLength  = errors.New("ykc: invalid length")
	ErrTooShort   = errors.New("ykc: frame too short")
)

// Frame is one decoded application-layer frame.
type Frame struct {
	Seq      uint16
	Encrypted bool
	Type     byte
	Body     []byte
}

// Encode builds a complete wire frame (including start, length and CRC).
func Encode(seq uint16, encrypted bool, typ byte, body []byte) []byte {
	enc := byte(0)
	if encrypted {
		enc = 1
	}
	dataLen := 2 + 1 + 1 + len(body) // seq + enc + type + body
	out := make([]byte, 0, 1+1+dataLen+2)
	out = append(out, StartByte)
	out = append(out, byte(dataLen))
	var seqBytes [2]byte
	binary.LittleEndian.PutUint16(seqBytes[:], seq)
	out = append(out, seqBytes[:]...)
	out = append(out, enc, typ)
	out = append(out, body...)
	crc := CRC16(out[2:]) // CRC covers seq..body
	out = append(out, byte(crc&0xFF), byte(crc>>8)) // low byte first
	return out
}

// Decode validates and decodes a complete wire frame.
func Decode(wire []byte) (Frame, error) {
	if len(wire) < 7 {
		return Frame{}, ErrTooShort
	}
	if wire[0] != StartByte {
		return Frame{}, ErrBadStart
	}
	dataLen := int(wire[1])
	if dataLen < 4 || dataLen > MaxBodyLen {
		return Frame{}, ErrBadLength
	}
	total := 1 + 1 + dataLen + 2
	if len(wire) < total {
		return Frame{}, ErrTooShort
	}
	data := wire[2 : 2+dataLen]
	gotCRC := binary.LittleEndian.Uint16(wire[2+dataLen : total])
	if CRC16(data) != gotCRC {
		return Frame{}, fmt.Errorf("%w: got 0x%04X want 0x%04X", ErrBadCRC, gotCRC, CRC16(data))
	}
	return Frame{
		Seq:       binary.LittleEndian.Uint16(data[0:2]),
		Encrypted: data[2] != 0,
		Type:      data[3],
		Body:      append([]byte(nil), data[4:]...),
	}, nil
}

// FrameLen returns the total wire length from the length byte, or 0 if invalid.
// Used by the stream splitter to know how many bytes to read next.
func FrameLen(header []byte) (int, error) {
	if len(header) < 2 {
		return 0, ErrTooShort
	}
	if header[0] != StartByte {
		return 0, ErrBadStart
	}
	n := int(header[1])
	if n < 4 || n > MaxBodyLen {
		return 0, ErrBadLength
	}
	return 1 + 1 + n + 2, nil
}
