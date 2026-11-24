package server

import (
	"sync"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

// memStore holds in-memory aggregates for a running server.
type memStore struct {
	mu    sync.Mutex
	stats Stats
}

func (s *memStore) snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.stats
	if s.stats.LastRealtime != nil {
		v := *s.stats.LastRealtime
		out.LastRealtime = &v
	}
	if s.stats.LastTransaction != nil {
		v := *s.stats.LastTransaction
		out.LastTransaction = &v
	}
	return out
}

func (s *memStore) addLogin()       { s.mu.Lock(); s.stats.LoginCount++; s.mu.Unlock() }
func (s *memStore) addHeartbeat()   { s.mu.Lock(); s.stats.HeartbeatCount++; s.mu.Unlock() }
func (s *memStore) addStartRequest() { s.mu.Lock(); s.stats.StartRequestCount++; s.mu.Unlock() }
func (s *memStore) addRealtime(r protocol.RealtimeBody) {
	s.mu.Lock(); s.stats.RealtimeCount++; s.stats.LastRealtime = &r; s.mu.Unlock()
}
func (s *memStore) addTransaction(t protocol.TransactionBody) {
	s.mu.Lock(); s.stats.TransactionCount++; s.stats.LastTransaction = &t; s.mu.Unlock()
}
