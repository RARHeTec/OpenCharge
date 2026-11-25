package protocol

// DeviceBCD encodes a device id into the 7-byte BCD form used by the spec:
// two decimal digits per byte, high nibble first. This is the exact inverse
// of DecodeBCD (sample: "55031412782305" -> 55 03 14 12 78 23 05).
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
		out[i] = (hi << 4) | lo
	}
	return out
}
