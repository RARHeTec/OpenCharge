// Package state holds real-time connector state (gun status, online flags,
// last heartbeat, active transaction). It is deliberately separate from the
// durable order store: state is fast-changing and may be lost on restart.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ConnectorState is the live view of one charging gun.
type ConnectorState struct {
	DeviceID       string    `json:"deviceId"`
	GunNo          int       `json:"gunNo"`
	TenantID       string    `json:"tenantId,omitempty"`
	StationID      string    `json:"stationId,omitempty"`
	Status         string    `json:"status"` // offline | idle | plugged | charging | faulted
	FaultCode      int       `json:"faultCode,omitempty"`
	TransactionID  string    `json:"transactionId,omitempty"`
	EnergyWh       int64     `json:"energyWh"`
	AmountUnits    int64     `json:"amountUnits"` // money scale=4
	LastSeen       time.Time `json:"lastSeen"`
}

type Store interface {
	Update(ctx context.Context, s ConnectorState) error
	Get(ctx context.Context, deviceID string, gunNo int) (ConnectorState, bool, error)
	List(ctx context.Context, deviceID string) ([]ConnectorState, error)
}

func Key(deviceID string, gunNo int) string {
	return fmt.Sprintf("connector:%s:%d", deviceID, gunNo)
}

// MemoryStore is an in-process state store for the default local path.
type MemoryStore struct {
	mu     sync.RWMutex
	states map[string]ConnectorState
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{states: make(map[string]ConnectorState)}
}

func (s *MemoryStore) Update(_ context.Context, c ConnectorState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[Key(c.DeviceID, c.GunNo)] = c
	return nil
}

func (s *MemoryStore) Get(_ context.Context, deviceID string, gunNo int) (ConnectorState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.states[Key(deviceID, gunNo)]
	return c, ok, nil
}

func (s *MemoryStore) List(_ context.Context, deviceID string) ([]ConnectorState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ConnectorState, 0, len(s.states))
	for _, c := range s.states {
		if deviceID == "" || c.DeviceID == deviceID {
			out = append(out, c)
		}
	}
	return out, nil
}

// marshal/unmarshal shared by the Redis implementation.
func marshal(c ConnectorState) string {
	b, _ := json.Marshal(c)
	return string(b)
}

func unmarshal(s string) (ConnectorState, error) {
	var c ConnectorState
	err := json.Unmarshal([]byte(s), &c)
	return c, err
}
