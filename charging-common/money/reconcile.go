package money

// This file implements the money-side helpers for comparing a platform
// calculation against a device-reported bill. Business policy (which fields
// are authoritative, exception queues, audit records) lives in the billing
// module; these are the exact integer primitives it builds on.

// Tolerance is an absolute plus a relative band. A difference passes only
// when it is within BOTH bands (the smaller of the two wins).
type Tolerance struct {
	AbsUnits  int64 // absolute band, in Money units (0.01 Yuan = 100)
	PercentBP int64 // relative band, in basis points (0.1% = 10)
}

// DefaultTolerance: 0.01 Yuan absolute AND 0.1% of the order amount.
func DefaultTolerance() Tolerance {
	return Tolerance{AbsUnits: 100, PercentBP: 10}
}

// WithinTolerance reports whether diff is inside both bands of amount.
// A zero difference always passes. Integer arithmetic only.
func WithinTolerance(diff, amount Money, tol Tolerance) bool {
	if diff < 0 {
		diff = -diff
	}
	if diff == 0 {
		return true
	}
	if diff.Units() > tol.AbsUnits {
		return false
	}
	// amount * PercentBP / 10000, computed in integer with no overflow for
	// realistic charging amounts (amount units < 1e14).
	limit := amount.Units() * tol.PercentBP / 10000
	return diff.Units() <= limit
}

// LowerOf returns the smaller of two amounts. Used for the "never charge the
// driver more than the device displayed" rule when a small delta is accepted.
func LowerOf(a, b Money) Money {
	if a < b {
		return a
	}
	return b
}
