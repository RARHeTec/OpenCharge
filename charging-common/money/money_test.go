package money

import "testing"

func TestParseYuan(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1", 10000},
		{"0.6234", 6234},
		{"-12.5", -125000},
		{"0.0001", 1},
		{"1.0050", 10050},
	}
	for _, c := range cases {
		got, err := ParseYuan(c.in)
		if err != nil {
			t.Fatalf("ParseYuan(%q): %v", c.in, err)
		}
		if got.Units() != c.want {
			t.Errorf("ParseYuan(%q) = %d, want %d", c.in, got.Units(), c.want)
		}
	}
	if _, err := ParseYuan("0.12345"); err == nil {
		t.Error("expected precision error")
	}
}

func TestRoundTrip(t *testing.T) {
	m := MustParseYuan("0.6234")
	if m.String() != "0.6234" {
		t.Fatalf("String() = %s", m.String())
	}
	if m.ToCents() != 62 {
		t.Errorf("ToCents() = %d, want 62", m.ToCents())
	}
	if m.FormatYuan() != "0.62" {
		t.Errorf("FormatYuan() = %s", m.FormatYuan())
	}
}

func TestHalfUp(t *testing.T) {
	if got := MustParseYuan("1.005").ToCents(); got != 101 {
		t.Errorf("1.005 -> %d cents, want 101", got)
	}
	if got := MustParseYuan("0.995").ToCents(); got != 100 {
		t.Errorf("0.995 -> %d cents, want 100", got)
	}
}

func TestMulRate(t *testing.T) {
	price := MustParseYuan("0.6234")
	kwh := int64(25000) // 2.5 kWh
	got := price.MulQty(kwh)
	if got.String() != "1.5585" {
		t.Errorf("billing = %s, want 1.5585", got.String())
	}
}

func TestNoFloatInType(t *testing.T) {
	if UnitsPerYuan != 10000 || Scale != 4 {
		t.Fatal("scale constants changed")
	}
}
