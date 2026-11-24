package protocol

import (
	"testing"
	"time"
)

func TestCP56RoundTrip(t *testing.T) {
	loc := time.Local
	want := time.Date(2026, 11, 25, 13, 45, 30, 0, loc)
	b := EncodeCP56Time2a(want)
	if len(b) != 7 {
		t.Fatalf("len = %d", len(b))
	}
	got, err := ParseCP56Time2a(b, loc)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Errorf("got %s want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestCP56RejectsBadMonth(t *testing.T) {
	b := EncodeCP56Time2a(time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local))
	b[5] = 13 // month 13
	if _, err := ParseCP56Time2a(b, time.Local); err == nil {
		t.Error("expected invalid month error")
	}
}

func TestCP56ShortBuffer(t *testing.T) {
	if _, err := ParseCP56Time2a([]byte{1, 2, 3}, time.Local); err == nil {
		t.Error("expected length error")
	}
}
