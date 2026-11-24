// Package ocpp16 implements OCPP 1.6J over WebSocket/JSON.
package ocpp16

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/RARHeTec/OpenCharge/charging-common/trace"
	model "github.com/RARHeTec/OpenCharge/charging-thing-model/event"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/protocol-spi"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Code() string { return "ocpp1.6j" }

// CALL = 2, CALLRESULT = 3. Message: [msgType, uniqueId, action, payload]
func (a *Adapter) Decode(ctx context.Context, deviceID string, frame []byte) ([]model.Envelope, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(frame, &raw); err != nil {
		return nil, fmt.Errorf("ocpp16: invalid json: %w", err)
	}
	if len(raw) < 3 {
		return nil, fmt.Errorf("ocpp16: short message")
	}
	var msgType int
	if err := json.Unmarshal(raw[0], &msgType); err != nil || msgType != 2 {
		return nil, nil
	}
	var action string
	if err := json.Unmarshal(raw[2], &action); err != nil {
		return nil, err
	}

	mapping := map[string]model.Type{
		"BootNotification":    model.TypeBootNotification,
		"Heartbeat":           model.TypeHeartbeat,
		"StatusNotification":  model.TypeStatusNotification,
		"MeterValues":         model.TypeMeterValues,
		"StopTransaction":     model.TypeStopTransaction,
		"DiagnosticsStatusNotification": model.TypeDiagnosticsResponse,
	}
	t, ok := mapping[action]
	if !ok {
		return nil, nil
	}
	payload := []byte("{}")
	if len(raw) > 3 {
		payload = raw[3]
	}
	env := model.NewEnvelope(t, deviceID, 0)
	env.TraceID = trace.FromContext(ctx)
	env.Payload = payload
	return []model.Envelope{env}, nil
}

func (a *Adapter) Encode(ctx context.Context, cmd model.Envelope) ([]byte, error) {
	return json.Marshal([]any{2, cmd.TransactionID, string(cmd.Type), json.RawMessage(cmd.Payload)})
}

func (a *Adapter) Heartbeat(deviceID string) []byte {
	b, _ := json.Marshal([]any{2, "heartbeat", "TriggerMessage", map[string]string{"requestedMessage": "Heartbeat"}})
	return b
}

var _ spi.Adapter = (*Adapter)(nil)
