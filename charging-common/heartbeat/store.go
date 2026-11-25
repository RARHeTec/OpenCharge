// Package heartbeat holds cumulative heartbeat counters per charging gun.
// Unlike connector state (which has a TTL), counters must survive a missed
// heartbeat window so operators can tell "never reported" from "reported 500 times".
package heartbeat

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Stats is the heartbeat history of one connector.
type Stats struct {
	DeviceID       string    `json:"deviceId"`
	GunNo          int       `json:"gunNo"`
	TenantID       string    `json:"tenantId,omitempty"`
	StationID      string    `json:"stationId,omitempty"`
	Count          int64     `json:"count"`
	FaultCount     int64     `json:"faultCount"`
	FirstHeartbeat time.Time `json:"firstHeartbeat"`
	LastHeartbeat  time.Time `json:"lastHeartbeat"`
}

type Store interface {
	// Record increments the counter for one heartbeat. fault=true records a
	// device-reported fault instead of a normal heartbeat.
	Record(ctx context.Context, deviceID string, gunNo int, fault bool, at time.Time) error
	Get(ctx context.Context, deviceID string, gunNo int) (Stats, bool, error)
	List(ctx context.Context, deviceID string) ([]Stats, error)
}

func Key(deviceID string, gunNo int) string {
	return fmt.Sprintf("heartbeat:%s:%d", deviceID, gunNo)
}

// MemoryStore keeps counters in process. A mutex makes concurrent device
// readers increment safely.
type MemoryStore struct {
	mu    sync.Mutex
	stats map[string]*Stats
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{stats: make(map[string]*Stats)}
}

func (s *MemoryStore) Record(_ context.Context, deviceID string, gunNo int, fault bool, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := Key(deviceID, gunNo)
	st, ok := s.stats[k]
	if !ok {
		st = &Stats{DeviceID: deviceID, GunNo: gunNo, FirstHeartbeat: at}
		s.stats[k] = st
	}
	st.Count++
	if fault {
		st.FaultCount++
	}
	st.LastHeartbeat = at
	return nil
}

func (s *MemoryStore) Get(_ context.Context, deviceID string, gunNo int) (Stats, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.stats[Key(deviceID, gunNo)]
	if !ok {
		return Stats{}, false, nil
	}
	return *st, true, nil
}

func (s *MemoryStore) List(_ context.Context, deviceID string) ([]Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Stats, 0, len(s.stats))
	for _, st := range s.stats {
		if deviceID == "" || st.DeviceID == deviceID {
			out = append(out, *st)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeviceID == out[j].DeviceID {
			return out[i].GunNo < out[j].GunNo
		}
		return out[i].DeviceID < out[j].DeviceID
	})
	return out, nil
}
