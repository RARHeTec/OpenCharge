package billing

import "github.com/RARHeTec/OpenCharge/charging-common/money"

// Segment is a time-sliced energy reading tied to a 30-minute slot.
type Segment struct {
	Slot     int
	EnergyWh int64
}

// Order state machine. Only the transitions listed here are legal; the
// gateway's StatusNotification must be validated against it to prevent
// out-of-order frames from overwriting the true state.
type State string

const (
	StateCreated   State = "created"
	StateStarting  State = "starting"
	StateCharging  State = "charging"
	StateStopping  State = "stopping"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

var allowed = map[State]map[State]bool{
	StateCreated:  {StateStarting: true, StateFailed: true},
	StateStarting: {StateCharging: true, StateFailed: true},
	StateCharging: {StateStopping: true, StateFailed: true},
	StateStopping: {StateCompleted: true, StateFailed: true},
}

func CanTransition(from, to State) bool {
	return allowed[from][to]
}

// Order is the billing entity. TariffVersionID is deliberately denormalized.
type Order struct {
	ID              string
	TenantID        string
	DeviceID        string
	ConnectorID     int
	TransactionID   string
	State           State
	TariffVersionID string
	EnergyWh        int64
	Fee             money.Money
	Offline         bool
}
