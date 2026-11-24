package state

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	now := time.Now()
	c := ConnectorState{DeviceID: "D1", GunNo: 1, Status: "charging", TransactionID: "TXN1", LastSeen: now}
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(ctx, "D1", 1)
	if err != nil || !ok {
		t.Fatalf("get = %v %v %v", got, ok, err)
	}
	if got.Status != "charging" || got.TransactionID != "TXN1" {
		t.Errorf("state = %+v", got)
	}
	c.Status = "idle"
	_ = s.Update(ctx, c)
	got, _, _ = s.Get(ctx, "D1", 1)
	if got.Status != "idle" {
		t.Error("update did not overwrite")
	}
	list, _ := s.List(ctx, "D1")
	if len(list) != 1 {
		t.Errorf("list = %d", len(list))
	}
	list, _ = s.List(ctx, "OTHER")
	if len(list) != 0 {
		t.Errorf("filtered list = %d", len(list))
	}
}

func TestKey(t *testing.T) {
	if got := Key("DEV001", 2); got != "connector:DEV001:2" {
		t.Errorf("key = %s", got)
	}
}
