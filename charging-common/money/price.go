package money

import (
	"errors"
	"fmt"
	"strings"
)

// The YunKuaiChong V1.6 protocol reports tariff prices with 5 fractional
// digits (e.g. 1.30000 Yuan/kWh). Money is scale=4, so a separate scale=5
// type is required to avoid truncating the price.
const (
	PriceScale          = 5
	PriceUnitsPerYuan   int64 = 100000
	priceUnitsPerMoney  int64 = 10 // 1 Money unit (1e-4) = 10 Price units (1e-5)
)

// Price is an exact tariff rate expressed in units of 0.00001 Yuan.
type Price int64

func PriceFromUnits(u int64) Price { return Price(u) }
func PriceFromYuan(y int64) Price  { return Price(y * PriceUnitsPerYuan) }

// ParsePrice parses a decimal string with up to 5 fractional digits.
func ParsePrice(s string) (Price, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("money: empty price")
	}
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("money: invalid price %q", s)
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	for _, c := range parts[0] + frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("money: invalid price digit in %q", s)
		}
	}
	for len(frac) < PriceScale {
		frac += "0"
	}
	if len(frac) > PriceScale {
		return 0, fmt.Errorf("money: price precision exceeds %d digits", PriceScale)
	}
	var v int64
	for _, c := range parts[0] + frac {
		v = v*10 + int64(c-'0')
	}
	if neg {
		v = -v
	}
	return Price(v), nil
}

func MustParsePrice(s string) Price {
	p, err := ParsePrice(s)
	if err != nil {
		panic(err)
	}
	return p
}

func (p Price) Units() int64 { return int64(p) }

// String returns the exact 5-digit representation, e.g. "1.30000".
func (p Price) String() string {
	v := int64(p)
	neg := v < 0
	if neg {
		v = -v
	}
	return fmt.Sprintf("%s%d.%05d", negSign(neg), v/PriceUnitsPerYuan, v%PriceUnitsPerYuan)
}

// MulEnergy multiplies a price (Yuan/kWh, scale=5) by an energy quantity
// (kWh, scale=4) and returns an amount in Money (Yuan, scale=4).
//
// price_units * energy_units has scale 5+4=9; dividing by 10^5 yields scale 4.
// Integer half-up rounding, no floating point anywhere.
func (p Price) MulEnergy(energyUnits int64) Money {
	product := int64(p) * energyUnits
	return Money(divRoundHalfUp(product, PriceUnitsPerYuan))
}

func (p Price) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

func (p *Price) UnmarshalText(b []byte) error {
	v, err := ParsePrice(string(b))
	if err != nil {
		return err
	}
	*p = v
	return nil
}

func negSign(neg bool) string {
	if neg {
		return "-"
	}
	return ""
}
