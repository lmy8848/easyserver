package database

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisRunner is the direct-connection channel for Redis instances. It mirrors
// driverSQLRunner (sql_runner.go) for the SQL engines, but Redis has no SQL —
// the operations are key-browser primitives (list DBs, scan keys, read/write
// values, TTL, flush) instead of Query/Exec. Service owns one and forwards the
// key-browser methods to it.
type redisRunner struct {
	// conns is keyed by (instance, logical db) — go-redis fixes the DB on
	// client construction, and Redis instances only have 0-15, so one client
	// per used db is cheap.
	mu    sync.Mutex
	conns map[redisPoolKey]*redis.Client
}

type redisPoolKey struct {
	instanceID int64
	db         int
}

func newRedisRunner() *redisRunner {
	return &redisRunner{conns: make(map[redisPoolKey]*redis.Client)}
}

// Close closes every client belonging to an instance (called when the instance
// is uninstalled/destroyed).
func (r *redisRunner) Close(instanceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, c := range r.conns {
		if key.instanceID == instanceID {
			c.Close()
			delete(r.conns, key)
		}
	}
}

func (r *redisRunner) clientFor(ctx context.Context, inst *DBInstance, db int) (*redis.Client, error) {
	key := redisPoolKey{instanceID: inst.ID, db: db}

	r.mu.Lock()
	if c, ok := r.conns[key]; ok {
		r.mu.Unlock()
		return c, nil
	}
	c := redis.NewClient(&redis.Options{
		Addr:        fmt.Sprintf("127.0.0.1:%d", inst.Port),
		Password:    inst.AdminPassword,
		DB:          db,
		DialTimeout: 5 * time.Second,
	})
	r.conns[key] = c
	r.mu.Unlock()

	// Ping establishes the first connection so a dead container / wrong mapping
	// surfaces here as a clear error, not on the first command.
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("无法连接 Redis 实例: %w", err)
	}
	return c, nil
}

// ListDBs reports the non-empty logical databases (DBSIZE > 0) in index order.
func (r *redisRunner) ListDBs(ctx context.Context, inst *DBInstance) ([]RedisDB, error) {
	var dbs []RedisDB
	for i := 0; i < 16; i++ {
		c, err := r.clientFor(ctx, inst, i)
		if err != nil {
			return nil, err
		}
		n, err := c.DBSize(ctx).Result()
		if err != nil {
			return nil, err
		}
		if n > 0 {
			dbs = append(dbs, RedisDB{Index: i, Size: n})
		}
	}
	return dbs, nil
}

// ScanKeys pages through keys with SCAN (type/TTL/size filled per key).
func (r *redisRunner) ScanKeys(ctx context.Context, inst *DBInstance, db int, cursor uint64, pattern string, count int64) ([]RedisKey, uint64, error) {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return nil, 0, err
	}
	keys, next, err := c.Scan(ctx, cursor, pattern, count).Result()
	if err != nil {
		return nil, 0, err
	}
	out := make([]RedisKey, 0, len(keys))
	for _, k := range keys {
		rk := RedisKey{Name: k}
		if t, err := c.Type(ctx, k).Result(); err == nil {
			rk.Type = t
		}
		if d, err := c.TTL(ctx, k).Result(); err == nil {
			rk.TTL = int64(d / time.Second) // -1 (no expiry) and -2 (gone) survive division
		}
		if sz, err := c.MemoryUsage(ctx, k, 0).Result(); err == nil {
			rk.Size = sz
		}
		out = append(out, rk)
	}
	return out, next, nil
}

// GetValue reads and decodes a key's value by its type.
func (r *redisRunner) GetValue(ctx context.Context, inst *DBInstance, db int, key string) (*RedisValue, error) {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return nil, err
	}
	typ, err := c.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	switch typ {
	case "string":
		v, err := c.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		return &RedisValue{Type: "string", Value: v}, nil
	case "hash":
		v, err := c.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		return &RedisValue{Type: "hash", Value: v}, nil
	case "list":
		v, err := c.LRange(ctx, key, 0, -1).Result()
		if err != nil {
			return nil, err
		}
		return &RedisValue{Type: "list", Value: v}, nil
	case "set":
		v, err := c.SMembers(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		sort.Strings(v)
		return &RedisValue{Type: "set", Value: v}, nil
	case "zset":
		zs, err := c.ZRangeWithScores(ctx, key, 0, -1).Result()
		if err != nil {
			return nil, err
		}
		out := make([]RedisZMember, 0, len(zs))
		for _, z := range zs {
			out = append(out, RedisZMember{Member: fmt.Sprintf("%v", z.Member), Score: z.Score})
		}
		return &RedisValue{Type: "zset", Value: out}, nil
	default:
		// stream / rejson / module types: no structured decode, report the type
		// only so the UI can show it's unsupported for editing.
		return &RedisValue{Type: typ, Value: nil}, nil
	}
}

// SetValue writes a string key (the UI only edits string values inline).
func (r *redisRunner) SetValue(ctx context.Context, inst *DBInstance, db int, key, value string, ttl time.Duration) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, value, ttl).Err()
}

func (r *redisRunner) DelKeys(ctx context.Context, inst *DBInstance, db int, keys ...string) (int64, error) {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return 0, err
	}
	return c.Del(ctx, keys...).Result()
}

func (r *redisRunner) Expire(ctx context.Context, inst *DBInstance, db int, key string, ttl time.Duration) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.Expire(ctx, key, ttl).Err()
}

func (r *redisRunner) Persist(ctx context.Context, inst *DBInstance, db int, key string) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.Persist(ctx, key).Err()
}

func (r *redisRunner) FlushDB(ctx context.Context, inst *DBInstance, db int) error {
	c, err := r.clientFor(ctx, inst, db)
	if err != nil {
		return err
	}
	return c.FlushDB(ctx).Err()
}

// BgSave triggers an asynchronous persistence; the caller still copies
// dump.rdb out of the container (a container file operation the driver cannot
// do). Used by backupRedis.
func (r *redisRunner) BgSave(ctx context.Context, inst *DBInstance) error {
	c, err := r.clientFor(ctx, inst, 0)
	if err != nil {
		return err
	}
	return c.BgSave(ctx).Err()
}

// ConfigGetAll reads every runtime config parameter (CONFIG GET *). Used by the
// structured-config GET to filter the panel-managed params.
func (r *redisRunner) ConfigGetAll(ctx context.Context, inst *DBInstance) (map[string]string, error) {
	c, err := r.clientFor(ctx, inst, 0)
	if err != nil {
		return nil, err
	}
	return c.ConfigGet(ctx, "*").Result()
}

// ConfigGet reads one config parameter (CONFIG GET name). Used by restoreRedis
// to check appendonly before overwriting dump.rdb.
func (r *redisRunner) ConfigGet(ctx context.Context, inst *DBInstance, name string) (string, error) {
	c, err := r.clientFor(ctx, inst, 0)
	if err != nil {
		return "", err
	}
	vals, err := c.ConfigGet(ctx, name).Result()
	if err != nil {
		return "", err
	}
	return vals[name], nil
}

// ConfigSet applies one config parameter online (CONFIG SET).
func (r *redisRunner) ConfigSet(ctx context.Context, inst *DBInstance, name, value string) error {
	c, err := r.clientFor(ctx, inst, 0)
	if err != nil {
		return err
	}
	return c.ConfigSet(ctx, name, value).Err()
}

// ConfigRewrite persists the running config back to the config file
// (CONFIG REWRITE — the panel-seeded redis.conf on the config volume).
func (r *redisRunner) ConfigRewrite(ctx context.Context, inst *DBInstance) error {
	c, err := r.clientFor(ctx, inst, 0)
	if err != nil {
		return err
	}
	return c.ConfigRewrite(ctx).Err()
}

// --- Redis key browser operations ---
//
// Redis has no SQL: the "数据库" tab renders a key browser instead. All ops go
// through the direct go-redis channel (s.redisOps), addressed by logical DB
// index (0-15).

// getRedisInstance loads an instance for a Redis operation and validates the
// logical DB index.
func (s *Service) getRedisInstance(ctx context.Context, instanceID int64, db int) (*DBInstance, error) {
	if db < 0 || db > 15 {
		return nil, fmt.Errorf("invalid redis db index: %d", db)
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil || instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	return instance, nil
}

// ListRedisDBs returns the non-empty logical databases (index + key count).
func (s *Service) ListRedisDBs(ctx context.Context, instanceID int64) ([]RedisDB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, 0)
	if err != nil {
		return nil, err
	}
	return s.redisFor().ListDBs(ctx, instance)
}

// ScanRedisKeys pages through keys in a logical DB (SCAN cursor).
func (s *Service) ScanRedisKeys(ctx context.Context, instanceID int64, db int, cursor uint64, pattern string, count int64) ([]RedisKey, uint64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return nil, 0, err
	}
	return s.redisFor().ScanKeys(ctx, instance, db, cursor, pattern, count)
}

// GetRedisValue reads one key's decoded value.
func (s *Service) GetRedisValue(ctx context.Context, instanceID int64, db int, key string) (*RedisValue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}
	return s.redisFor().GetValue(ctx, instance, db, key)
}

// SetRedisValue writes a string key (with optional TTL seconds; ttl <= 0 = no expiry).
func (s *Service) SetRedisValue(ctx context.Context, instanceID int64, db int, key, value string, ttl int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	return s.redisFor().SetValue(ctx, instance, db, key, value, time.Duration(ttl)*time.Second)
}

// DeleteRedisKeys deletes one or more keys; returns the number removed.
func (s *Service) DeleteRedisKeys(ctx context.Context, instanceID int64, db int, keys []string) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, fmt.Errorf("no keys specified")
	}
	return s.redisFor().DelKeys(ctx, instance, db, keys...)
}

// ExpireRedisKey sets a key's expiry (ttl seconds; ttl <= 0 clears to none).
func (s *Service) ExpireRedisKey(ctx context.Context, instanceID int64, db int, key string, ttl int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	return s.redisFor().Expire(ctx, instance, db, key, time.Duration(ttl)*time.Second)
}

// PersistRedisKey removes a key's expiry.
func (s *Service) PersistRedisKey(ctx context.Context, instanceID int64, db int, key string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	return s.redisFor().Persist(ctx, instance, db, key)
}

// FlushRedisDB removes all keys from a logical DB.
func (s *Service) FlushRedisDB(ctx context.Context, instanceID int64, db int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.getRedisInstance(ctx, instanceID, db)
	if err != nil {
		return err
	}
	return s.redisFor().FlushDB(ctx, instance, db)
}
