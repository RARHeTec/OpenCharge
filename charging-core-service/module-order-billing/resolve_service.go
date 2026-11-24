package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

// Resolution decides how an exception order is finally charged.
type Resolution string

const (
	// ResolvePlatform charges the platform's recalculated amount.
	ResolvePlatform Resolution = "platform"
	// ResolvePile charges the device-reported amount.
	ResolvePile Resolution = "pile"
	// ResolveCustom charges an operator-supplied amount (e.g. negotiated discount).
	ResolveCustom Resolution = "custom"
)

var (
	ErrNotException   = errors.New("billing: order is not in exception state")
	ErrOrderNotFound  = errors.New("billing: order not found")
	ErrBadResolution  = errors.New("billing: invalid resolution decision")
	ErrCustomRequired = errors.New("billing: custom resolution needs an amount")
)

// ResolveRequest is the operator's disposition of an exception bill.
type ResolveRequest struct {
	TransactionID string
	Decision      Resolution
	CustomAmount  money.Money // required only for ResolveCustom
	Operator      string
	Note          string
}

// ResolveResult reports the financial effect of the resolution.
type ResolveResult struct {
	OrderID       string
	ChargedAmount money.Money
	WalletBalance money.Money
	Decision      Resolution
}

// ResolveException closes an exception order for real: it charges the wallet
// idempotently, moves the order to completed, writes an audit record and marks
// the exception resolved. It never runs twice for the same transaction id.
func (s *SettlementService) ResolveException(ctx context.Context, req ResolveRequest) (ResolveResult, error) {
	if req.TransactionID == "" {
		return ResolveResult{}, fmt.Errorf("billing: transactionId required")
	}
	if req.Operator == "" {
		return ResolveResult{}, fmt.Errorf("billing: operator required")
	}
	order, ok, err := s.orders.GetByTransaction(ctx, req.TransactionID)
	if err != nil {
		return ResolveResult{}, err
	}
	if !ok {
		return ResolveResult{}, ErrOrderNotFound
	}

	// Already settled: report the existing charge and the real balance instead
	// of double-debiting. Returning a zero balance here would mislead operators.
	if order.State == storage.OrderCompleted {
		var balance money.Money
		if w, ok, err := s.wallets.Get(ctx, order.UserID, order.TenantID); err == nil && ok {
			balance = w.Principal
		}
		return ResolveResult{
			OrderID: order.ID, ChargedAmount: order.ChargeAmount,
			WalletBalance: balance, Decision: req.Decision,
		}, nil
	}
	if order.State != storage.OrderException {
		return ResolveResult{}, ErrNotException
	}

	amount, err := pickAmount(order, req)
	if err != nil {
		return ResolveResult{}, err
	}
	if amount.IsNegative() {
		return ResolveResult{}, fmt.Errorf("billing: negative charge amount")
	}
	if order.UserID == "" {
		return ResolveResult{}, fmt.Errorf("billing: order has no wallet owner")
	}

	wallet, err := s.wallets.Debit(ctx, order.UserID, order.TenantID, order.TransactionID, amount)
	if err != nil {
		return ResolveResult{}, err
	}

	now := s.now()
	order.State = storage.OrderCompleted
	order.ChargeAmount = amount
	order.SettledAt = &now
	order.ExceptionReason = string(req.Decision) + ":" + req.Note
	if err := s.orders.Save(ctx, order); err != nil {
		return ResolveResult{}, err
	}

	if err := s.audits.Write(ctx, storage.AuditRecord{
		ID: s.idGenerator(), TransactionID: order.TransactionID,
		Rule:       "manual_" + string(req.Decision),
		PileAmount: order.PileAmount, PlatformAmount: order.PlatformAmount,
		Diff: order.PlatformAmount.Sub(order.PileAmount), ChargeAmount: amount,
		TraceID: req.Operator, CreatedAt: now,
	}); err != nil {
		return ResolveResult{}, err
	}

	if _, err := s.audits.ResolveExceptionByTransaction(ctx, order.TransactionID, req.Operator); err != nil {
		return ResolveResult{}, err
	}

	return ResolveResult{
		OrderID: order.ID, ChargedAmount: amount,
		WalletBalance: wallet.Principal, Decision: req.Decision,
	}, nil
}

func pickAmount(order storage.Order, req ResolveRequest) (money.Money, error) {
	switch req.Decision {
	case ResolvePlatform:
		return order.PlatformAmount, nil
	case ResolvePile:
		return order.PileAmount, nil
	case ResolveCustom:
		if req.CustomAmount.IsZero() {
			return 0, ErrCustomRequired
		}
		return req.CustomAmount, nil
	default:
		return 0, ErrBadResolution
	}
}
