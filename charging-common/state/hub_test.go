package state

import (
	"sync"
	"testing"
	"time"
)

func TestHubBroadcastAndUnsubscribe(t *testing.T) {
	h := NewHub()
	a, b := h.Subscribe(), h.Subscribe()
	if h.SubscriberCount() != 2 {
		t.Fatalf("subs = %d", h.SubscriberCount())
	}
	h.Publish(Event{Type: "status", Connector: ConnectorState{DeviceID: "D1", GunNo: 1, Status: "occupied"}})
	for _, ch := range []chan Event{a, b} {
		select {
		case e := <-ch:
			if e.Connector.Status != "occupied" {
				t.Errorf("status = %s", e.Connector.Status)
			}
		case <-time.After(time.Second):
			t.Fatal("no event delivered")
		}
	}
	h.Unsubscribe(a)
	if h.SubscriberCount() != 1 {
		t.Errorf("subs after unsubscribe = %d", h.SubscriberCount())
	}
	if _, ok := <-a; ok {
		t.Error("unsubscribed channel should be closed")
	}
	h.Unsubscribe(b)
	if h.SubscriberCount() != 0 {
		t.Errorf("subs = %d", h.SubscriberCount())
	}
}

func TestHubSlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	slow := h.Subscribe() // never read
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			h.Publish(Event{Type: "status"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on slow subscriber")
	}
	h.Unsubscribe(slow)
}

func TestHubConcurrentSubscribePublish(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := h.Subscribe()
			h.Publish(Event{Type: "status"})
			<-ch
			h.Unsubscribe(ch)
		}()
	}
	wg.Wait()
	if h.SubscriberCount() != 0 {
		t.Errorf("leaked subscribers: %d", h.SubscriberCount())
	}
}
