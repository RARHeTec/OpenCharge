package billing

import (
	"fmt"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

// Period is one of the four time-of-use bands.
type Period string

const (
	Sharp  Period = "sharp"
	Peak   Period = "peak"
	Flat   Period = "flat"
	Valley Period = "valley"
)

// TariffSlot is one of up to 48 half-hour slots in a day.
type TariffSlot struct {
	Slot   int    `json:"slot"` // 0..47
	Period Period `json:"period"`
	// Price per kWh, expressed in money units (scale=4).
	Price money.Money `json:"price"`
}

// TariffVersion is an immutable price snapshot. Orders store its ID so that
// offline transactions can be repriced with the price in effect at charge time.
type TariffVersion struct {
	ID        string
	TenantID  string
	StationID string
	Slots     []TariffSlot
	Effective int64
}

// SlotAt returns the slot covering a unix timestamp (local time slot index must
// be computed by the caller with the station's IANA timezone).
func (v TariffVersion) SlotAt(slot int) (TariffSlot, error) {
	if slot < 0 || slot >= 48 {
		return TariffSlot{}, fmt.Errorf("billing: slot out of range: %d", slot)
	}
	for _, s := range v.Slots {
		if s.Slot == slot {
			return s, nil
		}
	}
	return TariffSlot{}, fmt.Errorf("billing: slot %d not configured", slot)
}

// CalculateFee computes fee from a time-sliced energy bill without any float.
// energyWh is watt-hours; price is Yuan per kWh at scale=4.
func CalculateFee(v TariffVersion, segs []Segment) money.Money {
	total := money.Zero()
	for _, s := range segs {
		slot, err := v.SlotAt(s.Slot)
		if err != nil {
			continue
		}
		// kWh = Wh / 1000; price unit is 0.0001 Yuan/kWh.
		// fee_units = price_units * Wh / 1000 / 10000 * 10000 = price_units * Wh / 1000
		fee := money.FromUnits(priceTimesEnergy(slot.Price, s.EnergyWh))
		total = total.Add(fee)
	}
	return total
}

// priceTimesEnergy = round(priceUnits * energyWh / 1000), half-up.
func priceTimesEnergy(price money.Money, energyWh int64) int64 {
	v := price.Units() * energyWh
	if v >= 0 {
		return (v + 500) / 1000
	}
	return (v - 500) / 1000
}
