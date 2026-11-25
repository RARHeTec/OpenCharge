package heartbeat

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryRecordAndGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := s.Record(ctx, "D1", 1, false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Record(ctx, "D1", 1, true, now.Add(10*time.Second))
	st, ok, _ := s.Get(ctx, "D1", 1)
	if !ok {
		t.Fatal("not found")
	}
	if st.Count != 6 || st.FaultCount != 1 {
		t.Errorf("count=%d fault=%d, want 6/1", st.Count, st.FaultCount)
	}
	if !st.FirstHeartbeat.Equal(now) {
		t.Errorf("first = %s", st.FirstHeartbeat)
	}
	if !st.LastHeartbeat.Equal(now.Add(10 * time.Second)) {
		t.Errorf("last = %s", st.LastHeartbeat)
	}
}

func TestMemoryListFiltersByDevice(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	_ = s.Record(ctx, "D1", 1, false, time.Now())
	_ = s.Record(ctx, "D1", 2, false, time.Now())
	_ = s.Record(ctx, "D2", 1, false, time.Now())

	rows, _ := s.List(ctx, "D1")
	if len(rows) != 2 {
		t.Fatalf("D1 rows = %d", len(rows))
	}
	if rows[0].GunNo != 1 || rows[1].GunNo != 2 {
		t.Errorf("order = %d,%d", rows[0].GunNo, rows[1].GunNo)
	}
	all, _ := s.List(ctx, "")
	if len(all) != 3 {
		t.Errorf("all = %d", len(all))
	}
}

func TestConcurrentRecord(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Record(ctx, "D1", 1, false, time.Now())
		}()
	}
	wg.Wait()
	st, _, _ := s.Get(ctx, "D1", 1)
	if st.Count != 100 {
		t.Errorf("count = %d, want 100 (lost updates)", st.Count)
	}
}

func TestParseKey(t *testing.T) {
	d, g, ok := parseKey("heartbeat:55304121873250:12")
	if !ok || d != "55304121873250" || g != 12 {
		t.Errorf("parse = %s %d %v", d, g, ok)
	}
	if _, _, ok := parseKey("heartbeat:bad"); ok {
		t.Error("expected failure for malformed key")
	}
}
