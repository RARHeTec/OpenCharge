package storage

import (
	"context"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

type OrderStore interface {
	// Create returns false when the transactionId already exists (idempotency).
	Create(ctx context.Context, o Order) (bool, error)
	GetByTransaction(ctx context.Context, txnID string) (Order, bool, error)
	Save(ctx context.Context, o Order) error
	SaveSegments(ctx context.Context, orderID string, segs []OrderSegment) error
	GetSegments(ctx context.Context, orderID string) ([]OrderSegment, error)
	List(ctx context.Context, tenantID string, limit int) ([]Order, error)
}

type TariffStore interface {
	Get(ctx context.Context, versionID string) (TariffVersion, bool, error)
	// Latest returns the active snapshot for a station at a point in time.
	Latest(ctx context.Context, tenantID, stationID string, at int64) (TariffVersion, bool, error)
	// List returns all versions for a tenant/station, newest effective first.
	List(ctx context.Context, tenantID, stationID string) ([]TariffVersion, error)
	Save(ctx context.Context, v TariffVersion) error
}

type WalletStore interface {
	Get(ctx context.Context, userID, tenantID string) (Wallet, bool, error)
	// Debit is idempotent on transactionID: a retry must not deduct twice.
	Debit(ctx context.Context, userID, tenantID, txnID string, amount money.Money) ( Wallet, error)
	Save(ctx context.Context, w Wallet) error
}

type AuditStore interface {
	Write(ctx context.Context, a AuditRecord) error
	WriteException(ctx context.Context, e ExceptionOrder) error
	ListExceptions(ctx context.Context, tenantID string, limit int) ([]ExceptionOrder, error)
	ResolveException(ctx context.Context, id, operator string) error
	// ResolveExceptionByTransaction marks the exception for a bill resolved.
	// Returns the number of rows updated (0 = no matching unresolved exception).
	ResolveExceptionByTransaction(ctx context.Context, transactionID, operator string) (int, error)
}

// RegistryStore holds the station/device master data that everything else
// hangs off: billing needs the tenant and station a device belongs to.
type RegistryStore interface {
	SaveStation(ctx context.Context, s Station) error
	GetStation(ctx context.Context, tenantID, stationID string) (Station, bool, error)
	ListStations(ctx context.Context, tenantID string) ([]Station, error)

	SaveDevice(ctx context.Context, d Device) error
	GetDevice(ctx context.Context, deviceID string) (Device, bool, error)
	ListDevices(ctx context.Context, tenantID, stationID string) ([]Device, error)
}

// PricePlanStore holds adjustable pricing versions shown on the admin
// "当前价格 / 历史定价" pages.
type PricePlanStore interface {
	Save(ctx context.Context, plan PricePlan) error
	Get(ctx context.Context, id string) (PricePlan, bool, error)
	// Current returns the valid plan for a tenant/station/deviceType.
	Current(ctx context.Context, tenantID, stationID, deviceType string) (PricePlan, bool, error)
	List(ctx context.Context, tenantID, stationID, deviceType string, history bool) ([]PricePlan, error)
	// InvalidateCurrent marks the active plan for this scope as invalid.
	InvalidateCurrent(ctx context.Context, tenantID, stationID, deviceType string) error
}
