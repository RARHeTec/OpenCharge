// Package lock provides the connector-level distributed lock required before
// any RemoteStartTransaction / RemoteStopTransaction command is dispatched.
package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const Prefix = "lock:connector"

// Key returns the canonical lock key: lock:connector:{deviceId}:{gunNo}
func Key(deviceID string, gunNo int) string {
	return fmt.Sprintf("%s:%s:%d", Prefix, deviceID, gunNo)
}

type ConnectorLock struct {
	rdb *redis.Client
}

func NewConnectorLock(rdb *redis.Client) *ConnectorLock {
	return &ConnectorLock{rdb: rdb}
}

// Acquire attempts to own the connector for ttl. token must be globally unique
// (e.g. TraceId) so that Release cannot delete another caller's lock.
func (l *ConnectorLock) Acquire(ctx context.Context, deviceID string, gunNo int, token string, ttl time.Duration) (bool, error) {
	return l.rdb.SetNX(ctx, Key(deviceID, gunNo), token, ttl).Result()
}

var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
else
  return 0
end
`)

func (l *ConnectorLock) Release(ctx context.Context, deviceID string, gunNo int, token string) (bool, error) {
	v, err := releaseScript.Run(ctx, l.rdb, []string{Key(deviceID, gunNo)}, token).Int64()
	return v == 1, err
}
