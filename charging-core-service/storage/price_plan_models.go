package storage

import (
	"fmt"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

// Period is the tariff band used by the YunKuaiChong V1.6 model.
// Protocol codes: 0=尖, 1=峰, 2=平, 3=谷.
type Period string

const (
	PeriodSharp  Period = "sharp"  // 尖
	PeriodPeak   Period = "peak"   // 峰
	PeriodFlat   Period = "flat"   // 平
	PeriodValley Period = "valley" // 谷
)

func (p Period) ProtocolCode() byte {
	switch p {
	case PeriodSharp:
		return 0
	case PeriodPeak:
		return 1
	case PeriodFlat:
		return 2
	case PeriodValley:
		return 3
	default:
		return 2 // safe default: flat rate
	}
}

func (p Period) Label() string {
	switch p {
	case PeriodSharp:
		return "尖"
	case PeriodPeak:
		return "峰"
	case PeriodFlat:
		return "平"
	case PeriodValley:
		return "谷"
	default:
		return string(p)
	}
}

// PriceSegment is one editable time window in the admin "调整定价" dialog.
// Boundaries are minutes from midnight (0..1440), so 22:30 is 1350.
type PriceSegment struct {
	Seq          int         `json:"seq"`
	StartMin     int         `json:"startMin"`
	EndMin       int         `json:"endMin"`
	Period       Period      `json:"period"`
	ElecPrice    money.Price `json:"elecPrice"`
	ServicePrice money.Price `json:"servicePrice"`
}

// Total is the unit price shown in the read-only 总单价 column.
func (s PriceSegment) Total() money.Price {
	return money.Price(int64(s.ElecPrice) + int64(s.ServicePrice))
}

func (s PriceSegment) TimeRange() string {
	return fmt.Sprintf("%02d:%02d~%02d:%02d",
		s.StartMin/60, s.StartMin%60, s.EndMin/60, s.EndMin%60)
}

// Validate checks a segment's boundaries and prices.
func (s PriceSegment) Validate() error {
	if s.StartMin < 0 || s.EndMin > 1440 || s.StartMin >= s.EndMin {
		return fmt.Errorf("invalid time range %d-%d", s.StartMin, s.EndMin)
	}
	if int64(s.ElecPrice) < 0 || int64(s.ServicePrice) < 0 {
		return fmt.Errorf("negative price in segment %d", s.Seq)
	}
	switch s.Period {
	case PeriodSharp, PeriodPeak, PeriodFlat, PeriodValley:
	default:
		return fmt.Errorf("unknown period %q", s.Period)
	}
	return nil
}

// PricePlan is one saved pricing version, scoped to a tenant/station/deviceType.
// Publishing a new plan marks the previous one invalid.
type PricePlan struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	StationID    string         `json:"stationId"`
	DeviceType   string         `json:"deviceType"`
	EffectiveAt  time.Time      `json:"effectiveAt"`
	EffectiveNow bool           `json:"effectiveNow"`
	Status       string         `json:"status"` // valid | invalid
	Source       string         `json:"source"` // manual | imported | copied
	Operator     string         `json:"operator"`
	Note         string         `json:"note"`
	// ModelCode is the 2-byte BCD billing-model number sent to piles in 0x58.
	ModelCode    int            `json:"modelCode"`
	Segments     []PriceSegment `json:"segments"`
	CreatedAt    time.Time      `json:"createdAt"`
	// Delivery info, filled by the 0x58 downlink phase later.
	DeliveredCount int `json:"deliveredCount"`
	SuccessCount   int `json:"successCount"`
}

// ValidatePlan checks full-day coverage with no gaps or overlaps.
func ValidatePlan(plan PricePlan) error {
	if plan.TenantID == "" || plan.StationID == "" {
		return fmt.Errorf("tenantId and stationId required")
	}
	if plan.DeviceType == "" {
		return fmt.Errorf("deviceType required")
	}
	if len(plan.Segments) == 0 {
		return fmt.Errorf("at least one price segment required")
	}
	// Segments must be contiguous from 00:00 to 24:00.
	cursor := 0
	for i, seg := range plan.Segments {
		if err := seg.Validate(); err != nil {
			return err
		}
		if seg.Seq != i+1 {
			return fmt.Errorf("segment seq %d at index %d", seg.Seq, i)
		}
		if seg.StartMin != cursor {
			return fmt.Errorf("segment %d starts at %d, expected %d (gap/overlap)", seg.Seq, seg.StartMin, cursor)
		}
		cursor = seg.EndMin
	}
	if cursor != 1440 {
		return fmt.Errorf("segments cover %d minutes, want 1440", cursor)
	}
	return nil
}


// PriceToProtocol converts an editable PricePlan into the 0x58 billing model.
// It maps variable minute windows onto 48 half-hour slots, and collapses the
// per-segment prices into one rate pair per period (the protocol only has 8
// price fields). Prices must be identical within a period or the plan cannot
// be delivered faithfully.
func PriceToProtocol(plan PricePlan) (modelCode uint16, rates struct {
	SharpElec, SharpService, PeakElec, PeakService,
	FlatElec, FlatService, ValleyElec, ValleyService uint64
}, slots [48]byte, err error) {
	if len(plan.Segments) == 0 {
		err = fmt.Errorf("no segments")
		return
	}
	// Fill slots by checking which minute window contains each slot.
	for i := 0; i < 48; i++ {
		minute := i * 30
		found := false
		for _, seg := range plan.Segments {
			if minute >= seg.StartMin && minute < seg.EndMin {
				slots[i] = seg.Period.ProtocolCode()
				found = true
				break
			}
		}
		if !found {
			err = fmt.Errorf("slot %d (%d min) uncovered", i, minute)
			return
		}
	}
	// First price pair seen for each period wins; a mismatch is an error.
	type pair struct{ elec, svc uint64 }
	seen := make(map[Period]pair)
	for _, seg := range plan.Segments {
		p := pair{uint64(seg.ElecPrice), uint64(seg.ServicePrice)}
		if old, ok := seen[seg.Period]; ok {
			if old != p {
				err = fmt.Errorf("period %s has inconsistent prices (%d/%d vs %d/%d); "+
					"protocol allows one rate per period", seg.Period, old.elec, old.svc, p.elec, p.svc)
				return
			}
		} else {
			seen[seg.Period] = p
		}
	}
	get := func(period Period) pair {
		if p, ok := seen[period]; ok {
			return p
		}
		return pair{}
	}
	sharp, peak := get(PeriodSharp), get(PeriodPeak)
	flat, valley := get(PeriodFlat), get(PeriodValley)
	rates.SharpElec, rates.SharpService = sharp.elec, sharp.svc
	rates.PeakElec, rates.PeakService = peak.elec, peak.svc
	rates.FlatElec, rates.FlatService = flat.elec, flat.svc
	rates.ValleyElec, rates.ValleyService = valley.elec, valley.svc
	modelCode = uint16(plan.ModelCode)
	return
}
