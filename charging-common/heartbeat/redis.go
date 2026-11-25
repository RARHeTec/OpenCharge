package heartbeat

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore keeps counters in a Redis hash so they survive gateway restarts
// and are shared by every gateway instance behind the load balancer.
type RedisStore struct{ rdb *redis.Client }

func NewRedisStore(rdb *redis.Client) *RedisStore { return &RedisStore{rdb: rdb} }

func (s *RedisStore) Record(ctx context.Context, deviceID string, gunNo int, fault bool, at time.Time) error {
	k := Key(deviceID, gunNo)
	pipe := s.rdb.Pipeline()
	pipe.HIncrBy(ctx, k, "count", 1)
	if fault {
		pipe.HIncrBy(ctx, k, "fault_count", 1)
	}
	pipe.HSetNX(ctx, k, "first_heartbeat", at.Format(time.RFC3339Nano))
	pipe.HSet(ctx, k, "last_heartbeat", at.Format(time.RFC3339Nano))
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisStore) Get(ctx context.Context, deviceID string, gunNo int) (Stats, bool, error) {
	raw, err := s.rdb.HGetAll(ctx, Key(deviceID, gunNo)).Result()
	if err != nil {
		return Stats{}, false, err
	}
	if len(raw) == 0 {
		return Stats{}, false, nil
	}
	st, err := decode(deviceID, gunNo, raw)
	return st, true, err
}

func (s *RedisStore) List(ctx context.Context, deviceID string) ([]Stats, error) {
	pattern := "heartbeat:*"
	if deviceID != "" {
		pattern = "heartbeat:" + deviceID + ":*"
	}
	keys, err := s.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Stats, 0, len(keys))
	for _, k := range keys {
		raw, err := s.rdb.HGetAll(ctx, k).Result()
		if err != nil || len(raw) == 0 {
			continue
		}
		st, err := decode(raw["device_id"], parseInt(raw["gun_no"]), raw)
		if err == nil {
			// Keys carry the authoritative ids; hash fields may be absent on
			// older records, so always prefer the key.
			if d, g, ok := parseKey(k); ok {
				st.DeviceID, st.GunNo = d, g
			}
			out = append(out, st)
		}
	}
	return out, nil
}

func decode(deviceID string, gunNo int, raw map[string]string) (Stats, error) {
	st := Stats{DeviceID: deviceID, GunNo: gunNo}
	st.Count, _ = strconv.ParseInt(raw["count"], 10, 64)
	st.FaultCount, _ = strconv.ParseInt(raw["fault_count"], 10, 64)
	if v := raw["first_heartbeat"]; v != "" {
		st.FirstHeartbeat, _ = time.Parse(time.RFC3339Nano, v)
	}
	if v := raw["last_heartbeat"]; v != "" {
		st.LastHeartbeat, _ = time.Parse(time.RFC3339Nano, v)
	}
	return st, nil
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseKey(k string) (string, int, bool) {
	// heartbeat:<deviceId>:<gunNo> - the gun number is the final segment.
	for i := len(k) - 1; i >= 0; i-- {
		if k[i] == ':' {
			g, err := strconv.Atoi(k[i+1:])
			if err != nil {
				return "", 0, false
			}
			return k[len("heartbeat:"):i], g, true
		}
	}
	return "", 0, false
}
