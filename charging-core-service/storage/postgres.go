package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PGStore implements all persistence ports with PostgreSQL via GORM.
type PGStore struct {
	db *gorm.DB
}

func OpenPostgres(dsn string) (*PGStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("storage: open postgres: %w", err)
	}
	return &PGStore{db: db}, nil
}

func (s *PGStore) DB() *gorm.DB { return s.db }

// ---- GORM record types (map to the domain structs) ----

type orderRow struct {
	ID              string `gorm:"primaryKey"`
	TenantID        string `gorm:"index"`
	UserID          string `gorm:"column:user_id;index"`
	TransactionID   string `gorm:"uniqueIndex"`
	DeviceID        string
	GunNo           int
	StationID       string
	State           string
	TariffVersionID string
	EnergyUnits     int64
	PileAmount      money.Money `gorm:"type:decimal(18,4)"`
	PlatformAmount  money.Money `gorm:"type:decimal(18,4)"`
	ChargeAmount    money.Money `gorm:"type:decimal(18,4)"`
	Offline         bool
	ExceptionReason string
	CreatedAt       time.Time
	SettledAt       *time.Time
}

func (orderRow) TableName() string { return "charging_order" }

type segmentRow struct {
	ID          uint `gorm:"primaryKey"`
	OrderID     string `gorm:"index"`
	Period      string
	Price       money.Price `gorm:"type:decimal(18,5)"`
	EnergyUnits int64
	Amount      money.Money `gorm:"type:decimal(18,4)"`
}

func (segmentRow) TableName() string { return "order_segments" }

type tariffRow struct {
	ID        string `gorm:"primaryKey"`
	TenantID  string `gorm:"index"`
	StationID string `gorm:"index"`
	Effective time.Time
	SlotsJSON string `gorm:"type:jsonb;column:slots_json"`
}

func (tariffRow) TableName() string { return "tariff_version" }

type walletRow struct {
	UserID    string `gorm:"primaryKey"`
	TenantID  string `gorm:"primaryKey"`
	Principal money.Money `gorm:"type:decimal(18,4)"`
	Bonus     money.Money `gorm:"type:decimal(18,4)"`
	AppliedTx []string `gorm:"-"`
}

func (walletRow) TableName() string { return "wallet" }

type appliedDebitRow struct {
	TxnID string `gorm:"primaryKey"`
}

func (appliedDebitRow) TableName() string { return "applied_debit" }

type auditRow struct {
	ID             string `gorm:"primaryKey"`
	TransactionID  string `gorm:"index"`
	Rule           string
	PileAmount     money.Money `gorm:"type:decimal(18,4)"`
	PlatformAmount money.Money `gorm:"type:decimal(18,4)"`
	Diff           money.Money `gorm:"type:decimal(18,4)"`
	ChargeAmount   money.Money `gorm:"type:decimal(18,4)"`
	TraceID        string
	CreatedAt      time.Time
}

func (auditRow) TableName() string { return "billing_audit" }

type exceptionRow struct {
	ID             string `gorm:"primaryKey"`
	TransactionID  string `gorm:"index"`
	ReasonCode     string
	PileAmount     money.Money `gorm:"type:decimal(18,4)"`
	PlatformAmount money.Money `gorm:"type:decimal(18,4)"`
	Diff           money.Money `gorm:"type:decimal(18,4)"`
	TraceID        string
	CreatedAt      time.Time
	Resolved       bool
	ResolvedBy     string
}

func (exceptionRow) TableName() string { return "exception_order" }

// AutoMigrate creates/updates tables. For production, prefer versioned SQL.
func (s *PGStore) AutoMigrate() error {
	return s.db.AutoMigrate(
		&orderRow{}, &segmentRow{}, &tariffRow{}, &walletRow{},
		&appliedDebitRow{}, &auditRow{}, &exceptionRow{},
	)
}

// ---- OrderStore ----

func (s *PGStore) CreateOrder(_ context.Context, o Order) (bool, error) {
	row := orderRow{
		ID: o.ID, TenantID: o.TenantID, UserID: o.UserID, TransactionID: o.TransactionID,
		DeviceID: o.DeviceID, GunNo: o.GunNo, StationID: o.StationID,
		State: string(o.State), TariffVersionID: o.TariffVersionID,
		EnergyUnits: o.EnergyUnits, PileAmount: o.PileAmount,
		PlatformAmount: o.PlatformAmount, ChargeAmount: o.ChargeAmount,
		Offline: o.Offline, CreatedAt: o.CreatedAt, SettledAt: o.SettledAt,
		ExceptionReason: o.ExceptionReason,
	}
	err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "transaction_id"}}, DoNothing: true}).Create(&row).Error
	if err != nil {
		return false, err
	}
	return row.ID == o.ID && s.orderExists(o.TransactionID), nil
}

func (s *PGStore) orderExists(txnID string) bool {
	var count int64
	s.db.Model(&orderRow{}).Where("transaction_id = ?", txnID).Count(&count)
	return count == 1
}

func rowToOrder(r orderRow) Order {
	return Order{
		ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, TransactionID: r.TransactionID,
		DeviceID: r.DeviceID, GunNo: r.GunNo, StationID: r.StationID,
		State: OrderState(r.State), TariffVersionID: r.TariffVersionID,
		EnergyUnits: r.EnergyUnits, PileAmount: r.PileAmount,
		PlatformAmount: r.PlatformAmount, ChargeAmount: r.ChargeAmount,
		Offline: r.Offline, CreatedAt: r.CreatedAt, SettledAt: r.SettledAt,
		ExceptionReason: r.ExceptionReason,
	}
}

func (s *PGStore) GetOrderByTransaction(_ context.Context, txnID string) (Order, bool, error) {
	var r orderRow
	err := s.db.Where("transaction_id = ?", txnID).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	return rowToOrder(r), true, nil
}

func (s *PGStore) SaveOrder(_ context.Context, o Order) error {
	return s.db.Model(&orderRow{}).Where("transaction_id = ?", o.TransactionID).Updates(map[string]any{
		"state": string(o.State), "energy_units": o.EnergyUnits,
		"pile_amount": o.PileAmount, "platform_amount": o.PlatformAmount,
		"charge_amount": o.ChargeAmount, "settled_at": o.SettledAt,
		"exception_reason": o.ExceptionReason,
	}).Error
}

func (s *PGStore) SaveOrderSegments(_ context.Context, orderID string, segs []OrderSegment) error {
	rows := make([]segmentRow, 0, len(segs))
	for _, seg := range segs {
		rows = append(rows, segmentRow{
			OrderID: orderID, Period: seg.Period, Price: seg.Price,
			EnergyUnits: seg.EnergyUnits, Amount: seg.Amount,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return s.db.Create(&rows).Error
}

func (s *PGStore) GetOrderSegments(_ context.Context, orderID string) ([]OrderSegment, error) {
	var rows []segmentRow
	if err := s.db.Where("order_id = ?", orderID).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OrderSegment, 0, len(rows))
	for _, r := range rows {
		out = append(out, OrderSegment{
			OrderID: r.OrderID, Period: r.Period, Price: r.Price,
			EnergyUnits: r.EnergyUnits, Amount: r.Amount,
		})
	}
	return out, nil
}

func (s *PGStore) ListOrders(_ context.Context, tenantID string, limit int) ([]Order, error) {
	q := s.db.Model(&orderRow{}).Order("created_at DESC")
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []orderRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Order, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToOrder(r))
	}
	return out, nil
}

// ---- WalletStore ----

func (s *PGStore) GetWallet(_ context.Context, userID, tenantID string) (Wallet, bool, error) {
	var r walletRow
	err := s.db.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Wallet{}, false, nil
	}
	if err != nil {
		return Wallet{}, false, err
	}
	return Wallet{UserID: r.UserID, TenantID: r.TenantID, Principal: r.Principal, Bonus: r.Bonus}, true, nil
}

func (s *PGStore) DebitWallet(_ context.Context, userID, tenantID, txnID string, amount money.Money) (Wallet, error) {
	var w Wallet
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var applied int64
		if err := tx.Model(&appliedDebitRow{}).Where("txn_id = ?", txnID).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil // idempotent: already debited
		}
		var r walletRow
		if err := tx.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&r).Error; err != nil {
			return err
		}
		if r.Principal.LessThan(amount) {
			return ErrInsufficient
		}
		r.Principal = r.Principal.Sub(amount)
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		if err := tx.Create(&appliedDebitRow{TxnID: txnID}).Error; err != nil {
			return err
		}
		w = Wallet{UserID: r.UserID, TenantID: r.TenantID, Principal: r.Principal, Bonus: r.Bonus}
		return nil
	})
	return w, err
}

func (s *PGStore) SaveWallet(_ context.Context, w Wallet) error {
	return s.db.Save(&walletRow{UserID: w.UserID, TenantID: w.TenantID, Principal: w.Principal, Bonus: w.Bonus}).Error
}

// ---- AuditStore ----

func (s *PGStore) WriteAudit(_ context.Context, a AuditRecord) error {
	return s.db.Create(&auditRow{
		ID: a.ID, TransactionID: a.TransactionID, Rule: a.Rule,
		PileAmount: a.PileAmount, PlatformAmount: a.PlatformAmount,
		Diff: a.Diff, ChargeAmount: a.ChargeAmount, TraceID: a.TraceID,
		CreatedAt: a.CreatedAt,
	}).Error
}

func (s *PGStore) WriteException(_ context.Context, e ExceptionOrder) error {
	return s.db.Create(&exceptionRow{
		ID: e.ID, TransactionID: e.TransactionID, ReasonCode: e.ReasonCode,
		PileAmount: e.PileAmount, PlatformAmount: e.PlatformAmount,
		Diff: e.Diff, TraceID: e.TraceID, CreatedAt: e.CreatedAt,
	}).Error
}

func (s *PGStore) ListExceptions(_ context.Context, limit int) ([]ExceptionOrder, error) {
	q := s.db.Model(&exceptionRow{}).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []exceptionRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ExceptionOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, ExceptionOrder{
			ID: r.ID, TransactionID: r.TransactionID, ReasonCode: r.ReasonCode,
			PileAmount: r.PileAmount, PlatformAmount: r.PlatformAmount,
			Diff: r.Diff, TraceID: r.TraceID, CreatedAt: r.CreatedAt, Resolved: r.Resolved,
		})
	}
	return out, nil
}

func (s *PGStore) ResolveException(_ context.Context, id, operator string) error {
	return s.db.Model(&exceptionRow{}).Where("id = ?", id).Updates(map[string]any{
		"resolved": true, "resolved_by": operator,
	}).Error
}

// ---- TariffStore (PG) ----

func (s *PGStore) SaveTariff(_ context.Context, v TariffVersion) error {
	slots := ""
	for i, sl := range v.Slots {
		if i > 0 {
			slots += ","
		}
		slots += fmt.Sprintf(`{"slot":%d,"price":"%s"}`, sl.Slot, sl.Price.String())
	}
	return s.db.Save(&tariffRow{
		ID: v.ID, TenantID: v.TenantID, StationID: v.StationID,
		Effective: v.Effective, SlotsJSON: "[" + slots + "]",
	}).Error
}


func (s *PGStore) getTariff(_ context.Context, id string) (TariffVersion, bool, error) {
	var r tariffRow
	err := s.db.Where("id = ?", id).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TariffVersion{}, false, nil
	}
	if err != nil {
		return TariffVersion{}, false, err
	}
	return rowToTariff(r), true, nil
}

func (s *PGStore) latestTariff(_ context.Context, tenantID, stationID string, _ int64) (TariffVersion, bool, error) {
	var r tariffRow
	err := s.db.Where("tenant_id = ? AND station_id = ?", tenantID, stationID).
		Order("effective DESC").First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TariffVersion{}, false, nil
	}
	if err != nil {
		return TariffVersion{}, false, err
	}
	return rowToTariff(r), true, nil
}

func rowToTariff(r tariffRow) TariffVersion {
	v := TariffVersion{
		ID: r.ID, TenantID: r.TenantID, StationID: r.StationID,
		Effective: r.Effective,
	}
	// Slots are stored as JSON text; a lightweight parse avoids extra deps.
	for _, part := range splitObjects(r.SlotsJSON) {
		slot := parseSlot(part)
		v.Slots = append(v.Slots, slot)
	}
	return v
}

func (s *PGStore) ListTariffs(_ context.Context, tenantID, stationID string) ([]TariffVersion, error) {
	q := s.db.Model(&tariffRow{}).Where("tenant_id = ?", tenantID).Order("effective DESC")
	if stationID != "" {
		q = q.Where("station_id = ?", stationID)
	}
	var rows []tariffRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]TariffVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToTariff(r))
	}
	return out, nil
}
