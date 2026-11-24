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
}
