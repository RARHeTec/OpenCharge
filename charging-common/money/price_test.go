package money

import "testing"

func TestParsePrice(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1.30000", 130000},
		{"0.62345", 62345},
		{"0", 0},
		{"12.5", 1250000},
	}
	for _, c := range cases {
		got, err := ParsePrice(c.in)
		if err != nil {
			t.Fatalf("ParsePrice(%q): %v", c.in, err)
		}
		if got.Units() != c.want {
			t.Errorf("ParsePrice(%q) = %d, want %d", c.in, got.Units(), c.want)
		}
	}
	if _, err := ParsePrice("0.123456"); err == nil {
		t.Error("expected precision error for 6 fractional digits")
	}
	if got := MustParsePrice("1.30000").String(); got != "1.30000" {
		t.Errorf("String() = %s", got)
	}
}

func TestPriceMulEnergy(t *testing.T) {
	// 1.30000 Yuan/kWh x 2.5 kWh (25000 units) = 3.2500 Yuan
	got := MustParsePrice("1.30000").MulEnergy(25000)
	if got.String() != "3.2500" {
		t.Errorf("amount = %s, want 3.2500", got.String())
	}
	// 0.62345 x 1.5 kWh = 0.935175 -> half-up at scale=4 -> 0.9352
	got = MustParsePrice("0.62345").MulEnergy(15000)
	if got.String() != "0.9352" {
		t.Errorf("amount = %s, want 0.9352", got.String())
	}
}

func TestWithinTolerance(t *testing.T) {
	tol := DefaultTolerance()
	amount := MustParseYuan("30") // 300000 units
	// 0.01 Yuan = 100 units: exactly at absolute band, and 100 <= 0.1% of 30 (300)
	if !WithinTolerance(MustParseYuan("0.01"), amount, tol) {
		t.Error("0.01 on 30 Yuan should pass")
	}
	// 0.02 Yuan = 200 units: exceeds absolute band
	if WithinTolerance(MustParseYuan("0.02"), amount, tol) {
		t.Error("0.02 on 30 Yuan should fail")
	}
	// Small order: 0.75 Yuan, 0.01 diff is 1.3% -> relative band is only 7 units
	if WithinTolerance(MustParseYuan("0.01"), MustParseYuan("0.75"), tol) {
		t.Error("0.01 on 0.75 Yuan should fail the relative band")
	}
	// exact match always passes, even zero amount
	if !WithinTolerance(Zero(), Zero(), tol) {
		t.Error("zero diff should pass")
	}
}

func TestLowerOf(t *testing.T) {
	a, b := MustParseYuan("0.76"), MustParseYuan("0.75")
	if got := LowerOf(a, b); got.String() != "0.7500" {
		t.Errorf("LowerOf = %s", got.String())
	}
}
