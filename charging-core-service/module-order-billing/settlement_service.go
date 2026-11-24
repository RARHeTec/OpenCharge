// Package billing settlement implements the end-of-charge reconciliation
// pipeline: idempotency -> hard exclusions -> platform repricing -> tolerance
// decision -> wallet debit -> order completion, with full audit trail.
package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

// Bill is the normalized input from a device transaction record (0x3B).
type Bill struct {
	TransactionID    string
	TenantID         string
	StationID        string
	UserID           string
	DeviceID         string
	GunNo            int
	TariffVersionID  string
	Offline          bool
	EnergyUnits      int64           // kWh, scale=4
	PileAmount       money.Money     // device reported 消费金额
	PileSegmentSum   money.Money     // sum of the four period amounts
	Segments         []BillSegment   // per-period energy from the device
	TraceID          string
	SettledAt        time.Time
}

type BillSegment struct {
	Period      string // sharp | peak | flat | valley
	Slot        int
	EnergyUnits int64
}

// Result describes what the settlement pipeline decided.
type Result struct {
	Status         ResultStatus
	ReasonCode     string
	OrderID        string
	PileAmount     money.Money
	PlatformAmount money.Money
	ChargeAmount   money.Money
	WalletBalance  money.Money
	AuditRule      string
}

type ResultStatus string

const (
	ResultSettled  ResultStatus = "settled"
	ResultDuplicate ResultStatus = "duplicate"
	ResultException ResultStatus = "exception"
)

// Hard-exclusion reason codes.
const (
	ReasonSegmentSum = "pile_segment_sum_mismatch"
	ReasonNoTariff   = "tariff_version_missing"
	ReasonBadEnergy  = "energy_not_monotonic"
	ReasonTooLarge   = "amount_over_auto_limit"
	ReasonTolerance  = "diff_exceeds_tolerance"
	ReasonNoWallet   = "wallet_not_found"
	ReasonInsufficient = "insufficient_balance"
)

// SettlementService orchestrates the full pipeline.
type SettlementService struct {
	orders  storage.OrderStore
	tariffs storage.TariffStore
	wallets storage.WalletStore
	audits  storage.AuditStore

	tolerance    money.Tolerance
	autoLimit    money.Money // hard exclusion above this amount; 0 = disabled
	idGenerator  func() string
	now          func() time.Time
}

type Option func(*SettlementService)

func WithTolerance(t money.Tolerance) Option {
	return func(s *SettlementService) { s.tolerance = t }
}
func WithAutoLimit(limit money.Money) Option {
	return func(s *SettlementService) { s.autoLimit = limit }
}
func WithIDGenerator(f func() string) Option {
	return func(s *SettlementService) { s.idGenerator = f }
}
func WithClock(f func() time.Time) Option {
	return func(s *SettlementService) { s.now = f }
}

func NewSettlementService(
	orders storage.OrderStore,
	tariffs storage.TariffStore,
	wallets storage.WalletStore,
	audits storage.AuditStore,
	opts ...Option,
) *SettlementService {
	s := &SettlementService{
		orders: orders, tariffs: tariffs, wallets: wallets, audits: audits,
		tolerance: money.DefaultTolerance(),
		idGenerator: func() string { return fmt.Sprintf("ord-%d", time.Now().UnixNano()) },
		now: time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Settle runs the pipeline. It is safe to call repeatedly with the same
// transactionId: the second call returns ResultDuplicate without side effects.
func (s *SettlementService) Settle(ctx context.Context, bill Bill) (Result, error) {
	// 1. Idempotency: a stored order with this transactionId means we already
	//    processed it (settled or exception). Never debit again.
	if existing, ok, err := s.orders.GetByTransaction(ctx, bill.TransactionID); err != nil {
		return Result{}, err
	} else if ok {
		return Result{
			Status: ResultDuplicate, ReasonCode: "", OrderID: existing.ID,
			PileAmount: existing.PileAmount, PlatformAmount: existing.PlatformAmount,
			ChargeAmount: existing.ChargeAmount,
		}, nil
	}

	now := s.now()
	if bill.SettledAt.IsZero() {
		bill.SettledAt = now
	}
	orderID := s.idGenerator()

	// 2. Hard exclusions - never auto-settle these.
	if code := s.hardExclusion(bill); code != "" {
		return s.hold(ctx, bill, orderID, money.Zero(), code, now)
	}

	// 3. Platform repricing from the immutable tariff snapshot.
	platform, segs, err := s.reprice(ctx, bill)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, errTariffMissing) {
			return s.hold(ctx, bill, orderID, money.Zero(), ReasonNoTariff, now)
		}
		return Result{}, err
	}

	// 4. Tolerance decision.
	diff := platform.Sub(bill.PileAmount)
	rule := "exact_match"
	charge := platform
	if diff != 0 {
		if !money.WithinTolerance(diff, platform, s.tolerance) {
			return s.hold(ctx, bill, orderID, platform, ReasonTolerance, now)
		}
		rule = "within_tolerance"
		// Never charge the driver more than the device displayed.
		charge = money.LowerOf(platform, bill.PileAmount)
	}

	// 5. Persist the order (idempotent create) and its segments.
	o := storage.Order{
		ID: orderID, TenantID: bill.TenantID, TransactionID: bill.TransactionID,
		DeviceID: bill.DeviceID, GunNo: bill.GunNo, StationID: bill.StationID,
		State: storage.OrderCompleted, TariffVersionID: bill.TariffVersionID,
		EnergyUnits: bill.EnergyUnits, PileAmount: bill.PileAmount,
		PlatformAmount: platform, ChargeAmount: charge, Offline: bill.Offline,
		CreatedAt: now, SettledAt: &now,
	}
	created, err := s.orders.Create(ctx, o)
	if err != nil {
		return Result{}, err
	}
	if !created {
		// concurrent duplicate arrived between step 1 and here
		existing, _, _ := s.orders.GetByTransaction(ctx, bill.TransactionID)
		return Result{Status: ResultDuplicate, OrderID: existing.ID, ChargeAmount: existing.ChargeAmount}, nil
	}
	for i := range segs {
		segs[i].OrderID = orderID
	}
	if err := s.orders.SaveSegments(ctx, orderID, segs); err != nil {
		return Result{}, err
	}

	// 6. Debit wallet (idempotent on transactionId).
	wallet, err := s.wallets.Debit(ctx, bill.UserID, bill.TenantID, bill.TransactionID, charge)
	if err != nil {
		if errors.Is(err, storage.ErrWalletNotFound) {
			return s.hold(ctx, bill, orderID, platform, ReasonNoWallet, now)
		}
		if errors.Is(err, storage.ErrInsufficient) {
			return s.hold(ctx, bill, orderID, platform, ReasonInsufficient, now)
		}
		return Result{}, err
	}

	// 7. Audit record for every auto-settled tolerance decision.
	if err := s.audits.Write(ctx, storage.AuditRecord{
		ID: s.idGenerator(), TransactionID: bill.TransactionID, Rule: rule,
		PileAmount: bill.PileAmount, PlatformAmount: platform, Diff: diff,
		ChargeAmount: charge, TraceID: bill.TraceID, CreatedAt: now,
	}); err != nil {
		return Result{}, err
	}

	return Result{
		Status: ResultSettled, ReasonCode: rule, OrderID: orderID,
		PileAmount: bill.PileAmount, PlatformAmount: platform,
		ChargeAmount: charge, WalletBalance: wallet.Principal, AuditRule: rule,
	}, nil
}

// hardExclusion returns a reason code when the bill must not be auto-settled.
func (s *SettlementService) hardExclusion(bill Bill) string {
	// Pile's own four period amounts must add up to its reported total.
	if bill.PileSegmentSum != bill.PileAmount {
		return ReasonSegmentSum
	}
	// Energy must be non-negative and consistent across segments.
	if bill.EnergyUnits < 0 {
		return ReasonBadEnergy
	}
	var segSum int64
	for _, seg := range bill.Segments {
		if seg.EnergyUnits < 0 {
			return ReasonBadEnergy
		}
		segSum += seg.EnergyUnits
	}
	if segSum > 0 && (segSum-bill.EnergyUnits > 1 || bill.EnergyUnits-segSum > 1) {
		return ReasonBadEnergy
	}
	if s.autoLimit > 0 && bill.PileAmount.GreaterThan(s.autoLimit) {
		return ReasonTooLarge
	}
	return ""
}

var errTariffMissing = errors.New("billing: tariff version missing")

// reprice computes the platform amount using the immutable tariff snapshot.
func (s *SettlementService) reprice(ctx context.Context, bill Bill) (money.Money, []storage.OrderSegment, error) {
	tariff, ok, err := s.tariffs.Get(ctx, bill.TariffVersionID)
	if err != nil {
		return 0, nil, err
	}
	if !ok {
		return 0, nil, errTariffMissing
	}
	// Safety: snapshot must belong to the same tenant/station.
	if tariff.TenantID != bill.TenantID || tariff.StationID != bill.StationID {
		return 0, nil, errTariffMissing
	}

	prices := make(map[int]money.Price, len(tariff.Slots))
	for _, sl := range tariff.Slots {
		prices[sl.Slot] = sl.Price
	}

	total := money.Zero()
	segs := make([]storage.OrderSegment, 0, len(bill.Segments))
	for _, bs := range bill.Segments {
		price, ok := prices[bs.Slot]
		if !ok {
			// Missing slot price in the tariff snapshot: hard fail, never guess.
			return 0, nil, errTariffMissing
		}
		amount := price.MulEnergy(bs.EnergyUnits)
		total = total.Add(amount)
		segs = append(segs, storage.OrderSegment{
			OrderID: "", Period: bs.Period, Price: price,
			EnergyUnits: bs.EnergyUnits, Amount: amount,
		})
	}
	return total, segs, nil
}

// hold stores an exception and never touches the wallet.
func (s *SettlementService) hold(ctx context.Context, bill Bill, orderID string, platform money.Money, reason string, now time.Time) (Result, error) {
	diff := platform.Sub(bill.PileAmount)
	o := storage.Order{
		ID: orderID, TenantID: bill.TenantID, TransactionID: bill.TransactionID,
		DeviceID: bill.DeviceID, GunNo: bill.GunNo, StationID: bill.StationID,
		State: storage.OrderException, TariffVersionID: bill.TariffVersionID,
		EnergyUnits: bill.EnergyUnits, PileAmount: bill.PileAmount,
		PlatformAmount: platform, Offline: bill.Offline, CreatedAt: now,
		ExceptionReason: reason,
	}
	created, err := s.orders.Create(ctx, o)
	if err != nil {
		return Result{}, err
	}
	if !created {
		existing, _, _ := s.orders.GetByTransaction(ctx, bill.TransactionID)
		return Result{Status: ResultDuplicate, OrderID: existing.ID}, nil
	}
	if err := s.audits.WriteException(ctx, storage.ExceptionOrder{
		ID: s.idGenerator(), TransactionID: bill.TransactionID, ReasonCode: reason,
		PileAmount: bill.PileAmount, PlatformAmount: platform, Diff: diff,
		TraceID: bill.TraceID, CreatedAt: now,
	}); err != nil {
		return Result{}, err
	}
	return Result{
		Status: ResultException, ReasonCode: reason, OrderID: orderID,
		PileAmount: bill.PileAmount, PlatformAmount: platform, ChargeAmount: money.Zero(),
	}, nil
}
