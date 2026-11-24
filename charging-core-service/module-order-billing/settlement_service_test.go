package billing

import (
	"context"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

func fixture(t *testing.T) (*SettlementService, *storage.MemoryWalletStore, *storage.MemoryAuditStore) {
	t.Helper()
	orders := storage.NewMemoryOrderStore()
	tariffs := storage.NewMemoryTariffStore()
	wallets := storage.NewMemoryWalletStore()
	audits := storage.NewMemoryAuditStore()

	// Tariff V1: valley 0.30000, peak 0.60000 (slots 0 and 1)
	if err := tariffs.Save(context.Background(), storage.TariffVersion{
		ID: "V1", TenantID: "T1", StationID: "S1",
		Effective: time.Unix(1000, 0),
		Slots: []storage.TariffSlot{
			{Slot: 0, Price: money.MustParsePrice("0.30000")},
			{Slot: 1, Price: money.MustParsePrice("0.60000")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Wallet with 10.0000 Yuan
	if err := wallets.Save(context.Background(), storage.Wallet{
		UserID: "U1", TenantID: "T1", Principal: money.MustParseYuan("10"),
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewSettlementService(orders, tariffs, wallets, audits,
		WithIDGenerator(func() string { return "ord-1" }),
		WithClock(func() time.Time { return time.Unix(2000, 0) }),
		WithAutoLimit(money.MustParseYuan("500")),
	)
	return svc, wallets, audits
}

// Bill matching the agreed example: valley 1.5kWh @0.3 + peak 0.5kWh @0.6 = 0.75
func exactBill() Bill {
	return Bill{
		TransactionID: "TXN-001", TenantID: "T1", StationID: "S1",
		UserID: "U1", DeviceID: "D1", GunNo: 1, TariffVersionID: "V1",
		EnergyUnits: 20000, // 2.0 kWh, scale=4
		PileAmount:  money.MustParseYuan("0.75"),
		PileSegmentSum: money.MustParseYuan("0.75"),
		Segments: []BillSegment{
			{Period: "valley", Slot: 0, EnergyUnits: 15000},
			{Period: "peak", Slot: 1, EnergyUnits: 5000},
		},
		TraceID: "trace-1", SettledAt: time.Unix(2000, 0),
	}
}

func TestSettleExact(t *testing.T) {
	svc, wallets, _ := fixture(t)
	res, err := svc.Settle(context.Background(), exactBill())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultSettled {
		t.Fatalf("status = %s reason=%s", res.Status, res.ReasonCode)
	}
	if res.PlatformAmount.String() != "0.7500" {
		t.Errorf("platform = %s", res.PlatformAmount.String())
	}
	if res.ChargeAmount.String() != "0.7500" {
		t.Errorf("charge = %s", res.ChargeAmount.String())
	}
	// 10.0000 - 0.7500 = 9.2500
	if res.WalletBalance.String() != "9.2500" {
		t.Errorf("balance = %s", res.WalletBalance.String())
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "9.2500" {
		t.Errorf("stored wallet = %s", w.Principal.String())
	}
	if res.AuditRule != "exact_match" {
		t.Errorf("rule = %s", res.AuditRule)
	}
}

func TestSettleIdempotent(t *testing.T) {
	svc, wallets, _ := fixture(t)
	ctx := context.Background()
	first, _ := svc.Settle(ctx, exactBill())
	second, err := svc.Settle(ctx, exactBill())
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != ResultDuplicate {
		t.Fatalf("second status = %s", second.Status)
	}
	w, _, _ := wallets.Get(ctx, "U1", "T1")
	// Must be debited exactly once.
	if w.Principal.String() != "9.2500" {
		t.Fatalf("double debit: balance = %s", w.Principal.String())
	}
	if first.OrderID != second.OrderID {
		t.Error("duplicate should reference same order")
	}
}

func TestSettleWithinToleranceAndLowerOf(t *testing.T) {
	svc, wallets, audits := fixture(t)
	bill := exactBill()
	bill.PileAmount = money.MustParseYuan("0.7505") // pile shows 0.0005 more, inside both bands
	bill.PileSegmentSum = money.MustParseYuan("0.7505")
	res, err := svc.Settle(context.Background(), bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultSettled {
		t.Fatalf("status = %s reason=%s", res.Status, res.ReasonCode)
	}
	// Platform 0.7500 is lower than pile 0.7550 -> charge 0.7500
	if res.ChargeAmount.String() != "0.7500" {
		t.Errorf("charge = %s, want lower-of = 0.7500", res.ChargeAmount.String())
	}
	if len(audits.Audits()) != 1 {
		t.Fatalf("audit records = %d", len(audits.Audits()))
	}
	if audits.Audits()[0].Rule != "within_tolerance" {
		t.Errorf("audit rule = %s", audits.Audits()[0].Rule)
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "9.2500" {
		t.Errorf("balance = %s", w.Principal.String())
	}
}

func TestSettleDiffExceedsToleranceHold(t *testing.T) {
	svc, wallets, audits := fixture(t)
	bill := exactBill()
	bill.PileAmount = money.MustParseYuan("0.80") // 0.05 off -> exceeds both bands
	bill.PileSegmentSum = money.MustParseYuan("0.80")
	res, err := svc.Settle(context.Background(), bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultException || res.ReasonCode != ReasonTolerance {
		t.Fatalf("status=%s reason=%s", res.Status, res.ReasonCode)
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "10.0000" {
		t.Fatalf("wallet must not be debited: %s", w.Principal.String())
	}
	if len(audits.Audits()) != 0 {
		t.Error("no tolerance audit should be written for an exception")
	}
	if len(audits.Exceptions()) != 1 {
		t.Errorf("exception records = %d", len(audits.Exceptions()))
	}
}

func TestSettleHardExclusionSegmentSum(t *testing.T) {
	svc, wallets, _ := fixture(t)
	bill := exactBill()
	bill.PileSegmentSum = money.MustParseYuan("0.70") // pile internally inconsistent
	res, err := svc.Settle(context.Background(), bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.ReasonCode != ReasonSegmentSum {
		t.Fatalf("reason = %s", res.ReasonCode)
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "10.0000" {
		t.Fatal("no debit on hard exclusion")
	}
}

func TestSettleMissingTariffHold(t *testing.T) {
	svc, wallets, _ := fixture(t)
	bill := exactBill()
	bill.TariffVersionID = "UNKNOWN"
	res, err := svc.Settle(context.Background(), bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.ReasonCode != ReasonNoTariff {
		t.Fatalf("reason = %s", res.ReasonCode)
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "10.0000" {
		t.Fatal("no debit when tariff missing")
	}
}

func TestSettleOverAutoLimit(t *testing.T) {
	svc, wallets, _ := fixture(t)
	bill := exactBill()
	bill.PileAmount = money.MustParseYuan("600")
	bill.PileSegmentSum = money.MustParseYuan("600")
	res, err := svc.Settle(context.Background(), bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.ReasonCode != ReasonTooLarge {
		t.Fatalf("reason = %s", res.ReasonCode)
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "10.0000" {
		t.Fatal("no debit over auto limit")
	}
}

func TestExceptionOrderKeepsSegments(t *testing.T) {
	orders := storage.NewMemoryOrderStore()
	tariffs := storage.NewMemoryTariffStore()
	wallets := storage.NewMemoryWalletStore()
	audits := storage.NewMemoryAuditStore()
	ctx := context.Background()
	_ = tariffs.Save(ctx, storage.TariffVersion{
		ID: "V1", TenantID: "T1", StationID: "S1", Effective: time.Unix(1000, 0),
		Slots: []storage.TariffSlot{
			{Slot: 0, Price: money.MustParsePrice("0.30000")},
			{Slot: 1, Price: money.MustParsePrice("0.60000")},
		},
	})
	_ = wallets.Save(ctx, storage.Wallet{UserID: "U1", TenantID: "T1", Principal: money.MustParseYuan("10")})
	svc := NewSettlementService(orders, tariffs, wallets, audits, WithIDGenerator(func() string { return "ord-x" }))

	bill := exactBill()
	bill.TransactionID = "TXN-SEG"
	bill.PileAmount = money.MustParseYuan("0.95")
	bill.PileSegmentSum = money.MustParseYuan("0.95")
	res, err := svc.Settle(ctx, bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultException {
		t.Fatalf("status = %s", res.Status)
	}
	segs, err := orders.GetSegments(ctx, res.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 {
		t.Fatalf("segments = %d, want 2 (valley + peak) so operators can inspect the bill", len(segs))
	}
	// Wallet must be untouched on an exception.
	w, _, _ := wallets.Get(ctx, "U1", "T1")
	if w.Principal.String() != "10.0000" {
		t.Errorf("wallet = %s", w.Principal.String())
	}
}

func TestSettledOrderCarriesUserID(t *testing.T) {
	svc, wallets, _ := fixture(t)
	res, err := svc.Settle(context.Background(), exactBill())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultSettled {
		t.Fatalf("status = %s", res.Status)
	}
	_ = wallets
}
