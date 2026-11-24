package state

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore persists connector state in Redis with a TTL so stale guns
// auto-expire after a missed heartbeat window.
type RedisStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisStore(rdb *redis.Client, ttl time.Duration) *RedisStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute // 3 missed 10s heartbeats plus margin
	}
	return &RedisStore{rdb: rdb, ttl: ttl}
}

func (s *RedisStore) Update(ctx context.Context, c ConnectorState) error {
	return s.rdb.Set(ctx, Key(c.DeviceID, c.GunNo), marshal(c), s.ttl).Err()
}

func (s *RedisStore) Get(ctx context.Context, deviceID string, gunNo int) (ConnectorState, bool, error) {
	raw, err := s.rdb.Get(ctx, Key(deviceID, gunNo)).Result()
	if err == redis.Nil {
		return ConnectorState{}, false, nil
	}
	if err != nil {
		return ConnectorState{}, false, err
	}
	c, err := unmarshal(raw)
	return c, err == nil, err
}

func (s *RedisStore) List(ctx context.Context, deviceID string) ([]ConnectorState, error) {
	pattern := "connector:*"
	if deviceID != "" {
		pattern = "connector:" + deviceID + ":*"
	}
	keys, err := s.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorState, 0, len(keys))
	for _, k := range keys {
		raw, err := s.rdb.Get(ctx, k).Result()
		if err != nil {
			continue
		}
		c, err := unmarshal(raw)
		if err == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

// NewRedisClient builds a go-redis client from an address.
func NewRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 2 * time.Second})
}

// TrimTenantPrefix is a helper for admin filtering of composite keys.
func TrimTenantPrefix(s string) string {
	return strings.TrimPrefix(s, "connector:")
}
