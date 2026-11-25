package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

func demoPlan(id, deviceType string, status string) PricePlan {
	return PricePlan{
		ID: id, TenantID: "T1", StationID: "S1", DeviceType: deviceType,
		EffectiveAt: time.Now(), EffectiveNow: true, Status: status,
		Source: "manual", Operator: "admin", CreatedAt: time.Now(),
		Segments: []PriceSegment{
			{Seq: 1, StartMin: 0, EndMin: 420, Period: PeriodValley,
				ElecPrice: money.MustParsePrice("0.6"), ServicePrice: money.MustParsePrice("0.18")},
			{Seq: 2, StartMin: 420, EndMin: 1440, Period: PeriodFlat,
				ElecPrice: money.MustParsePrice("0.88"), ServicePrice: money.MustParsePrice("0.15")},
		},
	}
}

func TestPricePlanCurrentAndHistory(t *testing.T) {
	s := NewMemoryPricePlanStore()
	ctx := context.Background()
	_ = s.Save(ctx, demoPlan("p1", "DC160", "invalid"))
	_ = s.Save(ctx, demoPlan("p2", "DC160", "valid"))
	_ = s.Save(ctx, demoPlan("p3", "DC240", "valid"))

	cur, ok, _ := s.Current(ctx, "T1", "S1", "DC160")
	if !ok || cur.ID != "p2" {
		t.Fatalf("current = %s ok=%v", cur.ID, ok)
	}
	hist, _ := s.List(ctx, "T1", "S1", "DC160", true)
	if len(hist) != 1 || hist[0].ID != "p1" {
		t.Fatalf("history = %+v", hist)
	}
	currents, _ := s.List(ctx, "T1", "S1", "", false)
	if len(currents) != 2 {
		t.Errorf("currents = %d", len(currents))
	}
}

func TestInvalidateCurrent(t *testing.T) {
	s := NewMemoryPricePlanStore()
	ctx := context.Background()
	_ = s.Save(ctx, demoPlan("p1", "DC160", "valid"))
	_ = s.Save(ctx, demoPlan("p2", "DC240", "valid"))
	_ = s.InvalidateCurrent(ctx, "T1", "S1", "DC160")

	_, ok, _ := s.Current(ctx, "T1", "S1", "DC160")
	if ok {
		t.Error("DC160 should have no valid plan")
	}
	_, ok, _ = s.Current(ctx, "T1", "S1", "DC240")
	if !ok {
		t.Error("DC240 must remain valid (scope isolation)")
	}
}

func TestValidatePlan(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*PricePlan)
		wantErr string
	}{
		{"good", func(p *PricePlan) {}, ""},
		{"gap", func(p *PricePlan) { p.Segments[1].StartMin = 480 }, "expected 420"},
		{"not full day", func(p *PricePlan) { p.Segments[1].EndMin = 1400 }, "1440"},
		{"negative price", func(p *PricePlan) { p.Segments[0].ElecPrice = -1 }, "negative"},
		{"bad period", func(p *PricePlan) { p.Segments[0].Period = "bogus" }, "unknown period"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := demoPlan("p", "DC160", "valid")
			tt.mutate(&p)
			err := ValidatePlan(p)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSegmentTotal(t *testing.T) {
	seg := PriceSegment{
		ElecPrice: money.MustParsePrice("0.6"),
		ServicePrice: money.MustParsePrice("0.18"),
	}
	if got := seg.Total().String(); got != "0.78000" {
		t.Errorf("total = %s", got)
	}
	if got := seg.TimeRange(); got != "00:00~00:00" {
		t.Errorf("range = %s", got)
	}
}

func TestProtocolCode(t *testing.T) {
	cases := map[Period]byte{
		PeriodSharp: 0, PeriodPeak: 1, PeriodFlat: 2, PeriodValley: 3,
	}
	for p, want := range cases {
		if got := p.ProtocolCode(); got != want {
			t.Errorf("%s = %d, want %d", p, got, want)
		}
	}
}
