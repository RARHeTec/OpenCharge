// Package money implements exact fixed-point monetary arithmetic.
// The platform uses int64 units with 4 fractional digits: 1 Yuan = 10000 units.
// Never use float64 for any monetary value.
package money

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// Scale is the number of fractional digits kept in all intermediate math.
	Scale = 4
	// UnitsPerYuan is how many units make one Yuan.
	UnitsPerYuan int64 = 10000
	// UnitsPerCent is how many units make one Cent (settlement precision).
	UnitsPerCent int64 = 100
)

// Money is an exact amount expressed in units of 0.0001 Yuan.
type Money int64

func Zero() Money             { return 0 }
func FromUnits(u int64) Money { return Money(u) }
func FromYuan(y int64) Money  { return Money(y * UnitsPerYuan) }
func FromCents(c int64) Money { return Money(c * UnitsPerCent) }

// ParseYuan parses a decimal string exactly, without any floating point.
// Accepted forms: "12", "12.5", "-0.6234". More than Scale digits is an error.
func ParseYuan(s string) (Money, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("money: empty string")
	}
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	if s == "" {
		return 0, errors.New("money: invalid sign")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("money: invalid decimal %q", s)
	}
	integer, frac := parts[0], ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if integer == "" {
		return 0, fmt.Errorf("money: invalid integer part %q", s)
	}
	for _, c := range integer + frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("money: invalid digit in %q", s)
		}
	}
	for len(frac) < Scale {
		frac += "0"
	}
	if len(frac) > Scale {
		return 0, fmt.Errorf("money: precision exceeds %d digits", Scale)
	}
	// frac has exactly Scale digits, so appending its digits to the integer
	// part yields the value in units directly. Do not multiply by UnitsPerYuan
	// here — that would scale by 10^8 instead of 10^4.
	var v int64
	for _, c := range integer + frac {
		v = v*10 + int64(c-'0')
	}
	if neg {
		v = -v
	}
	return Money(v), nil
}

// MustParseYuan panics on invalid input; use only with constants.
func MustParseYuan(s string) Money {
	m, err := ParseYuan(s)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) Units() int64       { return int64(m) }
func (m Money) Add(n Money) Money  { return m + n }
func (m Money) Sub(n Money) Money  { return m - n }
func (m Money) Neg() Money         { return -m }
func (m Money) IsZero() bool       { return m == 0 }
func (m Money) IsNegative() bool   { return m < 0 }
func (m Money) LessThan(n Money) bool { return m < n }
func (m Money) GreaterThan(n Money) bool { return m > n }

// MulRate multiplies by a rate expressed in units (scale=4), half-up.
func (m Money) MulRate(rate Money) Money {
	return Money(divRoundHalfUp(int64(m)*int64(rate), UnitsPerYuan))
}

// MulQty multiplies by a quantity expressed with Scale digits (e.g. kWh), half-up.
func (m Money) MulQty(qtyUnits int64) Money {
	return Money(divRoundHalfUp(int64(m)*qtyUnits, UnitsPerYuan))
}

// ToCents rounds to 2 decimals using half-up, for settlement boundary only.
func (m Money) ToCents() int64 {
	return divRoundHalfUp(int64(m), UnitsPerCent)
}

// String returns the exact 4-digit representation, e.g. "0.6234".
func (m Money) String() string {
	v := int64(m)
	neg := v < 0
	if neg {
		v = -v
	}
	return fmt.Sprintf("%s%d.%04d", sign(neg), v/UnitsPerYuan, v%UnitsPerYuan)
}

// FormatYuan returns the 2-digit settlement/presentation string.
func (m Money) FormatYuan() string {
	c := m.ToCents()
	neg := c < 0
	if neg {
		c = -c
	}
	return fmt.Sprintf("%s%d.%02d", sign(neg), c/100, c%100)
}

func (m Money) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

func (m *Money) UnmarshalText(b []byte) error {
	v, err := ParseYuan(string(b))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

func divRoundHalfUp(v, d int64) int64 {
	if v >= 0 {
		return (v + d/2) / d
	}
	return (v - d/2) / d
}

func sign(neg bool) string {
	if neg {
		return "-"
	}
	return ""
}
