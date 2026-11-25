package storage

import (
	"context"
	"sort"
	"sync"
)

// MemoryPricePlanStore is the in-process implementation for local runs.
type MemoryPricePlanStore struct {
	mu    sync.RWMutex
	plans map[string]PricePlan
}

func NewMemoryPricePlanStore() *MemoryPricePlanStore {
	return &MemoryPricePlanStore{plans: make(map[string]PricePlan)}
}

func (s *MemoryPricePlanStore) Save(_ context.Context, plan PricePlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.ID] = plan
	return nil
}

func (s *MemoryPricePlanStore) Get(_ context.Context, id string) (PricePlan, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	return p, ok, nil
}

func (s *MemoryPricePlanStore) Current(_ context.Context, tenantID, stationID, deviceType string) (PricePlan, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best PricePlan
	found := false
	for _, p := range s.plans {
		if p.TenantID == tenantID && p.StationID == stationID &&
			p.DeviceType == deviceType && p.Status == "valid" {
			if !found || p.CreatedAt.After(best.CreatedAt) {
				best, found = p, true
			}
		}
	}
	return best, found, nil
}

func (s *MemoryPricePlanStore) List(_ context.Context, tenantID, stationID, deviceType string, history bool) ([]PricePlan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PricePlan, 0, len(s.plans))
	for _, p := range s.plans {
		if tenantID != "" && p.TenantID != tenantID {
			continue
		}
		if stationID != "" && p.StationID != stationID {
			continue
		}
		if deviceType != "" && p.DeviceType != deviceType {
			continue
		}
		if history && p.Status != "invalid" {
			continue
		}
		if !history && p.Status != "valid" {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryPricePlanStore) InvalidateCurrent(_ context.Context, tenantID, stationID, deviceType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.plans {
		if p.TenantID == tenantID && p.StationID == stationID &&
			p.DeviceType == deviceType && p.Status == "valid" {
			p.Status = "invalid"
			s.plans[id] = p
		}
	}
	return nil
}
