package protocol

// CRC16 computes CRC-16/Modbus (reflected, init 0xFFFF, polynomial 0xA001).
// This matches the table-based ModbusCRC function in appendix 13.2 of the
// YunKuaiChong V1.6 spec. Bitwise implementation avoids a 512-byte table.
func CRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
