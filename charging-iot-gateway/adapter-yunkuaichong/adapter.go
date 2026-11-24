// Package yunkuaichong implements the cloud-charging binary protocol (1.5/1.6).
// Frame layout (little endian): 0xAA 0x55 | ver | cmd | len | body | CRC16-CCITT
package yunkuaichong

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/RARHeTec/OpenCharge/charging-common/trace"
	model "github.com/RARHeTec/OpenCharge/charging-thing-model/event"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/protocol-spi"
)

const (
	header1 = 0xAA
	header2 = 0x55
)

type Adapter struct {
	version string // "1.5" or "1.6"
}

func New(version string) *Adapter { return &Adapter{version: version} }

func (a *Adapter) Code() string { return "yunkuaichong-" + a.version }

func (a *Adapter) Decode(ctx context.Context, deviceID string, frame []byte) ([]model.Envelope, error) {
	if len(frame) < 8 || frame[0] != header1 || frame[1] != header2 {
		return nil, fmt.Errorf("yunkuaichong: invalid frame header")
	}
	cmd := binary.LittleEndian.Uint16(frame[4:6])
	body := frame[8 : len(frame)-2]

	env := model.NewEnvelope(model.Type("YunKuaiChong"), deviceID, int(body[0]))
	env.TraceID = trace.FromContext(ctx)
	env.Timestamp = int64(binary.LittleEndian.Uint32(body[1:5]))
	env.Payload = body

	// Map native command codes to unified events.
	switch cmd {
	case 0x01:
		env.Type = model.TypeBootNotification
	case 0x02:
		env.Type = model.TypeHeartbeat
	case 0x05:
		env.Type = model.TypeStatusNotification
	case 0x06:
		env.Type = model.TypeMeterValues
	case 0x09:
		env.Type = model.TypeStopTransaction
	default:
		return nil, nil
	}
	return []model.Envelope{env}, nil
}

func (a *Adapter) Encode(ctx context.Context, cmd model.Envelope) ([]byte, error) {
	// Native command mapping will be filled in during protocol integration.
	return []byte{header1, header2, 0x01, 0x00, 0x00, 0x00}, nil
}

func (a *Adapter) Heartbeat(deviceID string) []byte {
	return []byte{header1, header2, 0x01, 0x02, 0x00, 0x00}
}

var _ spi.Adapter = (*Adapter)(nil)
