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
	ID              string
	TenantID        string
	TransactionID   string // UNIQUE - the idempotency key
	DeviceID        string
	GunNo           int
	StationID       string
	State           OrderState
	TariffVersionID string
	EnergyUnits     int64      // kWh, scale=4
	PileAmount      money.Money
	PlatformAmount  money.Money
	ChargeAmount    money.Money // actual amount charged to the wallet
	Offline         bool
	CreatedAt       time.Time
	SettledAt       *time.Time
	ExceptionReason string
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
	ID             string
	TransactionID  string
	Rule           string
	PileAmount     money.Money
	PlatformAmount money.Money
	Diff           money.Money
	ChargeAmount   money.Money
	TraceID        string
	CreatedAt      time.Time
}

// ExceptionOrder is a bill that failed hard exclusion or exceeded tolerance.
type ExceptionOrder struct {
	ID             string
	TransactionID  string
	ReasonCode     string
	PileAmount     money.Money
	PlatformAmount money.Money
	Diff           money.Money
	TraceID        string
	CreatedAt      time.Time
	Resolved       bool
}
