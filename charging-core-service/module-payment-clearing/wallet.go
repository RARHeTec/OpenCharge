// Package payment manages wallets, payment routing and T+1 reconciliation.
package payment

import "github.com/RARHeTec/OpenCharge/charging-common/money"

// Wallet separates principal and bonus balances. Deduction always consumes
// principal first unless the tenant policy states otherwise.
type Wallet struct {
	UserID          string
	TenantID        string
	Principal       money.Money
	Bonus           money.Money
	FrozenPreAuth   money.Money
}

// RouteMode selects between platform facilitator split and operator direct.
type RouteMode string

const (
	RoutePlatformSplit RouteMode = "platform_split" // mode A
	RouteOperatorDirect RouteMode = "operator_direct" // mode B
)

// SplitResult is computed at settlement with integer arithmetic only.
type SplitResult struct {
	Operator money.Money
	Platform money.Money
	Venue    money.Money
}

// Split splits a settlement amount by basis points (bps), remainder to venue.
func Split(total money.Money, operatorBPS, platformBPS int) SplitResult {
	operator := money.FromUnits(total.Units() * int64(operatorBPS) / 10000)
	platform := money.FromUnits(total.Units() * int64(platformBPS) / 10000)
	venue := total.Sub(operator).Sub(platform)
	return SplitResult{Operator: operator, Platform: platform, Venue: venue}
}
