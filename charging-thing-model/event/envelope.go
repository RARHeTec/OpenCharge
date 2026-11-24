package event

import "encoding/json"

// Envelope is the common header carried by every unified event.
// The payload is decoded into one of the concrete structs below.
type Envelope struct {
	TraceID      string          `json:"traceId"`
	Type         Type            `json:"type"`
	DeviceID     string          `json:"deviceId"`
	ConnectorID  int             `json:"connectorId,omitempty"`
	TenantID     string          `json:"tenantId,omitempty"`
	TransactionID string         `json:"transactionId,omitempty"`
	CommandSeq   int64           `json:"commandSeq,omitempty"`
	Timestamp    int64           `json:"timestamp"` // unix seconds
	Payload      json.RawMessage `json:"payload"`
}

func NewEnvelope(t Type, deviceID string, connectorID int) Envelope {
	return Envelope{Type: t, DeviceID: deviceID, ConnectorID: connectorID}
}
