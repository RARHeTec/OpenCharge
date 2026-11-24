// Package storage defines persistence ports for the core business domain.
// Interfaces live here so the settlement service can be tested with in-memory
// implementations while production uses PostgreSQL via GORM.
package storage

import (
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

type OrderState string

const (
	OrderCreated   OrderState = "created"
	OrderCharging  OrderState = "charging"
	OrderCompleted OrderState = "completed"
	OrderException OrderState = "exception"
)

// Order is the persisted charging order. TariffVersionID is intentionally
// denormalized so historical bills can be repriced with the exact rate used.
type Order struct {
	ID              string      `json:"id"`
	TenantID        string      `json:"tenantId"`
	TransactionID   string      `json:"transactionId"` // UNIQUE - the idempotency key
	DeviceID        string      `json:"deviceId"`
	GunNo           int         `json:"gunNo"`
	StationID       string      `json:"stationId"`
	State           OrderState  `json:"state"`
	TariffVersionID string      `json:"tariffVersionId"`
	EnergyUnits     int64       `json:"energyUnits"` // kWh, scale=4
	PileAmount      money.Money `json:"pileAmount"`
	PlatformAmount  money.Money `json:"platformAmount"`
	ChargeAmount    money.Money `json:"chargeAmount"` // actual amount charged to the wallet
	Offline         bool        `json:"offline"`
	CreatedAt       time.Time   `json:"createdAt"`
	SettledAt       *time.Time  `json:"settledAt"`
	ExceptionReason string      `json:"exceptionReason"`
}

// OrderSegment is one time-of-use band inside a bill.
type OrderSegment struct {
	OrderID     string
	Period      string // sharp | peak | flat | valley
	Price       money.Price
	EnergyUnits int64
	Amount      money.Money
}

// TariffVersion is an immutable price snapshot.
type TariffVersion struct {
	ID        string
	TenantID  string
	StationID string
	Effective time.Time
	Slots     []TariffSlot
}

type TariffSlot struct {
	Slot  int // 0..47
	Price money.Price
}

// Wallet holds separated principal and bonus balances.
type Wallet struct {
	UserID    string
	TenantID  string
	Principal money.Money
	Bonus     money.Money
}

// AuditRecord is written every time the tolerance rule auto-settles a bill.
type AuditRecord struct {
	ID             string      `json:"id"`
	TransactionID  string      `json:"transactionId"`
	Rule           string      `json:"rule"`
	PileAmount     money.Money `json:"pileAmount"`
	PlatformAmount money.Money `json:"platformAmount"`
	Diff           money.Money `json:"diff"`
	ChargeAmount   money.Money `json:"chargeAmount"`
	TraceID        string      `json:"traceId"`
	CreatedAt      time.Time   `json:"createdAt"`
}

// ExceptionOrder is a bill that failed hard exclusion or exceeded tolerance.
type ExceptionOrder struct {
	ID             string      `json:"id"`
	TransactionID  string      `json:"transactionId"`
	ReasonCode     string      `json:"reasonCode"`
	PileAmount     money.Money `json:"pileAmount"`
	PlatformAmount money.Money `json:"platformAmount"`
	Diff           money.Money `json:"diff"`
	TraceID        string      `json:"traceId"`
	CreatedAt      time.Time   `json:"createdAt"`
	Resolved       bool        `json:"resolved"`
}
