package storage

import "fmt"

// ValidateTariff rejects snapshots that would break settlement:
// exactly 48 slots, each slot 0..47 present once, price non-negative.
func ValidateTariff(v TariffVersion) error {
	if v.ID == "" {
		return fmt.Errorf("tariff: id required")
	}
	if v.TenantID == "" || v.StationID == "" {
		return fmt.Errorf("tariff: tenant and station required")
	}
	if len(v.Slots) != 48 {
		return fmt.Errorf("tariff: need 48 slots, got %d", len(v.Slots))
	}
	seen := make(map[int]bool, 48)
	for _, sl := range v.Slots {
		if sl.Slot < 0 || sl.Slot > 47 {
			return fmt.Errorf("tariff: slot %d out of range 0..47", sl.Slot)
		}
		if seen[sl.Slot] {
			return fmt.Errorf("tariff: duplicate slot %d", sl.Slot)
		}
		seen[sl.Slot] = true
		if sl.Price < 0 {
			return fmt.Errorf("tariff: negative price at slot %d", sl.Slot)
		}
	}
	return nil
}
