package bus

import (
	"encoding/binary"
	"testing"
)

func TestProduceRequestWireFormat(t *testing.T) {
	req := buildProduceRequest(42, "charging.telemetry.metervalues.v1", []byte("D1:1"), []byte(`{"type":"MeterValues"}`), "trace-abc")

	// Envelope: 4-byte size followed by request header.
	if len(req) < 4 {
		t.Fatalf("request too short: %d", len(req))
	}
	size := int(binary.BigEndian.Uint32(req[0:4]))
	if size != len(req)-4 {
		t.Fatalf("size field %d != body %d", size, len(req)-4)
	}

	// Request header: apiKey=0 (Produce), apiVersion=3, correlationId=42.
	hdr := req[4:]
	if got := int16(binary.BigEndian.Uint16(hdr[0:2])); got != 0 {
		t.Errorf("api key = %d", got)
	}
	if got := int16(binary.BigEndian.Uint16(hdr[2:4])); got != 3 {
		t.Errorf("api version = %d", got)
	}
	if got := int32(binary.BigEndian.Uint32(hdr[4:8])); got != 42 {
		t.Errorf("correlation id = %d", got)
	}
	// clientId length follows at hdr[8:10].
	clientLen := int(binary.BigEndian.Uint16(hdr[8:10]))
	if clientLen == 0 || clientLen > 100 {
		t.Fatalf("client id length = %d", clientLen)
	}

	// Topic name must appear verbatim in the body.
	if !contains(req, []byte("charging.telemetry.metervalues.v1")) {
		t.Error("topic name missing from request")
	}
	// Trace header must be carried.
	if !contains(req, []byte("trace-abc")) || !contains(req, []byte("trace_id")) {
		t.Error("trace header missing from record batch")
	}
	// Partition key must be present for ordered delivery per connector.
	if !contains(req, []byte("D1:1")) {
		t.Error("partition key missing")
	}
}

func TestRecordBatchLengthAndCRC(t *testing.T) {
	batch := buildRecordBatch([]byte("K"), []byte("V"), "t1")
	if len(batch) < 61 {
		t.Fatalf("batch too short: %d", len(batch))
	}
	// batchLength at offset 8 must equal everything after it.
	length := int(binary.BigEndian.Uint32(batch[8:12]))
	if length != len(batch)-12 {
		t.Errorf("batchLength %d != %d", length, len(batch)-12)
	}
	// magic byte at offset 16 must be 2.
	if batch[16] != 2 {
		t.Errorf("magic = %d", batch[16])
	}
	// CRC32C placeholder was patched to a non-zero value.
	if binary.LittleEndian.Uint32(batch[12:16]) == 0 {
		t.Error("CRC not computed")
	}
}

func TestVarintEncoding(t *testing.T) {
	cases := []struct {
		v    int64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x02}},
		{64, []byte{0x80, 0x01}},
	}
	for _, c := range cases {
		got := encodeVarint(c.v)
		if !equal(got, c.want) {
			t.Errorf("varint(%d) = %x, want %x", c.v, got, c.want)
		}
	}
}

func TestCRC32CKnownVector(t *testing.T) {
	// CRC32C("123456789") = 0xE3069283
	if got := crc32c([]byte("123456789")); got != 0xE3069283 {
		t.Errorf("crc32c = 0x%08X", got)
	}
}

func contains(h, n []byte) bool {
	if len(n) == 0 {
		return true
	}
	for i := 0; i+len(n) <= len(h); i++ {
		if equal(h[i:i+len(n)], n) {
			return true
		}
	}
	return false
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
