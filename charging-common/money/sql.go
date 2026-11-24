package money

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
)

// Value implements driver.Valuer. Money is stored as a decimal string so the
// database never sees a float and precision is preserved exactly.
func (m Money) Value() (driver.Value, error) {
	return m.String(), nil
}

// Scan implements sql.Scanner. Accepts decimal strings, []byte, int64 (units).
func (m *Money) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*m = Zero()
		return nil
	case string:
		return m.scanString(v)
	case []byte:
		return m.scanString(string(v))
	case int64:
		*m = Money(v)
		return nil
	default:
		return fmt.Errorf("money: cannot scan %T", src)
	}
}

func (m *Money) scanString(s string) error {
	// PostgreSQL DECIMAL may return trailing zeros or scientific notation.
	// Our canonical form is a plain decimal with up to Scale digits.
	s = strings.TrimSpace(s)
	if s == "" {
		*m = Zero()
		return nil
	}
	parsed, err := ParseYuan(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// Value implements driver.Valuer for Price.
func (p Price) Value() (driver.Value, error) {
	return p.String(), nil
}

// Scan implements sql.Scanner for Price.
func (p *Price) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*p = 0
		return nil
	case string:
		return p.scanString(v)
	case []byte:
		return p.scanString(string(v))
	case int64:
		*p = Price(v)
		return nil
	default:
		return fmt.Errorf("money: cannot scan price %T", src)
	}
}

func (p *Price) scanString(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		*p = 0
		return nil
	}
	parsed, err := ParsePrice(s)
	if err != nil {
		// tolerate 4-digit values by padding to 5
		if !strings.Contains(s, ".") {
			return err
		}
		parsed, err = ParsePrice(s + "0")
		if err != nil {
			return errors.New("money: invalid price: " + s)
		}
	}
	*p = parsed
	return nil
}
