package lock

import (
	"context"
	"sync"
	"time"
)

type memLock struct {
	token   string
	expires time.Time
}

// MemoryLocker is an in-process Locker for tests and single-node integration.
type MemoryLocker struct {
	mu    sync.Mutex
	locks map[string]memLock
}

func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{locks: make(map[string]memLock)}
}

func (m *MemoryLocker) Acquire(_ context.Context, deviceID string, gunNo int, token string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := Key(deviceID, gunNo)
	if cur, ok := m.locks[k]; ok && time.Now().Before(cur.expires) {
		return false, nil
	}
	m.locks[k] = memLock{token: token, expires: time.Now().Add(ttl)}
	return true, nil
}

func (m *MemoryLocker) Release(_ context.Context, deviceID string, gunNo int, token string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := Key(deviceID, gunNo)
	if cur, ok := m.locks[k]; ok && cur.token == token {
		delete(m.locks, k)
		return true, nil
	}
	return false, nil
}

var _ Locker = (*MemoryLocker)(nil)
