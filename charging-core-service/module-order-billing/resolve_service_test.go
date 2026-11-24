package billing

import (
	"context"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

// exceptionFixture produces an order stuck in exception state: the pile bills
// 0.80 but the platform tariff computes 0.75, exceeding tolerance.
func exceptionFixture(t *testing.T) (*SettlementService, *storage.MemoryWalletStore, string) {
	t.Helper()
	svc, wallets, _ := fixture(t)
	bill := exactBill()
	bill.TransactionID = "TXN-EXC-1"
	bill.PileAmount = money.MustParseYuan("0.80")
	bill.PileSegmentSum = money.MustParseYuan("0.80")
	res, err := svc.Settle(context.Background(), bill)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != ResultException {
		t.Fatalf("setup: expected exception, got %s", res.Status)
	}
	return svc, wallets, "TXN-EXC-1"
}

func TestResolvePlatform(t *testing.T) {
	svc, wallets, txn := exceptionFixture(t)
	res, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: txn, Decision: ResolvePlatform, Operator: "alice", Note: "按平台价",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Platform computed 0.7500.
	if res.ChargedAmount.String() != "0.7500" {
		t.Errorf("charged = %s, want 0.7500", res.ChargedAmount.String())
	}
	// 10.0000 - 0.7500 = 9.2500
	if res.WalletBalance.String() != "9.2500" {
		t.Errorf("balance = %s", res.WalletBalance.String())
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	if w.Principal.String() != "9.2500" {
		t.Errorf("stored wallet = %s", w.Principal.String())
	}
}

func TestResolvePile(t *testing.T) {
	svc, wallets, txn := exceptionFixture(t)
	res, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: txn, Decision: ResolvePile, Operator: "bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ChargedAmount.String() != "0.8000" {
		t.Errorf("charged = %s, want pile 0.8000", res.ChargedAmount.String())
	}
	if res.WalletBalance.String() != "9.2000" {
		t.Errorf("balance = %s", res.WalletBalance.String())
	}
	_ = wallets
}

func TestResolveCustom(t *testing.T) {
	svc, _, txn := exceptionFixture(t)
	res, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: txn, Decision: ResolveCustom,
		CustomAmount: money.MustParseYuan("0.60"), Operator: "carol", Note: "协商折扣",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ChargedAmount.String() != "0.6000" {
		t.Errorf("charged = %s", res.ChargedAmount.String())
	}
}

func TestResolveCustomNeedsAmount(t *testing.T) {
	svc, _, txn := exceptionFixture(t)
	_, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: txn, Decision: ResolveCustom, Operator: "dan",
	})
	if err != ErrCustomRequired {
		t.Fatalf("err = %v, want ErrCustomRequired", err)
	}
}

func TestResolveIdempotent(t *testing.T) {
	svc, wallets, txn := exceptionFixture(t)
	first, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: txn, Decision: ResolvePile, Operator: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: txn, Decision: ResolvePile, Operator: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ChargedAmount != first.ChargedAmount {
		t.Errorf("second charged = %s", second.ChargedAmount.String())
	}
	if second.WalletBalance.String() != "9.2000" {
		t.Errorf("second resolve balance = %s, want real 9.2000 (not zero)", second.WalletBalance.String())
	}
	w, _, _ := wallets.Get(context.Background(), "U1", "T1")
	// Debited exactly once: 10.0000 - 0.8000.
	if w.Principal.String() != "9.2000" {
		t.Fatalf("double debit: %s", w.Principal.String())
	}
}

func TestResolveRejectsNonException(t *testing.T) {
	svc, _, _ := fixture(t)
	bill := exactBill() // settles cleanly
	if _, err := svc.Settle(context.Background(), bill); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: bill.TransactionID, Decision: ResolvePile, Operator: "x",
	})
	// A completed order is reported as already settled, not an error path.
	if err != nil {
		t.Fatalf("unexpected error for completed order: %v", err)
	}
}

func TestResolveUnknownOrder(t *testing.T) {
	svc, _, _ := fixture(t)
	_, err := svc.ResolveException(context.Background(), ResolveRequest{
		TransactionID: "NOPE", Decision: ResolvePile, Operator: "x",
	})
	if err != ErrOrderNotFound {
		t.Fatalf("err = %v", err)
	}
}
