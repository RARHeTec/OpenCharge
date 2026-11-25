package storage

import (
	"strings"
	"testing"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

// The operator's screenshot: 6 variable windows, valley/flat repeated with
// the same price, one peak band. It must expand losslessly into 48 slots.
func screenshotPlan() PricePlan {
	seg := func(seq, start, end int, period Period, elec, svc string) PriceSegment {
		return PriceSegment{Seq: seq, StartMin: start, EndMin: end, Period: period,
			ElecPrice: money.MustParsePrice(elec), ServicePrice: money.MustParsePrice(svc)}
	}
	return PricePlan{ModelCode: 1, Segments: []PriceSegment{
		seg(1, 0, 420, PeriodValley, "0.6", "0.18"),
		seg(2, 420, 660, PeriodFlat, "0.88", "0.15"),
		seg(3, 660, 840, PeriodValley, "0.6", "0.18"),
		seg(4, 840, 960, PeriodFlat, "0.88", "0.15"),
		seg(5, 960, 1350, PeriodPeak, "1.195", "0.095"),
		seg(6, 1350, 1440, PeriodFlat, "0.88", "0.15"),
	}}
}

func TestPriceToProtocolScreenshot(t *testing.T) {
	code, rates, slots, err := PriceToProtocol(screenshotPlan())
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Errorf("model code = %d", code)
	}
	if rates.ValleyElec != 60000 || rates.ValleyService != 18000 {
		t.Errorf("valley rates = %d/%d", rates.ValleyElec, rates.ValleyService)
	}
	if rates.FlatElec != 88000 || rates.FlatService != 15000 {
		t.Errorf("flat rates = %d/%d", rates.FlatElec, rates.FlatService)
	}
	if rates.PeakElec != 119500 {
		t.Errorf("peak elec = %d", rates.PeakElec)
	}
	// 00:00-07:00 = 14 slots valley; 11:00-14:00 = 6 slots valley.
	valleyCount := 0
	for i, s := range slots {
		if s == 3 {
			valleyCount++
		}
		if i == 0 && s != 3 {
			t.Error("slot 0 should be valley")
		}
		if i == 14 && s != 2 {
			t.Error("slot 14 should be flat")
		}
		if i == 32 && s != 1 {
			t.Error("slot 32 (16:00) should be peak")
		}
	}
	if valleyCount != 20 {
		t.Errorf("valley slots = %d, want 14+6=20", valleyCount)
	}
}

func TestPriceToProtocolRejectsInconsistentPeriod(t *testing.T) {
	p := screenshotPlan()
	// Make the second valley window a different price.
	p.Segments[2].ElecPrice = money.MustParsePrice("0.7")
	_, _, _, err := PriceToProtocol(p)
	if err == nil || !strings.Contains(err.Error(), "inconsistent") {
		t.Fatalf("err = %v, want inconsistent period", err)
	}
}

func TestPriceToProtocolRejectsUncoveredSlot(t *testing.T) {
	p := screenshotPlan()
	p.Segments[0].EndMin = 330 // gap 330-420 covers slots 11,12,13
	_, _, _, err := PriceToProtocol(p)
	if err == nil || !strings.Contains(err.Error(), "uncovered") {
		t.Fatalf("err = %v, want uncovered slot", err)
	}
}
