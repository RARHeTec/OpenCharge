package storage

import (
	"context"
	"strconv"
	"strings"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

// Separate wrapper types are required because the port interfaces define
// conflicting Save(ctx, X) methods; one Go type cannot implement all three.

type PGOrderStore struct{ S *PGStore }

func (w *PGOrderStore) Create(ctx context.Context, o Order) (bool, error) {
	return w.S.CreateOrder(ctx, o)
}
func (w *PGOrderStore) GetByTransaction(ctx context.Context, t string) (Order, bool, error) {
	return w.S.GetOrderByTransaction(ctx, t)
}
func (w *PGOrderStore) Save(ctx context.Context, o Order) error { return w.S.SaveOrder(ctx, o) }
func (w *PGOrderStore) SaveSegments(ctx context.Context, id string, segs []OrderSegment) error {
	return w.S.SaveOrderSegments(ctx, id, segs)
}
func (w *PGOrderStore) GetSegments(ctx context.Context, orderID string) ([]OrderSegment, error) {
	return w.S.GetOrderSegments(ctx, orderID)
}
func (w *PGOrderStore) List(ctx context.Context, tenant string, limit int) ([]Order, error) {
	return w.S.ListOrders(ctx, tenant, limit)
}

type PGTariffStore struct{ S *PGStore }

func (w *PGTariffStore) Get(ctx context.Context, id string) (TariffVersion, bool, error) {
	return w.S.getTariff(ctx, id)
}
func (w *PGTariffStore) Latest(ctx context.Context, tenant, station string, at int64) (TariffVersion, bool, error) {
	return w.S.latestTariff(ctx, tenant, station, at)
}
func (w *PGTariffStore) Save(ctx context.Context, v TariffVersion) error {
	return w.S.SaveTariff(ctx, v)
}
func (w *PGTariffStore) List(ctx context.Context, tenantID, stationID string) ([]TariffVersion, error) {
	return w.S.ListTariffs(ctx, tenantID, stationID)
}

type PGWalletStore struct{ S *PGStore }

func (w *PGWalletStore) Get(ctx context.Context, userID, tenantID string) (Wallet, bool, error) {
	return w.S.GetWallet(ctx, userID, tenantID)
}
func (w *PGWalletStore) Debit(ctx context.Context, userID, tenantID, txnID string, amount money.Money) (Wallet, error) {
	return w.S.DebitWallet(ctx, userID, tenantID, txnID, amount)
}
func (w *PGWalletStore) Save(ctx context.Context, wallet Wallet) error {
	return w.S.SaveWallet(ctx, wallet)
}

type PGAuditStore struct{ S *PGStore }

func (w *PGAuditStore) Write(ctx context.Context, a AuditRecord) error {
	return w.S.WriteAudit(ctx, a)
}
func (w *PGAuditStore) WriteException(ctx context.Context, e ExceptionOrder) error {
	return w.S.WriteException(ctx, e)
}
func (w *PGAuditStore) ListExceptions(ctx context.Context, tenantID string, limit int) ([]ExceptionOrder, error) {
	return w.S.ListExceptions(ctx, limit)
}
func (w *PGAuditStore) ResolveException(ctx context.Context, id, operator string) error {
	return w.S.db.Model(&exceptionRow{}).Where("id = ?", id).
		Updates(map[string]any{"resolved": true, "resolved_by": operator}).Error
}

func (w *PGAuditStore) ResolveExceptionByTransaction(_ context.Context, txnID, operator string) (int, error) {
	res := w.S.db.Model(&exceptionRow{}).
		Where("transaction_id = ? AND resolved = ?", txnID, false).
		Updates(map[string]any{"resolved": true, "resolved_by": operator})
	return int(res.RowsAffected), res.Error
}

// Minimal JSON helpers for the tariff slots column (avoids encoding/json
// import churn in the main file; these operate on our own generated format).
func splitObjects(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[]")
	var out []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				out = append(out, s[start:i+1])
				start = i + 1
			}
		}
	}
	return out
}

func parseSlot(obj string) TariffSlot {
	slot := TariffSlot{}
	obj = strings.Trim(obj, "{}")
	for _, field := range strings.Split(obj, ",") {
		kv := strings.SplitN(field, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(kv[0]), `"`)
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch key {
		case "slot":
			n, _ := strconv.Atoi(val)
			slot.Slot = n
		case "price":
			if p, err := money.ParsePrice(val); err == nil {
				slot.Price = p
			}
		}
	}
	return slot
}

var (
	_ OrderStore  = (*PGOrderStore)(nil)
	_ TariffStore = (*PGTariffStore)(nil)
	_ WalletStore = (*PGWalletStore)(nil)
	_ AuditStore  = (*PGAuditStore)(nil)
)
