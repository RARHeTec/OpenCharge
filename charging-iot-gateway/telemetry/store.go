// Package telemetry buffers device MeterValues and flushes them in batches
// to a time-series store. The fast path never blocks the settlement path.
package telemetry

import (
	"sync"
	"time"
)

// Sample is one point from a device meter report.
type Sample struct {
	Timestamp  time.Time
	DeviceID   string
	ConnectorID int
	VoltageMV  int64
	CurrentMA  int64
	PowerW     int64
	EnergyWh   int64
	SOCX100    int32
	CableTemp  int32
}

// Store receives batches of telemetry samples.
type Store interface {
	WriteBatch(ctxContext any, samples []Sample) error
	Close() error
}

// MemoryStore keeps the last samples in memory for local runs and tests.
type MemoryStore struct {
	mu      sync.Mutex
	samples []Sample
	batches int
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (s *MemoryStore) WriteBatch(_ any, samples []Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, samples...)
	s.batches++
	return nil
}
func (s *MemoryStore) Close() error { return nil }

func (s *MemoryStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.samples)
}
func (s *MemoryStore) Batches() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batches
}
