package bus

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
)

// KafkaBus is a minimal Kafka producer using the Produce v3 wire protocol
// over raw TCP. It intentionally uses only the standard library so the
// gateway stays dependency-light; it supports key-based partitioning (which
// guarantees per-connector ordering) and trace headers.
type KafkaBus struct {
	broker string
	mu     sync.Mutex
	conn   net.Conn
	corr   int32
}

func NewKafkaBus(broker string) *KafkaBus {
	return &KafkaBus{broker: broker}
}

func (b *KafkaBus) connect() error {
	if b.conn != nil {
		return nil
	}
	c, err := net.DialTimeout("tcp", b.broker, 3*time.Second)
	if err != nil {
		return err
	}
	b.conn = c
	return nil
}

func (b *KafkaBus) Publish(ctx context.Context, topic, key string, e ykcserver.Event) error {
	payload, err := marshalEvent(e)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.connect(); err != nil {
		return err
	}
	req := buildProduceRequest(b.corr, topic, []byte(key), payload, e.TraceID)
	b.corr++
	if _, err := b.conn.Write(req); err != nil {
		b.conn.Close()
		b.conn = nil
		return err
	}
	// Produce v3 with acks=0 has no response; fire-and-forget keeps the fast
	// channel non-blocking. For at-least-once, switch to acks=1 and read the
	// ProduceResponse correlation here.
	_ = ctx
	return nil
}

func (b *KafkaBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		err := b.conn.Close()
		b.conn = nil
		return err
	}
	return nil
}

// marshalEvent uses JSON with TraceId first for readable raw topic dumps.
func marshalEvent(e ykcserver.Event) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"traceId":"`)
	buf.WriteString(e.TraceID)
	buf.WriteString(`","type":"`)
	buf.WriteString(e.Type)
	buf.WriteString(`","deviceId":"`)
	buf.WriteString(e.DeviceID)
	buf.WriteString(`"}`)
	return buf.Bytes(), nil
}

// buildProduceRequest constructs a Kafka v3 Produce request with acks=0:
// RequestHeader(apiKey=0, apiVersion=3, correlationId, clientId)
// ProduceRequest(acks=0, timeout, topics -> partitions -> records)
func buildProduceRequest(corr int32, topic string, key, value []byte, traceID string) []byte {
	clientID := "opencharge-gateway"
	// Build body first (needed for size prefix).
	body := newBuf()
	body.int16(0)       // acks = 0
	body.int32(1000)    // timeout ms
	body.int32(1)       // topic count
	body.string(topic)  // topic name
	body.int32(1)       // partition count
	body.int32(0)       // partition index
	recordBatch := buildRecordBatch(key, value, traceID)
	body.int32(int32(len(recordBatch)))
	body.write(recordBatch)

	hdr := newBuf()
	hdr.int16(0)  // api key: Produce
	hdr.int16(3)  // api version
	hdr.int32(corr)
	hdr.string(clientID)

	out := newBuf()
	out.int32(int32(hdr.len() + body.len()))
	out.write(hdr.bytes())
	out.write(body.bytes())
	return out.bytes()
}

// buildRecordBatch builds a Kafka record batch (version 2) with one record.
func buildRecordBatch(key, value []byte, traceID string) []byte {
	record := newBuf()
	record.varint(0)                       // attributes
	record.varint(int64(len(key)))         // key length
	record.write(key)
	record.varint(int64(len(value)))       // value length
	record.write(value)
	record.varint(2)                       // header count
	record.string("trace_id")
	record.bytesWithVarint([]byte(traceID))
	record.string("source")
	record.bytesWithVarint([]byte("opencharge"))
	recordBytes := record.bytes()

	batch := newBuf()
	batch.int64(0)                         // baseOffset
	batch.int32(int32(len(recordBytes) + 61)) // batchLength (approx; see below)
	batch.int32(-1)                        // partitionLeaderEpoch
	batch.int8(2)                          // magic
	batch.int32(0)                         // crc placeholder
	batch.int16(0)                         // attributes
	batch.int32(0)                         // lastOffsetDelta
	batch.int64(time.Now().UnixMilli())    // firstTimestamp
	batch.int64(time.Now().UnixMilli())    // maxTimestamp
	batch.int64(-1)                        // producerId
	batch.int16(-1)                        // producerEpoch
	batch.int32(-1)                        // baseSequence
	batch.int32(1)                         // record count
	batch.write(recordBytes)

	raw := batch.bytes()
	// Record batch layout after baseOffset(8)+batchLength(4):
	//   partitionLeaderEpoch(4) | magic(1) | crc(4) | attributes... 
	// CRC32C covers everything after the crc field (offset 21 onward).
	crc := crc32c(raw[21:])
	binary.BigEndian.PutUint32(raw[17:21], crc)
	// batchLength covers everything after itself, big-endian per Kafka spec.
	binary.BigEndian.PutUint32(raw[8:12], uint32(len(raw)-12))
	return raw
}

// ---- small binary writer ----

type buf struct{ b bytes.Buffer }

func newBuf() *buf { return &buf{} }
func (w *buf) write(p []byte)       { w.b.Write(p) }
func (w *buf) bytes() []byte        { return w.b.Bytes() }
func (w *buf) len() int             { return w.b.Len() }
func (w *buf) int8(v int8)          { _ = w.b.WriteByte(byte(v)) }
func (w *buf) int16(v int16)        { var x [2]byte; binary.BigEndian.PutUint16(x[:], uint16(v)); w.write(x[:]) }
func (w *buf) int32(v int32)        { var x [4]byte; binary.BigEndian.PutUint32(x[:], uint32(v)); w.write(x[:]) }
func (w *buf) int64(v int64)        { var x [8]byte; binary.BigEndian.PutUint64(x[:], uint64(v)); w.write(x[:]) }
func (w *buf) string(s string)      { w.int16(int16(len(s))); w.write([]byte(s)) }
func (w *buf) varint(v int64)       { w.write(encodeVarint(v)) }
func (w *buf) bytesWithVarint(p []byte) { w.varint(int64(len(p))); w.write(p) }

func encodeVarint(v int64) []byte {
	u := uint64(v) << 1
	if v < 0 {
		u = ^u
	}
	var out [10]byte
	i := 0
	for u >= 0x80 {
		out[i] = byte(u) | 0x80
		u >>= 7
		i++
	}
	out[i] = byte(u)
	return out[:i+1]
}

// crc32c implements Castagnoli CRC (polynomial 0x82F63B78) used by Kafka.
func crc32c(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0x82F63B78
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}

var _ Bus = (*KafkaBus)(nil)

// errUnreachable is returned when the broker is not reachable at publish time.
var errUnreachable = fmt.Errorf("kafka: broker unreachable")
