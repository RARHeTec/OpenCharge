package state

import (
	"context"
	"testing"
)

func TestNotifyingStorePublishesUpdates(t *testing.T) {
	hub := NewHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	s := NewNotifyingStore(NewMemoryStore(), hub)
	if err := s.Update(context.Background(), ConnectorState{DeviceID: "D1", GunNo: 1, Status: "occupied"}); err != nil {
		t.Fatal(err)
	}
	e := <-ch
	if e.Connector.Status != "occupied" || e.Type != "status" {
		t.Errorf("event = %+v", e)
	}
	// The wrapped store must still hold the value.
	got, ok, _ := s.Get(context.Background(), "D1", 1)
	if !ok || got.Status != "occupied" {
		t.Errorf("underlying store missing update: %+v ok=%v", got, ok)
	}
}
