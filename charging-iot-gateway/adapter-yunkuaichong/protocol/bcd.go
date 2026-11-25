package protocol

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// BCDOrder controls how two decimal digits map into one byte's nibbles.
// The spec sample (55 03 14 ... -> "550314...") is HIGH_FIRST, but some
// real piles wire the nibbles the other way round (30 -> "03", not "30").
type BCDOrder int

const (
	BCDHighFirst BCDOrder = iota // 0x55 -> "55", 0x03 -> "03"
	BCDLowFirst                  // 0x30 -> "03", 0x41 -> "14"
)

// Default matches the real piles observed in production; the spec sample
// remains available via -bcd-order=high (and the built-in simulator uses it).
var currentBCDOrder atomic.Int32

func init() { currentBCDOrder.Store(int32(BCDLowFirst)) }

// SetBCDOrder configures decoding/encoding globally. Call once at startup.
func SetBCDOrder(name string) error {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "low", "low-first", "lo":
		currentBCDOrder.Store(int32(BCDLowFirst))
	case "high", "high-first", "hi":
		currentBCDOrder.Store(int32(BCDHighFirst))
	default:
		return fmt.Errorf("ykc: unknown bcd order %q (want low|high)", name)
	}
	return nil
}

func CurrentBCDOrder() BCDOrder { return BCDOrder(currentBCDOrder.Load()) }

// splitNibbles returns the two digits of one BCD byte in configured order.
func splitNibbles(v byte) (byte, byte) {
	hi, lo := (v>>4)&0x0F, v&0x0F
	if CurrentBCDOrder() == BCDLowFirst {
		return lo, hi
	}
	return hi, lo
}

// joinNibbles is the inverse of splitNibbles.
func joinNibbles(a, b byte) byte {
	if CurrentBCDOrder() == BCDLowFirst {
		return (b << 4) | a
	}
	return (a << 4) | b
}

// DecodeBCD decodes BCD bytes using the configured nibble order.
func DecodeBCD(b []byte) string {
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		a, c := splitNibbles(v)
		if a <= 9 {
			out = append(out, '0'+a)
		}
		if c <= 9 {
			out = append(out, '0'+c)
		}
	}
	return string(out)
}

// DeviceBCD encodes a device id into the 7-byte BCD form. It is the exact
// inverse of DecodeBCD under the same configured order.
func DeviceBCD(id string) []byte {
	out := make([]byte, 7)
	for i := 0; i < 7; i++ {
		hi, lo := byte(0), byte(0)
		if p := i * 2; p < len(id) && id[p] >= '0' && id[p] <= '9' {
			hi = id[p] - '0'
		}
		if p := i*2 + 1; p < len(id) && id[p] >= '0' && id[p] <= '9' {
			lo = id[p] - '0'
		}
		out[i] = joinNibbles(hi, lo)
	}
	return out
}
