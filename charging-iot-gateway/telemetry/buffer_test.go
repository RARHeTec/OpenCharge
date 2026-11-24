package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestBufferFlushesBySize(t *testing.T) {
	mem := NewMemoryStore()
	b := NewBuffer(mem, 3, time.Hour) // disable ticker
	defer b.Close()
	now := time.Now()
	for i := 0; i < 7; i++ {
		b.Push(Sample{Timestamp: now, DeviceID: "D1", ConnectorID: 1, EnergyWh: int64(i)})
	}
	deadline := time.Now().Add(2 * time.Second)
	for mem.Count() < 6 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if mem.Count() != 6 {
		t.Fatalf("written = %d, want 6 (two full batches)", mem.Count())
	}
	if mem.Batches() != 2 {
		t.Errorf("batches = %d", mem.Batches())
	}
}

func TestMemoryStoreWriteBatch(t *testing.T) {
	mem := NewMemoryStore()
	if err := mem.WriteBatch(context.Background(), []Sample{{DeviceID: "D1"}}); err != nil {
		t.Fatal(err)
	}
	if mem.Count() != 1 {
		t.Errorf("count = %d", mem.Count())
	}
}
