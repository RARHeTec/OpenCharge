package lock

import (
	"context"
	"time"
)

// Locker is the port for connector-level distributed locking. Production uses
// RedisLocker; tests and local integration use MemoryLocker.
type Locker interface {
	Acquire(ctx context.Context, deviceID string, gunNo int, token string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, deviceID string, gunNo int, token string) (bool, error)
}

var _ Locker = (*ConnectorLock)(nil)
